package iotbridge_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
)

// TestCommandMessageFromTheStoredAction: a stored set-temperature action becomes the module's command with its setpoint
// and expiry; an action without a kind is refused; the sent report names the command.
func TestCommandMessageFromTheStoredAction(t *testing.T) {
	waiting := iotbridge.Waiting{
		ID:        uuid.MustParse("9f67089f-44c9-5231-ba20-03d3b9a1be53"),
		DeviceID:  iotbridge.DeviceID("device-online-rto"),
		Action:    json.RawMessage(`{"kind":"set_temperature","celsius":26}`),
		ExpiresAt: time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC), UnitID: uuid.UUID{},
	}

	cmd, err := iotbridge.CommandMessage(waiting, 0)
	if err != nil {
		t.Fatal(err)
	}

	if cmd.CommandID != waiting.ID.String() || cmd.Kind != "set_temperature" ||
		cmd.Setpoint == nil ||
		*cmd.Setpoint != 26 ||
		!cmd.ExpiresAt.Equal(waiting.ExpiresAt) {
		t.Errorf("command %+v", cmd)
	}

	waiting.Action = json.RawMessage(`{"celsius":26}`)

	_, commandMessageErr := iotbridge.CommandMessage(waiting, 0)
	if commandMessageErr == nil {
		t.Error("an action without a kind is accepted")
	}

	trigger := iotbridge.SentTrigger(waiting, waiting.ExpiresAt)
	if trigger.EventType != "command_sent" || trigger.CommandID == nil ||
		*trigger.CommandID != waiting.ID ||
		trigger.Measurement != nil {
		t.Errorf("sent trigger %+v", trigger)
	}

	if iotbridge.SentTrigger(waiting, waiting.ExpiresAt).EventID != trigger.EventID {
		t.Error("a repeated report has another event ID")
	}
}

// TestCommandTopicOfTheDevice: the command goes down the device's own topic in its tenant.
func TestCommandTopicOfTheDevice(t *testing.T) {
	tenant := uuid.MustParse("820cc6b6-9ea8-5d18-8b10-5c8c1d126814")
	if got, want := iotbridge.CommandTopic(
		tenant,
		iotbridge.DeviceID("device-online-rto"),
	), "t/820cc6b6-9ea8-5d18-8b10-5c8c1d126814/d/71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b/cmd"; got != want {
		t.Errorf("topic %q, want %q", got, want)
	}
}

// TestCommandMessageRefusesBadActions: an action that is not JSON is refused with the command named.
func TestCommandMessageRefusesBadActions(t *testing.T) {
	waiting := iotbridge.Waiting{
		ID:        uuid.New(),
		Action:    json.RawMessage(`{"kind":`),
		DeviceID:  uuid.UUID{},
		UnitID:    uuid.UUID{},
		ExpiresAt: time.Time{},
	}

	_, err := iotbridge.CommandMessage(waiting, 0)
	if err == nil {
		t.Error("an action that is not JSON is accepted")
	}
}

// TestCommandExpiryOnTheDeviceClock: the expiry the module sees is the API's expiry moved onto real time, so a scenario
// clock 29 days behind real time gives an expiry 29 days later on the module's clock.
func TestCommandExpiryOnTheDeviceClock(t *testing.T) {
	scenario := time.Date(2026, 9, 14, 22, 0, 30, 0, time.UTC)
	offset := -29 * 24 * time.Hour // scenario minus real
	waiting := iotbridge.Waiting{
		ID:        uuid.New(),
		Action:    json.RawMessage(`{"kind":"set_temperature","celsius":26}`),
		ExpiresAt: scenario, DeviceID: uuid.UUID{}, UnitID: uuid.UUID{},
	}

	cmd, err := iotbridge.CommandMessage(waiting, offset)
	if err != nil {
		t.Fatal(err)
	}

	if !cmd.ExpiresAt.Equal(scenario.Add(29 * 24 * time.Hour)) {
		t.Errorf("device expiry %v, want %v", cmd.ExpiresAt, scenario.Add(29*24*time.Hour))
	}
}

// TestStateTriggerFromAHeartbeat: a heartbeat with a state becomes the device's state report at the scenario time; one
// without a state becomes a report with no settings, which the API takes as heard only (IR528, IR613); a bad device ID
// is an error.
func TestStateTriggerFromAHeartbeat(t *testing.T) {
	on, c := true, 24.0
	dev := iotbridge.DeviceID("device-mock-cool-on")
	when := time.Date(2026, 9, 15, 1, 30, 0, 0, time.UTC)

	trigger, err := iotbridge.StateTrigger(
		devicesim.Heartbeat{
			DeviceID: dev.String(),
			State: &devicesim.ReportedState{
				Power:    &on,
				Celsius:  &c,
				Mode:     nil,
				FanLevel: nil,
			},
			At:       time.Time{},
			Firmware: "",
		},
		when,
	)
	if err != nil || trigger.EventType != eventState || trigger.DeviceID == nil ||
		*trigger.DeviceID != dev ||
		trigger.State == nil ||
		*trigger.State.Celsius != 24 ||
		!trigger.OccurredAt.Equal(when) {
		t.Fatalf("state trigger %+v err=%v", trigger, err)
	}

	trigger, err = iotbridge.StateTrigger(
		devicesim.Heartbeat{DeviceID: dev.String(), At: time.Time{}, Firmware: "", State: nil},
		when,
	)
	if err != nil || trigger.EventType != eventState || trigger.State == nil ||
		*trigger.State != (devicesim.ReportedState{Power: nil, Celsius: nil, Mode: nil, FanLevel: nil}) ||
		!trigger.OccurredAt.Equal(when) {
		t.Errorf("a heartbeat without a state: %+v err=%v", trigger, err)
	}

	_, stateTriggerErr := iotbridge.StateTrigger(
		devicesim.Heartbeat{
			DeviceID: "nope",
			State:    &devicesim.ReportedState{Power: nil, Celsius: nil, Mode: nil, FanLevel: nil},
			At:       time.Time{},
			Firmware: "",
		},
		when,
	)
	if stateTriggerErr == nil {
		t.Error("a bad device ID is accepted")
	}
}
