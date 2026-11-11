package devices

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Exclusion reports work on a unit owned by other modules (Control commands / runs, active restrictions).
type Exclusion interface {
	UnitBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// operationTTL is the DeviceOperation lifetime (D05: failed/TIMEOUT 60 s after creation).
const operationTTL = 60 * time.Second

// DeviceOperation kinds and the statuses the module sets or reads.
const (
	kindCheck    = "check"
	kindFirmware = "firmware"

	statusRunning   = "running"
	statusSucceeded = "succeeded"
	statusFailed    = "failed"
)

// opBind is the devices.bind operation, also the audit action of every binding change, which emits DeviceBound.
const (
	opBind           = "devices.bind"
	eventDeviceBound = "DeviceBound"
)

type devState struct {
	unit       *uuid.UUID
	target     uuid.UUID
	creator    uuid.UUID
	version    int
	connection string
	tamper     string
	firmware   string
	iotModel   *uuid.UUID // devices.iot_models.id the device was registered with (DEC-105)
	iotVersion *int
}

// lockDevice loads the device for update (NOT_FOUND). The handler checks the caller's scope on the device's unit first
// and the expected version after it (current), so a caller outside the scope gets NOT_FOUND / FORBIDDEN for any
// version and learns nothing of the device (IR366).
func lockDevice(ctx context.Context, c *ops.Call, id uuid.UUID) (devState, error) {
	var state devState

	err := c.Tx.QueryRow(ctx, `SELECT unit_id, target_unit_id, created_by_membership_id, version, connection, tamper,
		firmware_version, iot_model_id, iot_model_version
		FROM devices.devices WHERE id = $1 FOR UPDATE`, id).
		Scan(&state.unit, &state.target, &state.creator, &state.version, &state.connection, &state.tamper, &state.firmware,
			&state.iotModel, &state.iotVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return state, apperr.Wrap(err, "select devices.devices")
	}

	return state, nil
}

// current checks the expected version of a locked device (CONFLICT).
func (s devState) current(c *ops.Call) error {
	if c.ExpectedVersion != nil && s.version != *c.ExpectedVersion {
		return apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return nil
}

// exclusive applies D05 on a unit: no queued/running DeviceOperation for its device and no in-flight control work.
func (m *Module) exclusive(ctx context.Context, call *ops.Call, unit uuid.UUID) error {
	var busy bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND status IN
		('queued','running') AND expires_at > $2)`,
		unit, call.Now).
		Scan(&busy)
	if err != nil {
		return apperr.Wrap(err, "select devices.device_operations")
	}

	for _, x := range m.Exclusion {
		if busy {
			break
		}

		b, err := x.UnitBusy(ctx, call, unit)
		if err != nil {
			return apperr.Wrap(err, "devices.Exclusion.UnitBusy")
		}

		busy = b
	}

	if busy {
		return apperr.E(apperr.Conflict, "error.unitBusy")
	}

	return nil
}

func bumpDevice(ctx context.Context, c *ops.Call, id uuid.UUID) (int, error) {
	var version int

	err := c.Tx.QueryRow(ctx, `UPDATE devices.devices SET version = version + 1, updated_at = platform.app_now() WHERE id =
		$1 RETURNING version`, id).
		Scan(&version)

	return version, apperr.Wrap(err, "update devices.devices")
}

// ---- bind ----

// BindInput is devices.bind input.
type BindInput struct {
	DeviceID uuid.UUID  `json:"deviceId"`
	UnitID   uuid.UUID  `json:"unitId"`
	JobID    *uuid.UUID `json:"jobId,omitempty"`
	Reason   string     `json:"reason"`
}

// Validate implements ops.Validator.
func (in *BindInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	if in.UnitID == uuid.Nil {
		fieldErrs["unitId"] = apperr.KeyRequired
	}

	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// bind attaches the device to a unit (D05): both units in scope, reason, exclusion on both units, no unresolved
// tamper, at most one active device per unit; the old binding keeps its end time and the new binding gets new sensors.
//
//	@Summary		devices.bind (write)
//	@ID				devices.bind
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T11, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string		true	"input field deviceId"
//	@Param			request				body		BindInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Device}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/bind [post]
func (m *Module) bind(ctx context.Context, c *ops.Call, in *BindInput) (Device, error) {
	d, err := m.bindDetail(ctx, c, in)

	return d.Device, err
}

// bindDetail is bind answering the device with its detail (IR392: the write itself answers Device).
func (m *Module) bindDetail(
	ctx context.Context,
	call *ops.Call,
	input *BindInput,
) (DeviceDetail, error) {
	err := m.technicianGate(ctx, call, input.JobID, input.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}

	state, err := lockDevice(ctx, call, input.DeviceID)
	if err != nil {
		return DeviceDetail{}, err
	}

	err = m.rebindable(ctx, call, input.UnitID, state)
	if err != nil {
		return DeviceDetail{}, err
	}

	unitInfo, err := m.existingUnit(ctx, call, input.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}

	err = m.unitsFree(ctx, call, input.UnitID, state.unit)
	if err != nil {
		return DeviceDetail{}, err
	}

	if state.unit != nil {
		err := releaseBinding(ctx, call, input.DeviceID, input.Reason)
		if err != nil {
			return DeviceDetail{}, err
		}
	}

	version, err := insertBinding(
		ctx,
		call,
		input.DeviceID,
		input.UnitID,
		unitInfo.OrgID,
		input.Reason,
	)
	if err != nil {
		return DeviceDetail{}, err
	}

	recordBinding(
		call,
		input.DeviceID,
		input.UnitID,
		state.unit,
		call.ExpectedVersion,
		version,
		input.Reason,
	)

	return m.detail(ctx, call, "d.id = $1", []any{input.DeviceID})
}

// ---- unbind / delete (DEC-103) ----

// UnbindInput is devices.unbind input.
type UnbindInput struct {
	DeviceID uuid.UUID `json:"deviceId"`
	Reason   string    `json:"reason"`
}

// DeleteDeviceInput is devices.delete input.
type DeleteDeviceInput struct {
	DeviceID uuid.UUID `json:"deviceId"`
	Reason   string    `json:"reason"`
}

// DeletedResource is DeletedResource of service-contracts.ts.
type DeletedResource struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// reasonErrors checks the reason of a write: 1–1000 characters after trimming.
func reasonErrors(reason *string, fieldErrs map[string]string) {
	*reason = strings.TrimSpace(*reason)
	if n := utf8.RuneCountInString(*reason); n < 1 || n > 1000 {
		fieldErrs["reason"] = apperr.KeyLength
	}
}

// Validate implements ops.Validator.
func (in *UnbindInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	reasonErrors(&in.Reason, fieldErrs)

	return fieldErrs
}

// Validate implements ops.Validator.
func (in *DeleteDeviceInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	reasonErrors(&in.Reason, fieldErrs)

	return fieldErrs
}

// unbind takes a bound device off its unit (DEC-103): HQ only, a unit with work under way is busy as for a bind, the
// binding row keeps its history and the sensors get new IDs, as when a device moves. The device stays registered, free
// to bind again or to delete.
//
//	@Summary		devices.unbind (write)
//	@ID				devices.unbind
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR652 ends the device's binding (its history stays); CONFLICT error.notBound when it is not bound; a \
//	@Description	unit with work under way is busy
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string		true	"input field deviceId"
//	@Param			request				body		UnbindInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Device}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/unbind [post]
func (m *Module) unbind(ctx context.Context, call *ops.Call, input *UnbindInput) (Device, error) {
	state, err := lockDevice(ctx, call, input.DeviceID)
	if err != nil {
		return Device{}, err
	}

	currentErr := state.current(call)
	if currentErr != nil {
		return Device{}, currentErr
	}

	if state.unit == nil {
		return Device{}, apperr.E(apperr.Conflict, "error.notBound")
	}

	exclusiveErr := m.exclusive(ctx, call, *state.unit)
	if exclusiveErr != nil {
		return Device{}, exclusiveErr
	}

	releaseErr := releaseBinding(ctx, call, input.DeviceID, input.Reason)
	if releaseErr != nil {
		return Device{}, releaseErr
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = NULL, binding_id = NULL, version = version + 1,
		updated_at = platform.app_now() WHERE id = $1 RETURNING version`, input.DeviceID).
		Scan(&version)
	if scanErr != nil {
		return Device{}, apperr.Wrap(scanErr, "update devices.devices")
	}

	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   input.DeviceID,
			Type:          "DeviceUnbound",
			Payload:       map[string]any{fieldPreviousUnitID: *state.unit},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "devices.unbind",
			TargetKind:      entityDevice,
			TargetID:        input.DeviceID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	detail, err := m.detail(ctx, call, "d.id = $1", []any{input.DeviceID})

	return detail.Device, err
}

