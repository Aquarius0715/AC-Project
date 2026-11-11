package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// TempRange is Capability.temperature.
type TempRange struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`
}

// SensorSpec is a Capability sensor (Sensor without id/deviceId/calibratedAt).
type SensorSpec struct {
	BoundaryID        *string `json:"boundaryId"`
	Metric            string  `json:"metric"`
	Unit              string  `json:"unit"`
	StaleAfterSeconds int     `json:"staleAfterSeconds"`
}

// Capability is Capability of service-contracts.ts (one row per version; the current row is listed).
type Capability struct {
	ID                 uuid.UUID    `json:"id"`
	TenantID           uuid.UUID    `json:"tenantId"`
	Version            int          `json:"version"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Control            bool         `json:"control"`
	ModeControl        bool         `json:"modeControl"`
	FanControl         bool         `json:"fanControl"`
	Temperature        *TempRange   `json:"temperature"`
	Modes              []string     `json:"modes"`
	FanLevels          []string     `json:"fanLevels"`
	Ventilation        bool         `json:"ventilation"`
	VentilationLevels  []string     `json:"ventilationLevels"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
}

const capCols = `id, tenant_id, version, created_at, created_at AS updated_at, manufacturer, model, control, ` +
	`mode_control, fan_control, temperature, modes, fan_levels,
	ventilation, ventilation_levels, sensors, firmware_candidates`

func scanCap(row pgx.Row) (Capability, error) {
	var (
		capability    Capability
		temp, sensors []byte
	)

	err := row.Scan(
		&capability.ID,
		&capability.TenantID,
		&capability.Version,
		&capability.CreatedAt,
		&capability.UpdatedAt,
		&capability.Manufacturer,
		&capability.Model,
		&capability.Control,
		&capability.ModeControl,
		&capability.FanControl,
		&temp,
		&capability.Modes,
		&capability.FanLevels,
		&capability.Ventilation,
		&capability.VentilationLevels,
		&sensors,
		&capability.FirmwareCandidates,
	)
	if err != nil {
		return capability, apperr.Wrap(err, "scan (scanCap)")
	}

	if len(temp) > 0 {
		capability.Temperature = &TempRange{Min: 0, Max: 0, Step: 0}

		err := json.Unmarshal(temp, capability.Temperature)
		if err != nil {
			return capability, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	capability.Sensors = []SensorSpec{}
	if len(sensors) > 0 {
		err := json.Unmarshal(sensors, &capability.Sensors)
		if err != nil {
			return capability, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	return capability, nil
}

// capabilitiesList serves capabilities.list.
//
//	@Summary		capabilities.list (read)
//	@ID				capabilities.list
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:device.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-A04 · Query: filters none · sort id,createdAt,updatedAt (default id asc)
//	@Tags			capabilities
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Success		200		{object}	ops.Envelope{data=CapabilityPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/capabilities [get]
func capabilitiesList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Capability], error) {
	if len(query.Filters) > 0 && string(query.Filters) != "{}" && string(query.Filters) != "null" {
		return paging.Page[Capability]{}, apperr.Fields(
			map[string]string{fieldFilters: apperr.KeyInvalid},
		) // query catalog: no filters
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "id", fieldCreatedAt: colCreatedAt, fieldUpdatedAt: colCreatedAt},
		"id ASC",
	)
	if err != nil {
		return paging.Page[Capability]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, nil, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Capability]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, `SELECT count(*) FROM devices.capabilities WHERE is_current`).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Capability]{}, apperr.Wrap(scanErr, "select devices.capabilities")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			`SELECT %s FROM devices.capabilities WHERE is_current ORDER BY %s LIMIT %d OFFSET %d`,
			capCols,
			order,
			window.Limit,
			window.Offset,
		),
	)
	if err != nil {
		return paging.Page[Capability]{}, apperr.Wrap(err, "select devices.capabilities")
	}

	items, err := scanCaps(rows)

	return paging.Page[Capability]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, err
}

// scanCaps reads a page of capabilities.
func scanCaps(rows pgx.Rows) ([]Capability, error) {
	defer rows.Close()

	items := []Capability{}

	for rows.Next() {
		capability, err := scanCap(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, capability)
	}

	return items, apperr.Wrap(rows.Err(), "select devices.capabilities")
}

// CapabilitySave is capabilities.save input.
type CapabilitySave struct {
	ID                 *uuid.UUID   `json:"id,omitempty"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Control            bool         `json:"control"`
	ModeControl        bool         `json:"modeControl"`
	FanControl         bool         `json:"fanControl"`
	Temperature        *TempRange   `json:"temperature"`
	Modes              []string     `json:"modes"`
	FanLevels          []string     `json:"fanLevels"`
	Ventilation        bool         `json:"ventilation"`
	VentilationLevels  []string     `json:"ventilationLevels"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
	ChangeReason       *string      `json:"changeReason,omitempty"`
}

// knownModes are the operating modes a model may support.
func knownModes() map[string]bool { return map[string]bool{"cool": true, "dry": true, "fan": true} }

// fanLevels are the fan (and ventilation) levels a model may support.
func fanLevels() map[string]bool { return map[string]bool{"low": true, "mid": true, "high": true} }

// metricUnits is the unit of each metric a sensor may measure.
func metricUnits() map[string]string {
	return map[string]string{
		"temperature":          "°C",
		"humidity":             "%",
		"co2":                  "ppm",
		"pm25":                 "µg/m³",
		"power":                "kW",
		"vibration":            "mm/s",
		"refrigerant_pressure": "kPa",
		"compressor_cycles":    "cycles/h",
		"airflow_drop":         "%",
		"heartbeat_gap":        "min",
	}
}

// boundaryIDs are the metering boundaries a power sensor may name.
func boundaryIDs() map[string]bool {
	return map[string]bool{"ac_input_electricity": true, "whole_building_electricity": true}
}

func uniqueIn(vals []string, allowed map[string]bool) bool {
	seen := map[string]bool{}
	for _, v := range vals {
		if (allowed != nil && !allowed[v]) || seen[v] || v == "" {
			return false
		}

		seen[v] = true
	}

	return true
}

// Validate implements ops.Validator (DD-A04).
func (in *CapabilitySave) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if n := utf8.RuneCountInString(in.Manufacturer); n < 1 || n > 120 {
		fieldErrs["manufacturer"] = apperr.KeyLength
	}

	if n := utf8.RuneCountInString(in.Model); n < 1 || n > 120 {
		fieldErrs["model"] = apperr.KeyLength
	}

	in.validateControl(fieldErrs)
	in.validateLevels(fieldErrs)
	validateSensors(fieldErrs, in.Sensors)
	validateFirmwareCandidates(fieldErrs, in.FirmwareCandidates)
	in.validateChangeReason(fieldErrs)

	return fieldErrs
}

// validateControl checks the temperature range and that mode, fan or temperature control comes with control.
func (in *CapabilitySave) validateControl(fieldErrs map[string]string) {
	if t := in.Temperature; t != nil && (t.Min >= t.Max || t.Step <= 0 || t.Step > t.Max-t.Min) {
		fieldErrs["temperature"] = apperr.KeyRange
	}

	if !in.Control && (in.ModeControl || in.FanControl || in.Temperature != nil) {
		fieldErrs["control"] = "error.controlRequired" // mode/fan/temperature control needs control=true
	}
}

// validateLevels checks the modes, fan levels and ventilation levels: known values, each once, and present when their
// control is (absent without ventilation).
func (in *CapabilitySave) validateLevels(fieldErrs map[string]string) {
	if !uniqueIn(in.Modes, knownModes()) || (in.ModeControl && len(in.Modes) == 0) {
		fieldErrs["modes"] = apperr.KeyInvalid
	}

	if !uniqueIn(in.FanLevels, fanLevels()) || (in.FanControl && len(in.FanLevels) == 0) {
		fieldErrs["fanLevels"] = apperr.KeyInvalid
	}

	if !uniqueIn(in.VentilationLevels, fanLevels()) ||
		(!in.Ventilation && len(in.VentilationLevels) > 0) {
		fieldErrs["ventilationLevels"] = apperr.KeyInvalid
	}
}

// validateChangeReason applies DD-A04: a change reason is required on update and not taken on create.
func (in *CapabilitySave) validateChangeReason(fieldErrs map[string]string) {
	switch {
	case in.ID != nil && in.ChangeReason == nil:
		fieldErrs["changeReason"] = apperr.KeyRequired
	case in.ChangeReason != nil &&
		(utf8.RuneCountInString(strings.TrimSpace(*in.ChangeReason)) < 1 || utf8.RuneCountInString(*in.ChangeReason) > 1000):
		fieldErrs["changeReason"] = apperr.KeyLength
	}
}

// validateSensors checks each sensor spec: a known metric at most once, in its unit, a stale limit of 10 s–24 h, and a
// boundary only on a power sensor.
func validateSensors(fieldErrs map[string]string, sensors []SensorSpec) {
	units, boundaries, seen := metricUnits(), boundaryIDs(), map[string]bool{}

	for i, sensorSpec := range sensors {
		key := fmt.Sprintf("sensors[%d]", i)

		switch {
		case units[sensorSpec.Metric] == "" || seen[sensorSpec.Metric]:
			fieldErrs[key+".metric"] = apperr.KeyInvalid
		case units[sensorSpec.Metric] != sensorSpec.Unit:
			fieldErrs[key+".unit"] = "error.unitMismatch"
		case sensorSpec.StaleAfterSeconds < 10 || sensorSpec.StaleAfterSeconds > 86400:
			fieldErrs[key+".staleAfterSeconds"] = apperr.KeyRange
		case sensorSpec.BoundaryID != nil && (!boundaries[*sensorSpec.BoundaryID] || sensorSpec.Metric != "power"):
			fieldErrs[key+".boundaryId"] = apperr.KeyInvalid
		}

		seen[sensorSpec.Metric] = true
	}
}

// validateFirmwareCandidates checks the firmware candidates: a candidate is 1–32 characters once trimmed, and no two
// candidates are the same after trimming (the "+ Add version" rule of the Admin device model editor).
func validateFirmwareCandidates(fieldErrs map[string]string, candidates []string) {
	seenFirmware := map[string]bool{}

	for _, v := range candidates {
		x := strings.TrimSpace(v)
		if n := utf8.RuneCountInString(x); n < 1 || n > 32 || seenFirmware[x] {
			fieldErrs["firmwareCandidates"] = apperr.KeyInvalid
		}

		seenFirmware[x] = true
	}
}

// fields are the audited capability values (the before/after of capabilities.save).
func (c Capability) fields() map[string]any {
	return map[string]any{
		"manufacturer":       c.Manufacturer,
		"model":              c.Model,
		"control":            c.Control,
		"modeControl":        c.ModeControl,
		"fanControl":         c.FanControl,
		"temperature":        c.Temperature,
		"modes":              c.Modes,
		"fanLevels":          c.FanLevels,
		"ventilation":        c.Ventilation,
		"ventilationLevels":  c.VentilationLevels,
		"sensors":            c.Sensors,
		"firmwareCandidates": c.FirmwareCandidates,
	}
}

// UnitCapabilities is what Devices calls on Assets when a model gets a new capability version.
type UnitCapabilities interface {
	MoveToCapabilityVersion(
		ctx context.Context,
		c *ops.Call,
		modelID uuid.UUID,
		version int,
	) (int, error)
}

// Capabilities is the operation set for the model register.
type Capabilities struct{ Units UnitCapabilities }

// CapabilityDeleteInput is capabilities.delete input.
type CapabilityDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *CapabilityDeleteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	reasonErrors(&in.Reason, fieldErrs)

	return fieldErrs
}

// delete deletes a model that is in no use (DEC-103), every version of it: no unit has it, and no firmware campaign
// (unfinished or finished, whose history names the model) targets it; otherwise CONFLICT with the reason.
//
//	@Summary		capabilities.delete (write)
//	@ID				capabilities.delete
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR652 only a model no unit uses and no firmware campaign targets (CONFLICT error.modelInUse / \
//	@Description	error.modelInCampaign); deletes every version of the model
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			capabilities
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target capabilities, read capabilities.list)"
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
//	@Router			/v1/capabilities/{id} [delete]
func (Capabilities) delete(
	ctx context.Context,
	call *ops.Call,
	input *CapabilityDeleteInput,
) (DeletedResource, error) {
	var version int

	lockErr := lockModelRows(ctx, call, input.ID)
	if lockErr != nil {
		return DeletedResource{}, lockErr
	}

	err := call.Tx.QueryRow(
		ctx,
		`SELECT version FROM devices.capabilities WHERE id = $1 AND is_current FOR UPDATE`,
		input.ID,
	).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select devices.capabilities")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	var units, campaigns bool

	scanErr := call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE model_id = $1),
		EXISTS (SELECT 1 FROM devices.firmware_campaigns WHERE model_id = $1)`,
		input.ID,
	).Scan(&units, &campaigns)
	if scanErr != nil {
		return DeletedResource{}, apperr.Wrap(scanErr, "select devices.capabilities")
	}

	switch {
	case units:
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.modelInUse")
	case campaigns:
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.modelInCampaign")
	}

	_, execErr := call.Tx.Exec(ctx, `DELETE FROM devices.capabilities WHERE id = $1`, input.ID)
	if execErr != nil {
		return DeletedResource{}, apperr.Wrap(execErr, "delete devices.capabilities")
	}

	call.Emit(
		ops.Event{
			AggregateType: entityCapability,
			AggregateID:   input.ID,
			Type:          "CapabilityDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "capabilities.delete",
			TargetKind:      entityCapability,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}

func nz(a []string) []string {
	if a == nil {
		return []string{}
	}

	return a
}

// save serves capabilities.save.
//
//	@Summary		capabilities.save (write)
//	@ID				capabilities.save
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A04
//	@Tags			capabilities
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		CapabilitySave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Capability}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/capabilities [post]
func (m Capabilities) save(
	ctx context.Context,
	call *ops.Call,
	input *CapabilitySave,
) (Capability, error) {
	temp, sensors, err := capabilityJSON(input)
	if err != nil {
		return Capability{}, err
	}

	var dup bool

	err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.capabilities WHERE is_current AND
		lower(manufacturer) =
		lower($1) AND lower(model) = lower($2)
		AND ($3::uuid IS NULL OR id <> $3))`, input.Manufacturer, input.Model, input.ID).
		Scan(&dup)
	if err != nil {
		return Capability{}, apperr.Wrap(err, "select devices.capabilities")
	}

	if dup {
		return Capability{}, apperr.E(apperr.Conflict, "error.duplicateModel")
	}

	revision, err := nextRevision(ctx, call, input)
	if err != nil {
		return Capability{}, err
	}

	capability, err := insertCapability(ctx, call, input, revision, temp, sensors)
	if err != nil {
		return Capability{}, err
	}

	if revision.version > 1 && m.Units != nil {
		_, err := m.Units.MoveToCapabilityVersion(ctx, call, revision.modelID, revision.version)
		if err != nil {
			return Capability{}, apperr.Wrap(
				err,
				"devices.UnitCapabilities.MoveToCapabilityVersion",
			)
		}
	}

	capabilitySaved(call, revision, capability)

	return capability, nil
}

// insertCapability stores the model version as the current row and returns it.
func insertCapability(
	ctx context.Context,
	call *ops.Call,
	input *CapabilitySave,
	revision capabilityRevision,
	temp any,
	sensors []byte,
) (Capability, error) {
	return scanCap(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control,
				temperature,
		modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
		VALUES ($1,$2,current_setting('app.tenant_id')::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,true) RETURNING
		`+capCols,
			revision.modelID,
			revision.version,
			input.Manufacturer,
			input.Model,
			input.Control,
			input.ModeControl,
			input.FanControl,
			temp,
			nz(input.Modes),
			nz(input.FanLevels),
			input.Ventilation,
			nz(input.VentilationLevels),
			sensors,
			nz(input.FirmwareCandidates),
		),
	)
}

