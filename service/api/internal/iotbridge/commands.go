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
// stored (for example {"kind":"set_temperature","celsius":26}); the module applies a set-temperature change.
func CommandMessage(w Waiting) (devicesim.Command, error) {
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
	return devicesim.Command{CommandID: w.ID.String(), Kind: a.Kind, Setpoint: a.Celsius, ExpiresAt: w.ExpiresAt.UTC()}, nil
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
