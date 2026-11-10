// Command iotbridge carries the simulated modules' telemetry from the MQTT broker to the Core API demo trigger
// (POST /v1/demo/trigger, the demo environment only). It subscribes to t/+/d/+/telemetry, converts each reading with
// internal/iotbridge and sends it with the event ID as Idempotency-Key, so a redelivered reading is one event.
//
// With REAL_MQTT_URL set it also listens to real sensor nodes on another broker (DEC-86, IR524): it subscribes to
// REAL_MQTT_TOPIC (smarthome/ac/# by default) with REAL_MQTT_USER / REAL_MQTT_PASSWORD and records one reading per
// node and minute on the device registered for that node_id (Admin's device register, IR635: the bridge reads
// devices.mqttNodes every 30 seconds), with the nodes' settings, their answers to commands and their last will. The
// only thing it publishes there is a command to the node of the device whose unit is commanded, and only with
// REAL_MQTT_COMMANDS=1 (DEC-89, IR528): the nodes switch a real air conditioner.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var (
	// errLiveBrokerDown fails a live command while the nodes' broker is out of reach.
	errLiveBrokerDown = errors.New("the nodes' broker is not connected")
	// errLiveCommandTimeout fails a live command the nodes' broker did not take in time.
	errLiveCommandTimeout = errors.New("the nodes' broker did not take the command in 5 s")
)

// The bridge's timings: httpTimeout bounds one API call, keepAlive is the MQTT keep-alive, disconnectWait how long (in
// milliseconds) a disconnect waits for the work in flight, clockRefresh how often the scenario clock is read again,
// commandPoll how often the requested commands are fetched, and liveCommandWait how long the nodes' broker has to
// take a live command.
const (
	httpTimeout     = 10 * time.Second
	keepAlive       = 60 * time.Second
	disconnectWait  = 500
	clockRefresh    = 5 * time.Minute
	commandPoll     = 2 * time.Second
	liveCommandWait = 5 * time.Second
	nodeRefresh     = 30 * time.Second
)

// The bridge's workers and the places in each one's queue: 3,000 devices at 9 seconds are about 330 batches a second,
// and a call to the API takes a few milliseconds, so 32 workers leave a wide margin.
const (
	workers     = 32
	workerQueue = 256
)

// defaultEquipmentURL is the equipment-api service inside the compose network, which has no TLS.
const defaultEquipmentURL = "http://equipment-api:8080" //nolint:revive // unsecure-url-scheme: private network

