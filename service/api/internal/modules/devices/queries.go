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
		return map[string]string{"deviceId": apperr.KeyRequired}
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

func connectionFault(
	ctx context.Context,
	call *ops.Call,
	input *ConnectionFaultInput,
) (ConnectionFault, error) {
	var out ConnectionFault

	err := call.Tx.QueryRow(ctx, `SELECT binding_id FROM devices.devices WHERE id = $1`, input.DeviceID).
		Scan(&out.BindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return out, apperr.Wrap(err, "select devices.devices")
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT COALESCE(max(sequence), 0) FROM devices.device_events WHERE device_id = $1 AND
		binding_id IS NOT DISTINCT FROM $2
		AND axis = 'connection'`, input.DeviceID, out.BindingID).
		Scan(&out.Sequence)
	if scanErr != nil {
		return out, apperr.Wrap(scanErr, "select devices.device_events")
	}
	// the newest fault of the axis, as RecordSignal's restored looks it up
	var (
		faultID  uuid.UUID
		restored *time.Time
	)

	err = call.Tx.QueryRow(ctx, `SELECT id, restored_at FROM devices.device_events WHERE device_id = $1 AND binding_id IS
		NOT
		DISTINCT FROM $2
		AND axis = 'connection' AND event_type <> 'restored' ORDER BY sequence DESC LIMIT 1`, input.DeviceID, out.BindingID).
		Scan(&faultID, &restored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return out, apperr.Wrap(err, "select devices.device_events")
	case restored == nil:
		out.FaultID = &faultID
	}

	return out, nil
}
