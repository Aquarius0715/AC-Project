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
	TamperAlert(ctx context.Context, tx pgx.Tx, unit, org uuid.UUID, text string, at time.Time) (uuid.UUID, error)
}

// operationStart is the delay from creating a check / firmware operation to its start tick (IR67).
const operationStart = time.Second

// setConnection changes the device connection (lastSeenAt too when seen is set) and mirrors both on the bound unit,
// whose connection units.get and units.list report (D07).
func setConnection(ctx context.Context, tx pgx.Tx, device uuid.UUID, conn string, seen *time.Time, now time.Time) error {
	var unit *uuid.UUID
	var last *time.Time
	if err := tx.QueryRow(ctx, `UPDATE devices.devices SET connection = $2, last_seen_at = COALESCE($3, last_seen_at), version = version + 1, updated_at = $4
		WHERE id = $1 RETURNING unit_id, last_seen_at`, device, conn, seen, now).Scan(&unit, &last); err != nil {
		return err
	}
	if unit == nil {
		return nil
	}
	_, err := tx.Exec(ctx, `UPDATE assets.units SET connection = $2, last_seen_at = $3 WHERE id = $1`, *unit, conn, last)
	return err
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
func (l Lifecycle) Advance(ctx context.Context, c *ops.Call) (int, error) {
	rows, err := c.Tx.Query(ctx, `UPDATE devices.device_operations SET status = 'failed', failure_code = 'TIMEOUT', finished_at = expires_at, version = version + 1, updated_at = $1
		WHERE status IN ('queued','running') AND expires_at <= $1 RETURNING id, device_id, kind`, c.Now)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	for _, o := range expired {
		if o.kind == "check" {
			if err := setConnection(ctx, c.Tx, o.device, "error", nil, c.Now); err != nil {
				return 0, err
			}
		}
		c.Emit(ops.Event{AggregateType: "device", AggregateID: o.device, Type: "DeviceOperationFinished", Payload: map[string]any{"operationId": o.id, "status": "failed", "failureCode": "TIMEOUT"}})
	}
	rows, err = c.Tx.Query(ctx, `SELECT id, device_id, unit_id, kind FROM devices.device_operations WHERE status = 'queued' AND created_at <= $1
		ORDER BY created_at, id FOR UPDATE`, c.Now.Add(-operationStart))
	if err != nil {
		return 0, err
	}
	due, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (op, error) {
		var o op
		return o, r.Scan(&o.id, &o.device, &o.unit, &o.kind)
	})
	if err != nil {
		return 0, err
	}
	for _, o := range due {
		failure, err := l.startFailure(ctx, c, o.id, o.device, o.unit, o.kind)
		if err != nil {
			return 0, err
		}
		if failure != "" {
			if _, err := c.Tx.Exec(ctx, `UPDATE devices.device_operations SET status = 'failed', failure_code = $2, finished_at = $3, version = version + 1, updated_at = $3 WHERE id = $1`,
				o.id, failure, c.Now); err != nil {
				return 0, err
			}
			c.Emit(ops.Event{AggregateType: "device", AggregateID: o.device, Type: "DeviceOperationFinished", Payload: map[string]any{"operationId": o.id, "status": "failed", "failureCode": failure}})
			continue
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.device_operations SET status = 'running', started_at = $2, version = version + 1, updated_at = $2 WHERE id = $1`, o.id, c.Now); err != nil {
			return 0, err
		}
		if o.kind == "check" {
			if err := setConnection(ctx, c.Tx, o.device, "connecting", nil, c.Now); err != nil {
				return 0, err
			}
		}
		c.Emit(ops.Event{AggregateType: "device", AggregateID: o.device, Type: "DeviceOperationStarted", Payload: map[string]any{"operationId": o.id}})
	}
	return len(expired) + len(due), nil
}

// startFailure is the failure code of an operation at its start tick ("" when it may run).
func (l Lifecycle) startFailure(ctx context.Context, c *ops.Call, id, device uuid.UUID, unit *uuid.UUID, kind string) (string, error) {
	if unit != nil {
		var other bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND id <> $2 AND status IN ('queued','running') AND expires_at > $3)`,
			*unit, id, c.Now).Scan(&other); err != nil {
			return "", err
		}
		if !other && l.Guard != nil {
			busy, err := l.Guard.OperationConflict(ctx, c, *unit)
			if err != nil {
				return "", err
			}
			other = busy
		}
		if other {
			return string(apperr.Conflict), nil
		}
	}
	if kind == "firmware" {
		var conn string
		if err := c.Tx.QueryRow(ctx, `SELECT connection FROM devices.devices WHERE id = $1`, device).Scan(&conn); err != nil {
			return "", err
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
func FinishOperation(ctx context.Context, tx pgx.Tx, id uuid.UUID, succeeded bool, at, now time.Time) error {
	var device uuid.UUID
	var kind, status string
	var target *string
	var expires time.Time
	err := tx.QueryRow(ctx, `SELECT device_id, kind, status, target_version, expires_at FROM devices.device_operations WHERE id = $1 FOR UPDATE`, id).
		Scan(&device, &kind, &status, &target, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return err
	}
	if status != "running" || !expires.After(now) {
		return apperr.E(apperr.Conflict, "errors.operation_not_running")
	}
	result, code := "succeeded", (*string)(nil)
	if !succeeded {
		u := string(apperr.Unavailable)
		result, code = "failed", &u
	}
	if _, err := tx.Exec(ctx, `UPDATE devices.device_operations SET status = $2, failure_code = $3, finished_at = $4, version = version + 1, updated_at = $5 WHERE id = $1`,
		id, result, code, at, now); err != nil {
		return err
	}
	switch {
	case kind == "check" && succeeded:
		return setConnection(ctx, tx, device, "online", &at, now)
	case kind == "check":
		return setConnection(ctx, tx, device, "error", nil, now)
	case succeeded && target != nil:
		_, err = tx.Exec(ctx, `UPDATE devices.devices SET firmware_version = $2, version = version + 1, updated_at = $3 WHERE id = $1`, device, *target, now)
	}
	return err
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
	FaultAxis      = map[string]string{"communication_lost": "connection", "power_lost": "power", "tamper": "tamper"}
	RecoveredValue = map[string]string{"connection": "online", "power": "on", "tamper": "clear"}
	evidenceOf     = map[string]string{"connection": "heartbeat", "power": "power_signal", "tamper": "tamper_signal"}
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
func RecordSignal(ctx context.Context, tx pgx.Tx, s Signal, now time.Time, alerts AlertOpener) (bool, error) {
	axis := FaultAxis[s.Kind]
	var recovery []byte
	if s.Kind == "restored" {
		axis = s.Recovery.Axis
		recovery, _ = json.Marshal(s.Recovery)
	}
	var seen struct {
		device  uuid.UUID
		kind    string
		binding *uuid.UUID
		seq     int64
		rec     []byte
	}
	err := tx.QueryRow(ctx, `SELECT device_id, event_type, binding_id, sequence, recovery FROM devices.device_events WHERE event_id = $1`, s.EventID).
		Scan(&seen.device, &seen.kind, &seen.binding, &seen.seq, &seen.rec)
	if err == nil {
		var prev *Recovery
		if len(seen.rec) > 0 {
			_ = json.Unmarshal(seen.rec, &prev)
		}
		if seen.device != s.DeviceID || seen.kind != s.Kind || !sameID(seen.binding, s.BindingID) || seen.seq != s.Sequence ||
			(prev == nil) != (s.Recovery == nil) || (prev != nil && *prev != *s.Recovery) {
			return false, apperr.E(apperr.Conflict, "errors.event_id_reused")
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var binding, unit *uuid.UUID
	var serial string
	err = tx.QueryRow(ctx, `SELECT binding_id, unit_id, serial FROM devices.devices WHERE id = $1 FOR UPDATE`, s.DeviceID).Scan(&binding, &unit, &serial)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return false, err
	}
	if !sameID(binding, s.BindingID) {
		return false, apperr.Fields(map[string]string{"bindingId": "errors.binding_mismatch"})
	}
	var latest *int64
	if err := tx.QueryRow(ctx, `SELECT max(sequence) FROM devices.device_events WHERE device_id = $1 AND binding_id IS NOT DISTINCT FROM $2 AND axis = $3`,
		s.DeviceID, s.BindingID, axis).Scan(&latest); err != nil {
		return false, err
	}
	if latest != nil && s.Sequence <= *latest {
		return false, nil // an old heartbeat does not restore the device (SR20)
	}
	evidence, alertIDs := evidenceOf[axis], []uuid.UUID{}
	if s.Kind == "restored" {
		var src uuid.UUID
		var restored *time.Time
		err := tx.QueryRow(ctx, `SELECT id, restored_at, evidence_source, alert_ids FROM devices.device_events WHERE device_id = $1 AND binding_id IS NOT DISTINCT FROM $2
			AND axis = $3 AND event_type <> 'restored' ORDER BY sequence DESC LIMIT 1 FOR UPDATE`, s.DeviceID, s.BindingID, axis).Scan(&src, &restored, &evidence, &alertIDs)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && (src != s.Recovery.SourceEventID || restored != nil)) {
			return false, apperr.E(apperr.Conflict, "errors.recovery_source_not_current")
		}
		if err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE devices.device_events SET restored_at = $2, version = version + 1 WHERE id = $1`, src, s.OccurredAt); err != nil {
			return false, err
		}
	}
	if s.Kind == "tamper" && unit != nil && binding != nil {
		var org uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT customer_org_id FROM devices.device_bindings WHERE id = $1`, *binding).Scan(&org); err != nil {
			return false, err
		}
		a, err := alerts.TamperAlert(ctx, tx, *unit, org, "Device "+serial+" reported its cover opened while powered (tamper_signal)", s.OccurredAt)
		if err != nil {
			return false, err
		}
		alertIDs = []uuid.UUID{a}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO devices.device_events (id, tenant_id, device_id, unit_id, binding_id, event_id, event_type, axis, evidence_source, recovery, alert_ids,
		sequence, occurred_at, created_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		uuid.Must(uuid.NewV7()), s.DeviceID, unit, s.BindingID, s.EventID, s.Kind, axis, evidence, recovery, alertIDs, s.Sequence, s.OccurredAt, now); err != nil {
		return false, err
	}
	restored := s.Kind == "restored"
	switch {
	case axis == "connection" && restored:
		err = setConnection(ctx, tx, s.DeviceID, "online", &s.OccurredAt, now)
	case axis == "connection":
		err = setConnection(ctx, tx, s.DeviceID, "offline", nil, now)
	default:
		col, value := map[string]string{"power": "power_signal", "tamper": "tamper"}[axis], map[string]string{"power": "off", "tamper": "detected"}[axis]
		if restored {
			value = RecoveredValue[axis]
		}
		_, err = tx.Exec(ctx, `UPDATE devices.devices SET `+col+` = $2, version = version + 1, updated_at = $3 WHERE id = $1`, s.DeviceID, value, now)
	}
	return err == nil, err
}
