package devicesim_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
)

const (
	tenant = "820cc6b6-9ea8-5d18-8b10-5c8c1d126814"
	device = "11111111-2222-3333-4444-555555555555"
)

// TestTopicsFollowTheDesign: the topics are t/<tenantId>/d/<deviceId>/<kind>, telemetry and heartbeat are the routine
// ones.
func TestTopicsFollowTheDesign(t *testing.T) {
	t.Parallel()

	m := devicesim.NewModule(tenant, device, "v1", 1)
	if got, want := m.TelemetryTopic(), "t/"+tenant+"/d/"+device+"/telemetry"; got != want {
		t.Errorf("telemetry topic %q, want %q", got, want)
	}

	if got, want := m.HeartbeatTopic(), "t/"+tenant+"/d/"+device+"/heartbeat"; got != want {
		t.Errorf("heartbeat topic %q, want %q", got, want)
	}
}

// TestTelemetryBatchShape: one reading per sensor, the design's field names, and readings in the simulated range.
func TestTelemetryBatchShape(t *testing.T) {
	t.Parallel()

	m := devicesim.NewModule(tenant, device, "v1", 7)
	at := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)

	raw, err := m.Telemetry(at)
	if err != nil {
		t.Fatal(err)
	}

	var batch map[string]any

	unmarshalErr := json.Unmarshal(raw, &batch)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	if batch["deviceId"] != device {
		t.Errorf("deviceId %v", batch["deviceId"])
	}

	ms, found := batch["measurements"].([]any)
	if !found || len(ms) != len(devicesim.Sensors) {
		t.Fatalf("%d measurements, want %d", len(ms), len(devicesim.Sensors))
	}

	for idx, x := range ms {
		checkReading(t, idx, x, devicesim.Sensors[idx])
	}
}

// checkReading checks one reading of the batch TestTelemetryBatchShape takes: the design's field names, a value in the
// sensor's simulated range, and the observed time.
func checkReading(t *testing.T, idx int, reading any, sensor devicesim.Sensor) {
	t.Helper()

	row, found := reading.(map[string]any)
	if !found {
		t.Fatalf("measurement %d is %v", idx, reading)
	}

	for _, k := range []string{"sensorId", "metric", "value", "unit", "observedAt", "sequence"} {
		if _, ok := row[k]; !ok {
			t.Errorf("measurement %d lacks %s", idx, k)
		}
	}

	value, found := row["value"].(float64)
	if !found {
		t.Fatalf("measurement %d value %v", idx, row["value"])
	}

	if math.Abs(value-sensor.Base) > sensor.Swing+0.01 {
		t.Errorf("%s value %v outside %v±%v", sensor.ID, value, sensor.Base, sensor.Swing)
	}

	if row["observedAt"] != "2026-10-13T02:00:00Z" {
		t.Errorf("observedAt %v", row["observedAt"])
	}
}

// TestSequenceNeverRepeats: each reading takes the next sequence number, across batches of the same module.
func TestSequenceNeverRepeats(t *testing.T) {
	t.Parallel()

	m := devicesim.NewModule(tenant, device, "v1", 3)
	seen := map[float64]bool{}

	for range 3 {
		raw, err := m.Telemetry(time.Now())
		if err != nil {
			t.Fatal(err)
		}

		var batch struct {
			Measurements []struct {
				Sequence float64 `json:"sequence"`
			} `json:"measurements"`
		}

		unmarshalErr := json.Unmarshal(raw, &batch)
		if unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}

		for _, x := range batch.Measurements {
			if seen[x.Sequence] {
				t.Fatalf("sequence %v repeated", x.Sequence)
			}

			seen[x.Sequence] = true
		}
	}

	if len(seen) != 3*len(devicesim.Sensors) {
		t.Errorf("%d distinct sequences, want %d", len(seen), 3*len(devicesim.Sensors))
	}
}

