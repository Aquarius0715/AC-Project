// Command iotbridge carries the simulated modules' telemetry from the MQTT broker to the Core API demo trigger
// (POST /v1/demo/trigger, the demo environment only). It subscribes to t/+/d/+/telemetry, converts each reading with
// internal/iotbridge and sends it with the event ID as Idempotency-Key, so a redelivered reading is one event.
//
// With REAL_MQTT_URL set it also listens to real sensor nodes on another broker (DEC-86, IR524): it subscribes to
// REAL_MQTT_TOPIC (smarthome/ac/# by default) with REAL_MQTT_USER / REAL_MQTT_PASSWORD and records one reading per
// node and minute on the live site (seed.LiveSite). The only thing it publishes there is the live unit's power command
// to the command node (REAL_MQTT_COMMAND_NODE, node_1 by default), and only with REAL_MQTT_COMMANDS=1 (DEC-89, IR528):
// the nodes switch a real air conditioner.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
)

func main() {
	broker := flag.String("broker", envOr("MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
	api := flag.String(
		"api",
		envOr("CORE_API_URL", "http://localhost:8080"),
		"Core API base URL (the gateway)",
	)
	equipment := flag.String(
		"equipment",
		envOr("EQUIPMENT_API_URL", "http://equipment-api:8080"),
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

	resolver := iotbridge.NewResolver(
		append(append([]iotbridge.Link{}, iotbridge.Links()...), iotbridge.FleetLinks(fleet)...),
	)
	httpc := &http.Client{Timeout: httpTimeout, Transport: nil, CheckRedirect: nil, Jar: nil}
	endpoint := api + "/v1/demo/trigger"

	clock := &demoClock{api: api, c: httpc, mu: sync.Mutex{}, off: 0}

	refreshErr := clock.refresh(ctx)
	if refreshErr != nil {
		return fmt.Errorf("demo clock: %w", refreshErr)
	}

	go clock.keepRefreshing(ctx)

	// the subscriptions are made on every connect: a clean session loses them when the broker restarts
	opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID("iot-bridge").
		SetUsername(os.Getenv("MQTT_USER")).SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).SetConnectRetry(true).SetKeepAlive(keepAlive).
		SetOnConnectHandler(func(c mqtt.Client) { subscribeDemo(ctx, c, resolver, clock, httpc, endpoint) }).
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

// subscribeDemo subscribes to the simulated modules' topics (QoS 1, as the design says). It runs on every connect; a
// subscription that fails is logged, and the next reconnect tries again.
func subscribeDemo(
	ctx context.Context,
	client mqtt.Client,
	resolver *iotbridge.Resolver,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
) {
	sub := func(topic string, h mqtt.MessageHandler) {
		if tok := client.Subscribe(topic, 1, h); tok.Wait() && tok.Error() != nil {
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
// control of the live unit's commands with the disconnect to defer; without it there is no live control. The client
// leaves no will message, and its only publishes are liveControl's commands. The connection is not waited for: the
// broker may be out of reach (another network), and the client keeps trying.
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

	client := mqtt.NewClient(
		liveOptions(raw, host, topic, liveReadings(ctx, clock, httpc, endpoint, api)),
	)
	client.Connect() // not waited for: the client keeps trying while the broker is out of reach
	log.Printf(      // the units' customer is the database's (migrate's SEED_LIVE_CLIENT)
		"iot-bridge: live sensors from %s (%s) → %s",
		strconv.Quote(host),
		strconv.Quote(topic),
		strings.Join(seed.LiveSite().UnitNames(), ", "),
	)

	control := newLiveControl(
		client,
		envOr("REAL_MQTT_COMMAND_NODE", iotbridge.LiveCommandNode),
		os.Getenv("REAL_MQTT_COMMANDS") == "1",
	)

	return control, func() { client.Disconnect(disconnectWait) }
}

// brokerHost is a broker URL's host for the logs, never the credentials the URL may carry.
func brokerHost(raw string) string {
	u, err := url.Parse(raw)
	if err == nil && u.Host != "" {
		return u.Host
	}

	return raw
}

// liveReadings is the live broker's message handler: it records the real sensor nodes' readings on the live site, one
// message per node and scenario minute, and skips the retained messages, which are old readings. It also starts the
// report of the nodes that go quiet.
func liveReadings(
	ctx context.Context,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
) mqtt.MessageHandler {
	gate := &iotbridge.LiveGate{}
	heard := iotbridge.NewLiveHeard(time.Now())
	rep := &logging.Repeats{
		Logf:  log.Printf,
		Every: 0,
		Now:   nil,
	} // while the API is away, once a minute

	go reportGap(ctx, heard, clock, httpc, endpoint, api)

	var warned sync.Map // nodes and errors already logged

	once := func(key, msg string) {
		if _, seen := warned.LoadOrStore(key, true); !seen {
			log.Print(msg)
		}
	}

	return func(_ mqtt.Client, msg mqtt.Message) {
		node := iotbridge.LiveNode(msg.Topic())
		if node == "" || msg.Retained() {
			return
		}

		heard.Heard(node, time.Now())
		when := time.Now().Add(clock.offset() - clockMargin).UTC()

		slot := when.Truncate(time.Minute)
		if !gate.Claim(node, slot) {
			return
		}

		triggers, err := iotbridge.LiveTriggers(node, msg.Payload(), when)
		if err != nil || len(triggers) == 0 {
			gate.Release(node, slot) // the node's next message of the minute may have a reading

			if err != nil {
				once("convert/"+node, fmt.Sprintf("live %s: %v (logged once)", msg.Topic(), err))
			}

			return
		}

		for _, tr := range triggers {
			err := send(ctx, httpc, endpoint, tr)
			if err != nil && !recorded(err) {
				gate.Release(node, slot)
				rep.Failed(fmt.Sprintf("live %s: %v", node, err))

				return
			}
		}

		rep.OK("live readings")
	}
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

	resp, err := client.Do(req)
	if err != nil {
		return apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = resp.Body.Close() }()

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
				if live != nil && command.DeviceID == live.device {
					err = live.send(ctx, httpc, endpoint, command, clock.offset())
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

// reportGap runs once a minute for each live module. Its connection follows the heartbeat gap of its nodes (IR532):
// offline once one of them has been silent for 3 minutes, online again once every one is heard (device events of its
// binding, the open fault read back from the API, so a restart of the bridge picks it up). Then it sends how long the
// module has gone unheard (heartbeat_gap) for its unit, so the default rule ac_offline alerts when the nodes stop or
// their broker is out of reach and resolves when they are back (IR529).
func reportGap(
	ctx context.Context,
	heard *iotbridge.LiveHeard,
	clock *demoClock,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	modules := seed.LiveSite().Modules()

	reps := make(
		[]*logging.Repeats,
		len(modules),
	) // one per module, so that one's failures do not hide another's
	for i := range reps {
		reps[i] = &logging.Repeats{Logf: log.Printf, Every: 0, Now: nil}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			when := now.Add(clock.offset() - clockMargin).UTC()
			for i, module := range modules {
				gap := heard.GapOf(iotbridge.ModuleNodes(module), now)
				reportModuleGap(ctx, httpc, endpoint, api, reps[i], module, gap, when)
			}
		}
	}
}

// reportModuleGap is a live module's report of one minute: its connection brought in line with the gap of its nodes,
// then that gap for its unit.
func reportModuleGap(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	api liveAPI,
	rep *logging.Repeats,
	module seed.LiveModule,
	gap time.Duration,
	when time.Time,
) {
	dev := seed.ID(module.Device)

	f, err := fetchConnectionFault(ctx, httpc, api, dev)
	if err != nil {
		rep.Failed(fmt.Sprintf("live connection of %s: %v", module.UnitName, err))
	} else if trigger, ok := iotbridge.LiveConnectionTrigger(dev, f, gap, when); ok {
		err := send(ctx, httpc, endpoint, trigger)
		if err != nil && !recorded(err) {
			rep.Failed(
				fmt.Sprintf("live connection %s of %s: %v", trigger.Kind, module.UnitName, err),
			)
		} else {
			log.Printf(
				"live module %s: %s (nodes unheard for %s)",
				module.UnitName,
				trigger.Kind,
				gap.Round(time.Second),
			)
		}
	}

	sendErr := send(
		ctx,
		httpc,
		endpoint,
		iotbridge.LiveGapTrigger(seed.ID(module.Unit), gap, when.Truncate(time.Minute)),
	)
	if sendErr != nil && !recorded(sendErr) {
		rep.Failed(fmt.Sprintf("live heartbeat gap of %s: %v", module.UnitName, sendErr))

		return
	}

	rep.OK("live heartbeat gap of " + module.UnitName)
}

// fetchConnectionFault reads the live module's connection axis (devices.connectionFault, IR532).
func fetchConnectionFault(
	ctx context.Context,
	client *http.Client,
	api liveAPI,
	device uuid.UUID,
) (iotbridge.ConnectionFault, error) {
	var out iotbridge.ConnectionFault

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		api.equipment+"/internal/v1/system/queries/devices.connectionFault",
		strings.NewReader(`{"deviceId":"`+device.String()+`"}`),
	)
	if err != nil {
		return out, apperr.Wrap(err, "http.NewRequest")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ops.InternalTokenHeader, api.token)
	req.Header.Set("X-Tenant-Id", api.tenant)

	resp, err := client.Do(req)
	if err != nil {
		return out, apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return out, &statusError{code: resp.StatusCode}
	}

	var body struct {
		Data iotbridge.ConnectionFault `json:"data"`
	}

	decodeErr := json.NewDecoder(resp.Body).Decode(&body)
	if decodeErr != nil {
		return out, apperr.Wrap(decodeErr, "json.Decoder.Decode")
	}

	return body.Data, nil
}

// liveControl carries the live unit's commands to the nodes' broker (DEC-89): the firmware command, on the command
// node's topic only. Sending is on only with REAL_MQTT_COMMANDS=1; without it a command of the live unit fails.
type liveControl struct {
	client  mqtt.Client
	node    string
	enabled bool
	device  uuid.UUID
}

func newLiveControl(client mqtt.Client, node string, enabled bool) *liveControl {
	if _, ok := iotbridge.LiveNodes()[node]; !ok {
		log.Printf("live commands: %q is not a live node; sending is off", node)

		enabled = false
	}

	if enabled {
		log.Printf(
			"live commands: power on/off of %s goes to %s",
			seed.LiveSite().UnitName,
			iotbridge.LiveCommandTopic(node),
		)
	} else {
		log.Printf(
			"live commands: off (REAL_MQTT_COMMANDS); commands of %s fail",
			seed.LiveSite().UnitName,
		)
	}

	return &liveControl{
		client:  client,
		node:    node,
		enabled: enabled,
		device:  seed.ID(seed.LiveSite().Device),
	}
}

// send carries one requested command of the live unit: the firmware command to the command node (QoS 1), then sent,
// acknowledged and the settings it runs with. An action the firmware does not take, or any while sending is off,
// fails. While the broker is out of reach the command stays requested, and the API ends it at its expiry.
func (l *liveControl) send(
	ctx context.Context,
	httpc *http.Client,
	endpoint string,
	waiting iotbridge.Waiting,
	off time.Duration,
) error {
	command, supported, err := iotbridge.LiveCommand(waiting.Action)
	if err != nil {
		return apperr.Wrap(err, "iotbridge.LiveCommand")
	}

	when := time.Now().Add(off)
	if !supported || !l.enabled {
		return send(ctx, httpc, endpoint, iotbridge.LiveRefusedTrigger(waiting, when))
	}

	if !l.client.IsConnectionOpen() {
		return errLiveBrokerDown
	}

	tok := l.client.Publish(iotbridge.LiveCommandTopic(l.node), 1, false, command.Payload)
	if !tok.WaitTimeout(liveCommandWait) {
		return errLiveCommandTimeout
	}

	errorErr := tok.Error()
	if errorErr != nil {
		return apperr.Wrap(errorErr, "mqtt.Token.Error")
	}

	log.Printf(
		"live command %s: %s → %s",
		waiting.ID,
		strconv.Quote(command.Payload),
		strconv.Quote(iotbridge.LiveCommandTopic(l.node)),
	)

	for _, tr := range iotbridge.LiveSentTriggers(waiting, when, when.Add(-clockMargin), command.State) {
		err := send(ctx, httpc, endpoint, tr)
		if err != nil {
			return err
		}
	}

	return nil
}