func main() {
	broker := flag.String("broker", envOr("MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
	api := flag.String(
		"api",
		envOr("CORE_API_URL", "http://localhost:8080"),
		"Core API base URL (the gateway)",
	)
	equipment := flag.String(
		"equipment",
		envOr("EQUIPMENT_API_URL", defaultEquipmentURL),
		"equipment API base URL: the internal queries of the control domain are served there",
	)
	tenant := flag.String(
		"tenant",
		seed.ID("tenant-a").String(),
		"tenant whose commands the bridge sends (the demo tenant by default)",
	)

	flag.Parse()

	err := run(*broker, *api, *equipment, *tenant)
	if err != nil {
		log.Fatal(err)
	}
}

// run bridges the brokers and the API until a signal ends it; main exits on its error once its deferred clean-up has
// run.
func run(broker, api, equipment, tenant string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fleet, err := devicesim.MockFleet()
	if err != nil {
		return apperr.Wrap(err, "devicesim.MockFleet")
	}

	// devices outside the demo fixture (the load test's) come from a file the environment names
	extra, err := iotbridge.LoadLinks(os.Getenv("IOT_LINKS_FILE"))
	if err != nil {
		return apperr.Wrap(err, "iotbridge.LoadLinks")
	}

	links := append(
		append([]iotbridge.Link{}, iotbridge.Links()...),
		iotbridge.FleetLinks(fleet)...)
	resolver := iotbridge.NewResolver(append(links, extra...))
	httpc := newHTTPClient()
	endpoint := api + "/v1/demo/trigger"

	clock := &demoClock{api: api, c: httpc, mu: sync.Mutex{}, off: 0}

	refreshErr := clock.refresh(ctx)
	if refreshErr != nil {
		return fmt.Errorf("demo clock: %w", refreshErr)
	}

	go clock.keepRefreshing(ctx)

	// the messages are worked off by a fixed set of workers, a device always on the same one (its readings, heartbeats and
	// acknowledgements stay in order), so one slow call does not hold up the rest and the broker's queue for the bridge
	// does not overflow
	work := iotbridge.NewDispatcher(workers, workerQueue)
	defer work.Close() // runs after the disconnect below: no message is handed to it any more

	// the subscriptions are made on every connect: a clean session loses them when the broker restarts
	opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID("iot-bridge").
		SetUsername(os.Getenv("MQTT_USER")).SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).SetConnectRetry(true).SetKeepAlive(keepAlive).
		SetOnConnectHandler(func(c mqtt.Client) { subscribeDemo(ctx, c, work, resolver, clock, httpc, endpoint) }).
		SetConnectionLostHandler(
			func(_ mqtt.Client, err error) { log.Printf("broker %s: connection lost: %v", broker, err) })

	client := mqtt.NewClient(opts)
	if tok := client.Connect(); tok.Wait() && tok.Error() != nil {
		return fmt.Errorf("connect %s: %w", broker, tok.Error())
	}
	defer client.Disconnect(disconnectWait)

	live, stopLive := startLive(
		ctx,
		clock,
		httpc,
		endpoint,
		liveAPI{equipment: equipment, token: internalToken(), tenant: tenant},
	)
	defer stopLive()
	// commands: poll the API for the requested commands of the demo tenant, send each to its device on its command topic,
	// then report it sent (command_sent). The API moves it requested → sent; the module's acknowledgement comes back on
	// cmd/ack (above) and moves it on.
	go pollCommands(ctx, client, httpc, equipment, endpoint, internalToken(), clock, tenant, live)

	log.Printf("iot-bridge: %s → %s", broker, endpoint)
	<-ctx.Done()
	log.Print("iot-bridge: stopping")

	return nil
}

// newHTTPClient is the bridge's client of the API. It keeps a connection for each worker: the default keeps two for a
// host, and the workers would open and close one for nearly every call until the local ports ran out.
func newHTTPClient() *http.Client {
	client := &http.Client{Timeout: httpTimeout, Transport: nil, CheckRedirect: nil, Jar: nil}

	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		transport := base.Clone()
		transport.MaxIdleConnsPerHost = workers
		client.Transport = transport
	}

	return client
}

// subscribeDemo subscribes to the simulated modules' topics (QoS 1, as the design says). It runs on every connect; a
// subscription that fails is logged, and the next reconnect tries again.
func subscribeDemo(
	ctx context.Context,
	client mqtt.Client,
	work *iotbridge.Dispatcher,
	resolver *iotbridge.Resolver,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
) {
	sub := func(topic string, h mqtt.MessageHandler) {
		// The handler only queues the message on its device's worker and returns, so the library takes the next message
		// (and acknowledges this one to the broker) at once. A message acknowledged but not yet worked off is lost if the
		// bridge stops then: the price of the parallel work, accepted because this bridge only stands in for AWS IoT rules.
		queued := func(conn mqtt.Client, msg mqtt.Message) {
			work.Submit(deviceKey(msg.Topic()), func() { h(conn, msg) })
		}

		if tok := client.Subscribe(topic, 1, queued); tok.Wait() && tok.Error() != nil {
			log.Printf("subscribe %s: %v", topic, tok.Error())
		}
	}
	// t/<tenantId>/d/<deviceId>/telemetry, every tenant and device
	sub("t/+/d/+/telemetry", func(_ mqtt.Client, msg mqtt.Message) {
		var batch devicesim.Batch

		err := json.Unmarshal(msg.Payload(), &batch)
		if err != nil {
			log.Printf("telemetry on %s: %v", msg.Topic(), err)

			return
		}
		// stamp a little in the past: the offset is an estimate, and the API refuses a reading received after its clock
		now, off := time.Now(), clock.offset()
		batch = iotbridge.ToScenario(batch, now, off, clockMargin)

		triggers, err := resolver.Triggers(batch, now.Add(off-clockMargin))
		if err != nil {
			log.Printf("telemetry from %s: %v", batch.DeviceID, err)
		}

		for _, tr := range triggers {
			err := send(ctx, httpc, endpoint, tr)
			if err != nil {
				log.Printf("trigger %s: %v", tr.EventID, err)
			}
		}
	})
	// t/<tenantId>/d/<deviceId>/heartbeat: a module that reports its air conditioner's state carries it; one that does
	// not is heard all the same (IR613)
	sub("t/+/d/+/heartbeat", func(_ mqtt.Client, msg mqtt.Message) {
		var heartbeat devicesim.Heartbeat

		err := json.Unmarshal(msg.Payload(), &heartbeat)
		if err != nil {
			log.Printf("heartbeat on %s: %v", msg.Topic(), err)

			return
		}

		trigger, err := iotbridge.StateTrigger(
			heartbeat,
			time.Now().Add(clock.offset()-clockMargin),
		)
		if err != nil {
			log.Printf("heartbeat from %s: %v", heartbeat.DeviceID, err)

			return
		}

		sendErr := send(ctx, httpc, endpoint, trigger)
		if sendErr != nil {
			log.Printf("state %s: %v", heartbeat.DeviceID, sendErr)
		}
	})
	// t/<tenantId>/d/<deviceId>/cmd/ack: a module's answer to a command the API sent
	sub("t/+/d/+/cmd/ack", func(_ mqtt.Client, msg mqtt.Message) {
		var ack devicesim.Ack

		err := json.Unmarshal(msg.Payload(), &ack)
		if err != nil {
			log.Printf("ack on %s: %v", msg.Topic(), err)

			return
		}

		trigger, forward, err := iotbridge.CommandTrigger(ack, time.Now().Add(clock.offset()))
		if err != nil {
			log.Printf("ack from %s: %v", ack.DeviceID, err)

			return
		}

		if !forward {
			log.Printf(
				"ack %s: %s, not forwarded (the backend decides a late acknowledgement)",
				ack.CommandID,
				ack.Result,
			)

			return
		}

		sendErr := send(ctx, httpc, endpoint, trigger)
		if sendErr != nil {
			log.Printf("trigger %s: %v", trigger.EventID, sendErr)
		}
	})
}

