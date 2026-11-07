package devices

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// opSetMqttNode is the operation that sets or clears a device's MQTT node_id.
const opSetMqttNode = "devices.setMqttNode"

// eventMqttNodeChanged is the event of a changed node_id.
const eventMqttNodeChanged = "DeviceMqttNodeChanged"

// SetMqttNodeInput is devices.setMqttNode input: the node_id the device's ESP32 sends as the <node> of
// smarthome/ac/<node>/… (null or empty clears it).
type SetMqttNodeInput struct {
	DeviceID uuid.UUID `json:"deviceId"`
	MqttNode *string   `json:"mqttNode"`
}

// Validate implements ops.Validator.
func (in *SetMqttNodeInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	node, ok := normalizeMqttNode(in.MqttNode)
	if !ok {
		fieldErrs["mqttNode"] = errMqttNodeFormat
	}

	in.MqttNode = node

	return fieldErrs
}

// setMqttNode serves devices.setMqttNode.
//
//	@Summary		devices.setMqttNode (write)
//	@ID				devices.setMqttNode
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; node_id 1–64 letters, digits, hyphens or underscores; \
//	@Description	one device per node_id in the tenant; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string				true	"input field deviceId"
//	@Param			request				body		SetMqttNodeInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Device}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/set-mqtt-node [post]
func (m *Module) setMqttNode(
	ctx context.Context,
	call *ops.Call,
	input *SetMqttNodeInput,
) (Device, error) {
	state, err := lockDevice(ctx, call, input.DeviceID)
	if err != nil {
		return Device{}, err
	}

	err = state.current(call)
	if err != nil {
		return Device{}, err
	}

	err = mqttNodeFree(ctx, call, input.MqttNode, input.DeviceID)
	if err != nil {
		return Device{}, err
	}

	var before *string

	err = call.Tx.QueryRow(ctx, `SELECT mqtt_node FROM devices.devices WHERE id = $1`, input.DeviceID).
		Scan(&before)
	if err != nil {
		return Device{}, apperr.Wrap(err, "select devices.devices")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE devices.devices SET mqtt_node = $2 WHERE id = $1`,
		input.DeviceID,
		input.MqttNode,
	)
	if err != nil {
		return Device{}, apperr.Wrap(err, "update devices.devices")
	}

	version, err := bumpDevice(ctx, call, input.DeviceID)
	if err != nil {
		return Device{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   input.DeviceID,
			Type:          eventMqttNodeChanged,
			Payload:       map[string]any{"mqttNode": input.MqttNode, "previousMqttNode": before},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          opSetMqttNode,
			TargetKind:      entityDevice,
			TargetID:        input.DeviceID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version,
			Reason:          "",
			Result:          "",
			Before:          nodeFields(before),
			After:           nodeFields(input.MqttNode),
		},
	)

	detail, err := m.detail(ctx, call, "d.id = $1", []any{input.DeviceID})

	return detail.Device, err
}

// nodeFields is a node_id as the audit entry's changed field.
func nodeFields(node *string) map[string]*string {
	return map[string]*string{"mqttNode": node}
}