// deleteDevice deletes a registered device that is in no use (DEC-103): not bound to a unit, no operation under way and
// in no unfinished firmware campaign; otherwise CONFLICT with the reason. The device's own records go with it: its
// sensors, bindings, operations, events and notes, calibrations and its rows in firmware campaigns. What other
// modules keep (commands, readings, alerts) holds the ID only.
//
//	@Summary		devices.delete (write)
//	@ID				devices.delete
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR652 only a device that is not bound, has no operation under way and is in no unfinished firmware \
//	@Description	campaign (CONFLICT error.deviceBound / error.deviceBusy / error.deviceInCampaign); deletes the \
//	@Description	device with its own sensors, bindings, operations, events, calibrations and campaign rows
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string	true	"input field deviceId"
//	@Param			reason				query		string	false	"input field reason"
//	@Success		200					{object}	ops.Envelope{data=DeletedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId} [delete]
func (*Module) deleteDevice(
	ctx context.Context,
	call *ops.Call,
	input *DeleteDeviceInput,
) (DeletedResource, error) {
	state, err := lockDevice(ctx, call, input.DeviceID)
	if err != nil {
		return DeletedResource{}, err
	}

	currentErr := state.current(call)
	if currentErr != nil {
		return DeletedResource{}, currentErr
	}

	useErr := deviceUse(ctx, call, input.DeviceID, state)
	if useErr != nil {
		return DeletedResource{}, useErr
	}

	for _, stmt := range []string{
		`DELETE FROM devices.firmware_campaign_devices WHERE device_id = $1`,
		`DELETE FROM devices.calibration_records WHERE device_id = $1`,
		`DELETE FROM devices.device_event_notes WHERE event_id IN
			(SELECT id FROM devices.device_events WHERE device_id = $1)`,
		`DELETE FROM devices.device_events WHERE device_id = $1`,
		`DELETE FROM devices.device_operations WHERE device_id = $1`,
		`DELETE FROM devices.sensors WHERE device_id = $1`,
		`DELETE FROM devices.device_bindings WHERE device_id = $1`,
		`DELETE FROM devices.devices WHERE id = $1`,
	} {
		_, execErr := call.Tx.Exec(ctx, stmt, input.DeviceID)
		if execErr != nil {
			return DeletedResource{}, apperr.Wrap(execErr, "delete devices.devices")
		}
	}

	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   input.DeviceID,
			Type:          "DeviceDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "devices.delete",
			TargetKind:      entityDevice,
			TargetID:        input.DeviceID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.DeviceID, Deleted: true}, nil
}

