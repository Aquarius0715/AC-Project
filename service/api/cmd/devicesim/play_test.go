package main

import (
	"context"
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// doneToken is a publish the broker took at once.
type doneToken struct{}

func (doneToken) Wait() bool                     { return true }
func (doneToken) WaitTimeout(time.Duration) bool { return true }
func (doneToken) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)

	return done
}
func (doneToken) Error() error { return nil }

// sent is one message the fake broker took.
type sent struct {
	topic    string
	retained bool
	payload  []byte
}

// fakeBroker keeps what is published; the client's other methods are not used.
type fakeBroker struct {
	mqtt.Client

	mu   sync.Mutex
	sent []sent
}

//nolint:ireturn // mqtt.Client's Publish returns the Token interface, which the fake implements
func (b *fakeBroker) Publish(topic string, _ byte, retained bool, payload any) mqtt.Token {
	b.mu.Lock()
	defer b.mu.Unlock()

	raw, _ := payload.([]byte)
	b.sent = append(b.sent, sent{topic: topic, retained: retained, payload: raw})

	return doneToken{}
}

func (b *fakeBroker) topics() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]string, 0, len(b.sent))
	for _, msg := range b.sent {
		out = append(out, msg.topic)
	}

	return out
}

func (b *fakeBroker) message(topic string) (sent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, msg := range b.sent {
		if msg.topic == topic {
			return msg, true
		}
	}

	return sent{topic: "", retained: false, payload: nil}, false
}

// powerReading is the power value of a telemetry batch (nil when the batch has none).
func powerReading(t *testing.T, raw []byte) *float64 {
	t.Helper()

	batch := new(devicesim.Batch)

	err := json.Unmarshal(raw, batch)
	if err != nil {
		t.Fatalf("batch %s: %v", raw, err)
	}

	for _, m := range batch.Measurements {
		if m.Metric == "power" {
			return m.Value
		}
	}

	return nil
}

// TestFleetModulesPlayTheirPattern: the fleet runs one module per pattern with a device, beats only for the online
// ones, and an air conditioner that is switched off or has lost its power signal reports 0 kW.
func TestFleetModulesPlayTheirPattern(t *testing.T) {
	t.Parallel()

	modules, beats, err := fleetModules("t")
	if err != nil {
		t.Fatal(err)
	}

	byKey := map[string]*devicesim.Module{}
	for _, m := range modules {
		byKey[m.DeviceID] = m
	}

	module := func(key string) *devicesim.Module {
		return byKey[seed.ID("device-mock-"+key).String()]
	}

	if len(modules) != 18 || module("no-module") != nil || module("archived") != nil {
		t.Fatalf("%d modules; a pattern without a device has a module", len(modules))
	}

	for key, want := range map[string]bool{"cool-on": true, "offline": false, "connecting": false, "power-lost": true} {
		if beats[module(key)] != want {
			t.Errorf("%s beats=%v, want %v", key, beats[module(key)], want)
		}
	}

	when := time.Date(2026, 9, 15, 1, 30, 0, 0, time.UTC)

	for key, off := range map[string]bool{"cool-on": false, "dry-off": true, "power-lost": true} {
		raw, err := module(key).Telemetry(when)
		if err != nil {
			t.Fatal(err)
		}

		kw := powerReading(t, raw)
		if kw == nil || (*kw == 0) != off {
			t.Errorf("%s power %v, want off=%v", key, kw, off)
		}
	}
}

// TestFixtureModulesSkipBlankNames: blank names in --devices are skipped; each named device gets one module.
func TestFixtureModulesSkipBlankNames(t *testing.T) {
	t.Parallel()

	modules, beats := fixtureModules("t", " device-online-rto, ,device-offline-rto,", "v1")
	if len(modules) != 2 || len(beats) != 2 {
		t.Fatalf("%d modules, %d beats", len(modules), len(beats))
	}

	if modules[0].DeviceID != seed.ID("device-online-rto").String() || !beats[modules[0]] ||
		beats[modules[1]] {
		t.Errorf("modules %s %s beats %v", modules[0].DeviceID, modules[1].DeviceID, beats)
	}
}

// TestSendOnlyWhatThePatternSends: a module that does not beat sends no heartbeat, a silent one no telemetry.
func TestSendOnlyWhatThePatternSends(t *testing.T) {
	t.Parallel()

	online := devicesim.NewModule("t", "on", "v1", 1)
	silent := devicesim.NewModule("t", "off", "v1", 2).Behave("silent", "ignore", false)
	modules := []*devicesim.Module{online, silent}
	now := time.Date(2026, 9, 15, 1, 30, 0, 0, time.UTC)

	broker := new(fakeBroker)
	sendHeartbeats(broker, modules, map[*devicesim.Module]bool{online: true, silent: false}, now)
	sendTelemetry(broker, modules, now)

	want := []string{online.HeartbeatTopic(), online.TelemetryTopic()}
	if got := broker.topics(); !slices.Equal(got, want) {
		t.Errorf("published %v, want %v", got, want)
	}
}

