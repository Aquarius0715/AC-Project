package seed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// maxHistoryDays is the longest series the API serves, and seriesStep the step of the 15-minute series.
const (
	maxHistoryDays = 35
	seriesStep     = 15 * time.Minute
)

// runningSQL is whether the unit runs at t: on the Kuala Lumpur schedule, and not in the switched-off window from $6.
const runningSQL = `((extract(isodow from t AT TIME ZONE 'Asia/Kuala_Lumpur') < 6 AND extract(hour from t AT TIME ` +
	`ZONE 'Asia/Kuala_Lumpur') BETWEEN 8 AND 21)
	OR (extract(isodow from t AT TIME ZONE 'Asia/Kuala_Lumpur') >= 6 AND extract(hour from t AT TIME ZONE ` +
	`'Asia/Kuala_Lumpur') BETWEEN 10 AND 22)) AND t < $6`

// jitterSQL is a stable 0..1 per sensor and minute; $9 is the sensor ID as text.
const jitterSQL = `(abs(hashtext($9 || t::text)) % 1000) / 1000.0`

// historySeries writes one series of a module's past readings over the window and returns how many it wrote.
type historySeries func(
	ctx context.Context,
	txn pgx.Tx,
	tenant string,
	pattern devicesim.Pattern,
	unit uuid.UUID,
	window historyWindow,
) (int64, error)

// errHistoryDays refuses a history length outside what the API serves.
var errHistoryDays = errors.New("days (1–35, the longest series the API serves)")

// ApplyFleetHistory gives the mock fleet days of past readings ending at until, so the charts have a long series:
// power every minute (the slots energy counts, D07) and temperature every 15 minutes, with a daily rhythm in Kuala
// Lumpur time (on 08:00–22:00 on weekdays, 10:00–23:00 at weekends; standby power and a warmer room otherwise), and
// humidity, CO2 and PM2.5 every 15 minutes. A
// module that is not reporting now (its pattern is silent or not online) stops a day before until; a unit switched off
// has been off for the last three hours; a power signal that is off has read 0 kW for the last six hours.
//
// Development only. The rows are written straight to the measurements and to energy's copy of the power samples, with
// the row-capture triggers off for this transaction, instead of one capture event per row. A minute that already has a
// reading of the sensor (an earlier run, or the simulator's live reading) is skipped, so it can run again.
func ApplyFleetHistory(
	ctx context.Context,
	txn pgx.Tx,
	fleet devicesim.Fleet,
	until time.Time,
	days int,
) (int64, error) {
	if days < 1 || days > maxHistoryDays {
		return 0, fmt.Errorf("history: %d %w", days, errHistoryDays)
	}

	var tenant string

	err := txn.QueryRow(ctx, `SELECT tenant_id::text FROM assets.customers WHERE organization_id =
		$1`, ID(fleet.CustomerOrgID)).
		Scan(&tenant)
	if err != nil {
		return 0, fmt.Errorf("history customer %s: %w", fleet.CustomerOrgID, err)
	}

	_, err = txn.Exec(
		ctx,
		`SET LOCAL session_replication_role = replica`,
	)
	if err != nil { // no capture events for history
		return 0, apperr.Wrap(err, "exec (ApplyFleetHistory)")
	}

	end := until.UTC().Truncate(time.Minute)
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	// the 15-minute series sit on the quarter hours, so a later run lands on the same instants and skips them
	quarter := start.Truncate(seriesStep)

	var total int64

	for _, pattern := range fleet.Patterns {
		if pattern.NoDevice {
			continue
		}

		added, err := patternHistory(
			ctx,
			txn,
			tenant,
			pattern,
			newHistoryWindow(pattern, start, quarter, end),
		)
		total += added

		if err != nil {
			return total, err
		}
	}

	return total, nil
}

// historyWindow is the time a module's past readings span: from start (quarter, the quarter hour at or before it, for
// the 15-minute series) to last, with the unit running on its schedule until offFrom and its meter reading 0 kW from
// zeroFrom.
type historyWindow struct {
	start, quarter, last, offFrom, zeroFrom time.Time
}