// deviceUse refuses a device that is bound, has a queued or running operation, or is in a firmware campaign that has
// not ended, with the reason in that order.
func deviceUse(ctx context.Context, call *ops.Call, device uuid.UUID, state devState) error {
	if state.unit != nil {
		return apperr.E(apperr.Conflict, "error.deviceBound")
	}

	var busy, inCampaign bool

	err := call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE device_id = $1 AND status IN ('queued','running') AND
			expires_at > $2),
		EXISTS (SELECT 1 FROM devices.firmware_campaign_devices cd JOIN devices.firmware_campaigns c ON c.id = cd.campaign_id
			WHERE cd.device_id = $1 AND c.state IN ('scheduled','running','paused') AND cd.result IN ('pending','installing'))`,
		device,
		call.Now,
	).Scan(&busy, &inCampaign)
	if err != nil {
		return apperr.Wrap(err, "select devices.device_operations")
	}

	switch {
	case busy:
		return apperr.E(apperr.Conflict, "error.deviceBusy")
	case inCampaign:
		return apperr.E(apperr.Conflict, "error.deviceInCampaign")
	}

	return nil
}

// hiddenUnbound reports whether an unbound device is one the caller does not read: everyone but HQ reads only the
// unbound devices they registered (deviceScope, devices.get).
func hiddenUnbound(call *ops.Call, state devState) bool {
	return state.unit == nil && call.Principal.Role != roleAdmin &&
		state.creator != call.Principal.MembershipID
}

// rebindable checks the locked device against a bind to unit: a technician moves a bound device only from a unit of
// an assignment of theirs as well (the unit it leaves is in scope too, and DD-T11 rejects an unauthorized rebinding,
// IR366), and binds an unbound one only to its planned unit unless they registered it (IR709); then the expected
// version, a device already on the unit and an unresolved tamper.
func (m *Module) rebindable(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	state devState,
) error {
	if state.unit != nil && *state.unit != unit && call.Principal.Role == roleTechnician {
		err := m.UnitAccess.TechnicianUnit(ctx, call, *state.unit, false, false)
		if err != nil {
			return apperr.Wrap(err, "devices.TechUnitAccess.TechnicianUnit")
		}
	}
	// a device not bound yet goes to the unit HQ planned for it, or anywhere by HQ or the member who registered it: a
	// technician cannot take another member's planned device to a unit of their own (NOT_FOUND, as the device reads
	// for them, whatever its version: IR366, IR709)
	if state.unit == nil && unit != state.target && hiddenUnbound(call, state) {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	currentErr := state.current(call)
	if currentErr != nil {
		return currentErr
	}

	if state.unit != nil && *state.unit == unit {
		return apperr.E(apperr.Conflict, "error.alreadyBound")
	}

	if state.tamper == tamperDetected {
		return apperr.E(apperr.Conflict, apperr.KeyTamperUnresolved)
	}

	return nil
}

// unitsFree applies the exclusion (D05) to the unit a bind targets and the unit the device leaves, in that order, and
// refuses a target unit that already has an active device.
func (m *Module) unitsFree(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	previous *uuid.UUID,
) error {
	units := []uuid.UUID{unit}
	if previous != nil {
		units = append(units, *previous)
	}

	for _, unitID := range units {
		err := m.exclusive(ctx, call, unitID)
		if err != nil {
			return err
		}
	}

	var taken bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_bindings WHERE unit_id = $1 AND
		unbound_at
		IS NULL)`, unit).
		Scan(&taken)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select devices.device_bindings")
	}

	if taken {
		return apperr.E(apperr.Conflict, "error.unitHasDevice")
	}

	return nil
}

