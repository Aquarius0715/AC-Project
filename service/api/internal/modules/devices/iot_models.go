package devices

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

const entityIoTModel = "iotModel"

// IoTModel is IoTModel of service-contracts.ts (DEC-105): the register of IoT device models, one row per version; the
// current row is listed. It holds what a device of the model measures and which firmware it can be updated to.
type IoTModel struct {
	ID                 uuid.UUID    `json:"id"`
	TenantID           uuid.UUID    `json:"tenantId"`
	Version            int          `json:"version"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
}

const iotCols = `id, tenant_id, version, created_at, created_at AS updated_at, manufacturer, model, sensors, ` +
	`firmware_candidates`

func scanIoTModel(row pgx.Row) (IoTModel, error) {
	var (
		model   IoTModel
		sensors []byte
	)

	err := row.Scan(
		&model.ID,
		&model.TenantID,
		&model.Version,
		&model.CreatedAt,
		&model.UpdatedAt,
		&model.Manufacturer,
		&model.Model,
		&sensors,
		&model.FirmwareCandidates,
	)
	if err != nil {
		return model, apperr.Wrap(err, "scan (scanIoTModel)")
	}

	model.Sensors = []SensorSpec{}
	if len(sensors) > 0 {
		err := json.Unmarshal(sensors, &model.Sensors)
		if err != nil {
			return model, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	return model, nil
}

// fields are the audited IoT model values (the before/after of iotModels.save).
func (m IoTModel) fields() map[string]any {
	return map[string]any{
		"manufacturer":       m.Manufacturer,
		"model":              m.Model,
		"sensors":            m.Sensors,
		"firmwareCandidates": m.FirmwareCandidates,
	}
}

// LoadIoTModel returns an IoT model version (UnitDetail.iotModel).
func LoadIoTModel(
	ctx context.Context,
	c *ops.Call,
	modelID uuid.UUID,
	version int,
) (IoTModel, bool, error) {
	model, err := scanIoTModel(
		c.Tx.QueryRow(
			ctx,
			`SELECT `+iotCols+` FROM devices.iot_models WHERE id = $1 AND version = $2`,
			modelID,
			version,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return model, false, nil
	}

	return model, err == nil, err
}

// iotModelsList serves iotModels.list.
//
//	@Summary		iotModels.list (read)
//	@ID				iotModels.list
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:device.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-A04 · Query: filters none · sort id,createdAt,updatedAt (default id asc)
//	@Tags			iotModels
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Success		200		{object}	ops.Envelope{data=IoTModelPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/iot-models [get]
func iotModelsList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[IoTModel], error) {
	return iotModelRegister().list(ctx, call, query)
}

// IoTModelSave is iotModels.save input.
type IoTModelSave struct {
	ID                 *uuid.UUID   `json:"id,omitempty"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
	ChangeReason       *string      `json:"changeReason,omitempty"`
}

// Validate implements ops.Validator (DD-A04).
func (in *IoTModelSave) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if n := utf8.RuneCountInString(in.Manufacturer); n < 1 || n > 120 {
		fieldErrs["manufacturer"] = apperr.KeyLength
	}

	if n := utf8.RuneCountInString(in.Model); n < 1 || n > 120 {
		fieldErrs["model"] = apperr.KeyLength
	}

	validateSensors(fieldErrs, in.Sensors)
	validateFirmwareCandidates(fieldErrs, in.FirmwareCandidates)

	switch {
	case in.ID != nil && in.ChangeReason == nil:
		fieldErrs["changeReason"] = apperr.KeyRequired
	case in.ChangeReason != nil &&
		(utf8.RuneCountInString(strings.TrimSpace(*in.ChangeReason)) < 1 || utf8.RuneCountInString(*in.ChangeReason) > 1000):
		fieldErrs["changeReason"] = apperr.KeyLength
	}

	return fieldErrs
}

// IoTModelUnits is what Devices calls on Assets when an IoT model gets a new version.
type IoTModelUnits interface {
	MoveToIoTModelVersion(
		ctx context.Context,
		c *ops.Call,
		modelID uuid.UUID,
		version int,
	) (int, error)
}

// IoTModels is the operation set for the IoT model register.
type IoTModels struct{ Units IoTModelUnits }

// save serves iotModels.save.
//
//	@Summary		iotModels.save (write)
//	@ID				iotModels.save
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			iotModels
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		IoTModelSave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=IoTModel}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/iot-models [post]
func (m IoTModels) save(
	ctx context.Context,
	call *ops.Call,
	input *IoTModelSave,
) (IoTModel, error) {
	sensors, err := json.Marshal(append([]SensorSpec{}, input.Sensors...))
	if err != nil {
		return IoTModel{}, apperr.Wrap(err, "json.Marshal")
	}

	var dup bool

	err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.iot_models WHERE is_current AND
		lower(manufacturer) = lower($1) AND lower(model) = lower($2) AND ($3::uuid IS NULL OR id <> $3))`,
		input.Manufacturer, input.Model, input.ID).
		Scan(&dup)
	if err != nil {
		return IoTModel{}, apperr.Wrap(err, "select devices.iot_models")
	}

	if dup {
		return IoTModel{}, apperr.E(apperr.Conflict, "error.duplicateModel")
	}

	revision, err := iotModelRegister().revise(ctx, call, input.ID, input.ChangeReason)
	if err != nil {
		return IoTModel{}, err
	}

	model, err := scanIoTModel(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO devices.iot_models (id, version, tenant_id, manufacturer, model, sensors, firmware_candidates,
				is_current)
			VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3, $4, $5, $6, true) RETURNING `+iotCols,
			revision.modelID,
			revision.version,
			input.Manufacturer,
			input.Model,
			sensors,
			nz(input.FirmwareCandidates),
		),
	)
	if err != nil {
		return IoTModel{}, err
	}

	if revision.version > 1 {
		err := m.moveToVersion(ctx, call, revision)
		if err != nil {
			return IoTModel{}, err
		}
	}

	iotModelSaved(call, revision, model)

	return model, nil
}