// command is a set_temperature command to 22 °C that expires after the given time.
func command(t *testing.T, expiresIn time.Duration) []byte {
	t.Helper()

	setpoint := 22.0

	raw, err := json.Marshal(devicesim.Command{
		CommandID: "c1",
		Kind:      "set_temperature",
		Setpoint:  &setpoint,
		ExpiresAt: time.Now().Add(expiresIn),
	})
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

// TestAnswerKeepsTheStateThenAnswersAsThePatternDoes: every command first keeps the state (retained); then an apply
// module answers at once, an ignore module never, a late one only after the command expired.
func TestAnswerKeepsTheStateThenAnswersAsThePatternDoes(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"apply", "ignore", "late"} {
		t.Run(pattern, func(t *testing.T) {
			t.Parallel()

			module := devicesim.NewModule("t", pattern, "v1", 1).Behave("normal", pattern, false)
			broker := new(fakeBroker)

			// far enough ahead that a loaded runner does not reach the expiry before answer reads the clock
			answer(broker, module, command(t, 500*time.Millisecond))

			kept, ok := broker.message(module.KeptTopic())
			if !ok || !kept.retained {
				t.Fatalf("kept state %+v %v", kept, ok)
			}

			// an ignore module applies nothing, so it keeps its pattern's setpoint
			if pattern != "ignore" && string(kept.payload) != `{"setpointCelsius":22}` {
				t.Errorf("kept %s", kept.payload)
			}

			_, acked := broker.message(module.AckTopic())
			if acked != (pattern == "apply") {
				t.Errorf("answered at once: %v", acked)
			}

			if pattern != "late" {
				return
			}

			// the late answer comes 2 s after the expiry
			deadline := time.Now().Add(10 * time.Second)
			for !acked && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)

				_, acked = broker.message(module.AckTopic())
			}

			if !acked {
				t.Error("the late module never answered")
			}
		})
	}
}

// TestAnswerDropsABrokenCommand: a command that is not JSON is neither kept nor answered.
func TestAnswerDropsABrokenCommand(t *testing.T) {
	t.Parallel()

	broker := new(fakeBroker)
	answer(broker, devicesim.NewModule("t", "d", "v1", 1), []byte("{"))

	if got := broker.topics(); len(got) != 0 {
		t.Errorf("published %v", got)
	}
}

// subscribingBroker also records the subscriptions, in order, and their handlers.
type subscribingBroker struct {
	fakeBroker

	order    []string
	handlers map[string]mqtt.MessageHandler
}

//nolint:ireturn // mqtt.Client's Subscribe returns the Token interface, which the fake implements
func (b *subscribingBroker) Subscribe(
	topic string,
	_ byte,
	callback mqtt.MessageHandler,
) mqtt.Token {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.order = append(b.order, topic)
	b.handlers[topic] = callback

	return doneToken{}
}

// message is a delivered message; only its payload is read.
type message struct {
	mqtt.Message

	payload []byte
}

func (m message) Payload() []byte { return m.payload }

// TestSubscribeRestoresTheKeptStateThenAnswers: every module first subscribes to its kept topic, which restores the
// setpoint, and only then do the modules hear their command topics, which answer; a command after a restart keeps the
// restored setpoint (IR613).
func TestSubscribeRestoresTheKeptStateThenAnswers(t *testing.T) {
	t.Parallel()

	module := devicesim.NewModule("t", "d", "v1", 1)
	other := devicesim.NewModule("t", "e", "v1", 2)
	broker := new(subscribingBroker)
	broker.handlers = map[string]mqtt.MessageHandler{}

	subscribe(broker, []*devicesim.Module{module, other})

	if want := []string{
		module.KeptTopic(),
		other.KeptTopic(),
		module.CommandTopic(),
		other.CommandTopic(),
	}; !slices.Equal(
		broker.order,
		want,
	) {
		t.Fatalf("subscribed %v, want %v", broker.order, want)
	}

	broker.handlers[module.KeptTopic()](
		broker,
		message{Message: nil, payload: []byte(`{"setpointCelsius":19}`)},
	)

	// a command that changes no setpoint: the kept state it publishes is the restored one
	raw, err := json.Marshal(devicesim.Command{
		CommandID: "c1",
		Kind:      "power",
		Setpoint:  nil,
		ExpiresAt: time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	broker.handlers[module.CommandTopic()](broker, message{Message: nil, payload: raw})

	kept, ok := broker.message(module.KeptTopic())
	if !ok || string(kept.payload) != `{"setpointCelsius":19}` {
		t.Errorf("kept %s %v, want the restored setpoint 19", kept.payload, ok)
	}

	if _, acked := broker.message(module.AckTopic()); !acked {
		t.Error("the command was not answered")
	}
}

// TestPlaySendsOnItsTickersUntilTheContextEnds: play sends heartbeats and telemetry on their tickers and returns when
// the context ends.
func TestPlaySendsOnItsTickersUntilTheContextEnds(t *testing.T) {
	t.Parallel()

	module := devicesim.NewModule("t", "d", "v1", 1)
	broker := new(fakeBroker)
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		play(
			ctx,
			broker,
			[]*devicesim.Module{module},
			map[*devicesim.Module]bool{module: true},
			config{
				broker: "", tenant: "", names: "", firmware: "",
				interval: time.Millisecond, beat: time.Millisecond, withFleet: false,
			},
		)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got := broker.topics()
		if slices.Contains(got, module.TelemetryTopic()) &&
			slices.Contains(got, module.HeartbeatTopic()) {
			break
		}

		time.Sleep(5 * time.Millisecond)
	}

	cancel()

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("play did not stop when the context ended")
	}

	got := broker.topics()
	if !slices.Contains(got, module.TelemetryTopic()) ||
		!slices.Contains(got, module.HeartbeatTopic()) {
		t.Errorf("published %v, want telemetry and heartbeats", got)
	}
}
