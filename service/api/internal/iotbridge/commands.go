package iotbridge

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

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
func CommandMessage(w Waiting, offset time.Duration) (devicesim.Command, error) {
	var a struct {
		Kind    string   `json:"kind"`
		Celsius *float64 `json:"celsius"`
	}
	if err := json.Unmarshal(w.Action, &a); err != nil {
		return devicesim.Command{}, fmt.Errorf("command %s action: %w", w.ID, err)
	}
	if a.Kind == "" {
		return devicesim.Command{}, fmt.Errorf("command %s has no action kind", w.ID)
	}
	return devicesim.Command{CommandID: w.ID.String(), Kind: a.Kind, Setpoint: a.Celsius, ExpiresAt: w.ExpiresAt.Add(-offset).UTC()}, nil
}

// SentTrigger reports a command sent to its device: the demo trigger command_sent moves it requested → sent.
func SentTrigger(w Waiting, at time.Time) Trigger {
	cmd := w.ID
	return Trigger{
		ScenarioID: Scenario,
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/sent", cmd))),
		OccurredAt: at.UTC(),
		EventType:  "command_sent",
		CommandID:  &cmd,
	}
}

// CommandTopic is the downward topic of a device's commands for a tenant (backend-architecture §9).
func CommandTopic(tenant, device uuid.UUID) string {
	return devicesim.Topic(tenant.String(), device.String(), "cmd")
}

// StateTrigger turns a heartbeat into the device's state report (DemoTrigger state) at the given scenario time; a
// heartbeat without a state reports nothing.
func StateTrigger(hb devicesim.Heartbeat, at time.Time) (Trigger, bool, error) {
	if hb.State == nil {
		return Trigger{}, false, nil
	}
	dev, err := uuid.Parse(hb.DeviceID)
	if err != nil {
		return Trigger{}, false, fmt.Errorf("device %q: %w", hb.DeviceID, err)
	}
	at = at.UTC()
	return Trigger{
		ScenarioID: Scenario,
		EventID:    uuid.NewSHA1(seed.Namespace, []byte(fmt.Sprintf("%s/state/%s", dev, at.Format(time.RFC3339Nano)))),
		OccurredAt: at,
		EventType:  "state",
		DeviceID:   &dev,
		State:      hb.State,
	}, true, nil
}
