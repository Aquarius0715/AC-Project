package iotbridge_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// liveWaiting is a requested command of the live air conditioner with the action.
func liveWaiting(action string) iotbridge.Waiting {
	return iotbridge.Waiting{
		ID:        seed.ID("cmd-live"),
		DeviceID:  seed.ID("device-live-ac"),
		UnitID:    seed.ID("unit-live-ac"),
		Action:    json.RawMessage(action),
		ExpiresAt: liveAt().Add(time.Minute),
	}
}

// TestLiveCommandJSON: a node on firmware 0.4 gets one JSON setting per action with the command's ID, which it answers
// itself; a setpoint off the firmware's whole degrees from 16 to 30, a mode or fan level it has no word for, and the
// actions it has no setting for are not sent.
func TestLiveCommandJSON(t *testing.T) {
	t.Parallel()

	id := seed.ID("cmd-live").String()

	for action, want := range map[string]string{
		`{"kind":"set_power","power":true}`:       `{"commandId":"` + id + `","power":"ON"}`,
		`{"kind":"set_power","power":false}`:      `{"commandId":"` + id + `","power":"OFF"}`,
		`{"kind":"set_temperature","celsius":24}`: `{"commandId":"` + id + `","temp":24}`,
		`{"kind":"set_temperature","celsius":16}`: `{"commandId":"` + id + `","temp":16}`,
		`{"kind":"set_temperature","celsius":30}`: `{"commandId":"` + id + `","temp":30}`,
		`{"kind":"set_mode","mode":"dry"}`:        `{"commandId":"` + id + `","mode":"dry"}`,
		`{"kind":"set_fan","fanLevel":"mid"}`:     `{"commandId":"` + id + `","fan":"mid"}`,
	} {
		command, ok, err := iotbridge.LiveCommand(liveWaiting(action), true)
		if err != nil || !ok || command.Payload != want || !command.Answers ||
			command.State != nil {
			t.Errorf("%s: %+v %t %v, want %s", action, command, ok, err, want)
		}
	}

	for _, action := range []string{
		`{"kind":"set_temperature","celsius":24.5}`, `{"kind":"set_temperature","celsius":15}`,
		`{"kind":"set_temperature","celsius":31}`, `{"kind":"set_mode","mode":"auto"}`,
		`{"kind":"set_fan","fanLevel":"auto"}`, `{"kind":"ventilate","level":"low"}`, `{"kind":"set_power"}`,
		`{"kind":"set_temperature"}`, `{"kind":"restrict"}`,
	} {
		command, ok, err := iotbridge.LiveCommand(liveWaiting(action), true)
		if err != nil || ok || command.Payload != "" {
			t.Errorf("%s: %+v %t %v, want not sent", action, command, ok, err)
		}
	}

	_, _, err := iotbridge.LiveCommand(liveWaiting(`power on`), true)
	if err == nil {
		t.Error("an action that is no JSON was taken")
	}
}

// TestLiveCommandText: a node on the first firmware gets its text: power on is COOL24 with the settings it runs, power
// off is OFF and keeps the other settings; it answers neither, and it takes no other action.
func TestLiveCommandText(t *testing.T) {
	t.Parallel()

	command, ok, err := iotbridge.LiveCommand(
		liveWaiting(`{"kind":"set_power","power":true}`),
		false,
	)

	state := command.State
	if err != nil || !ok || command.Payload != "COOL24" || command.Answers || state == nil ||
		!*state.Power ||
		*state.Celsius != 24 ||
		*state.Mode != "cool" ||
		state.FanLevel != nil {
		t.Errorf("power on: %+v %t %v", command, ok, err)
	}

	command, ok, err = iotbridge.LiveCommand(
		liveWaiting(`{"kind":"set_power","power":false}`),
		false,
	)
	if err != nil || !ok || command.Payload != "OFF" || command.Answers || command.State != nil {
		t.Errorf("power off: %+v %t %v", command, ok, err)
	}

	for _, action := range []string{
		`{"kind":"set_temperature","celsius":24}`, `{"kind":"set_mode","mode":"cool"}`,
		`{"kind":"set_fan","fanLevel":"mid"}`, `{"kind":"set_power"}`,
	} {
		command, ok, err := iotbridge.LiveCommand(liveWaiting(action), false)
		if err != nil || ok || command.Payload != "" {
			t.Errorf("%s: %+v %t %v", action, command, ok, err)
		}
	}
}