// releaseBinding ends the device's active binding before it moves to another unit.
func releaseBinding(ctx context.Context, call *ops.Call, device uuid.UUID, reason string) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE devices.device_bindings SET unbound_at = $2, reason = $3 WHERE device_id = $1 AND unbound_at IS NULL`,
		device,
		call.Now,
		reason,
	)
	if err != nil {
		return apperr.Wrap(err, "update devices.device_bindings")
	}
	// new binding → new sensor IDs; the old sensors are detached (device_id kept for history) by re-keying
	_, err = call.Tx.Exec(
		ctx,
		`UPDATE devices.sensors SET id = gen_random_uuid(), calibrated_at = NULL WHERE device_id = $1 AND NOT EXISTS
		(SELECT 1 FROM devices.calibration_records r WHERE r.sensor_id = devices.sensors.id)`,
		device,
	)

	return apperr.Wrap(err, "update devices.sensors")
}

// insertBinding binds the device to the unit of the customer org: a new binding row, which the device then points
// to. It returns the new device version.
func insertBinding(
	ctx context.Context,
	call *ops.Call,
	device, unit, org uuid.UUID,
	reason string,
) (int, error) {
	binding := uuid.Must(uuid.NewV7())

	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
			actor_membership_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7)`,
		binding,
		device,
		unit,
		org,
		call.Now,
		reason,
		call.Principal.MembershipID,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "insert devices.device_bindings")
	}

	var version int

	err = call.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = $2, binding_id = $3, target_unit_id = $2, version =
		version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, device, unit, binding).
		Scan(&version)
	if err != nil {
		return 0, apperr.Wrap(err, "update devices.devices")
	}

	return version, nil
}

