package devices

import (
	"context"
	"errors"
	"math"
	"slices"
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

type devState struct {
	unit       *uuid.UUID
	target     uuid.UUID
	creator    uuid.UUID
	version    int
	connection string
	tamper     string
	firmware   string
}

// lockDevice loads the device for update and checks the expected version (NOT_FOUND / CONFLICT).
func lockDevice(ctx context.Context, c *ops.Call, id uuid.UUID) (devState, error) {
	var s devState
	err := c.Tx.QueryRow(ctx, `SELECT unit_id, target_unit_id, created_by_membership_id, version, connection, tamper, firmware_version
		FROM devices.devices WHERE id = $1 FOR UPDATE`, id).Scan(&s.unit, &s.target, &s.creator, &s.version, &s.connection, &s.tamper, &s.firmware)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return s, err
	}
	if c.ExpectedVersion != nil && s.version != *c.ExpectedVersion {
		return s, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return s, nil
}

// exclusive applies D05 on a unit: no queued/running DeviceOperation for its device and no in-flight control work.
func (m *Module) exclusive(ctx context.Context, c *ops.Call, unit uuid.UUID) error {
	var busy bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND status IN ('queued','running') AND expires_at > $2)`,
		unit, c.Now).Scan(&busy); err != nil {
		return err
	}
	for _, x := range m.Exclusion {
		if busy {
			break
		}
		b, err := x.UnitBusy(ctx, c, unit)
		if err != nil {
			return err
		}
		busy = b
	}
	if busy {
		return apperr.E(apperr.Conflict, "error.unitBusy")
	}
	return nil
}

func bumpDevice(ctx context.Context, c *ops.Call, id uuid.UUID) (int, error) {
	var v int
	err := c.Tx.QueryRow(ctx, `UPDATE devices.devices SET version = version + 1, updated_at = platform.app_now() WHERE id = $1 RETURNING version`, id).Scan(&v)
	return v, err
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
	fe := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fe["deviceId"] = "error.required"
	}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	return fe
}

// bind attaches the device to a unit (D05): both units in scope, reason, exclusion on both units, no unresolved
// tamper, at most one active device per unit; the old binding keeps its end time and the new binding gets new sensors.
func (m *Module) bind(ctx context.Context, c *ops.Call, in *BindInput) (DeviceDetail, error) {
	if err := m.technicianGate(ctx, c, in.JobID, in.UnitID); err != nil {
		return DeviceDetail{}, err
	}
	s, err := lockDevice(ctx, c, in.DeviceID)
	if err != nil {
		return DeviceDetail{}, err
	}
	if s.unit != nil && *s.unit == in.UnitID {
		return DeviceDetail{}, apperr.E(apperr.Conflict, "error.alreadyBound")
	}
	if s.tamper == "detected" {
		return DeviceDetail{}, apperr.E(apperr.Conflict, "error.tamperUnresolved")
	}
	u, ok, err := m.Units.UnitInfo(ctx, c, in.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}
	if !ok {
		return DeviceDetail{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	units := []uuid.UUID{in.UnitID}
	if s.unit != nil {
		units = append(units, *s.unit)
	}
	for _, unit := range units {
		if err := m.exclusive(ctx, c, unit); err != nil {
			return DeviceDetail{}, err
		}
	}
	var taken bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_bindings WHERE unit_id = $1 AND unbound_at IS NULL)`, in.UnitID).Scan(&taken); err != nil {
		return DeviceDetail{}, err
	}
	if taken {
		return DeviceDetail{}, apperr.E(apperr.Conflict, "error.unitHasDevice")
	}
	if s.unit != nil {
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.device_bindings SET unbound_at = $2, reason = $3 WHERE device_id = $1 AND unbound_at IS NULL`, in.DeviceID, c.Now, in.Reason); err != nil {
			return DeviceDetail{}, err
		}
		// new binding → new sensor IDs; the old sensors are detached (device_id kept for history) by re-keying
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.sensors SET id = gen_random_uuid(), calibrated_at = NULL WHERE device_id = $1 AND NOT EXISTS
			(SELECT 1 FROM devices.calibration_records r WHERE r.sensor_id = devices.sensors.id)`, in.DeviceID); err != nil {
			return DeviceDetail{}, err
		}
	}
	binding := uuid.Must(uuid.NewV7())
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7)`, binding, in.DeviceID, in.UnitID, u.OrgID, c.Now, in.Reason, c.Principal.MembershipID); err != nil {
		return DeviceDetail{}, err
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = $2, binding_id = $3, target_unit_id = $2, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, in.DeviceID, in.UnitID, binding).Scan(&v); err != nil {
		return DeviceDetail{}, err
	}
	c.Emit(ops.Event{AggregateType: "device", AggregateID: in.DeviceID, Type: "DeviceBound", Payload: map[string]any{"unitId": in.UnitID, "previousUnitId": s.unit}})
	c.Audit(ops.AuditEntry{Action: "devices.bind", TargetKind: "device", TargetID: in.DeviceID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: in.Reason})
	return m.detail(ctx, c, "d.id = $1", []any{in.DeviceID})
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

const opCols = `o.id, o.tenant_id, o.version, o.created_at, o.updated_at, b.id, o.unit_id, b.customer_org_id, o.device_id, o.kind, o.status, o.target_version,
	o.failure_code, o.started_at, o.finished_at, o.expires_at`

func scanOp(r pgx.Row) (DeviceOperation, error) {
	var o DeviceOperation
	err := r.Scan(&o.ID, &o.TenantID, &o.Version, &o.CreatedAt, &o.UpdatedAt, &o.BindingID, &o.UnitIDAtOccurrence, &o.CustomerOrgIDAtOccurrence, &o.DeviceID,
		&o.Kind, &o.Status, &o.TargetVersion, &o.FailureCode, &o.StartedAt, &o.FinishedAt, &o.ExpiresAt)
	return o, err
}

func (m *Module) startOperation(ctx context.Context, c *ops.Call, device uuid.UUID, jobID *uuid.UUID, kind string, target *string) (DeviceOperation, error) {
	s, err := lockDevice(ctx, c, device)
	if err != nil {
		return DeviceOperation{}, err
	}
	if s.unit == nil {
		return DeviceOperation{}, apperr.E(apperr.Conflict, "error.deviceNotBound")
	}
	if err := m.technicianGate(ctx, c, jobID, *s.unit); err != nil {
		return DeviceOperation{}, err
	}
	if kind == "firmware" {
		u, _, err := m.Units.UnitInfo(ctx, c, *s.unit)
		if err != nil {
			return DeviceOperation{}, err
		}
		var candidates []string
		if err := c.Tx.QueryRow(ctx, `SELECT firmware_candidates FROM devices.capabilities WHERE id = $1 AND version = $2`, u.ModelID, u.CapabilityVersion).Scan(&candidates); err != nil {
			return DeviceOperation{}, err
		}
		if !slices.Contains(candidates, *target) || *target == s.firmware {
			return DeviceOperation{}, apperr.Fields(map[string]string{"firmwareVersion": "error.unsupportedFirmware"})
		}
		if s.connection != "online" {
			return DeviceOperation{}, apperr.E(apperr.Offline, "error.offline") // IR94: no DeviceOperation when not online
		}
	}
	if err := m.exclusive(ctx, c, *s.unit); err != nil {
		return DeviceOperation{}, err
	}
	id := uuid.Must(uuid.NewV7())
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, job_id, kind, status, target_version, expires_at, actor_membership_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, 'queued', $6, $7, $8)`, id, device, *s.unit, jobID, kind, target, c.Now.Add(operationTTL), c.Principal.MembershipID); err != nil {
		return DeviceOperation{}, err
	}
	v, err := bumpDevice(ctx, c, device)
	if err != nil {
		return DeviceOperation{}, err
	}
	c.Emit(ops.Event{AggregateType: "device", AggregateID: device, Type: "DeviceOperationRequested", Payload: map[string]any{"operationId": id, "kind": kind}})
	c.Audit(ops.AuditEntry{Action: "devices." + map[string]string{"check": "check", "firmware": "updateFirmware"}[kind], TargetKind: "device", TargetID: device.String(),
		PreviousVersion: c.ExpectedVersion, NextVersion: &v})
	return scanOp(c.Tx.QueryRow(ctx, `SELECT `+opCols+` FROM devices.device_operations o LEFT JOIN devices.device_bindings b ON b.device_id = o.device_id AND b.unbound_at IS NULL WHERE o.id = $1`, id))
}

