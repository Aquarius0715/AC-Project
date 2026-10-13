package devicesim

import (
	"encoding/json"
	"testing"
	"time"
)

func readings(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var b struct {
		Measurements []map[string]any `json:"measurements"`
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	return b.Measurements
}

// TestTelemetryBehaviours: each pattern's readings are the problem the API is meant to flag.
func TestTelemetryBehaviours(t *testing.T) {
	at := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	run := func(telemetry string, powerOff bool) []byte {
		raw, err := NewModule(tenant, device, "v1", 1).Behave(telemetry, "apply", powerOff).Telemetry(at)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if run("silent", false) != nil {
		t.Error("a silent module reports")
	}
	if r := readings(t, run("unit_mismatch", false)); r[0]["unit"] != "K" || r[1]["unit"] != "kW" {
		t.Errorf("unit mismatch: %v", r)
	}
	if r := readings(t, run("out_of_range", false)); r[0]["value"] != 250.0 {
		t.Errorf("out of range: %v", r)
	}
	for _, x := range readings(t, run("missing", false)) {
		if x["value"] != nil {
			t.Errorf("missing has a value: %v", x)
		}
	}
	if r := readings(t, run("invalid_time", false)); r[0]["observedAt"] != "2026-10-13T03:00:00Z" {
		t.Errorf("invalid time: %v", r)
	}
	if r := readings(t, run("normal", true)); r[1]["metric"] != "power" || r[1]["value"] != 0.0 {
		t.Errorf("power off: %v", r)
	}
}

// TestCommandBehaviours: reject answers unsupported and keeps the setpoint, ignore answers nothing, late applies the
// command and sends the answer after the command expired.
func TestCommandBehaviours(t *testing.T) {
	now := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	cmd := []byte(`{"commandId":"c1","kind":"set_temperature","setpointCelsius":26,"expiresAt":"2026-10-13T02:00:30Z"}`)
	answer := func(commands string) (map[string]any, time.Time) {
		raw, at, err := NewModule(tenant, device, "v1", 1).Behave("normal", commands, false).Acknowledge(cmd, now)
		if err != nil {
			t.Fatal(err)
		}
		if raw == nil {
			return nil, at
		}
		var a map[string]any
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		return a, at
	}
	if a, at := answer("apply"); a["result"] != "applied" || !at.Equal(now) {
		t.Errorf("apply: %v at %v", a, at)
	}
	if a, _ := answer("reject"); a["result"] != "unsupported" || a["observed"].(map[string]any)["setpointCelsius"] != 24.0 {
		t.Errorf("reject: %v", a)
	}
	if a, _ := answer("ignore"); a != nil {
		t.Errorf("ignore answered: %v", a)
	}
	a, at := answer("late")
	if a["result"] != "applied" || !at.Equal(time.Date(2026, 10, 13, 2, 0, 32, 0, time.UTC)) {
		t.Errorf("late: %v at %v", a, at)
	}
}

// TestOnlyNarrowsTheReadings: a module narrowed to two sensors reports those two, in the register's order.
func TestOnlyNarrowsTheReadings(t *testing.T) {
	raw, err := NewModule(tenant, device, "v1", 1).Only("power", "temperature", "unknown").Telemetry(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := readings(t, raw)
	if len(r) != 2 || r[0]["metric"] != "temperature" || r[1]["metric"] != "power" {
		t.Errorf("readings %v", r)
	}
}