// capabilityJSON is the temperature range (nil without one) and the sensor specs as their JSON columns store them.
func capabilityJSON(input *CapabilitySave) (any, []byte, error) {
	var temp any

	if input.Temperature != nil {
		tempJSON, err := json.Marshal(input.Temperature)
		if err != nil {
			return nil, nil, apperr.Wrap(err, "json.Marshal")
		}

		temp = tempJSON
	}

	sensors, err := json.Marshal(append([]SensorSpec{}, input.Sensors...))
	if err != nil {
		return nil, nil, apperr.Wrap(err, "json.Marshal")
	}

	return temp, sensors, nil
}

// capabilityRevision is the model row a save writes: its ID and version, the change reason and the audited values
// of the row it replaces.
type capabilityRevision struct {
	modelID uuid.UUID
	version int
	reason  string
	before  map[string]any
}

// nextRevision is version 1 of a new model or, for an update, the next version of the current row, which it retires
// after the expected-version check (the change reason is taken on update only).
func nextRevision(
	ctx context.Context,
	call *ops.Call,
	input *CapabilitySave,
) (capabilityRevision, error) {
	revision := capabilityRevision{
		modelID: uuid.Must(uuid.NewV7()),
		version: 1,
		reason:  "",
		before:  nil,
	}
	if input.ID == nil {
		return revision, nil
	}

	lockErr := lockModelRows(ctx, call, *input.ID)
	if lockErr != nil {
		return revision, lockErr
	}

	old, err := scanCap(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+capCols+` FROM devices.capabilities WHERE id = $1 AND is_current FOR UPDATE`,
			*input.ID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return revision, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return revision, err
	}

	revision.before = old.fields()

	if old.Version != *call.ExpectedVersion {
		return revision, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE devices.capabilities SET is_current = false WHERE id = $1 AND version = $2`,
		*input.ID,
		old.Version,
	)
	if execErr != nil {
		return revision, apperr.Wrap(execErr, "update devices.capabilities")
	}

	revision.modelID, revision.version = *input.ID, old.Version+1
	if input.ChangeReason != nil {
		revision.reason = *input.ChangeReason
	}

	return revision, nil
}

// capabilitySaved emits CapabilityChanged and audits capabilities.save with the values it changed.
func capabilitySaved(call *ops.Call, revision capabilityRevision, capability Capability) {
	var prev *int

	if revision.version > 1 {
		p := revision.version - 1
		prev = &p
	}

	call.Emit(
		ops.Event{
			AggregateType: entityCapability,
			AggregateID:   revision.modelID,
			Type:          "CapabilityChanged",
			Payload:       map[string]int{"version": revision.version},
		},
	)

	changedBefore, changedAfter := ops.Changes(revision.before, capability.fields())
	call.Audit(
		ops.AuditEntry{
			Action:          "capabilities.save",
			TargetKind:      entityCapability,
			TargetID:        revision.modelID.String(),
			PreviousVersion: prev,
			NextVersion:     &revision.version,
			Reason:          revision.reason,
			Before:          changedBefore,
			After:           changedAfter, Result: "",
		},
	)
}

// Register binds the Devices operations implemented so far.
func Register(r *ops.Registry, m Capabilities) {
	ops.Register(r, "capabilities.list", capabilitiesList)
	ops.Register(r, "capabilities.save", m.save)
	ops.Register(r, "capabilities.delete", m.delete)
}

// LoadCapability returns a capability version (UnitDetail.capabilities).
func LoadCapability(
	ctx context.Context,
	c *ops.Call,
	modelID uuid.UUID,
	version int,
) (Capability, bool, error) {
	capability, err := scanCap(
		c.Tx.QueryRow(
			ctx,
			`SELECT `+capCols+` FROM devices.capabilities WHERE id = $1 AND version = $2`,
			modelID,
			version,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return capability, false, nil
	}

	return capability, err == nil, err
}

// lockModelRows locks every version row of a capability FOR UPDATE before a write reads its current version (IR766):
// a write that waited for a new version saved meanwhile then reads that version and answers a version conflict,
// instead of finding the row it waited for no longer current and answering NOT_FOUND.
func lockModelRows(ctx context.Context, call *ops.Call, id uuid.UUID) error {
	_, err := call.Tx.Exec(
		ctx,
		`SELECT 1 FROM devices.capabilities WHERE id = $1 ORDER BY version FOR UPDATE`,
		id,
	)

	return apperr.Wrap(err, "select devices.capabilities")
}