// CheckInput is devices.check input.
type CheckInput struct {
	ID    uuid.UUID  `json:"id"`
	JobID *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *CheckInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (m *Module) check(ctx context.Context, c *ops.Call, in *CheckInput) (DeviceOperation, error) {
	return m.startOperation(ctx, c, in.ID, in.JobID, "check", nil) // accepted regardless of connection (IR94)
}

// FirmwareInput is devices.updateFirmware input.
type FirmwareInput struct {
	DeviceID        uuid.UUID  `json:"deviceId"`
	FirmwareVersion string     `json:"firmwareVersion"`
	JobID           *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *FirmwareInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fe["deviceId"] = "error.required"
	}
	if n := utf8.RuneCountInString(in.FirmwareVersion); n < 1 || n > 64 {
		fe["firmwareVersion"] = "error.required"
	}
	return fe
}

func (m *Module) updateFirmware(ctx context.Context, c *ops.Call, in *FirmwareInput) (DeviceOperation, error) {
	return m.startOperation(ctx, c, in.DeviceID, in.JobID, "firmware", &in.FirmwareVersion)
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
	fe := map[string]string{}
	if in.DeviceID == uuid.Nil {
		fe["deviceId"] = "error.required"
	}
	if in.SensorID == uuid.Nil {
		fe["sensorId"] = "error.required"
	}
	if metricUnit[in.Metric] == "" || metricUnit[in.Metric] != in.Unit {
		fe["unit"] = "error.unitMismatch"
	}
	for k, v := range map[string]float64{"referenceValue": in.ReferenceValue, "measuredValue": in.MeasuredValue} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			fe[k] = "error.invalid"
		}
	}
	if in.CalibratedAt.IsZero() {
		fe["calibratedAt"] = "error.required"
	}
	return fe
}

