// Command devicesim runs simulated IoT modules for the demo: each module publishes a telemetry batch every minute and a
// heartbeat every 60 seconds to the MQTT broker, as backend-architecture §9 describes. The devices must already be
// registered (their IDs come from --device-ids); the broker login comes from MQTT_USER and MQTT_PASSWORD, because the
// broker refuses anonymous clients (security loop, IR413).
package main

import (
	"context"
	"flag"
	"hash/fnv"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func main() {
	broker := flag.String("broker", envOr("MQTT_URL", "tcp://localhost:1883"), "MQTT broker URL")
	tenant := flag.String("tenant", seed.ID("tenant-a").String(), "tenant ID the devices belong to (the demo tenant by default)")
	names := flag.String("devices", strings.Join(demoDevices(), ","), "comma-separated fixture device IDs (demoSeed.devices); their API IDs are derived with seed.ID")
	firmware := flag.String("firmware", "v1", "firmware version the modules report")
	interval := flag.Duration("telemetry-interval", time.Minute, "time between telemetry batches (design: every minute)")
	beat := flag.Duration("heartbeat-interval", time.Minute, "time between heartbeats (design: every 60 seconds)")
	flag.Parse()
	if *names == "" {
		log.Fatal("--devices must name at least one device")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := mqtt.NewClientOptions().AddBroker(*broker).SetClientID("devicesim-" + clientTag(*tenant)).
		SetUsername(os.Getenv("MQTT_USER")).SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).SetConnectRetry(true).SetKeepAlive(60 * time.Second)
	client := mqtt.NewClient(opts)
	if tok := client.Connect(); tok.Wait() && tok.Error() != nil {
		log.Fatalf("connect %s: %v", *broker, tok.Error())
	}
	defer client.Disconnect(500)

	var modules []*devicesim.Module
	for _, name := range strings.Split(*names, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id := seed.ID(name).String()
		modules = append(modules, devicesim.NewModule(*tenant, id, *firmware, seedOf(id)))
	}
	log.Printf("devicesim: %d modules on %s", len(modules), *broker)
	// each module hears its own command topic and answers on the acknowledgement topic (QoS 1, as the design says)
	for _, m := range modules {
		m := m
		tok := client.Subscribe(m.CommandTopic(), 1, func(_ mqtt.Client, msg mqtt.Message) {
			raw, err := m.Acknowledge(msg.Payload(), time.Now())
			if err != nil {
				log.Printf("command %s: %v", m.DeviceID, err)
				return
			}
			publish(client, m.AckTopic(), raw)
		})
		if tok.Wait() && tok.Error() != nil {
			log.Fatalf("subscribe %s: %v", m.CommandTopic(), tok.Error())
		}
	}

	telemetry := time.NewTicker(*interval)
	heartbeat := time.NewTicker(*beat)
	defer telemetry.Stop()
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("devicesim: stopping")
			return
		case now := <-telemetry.C:
			for _, m := range modules {
				raw, err := m.Telemetry(now)
				if err != nil {
					log.Printf("telemetry %s: %v", m.DeviceID, err)
					continue
				}
				publish(client, m.TelemetryTopic(), raw)
			}
		case now := <-heartbeat.C:
			for _, m := range modules {
				raw, err := m.Beat(now)
				if err != nil {
					log.Printf("heartbeat %s: %v", m.DeviceID, err)
					continue
				}
				publish(client, m.HeartbeatTopic(), raw)
			}
		}
	}
}

// publish sends one message at QoS 1 (the design's delivery level) and logs a failure without stopping the others.
func publish(c mqtt.Client, topic string, payload []byte) {
	tok := c.Publish(topic, 1, false, payload)
	tok.Wait()
	if tok.Error() != nil {
		log.Printf("publish %s: %v", topic, tok.Error())
	}
}

// clientTag keeps the MQTT client ID short and unique per tenant.
func clientTag(tenant string) string {
	if len(tenant) > 8 {
		return tenant[:8]
	}
	return tenant
}

// demoDevices are the fixture devices that report measurements (the links of internal/iotbridge).
func demoDevices() []string {
	out := make([]string, 0, len(iotbridge.Links))
	for _, l := range iotbridge.Links {
		out = append(out, l.Device)
	}
	return out
}

// seedOf gives each device its own deterministic random seed, so a demo run is repeatable per device.
func seedOf(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