// newHistoryWindow is the pattern's history up to end: a module that is not reporting now (its pattern is silent or
// not online) stopped a day before; a unit switched off has been off for the last three hours; a power signal that is
// off has read 0 kW for the last six hours.
func newHistoryWindow(pattern devicesim.Pattern, start, quarter, end time.Time) historyWindow {
	window := historyWindow{
		start:    start,
		quarter:  quarter,
		last:     end,
		offFrom:  end.Add(time.Hour),
		zeroFrom: end.Add(time.Hour),
	}
	if pattern.Telemetry == "silent" || pattern.Connection != "online" {
		window.last = end.Add(-24 * time.Hour) // stopped reporting a day ago
	}

	if pattern.State.Power != nil && !*pattern.State.Power {
		window.offFrom = end.Add(-3 * time.Hour)
	}

	if pattern.PowerSignal == "off" {
		window.zeroFrom = end.Add(-6 * time.Hour)
	}

	return window
}

// patternHistory writes one module's past readings over the window and moves its last seen, and its unit's, to its
// last reading.
func patternHistory(
	ctx context.Context,
	txn pgx.Tx,
	tenant string,
	pattern devicesim.Pattern,
	window historyWindow,
) (int64, error) {
	unit := ID(devicesim.UnitName(pattern))

	var total int64

	for _, series := range []historySeries{powerHistory, temperatureHistory, airHistory} {
		added, err := series(ctx, txn, tenant, pattern, unit, window)
		total += added

		if err != nil {
			return total, err
		}
	}

	return total, lastHeard(ctx, txn, pattern, unit, window)
}

// fleetSensor is the mock modules' sensor of the metric.
func fleetSensor(metric string) devicesim.Sensor {
	var none devicesim.Sensor

	for _, sensor := range devicesim.Sensors() {
		if sensor.Metric == metric {
			return sensor
		}
	}

	return none
}

// powerHistory writes the module's power every minute (the slots energy counts, D07), also to energy's copy of the
// power samples: a daily curve while the unit runs, standby power otherwise, and 0 kW once the meter reads none.
func powerHistory(
	ctx context.Context,
	txn pgx.Tx,
	tenant string,
	pattern devicesim.Pattern,
	unit uuid.UUID,
	window historyWindow,
) (int64, error) {
	power := fleetSensor(keyPower)
	powerID := ID(devicesim.SensorName(pattern, power))

	tag, err := txn.Exec(
		ctx,
		`WITH s AS (
			SELECT t, CASE WHEN t >= $7 THEN 0
				WHEN `+runningSQL+` THEN round((0.9 + 0.35 * sin(pi() * ((extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') + `+
			`extract(minute from t) / 60.0) - 8) / 14) + 0.15 * `+jitterSQL+`)::numeric, 2)::float8
				ELSE round((0.02 + 0.01 * `+jitterSQL+`)::numeric, 3)::float8 END AS v
			FROM generate_series($4::timestamptz, $5::timestamptz, interval '1 minute') AS t
			WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)),
		m AS (INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, `+
			`unit, origin, quality, boundary_id, received_at, event_id)
			SELECT $1, $3, $2, 'power', t, extract(epoch from t)::bigint / 60, v, 'kW', 'measured', 'valid', $8, t + `+
			`interval '2 seconds', md5($9 || t::text)::uuid FROM s
			ON CONFLICT DO NOTHING)
		INSERT INTO energy.ref_power_samples (sensor_id, observed_at, sequence, tenant_id, unit_id, value, unit, origin, `+
			`quality, boundary_id, event_id)
		SELECT $2, t, extract(epoch from t)::bigint / 60, $1, $3, v, 'kW', 'measured', 'valid', $8, md5($9 || `+
			`t::text)::uuid FROM s
		ON CONFLICT DO NOTHING`,
		tenant,
		powerID,
		unit,
		window.start,
		window.last,
		window.offFrom,
		window.zeroFrom,
		power.Boundary,
		powerID.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("history power %s: %w", pattern.Key, err)
	}

	return tag.RowsAffected(), nil
}