// recordBinding emits DeviceBound and audits devices.bind for a device that moved from previousUnit to unit; an
// unbound side is nil.
func recordBinding(
	call *ops.Call,
	device uuid.UUID,
	unit, previousUnit any,
	previousVersion *int,
	version int,
	reason string,
) {
	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   device,
			Type:          eventDeviceBound,
			Payload:       map[string]any{fieldUnitID: unit, fieldPreviousUnitID: previousUnit},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          opBind,
			TargetKind:      entityDevice,
			TargetID:        device.String(),
			PreviousVersion: previousVersion,
			NextVersion:     &version,
			Reason:          reason, Result: "", Before: nil, After: nil,
		},
	)
}

// ---- check / firmware ----

// DeviceOperation is DeviceOperation of service-contracts.ts.
type DeviceOperation struct {
	ID                        uuid.UUID  `json:"id"`
	TenantID                  uuid.UUID  `json:"tenantId"`
	Version                   int        `json:"version"`
	CreatedAt                 time.Time  `json:"createdAt"`
	UpdatedAt                 time.Time  `json:"updatedAt"`
	BindingID                 *uuid.UUID `json:"bindingId"`
	UnitIDAtOccurrence        *uuid.UUID `json:"unitIdAtOccurrence"`
	CustomerOrgIDAtOccurrence *uuid.UUID `json:"customerOrgIdAtOccurrence"`
	DeviceID                  uuid.UUID  `json:"deviceId"`
	Kind                      string     `json:"kind"`
	Status                    string     `json:"status"`
	TargetVersion             *string    `json:"targetVersion"`
	FailureCode               *string    `json:"failureCode"`
	StartedAt                 *time.Time `json:"startedAt"`
	FinishedAt                *time.Time `json:"finishedAt"`
	ExpiresAt                 time.Time  `json:"expiresAt"`
}

const opCols = `o.id, o.tenant_id, o.version, o.created_at, o.updated_at, b.id, o.unit_id, b.customer_org_id, ` +
	`o.device_id, o.kind, o.status, o.target_version,
	o.failure_code, o.started_at, o.finished_at, o.expires_at`

func scanOp(r pgx.Row) (DeviceOperation, error) {
	var deviceOperation DeviceOperation

	err := r.Scan(
		&deviceOperation.ID,
		&deviceOperation.TenantID,
		&deviceOperation.Version,
		&deviceOperation.CreatedAt,
		&deviceOperation.UpdatedAt,
		&deviceOperation.BindingID,
		&deviceOperation.UnitIDAtOccurrence,
		&deviceOperation.CustomerOrgIDAtOccurrence,
		&deviceOperation.DeviceID,
		&deviceOperation.Kind,
		&deviceOperation.Status,
		&deviceOperation.TargetVersion,
		&deviceOperation.FailureCode,
		&deviceOperation.StartedAt,
		&deviceOperation.FinishedAt,
		&deviceOperation.ExpiresAt,
	)

	return deviceOperation, apperr.Wrap(err, "scan (scanOp)")
}

