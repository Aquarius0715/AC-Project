package iotbridge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestCommandMessageFromTheStoredAction: a stored set-temperature action becomes the module's command with its setpoint and
// expiry; an action without a kind is refused; the sent report names the command.
func TestCommandMessageFromTheStoredAction(t *testing.T) {
	w := Waiting{
		ID:        uuid.MustParse("9f67089f-44c9-5231-ba20-03d3b9a1be53"),
		DeviceID:  DeviceID("device-online-rto"),
		Action:    json.RawMessage(`{"kind":"set_temperature","celsius":26}`),
		ExpiresAt: time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC),
	}
	cmd, err := CommandMessage(w)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.CommandID != w.ID.String() || cmd.Kind != "set_temperature" || cmd.Setpoint == nil || *cmd.Setpoint != 26 || !cmd.ExpiresAt.Equal(w.ExpiresAt) {
		t.Errorf("command %+v", cmd)
	}
	w.Action = json.RawMessage(`{"celsius":26}`)
	if _, err := CommandMessage(w); err == nil {
		t.Error("an action without a kind is accepted")
	}
	tr := SentTrigger(w, w.ExpiresAt)
	if tr.EventType != "command_sent" || tr.CommandID == nil || *tr.CommandID != w.ID || tr.Measurement != nil {
		t.Errorf("sent trigger %+v", tr)
	}
	if SentTrigger(w, w.ExpiresAt).EventID != tr.EventID {
		t.Error("a repeated report has another event ID")
	}
}

// TestCommandTopicOfTheDevice: the command goes down the device's own topic in its tenant.
func TestCommandTopicOfTheDevice(t *testing.T) {
	tenant := uuid.MustParse("820cc6b6-9ea8-5d18-8b10-5c8c1d126814")
	if got, want := CommandTopic(tenant, DeviceID("device-online-rto")), "t/820cc6b6-9ea8-5d18-8b10-5c8c1d126814/d/71f1fdc1-0d2b-516c-8bfa-bd8dfecd338b/cmd"; got != want {
		t.Errorf("topic %q, want %q", got, want)
	}
}

// TestCommandMessageRefusesBadActions: an action that is not JSON is refused with the command named.
func TestCommandMessageRefusesBadActions(t *testing.T) {
	w := Waiting{ID: uuid.New(), Action: json.RawMessage(`{"kind":`)}
	if _, err := CommandMessage(w); err == nil {
		t.Error("an action that is not JSON is accepted")
	}
}