// deviceKey is the device of a topic t/<tenant>/d/<device>/…, which decides the worker of its messages; a topic of any
// other shape is its own key.
func deviceKey(topic string) string {
	const deviceAt = 3 // t, <tenant>, d, <device>

	parts := strings.Split(topic, "/")

	if len(parts) > deviceAt {
		return parts[deviceAt]
	}

	return topic
}

// liveOptions are the live broker's client options: the nodes' login, a clean session with no will message, and the
// subscription to topic, made on every connect, whose handler sends to the API and so must not hold up the keep-alive.
// host is the broker without the credentials its URL may carry, for the logs.
func liveOptions(raw, host, topic string, handle mqtt.MessageHandler) *mqtt.ClientOptions {
	return mqtt.NewClientOptions().
		AddBroker(raw).
		SetClientID("ac-iot-bridge-" + uuid.NewString()[:8]).
		SetUsername(os.Getenv("REAL_MQTT_USER")).
		SetPassword(os.Getenv("REAL_MQTT_PASSWORD")).
		SetCleanSession(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetKeepAlive(keepAlive).
		SetOrderMatters(false).
		SetOnConnectHandler(func(c mqtt.Client) {
			if tok := c.Subscribe(topic, 0, handle); tok.Wait() && tok.Error() != nil {
				log.Printf(
					"live broker %s: subscribe %s: %v",
					strconv.Quote(host),
					strconv.Quote(topic),
					tok.Error(),
				)

				return
			}

			log.Printf("live broker %s: listening to %s", strconv.Quote(host), strconv.Quote(topic))
		}).
		SetConnectionLostHandler(
			func(_ mqtt.Client, err error) {
				log.Printf("live broker %s: connection lost: %v", strconv.Quote(host), err)
			})
}

// startLive connects to the real sensor nodes' broker when REAL_MQTT_URL is set (DEC-86, IR524), and returns the
// control of the live units' commands with the disconnect to defer; without it there is no live control. The registered
// nodes (devices.mqttNodes) are read at once and every 30 seconds. The client leaves no will message, and its only
// publishes are liveControl's commands. The connection is not waited for: the broker may be out of reach (another
// network), and the client keeps trying.
func startLive(
	ctx context.Context,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
) (*liveControl, func()) {
	raw := os.Getenv("REAL_MQTT_URL")
	if raw == "" {
		return nil, func() {}
	}

	topic := envOr("REAL_MQTT_TOPIC", "smarthome/ac/#")
	host := brokerHost(raw)
	nodes := iotbridge.NewNodeRegistry()
	handler := newLiveHandler(clock, httpc, endpoint, nodes)

	go keepNodes(ctx, httpc, api, nodes)
	go reportGap(ctx, handler.heard, clock, httpc, endpoint, api, nodes)
	go answerChecks(ctx, handler.heard, clock, httpc, endpoint, api)

	client := mqtt.NewClient(
		liveOptions(
			raw,
			host,
			topic,
			func(_ mqtt.Client, msg mqtt.Message) { handler.handle(ctx, msg) },
		),
	)
	client.Connect() // not waited for: the client keeps trying while the broker is out of reach
	log.Printf(
		"iot-bridge: live sensors from %s (%s) → %s; the nodes are the devices' MQTT node IDs",
		strconv.Quote(host),
		strconv.Quote(topic),
		endpoint,
	)

	control := newLiveControl(
		client,
		nodes,
		handler.firmware,
		handler.outstanding,
		os.Getenv("REAL_MQTT_COMMANDS") == "1",
	)

	return control, func() { client.Disconnect(disconnectWait) }
}

// keepNodes reads the registered nodes now and every nodeRefresh until the context ends. A read that fails is logged
// (once a minute while it keeps failing) and the last one stands.
func keepNodes(
	ctx context.Context,
	httpc *http.Client,
	api liveAPI,
	nodes *iotbridge.NodeRegistry,
) {
	rep := &logging.Repeats{Logf: log.Printf, Every: 0, Now: nil}
	ticker := time.NewTicker(nodeRefresh)

	defer ticker.Stop()

	for {
		found, err := fetchNodes(ctx, httpc, api)
		if err != nil {
			rep.Failed(fmt.Sprintf("registered nodes: %v", err))
		} else {
			nodes.Set(found)
			rep.OK("registered nodes")
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// liveQuery calls an internal query of the equipment service (devices.<name>) with the internal service token and
// the tenant, and decodes its answer's data into out.
func liveQuery[T any](
	ctx context.Context,
	client *http.Client,
	api liveAPI,
	name, input string,
	out *T,
) error {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		api.equipment+"/internal/v1/system/queries/devices."+name,
		strings.NewReader(input),
	)
	if err != nil {
		return apperr.Wrap(err, "http.NewRequest")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ops.InternalTokenHeader, api.token)
	req.Header.Set("X-Tenant-Id", api.tenant)

	resp, err := client.Do(req)
	if err != nil {
		return apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode}
	}

	var body struct {
		Data *T `json:"data"`
	}

	body.Data = out

	return apperr.Wrap(json.NewDecoder(resp.Body).Decode(&body), "json.Decoder.Decode")
}

// fetchNodes reads the bound devices that have a node_id (devices.mqttNodes, IR635).
func fetchNodes(
	ctx context.Context,
	client *http.Client,
	api liveAPI,
) ([]iotbridge.NodeDevice, error) {
	var data struct {
		Nodes []iotbridge.NodeDevice `json:"nodes"`
	}

	err := liveQuery(ctx, client, api, "mqttNodes", `{}`, &data)

	return data.Nodes, err
}

// brokerHost is a broker URL's host for the logs, never the credentials the URL may carry.
func brokerHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}

	return raw
}

