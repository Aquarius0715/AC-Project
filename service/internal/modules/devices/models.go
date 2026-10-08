// Package devices implements the Devices module (capabilities, devices, firmware).
package devices

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/modules/control"
	"github.com/pradita/ac-project/service/internal/ops"
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

// SerialBound reports whether a device with this serial is currently bound to a unit.
func (Models) SerialBound(ctx context.Context, c *ops.Call, serial string) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.devices d JOIN devices.device_bindings b ON b.device_id = d.id AND b.unbound_at IS NULL
		WHERE upper(d.serial) = upper($1))`, serial).Scan(&b)
	return b, err
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
