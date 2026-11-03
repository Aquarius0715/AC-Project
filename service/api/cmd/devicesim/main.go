// Command devicesim runs simulated IoT modules for the demo: each module publishes a telemetry batch every minute and a
// heartbeat every 60 seconds to the MQTT broker, as backend-architecture §9 describes; a device that is not online in
// the fixture or the fleet sends neither (IR613). The devices must already be
// registered (their IDs come from --device-ids); the broker login comes from MQTT_USER and MQTT_PASSWORD, because the
// broker refuses anonymous clients (security loop, IR413).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"log"
	"maps"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// keepAlive is the MQTT keep-alive interval, disconnectWait how long (in milliseconds) a disconnect waits for the work
// in flight, and clientTagLength how many characters of the tenant ID the client ID keeps.
const (
	keepAlive       = 60 * time.Second
	disconnectWait  = 500
	clientTagLength = 8
)

// errNoDevices refuses a run without a device to simulate.
var errNoDevices = errors.New("--devices must name at least one device")

// config is what the flags set.
type config struct {
	broker, tenant, names, firmware string
	interval, beat                  time.Duration
	withFleet                       bool
}

func main() {
	var cfg config

	flag.StringVar(
		&cfg.broker,
		"broker",
		envOr("MQTT_URL", "tcp://localhost:1883"),
		"MQTT broker URL",
	)
	flag.StringVar(
		&cfg.tenant,
		"tenant",
		seed.ID("tenant-a").String(),
		"tenant ID the devices belong to (the demo tenant by default)",
	)
	flag.StringVar(
		&cfg.names,
		"devices",
		strings.Join(demoDevices(), ","),
		"comma-separated fixture device IDs (demoSeed.devices); their API IDs are derived with seed.ID",
	)
	flag.StringVar(&cfg.firmware, "firmware", "v1", "firmware version the modules report")
	flag.DurationVar(
		&cfg.interval,
		"telemetry-interval",
		time.Minute,
		"time between telemetry batches (design: every minute)",
	)
	flag.DurationVar(
		&cfg.beat,
		"heartbeat-interval",
		time.Minute,
		"time between heartbeats (design: every 60 seconds)",
	)
	flag.BoolVar(
		&cfg.withFleet,
		"fleet",
		true,
		"also run the mock fleet (internal/devicesim/fleet.json): one module per state pattern",
	)

	flag.Parse()

	err := run(cfg)
	if err != nil {
		log.Fatal(err)
	}
}

// run plays the modules until a signal ends it; main exits on its error once its deferred clean-up has run.
func run(cfg config) error {
	if cfg.names == "" {
		return errNoDevices
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	modules, beats := fixtureModules(cfg.tenant, cfg.names, cfg.firmware)

	if cfg.withFleet {
		fleet, fleetBeats, err := fleetModules(cfg.tenant)
		if err != nil {
			return err
		}

		modules = append(modules, fleet...)

		maps.Copy(beats, fleetBeats)
	}

	client := mqtt.NewClient(clientOptions(cfg.broker, cfg.tenant, modules))
	if tok := client.Connect(); tok.Wait() && tok.Error() != nil {
		return fmt.Errorf("connect %s: %w", cfg.broker, tok.Error())
	}
	defer client.Disconnect(disconnectWait)

	log.Printf("devicesim: %d modules on %s", len(modules), cfg.broker)

	play(ctx, client, modules, beats, cfg)

	return nil
}

// fixtureModules are the modules of the named fixture devices (comma-separated) and which of them send heartbeats (a
// module that is not online sends none).
func fixtureModules(
	tenant, names, firmware string,
) ([]*devicesim.Module, map[*devicesim.Module]bool) {
	var modules []*devicesim.Module

	beats := map[*devicesim.Module]bool{}

	for name := range strings.SplitSeq(names, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		module, beating := fixtureModule(tenant, name, firmware)
		modules = append(modules, module)
		beats[module] = beating
	}

	return modules, beats
}

// fleetModules are the mock fleet's modules, one per state pattern that has a device, so the fleet's units show their
// state live, and which of them send heartbeats.
func fleetModules(tenant string) ([]*devicesim.Module, map[*devicesim.Module]bool, error) {
	fleet, err := devicesim.MockFleet()
	if err != nil {
		return nil, nil, apperr.Wrap(err, "devicesim.MockFleet")
	}

	modules := make([]*devicesim.Module, 0, len(fleet.Patterns))
	beats := map[*devicesim.Module]bool{}

	for _, pattern := range fleet.Patterns {
		if pattern.NoDevice {
			continue
		}

		id := seed.ID(devicesim.DeviceName(pattern)).String()
		powerOff := pattern.PowerSignal == "off" ||
			(pattern.State.Power != nil && !*pattern.State.Power)
		module := devicesim.NewModule(tenant, id, fleet.FirmwareOf(pattern), seedOf(id)).
			Behave(pattern.Telemetry, pattern.Commands, powerOff).
			Reports(pattern.State.Power, pattern.State.Celsius, pattern.State.Mode, pattern.State.FanLevel)
		modules = append(modules, module)
		beats[module] = pattern.Connection == "online"
	}

	return modules, beats, nil
}

// clientOptions are the broker connection's options. The subscriptions are made on every connect: a clean session
// loses them when the broker restarts.
func clientOptions(broker, tenant string, modules []*devicesim.Module) *mqtt.ClientOptions {
	return mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("devicesim-" + clientTag(tenant)).
		SetUsername(os.Getenv("MQTT_USER")).
		SetPassword(os.Getenv("MQTT_PASSWORD")).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetKeepAlive(keepAlive).
		SetOnConnectHandler(func(c mqtt.Client) { subscribe(c, modules) }).
		SetConnectionLostHandler(
			func(_ mqtt.Client, err error) { log.Printf("broker %s: connection lost: %v", broker, err) })
}

// play sends the telemetry batches and the heartbeats on their tickers until the context ends.
func play(
	ctx context.Context,
	client mqtt.Client,
	modules []*devicesim.Module,
	beats map[*devicesim.Module]bool,
	cfg config,
) {
	telemetry := time.NewTicker(cfg.interval)
	heartbeat := time.NewTicker(cfg.beat)

	defer telemetry.Stop()
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Print("devicesim: stopping")

			return
		case now := <-telemetry.C:
			sendTelemetry(client, modules, now)
		case now := <-heartbeat.C:
			sendHeartbeats(client, modules, beats, now)
		}
	}
}

