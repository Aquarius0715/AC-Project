// Package devices implements the Devices module (capabilities, devices, firmware).
package devices

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Models answers model-register questions for other modules (assets.Models).
type Models struct{}

// CurrentVersion returns the current capability version of modelID, or ok=false when the model does not exist.
func (Models) CurrentVersion(ctx context.Context, c *ops.Call, modelID uuid.UUID) (int, bool, error) {
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT version FROM devices.capabilities WHERE id = $1 AND is_current`, modelID).Scan(&v)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return v, true, nil
}

// ByCode resolves a model code (Capability.model, case-insensitive) to its ID and current version.
func (Models) ByCode(ctx context.Context, c *ops.Call, code string) (uuid.UUID, int, bool, error) {
	var id uuid.UUID
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT id, version FROM devices.capabilities WHERE lower(model) = lower($1) AND is_current ORDER BY id LIMIT 1`, code).Scan(&id, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, false, nil
	}
	return id, v, err == nil, err
}

// SerialState is the device behind a serial for a CSV import row (DD-A18, case-insensitive): "unknown", "bound" (to
// any unit), "tamper" (an unresolved tamper blocks binding, D05) or "free".
func (Models) SerialState(ctx context.Context, c *ops.Call, serial string) (uuid.UUID, string, error) {
	var id uuid.UUID
	var bound bool
	var tamper string
	err := c.Tx.QueryRow(ctx, `SELECT d.id, d.unit_id IS NOT NULL OR EXISTS (SELECT 1 FROM devices.device_bindings b WHERE b.device_id = d.id AND b.unbound_at IS NULL), d.tamper
		FROM devices.devices d WHERE upper(d.serial) = upper($1) ORDER BY d.id LIMIT 1`, serial).Scan(&id, &bound, &tamper)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "unknown", nil
	}
	if err != nil {
		return uuid.Nil, "", err
	}
	switch {
	case bound:
		return id, "bound", nil
	case tamper == "detected":
		return id, "tamper", nil
	}
	return id, "free", nil
}

// BindImported binds a free device to a unit the CSV import has just created, like devices.bind; false when the device
// was bound or tampered with since the preview.
func (Models) BindImported(ctx context.Context, c *ops.Call, device, unit, org uuid.UUID, reason string) (bool, error) {
	var bound bool
	var tamper string
	if err := c.Tx.QueryRow(ctx, `SELECT d.unit_id IS NOT NULL OR EXISTS (SELECT 1 FROM devices.device_bindings b WHERE b.device_id = d.id AND b.unbound_at IS NULL), d.tamper
		FROM devices.devices d WHERE d.id = $1 FOR UPDATE`, device).Scan(&bound, &tamper); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if bound || tamper == "detected" {
		return false, nil
	}
	binding := uuid.Must(uuid.NewV7())
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7)`, binding, device, unit, org, c.Now, reason, c.Principal.MembershipID); err != nil {
		return false, err
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = $2, binding_id = $3, target_unit_id = $2, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, device, unit, binding).Scan(&v); err != nil {
		return false, err
	}
	c.Emit(ops.Event{AggregateType: "device", AggregateID: device, Type: "DeviceBound", Payload: map[string]any{"unitId": unit, "previousUnitId": nil}})
	c.Audit(ops.AuditEntry{Action: "devices.bind", TargetKind: "device", TargetID: device.String(), NextVersion: &v, Reason: reason})
	return true, nil
}

// UnbindUnits ends the active bindings of the units (an undone CSV import); the binding rows keep their history.
func (Models) UnbindUnits(ctx context.Context, c *ops.Call, units []uuid.UUID, reason string) error {
	rows, err := c.Tx.Query(ctx, `UPDATE devices.device_bindings SET unbound_at = $2, reason = $3 WHERE unit_id = ANY($1) AND unbound_at IS NULL RETURNING device_id, unit_id`,
		units, c.Now, reason)
	if err != nil {
		return err
	}
	type pair struct{ device, unit uuid.UUID }
	var ended []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.device, &p.unit); err != nil {
			rows.Close()
			return err
		}
		ended = append(ended, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range ended {
		var v int
		if err := c.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = NULL, binding_id = NULL, version = version + 1, updated_at = platform.app_now()
			WHERE id = $1 RETURNING version`, p.device).Scan(&v); err != nil {
			return err
		}
		c.Emit(ops.Event{AggregateType: "device", AggregateID: p.device, Type: "DeviceBound", Payload: map[string]any{"unitId": nil, "previousUnitId": p.unit}})
		c.Audit(ops.AuditEntry{Action: "devices.bind", TargetKind: "device", TargetID: p.device.String(), NextVersion: &v, Reason: reason})
	}
	return nil
}