// TestLiveSentTriggers: a JSON command is only sent, as its node answers it; a text command is sent and acknowledged,
// COOL24 also reports the settings it runs with, OFF only its own action; a refused command fails.
func TestLiveSentTriggers(t *testing.T) {
	t.Parallel()

	waiting := liveWaiting(`{"kind":"set_power","power":true}`)

	answered, _, err := iotbridge.LiveCommand(waiting, true)
	if err != nil {
		t.Fatal(err)
	}

	if sent := iotbridge.LiveSentTriggers(
		waiting,
		waiting.DeviceID,
		liveAt(),
		liveAt(),
		answered,
	); len(
		sent,
	) != 1 ||
		sent[0].EventType != eventCommandSent ||
		*sent[0].CommandID != waiting.ID {
		t.Errorf("a JSON command reports %+v, want sent only", sent)
	}

	text, _, err := iotbridge.LiveCommand(waiting, false)
	if err != nil {
		t.Fatal(err)
	}

	powerOn := iotbridge.LiveSentTriggers(
		waiting,
		waiting.DeviceID,
		liveAt(),
		liveAt().Add(-2*time.Second),
		text,
	)
	if len(powerOn) != 3 || powerOn[0].EventType != eventCommandSent ||
		powerOn[1].EventType != eventCommandAck ||
		powerOn[2].EventType != eventState {
		t.Fatalf("power on triggers %+v", powerOn)
	}

	if *powerOn[1].CommandID != waiting.ID || powerOn[1].EventID == powerOn[0].EventID ||
		*powerOn[2].DeviceID != seed.ID("device-live-ac") ||
		*powerOn[2].State.Celsius != 24 ||
		!powerOn[2].OccurredAt.Equal(liveAt().Add(-2*time.Second)) {
		t.Errorf("ack %+v, state %+v", powerOn[1], powerOn[2])
	}

	off, _, err := iotbridge.LiveCommand(liveWaiting(`{"kind":"set_power","power":false}`), false)
	if err != nil {
		t.Fatal(err)
	}

	if got := iotbridge.LiveSentTriggers(
		waiting,
		waiting.DeviceID,
		liveAt(),
		liveAt(),
		off,
	); len(
		got,
	) != 2 {
		t.Errorf("power off reports %d triggers, want sent and acknowledged", len(got))
	}

	if refused := iotbridge.LiveRefusedTrigger(
		waiting,
		waiting.DeviceID,
		liveAt(),
	); refused.EventType != eventCommandFail || *refused.CommandID != waiting.ID ||
		!refused.OccurredAt.Equal(liveAt()) {
		t.Errorf("refusal %+v", refused)
	}
}

// TestLiveResultTrigger: a node's sent is the command's acknowledgement, invalid and unsupported its failure, one event
// per command and outcome; an answer that names no command, or a command ID the bridge did not write, or another
// result, is not passed on, and an answer that is no JSON is an error.
func TestLiveResultTrigger(t *testing.T) {
	t.Parallel()

	dev, cmd := seed.ID("device-live-ac"), seed.ID("cmd-live")
	answer := func(result string) []byte {
		return []byte(`{"commandId":"` + cmd.String() + `","command":"{}","result":"` + result +
			`","at":"2026-11-10T17:35:26+07:00"}`)
	}

	ack, ok, err := iotbridge.LiveResultTrigger(dev, answer("sent"), liveAt())
	if err != nil || !ok || ack.EventType != eventCommandAck || *ack.CommandID != cmd ||
		!ack.OccurredAt.Equal(liveAt()) {
		t.Errorf("sent: %+v %t %v", ack, ok, err)
	}

	for _, result := range []string{"invalid", "unsupported"} {
		fail, ok, err := iotbridge.LiveResultTrigger(dev, answer(result), liveAt())
		if err != nil || !ok || fail.EventType != eventCommandFail || *fail.CommandID != cmd ||
			fail.EventID == ack.EventID {
			t.Errorf("%s: %+v %t %v", result, fail, ok, err)
		}
	}

	if again, _, _ := iotbridge.LiveResultTrigger(
		dev,
		answer("sent"),
		liveAt().Add(time.Second),
	); again.EventID !=
		ack.EventID {
		t.Error("a repeated answer is another event")
	}

	for _, payload := range []string{
		`{"commandId":null,"command":"ON","result":"sent"}`,
		`{"command":"ON","result":"sent"}`,
		`{"commandId":"my-test","command":"{}","result":"sent"}`,
		`{"commandId":"` + cmd.String() + `","result":"applied"}`,
	} {
		trigger, ok, err := iotbridge.LiveResultTrigger(dev, []byte(payload), liveAt())
		if err != nil || ok {
			t.Errorf("%s: %+v %t %v, want not passed on", payload, trigger, ok, err)
		}
	}

	_, _, err = iotbridge.LiveResultTrigger(dev, []byte(`sent`), liveAt())
	if err == nil {
		t.Error("an answer that is no JSON was taken")
	}
}

