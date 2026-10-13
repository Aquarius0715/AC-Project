// Command iotbridge carries the simulated modules' telemetry from the MQTT broker to the Core API demo trigger
// (POST /v1/demo/trigger, the demo environment only). It subscribes to t/+/d/+/telemetry, converts each reading with
// internal/iotbridge and sends it with the event ID as Idempotency-Key, so a redelivered reading is one event.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
)

func main() {
	broker := flag.String("broker", envOr("MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
	api := flag.String("api", envOr("CORE_API_URL", "http://localhost:8080"), "Core API base URL (the gateway)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	resolver := iotbridge.NewResolver(iotbridge.Links)
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
		off := clock.offset()
		for i := range b.Measurements {
			b.Measurements[i].ObservedAt = b.Measurements[i].ObservedAt.Add(off)
		}
		triggers, err := resolver.Triggers(b, time.Now().Add(off))
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
	body := []byte(`{"demoEmail":"clock.probe@iot-bridge.invalid"}`)
	req, err := http.NewRequest(http.MethodPost, d.api+"/v1/auth/password-reset-preview", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Meta struct {
			SnapshotAt time.Time `json:"snapshotAt"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	if out.Meta.SnapshotAt.IsZero() {
		return errors.New("no snapshot time in the preview's answer")
	}
	mid := before.Add(time.Since(before) / 2)
	d.mu.Lock()
	d.off = out.Meta.SnapshotAt.Sub(mid)
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
