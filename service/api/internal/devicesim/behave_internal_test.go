package devicesim

import (
	"encoding/json"
	"testing"
	"time"
)

const (
	tenant = "820cc6b6-9ea8-5d18-8b10-5c8c1d126814"
	device = "11111111-2222-3333-4444-555555555555"
	// setTo27 is a set-temperature command to 27 °C that never expires.
	setTo27 = `{"commandId":"c1","kind":"set_temperature","setpointCelsius":27,"expiresAt":"2099-01-01T00:00:00Z"}`
)

func readings(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	var body struct {
		Measurements []map[string]any `json:"measurements"`
	}

	err := json.Unmarshal(raw, &body)
	if err != nil {
		t.Fatal(err)
	}

	return body.Measurements
}

// telemetryOf is the telemetry batch at when of a module playing a telemetry behaviour.
func telemetryOf(t *testing.T, when time.Time, telemetry string, powerOff bool) []byte {
	t.Helper()

	raw, err := NewModule(
		tenant,
		device,
		"v1",
		1,
	).Behave(telemetry, "apply", powerOff).
		Telemetry(when)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

// TestTelemetryBehaviours: each pattern's readings are the problem the API is meant to flag.
func TestTelemetryBehaviours(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)

	if telemetryOf(t, when, "silent", false) != nil {
		t.Error("a silent module reports")
	}

	if r := readings(
		t,
		telemetryOf(t, when, "unit_mismatch", false),
	); r[0]["unit"] != "K" ||
		r[1]["unit"] != "kW" {
		t.Errorf("unit mismatch: %v", r)
	}

	if r := readings(t, telemetryOf(t, when, "out_of_range", false)); r[0]["value"] != 250.0 {
		t.Errorf("out of range: %v", r)
	}

	for _, x := range readings(t, telemetryOf(t, when, "missing", false)) {
		if x["value"] != nil {
			t.Errorf("missing has a value: %v", x)
		}
	}

	if r := readings(
		t,
		telemetryOf(t, when, "invalid_time", false),
	); r[0]["observedAt"] != "2026-10-13T03:00:00Z" {
		t.Errorf("invalid time: %v", r)
	}

	if r := readings(
		t,
		telemetryOf(t, when, "normal", true),
	); r[1]["metric"] != "power" ||
		r[1]["value"] != 0.0 {
		t.Errorf("power off: %v", r)
	}
}