// send posts one trigger. The event ID is the idempotency key: the API answers a repeat with the original receipt.
func send(
	ctx context.Context,
	client *http.Client,
	endpoint string,
	trigger iotbridge.Trigger,
) error {
	body, err := json.Marshal(trigger)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return apperr.Wrap(err, "http.NewRequest")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", trigger.EventID.String())

	resp, err := client.Do(req) //nolint:gosec // G704: URL is --api, the file fills the body only
	if err != nil {
		return apperr.Wrap(err, "http.Client.Do")
	}

	defer func() {
		// a connection goes back to the client only when its answer was read to the end; a failure here only costs it
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return &statusError{code: resp.StatusCode}
	}

	return nil
}

type statusError struct{ code int }

// recorded tells a CONFLICT answer apart: the event ID was taken by an earlier send of the same event (its answer
// lost), so the event is recorded and need not be sent again.
func recorded(err error) bool {
	var se *statusError

	return errors.As(err, &se) && se.code == http.StatusConflict
}

func (e *statusError) Error() string { return "API answered " + http.StatusText(e.code) }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// demoClock keeps the bridge on the API's scenario clock. The demo runs its own clock (a fixed seed date that moves
// with real time), and the API refuses a reading received after it, so the bridge stamps readings with the scenario
// time. The offset comes from the snapshot time of the demo-only preview (it saves and sends nothing) and is refreshed
// every five minutes.
type demoClock struct {
	api string
	c   *http.Client
	mu  sync.Mutex
	off time.Duration
}

