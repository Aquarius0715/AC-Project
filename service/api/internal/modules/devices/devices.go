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

const devCols = `d.id, d.tenant_id, d.version, d.created_at, d.updated_at, d.binding_id, d.unit_id, d.target_unit_id,` +
	` d.created_by_membership_id,
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
func deviceScope(call *ops.Call, args *[]any, mode unitscope.Mode) string {
	principal := call.Principal
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	switch principal.Role {
	case "admin":
		return "TRUE"
	case "client":
		return "EXISTS (SELECT 1 FROM devices.device_bindings b WHERE b.device_id = d.id AND b.unbound_at IS NULL AND " +
			"b.customer_org_id = " + add(
			principal.OrgID,
		) + ")"
	}

	own := "(d.unit_id IS NULL AND d.created_by_membership_id = " + add(
		principal.MembershipID,
	) + ")"

	return "(" + own + " OR (d.unit_id IS NOT NULL AND " + unitscope.SQL(
		call,
		args,
		"d.unit_id",
		mode,
	) + "))"
}

func (m *Module) load(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	order string,
	limit, offset int,
) ([]Device, int, error) {
	var total int

	err := call.Tx.QueryRow(ctx, "SELECT count(*) FROM devices.devices d WHERE "+where, args...).
		Scan(&total)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select devices.devices")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM devices.devices d WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			devCols,
			where,
			order,
			limit,
			offset,
		),
		args...)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select devices.devices")
	}

	var out []Device

	for rows.Next() {
		var device Device

		err := rows.Scan(
			&device.ID,
			&device.TenantID,
			&device.Version,
			&device.CreatedAt,
			&device.UpdatedAt,
			&device.BindingID,
			&device.UnitID,
			&device.TargetUnitID,
			&device.CreatedByMembershipID,
			&device.Serial,
			&device.Connection,
			&device.LastSeenAt,
			&device.FirmwareVersion,
			&device.PowerSignal,
			&device.Tamper,
		)
		if err != nil {
			rows.Close()

			return nil, 0, apperr.Wrap(err, "select devices.devices")
		}

		device.Sensors = []Sensor{}
		out = append(out, device)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, 0, apperr.Wrap(rowsErr, "select devices.devices")
	}

	if len(out) == 0 {
		return out, total, nil
	}
	// the sensors of the whole page in one query, not one per device (IR497)
	ids, at := make([]uuid.UUID, len(out)), make(map[uuid.UUID]int, len(out))
	for i := range out {
		ids[i], at[out[i].ID] = out[i].ID, i
	}

	srows, err := call.Tx.Query(
		ctx,
		`SELECT boundary_id, id, device_id, metric, unit, stale_after_seconds, calibrated_at FROM devices.sensors
		WHERE device_id = ANY($1) ORDER BY device_id, metric`,
		ids,
	)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select devices.sensors")
	}

	for srows.Next() {
		var sensor Sensor

		err := srows.Scan(
			&sensor.BoundaryID,
			&sensor.ID,
			&sensor.DeviceID,
			&sensor.Metric,
			&sensor.Unit,
			&sensor.StaleAfterSeconds,
			&sensor.CalibratedAt,
		)
		if err != nil {
			srows.Close()

			return nil, 0, apperr.Wrap(err, "select devices.sensors")
		}

		if i, ok := at[sensor.DeviceID]; ok {
			out[i].Sensors = append(out[i].Sensors, sensor)
		}
	}

	srows.Close()

	rowsErr = srows.Err()
	if rowsErr != nil {
		return nil, 0, apperr.Wrap(rowsErr, "select devices.sensors")
	}

	return out, total, nil
}

// list serves devices.list.
//
//	@Summary		devices.list (read)
//	@ID				devices.list
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:device.read | admin:audit.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A04, DD-T02, DD-T11, DD-A16, DD-A20, DD-T12 · Query: filters unitId,status · sort \
//	@Description	id,createdAt,updatedAt (default id asc)
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			unitId	query		string	false	"filter → unitId"
//	@Param			status	query		string	false	"filter → connection"
//	@Success		200		{object}	ops.Envelope{data=DevicePage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices [get]
func (m *Module) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Device], error) {
	var f struct {
		UnitID *uuid.UUID `json:"unitId,omitempty"`
		Status *string    `json:"status,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil {
			return paging.Page[Device]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "d.id", "createdAt": "d.created_at", "updatedAt": "d.updated_at"},
		"d.id ASC",
	)
	if err != nil {
		return paging.Page[Device]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Device]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	conds := []string{deviceScope(call, &args, unitscope.Equipment)}
	if f.UnitID != nil {
		args = append(args, *f.UnitID)
		conds = append(conds, fmt.Sprintf("d.unit_id = $%d", len(args)))
	}

	if f.Status != nil {
		switch *f.Status {
		case "online", "offline", "unknown", "connecting", "error":
		default:
			return paging.Page[Device]{}, apperr.Fields(
				map[string]string{"filters.status": apperr.KeyInvalid},
			)
		}

		args = append(args, *f.Status)
		conds = append(conds, fmt.Sprintf("d.connection = $%d", len(args)))
	}

	items, total, err := m.load(
		ctx,
		call,
		strings.Join(conds, " AND "),
		args,
		order,
		window.Limit,
		window.Offset,
	)
	if err != nil {
		return paging.Page[Device]{}, err
	}

	if items == nil {
		items = []Device{}
	}

	return paging.Page[Device]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// IDInput is {id}.
type IDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *IDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// get serves devices.get.
//
//	@Summary		devices.get (read)
//	@ID				devices.get
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:device.read | admin:audit.read; SR24 current-device AND occurrence-scope
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T11, DD-T12, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=DeviceDetail}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices/{id} [get]
func (m *Module) get(ctx context.Context, call *ops.Call, in *IDInput) (DeviceDetail, error) {
	args := []any{in.ID}

	d, err := m.detail(ctx, call, "d.id = $1 AND "+deviceScope(call, &args, unitscope.List), args)
	if err != nil || d.UnitID == nil {
		return d, err
	}

	return d, apperr.Wrap(unitscope.Gate(ctx, call, []uuid.UUID{*d.UnitID}), "unitscope.Gate")
}

// detail loads one device with calibration references and its active operation.
func (m *Module) detail(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
) (DeviceDetail, error) {
	items, _, err := m.load(ctx, call, where, args, "d.id", 1, 0)
	if err != nil {
		return DeviceDetail{}, err
	}

	if len(items) == 0 {
		return DeviceDetail{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	deviceDetail := DeviceDetail{Device: items[0], CalibrationRefs: nil, ActiveOperation: nil}
	deviceDetail.CalibrationRefs = []uuid.UUID{}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM devices.calibration_records WHERE device_id = $1 ORDER BY created_at DESC, id`,
		deviceDetail.ID,
	)
	if err != nil {
		return DeviceDetail{}, apperr.Wrap(err, "select devices.calibration_records")
	}

	for rows.Next() {
		var id uuid.UUID

		err := rows.Scan(&id)
		if err != nil {
			rows.Close()

			return DeviceDetail{}, apperr.Wrap(err, "select devices.calibration_records")
		}

		deviceDetail.CalibrationRefs = append(deviceDetail.CalibrationRefs, id)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return DeviceDetail{}, apperr.Wrap(rowsErr, "select devices.calibration_records")
	}

	var op []byte

	err = call.Tx.QueryRow(ctx, `SELECT to_jsonb(o) FROM (SELECT id, tenant_id AS "tenantId", version, created_at AS
		"createdAt", updated_at AS "updatedAt",
		device_id AS "deviceId", unit_id AS "unitId", job_id AS "jobId", kind, status, target_version AS "targetVersion",
		failure_code AS "failureCode",
		started_at AS "startedAt", finished_at AS "finishedAt", expires_at AS "expiresAt"
		FROM devices.device_operations WHERE device_id = $1 AND status IN ('queued','running') ORDER BY created_at DESC LIMIT
		1) o`, deviceDetail.ID).
		Scan(&op)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return DeviceDetail{}, apperr.Wrap(err, "select devices.device_operations")
	}

	deviceDetail.ActiveOperation = json.RawMessage("null")
	if len(op) > 0 {
		deviceDetail.ActiveOperation = op
	}

	return deviceDetail, nil
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
	fieldErrs := map[string]string{}

	in.Serial = strings.ToUpper(strings.TrimSpace(in.Serial))
	if !serialRe.MatchString(in.Serial) {
		fieldErrs["serial"] = "error.serialFormat"
	}

	if in.SensorTypes == nil {
		fieldErrs["sensorTypes"] = apperr.KeyRequired
	}

	if !uniqueIn(in.SensorTypes, nil) {
		fieldErrs["sensorTypes"] = apperr.KeyDuplicate
	}

	if in.UnitID == uuid.Nil {
		fieldErrs["unitId"] = apperr.KeyRequired
	}

	return fieldErrs
}

// technicianGate applies IR94 to technician writes that carry jobId.
func (m *Module) technicianGate(
	ctx context.Context,
	call *ops.Call,
	jobID *uuid.UUID,
	unit uuid.UUID,
) error {
	if call.Principal.Role != "technician" {
		return nil
	}

	if jobID == nil {
		return apperr.Fields(map[string]string{"jobId": apperr.KeyRequired})
	}

	return apperr.Wrap(
		m.Access.TechnicianJob(ctx, call, *jobID, unit),
		"devices.TechAccess.TechnicianJob",
	)
}

// register serves devices.register.
//
//	@Summary		devices.register (write)
//	@ID				devices.register
//	@Description	Authorization: technician:device.maintain:assigned-valid-job | admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; IR43 sensorTypes⊆Capability.sensors; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T11, DD-A04
//	@Tags			devices
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		RegisterInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Device}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/devices [post]
func (m *Module) register(ctx context.Context, c *ops.Call, in *RegisterInput) (Device, error) {
	d, err := m.registerDetail(ctx, c, in)

	return d.Device, err
}

// registerDetail is register answering the device with its detail (IR392: the write itself answers Device).
func (m *Module) registerDetail(
	ctx context.Context,
	call *ops.Call,
	input *RegisterInput,
) (DeviceDetail, error) {
	err := m.technicianGate(ctx, call, input.JobID, input.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}

	unitInfo, ok, err := m.Units.UnitInfo(ctx, call, input.UnitID)
	if err != nil {
		return DeviceDetail{}, apperr.Wrap(err, "devices.Units.UnitInfo")
	}

	if !ok {
		return DeviceDetail{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	var sensorsRaw []byte

	scanErr := call.Tx.QueryRow(ctx, `SELECT sensors FROM devices.capabilities WHERE id = $1 AND version =
		$2`, unitInfo.ModelID, unitInfo.CapabilityVersion).
		Scan(&sensorsRaw)
	if scanErr != nil {
		return DeviceDetail{}, apperr.Wrap(scanErr, "select devices.capabilities")
	}

	var specs []SensorSpec

	_ = json.Unmarshal(sensorsRaw, &specs)

	byMetric := map[string]SensorSpec{}
	for _, s := range specs {
		byMetric[s.Metric] = s
	}

	for _, mt := range input.SensorTypes {
		if _, ok := byMetric[mt]; !ok {
			return DeviceDetail{}, apperr.Fields(
				map[string]string{"sensorTypes": "error.notInCapability"},
			) // IR43
		}
	}

	var dup bool

	scanErr = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.devices WHERE upper(serial) =
		$1)`, input.Serial).
		Scan(&dup)
	if scanErr != nil {
		return DeviceDetail{}, apperr.Wrap(scanErr, "select devices.devices")
	}

	if dup {
		return DeviceDetail{}, apperr.E(
			apperr.Conflict,
			apperr.KeyDuplicateSerial,
		) // tenant-wide after normalisation (D05)
	}
	// D05: register reserves targetUnitId; unitId stays null until devices.bind
	id := uuid.Must(uuid.NewV7())

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, 'v1')`,
		id,
		input.Serial,
		input.UnitID,
		call.Principal.MembershipID,
	)
	if execErr != nil {
		return DeviceDetail{}, apperr.Wrap(execErr, "insert devices.devices")
	}

	for _, mt := range input.SensorTypes {
		sensorSpec := byMetric[mt]

		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO devices.sensors (tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
			id,
			sensorSpec.Metric,
			sensorSpec.Unit,
			sensorSpec.BoundaryID,
			sensorSpec.StaleAfterSeconds,
		)
		if err != nil {
			return DeviceDetail{}, apperr.Wrap(err, "insert devices.sensors")
		}
	}

	one := 1

	call.Emit(
		ops.Event{
			AggregateType: "device",
			AggregateID:   id,
			Type:          "DeviceRegistered",
			Payload:       map[string]any{"targetUnitId": input.UnitID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "devices.register",
			TargetKind:      "device",
			TargetID:        id.String(),
			NextVersion:     &one,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return m.detail(
		ctx,
		call,
		"d.id = $1",
		[]any{id},
	) // own write result; read scope applies to later reads
}

// RegisterDevices binds the device operations.
func RegisterDevices(r *ops.Registry, module *Module) {
	if module.Access == nil {
		module.Access = maintenance.Access{}
	}

	if module.UnitAccess == nil {
		module.UnitAccess = maintenance.Access{}
	}

	ops.Register(r, "devices.list", module.list)
	ops.Register(r, "devices.get", module.get)
	ops.Register(r, "devices.register", module.register)
	ops.Register(r, "devices.bind", module.bind)
	ops.Register(r, "devices.check", module.check)
	ops.Register(r, "devices.updateFirmware", module.updateFirmware)
	ops.Register(r, "devices.calibrate", module.calibrate)
	ops.Register(r, "devices.operations", module.operations)
	ops.Register(r, "devices.calibrations", module.calibrations)
	ops.Register(r, "devices.events", module.events)
	ops.Register(r, "devices.addResponseNote", module.addResponseNote)
}
