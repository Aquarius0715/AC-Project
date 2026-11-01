package devices

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// QueryConnectionFault reads a device's connection axis on its current binding: the open fault, if any, and the
// newest sequence. A demo.trigger restored must name the newest fault by its DeviceEvent ID (SR20), which a gateway
// cannot know from what it sent, so the IoT gateway (cmd/iotbridge) reads it here with the internal service token to
// take a live device offline and back online, also after its own restart (IR532).
const QueryConnectionFault = "devices.connectionFault"

// ConnectionFaultInput is QueryConnectionFault input.
type ConnectionFaultInput struct {
	DeviceID uuid.UUID `json:"deviceId"`
}

// Validate implements ops.Validator.
func (in *ConnectionFaultInput) Validate() map[string]string {
	if in.DeviceID == uuid.Nil {
		return map[string]string{"deviceId": "error.required"}
	}
	return nil
}

// ConnectionFault is the connection axis of the device's current binding.
type ConnectionFault struct {
	BindingID *uuid.UUID `json:"bindingId"` // null while the device is unbound
	// FaultID is the DeviceEvent ID of the newest fault on the axis while it is not restored (a restored names it as
	// recovery.sourceEventId); null when the connection has no open fault.
	FaultID  *uuid.UUID `json:"faultId"`
	Sequence int64      `json:"sequence"` // the newest sequence on the axis (0 when none): a new event needs a greater one
}

// RegisterQueries binds the Devices internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainEquipment, QueryConnectionFault, connectionFault)
}

func connectionFault(ctx context.Context, c *ops.Call, in *ConnectionFaultInput) (ConnectionFault, error) {
	var out ConnectionFault
	err := c.Tx.QueryRow(ctx, `SELECT binding_id FROM devices.devices WHERE id = $1`, in.DeviceID).Scan(&out.BindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return out, err
	}
	if err := c.Tx.QueryRow(ctx, `SELECT COALESCE(max(sequence), 0) FROM devices.device_events WHERE device_id = $1 AND binding_id IS NOT DISTINCT FROM $2
		AND axis = 'connection'`, in.DeviceID, out.BindingID).Scan(&out.Sequence); err != nil {
		return out, err
	}
	// the newest fault of the axis, as RecordSignal's restored looks it up
	var id uuid.UUID
	var restored *time.Time
	err = c.Tx.QueryRow(ctx, `SELECT id, restored_at FROM devices.device_events WHERE device_id = $1 AND binding_id IS NOT DISTINCT FROM $2
		AND axis = 'connection' AND event_type <> 'restored' ORDER BY sequence DESC LIMIT 1`, in.DeviceID, out.BindingID).Scan(&id, &restored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return out, err
	case restored == nil:
		out.FaultID = &id
	}
	return out, nil
}
