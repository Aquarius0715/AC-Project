package iotbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// errNoActionKind refuses a stored command whose action names no kind.
var errNoActionKind = errors.New("has no action kind")

// Waiting is one command the API lists as requested (control.requestedCommands): not yet sent to its device.
type Waiting struct {
	ID        uuid.UUID       `json:"id"`
	DeviceID  uuid.UUID       `json:"deviceId"`
	UnitID    uuid.UUID       `json:"unitId"`
	Action    json.RawMessage `json:"action"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

// CommandMessage is the message the module receives on its command topic. The action is the unit action the API
// stored (for example {"kind":"set_temperature","celsius":26}); the module applies a set-temperature change. The API
// keeps the expiry on its scenario clock and the module runs on real time, so the expiry is moved by the clock offset
// (scenario minus real): the module then decides "in time" and "late" on its own clock.
func CommandMessage(waiting Waiting, offset time.Duration) (devicesim.Command, error) {
	var a struct {
		Kind    string   `json:"kind"`
		Celsius *float64 `json:"celsius"`
	}

	err := json.Unmarshal(waiting.Action, &a)
	if err != nil {
		return devicesim.Command{}, fmt.Errorf("command %s action: %w", waiting.ID, err)
	}

	if a.Kind == "" {
		return devicesim.Command{}, fmt.Errorf("command %s %w", waiting.ID, errNoActionKind)
	}

	return devicesim.Command{
		CommandID: waiting.ID.String(),
		Kind:      a.Kind,
		Setpoint:  a.Celsius,
		ExpiresAt: waiting.ExpiresAt.Add(-offset).UTC(),
	}, nil
}

// SentTrigger reports a command sent to its device: the demo trigger command_sent moves it requested → sent.
func SentTrigger(w Waiting, when time.Time) Trigger {
	cmd := w.ID

	return Trigger{
		ScenarioID:  Scenario,
		EventID:     uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/sent", cmd))),
		OccurredAt:  when.UTC(),
		EventType:   "command_sent",
		CommandID:   &cmd,
		Measurement: nil,
		DeviceID:    nil,
		State:       nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}
}

// CommandTopic is the downward topic of a device's commands for a tenant (backend-architecture §9).
func CommandTopic(tenant, device uuid.UUID) string {
	return devicesim.Topic(tenant.String(), device.String(), "cmd")
}

// StateTrigger turns a heartbeat into the device's state report (DemoTrigger state) at the given scenario time. A
// heartbeat without a state, as a module that reads no air-conditioner settings sends it, is a report with no settings,
// which the API takes as heard only (IR528): the device's last seen time follows its heartbeats, as the live module's
// does (IR613).
func StateTrigger(hb devicesim.Heartbeat, when time.Time) (Trigger, error) {
	dev, err := uuid.Parse(hb.DeviceID)
	if err != nil {
		return Trigger{}, fmt.Errorf("device %q: %w", hb.DeviceID, err)
	}

	state := hb.State
	if state == nil {
		state = &devicesim.ReportedState{Power: nil, Celsius: nil, Mode: nil, FanLevel: nil}
	}

	when = when.UTC()

	return Trigger{
		ScenarioID: Scenario,
		EventID: uuid.NewSHA1(
			seed.Namespace,
			[]byte(fmt.Sprintf("%s/state/%s", dev, when.Format(time.RFC3339Nano))),
		),
		OccurredAt:  when,
		EventType:   "state",
		DeviceID:    &dev,
		State:       state,
		CommandID:   nil,
		Measurement: nil,
		Event:       nil,
		BindingID:   nil,
		Sequence:    nil,
		Kind:        "",
		Recovery:    nil,
	}, nil
}