// sendTelemetry publishes one telemetry batch per module; a silent pattern reports nothing.
func sendTelemetry(client mqtt.Client, modules []*devicesim.Module, now time.Time) {
	for _, module := range modules {
		raw, err := module.Telemetry(now)
		if err != nil {
			log.Printf("telemetry %s: %v", module.DeviceID, err)

			continue
		}

		if raw != nil {
			publish(client, module.TelemetryTopic(), raw)
		}
	}
}

// sendHeartbeats publishes one heartbeat per module that beats.
func sendHeartbeats(
	client mqtt.Client,
	modules []*devicesim.Module,
	beats map[*devicesim.Module]bool,
	now time.Time,
) {
	for _, module := range modules {
		if !beats[module] {
			continue
		}

		raw, err := module.Beat(now)
		if err != nil {
			log.Printf("heartbeat %s: %v", module.DeviceID, err)

			continue
		}

		publish(client, module.HeartbeatTopic(), raw)
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
	if len(tenant) > clientTagLength {
		return tenant[:clientTagLength]
	}

	return tenant
}

// fixtureModule is the module of a fixture device and whether it sends heartbeats. It plays its device's connection in
// the fixture, as the mock fleet does (IR613): an online device reports its sensors and beats; one that is offline
// sends neither readings nor heartbeats and leaves commands unanswered, so the demo shows its last known values. A
// device without a link is taken as online.
func fixtureModule(tenant, name, firmware string) (*devicesim.Module, bool) {
	id := seed.ID(name).String()

	module := devicesim.NewModule(tenant, id, firmware, seedOf(id)).Only(sensorsOf(name)...)
	for _, l := range iotbridge.Links {
		if l.Device == name && l.Connection != "online" {
			return module.Behave("silent", "ignore", false), false
		}
	}

	return module, true
}

// sensorsOf names the sensors a fixture device has (its bridge link), so it reports only those.
func sensorsOf(device string) []string {
	for _, l := range iotbridge.Links {
		if l.Device == device {
			out := make([]string, 0, len(l.Sensors))
			for name := range l.Sensors {
				out = append(out, name)
			}

			return out
		}
	}

	return nil
}

// demoDevices are the fixture devices (the links of internal/iotbridge), with or without sensors.
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

// subscribe makes the modules' subscriptions; it runs on every connect. Each module first reads back its kept state (a
// retained message; none on the first run), so a restart keeps the setpoint its last command set, then hears its own
// command topic and answers on the acknowledgement topic (QoS 1, as the design says). A subscription that fails is
// logged, and the next reconnect tries again.
func subscribe(client mqtt.Client, modules []*devicesim.Module) {
	for _, module := range modules {
		tok := client.Subscribe(module.KeptTopic(), 1, func(_ mqtt.Client, msg mqtt.Message) {
			err := module.Restore(msg.Payload())
			if err != nil {
				log.Printf("kept %s: %v", module.DeviceID, err)
			}
		})
		if tok.Wait() && tok.Error() != nil {
			log.Printf("subscribe %s: %v", module.KeptTopic(), tok.Error())
		}
	}

	for _, module := range modules {
		tok := client.Subscribe(module.CommandTopic(), 1, func(_ mqtt.Client, msg mqtt.Message) {
			answer(client, module, msg.Payload())
		})
		if tok.Wait() && tok.Error() != nil {
			log.Printf("subscribe %s: %v", module.CommandTopic(), tok.Error())
		}
	}
}

// answer acknowledges one command as the module's pattern does. It first keeps the new state (retained, so the next
// start reads it back), then answers at once, after the command expired, or never.
func answer(client mqtt.Client, module *devicesim.Module, payload []byte) {
	raw, sendAt, err := module.Acknowledge(payload, time.Now())
	if err != nil {
		log.Printf("command %s: %v", module.DeviceID, err)

		return
	}

	kept, keepErr := module.Keep()
	if keepErr == nil {
		tok := client.Publish(module.KeptTopic(), 1, true, kept)
		if tok.Wait() && tok.Error() != nil {
			log.Printf("publish %s: %v", module.KeptTopic(), tok.Error())
		}
	}

	if raw == nil { // the pattern never answers
		return
	}

	if wait := time.Until(sendAt); wait > 0 { // the pattern answers after the command expired
		time.AfterFunc(wait, func() { publish(client, module.AckTopic(), raw) })

		return
	}

	publish(client, module.AckTopic(), raw)
}
