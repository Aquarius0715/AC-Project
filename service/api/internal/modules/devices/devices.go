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
	"github.com/jackc/pgx/v5/pgconn"

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
	MqttNode              *string    `json:"mqttNode"`
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
	d.serial, d.mqtt_node, d.connection, d.last_seen_at, d.firmware_version, d.power_signal, d.tamper`

// The aggregate and audit target kind of a device, its connection states, the tamper state that blocks binding (D05),
// and the roles the device scope tells apart.
const (
	entityDevice     = "device"
	entityCapability = "capability"

	connOnline     = "online"
	connOffline    = "offline"
	connConnecting = "connecting"
	connError      = "error"
	connUnknown    = "unknown"

	tamperDetected = "detected"

	roleAdmin      = "admin"
	roleClient     = "client"
	roleTechnician = "technician"
)

// JSON field names the module writes in more than one place: sort keys, field errors and event payloads.
const (
	fieldAction         = "action"
	fieldCreatedAt      = "createdAt"
	fieldDeviceIDs      = "deviceIds"
	fieldFilters        = "filters"
	fieldOperationID    = "operationId"
	fieldPreviousUnitID = "previousUnitId"
	fieldState          = "state"
	fieldStatus         = "status"
	fieldUnitID         = "unitId"
	fieldUpdatedAt      = "updatedAt"
)

// colCreatedAt is the creation time column that the createdAt sort key orders by (and updatedAt, on rows that never
// change).
const colCreatedAt = "created_at"

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
	case roleAdmin:
		return "TRUE"
	case roleClient:
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

func (*Module) load(
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

	out, err := scanDevices(rows)
	if err != nil {
		return nil, 0, err
	}

	if len(out) == 0 {
		return out, total, nil
	}

	err = withSensors(ctx, call, out)
	if err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// scanDevices reads a page of devices (devCols), each without sensors yet.
func scanDevices(rows pgx.Rows) ([]Device, error) {
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
			&device.MqttNode,
			&device.Connection,
			&device.LastSeenAt,
			&device.FirmwareVersion,
			&device.PowerSignal,
			&device.Tamper,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select devices.devices")
		}

		device.Sensors = []Sensor{}
		out = append(out, device)
	}

	rows.Close()

	return out, apperr.Wrap(rows.Err(), "select devices.devices")
}

// withSensors reads the sensors of the whole page in one query, not one per device (IR497).
func withSensors(ctx context.Context, call *ops.Call, devices []Device) error {
	ids, indexOf := make([]uuid.UUID, len(devices)), make(map[uuid.UUID]int, len(devices))
	for i := range devices {
		ids[i], indexOf[devices[i].ID] = devices[i].ID, i
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT boundary_id, id, device_id, metric, unit, stale_after_seconds, calibrated_at FROM devices.sensors
		WHERE device_id = ANY($1) ORDER BY device_id, metric`,
		ids,
	)
	if err != nil {
		return apperr.Wrap(err, "select devices.sensors")
	}

	for rows.Next() {
		var sensor Sensor

		err := rows.Scan(
			&sensor.BoundaryID,
			&sensor.ID,
			&sensor.DeviceID,
			&sensor.Metric,
			&sensor.Unit,
			&sensor.StaleAfterSeconds,
			&sensor.CalibratedAt,
		)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select devices.sensors")
		}

		if i, ok := indexOf[sensor.DeviceID]; ok {
			devices[i].Sensors = append(devices[i].Sensors, sensor)
		}
	}

	rows.Close()

	return apperr.Wrap(rows.Err(), "select devices.sensors")
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
	var filter deviceFilter

	err := decodeFilters(query, &filter)
	if err != nil {
		return paging.Page[Device]{}, err
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "d.id",
			fieldCreatedAt: "d.created_at",
			fieldUpdatedAt: "d.updated_at",
		},
		"d.id ASC",
	)
	if err != nil {
		return paging.Page[Device]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filter, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Device]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args, err := deviceConditions(call, filter)
	if err != nil {
		return paging.Page[Device]{}, err
	}

	items, total, err := m.load(
		ctx,
		call,
		where,
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

// deviceFilter is the devices.list filters of the query catalog.
type deviceFilter struct {
	UnitID *uuid.UUID `json:"unitId,omitempty"`
	Status *string    `json:"status,omitempty"`
}

// decodeFilters reads the query's filters into filter, a struct of the catalogued keys: another key is VALIDATION.
func decodeFilters(query *paging.Query, filter any) error {
	if len(query.Filters) == 0 {
		return nil
	}

	dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
	dec.DisallowUnknownFields()

	if dec.Decode(filter) != nil {
		return apperr.Fields(
			map[string]string{fieldFilters: apperr.KeyInvalid},
		)
	}

	return nil
}

// deviceConditions is the devices.list WHERE clause and its arguments: the caller's device scope and the filters (a
// status that is no connection state is VALIDATION).
func deviceConditions(call *ops.Call, filter deviceFilter) (string, []any, error) {
	var args []any

	conds := []string{deviceScope(call, &args, unitscope.Equipment)}
	if filter.UnitID != nil {
		args = append(args, *filter.UnitID)
		conds = append(conds, fmt.Sprintf("d.unit_id = $%d", len(args)))
	}

	if filter.Status != nil {
		switch *filter.Status {
		case connOnline, connOffline, connUnknown, connConnecting, connError:
		default:
			return "", nil, apperr.Fields(
				map[string]string{"filters.status": apperr.KeyInvalid},
			)
		}

		args = append(args, *filter.Status)
		conds = append(conds, fmt.Sprintf("d.connection = $%d", len(args)))
	}

	return strings.Join(conds, " AND "), args, nil
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

	deviceDetail.CalibrationRefs, err = calibrationRefs(ctx, call, deviceDetail.ID)
	if err != nil {
		return DeviceDetail{}, err
	}

	deviceDetail.ActiveOperation, err = activeOperation(ctx, call, deviceDetail.ID)
	if err != nil {
		return DeviceDetail{}, err
	}

	return deviceDetail, nil
}

// calibrationRefs lists the device's calibration records, the newest first.
func calibrationRefs(ctx context.Context, call *ops.Call, device uuid.UUID) ([]uuid.UUID, error) {
	refs := []uuid.UUID{}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM devices.calibration_records WHERE device_id = $1 ORDER BY created_at DESC, id`,
		device,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select devices.calibration_records")
	}

	for rows.Next() {
		var recordID uuid.UUID

		err := rows.Scan(&recordID)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select devices.calibration_records")
		}

		refs = append(refs, recordID)
	}

	rows.Close()

	return refs, apperr.Wrap(rows.Err(), "select devices.calibration_records")
}

// activeOperation is the device's queued or running operation as DeviceOperation JSON, null when it has none.
func activeOperation(
	ctx context.Context,
	call *ops.Call,
	device uuid.UUID,
) (json.RawMessage, error) {
	var operationJSON []byte

	err := call.Tx.QueryRow(ctx, `SELECT to_jsonb(o) FROM (SELECT id, tenant_id AS "tenantId", version, created_at AS
		"createdAt", updated_at AS "updatedAt",
		device_id AS "deviceId", unit_id AS "unitId", job_id AS "jobId", kind, status, target_version AS "targetVersion",
		failure_code AS "failureCode",
		started_at AS "startedAt", finished_at AS "finishedAt", expires_at AS "expiresAt"
		FROM devices.device_operations WHERE device_id = $1 AND status IN ('queued','running') ORDER BY created_at DESC LIMIT
		1) o`, device).
		Scan(&operationJSON)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.Wrap(err, "select devices.device_operations")
	}

	if len(operationJSON) > 0 {
		return operationJSON, nil
	}

	return json.RawMessage("null"), nil
}

var (
	serialRe = regexp.MustCompile(`^[A-Z0-9-]{3,64}$`)
	// mqttNodeRe is a node_id as the firmware sends it (device-firmware.md §2): letters of either case, digits, hyphen
	// and underscore, matched to the topic exactly (IR635).
	mqttNodeRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// errMqttNodeFormat is the field error of a node_id that is not 1–64 letters, digits, hyphens or underscores.
const errMqttNodeFormat = "error.mqttNodeFormat"

// errDuplicateMqttNode refuses a node_id that another device of the tenant has.
const errDuplicateMqttNode = "error.duplicateMqttNode"

// dupDevice maps the refusal of the unique indexes devices_serial and devices_mqtt_node, which a device saved at the
// same moment as the check before the write (serialFree, mqttNodeFree) causes, to the conflict that check answers.
func dupDevice(err error) error {
	pgErr, matched := errors.AsType[*pgconn.PgError](err)
	if !matched || pgErr.Code != "23505" {
		return err
	}

	switch pgErr.ConstraintName {
	case "devices_serial":
		return apperr.E(apperr.Conflict, apperr.KeyDuplicateSerial)
	case "devices_mqtt_node":
		return apperr.E(apperr.Conflict, errDuplicateMqttNode)
	}

	return err
}

// normalizeMqttNode trims a node_id and makes the empty one absent; ok is false for a malformed one.
func normalizeMqttNode(raw *string) (*string, bool) {
	if raw == nil {
		return nil, true
	}

	node := strings.TrimSpace(*raw)
	if node == "" {
		return nil, true
	}

	return &node, mqttNodeRe.MatchString(node)
}

// RegisterInput is devices.register input.
type RegisterInput struct {
	Serial      string     `json:"serial"`
	SensorTypes []string   `json:"sensorTypes"`
	UnitID      uuid.UUID  `json:"unitId"`
	JobID       *uuid.UUID `json:"jobId,omitempty"`
	// MqttNode is the node_id the device's ESP32 sends as the <node> of smarthome/ac/<node>/… (IR635); the IoT bridge
	// takes its readings and sends its commands by it once the device is bound.
	MqttNode *string `json:"mqttNode,omitempty"`
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

	node, ok := normalizeMqttNode(in.MqttNode)
	if !ok {
		fieldErrs["mqttNode"] = errMqttNodeFormat
	}

	in.MqttNode = node

	return fieldErrs
}

// technicianGate applies IR94 to technician writes that carry jobId.
func (m *Module) technicianGate(
	ctx context.Context,
	call *ops.Call,
	jobID *uuid.UUID,
	unit uuid.UUID,
) error {
	if call.Principal.Role != roleTechnician {
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

// existingUnit is what Assets knows of a unit that must exist (NOT_FOUND).
func (m *Module) existingUnit(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (UnitInfo, error) {
	unitInfo, found, err := m.Units.UnitInfo(ctx, call, unit)
	if err != nil {
		return UnitInfo{}, apperr.Wrap(err, "devices.Units.UnitInfo")
	}

	if !found {
		return UnitInfo{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return unitInfo, nil
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

	unitInfo, err := m.existingUnit(ctx, call, input.UnitID)
	if err != nil {
		return DeviceDetail{}, err
	}

	byMetric, err := capabilitySensors(ctx, call, unitInfo, input.SensorTypes)
	if err != nil {
		return DeviceDetail{}, err
	}

	err = serialFree(ctx, call, input.Serial)
	if err != nil {
		return DeviceDetail{}, err
	}

	err = mqttNodeFree(ctx, call, input.MqttNode, uuid.Nil)
	if err != nil {
		return DeviceDetail{}, err
	}

	deviceID := uuid.Must(uuid.NewV7())

	err = insertDevice(ctx, call, deviceID, input, byMetric)
	if err != nil {
		return DeviceDetail{}, err
	}

	one := 1

	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   deviceID,
			Type:          "DeviceRegistered",
			Payload:       map[string]any{"targetUnitId": input.UnitID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "devices.register",
			TargetKind:      entityDevice,
			TargetID:        deviceID.String(),
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
		[]any{deviceID},
	) // own write result; read scope applies to later reads
}

// capabilitySensors is the sensor specs of the unit's model version by metric; each requested sensor type must be one
// of them (IR43).
func capabilitySensors(
	ctx context.Context,
	call *ops.Call,
	unitInfo UnitInfo,
	sensorTypes []string,
) (map[string]SensorSpec, error) {
	var sensorsRaw []byte

	scanErr := call.Tx.QueryRow(ctx, `SELECT sensors FROM devices.capabilities WHERE id = $1 AND version =
		$2`, unitInfo.ModelID, unitInfo.CapabilityVersion).
		Scan(&sensorsRaw)
	if scanErr != nil {
		return nil, apperr.Wrap(scanErr, "select devices.capabilities")
	}

	var specs []SensorSpec

	_ = json.Unmarshal(sensorsRaw, &specs)

	byMetric := map[string]SensorSpec{}
	for _, s := range specs {
		byMetric[s.Metric] = s
	}

	for _, mt := range sensorTypes {
		if _, ok := byMetric[mt]; !ok {
			return nil, apperr.Fields(
				map[string]string{"sensorTypes": "error.notInCapability"},
			) // IR43
		}
	}

	return byMetric, nil
}

// serialFree refuses a serial that another device has, tenant-wide after normalisation (D05).
func serialFree(ctx context.Context, call *ops.Call, serial string) error {
	var dup bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.devices WHERE upper(serial) =
		$1)`, serial).
		Scan(&dup)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select devices.devices")
	}

	if dup {
		return apperr.E(
			apperr.Conflict,
			apperr.KeyDuplicateSerial,
		) // tenant-wide after normalisation (D05)
	}

	return nil
}