// TestLiveStateTrigger: a node's state is its device's state report with the platform's settings; auto mode and auto
// fan are unknown, as are the numbers a board on firmware 0.4.0 sent at start and a setpoint off the firmware's range;
// a state at another time is another event, and a state that is no JSON is an error.
func TestLiveStateTrigger(t *testing.T) {
	t.Parallel()

	dev := airConditioner()
	ptr := func(s string) *string { return &s }
	powerOn, powerOff, celsius := true, false, 26.0

	for payload, want := range map[string]devicesim.ReportedState{
		`{"power":"ON","temp":26,"mode":"dry","fan":"high"}`: {
			Power: &powerOn, Celsius: &celsius, Mode: ptr("dry"), FanLevel: ptr("high"),
		},
		`{"power":"OFF","temp":26,"mode":"auto","fan":"auto"}`: {
			Power: &powerOff, Celsius: &celsius, Mode: nil, FanLevel: nil,
		},
		`{"power":"ON","temp":26,"mode":0,"fan":10}`: {
			Power: &powerOn, Celsius: &celsius, Mode: nil, FanLevel: nil,
		},
		`{"power":"maybe","temp":45,"mode":"heat"}`: {Power: nil, Celsius: nil, Mode: nil, FanLevel: nil},
	} {
		trigger, err := iotbridge.LiveStateTrigger(dev, []byte(payload), liveAt())
		if err != nil || trigger.EventType != eventState || trigger.DeviceID == nil ||
			*trigger.DeviceID != dev.DeviceID ||
			!trigger.OccurredAt.Equal(liveAt()) ||
			trigger.State == nil {
			t.Fatalf("%s: %+v %v", payload, trigger, err)
		}

		if got := *trigger.State; !sameSetting(got.Power, want.Power) ||
			!sameSetting(got.Celsius, want.Celsius) ||
			!sameSetting(got.Mode, want.Mode) ||
			!sameSetting(got.FanLevel, want.FanLevel) {
			t.Errorf("%s: state %s, want %s", payload, stateText(got), stateText(want))
		}
	}

	first, _ := iotbridge.LiveStateTrigger(dev, []byte(`{"power":"ON"}`), liveAt())
	second, _ := iotbridge.LiveStateTrigger(
		dev,
		[]byte(`{"power":"ON"}`),
		liveAt().Add(time.Millisecond),
	)

	if first.EventID == second.EventID {
		t.Error("the same settings reported again share their event")
	}

	_, err := iotbridge.LiveStateTrigger(dev, []byte(`ON`), liveAt())
	if err == nil {
		t.Error("a state that is no JSON was taken")
	}
}

// sameSetting reports whether two settings are both unknown or both the same value.
func sameSetting[T comparable](a, b *T) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// stateText is a state for a test's message.
func stateText(s devicesim.ReportedState) string {
	raw, err := json.Marshal(s)
	if err != nil {
		return err.Error()
	}

	return string(raw)
}

// TestLiveOutstanding: an answer counts once, for a command the bridge sent to that node, until 10 minutes after the
// command expired (the API decides a late answer); the zero value waits for nothing.
func TestLiveOutstanding(t *testing.T) {
	t.Parallel()

	var waiting iotbridge.LiveOutstanding

	cmd, other := seed.ID("cmd-live"), uuid.New()
	if waiting.Take(cmd, node1, liveAt()) {
		t.Fatal("the zero value waits for a command")
	}

	waiting.Add(cmd, node1, liveAt())
	waiting.Add(other, node1, liveAt().Add(-time.Hour))

	if waiting.Take(cmd, node2, liveAt()) {
		t.Error("another node's answer counted")
	}

	if !waiting.Take(cmd, node1, liveAt().Add(5*time.Minute)) {
		t.Error("an answer 5 minutes after the expiry was dropped")
	}

	if waiting.Take(cmd, node1, liveAt()) {
		t.Error("an answer counted twice")
	}

	if waiting.Take(other, node1, liveAt()) {
		t.Error("an answer long after the expiry counted")
	}

	waiting.Add(cmd, node1, liveAt())

	if waiting.Take(cmd, node1, liveAt().Add(11*time.Minute)) {
		t.Error("an answer 11 minutes after the expiry counted")
	}
}

// TestLiveFirmware: a node answers its commands once it has sent a status with its firmware version; a last will, a
// status without a version, or another node's status changes nothing; a status that is no JSON is an error.
func TestLiveFirmware(t *testing.T) {
	t.Parallel()

	var firmware iotbridge.LiveFirmware

	if firmware.Answers(node1) {
		t.Fatal("the zero value knows a node")
	}

	for _, payload := range []string{`{"online":false}`, `{"online":true}`} {
		status, err := iotbridge.ParseLiveStatus([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}

		firmware.Noted(node1, status)
	}

	if firmware.Answers(node1) {
		t.Error("a status without a firmware version marked the node")
	}

	status, err := iotbridge.ParseLiveStatus(
		[]byte(
			`{"online":true,"fw":"0.4.0","rssi":-20,"uptime_s":7640,"parts":{"ir":true,"pm25":false}}`,
		),
	)
	if err != nil || !status.Online || status.Firmware != "0.4.0" {
		t.Fatalf("status %+v %v", status, err)
	}

	firmware.Noted(node2, status)

	if firmware.Answers(node1) || !firmware.Answers(node2) {
		t.Error("the version marked the wrong node")
	}

	gone, _ := iotbridge.ParseLiveStatus([]byte(`{"online":false}`))
	firmware.Noted(node2, gone)

	if !firmware.Answers(node2) {
		t.Error("the last will forgot the node's firmware")
	}

	_, err = iotbridge.ParseLiveStatus([]byte(`online`))
	if err == nil {
		t.Error("a status that is no JSON was taken")
	}
}
