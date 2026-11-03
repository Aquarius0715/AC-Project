package devices

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// OperationGuard reports control work that makes a queued operation fail at its start (REV19-018; control.Busy).
type OperationGuard interface {
	OperationConflict(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// AlertOpener opens the Alert of a device fault in the transition of its event (SR23; monitoring.Alerts).
type AlertOpener interface {
	TamperAlert(
		ctx context.Context,
		tx pgx.Tx,
		unit, org uuid.UUID,
		text string,
		at time.Time,
	) (uuid.UUID, error)
}

// operationStart is the delay from creating a check / firmware operation to its start tick (IR67).
const operationStart = time.Second

// setConnection changes the device connection (lastSeenAt too when seen is set) and mirrors both on the bound unit,
// whose connection units.get and units.list report (D07).
func setConnection(
	ctx context.Context,
	txn pgx.Tx,
	device uuid.UUID,
	conn string,
	seen *time.Time,
	now time.Time,
) error {
	var (
		unit *uuid.UUID
		last *time.Time
	)

	scanErr := txn.QueryRow(ctx, `UPDATE devices.devices SET connection = $2, last_seen_at = COALESCE($3, last_seen_at),
		version = version + 1, updated_at = $4
		WHERE id = $1 RETURNING unit_id, last_seen_at`, device, conn, seen, now).
		Scan(&unit, &last)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "update devices.devices")
	}

	if unit == nil {
		return nil
	}

	_, err := txn.Exec(
		ctx,
		`UPDATE assets.units SET connection = $2, last_seen_at = $3 WHERE id = $1`,
		*unit,
		conn,
		last,
	)

	return apperr.Wrap(err, "update assets.units")
}

// Lifecycle moves check and firmware operations on the demo clock (IR67, D05).
type Lifecycle struct {
	Guard OperationGuard
}

// Advance is the equipment scheduler job of device operations. An operation still queued or running at expiresAt
// (60 s after creation) fails with TIMEOUT, and a timed-out check leaves the device in error. A queued operation
// starts at the first tick one second after its creation: the mutual exclusion is checked again (a normal Command
// requested/sent, an active DiagnosticRun or another open operation on the unit fails it with CONFLICT, REV19-018),
// a firmware update of a device that is no longer online fails with OFFLINE (IR94), otherwise it runs and a check
// sets the device connecting. The device version changes only with the device fields.
func (l Lifecycle) Advance(ctx context.Context, call *ops.Call) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE devices.device_operations SET status = 'failed', failure_code = 'TIMEOUT', finished_at = expires_at, version =
			version + 1, updated_at = $1
		WHERE status IN ('queued','running') AND expires_at <= $1 RETURNING id, device_id, kind`,
		call.Now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update devices.device_operations")
	}

	type op struct {
		id, device uuid.UUID
		unit       *uuid.UUID
		kind       string
	}

	expired, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (op, error) {
		var o op

		return o, r.Scan(&o.id, &o.device, &o.kind)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "update devices.device_operations")
	}

	for _, o := range expired {
		if o.kind == "check" {
			err := setConnection(ctx, call.Tx, o.device, "error", nil, call.Now)
			if err != nil {
				return 0, err
			}
		}

		call.Emit(
			ops.Event{
				AggregateType: "device",
				AggregateID:   o.device,
				Type:          "DeviceOperationFinished",
				Payload: map[string]any{
					"operationId": o.id,
					"status":      "failed",
					"failureCode": "TIMEOUT",
				},
			},
		)
	}

	rows, err = call.Tx.Query(
		ctx,
		`SELECT id, device_id, unit_id, kind FROM devices.device_operations WHERE status = 'queued' AND created_at <= $1
		ORDER BY created_at, id FOR UPDATE`,
		call.Now.Add(-operationStart),
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.device_operations")
	}

	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (op, error) {
		var o op

		return o, r.Scan(&o.id, &o.device, &o.unit, &o.kind)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.device_operations")
	}

	for _, o := range due {
		failure, err := l.startFailure(ctx, call, o.id, o.device, o.unit, o.kind)
		if err != nil {
			return 0, err
		}

		if failure != "" {
			_, err := call.Tx.Exec(
				ctx,
				`UPDATE devices.device_operations SET status = 'failed', failure_code = $2, finished_at = $3, version = version + 1,
					updated_at = $3 WHERE id = $1`,
				o.id,
				failure,
				call.Now,
			)
			if err != nil {
				return 0, apperr.Wrap(err, "update devices.device_operations")
			}

			call.Emit(
				ops.Event{
					AggregateType: "device",
					AggregateID:   o.device,
					Type:          "DeviceOperationFinished",
					Payload: map[string]any{
						"operationId": o.id,
						"status":      "failed",
						"failureCode": failure,
					},
				},
			)

			continue
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE devices.device_operations SET status = 'running', started_at = $2, version = version + 1, updated_at = $2
				WHERE id = $1`,
			o.id,
			call.Now,
		)
		if execErr != nil {
			return 0, apperr.Wrap(execErr, "update devices.device_operations")
		}

		if o.kind == "check" {
			err := setConnection(ctx, call.Tx, o.device, "connecting", nil, call.Now)
			if err != nil {
				return 0, err
			}
		}

		call.Emit(
			ops.Event{
				AggregateType: "device",
				AggregateID:   o.device,
				Type:          "DeviceOperationStarted",
				Payload:       map[string]any{"operationId": o.id},
			},
		)
	}

	return len(expired) + len(due), nil
}