// temperatureHistory writes the room temperature every 15 minutes: cooler while the unit runs, warmer otherwise.
func temperatureHistory(
	ctx context.Context,
	txn pgx.Tx,
	tenant string,
	pattern devicesim.Pattern,
	unit uuid.UUID,
	window historyWindow,
) (int64, error) {
	tempID := ID(devicesim.SensorName(pattern, fleetSensor(keyTemperature)))

	tag, err := txn.Exec(
		ctx,
		`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
			origin, quality, received_at, event_id)
		SELECT $1, $3, $2, 'temperature', t, extract(epoch from t)::bigint / 60,
			CASE WHEN
			`+runningSQL+` THEN round((24.2 + 0.6 * sin(pi() * extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') / 12) `+
			`+ 0.5 * `+strings.ReplaceAll(
			jitterSQL,
			"$9",
			"$7",
		)+`)::numeric, 1)::float8
				ELSE round((28.0 + 1.0 * sin(pi() * (extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') - 6) / 12) + 0.5 `+
			`* `+strings.ReplaceAll(
			jitterSQL,
			"$9",
			"$7",
		)+`)::numeric, 1)::float8 END,
			'°C', 'measured', 'valid', t + interval '2 seconds', md5($7 || t::text)::uuid
		FROM generate_series($4::timestamptz, $5::timestamptz, interval '15 minutes') AS t
		WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)
		ON CONFLICT DO NOTHING`,
		tenant,
		tempID,
		unit,
		window.quarter,
		window.last,
		window.offFrom,
		tempID.String(),
	)
	if err != nil {
		return 0, fmt.Errorf("history temperature %s: %w", pattern.Key, err)
	}

	return tag.RowsAffected(), nil
}

// airHistory writes the other air readings every 15 minutes: humidity drops while the AC runs, CO2 follows the people
// in the room, PM2.5 drifts.
func airHistory(
	ctx context.Context,
	txn pgx.Tx,
	tenant string,
	pattern devicesim.Pattern,
	unit uuid.UUID,
	window historyWindow,
) (int64, error) {
	var total int64

	for _, reading := range []struct{ metric, unit, on, off string }{
		{keyHumidity, "%", `52 + 4 * sin(pi() * extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') / 12) + 4 * ` +
			`j`, `66 + 5 * sin(pi() * (extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') - 6) / 12) + 4 * j`},
		{"co2", "ppm", `600 + 350 * sin(pi() * ((extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') + extract(minute ` +
			`from t) / 60.0) - 8) / 14) + 80 * j`, `430 + 40 * j`},
		{"pm25", "µg/m³", `12 + 10 * j`, `8 + 8 * j`},
	} {
		sensorID := ID(
			devicesim.SensorName(
				pattern,
				devicesim.Sensor{
					Metric:   reading.metric,
					ID:       "",
					Unit:     "",
					Base:     0,
					Swing:    0,
					Boundary: "",
				},
			),
		)

		tag, err := txn.Exec(
			ctx,
			`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
				origin, quality, received_at, event_id)
			SELECT $1, $3, $2, $8, t, extract(epoch from t)::bigint / 60,
				round((CASE WHEN
				`+runningSQL+` THEN `+reading.on+` ELSE `+reading.off+` END)::numeric, 1)::float8, $9, 'measured', 'valid', t + `+
				`interval '2 seconds', md5($7 || t::text)::uuid
			FROM (SELECT t, `+strings.ReplaceAll(
				jitterSQL,
				"$9",
				"$7",
			)+` AS j FROM generate_series($4::timestamptz, $5::timestamptz, interval '15 minutes') AS t
				WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)) g
			ON CONFLICT DO NOTHING`,
			tenant,
			sensorID,
			unit,
			window.quarter,
			window.last,
			window.offFrom,
			sensorID.String(),
			reading.metric,
			reading.unit,
		)
		if err != nil {
			return total, fmt.Errorf("history %s %s: %w", reading.metric, pattern.Key, err)
		}

		total += tag.RowsAffected()
	}

	return total, nil
}

// lastHeard moves the module's last seen, and its unit's, to its last reading; a later one is kept.
func lastHeard(
	ctx context.Context,
	txn pgx.Tx,
	pattern devicesim.Pattern,
	unit uuid.UUID,
	window historyWindow,
) error {
	// the module was window.last heard from at its window.last reading
	_, execErr := txn.Exec(
		ctx,
		`UPDATE devices.devices SET last_seen_at = $2 WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < $2)`,
		ID(devicesim.DeviceName(pattern)),
		window.last,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update devices.devices")
	}

	_, execErr = txn.Exec(
		ctx,
		`UPDATE assets.units SET last_seen_at = $2 WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < $2)`,
		unit,
		window.last,
	)

	return apperr.Wrap(execErr, "update assets.units")
}
