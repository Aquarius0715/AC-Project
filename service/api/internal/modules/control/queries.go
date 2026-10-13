package control

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// QueryRequestedCommands lists the commands waiting to be sent to a device (requested, not yet delivered, not expired).
// The IoT gateway (cmd/iotbridge) calls it with the internal service token and sends each one, then reports it sent
// (demo trigger command_sent), so a command moves requested → sent → acknowledged as backend-architecture §9 describes.
const QueryRequestedCommands = "control.requestedCommands"

// RequestedInput is QueryRequestedCommands input: how many commands to take at most.
type RequestedInput struct {
	Limit int `json:"limit"`
}

// Validate implements ops.Validator.
func (in *RequestedInput) Validate() map[string]string {
	if in.Limit < 1 || in.Limit > 100 {
		return map[string]string{"limit": "error.range"}
	}
	return nil
}

// Requested is one waiting command with the device it goes to.
type Requested struct {
	ID        uuid.UUID       `json:"id"`
	DeviceID  uuid.UUID       `json:"deviceId"`
	UnitID    uuid.UUID       `json:"unitId"`
	Action    json.RawMessage `json:"action"`
	ExpiresAt time.Time       `json:"expiresAt"`
}

// RegisterQueries binds control's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainEquipment, QueryRequestedCommands, requested)
}

// requested reads the waiting commands of the tenant in the order they were requested. A command whose device is not
// known is left out (it cannot be sent), and a command past its expiry is left for the expiry job.
func requested(ctx context.Context, c *ops.Call, in *RequestedInput) ([]Requested, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, device_id, unit_id, action, expires_at FROM control.commands
		WHERE status = 'requested' AND delivery = 'not_sent' AND device_id IS NOT NULL AND expires_at > $1
		ORDER BY requested_at, id LIMIT $2`, c.Now, in.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Requested{}
	for rows.Next() {
		var x Requested
		var action []byte
		if err := rows.Scan(&x.ID, &x.DeviceID, &x.UnitID, &action, &x.ExpiresAt); err != nil {
			return nil, err
		}
		x.Action = action
		out = append(out, x)
	}
	return out, rows.Err()
}