// startFailure is the failure code of an operation at its start tick ("" when it may run).
func (l Lifecycle) startFailure(
	ctx context.Context,
	call *ops.Call,
	id, device uuid.UUID,
	unit *uuid.UUID,
	kind string,
) (string, error) {
	if unit != nil {
		var other bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND id <> $2
			AND
			status IN ('queued','running') AND expires_at > $3)`,
			*unit, id, call.Now).
			Scan(&other)
		if err != nil {
			return "", apperr.Wrap(err, "select devices.device_operations")
		}

		if !other && l.Guard != nil {
			busy, err := l.Guard.OperationConflict(ctx, call, *unit)
			if err != nil {
				return "", apperr.Wrap(err, "devices.OperationGuard.OperationConflict")
			}

			other = busy
		}

		if other {
			return string(apperr.Conflict), nil
		}
	}

	if kind == "firmware" {
		var conn string

		err := call.Tx.QueryRow(ctx, `SELECT connection FROM devices.devices WHERE id = $1`, device).
			Scan(&conn)
		if err != nil {
			return "", apperr.Wrap(err, "select devices.devices")
		}

		if conn != "online" {
			return string(apperr.Offline), nil
		}
	}

	return "", nil
}

// FinishOperation applies the device's result to a running operation (IR67, demo.trigger operation): a queued,
// finished or expired operation is CONFLICT. A succeeded check sets the device online (lastSeenAt at the result) and
// a failed one error; a succeeded firmware update installs targetVersion, a failed one (UNAVAILABLE) keeps the
// version and the connection.
func FinishOperation(
	ctx context.Context,
	txn pgx.Tx,
	id uuid.UUID,
	succeeded bool,
	when, now time.Time,
) error {
	var (
		device       uuid.UUID
		kind, status string
		target       *string
		expires      time.Time
	)

	err := txn.QueryRow(ctx, `SELECT device_id, kind, status, target_version, expires_at FROM devices.device_operations
		WHERE id = $1 FOR UPDATE`, id).
		Scan(&device, &kind, &status, &target, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return apperr.Wrap(err, "select devices.device_operations")
	}

	if status != "running" || !expires.After(now) {
		return apperr.E(apperr.Conflict, "errors.operation_not_running")
	}

	result, code := "succeeded", (*string)(nil)

	if !succeeded {
		u := string(apperr.Unavailable)
		result, code = "failed", &u
	}

	_, execErr := txn.Exec(
		ctx,
		`UPDATE devices.device_operations SET status = $2, failure_code = $3, finished_at = $4, version = version + 1,
			updated_at = $5 WHERE id = $1`,
		id,
		result,
		code,
		when,
		now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update devices.device_operations")
	}

	switch {
	case kind == "check" && succeeded:
		return setConnection(ctx, txn, device, "online", &when, now)
	case kind == "check":
		return setConnection(ctx, txn, device, "error", nil, now)
	case succeeded && target != nil:
		_, err = txn.Exec(
			ctx,
			`UPDATE devices.devices SET firmware_version = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
			device,
			*target,
			now,
		)
	}

	return apperr.Wrap(err, "update devices.devices")
}

