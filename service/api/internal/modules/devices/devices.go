package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// UnitInfo is what Devices needs from Assets about a unit.
type UnitInfo struct {
	OrgID, PropertyID, ModelID uuid.UUID
	CapabilityVersion          int
}

// Units looks units up in the Assets module.
type Units interface {
	UnitInfo(ctx context.Context, c *ops.Call, unit uuid.UUID) (UnitInfo, bool, error)
}

// TechAccess is the IR94 technician write check (maintenance.Access).
type TechAccess interface {
	TechnicianJob(ctx context.Context, c *ops.Call, jobID uuid.UUID, unit uuid.UUID) error
}

// Sensor is Sensor of service-contracts.ts.
type Sensor struct {
	BoundaryID        *string    `json:"boundaryId"`
	ID                uuid.UUID  `json:"id"`
	DeviceID          uuid.UUID  `json:"deviceId"`
	Metric            string     `json:"metric"`
	Unit              string     `json:"unit"`
	StaleAfterSeconds int        `json:"staleAfterSeconds"`
	CalibratedAt      *time.Time `json:"calibratedAt"`
}

// Device is Device of service-contracts.ts (DeviceDetail adds calibrationRefs and activeOperation).
type Device struct {
	ID                    uuid.UUID  `json:"id"`
	TenantID              uuid.UUID  `json:"tenantId"`
	Version               int        `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	BindingID             *uuid.UUID `json:"bindingId"`
	UnitID                *uuid.UUID `json:"unitId"`
	TargetUnitID          uuid.UUID  `json:"targetUnitId"`
	CreatedByMembershipID uuid.UUID  `json:"createdByMembershipId"`
	Serial                string     `json:"serial"`
	Connection            string     `json:"connection"`
	LastSeenAt            *time.Time `json:"lastSeenAt"`
	FirmwareVersion       string     `json:"firmwareVersion"`
	PowerSignal           string     `json:"powerSignal"`
	Tamper                string     `json:"tamper"`
	Sensors               []Sensor   `json:"sensors"`
}

// DeviceDetail is DeviceDetail of service-contracts.ts.
type DeviceDetail struct {
	Device
	CalibrationRefs []uuid.UUID     `json:"calibrationRefs"`
	ActiveOperation json.RawMessage `json:"activeOperation" swaggertype:"object"`
}

const devCols = `d.id, d.tenant_id, d.version, d.created_at, d.updated_at, d.binding_id, d.unit_id, d.target_unit_id, d.created_by_membership_id,
	d.serial, d.connection, d.last_seen_at, d.firmware_version, d.power_signal, d.tamper`

// Module is the device register.
type Module struct {
	Units      Units
	Access     TechAccess
	UnitAccess TechUnitAccess
	Exclusion  []Exclusion
}

// deviceScope limits devices to the caller (D01): admins the tenant; clients devices bound to their organization;
// everyone else their own unbound registrations (D05) and devices on units in the unitscope (IR169; lists in
// Equipment mode, single reads in List mode followed by unitscope.Gate, IR49(b)).
func deviceScope(c *ops.Call, args *[]any, mode unitscope.Mode) string {
	p := c.Principal
	add := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	switch p.Role {
	case "admin":
		return "TRUE"
	case "client":
		return "EXISTS (SELECT 1 FROM devices.device_bindings b WHERE b.device_id = d.id AND b.unbound_at IS NULL AND b.customer_org_id = " + add(p.OrgID) + ")"
	}
	own := "(d.unit_id IS NULL AND d.created_by_membership_id = " + add(p.MembershipID) + ")"
	return "(" + own + " OR (d.unit_id IS NOT NULL AND " + unitscope.SQL(c, args, "d.unit_id", mode) + "))"
}

func (m *Module) load(ctx context.Context, c *ops.Call, where string, args []any, order string, limit, offset int) ([]Device, int, error) {
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM devices.devices d WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM devices.devices d WHERE %s ORDER BY %s LIMIT %d OFFSET %d", devCols, where, order, limit, offset), args...)
	if err != nil {
		return nil, 0, err
	}
	var out []Device
	for rows.Next() {
		var d Device
		if err := rows.Scan(&d.ID, &d.TenantID, &d.Version, &d.CreatedAt, &d.UpdatedAt, &d.BindingID, &d.UnitID, &d.TargetUnitID, &d.CreatedByMembershipID,
			&d.Serial, &d.Connection, &d.LastSeenAt, &d.FirmwareVersion, &d.PowerSignal, &d.Tamper); err != nil {
			rows.Close()
			return nil, 0, err
		}
		d.Sensors = []Sensor{}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		srows, err := c.Tx.Query(ctx, `SELECT boundary_id, id, device_id, metric, unit, stale_after_seconds, calibrated_at FROM devices.sensors WHERE device_id = $1 ORDER BY metric`, out[i].ID)
		if err != nil {
			return nil, 0, err
		}
		for srows.Next() {
			var s Sensor
			if err := srows.Scan(&s.BoundaryID, &s.ID, &s.DeviceID, &s.Metric, &s.Unit, &s.StaleAfterSeconds, &s.CalibratedAt); err != nil {
				srows.Close()
				return nil, 0, err
			}
			out[i].Sensors = append(out[i].Sensors, s)
		}
		srows.Close()
	}
	return out, total, nil
}

// @Summary		devices.list (read)
// @ID				devices.list
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:device.read | admin:audit.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A04, DD-T02, DD-T11, DD-A16, DD-A20, DD-T12 · Query: filters unitId,status · sort id,createdAt,updatedAt (default id asc)
// @Tags			devices
// @Accept			json
// @Produce		json
// @Param			request	body		paging.Query	true	"input"
// @Success		200		{object}	ops.Envelope{data=DevicePage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/devices.list [post]
func (m *Module) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Device], error) {
	var f struct {
		UnitID *uuid.UUID `json:"unitId,omitempty"`
		Status *string    `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Device]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "d.id", "createdAt": "d.created_at", "updatedAt": "d.updated_at"}, "d.id ASC")
	if err != nil {
		return paging.Page[Device]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Device]{}, err
	}
	var args []any
	conds := []string{deviceScope(c, &args, unitscope.Equipment)}
	if f.UnitID != nil {
		args = append(args, *f.UnitID)
		conds = append(conds, fmt.Sprintf("d.unit_id = $%d", len(args)))
	}
	if f.Status != nil {
		switch *f.Status {
		case "online", "offline", "unknown", "connecting", "error":
		default:
			return paging.Page[Device]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
		}
		args = append(args, *f.Status)
		conds = append(conds, fmt.Sprintf("d.connection = $%d", len(args)))
	}
	items, total, err := m.load(ctx, c, strings.Join(conds, " AND "), args, order, w.Limit, w.Offset)
	if err != nil {
		return paging.Page[Device]{}, err
	}
	if items == nil {
		items = []Device{}
	}
	return paging.Page[Device]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// IDInput is {id}.
type IDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *IDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// @Summary		devices.get (read)
// @ID				devices.get
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:device.read | admin:audit.read; SR24 current-device AND occurrence-scope
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-T11, DD-T12, DD-A04
// @Tags			devices
// @Accept			json
// @Produce		json
// @Param			request	body		IDInput	true	"input"
// @Success		200		{object}	ops.Envelope{data=DeviceDetail}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/devices.get [post]
func (m *Module) get(ctx context.Context, c *ops.Call, in *IDInput) (DeviceDetail, error) {
	args := []any{in.ID}
	d, err := m.detail(ctx, c, "d.id = $1 AND "+deviceScope(c, &args, unitscope.List), args)
	if err != nil || d.UnitID == nil {
		return d, err
	}
	return d, unitscope.Gate(ctx, c, []uuid.UUID{*d.UnitID})
}

// detail loads one device with calibration references and its active operation.
func (m *Module) detail(ctx context.Context, c *ops.Call, where string, args []any) (DeviceDetail, error) {
	items, _, err := m.load(ctx, c, where, args, "d.id", 1, 0)
	if err != nil {
		return DeviceDetail{}, err
	}
	if len(items) == 0 {
		return DeviceDetail{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	d := DeviceDetail{Device: items[0]}
	d.CalibrationRefs = []uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT id FROM devices.calibration_records WHERE device_id = $1 ORDER BY created_at DESC, id`, d.ID)
	if err != nil {
		return DeviceDetail{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return DeviceDetail{}, err
		}
		d.CalibrationRefs = append(d.CalibrationRefs, id)
	}
	rows.Close()
	var op []byte
	err = c.Tx.QueryRow(ctx, `SELECT to_jsonb(o) FROM (SELECT id, tenant_id AS "tenantId", version, created_at AS "createdAt", updated_at AS "updatedAt",
		device_id AS "deviceId", unit_id AS "unitId", job_id AS "jobId", kind, status, target_version AS "targetVersion", failure_code AS "failureCode",
		started_at AS "startedAt", finished_at AS "finishedAt", expires_at AS "expiresAt"
		FROM devices.device_operations WHERE device_id = $1 AND status IN ('queued','running') ORDER BY created_at DESC LIMIT 1) o`, d.ID).Scan(&op)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DeviceDetail{}, err
	}
	d.ActiveOperation = json.RawMessage("null")
	if len(op) > 0 {
		d.ActiveOperation = op
	}
	return d, nil
}

var serialRe = regexp.MustCompile(`^[A-Z0-9-]{3,64}$`)

// RegisterInput is devices.register input.
type RegisterInput struct {
	Serial      string     `json:"serial"`
	SensorTypes []string   `json:"sensorTypes"`
	UnitID      uuid.UUID  `json:"unitId"`
	JobID       *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator (DD-T11: serial 3–64 letters, digits or hyphens after trim + upper-case).
func (in *RegisterInput) Validate() map[string]string {
	fe := map[string]string{}
	in.Serial = strings.ToUpper(strings.TrimSpace(in.Serial))
	if !serialRe.MatchString(in.Serial) {
		fe["serial"] = "error.serialFormat"
	}
	if in.SensorTypes == nil {
		fe["sensorTypes"] = "error.required"
	}
	if !uniqueIn(in.SensorTypes, nil) {
		fe["sensorTypes"] = "error.duplicate"
	}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	return fe
}

// technicianGate applies IR94 to technician writes that carry jobId.
func (m *Module) technicianGate(ctx context.Context, c *ops.Call, jobID *uuid.UUID, unit uuid.UUID) error {
	if c.Principal.Role != "technician" {
		return nil
	}
	if jobID == nil {
		return apperr.Fields(map[string]string{"jobId": "error.required"})
	}
	return m.Access.TechnicianJob(ctx, c, *jobID, unit)
}

// @Summary		devices.register (write)
// @ID				devices.register
// @Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
// @Description	Validation: D01; input constraints in the corresponding DD; IR43 sensorTypes⊆Capability.sensors; IR94 technician write table (assignment and work window, jobId required when typed)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-T11, DD-A04
// @Tags			devices
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		RegisterInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=DeviceDetail}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/devices.register [post]
func (m *Module) register(ctx context.Context, c *ops.Call, in *RegisterInput) (DeviceDetail, error) {
	if err := m.technicianGate(ctx, c, in.JobID, in.UnitID); err != nil {
		return DeviceDetail{}, err
	}
	u, ok, err := m.Units.UnitInfo(ctx, c, in.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}
	if !ok {
		return DeviceDetail{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	var sensorsRaw []byte
	if err := c.Tx.QueryRow(ctx, `SELECT sensors FROM devices.capabilities WHERE id = $1 AND version = $2`, u.ModelID, u.CapabilityVersion).Scan(&sensorsRaw); err != nil {
		return DeviceDetail{}, err
	}
	var specs []SensorSpec
	_ = json.Unmarshal(sensorsRaw, &specs)
	byMetric := map[string]SensorSpec{}
	for _, s := range specs {
		byMetric[s.Metric] = s
	}
	for _, mt := range in.SensorTypes {
		if _, ok := byMetric[mt]; !ok {
			return DeviceDetail{}, apperr.Fields(map[string]string{"sensorTypes": "error.notInCapability"}) // IR43
		}
	}
	var dup bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.devices WHERE upper(serial) = $1)`, in.Serial).Scan(&dup); err != nil {
		return DeviceDetail{}, err
	}
	if dup {
		return DeviceDetail{}, apperr.E(apperr.Conflict, "error.duplicateSerial") // tenant-wide after normalisation (D05)
	}
	// D05: register reserves targetUnitId; unitId stays null until devices.bind
	id := uuid.Must(uuid.NewV7())
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, 'v1')`, id, in.Serial, in.UnitID, c.Principal.MembershipID); err != nil {
		return DeviceDetail{}, err
	}
	for _, mt := range in.SensorTypes {
		s := byMetric[mt]
		if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.sensors (tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`, id, s.Metric, s.Unit, s.BoundaryID, s.StaleAfterSeconds); err != nil {
			return DeviceDetail{}, err
		}
	}
	one := 1
	c.Emit(ops.Event{AggregateType: "device", AggregateID: id, Type: "DeviceRegistered", Payload: map[string]any{"targetUnitId": in.UnitID}})
	c.Audit(ops.AuditEntry{Action: "devices.register", TargetKind: "device", TargetID: id.String(), NextVersion: &one})
	return m.detail(ctx, c, "d.id = $1", []any{id}) // own write result; read scope applies to later reads
}

// RegisterDevices binds the device operations.
func RegisterDevices(r *ops.Registry, m *Module) {
	if m.Access == nil {
		m.Access = maintenance.Access{}
	}
	if m.UnitAccess == nil {
		m.UnitAccess = maintenance.Access{}
	}
	ops.Register(r, "devices.list", m.list)
	ops.Register(r, "devices.get", m.get)
	ops.Register(r, "devices.register", m.register)
	ops.Register(r, "devices.bind", m.bind)
	ops.Register(r, "devices.check", m.check)
	ops.Register(r, "devices.updateFirmware", m.updateFirmware)
	ops.Register(r, "devices.calibrate", m.calibrate)
	ops.Register(r, "devices.operations", m.operations)
	ops.Register(r, "devices.calibrations", m.calibrations)
	ops.Register(r, "devices.events", m.events)
	ops.Register(r, "devices.addResponseNote", m.addResponseNote)
}