// Caps returns the control capabilities of one model version (ok=false when it does not exist).
func (Models) Caps(ctx context.Context, c *ops.Call, modelID uuid.UUID, version int) (control.Caps, bool, error) {
	x, err := scanCap(c.Tx.QueryRow(ctx, `SELECT `+capCols+` FROM devices.capabilities WHERE id = $1 AND version = $2`, modelID, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return control.Caps{}, false, nil
	}
	if err != nil {
		return control.Caps{}, false, err
	}
	out := control.Caps{Control: x.Control, ModeControl: x.ModeControl, FanControl: x.FanControl, Modes: x.Modes, FanLevels: x.FanLevels,
		Ventilation: x.Ventilation, VentilationLevels: x.VentilationLevels}
	if x.Temperature != nil {
		out.HasTemperature, out.TempMin, out.TempMax, out.TempStep = true, x.Temperature.Min, x.Temperature.Max, x.Temperature.Step
	}
	return out, true, nil
}

// StaleAfter returns the D07 stale limit per sensor (monitoring.SensorLimits).
func (Models) StaleAfter(ctx context.Context, c *ops.Call, sensors []uuid.UUID) (map[uuid.UUID]time.Duration, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, stale_after_seconds FROM devices.sensors WHERE id = ANY($1)`, sensors)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]time.Duration{}
	for rows.Next() {
		var id uuid.UUID
		var sec int
		if err := rows.Scan(&id, &sec); err != nil {
			return nil, err
		}
		out[id] = time.Duration(sec) * time.Second
	}
	return out, rows.Err()
}

// BoundDevice returns the connection and power signal of the device currently bound to a unit.
func (Models) BoundDevice(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, string, string, bool, error) {
	var id uuid.UUID
	var conn, power string
	err := c.Tx.QueryRow(ctx, `SELECT d.id, d.connection, d.power_signal FROM devices.device_bindings b JOIN devices.devices d ON d.id = b.device_id
		WHERE b.unit_id = $1 AND b.unbound_at IS NULL`, unit).Scan(&id, &conn, &power)
	if errors.Is(err, pgx.ErrNoRows) {
		return id, "", "", false, nil
	}
	return id, conn, power, err == nil, err
}

// OperationBusy reports a queued/running device operation on the unit (D04 mutual exclusion).
func (Models) OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND status IN ('queued','running') AND expires_at > $2)`, unit, c.Now).Scan(&b)
	return b, err
}

// AlertRecovery is a recovery event of a device linked to an alert.
type AlertRecovery struct {
	ID         uuid.UUID
	EventType  string
	OccurredAt time.Time
}

// AlertRecoveries lists the recovery events (restored) that carry an alert since a time, the newest first: the
// evidence a tamper or connection alert's resolution may cite (IR327).
func (Models) AlertRecoveries(ctx context.Context, c *ops.Call, alert uuid.UUID, since time.Time) ([]AlertRecovery, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, event_type, occurred_at FROM devices.device_events WHERE $1 = ANY(alert_ids) AND event_type = 'restored' AND occurred_at >= $2
		ORDER BY occurred_at DESC, id LIMIT 50`, alert, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AlertRecovery, error) {
		var x AlertRecovery
		err := r.Scan(&x.ID, &x.EventType, &x.OccurredAt)
		return x, err
	})
}
