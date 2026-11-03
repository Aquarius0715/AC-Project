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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Models answers model-register questions for other modules (assets.Models).
type Models struct{}

// CurrentVersion returns the current capability version of modelID, or ok=false when the model does not exist.
func (Models) CurrentVersion(
	ctx context.Context,
	c *ops.Call,
	modelID uuid.UUID,
) (int, bool, error) {
	var version int

	err := c.Tx.QueryRow(ctx, `SELECT version FROM devices.capabilities WHERE id = $1 AND is_current`, modelID).
		Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}

		return 0, false, apperr.Wrap(err, "select devices.capabilities")
	}

	return version, true, nil
}

// ByCode resolves a model code (Capability.model, case-insensitive) to its ID and current version.
func (Models) ByCode(
	ctx context.Context,
	call *ops.Call,
	code string,
) (uuid.UUID, int, bool, error) {
	var (
		modelID uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, version FROM devices.capabilities WHERE lower(model) = lower($1) AND
		is_current
		ORDER BY id LIMIT 1`, code).
		Scan(&modelID, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, false, nil
	}

	return modelID, version, err == nil, apperr.Wrap(err, "select devices.capabilities")
}

// SerialState is the device behind a serial for a CSV import row (DD-A18, case-insensitive): "unknown", "bound" (to
// any unit), "tamper" (an unresolved tamper blocks binding, D05) or "free".
func (Models) SerialState(
	ctx context.Context,
	call *ops.Call,
	serial string,
) (uuid.UUID, string, error) {
	var (
		deviceID uuid.UUID
		bound    bool
		tamper   string
	)

	err := call.Tx.QueryRow(ctx, `SELECT d.id, d.unit_id IS NOT NULL OR EXISTS (SELECT 1 FROM devices.device_bindings b
		WHERE
		b.device_id = d.id AND b.unbound_at IS NULL), d.tamper
		FROM devices.devices d WHERE upper(d.serial) = upper($1) ORDER BY d.id LIMIT 1`, serial).
		Scan(&deviceID, &bound, &tamper)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "unknown", nil
	}

	if err != nil {
		return uuid.Nil, "", apperr.Wrap(err, "select devices.devices")
	}

	switch {
	case bound:
		return deviceID, "bound", nil
	case tamper == tamperDetected:
		return deviceID, "tamper", nil
	}

	return deviceID, "free", nil
}

// BindImported binds a free device to a unit the CSV import has just created, like devices.bind; false when the device
// was bound or tampered with since the preview.
func (Models) BindImported(
	ctx context.Context,
	call *ops.Call,
	device, unit, org uuid.UUID,
	reason string,
) (bool, error) {
	var (
		bound  bool
		tamper string
	)

	err := call.Tx.QueryRow(ctx, `SELECT d.unit_id IS NOT NULL OR EXISTS (SELECT 1 FROM devices.device_bindings b WHERE
		b.device_id = d.id AND b.unbound_at IS NULL), d.tamper
		FROM devices.devices d WHERE d.id = $1 FOR UPDATE`, device).
		Scan(&bound, &tamper)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, apperr.Wrap(err, "select devices.devices")
	}

	if bound || tamper == tamperDetected {
		return false, nil
	}

	version, err := insertBinding(ctx, call, device, unit, org, reason)
	if err != nil {
		return false, err
	}

	recordBinding(call, device, unit, nil, nil, version, reason)

	return true, nil
}

// UnbindUnits ends the active bindings of the units (an undone CSV import); the binding rows keep their history.
func (Models) UnbindUnits(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
	reason string,
) error {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE devices.device_bindings SET unbound_at = $2, reason = $3 WHERE unit_id = ANY($1) AND unbound_at IS NULL
			RETURNING device_id, unit_id`,
		units,
		call.Now,
		reason,
	)
	if err != nil {
		return apperr.Wrap(err, "update devices.device_bindings")
	}

	type pair struct{ device, unit uuid.UUID }

	var ended []pair

	for rows.Next() {
		var binding pair

		err := rows.Scan(&binding.device, &binding.unit)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "update devices.device_bindings")
		}

		ended = append(ended, binding)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "update devices.device_bindings")
	}

	for _, binding := range ended {
		var version int

		err := call.Tx.QueryRow(ctx, `UPDATE devices.devices SET unit_id = NULL, binding_id = NULL, version = version + 1,
			updated_at = platform.app_now()
			WHERE id = $1 RETURNING version`, binding.device).
			Scan(&version)
		if err != nil {
			return apperr.Wrap(err, "update devices.devices")
		}

		recordBinding(call, binding.device, nil, binding.unit, nil, version, reason)
	}

	return nil
}

