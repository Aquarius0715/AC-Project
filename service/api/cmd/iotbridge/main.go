// Command iotbridge carries the simulated modules' telemetry from the MQTT broker to the Core API demo trigger
// (POST /v1/demo/trigger, the demo environment only). It subscribes to t/+/d/+/telemetry, converts each reading with
// internal/iotbridge and sends it with the event ID as Idempotency-Key, so a redelivered reading is one event.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func main() {
	broker := flag.String("broker", envOr("MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
	api := flag.String("api", envOr("CORE_API_URL", "http://localhost:8080"), "Core API base URL (the gateway)")
	equipment := flag.String("equipment", envOr("EQUIPMENT_API_URL", "http://equipment-api:8080"), "equipment API base URL: the internal queries of the control domain are served there")
	tenant := flag.String("tenant", seed.ID("tenant-a").String(), "tenant whose commands the bridge sends (the demo tenant by default)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fleet, err := devicesim.MockFleet()
	if err != nil {
		log.Fatal(err)
	}
	resolver := iotbridge.NewResolver(append(append([]iotbridge.Link{}, iotbridge.Links...), iotbridge.FleetLinks(fleet)...))
	httpc := &http.Client{Timeout: 10 * time.Second}
	endpoint := *api + "/v1/demo/trigger"
	clock := &demoClock{api: *api, c: httpc}
	if err := clock.refresh(); err != nil {
		log.Fatalf("demo clock: %v", err)
	}
	go clock.keepRefreshing(ctx)

	opts := mqtt.NewClientOptions().AddBroker(*broker).SetClientID("iot-bridge").
		SetUsername(os.Getenv("MQTT_USER")).SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).SetConnectRetry(true).SetKeepAlive(60 * time.Second)
	client := mqtt.NewClient(opts)
	if tok := client.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("connect %s: %v", *broker, tok.Error())
	}
	defer client.Disconnect(500)

	// t/<tenantId>/d/<deviceId>/telemetry, every tenant and device (QoS 1, as the design says)
	tok := client.Subscribe("t/+/d/+/telemetry", 1, func(_ mqtt.Client, msg mqtt.Message) {
		var b devicesim.Batch
		if err := json.Unmarshal(msg.Payload(), &b); err != nil {
			log.Printf("telemetry on %s: %v", msg.Topic(), err)
			return
		}
		// stamp a little in the past: the offset is an estimate, and the API refuses a reading received after its clock
		now, off := time.Now(), clock.offset()
		b = iotbridge.ToScenario(b, now, off, clockMargin)
		triggers, err := resolver.Triggers(b, now.Add(off-clockMargin))
		if err != nil {
			log.Printf("telemetry from %s: %v", b.DeviceID, err)
		}
		for _, tr := range triggers {
			if err := send(httpc, endpoint, tr); err != nil {
				log.Printf("trigger %s: %v", tr.EventID, err)
			}
		}
	})
	if tok.Wait() && tok.Error() != nil {
		log.Fatalf("subscribe: %v", tok.Error())
	}
	// t/<tenantId>/d/<deviceId>/heartbeat: a module that reports its air conditioner's state carries it (QoS 1)
	hbTok := client.Subscribe("t/+/d/+/heartbeat", 1, func(_ mqtt.Client, msg mqtt.Message) {
		var hb devicesim.Heartbeat
		if err := json.Unmarshal(msg.Payload(), &hb); err != nil {
			log.Printf("heartbeat on %s: %v", msg.Topic(), err)
			return
		}
		tr, ok, err := iotbridge.StateTrigger(hb, time.Now().Add(clock.offset()-clockMargin))
		if err != nil {
			log.Printf("heartbeat from %s: %v", hb.DeviceID, err)
			return
		}
		if !ok {
			return
		}
		if err := send(httpc, endpoint, tr); err != nil {
			log.Printf("state %s: %v", hb.DeviceID, err)
		}
	})
	if hbTok.Wait() && hbTok.Error() != nil {
		log.Fatalf("subscribe heartbeats: %v", hbTok.Error())
	}
	// t/<tenantId>/d/<deviceId>/cmd/ack: a module's answer to a command the API sent (QoS 1)
	ackTok := client.Subscribe("t/+/d/+/cmd/ack", 1, func(_ mqtt.Client, msg mqtt.Message) {
		var a devicesim.Ack
		if err := json.Unmarshal(msg.Payload(), &a); err != nil {
			log.Printf("ack on %s: %v", msg.Topic(), err)
			return
		}
		tr, ok, err := iotbridge.CommandTrigger(a, time.Now().Add(clock.offset()))
		if err != nil {
			log.Printf("ack from %s: %v", a.DeviceID, err)
			return
		}
		if !ok {
			log.Printf("ack %s: %s, not forwarded (the backend decides a late acknowledgement)", a.CommandID, a.Result)
			return
		}
		if err := send(httpc, endpoint, tr); err != nil {
			log.Printf("trigger %s: %v", tr.EventID, err)
		}
	})
	if ackTok.Wait() && ackTok.Error() != nil {
		log.Fatalf("subscribe acks: %v", ackTok.Error())
	}
	// commands: poll the API for the requested commands of the demo tenant, send each to its device on its command topic,
	// then report it sent (command_sent). The API moves it requested → sent; the module's acknowledgement comes back on
	// cmd/ack (above) and moves it on.
	go pollCommands(ctx, client, httpc, *equipment, endpoint, internalToken(), clock, *tenant)
	log.Printf("iot-bridge: %s → %s", *broker, endpoint)
	<-ctx.Done()
	log.Print("iot-bridge: stopping")
}