// lockBoundDevice locks a bound device for a write on it: NOT_FOUND, CONFLICT while it is unbound, the IR94
// technician gate on its unit, then the expected version.
func (m *Module) lockBoundDevice(
	ctx context.Context,
	call *ops.Call,
	device uuid.UUID,
	jobID *uuid.UUID,
) (devState, error) {
	state, err := lockDevice(ctx, call, device)
	if err != nil {
		return state, err
	}

	if state.unit == nil {
		if hiddenUnbound(
			call,
			state,
		) { // it reads as not found for the caller: say nothing of it (IR366, IR709)
			return state, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		return state, apperr.E(apperr.Conflict, apperr.KeyDeviceNotBound)
	}

	technicianGateErr := m.technicianGate(ctx, call, jobID, *state.unit)
	if technicianGateErr != nil {
		return state, technicianGateErr
	}

	return state, state.current(call)
}

// firmwareAllowed checks a firmware update of a locked device: the target is a candidate of its IoT model's current
// version other than the installed one, and the device is online (IR94: no DeviceOperation when not online).
func (*Module) firmwareAllowed(
	ctx context.Context,
	call *ops.Call,
	state devState,
	target string,
) error {
	var candidates []string // none for a device without an IoT model

	if state.iotModel != nil {
		scanErr := call.Tx.QueryRow(ctx, `SELECT firmware_candidates FROM devices.iot_models WHERE id = $1 AND is_current`,
			*state.iotModel).
			Scan(&candidates)
		if scanErr != nil {
			return apperr.Wrap(scanErr, "select devices.iot_models")
		}
	}

	if !slices.Contains(candidates, target) || target == state.firmware {
		return apperr.Fields(
			map[string]string{"firmwareVersion": "error.unsupportedFirmware"},
		)
	}

	if state.connection != connOnline {
		return apperr.E(
			apperr.Offline,
			"error.offline",
		) // IR94: no DeviceOperation when not online
	}

	return nil
}

func (m *Module) startOperation(
	ctx context.Context,
	call *ops.Call,
	device uuid.UUID,
	jobID *uuid.UUID,
	kind string,
	target *string,
) (DeviceOperation, error) {
	state, err := m.lockBoundDevice(ctx, call, device, jobID)
	if err != nil {
		return DeviceOperation{}, err
	}

	if kind == kindFirmware {
		err := m.firmwareAllowed(ctx, call, state, *target)
		if err != nil {
			return DeviceOperation{}, err
		}
	}

	exclusiveErr := m.exclusive(ctx, call, *state.unit)
	if exclusiveErr != nil {
		return DeviceOperation{}, exclusiveErr
	}

	operationID := uuid.Must(uuid.NewV7())

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, job_id, kind, status, target_version,
			expires_at, actor_membership_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, 'queued', $6, $7, $8)`,
		operationID,
		device,
		*state.unit,
		jobID,
		kind,
		target,
		call.Now.Add(operationTTL),
		call.Principal.MembershipID,
	)
	if execErr != nil {
		return DeviceOperation{}, apperr.Wrap(execErr, "insert devices.device_operations")
	}

	version, err := bumpDevice(ctx, call, device)
	if err != nil {
		return DeviceOperation{}, err
	}

	operationRequested(call, device, operationID, kind, version)

	return scanOp(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+opCols+` FROM devices.device_operations o LEFT JOIN devices.device_bindings b ON b.device_id = o.device_id
				AND b.unbound_at IS NULL WHERE o.id = $1`,
			operationID,
		),
	)
}

// operationRequested emits DeviceOperationRequested and audits the devices.check or devices.updateFirmware that
// requested the operation.
func operationRequested(call *ops.Call, device, operationID uuid.UUID, kind string, version int) {
	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   device,
			Type:          "DeviceOperationRequested",
			Payload:       map[string]any{fieldOperationID: operationID, fieldKind: kind},
		},
	)
	call.Audit(ops.AuditEntry{
		Action:          "devices." + map[string]string{kindCheck: "check", kindFirmware: "updateFirmware"}[kind],
		TargetKind:      entityDevice,
		TargetID:        device.String(),
		PreviousVersion: call.ExpectedVersion,
		NextVersion:     &version,
		Reason:          "",
		Result:          "",
		Before:          nil,
		After:           nil,
	})
}

// CheckInput is devices.check input.
type CheckInput struct {
	ID    uuid.UUID  `json:"id"`
	JobID *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *CheckInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// check serves devices.check.
//
//	@Summary		devices.check (write)
//	@ID				devices.check
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR67 queued→running after 1s tick; IR90 recheck failure CONFLICT; restriction commands not_sent \
//	@Description	while running; IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T11, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target device, read devices.get)"
//	@Param			id					path		string		true	"input field id"
//	@Param			request				body		CheckInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=DeviceOperation}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{id}/check [post]
func (m *Module) check(ctx context.Context, c *ops.Call, in *CheckInput) (DeviceOperation, error) {
	return m.startOperation(
		ctx,
		c,
		in.ID,
		in.JobID,
		kindCheck,
		nil,
	) // accepted regardless of connection (IR94)
}

// FirmwareInput is devices.updateFirmware input.
type FirmwareInput struct {
	DeviceID        uuid.UUID  `json:"deviceId"`
	FirmwareVersion string     `json:"firmwareVersion"`
	JobID           *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *FirmwareInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	if n := utf8.RuneCountInString(in.FirmwareVersion); n < 1 || n > 64 {
		fieldErrs["firmwareVersion"] = apperr.KeyRequired
	}

	return fieldErrs
}

// updateFirmware serves devices.updateFirmware.
//
//	@Summary		devices.updateFirmware (write)
//	@ID				devices.updateFirmware
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR67 queued→running after 1s tick; IR90 recheck failure CONFLICT; restriction commands not_sent \
//	@Description	while running; IR94 technician write table (assignment and work window, jobId required when typed); \
//	@Description	IR94 non-online at request: OFFLINE without operation
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T11, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string			true	"input field deviceId"
//	@Param			request				body		FirmwareInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=DeviceOperation}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/update-firmware [post]
func (m *Module) updateFirmware(
	ctx context.Context,
	c *ops.Call,
	in *FirmwareInput,
) (DeviceOperation, error) {
	return m.startOperation(ctx, c, in.DeviceID, in.JobID, kindFirmware, &in.FirmwareVersion)
}

// ---- calibrate ----

// CalibrationRecord is CalibrationRecord of service-contracts.ts.
type CalibrationRecord struct {
	ID                        uuid.UUID  `json:"id"`
	TenantID                  uuid.UUID  `json:"tenantId"`
	Version                   int        `json:"version"`
	CreatedAt                 time.Time  `json:"createdAt"`
	UpdatedAt                 time.Time  `json:"updatedAt"`
	BindingID                 *uuid.UUID `json:"bindingId"`
	UnitIDAtOccurrence        *uuid.UUID `json:"unitIdAtOccurrence"`
	CustomerOrgIDAtOccurrence *uuid.UUID `json:"customerOrgIdAtOccurrence"`
	DeviceID                  uuid.UUID  `json:"deviceId"`
	SensorID                  uuid.UUID  `json:"sensorId"`
	Metric                    string     `json:"metric"`
	Unit                      string     `json:"unit"`
	ReferenceValue            float64    `json:"referenceValue"`
	MeasuredValue             float64    `json:"measuredValue"`
	CalibratedAt              time.Time  `json:"calibratedAt"`
	ActorID                   uuid.UUID  `json:"actorId"`
	IsDemo                    bool       `json:"isDemo"`
}

// CalibrateInput is devices.calibrate input.
type CalibrateInput struct {
	DeviceID       uuid.UUID  `json:"deviceId"`
	SensorID       uuid.UUID  `json:"sensorId"`
	Metric         string     `json:"metric"`
	Unit           string     `json:"unit"`
	ReferenceValue float64    `json:"referenceValue"`
	MeasuredValue  float64    `json:"measuredValue"`
	CalibratedAt   time.Time  `json:"calibratedAt"`
	JobID          *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator (DD-T11).
func (in *CalibrateInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	if in.SensorID == uuid.Nil {
		fieldErrs["sensorId"] = apperr.KeyRequired
	}

	if unit := metricUnits()[in.Metric]; unit == "" || unit != in.Unit {
		fieldErrs["unit"] = "error.unitMismatch"
	}

	for k, v := range map[string]float64{"referenceValue": in.ReferenceValue, "measuredValue": in.MeasuredValue} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			fieldErrs[k] = apperr.KeyInvalid
		}
	}

	if in.CalibratedAt.IsZero() {
		fieldErrs["calibratedAt"] = apperr.KeyRequired
	}

	return fieldErrs
}

// calibrate serves devices.calibrate.
//
//	@Summary		devices.calibrate (write)
//	@ID				devices.calibrate
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T11, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target device, read devices.get)"
//	@Param			deviceId			path		string			true	"input field deviceId"
//	@Param			request				body		CalibrateInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=CalibrationRecord}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{deviceId}/calibrations [post]
func (m *Module) calibrate(
	ctx context.Context,
	call *ops.Call,
	input *CalibrateInput,
) (CalibrationRecord, error) {
	if input.CalibratedAt.After(call.Now) {
		return CalibrationRecord{}, apperr.Fields(
			map[string]string{"calibratedAt": apperr.KeyFuture},
		)
	}

	state, err := m.lockBoundDevice(ctx, call, input.DeviceID, input.JobID)
	if err != nil {
		return CalibrationRecord{}, err
	}

	err = matchSensor(ctx, call, input)
	if err != nil {
		return CalibrationRecord{}, err
	}

	exclusiveErr := m.exclusive(ctx, call, *state.unit)
	if exclusiveErr != nil {
		return CalibrationRecord{}, exclusiveErr
	}

	recordID := uuid.Must(uuid.NewV7())

	err = insertCalibration(ctx, call, input, recordID)
	if err != nil {
		return CalibrationRecord{}, err
	}

	version, err := bumpDevice(ctx, call, input.DeviceID)
	if err != nil {
		return CalibrationRecord{}, err
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "devices.calibrate",
			TargetKind:      entityDevice,
			TargetID:        input.DeviceID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   input.DeviceID,
			Type:          "DeviceCalibrated",
			Payload:       nil,
		},
	)

	return m.calibration(ctx, call, recordID)
}

// matchSensor checks that the sensor is on the device and measures the input's metric in its unit.
func matchSensor(ctx context.Context, call *ops.Call, input *CalibrateInput) error {
	var metric, unit string

	err := call.Tx.QueryRow(ctx, `SELECT metric, unit FROM devices.sensors WHERE id = $1 AND device_id =
		$2`, input.SensorID, input.DeviceID).
		Scan(&metric, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Fields(
			map[string]string{"sensorId": "error.notOnDevice"},
		)
	}

	if err != nil {
		return apperr.Wrap(err, "select devices.sensors")
	}

	if metric != input.Metric || unit != input.Unit {
		return apperr.Fields(
			map[string]string{"metric": "error.sensorMismatch"},
		)
	}

	return nil
}

// insertCalibration stores the calibration record and, when it is the later one, the sensor's calibratedAt.
func insertCalibration(
	ctx context.Context,
	call *ops.Call,
	input *CalibrateInput,
	recordID uuid.UUID,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.calibration_records (id, tenant_id, device_id, sensor_id, metric, unit, reference_value,
			measured_value, calibrated_at, actor_id, job_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		recordID,
		input.DeviceID,
		input.SensorID,
		input.Metric,
		input.Unit,
		input.ReferenceValue,
		input.MeasuredValue,
		input.CalibratedAt,
		call.Principal.UserID,
		input.JobID,
	)
	if err != nil {
		return apperr.Wrap(err, "insert devices.calibration_records")
	}

	// the sensor keeps its latest calibration: one entered late, done earlier, only joins the history (IR748)
	_, err = call.Tx.Exec(
		ctx,
		`UPDATE devices.sensors SET calibrated_at = GREATEST(calibrated_at, $2) WHERE id = $1`,
		input.SensorID,
		input.CalibratedAt,
	)

	return apperr.Wrap(err, "update devices.sensors")
}

const calCols = `r.id, r.tenant_id, r.created_at, r.created_at AS updated_at, b.id, b.unit_id, b.customer_org_id, ` +
	`r.device_id, r.sensor_id, r.metric, r.unit,
	r.reference_value, r.measured_value, r.calibrated_at, r.actor_id`

func scanCal(row pgx.Row) (CalibrationRecord, error) {
	var calibrationRecord CalibrationRecord

	err := row.Scan(
		&calibrationRecord.ID,
		&calibrationRecord.TenantID,
		&calibrationRecord.CreatedAt,
		&calibrationRecord.UpdatedAt,
		&calibrationRecord.BindingID,
		&calibrationRecord.UnitIDAtOccurrence,
		&calibrationRecord.CustomerOrgIDAtOccurrence,
		&calibrationRecord.DeviceID,
		&calibrationRecord.SensorID,
		&calibrationRecord.Metric,
		&calibrationRecord.Unit,
		&calibrationRecord.ReferenceValue,
		&calibrationRecord.MeasuredValue,
		&calibrationRecord.CalibratedAt,
		&calibrationRecord.ActorID,
	)
	calibrationRecord.Version, calibrationRecord.IsDemo = 1, true

	return calibrationRecord, apperr.Wrap(err, "scan (scanCal)")
}

// calibration rows carry the binding at their time (SR24).
const calFrom = ` FROM devices.calibration_records r LEFT JOIN devices.device_bindings b ON b.device_id = r.device_id
	AND b.bound_at <= r.created_at AND (b.unbound_at IS NULL OR b.unbound_at > r.created_at)`

func (*Module) calibration(
	ctx context.Context,
	c *ops.Call,
	id uuid.UUID,
) (CalibrationRecord, error) {
	return scanCal(c.Tx.QueryRow(ctx, `SELECT `+calCols+calFrom+` WHERE r.id = $1`, id))
}