// mqttNodeFree refuses a node_id that a device other than `self` has (tenant-wide, exact match).
func mqttNodeFree(ctx context.Context, call *ops.Call, node *string, self uuid.UUID) error {
	if node == nil {
		return nil
	}

	var taken bool

	scanErr := call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM devices.devices WHERE mqtt_node = $1 AND id <> $2)`,
		*node,
		self,
	).Scan(&taken)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select devices.devices")
	}

	if taken {
		return apperr.E(apperr.Conflict, errDuplicateMqttNode)
	}

	return nil
}

// insertDevice registers the device with one sensor per requested type, as its model version specifies it. D05:
// register reserves targetUnitId; unitId stays null until devices.bind.
func insertDevice(
	ctx context.Context,
	call *ops.Call,
	device uuid.UUID,
	input *RegisterInput,
	byMetric map[string]SensorSpec,
) error {
	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version,
			mqtt_node)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, 'v1', $5)`,
		device,
		input.Serial,
		input.UnitID,
		call.Principal.MembershipID,
		input.MqttNode,
	)
	if execErr != nil {
		return apperr.Wrap(dupDevice(execErr), "insert devices.devices")
	}

	for _, mt := range input.SensorTypes {
		sensorSpec := byMetric[mt]

		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO devices.sensors (tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
			device,
			sensorSpec.Metric,
			sensorSpec.Unit,
			sensorSpec.BoundaryID,
			sensorSpec.StaleAfterSeconds,
		)
		if err != nil {
			return apperr.Wrap(err, "insert devices.sensors")
		}
	}

	return nil
}

// RegisterDevices binds the device operations.
func RegisterDevices(reg *ops.Registry, module *Module) {
	if module.Access == nil {
		module.Access = maintenance.Access{}
	}

	if module.UnitAccess == nil {
		module.UnitAccess = maintenance.Access{}
	}

	ops.Register(reg, "devices.list", module.list)
	ops.Register(reg, "devices.get", module.get)
	ops.Register(reg, "devices.register", module.register)
	ops.Register(reg, opBind, module.bind)
	ops.Register(reg, "devices.unbind", module.unbind)
	ops.Register(reg, "devices.delete", module.deleteDevice)
	ops.Register(reg, opSetMqttNode, module.setMqttNode)
	ops.Register(reg, "devices.check", module.check)
	ops.Register(reg, "devices.updateFirmware", module.updateFirmware)
	ops.Register(reg, "devices.calibrate", module.calibrate)
	ops.Register(reg, "devices.operations", module.operations)
	ops.Register(reg, "devices.calibrations", module.calibrations)
	ops.Register(reg, "devices.events", module.events)
	ops.Register(reg, "devices.addResponseNote", module.addResponseNote)
}