// Caps returns the control capabilities of one model version (ok=false when it does not exist).
func (Models) Caps(
	ctx context.Context,
	c *ops.Call,
	modelID uuid.UUID,
	version int,
) (control.Caps, bool, error) {
	capability, err := scanCap(
		c.Tx.QueryRow(
			ctx,
			`SELECT `+capCols+` FROM devices.capabilities WHERE id = $1 AND version = $2`,
			modelID,
			version,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return control.Caps{
			Control:           false,
			ModeControl:       false,
			FanControl:        false,
			TempMin:           0,
			TempMax:           0,
			TempStep:          0,
			HasTemperature:    false,
			Modes:             nil,
			FanLevels:         nil,
			Ventilation:       false,
			VentilationLevels: nil,
		}, false, nil
	}

	if err != nil {
		return control.Caps{}, false, err
	}

	out := control.Caps{
		Control:           capability.Control,
		ModeControl:       capability.ModeControl,
		FanControl:        capability.FanControl,
		Modes:             capability.Modes,
		FanLevels:         capability.FanLevels,
		Ventilation:       capability.Ventilation,
		VentilationLevels: capability.VentilationLevels,
		TempMin:           0,
		TempMax:           0,
		TempStep:          0,
		HasTemperature:    false,
	}
	if capability.Temperature != nil {
		out.HasTemperature = true
		out.TempMin = capability.Temperature.Min
		out.TempMax = capability.Temperature.Max
		out.TempStep = capability.Temperature.Step
	}

	return out, true, nil
}

// StaleAfter returns the D07 stale limit per sensor (monitoring.SensorLimits).
func (Models) StaleAfter(
	ctx context.Context,
	c *ops.Call,
	sensors []uuid.UUID,
) (map[uuid.UUID]time.Duration, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, stale_after_seconds FROM devices.sensors WHERE id = ANY($1)`,
		sensors,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select devices.sensors")
	}
	defer rows.Close()

	out := map[uuid.UUID]time.Duration{}

	for rows.Next() {
		var (
			sensorID uuid.UUID
			sec      int
		)

		err := rows.Scan(&sensorID, &sec)
		if err != nil {
			return nil, apperr.Wrap(err, "select devices.sensors")
		}

		out[sensorID] = time.Duration(sec) * time.Second
	}

	return out, apperr.Wrap(rows.Err(), "select devices.sensors")
}

// BoundDevice returns the connection and power signal of the device currently bound to a unit.
func (Models) BoundDevice(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (uuid.UUID, string, string, bool, error) {
	var (
		deviceID    uuid.UUID
		conn, power string
	)

	err := call.Tx.QueryRow(ctx, `SELECT d.id, d.connection, d.power_signal FROM devices.device_bindings b JOIN
		devices.devices d ON d.id = b.device_id
		WHERE b.unit_id = $1 AND b.unbound_at IS NULL`, unit).
		Scan(&deviceID, &conn, &power)
	if errors.Is(err, pgx.ErrNoRows) {
		return deviceID, "", "", false, nil
	}

	return deviceID, conn, power, err == nil, apperr.Wrap(err, "select devices.device_bindings")
}

// OperationBusy reports a queued/running device operation on the unit (D04 mutual exclusion).
func (Models) OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var busy bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE unit_id = $1 AND status IN
		('queued','running') AND expires_at > $2)`, unit, c.Now).
		Scan(&busy)

	return busy, apperr.Wrap(err, "select devices.device_operations")
}

// AlertRecovery is a recovery event of a device linked to an alert.
type AlertRecovery struct {
	ID         uuid.UUID
	EventType  string
	OccurredAt time.Time
}

// AlertRecoveries lists the recovery events (restored) that carry an alert since a time, the newest first: the
// evidence a tamper or connection alert's resolution may cite (IR327).
func (Models) AlertRecoveries(
	ctx context.Context,
	c *ops.Call,
	alert uuid.UUID,
	since time.Time,
) ([]AlertRecovery, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, event_type, occurred_at FROM devices.device_events WHERE $1 = ANY(alert_ids) AND event_type = 'restored'
			AND occurred_at >= $2
		ORDER BY occurred_at DESC, id LIMIT 50`,
		alert,
		since,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select devices.device_events")
	}

	recoveries, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (AlertRecovery, error) {
		var x AlertRecovery

		err := r.Scan(&x.ID, &x.EventType, &x.OccurredAt)

		return x, apperr.Wrap(err, "scan (AlertRecoveries)")
	})

	return recoveries, apperr.Wrap(err, "select devices.device_events")
}