// answerOf is the acknowledgement (nil when there is none) of a module playing a command behaviour, and when it is
// sent.
func answerOf(
	t *testing.T,
	commands string,
	cmd []byte,
	now time.Time,
) (map[string]any, time.Time) {
	t.Helper()

	raw, when, err := NewModule(
		tenant,
		device,
		"v1",
		1,
	).Behave("normal", commands, false).
		Acknowledge(cmd, now)
	if err != nil {
		t.Fatal(err)
	}

	if raw == nil {
		return nil, when
	}

	var answer map[string]any

	unmarshalErr := json.Unmarshal(raw, &answer)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	return answer, when
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

// TestCommandBehaviours: reject answers unsupported and keeps the setpoint, ignore answers nothing, late applies the
// command and sends the answer after the command expired.
func TestCommandBehaviours(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	cmd := []byte(
		`{"commandId":"c1","kind":"set_temperature","setpointCelsius":26,"expiresAt":"2026-10-13T02:00:30Z"}`,
	)

	answer, sentAt := answerOf(t, "apply", cmd, now)
	if answer["result"] != resultApplied || !sentAt.Equal(now) {
		t.Errorf("apply: %v at %v", answer, sentAt)
	}

	answer, _ = answerOf(t, "reject", cmd, now)
	if answer["result"] != resultUnsupported || observedSetpoint(t, answer) != 24.0 {
		t.Errorf("reject: %v", answer)
	}

	answer, _ = answerOf(t, "ignore", cmd, now)
	if answer != nil {
		t.Errorf("ignore answered: %v", answer)
	}

	answer, sentAt = answerOf(t, "late", cmd, now)
	if answer["result"] != resultApplied ||
		!sentAt.Equal(time.Date(2026, 10, 13, 2, 0, 32, 0, time.UTC)) {
		t.Errorf("late: %v at %v", answer, sentAt)
	}
}

// TestOnlyNarrowsTheReadings: a module narrowed to two sensors reports those two, in the register's order.
func TestOnlyNarrowsTheReadings(t *testing.T) {
	t.Parallel()

	raw, err := NewModule(
		tenant,
		device,
		"v1",
		1,
	).Only("power", "temperature", "unknown").
		Telemetry(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	r := readings(t, raw)
	if len(r) != 2 || r[0]["metric"] != "temperature" || r[1]["metric"] != "power" {
		t.Errorf("readings %v", r)
	}
}

// TestAModuleWithoutSensorsOnlyBeats: a module narrowed to no sensor sends no readings, and still its heartbeat
// (IR613).
func TestAModuleWithoutSensorsOnlyBeats(t *testing.T) {
	t.Parallel()

	module := NewModule(tenant, device, "v1", 1).Only()

	raw, err := module.Telemetry(time.Now())
	if err != nil || raw != nil {
		t.Errorf("readings %s %v", raw, err)
	}

	raw, err = module.Beat(time.Now())
	if err != nil || raw == nil {
		t.Errorf("heartbeat %s %v", raw, err)
	}
}

// heartbeatOf is a module's heartbeat now.
func heartbeatOf(t *testing.T, module *Module) map[string]any {
	t.Helper()

	raw, err := module.Beat(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	var heartbeat map[string]any

	unmarshalErr := json.Unmarshal(raw, &heartbeat)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	return heartbeat
}

// reportedState is the state a module's heartbeat reports; the test fails without one.
func reportedState(t *testing.T, module *Module) map[string]any {
	t.Helper()

	state, found := heartbeatOf(t, module)["state"].(map[string]any)
	if !found {
		t.Fatal("the heartbeat reports no state")
	}

	return state
}

// acknowledge applies a command to the module now; the test fails when the command is malformed.
func acknowledge(t *testing.T, module *Module, cmd string) {
	t.Helper()

	_, _, err := module.Acknowledge([]byte(cmd), time.Now())
	if err != nil {
		t.Fatal(err)
	}
}

// TestHeartbeatReportsTheState: a module that reports its state sends it with the heartbeat, and a set-temperature
// command it applies changes the reported setpoint; a module that does not report sends no state.
func TestHeartbeatReportsTheState(t *testing.T) {
	t.Parallel()

	on, c, mode, fan := true, 24.0, "cool", "mid"
	module := NewModule(tenant, device, "v1", 1).Reports(&on, &c, &mode, &fan)

	st := reportedState(t, module)
	if st["power"] != true || st["celsius"] != 24.0 || st["mode"] != "cool" ||
		st["fanLevel"] != "mid" {
		t.Errorf("state %v", st)
	}

	acknowledge(t, module, setTo27)

	if st := reportedState(t, module); st["celsius"] != 27.0 {
		t.Errorf("state after the command %v", st)
	}

	if _, ok := heartbeatOf(t, NewModule(tenant, device, "v1", 1))["state"]; ok {
		t.Error("a module that does not report sends a state")
	}
}

// TestKeptSetpoint: the setpoint a command applied survives a restart through the kept state, and the reported state
// follows it; a kept setpoint outside 16–30 °C or a broken message is refused.
func TestKeptSetpoint(t *testing.T) {
	t.Parallel()

	celsius, power := 24.0, true

	module := NewModule("t", "d", "v1", 1).Reports(&power, &celsius, nil, nil)
	acknowledge(t, module, setTo27)

	kept, err := module.Keep()
	if err != nil || string(kept) != `{"setpointCelsius":27}` {
		t.Fatalf("keep: %s %v", kept, err)
	}

	c2 := 24.0

	again := NewModule("t", "d", "v1", 1).Reports(&power, &c2, nil, nil)

	restoreErr := again.Restore(
		kept,
	)
	if restoreErr != nil || again.setpoint != 27 ||
		*again.state.Celsius != 27 {
		t.Fatalf("restore: %v %v", restoreErr, again.setpoint)
	}

	if again.KeptTopic() != "t/t/d/d/sim/kept" {
		t.Errorf("topic %s", again.KeptTopic())
	}

	for _, bad := range []string{`{"setpointCelsius":45}`, `not json`} {
		err := again.Restore([]byte(bad))
		if err == nil || again.setpoint != 27 {
			t.Errorf("restore %s: %v %v", bad, err, again.setpoint)
		}
	}
}