// moveToVersion points the model's units and devices at its new version.
func (m IoTModels) moveToVersion(
	ctx context.Context,
	call *ops.Call,
	revision capabilityRevision,
) error {
	if m.Units != nil {
		_, err := m.Units.MoveToIoTModelVersion(ctx, call, revision.modelID, revision.version)
		if err != nil {
			return apperr.Wrap(err, "devices.IoTModelUnits.MoveToIoTModelVersion")
		}
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE devices.devices SET iot_model_version = $2 WHERE iot_model_id = $1 AND iot_model_version IS DISTINCT FROM $2`,
		revision.modelID,
		revision.version,
	)

	return apperr.Wrap(err, "update devices.devices")
}

// iotModelSaved emits IoTModelChanged and audits iotModels.save with the values it changed.
func iotModelSaved(call *ops.Call, revision capabilityRevision, model IoTModel) {
	var prev *int

	if revision.version > 1 {
		p := revision.version - 1
		prev = &p
	}

	call.Emit(
		ops.Event{
			AggregateType: entityIoTModel,
			AggregateID:   revision.modelID,
			Type:          "IoTModelChanged",
			Payload:       map[string]int{"version": revision.version},
		},
	)

	changedBefore, changedAfter := ops.Changes(revision.before, model.fields())
	call.Audit(
		ops.AuditEntry{
			Action:          "iotModels.save",
			TargetKind:      entityIoTModel,
			TargetID:        revision.modelID.String(),
			PreviousVersion: prev,
			NextVersion:     &revision.version,
			Reason:          revision.reason,
			Before:          changedBefore,
			After:           changedAfter,
			Result:          "",
		},
	)
}

// IoTModelDeleteInput is iotModels.delete input.
type IoTModelDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *IoTModelDeleteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	reasonErrors(&in.Reason, fieldErrs)

	return fieldErrs
}

// delete deletes an IoT model that is in no use, every version of it: no unit or device has it and no firmware
// campaign (unfinished or finished, whose history names the model) targets it; otherwise CONFLICT with the reason.
//
//	@Summary		iotModels.delete (write)
//	@ID				iotModels.delete
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	DEC-105 only an IoT model no unit or device uses and no firmware campaign targets (CONFLICT \
//	@Description	error.modelInUse / error.modelInCampaign); deletes every version of the model
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			iotModels
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target iotModels, read iotModels.list)"
//	@Param			id					path		string	true	"input field id"
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
//	@Router			/v1/iot-models/{id} [delete]
func (IoTModels) delete(
	ctx context.Context,
	call *ops.Call,
	input *IoTModelDeleteInput,
) (DeletedResource, error) {
	var version int

	lockErr := iotModelRegister().lock(ctx, call, input.ID)
	if lockErr != nil {
		return DeletedResource{}, lockErr
	}

	err := call.Tx.QueryRow(
		ctx,
		`SELECT version FROM devices.iot_models WHERE id = $1 AND is_current FOR UPDATE`,
		input.ID,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select devices.iot_models")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	var used, campaigns bool

	scanErr := call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE iot_model_id = $1)
			OR EXISTS (SELECT 1 FROM devices.devices WHERE iot_model_id = $1),
		EXISTS (SELECT 1 FROM devices.firmware_campaigns WHERE model_id = $1)`,
		input.ID,
	).Scan(&used, &campaigns)
	if scanErr != nil {
		return DeletedResource{}, apperr.Wrap(scanErr, "select devices.iot_models")
	}

	switch {
	case used:
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.modelInUse")
	case campaigns:
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.modelInCampaign")
	}

	_, execErr := call.Tx.Exec(ctx, `DELETE FROM devices.iot_models WHERE id = $1`, input.ID)
	if execErr != nil {
		return DeletedResource{}, apperr.Wrap(execErr, "delete devices.iot_models")
	}

	call.Emit(
		ops.Event{
			AggregateType: entityIoTModel,
			AggregateID:   input.ID,
			Type:          "IoTModelDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "iotModels.delete",
			TargetKind:      entityIoTModel,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}

// RegisterIoTModels binds the IoT model register operations.
func RegisterIoTModels(r *ops.Registry, m IoTModels) {
	ops.Register(r, "iotModels.list", iotModelsList)
	ops.Register(r, "iotModels.save", m.save)
	ops.Register(r, "iotModels.delete", m.delete)
}
