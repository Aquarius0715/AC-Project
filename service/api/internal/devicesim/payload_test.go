package devicesim

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

const (
	tenant = "820cc6b6-9ea8-5d18-8b10-5c8c1d126814"
	device = "11111111-2222-3333-4444-555555555555"
)

// TestTopicsFollowTheDesign: the topics are t/<tenantId>/d/<deviceId>/<kind>, telemetry and heartbeat are the routine ones.
func TestTopicsFollowTheDesign(t *testing.T) {
	m := NewModule(tenant, device, "v1", 1)
	if got, want := m.TelemetryTopic(), "t/"+tenant+"/d/"+device+"/telemetry"; got != want {
		t.Errorf("telemetry topic %q, want %q", got, want)
	}
	if got, want := m.HeartbeatTopic(), "t/"+tenant+"/d/"+device+"/heartbeat"; got != want {
		t.Errorf("heartbeat topic %q, want %q", got, want)
	}
}

// TestTelemetryBatchShape: one reading per sensor, the design's field names, and readings in the simulated range.
func TestTelemetryBatchShape(t *testing.T) {
	m := NewModule(tenant, device, "v1", 7)
	at := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	raw, err := m.Telemetry(at)
	if err != nil {
		t.Fatal(err)
	}
	var b map[string]any
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	if b["deviceId"] != device {
		t.Errorf("deviceId %v", b["deviceId"])
	}
	ms := b["measurements"].([]any)
	if len(ms) != len(Sensors) {
		t.Fatalf("%d measurements, want %d", len(ms), len(Sensors))
	}
	for i, x := range ms {
		row := x.(map[string]any)
		for _, k := range []string{"sensorId", "metric", "value", "unit", "observedAt", "sequence"} {
			if _, ok := row[k]; !ok {
				t.Errorf("measurement %d lacks %s", i, k)
			}
		}
		s := Sensors[i]
		v := row["value"].(float64)
		if math.Abs(v-s.Base) > s.Swing+0.01 {
			t.Errorf("%s value %v outside %v±%v", s.ID, v, s.Base, s.Swing)
		}
		if row["observedAt"] != "2026-10-13T02:00:00Z" {
			t.Errorf("observedAt %v", row["observedAt"])
		}
	}
}

// TestSequenceNeverRepeats: each reading takes the next sequence number, across batches of the same module.
func TestSequenceNeverRepeats(t *testing.T) {
	m := NewModule(tenant, device, "v1", 3)
	seen := map[float64]bool{}
	for i := 0; i < 3; i++ {
		raw, err := m.Telemetry(time.Now())
		if err != nil {
			t.Fatal(err)
		}
		var b struct {
			Measurements []struct {
				Sequence float64 `json:"sequence"`
			} `json:"measurements"`
		}
		if err := json.Unmarshal(raw, &b); err != nil {
			t.Fatal(err)
		}
		for _, x := range b.Measurements {
			if seen[x.Sequence] {
				t.Fatalf("sequence %v repeated", x.Sequence)
			}
			seen[x.Sequence] = true
		}
	}
	if len(seen) != 3*len(Sensors) {
		t.Errorf("%d distinct sequences, want %d", len(seen), 3*len(Sensors))
	}
}

// TestHeartbeatCarriesFirmware: the heartbeat names the device, its time in UTC and its firmware.
func TestHeartbeatCarriesFirmware(t *testing.T) {
	m := NewModule(tenant, device, "v2.1", 1)
	raw, err := m.Beat(time.Date(2026, 10, 13, 10, 0, 0, 0, time.FixedZone("MYT", 8*3600)))
	if err != nil {
		t.Fatal(err)
	}
	var h map[string]any
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if h["deviceId"] != device || h["firmware"] != "v2.1" || h["at"] != "2026-10-13T02:00:00Z" {
		t.Errorf("heartbeat %v", h)
	}
}

// TestAcknowledgeAppliesAndExpires: a set-temperature command in time is applied and acknowledged with the new
// setpoint; a command past its expiry is not applied and the module keeps its setpoint; an unknown kind is unsupported.
func TestAcknowledgeAppliesAndExpires(t *testing.T) {
	m := NewModule(tenant, device, "v1", 1)
	now := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	ack := func(cmd string, at time.Time) map[string]any {
		raw, err := m.Acknowledge([]byte(cmd), at)
		if err != nil {
			t.Fatal(err)
		}
		var a map[string]any
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	a := ack(`{"commandId":"c1","kind":"set_temperature","setpointCelsius":26,"expiresAt":"2026-10-13T02:05:00Z"}`, now)
	if a["result"] != "applied" || a["commandId"] != "c1" || a["observed"].(map[string]any)["setpointCelsius"] != 26.0 {
		t.Fatalf("applied: %v", a)
	}
	a = ack(`{"commandId":"c2","kind":"set_temperature","setpointCelsius":18,"expiresAt":"2026-10-13T02:05:00Z"}`, now.Add(10*time.Minute))
	if a["result"] != "expired" || a["observed"].(map[string]any)["setpointCelsius"] != 26.0 {
		t.Fatalf("expired command changed the setpoint: %v", a)
	}
	a = ack(`{"commandId":"c3","kind":"reboot","expiresAt":"2026-10-13T02:05:00Z"}`, now)
	if a["result"] != "unsupported" {
		t.Fatalf("unsupported: %v", a)
	}
	if _, err := m.Acknowledge([]byte(`not json`), now); err == nil {
		t.Error("a malformed command is accepted")
	}
}

// TestCommandAndAckTopics: a module hears its commands on cmd and answers on cmd/ack, both in its own tenant.
func TestCommandAndAckTopics(t *testing.T) {
	m := NewModule(tenant, device, "v1", 1)
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
	m := NewModule(tenant, device, "v1", 1)
	if _, err := m.Acknowledge([]byte(`{"commandId":`), time.Now()); err == nil {
		t.Error("a malformed command is accepted")
	}
	raw, err := m.Acknowledge([]byte(`{"commandId":"c9","kind":"reboot","expiresAt":"2030-01-01T00:00:00Z"}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var a map[string]any
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	if a["result"] != "unsupported" || a["observed"].(map[string]any)["setpointCelsius"] != 24.0 {
		t.Errorf("unknown kind: %v", a)
	}
}