// TestHeartbeatCarriesFirmware: the heartbeat names the device, its time in UTC and its firmware.
func TestHeartbeatCarriesFirmware(t *testing.T) {
	t.Parallel()

	m := devicesim.NewModule(tenant, device, "v2.1", 1)

	raw, err := m.Beat(time.Date(2026, 10, 13, 10, 0, 0, 0, time.FixedZone("MYT", 8*3600)))
	if err != nil {
		t.Fatal(err)
	}

	var heartbeat map[string]any

	unmarshalErr := json.Unmarshal(raw, &heartbeat)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	if heartbeat["deviceId"] != device || heartbeat["firmware"] != "v2.1" ||
		heartbeat["at"] != "2026-10-13T02:00:00Z" {
		t.Errorf("heartbeat %v", heartbeat)
	}
}

// observedSetpoint is the setpoint an acknowledgement says the module observes; the test fails without one.
func observedSetpoint(t *testing.T, answer map[string]any) any {
	t.Helper()

	observed, found := answer["observed"].(map[string]any)
	if !found {
		t.Fatalf("no observed state in %v", answer)
	}

	return observed["setpointCelsius"]
}

// TestAcknowledgeAppliesAndExpires: a set-temperature command in time is applied and acknowledged with the new
// setpoint; a command past its expiry is not applied and the module keeps its setpoint; an unknown kind is unsupported.
func TestAcknowledgeAppliesAndExpires(t *testing.T) {
	t.Parallel()

	module := devicesim.NewModule(tenant, device, "v1", 1)
	now := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	ack := func(cmd string, at time.Time) map[string]any {
		raw, _, err := module.Acknowledge([]byte(cmd), at)
		if err != nil {
			t.Fatal(err)
		}

		var body map[string]any

		unmarshalErr := json.Unmarshal(raw, &body)
		if unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}

		return body
	}

	answer := ack(
		`{"commandId":"c1","kind":"set_temperature","setpointCelsius":26,"expiresAt":"2026-10-13T02:05:00Z"}`,
		now,
	)
	if answer["result"] != "applied" || answer["commandId"] != "c1" ||
		observedSetpoint(t, answer) != 26.0 {
		t.Fatalf("applied: %v", answer)
	}

	answer = ack(
		`{"commandId":"c2","kind":"set_temperature","setpointCelsius":18,"expiresAt":"2026-10-13T02:05:00Z"}`,
		now.Add(10*time.Minute),
	)
	if answer["result"] != "expired" || observedSetpoint(t, answer) != 26.0 {
		t.Fatalf("expired command changed the setpoint: %v", answer)
	}

	answer = ack(`{"commandId":"c3","kind":"reboot","expiresAt":"2026-10-13T02:05:00Z"}`, now)
	if answer["result"] != "unsupported" {
		t.Fatalf("unsupported: %v", answer)
	}

	_, _, err := module.Acknowledge([]byte(`not json`), now)
	if err == nil {
		t.Error("a malformed command is accepted")
	}
}

// TestCommandAndAckTopics: a module hears its commands on cmd and answers on cmd/ack, both in its own tenant.
func TestCommandAndAckTopics(t *testing.T) {
	t.Parallel()

	m := devicesim.NewModule(tenant, device, "v1", 1)
	if got, want := m.CommandTopic(), "t/"+tenant+"/d/"+device+"/cmd"; got != want {
		t.Errorf("command topic %q, want %q", got, want)
	}

	if got, want := m.AckTopic(), "t/"+tenant+"/d/"+device+"/cmd/ack"; got != want {
		t.Errorf("ack topic %q, want %q", got, want)
	}
}

// TestAcknowledgeRefusesMalformedAndKeepsUnknownKinds: a command that is not JSON is an error and changes nothing; a
// command of an unknown kind is answered unsupported with the setpoint unchanged.
func TestAcknowledgeRefusesMalformedAndKeepsUnknownKinds(t *testing.T) {
	t.Parallel()

	module := devicesim.NewModule(tenant, device, "v1", 1)

	_, _, err := module.Acknowledge([]byte(`{"commandId":`), time.Now())
	if err == nil {
		t.Error("a malformed command is accepted")
	}

	raw, _, err := module.Acknowledge(
		[]byte(`{"commandId":"c9","kind":"reboot","expiresAt":"2030-01-01T00:00:00Z"}`),
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	var answer map[string]any

	unmarshalErr := json.Unmarshal(raw, &answer)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	if answer["result"] != "unsupported" || observedSetpoint(t, answer) != 24.0 {
		t.Errorf("unknown kind: %v", answer)
	}
}