// send posts one trigger. The event ID is the idempotency key: the API answers a repeat with the original receipt.
func send(c *http.Client, endpoint string, tr iotbridge.Trigger) error {
	body, err := json.Marshal(tr)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", tr.EventID.String())
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &statusError{code: resp.StatusCode}
	}
	return nil
}

type statusError struct{ code int }

func (e *statusError) Error() string { return "API answered " + http.StatusText(e.code) }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// demoClock keeps the bridge on the API's scenario clock. The demo runs its own clock (a fixed seed date that moves with
// real time), and the API refuses a reading received after it, so the bridge stamps readings with the scenario time. The
// offset comes from the snapshot time of the demo-only preview (it saves and sends nothing) and is refreshed every five minutes.
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

func (d *demoClock) refresh() error {
	before := time.Now()
	scenario, err := iotbridge.ScenarioNow(d.c, d.api)
	if err != nil {
		return err
	}
	mid := before.Add(time.Since(before) / 2)
	d.mu.Lock()
	d.off = scenario.Sub(mid)
	d.mu.Unlock()
	return nil
}

func (d *demoClock) keepRefreshing(ctx context.Context) {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := d.refresh(); err != nil {
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

// pollCommands sends the commands the API lists as requested, every two seconds, until the context ends.
func pollCommands(ctx context.Context, mc mqtt.Client, httpc *http.Client, api, endpoint, token string, clock *demoClock, tenant string) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	rep := &logging.Repeats{Logf: log.Printf} // while the API is away, once a minute instead of every poll (IR429)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			waiting, err := fetchWaiting(httpc, api, token, tenant)
			if err != nil {
				rep.Failed(fmt.Sprintf("requested commands: %v", err))
				continue
			}
			rep.OK("requested commands")
			for _, w := range waiting {
				if err := sendCommand(mc, httpc, endpoint, tenant, w, clock.offset()); err != nil {
					log.Printf("command %s: %v", w.ID, err)
				}
			}
		}
	}
}

// fetchWaiting asks the API for the requested commands with the internal service token and the tenant.
func fetchWaiting(c *http.Client, api, token, tenant string) ([]iotbridge.Waiting, error) {
	req, err := http.NewRequest(http.MethodPost, api+"/internal/v1/system/queries/control.requestedCommands", strings.NewReader(`{"limit":50}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ops.InternalTokenHeader, token)
	req.Header.Set("X-Tenant-Id", tenant)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API answered %s", http.StatusText(resp.StatusCode))
	}
	var out struct {
		Data []iotbridge.Waiting `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// sendCommand publishes one requested command to its device, then reports it sent. The report comes only after the
// publish succeeded, so a command is never reported sent without leaving the bridge.
func sendCommand(mc mqtt.Client, httpc *http.Client, endpoint, tenant string, w iotbridge.Waiting, off time.Duration) error {
	cmd, err := iotbridge.CommandMessage(w, off) // the expiry on the module's clock
	if err != nil {
		return err
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	tok := mc.Publish(iotbridge.CommandTopic(uuid.MustParse(tenant), w.DeviceID), 1, false, raw)
	tok.Wait()
	if tok.Error() != nil {
		return tok.Error()
	}
	return send(httpc, endpoint, iotbridge.SentTrigger(w, time.Now().Add(off)))
}