func (d *demoClock) offset() time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.off
}

func (d *demoClock) refresh(ctx context.Context) error {
	before := time.Now()

	scenario, err := iotbridge.ScenarioNow(ctx, d.c, d.api)
	if err != nil {
		return apperr.Wrap(err, "iotbridge.ScenarioNow")
	}

	//nolint:mnd // the server read its clock about halfway through the round trip
	mid := before.Add(time.Since(before) / 2)

	d.mu.Lock()
	d.off = scenario.Sub(mid)
	d.mu.Unlock()

	return nil
}

func (d *demoClock) keepRefreshing(ctx context.Context) {
	ticker := time.NewTicker(clockRefresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := d.refresh(ctx)
			if err != nil {
				log.Printf("demo clock: %v", err)
			}
		}
	}
}

// clockMargin absorbs the error of the scenario-clock estimate: readings are stamped this much earlier, both their
// observed and their received time, so the order of the two is kept.
const clockMargin = 2 * time.Second

// internalToken is the service token of the internal queries (INTERNAL_API_TOKEN, the same as the Core API's).
func internalToken() string { return os.Getenv("INTERNAL_API_TOKEN") }

// pollCommands sends the commands the API lists as requested, every two seconds, until the context ends: the live
// unit's to its command node (live, when the nodes' broker is configured), the others to the simulated modules.
func pollCommands(
	ctx context.Context,
	client mqtt.Client,
	httpc *http.Client,
	api, endpoint, token string,
	clock *demoClock,
	tenant string,
	live *liveControl,
) {
	ticker := time.NewTicker(commandPoll)
	defer ticker.Stop()

	rep := &logging.Repeats{
		Logf: log.Printf, Every: 0, Now: nil,
	} // while the API is away, once a minute instead of every poll (IR429)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			waiting, err := fetchWaiting(ctx, httpc, api, token, tenant)
			if err != nil {
				rep.Failed(fmt.Sprintf("requested commands: %v", err))

				continue
			}

			rep.OK("requested commands")

			for _, command := range waiting {
				var err error
				if dev, ok := liveNode(live, command.DeviceID); ok {
					err = live.send(ctx, httpc, endpoint, dev, command, clock.offset())
				} else {
					err = sendCommand(ctx, client, httpc, endpoint, tenant, command, clock.offset())
				}

				if err != nil {
					log.Printf("command %s: %v", command.ID, err)
				}
			}
		}
	}
}

// liveNode is the registered node of a command's device when the nodes' broker is configured.
func liveNode(live *liveControl, device uuid.UUID) (iotbridge.NodeDevice, bool) {
	var none iotbridge.NodeDevice
	if live == nil {
		return none, false
	}

	return live.node(device)
}

// fetchWaiting asks the API for the requested commands with the internal service token and the tenant.
func fetchWaiting(
	ctx context.Context,
	client *http.Client,
	api, token, tenant string,
) ([]iotbridge.Waiting, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		api+"/internal/v1/system/queries/control.requestedCommands",
		strings.NewReader(`{"limit":50}`),
	)
	if err != nil {
		return nil, apperr.Wrap(err, "http.NewRequest")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ops.InternalTokenHeader, token)
	req.Header.Set("X-Tenant-Id", tenant)

	resp, err := client.Do(req)
	if err != nil {
		return nil, apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{code: resp.StatusCode}
	}

	var out struct {
		Data []iotbridge.Waiting `json:"data"`
	}

	decodeErr := json.NewDecoder(resp.Body).Decode(&out)
	if decodeErr != nil {
		return nil, apperr.Wrap(decodeErr, "json.Decoder.Decode")
	}

	return out.Data, nil
}