// Recovery is DeviceRecovery of service-contracts.ts.
type Recovery struct {
	Axis          string    `json:"axis"`
	SourceEventID uuid.UUID `json:"sourceEventId"`
	Value         string    `json:"value"`
}

// Signal is a fault or recovery the device reports (DemoTrigger device branch).
type Signal struct {
	EventID    uuid.UUID
	DeviceID   uuid.UUID
	BindingID  *uuid.UUID
	Sequence   int64
	Kind       string // communication_lost | power_lost | tamper | restored
	Recovery   *Recovery
	OccurredAt time.Time
}

// FaultAxis is the state axis of each fault kind and RecoveredValue the value restored gives back on an axis (SR20);
// each axis has one evidence source (REV18-040).
var (
	FaultAxis = map[string]string{
		"communication_lost": "connection",
		"power_lost":         "power",
		"tamper":             "tamper",
	}
	RecoveredValue = map[string]string{"connection": "online", "power": "on", "tamper": "clear"}
	evidenceOf     = map[string]string{
		"connection": "heartbeat",
		"power":      "power_signal",
		"tamper":     "tamper_signal",
	}
)

func sameID(a, b *uuid.UUID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// RecordSignal stores a device fault or recovery as a DeviceEvent and applies it to the device in one transition
// (SR20, SR23). A repeated eventId with the same content changes nothing, with other content it is CONFLICT; the
// binding must be the device's current one (VALIDATION); a sequence not newer than the latest event of the
// device/binding/axis is an out-of-order report that leaves history and state unchanged (IR102 G1-023). It reports
// whether the event was applied. communication_lost sets the connection offline, power_lost the power signal off and
// tamper the tamper state detected with the unit's tamper Alert in alertIds. restored must name the current
// unresolved fault of its axis (else CONFLICT): the fault gets restoredAt, only that axis recovers, and the event
// inherits the fault's evidence source and alertIds without resolving the Alerts.
func RecordSignal(
	ctx context.Context,
	txn pgx.Tx,
	signal Signal,
	now time.Time,
	alerts AlertOpener,
) (bool, error) {
	axis := FaultAxis[signal.Kind]

	var recovery []byte

	if signal.Kind == "restored" {
		axis = signal.Recovery.Axis
		recovery, _ = json.Marshal(signal.Recovery)
	}

	var seen struct {
		device  uuid.UUID
		kind    string
		binding *uuid.UUID
		seq     int64
		rec     []byte
	}

	err := txn.QueryRow(ctx, `SELECT device_id, event_type, binding_id, sequence, recovery FROM devices.device_events WHERE
		event_id = $1`, signal.EventID).
		Scan(&seen.device, &seen.kind, &seen.binding, &seen.seq, &seen.rec)
	if err == nil {
		var prev *Recovery
		if len(seen.rec) > 0 {
			_ = json.Unmarshal(seen.rec, &prev)
		}

		if seen.device != signal.DeviceID || seen.kind != signal.Kind ||
			!sameID(seen.binding, signal.BindingID) ||
			seen.seq != signal.Sequence ||
			(prev == nil) != (signal.Recovery == nil) ||
			(prev != nil && *prev != *signal.Recovery) {
			return false, apperr.E(apperr.Conflict, "errors.event_id_reused")
		}

		return false, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return false, apperr.Wrap(err, "select devices.device_events")
	}

	var (
		binding, unit *uuid.UUID
		serial        string
	)

	err = txn.QueryRow(ctx, `SELECT binding_id, unit_id, serial FROM devices.devices WHERE id = $1 FOR
		UPDATE`, signal.DeviceID).
		Scan(&binding, &unit, &serial)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return false, apperr.Wrap(err, "select devices.devices")
	}

	if !sameID(binding, signal.BindingID) {
		return false, apperr.Fields(map[string]string{"bindingId": "errors.binding_mismatch"})
	}

	var latest *int64

	scanErr := txn.QueryRow(ctx, `SELECT max(sequence) FROM devices.device_events WHERE device_id = $1 AND binding_id IS
		NOT
		DISTINCT FROM $2 AND axis = $3`,
		signal.DeviceID, signal.BindingID, axis).
		Scan(&latest)
	if scanErr != nil {
		return false, apperr.Wrap(scanErr, "select devices.device_events")
	}

	if latest != nil && signal.Sequence <= *latest {
		return false, nil // an old heartbeat does not restore the device (SR20)
	}

	evidence, alertIDs := evidenceOf[axis], []uuid.UUID{}

	if signal.Kind == "restored" {
		var (
			src      uuid.UUID
			restored *time.Time
		)

		err := txn.QueryRow(ctx, `SELECT id, restored_at, evidence_source, alert_ids FROM devices.device_events WHERE
			device_id
			= $1 AND binding_id IS NOT DISTINCT FROM $2
			AND axis = $3 AND event_type <> 'restored' ORDER BY sequence DESC LIMIT 1 FOR
			UPDATE`, signal.DeviceID, signal.BindingID, axis).
			Scan(&src, &restored, &evidence, &alertIDs)
		if errors.Is(err, pgx.ErrNoRows) ||
			(err == nil && (src != signal.Recovery.SourceEventID || restored != nil)) {
			return false, apperr.E(apperr.Conflict, "errors.recovery_source_not_current")
		}

		if err != nil {
			return false, apperr.Wrap(err, "select devices.device_events")
		}

		_, execErr := txn.Exec(
			ctx,
			`UPDATE devices.device_events SET restored_at = $2, version = version + 1 WHERE id = $1`,
			src,
			signal.OccurredAt,
		)
		if execErr != nil {
			return false, apperr.Wrap(execErr, "update devices.device_events")
		}
	}

	if signal.Kind == "tamper" && unit != nil && binding != nil {
		var org uuid.UUID

		err := txn.QueryRow(ctx, `SELECT customer_org_id FROM devices.device_bindings WHERE id = $1`, *binding).
			Scan(&org)
		if err != nil {
			return false, apperr.Wrap(err, "select devices.device_bindings")
		}

		a, err := alerts.TamperAlert(
			ctx,
			txn,
			*unit,
			org,
			"Device "+serial+" reported its cover opened while powered (tamper_signal)",
			signal.OccurredAt,
		)
		if err != nil {
			return false, apperr.Wrap(err, "devices.AlertOpener.TamperAlert")
		}

		alertIDs = []uuid.UUID{a}
	}

	_, execErr := txn.Exec(
		ctx,
		`INSERT INTO devices.device_events (id, tenant_id, device_id, unit_id, binding_id, event_id, event_type, axis,
			evidence_source, recovery, alert_ids,
		sequence, occurred_at, created_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9,
		$10, $11, $12, $13)`,
		uuid.Must(
			uuid.NewV7(),
		),
		signal.DeviceID,
		unit,
		signal.BindingID,
		signal.EventID,
		signal.Kind,
		axis,
		evidence,
		recovery,
		alertIDs,
		signal.Sequence,
		signal.OccurredAt,
		now,
	)
	if execErr != nil {
		return false, apperr.Wrap(execErr, "insert devices.device_events")
	}

	restored := signal.Kind == "restored"
	switch {
	case axis == "connection" && restored:
		err = setConnection(ctx, txn, signal.DeviceID, "online", &signal.OccurredAt, now)
	case axis == "connection":
		err = setConnection(ctx, txn, signal.DeviceID, "offline", nil, now)
	default:
		col, value := map[string]string{"power": "power_signal", "tamper": "tamper"}[axis], map[string]string{
			"power":  "off",
			"tamper": "detected",
		}[axis]
		if restored {
			value = RecoveredValue[axis]
		}

		_, err = txn.Exec(
			ctx,
			`UPDATE devices.devices SET `+col+` = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
			signal.DeviceID,
			value,
			now,
		)
	}

	return err == nil, apperr.Wrap(err, "update devices.devices")
}