func (m *Module) calibrate(ctx context.Context, c *ops.Call, in *CalibrateInput) (CalibrationRecord, error) {
	if in.CalibratedAt.After(c.Now) {
		return CalibrationRecord{}, apperr.Fields(map[string]string{"calibratedAt": "error.future"})
	}
	s, err := lockDevice(ctx, c, in.DeviceID)
	if err != nil {
		return CalibrationRecord{}, err
	}
	if s.unit == nil {
		return CalibrationRecord{}, apperr.E(apperr.Conflict, "error.deviceNotBound")
	}
	if err := m.technicianGate(ctx, c, in.JobID, *s.unit); err != nil {
		return CalibrationRecord{}, err
	}
	var metric, unit string
	err = c.Tx.QueryRow(ctx, `SELECT metric, unit FROM devices.sensors WHERE id = $1 AND device_id = $2`, in.SensorID, in.DeviceID).Scan(&metric, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return CalibrationRecord{}, apperr.Fields(map[string]string{"sensorId": "error.notOnDevice"})
	}
	if err != nil {
		return CalibrationRecord{}, err
	}
	if metric != in.Metric || unit != in.Unit {
		return CalibrationRecord{}, apperr.Fields(map[string]string{"metric": "error.sensorMismatch"})
	}
	if err := m.exclusive(ctx, c, *s.unit); err != nil {
		return CalibrationRecord{}, err
	}
	id := uuid.Must(uuid.NewV7())
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.calibration_records (id, tenant_id, device_id, sensor_id, metric, unit, reference_value, measured_value, calibrated_at, actor_id, job_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, id, in.DeviceID, in.SensorID, in.Metric, in.Unit,
		in.ReferenceValue, in.MeasuredValue, in.CalibratedAt, c.Principal.UserID, in.JobID); err != nil {
		return CalibrationRecord{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE devices.sensors SET calibrated_at = $2 WHERE id = $1`, in.SensorID, in.CalibratedAt); err != nil {
		return CalibrationRecord{}, err
	}
	v, err := bumpDevice(ctx, c, in.DeviceID)
	if err != nil {
		return CalibrationRecord{}, err
	}
	c.Audit(ops.AuditEntry{Action: "devices.calibrate", TargetKind: "device", TargetID: in.DeviceID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v})
	c.Emit(ops.Event{AggregateType: "device", AggregateID: in.DeviceID, Type: "DeviceCalibrated"})
	return m.calibration(ctx, c, id)
}

const calCols = `r.id, r.tenant_id, r.created_at, r.created_at, b.id, b.unit_id, b.customer_org_id, r.device_id, r.sensor_id, r.metric, r.unit,
	r.reference_value, r.measured_value, r.calibrated_at, r.actor_id`

func scanCal(row pgx.Row) (CalibrationRecord, error) {
	var x CalibrationRecord
	err := row.Scan(&x.ID, &x.TenantID, &x.CreatedAt, &x.UpdatedAt, &x.BindingID, &x.UnitIDAtOccurrence, &x.CustomerOrgIDAtOccurrence, &x.DeviceID, &x.SensorID,
		&x.Metric, &x.Unit, &x.ReferenceValue, &x.MeasuredValue, &x.CalibratedAt, &x.ActorID)
	x.Version, x.IsDemo = 1, true
	return x, err
}

// calibration rows carry the binding at their time (SR24).
const calFrom = ` FROM devices.calibration_records r LEFT JOIN devices.device_bindings b ON b.device_id = r.device_id
	AND b.bound_at <= r.created_at AND (b.unbound_at IS NULL OR b.unbound_at > r.created_at)`

func (m *Module) calibration(ctx context.Context, c *ops.Call, id uuid.UUID) (CalibrationRecord, error) {
	return scanCal(c.Tx.QueryRow(ctx, `SELECT `+calCols+calFrom+` WHERE r.id = $1`, id))
}