// sendCommand publishes one requested command to its device, then reports it sent. The report comes only after the
// publish succeeded, so a command is never reported sent without leaving the bridge.
func sendCommand(
	ctx context.Context,
	client mqtt.Client,
	httpc *http.Client,
	endpoint, tenant string,
	waiting iotbridge.Waiting,
	off time.Duration,
) error {
	cmd, err := iotbridge.CommandMessage(waiting, off) // the expiry on the module's clock
	if err != nil {
		return apperr.Wrap(err, "iotbridge.CommandMessage")
	}

	raw, err := json.Marshal(cmd)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	tok := client.Publish(
		iotbridge.CommandTopic(uuid.MustParse(tenant), waiting.DeviceID),
		1,
		false,
		raw,
	)
	tok.Wait()

	if tok.Error() != nil {
		return apperr.Wrap(tok.Error(), "mqtt.Token.Error")
	}

	return send(ctx, httpc, endpoint, iotbridge.SentTrigger(waiting, time.Now().Add(off)))
}

// liveAPI is where the bridge reads the live module's connection axis: the equipment service's internal queries, with
// the internal service token and the tenant.
type liveAPI struct{ equipment, token, tenant string }

// reportGap runs once a minute for each registered node's device. Its connection follows the heartbeat gap of its node
// (IR532): offline once the node has been silent for 3 minutes, online again once it is heard (device events of its
// binding, the open fault read back from the API, so a restart of the bridge picks it up). Then it sends how long the
// node has gone unheard (heartbeat_gap) for its unit, so the default rule ac_offline alerts when the node stops or its
// broker is out of reach and resolves when it is back (IR529).
func reportGap(
	ctx context.Context,
	heard *iotbridge.LiveHeard,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
	nodes *iotbridge.NodeRegistry,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	reps := map[string]*logging.Repeats{} // one per node, so that one's failures do not hide another's

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			when := now.Add(clock.offset() - clockMargin).UTC()

			for _, dev := range nodes.Devices() {
				if reps[dev.Node] == nil {
					reps[dev.Node] = &logging.Repeats{Logf: log.Printf, Every: 0, Now: nil}
				}

				gap := heard.GapOf([]string{dev.Node}, now)
				reportNodeGap(ctx, httpc, endpoint, api, reps[dev.Node], dev, gap, when)
			}
		}
	}
}

// reportNodeGap is a registered node's report of one minute: its device's connection brought in line with the gap of
// the node, then that gap for its unit.
func reportNodeGap(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
	rep *logging.Repeats,
	dev iotbridge.NodeDevice,
	gap time.Duration,
	when time.Time,
) {
	f, err := fetchConnectionFault(ctx, httpc, api, dev.DeviceID)
	if err != nil {
		rep.Failed(fmt.Sprintf("live connection of node %s: %v", dev.Node, err))
	} else if trigger, ok := iotbridge.LiveConnectionTrigger(dev.DeviceID, f, gap, when); ok {
		err := send(ctx, httpc, endpoint, trigger)
		if err != nil && !recorded(err) {
			rep.Failed(
				fmt.Sprintf("live connection %s of node %s: %v", trigger.Kind, dev.Node, err),
			)
		} else {
			log.Printf(
				"live node %s: %s (unheard for %s)",
				dev.Node,
				trigger.Kind,
				gap.Round(time.Second),
			)
		}
	}

	sendErr := send(
		ctx,
		httpc,
		endpoint,
		iotbridge.LiveGapTrigger(dev.UnitID, gap, when.Truncate(time.Minute)),
	)
	if sendErr != nil && !recorded(sendErr) {
		rep.Failed(fmt.Sprintf("live heartbeat gap of node %s: %v", dev.Node, sendErr))

		return
	}

	rep.OK("live heartbeat gap of node " + dev.Node)
}

// fetchConnectionFault reads the live module's connection axis (devices.connectionFault, IR532).
func fetchConnectionFault(
	ctx context.Context,
	client *http.Client,
	api liveAPI,
	device uuid.UUID,
) (iotbridge.ConnectionFault, error) {
	var out iotbridge.ConnectionFault

	err := liveQuery(
		ctx,
		client,
		api,
		"connectionFault",
		`{"deviceId":"`+device.String()+`"}`,
		&out,
	)

	return out, err
}
