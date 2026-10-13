package seed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
)

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
func ApplyFleetHistory(ctx context.Context, tx pgx.Tx, f devicesim.Fleet, until time.Time, days int) (int64, error) {
	if days < 1 || days > 35 {
		return 0, fmt.Errorf("history: %d days (1–35, the longest series the API serves)", days)
	}
	var tenant string
	if err := tx.QueryRow(ctx, `SELECT tenant_id::text FROM assets.customers WHERE organization_id = $1`, ID(f.CustomerOrgID)).Scan(&tenant); err != nil {
		return 0, fmt.Errorf("history customer %s: %w", f.CustomerOrgID, err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL session_replication_role = replica`); err != nil { // no capture events for history
		return 0, err
	}
	end := until.UTC().Truncate(time.Minute)
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	var total int64
	for _, p := range f.Patterns {
		if p.NoDevice {
			continue
		}
		last := end
		if p.Telemetry == "silent" || p.Connection != "online" {
			last = end.Add(-24 * time.Hour) // stopped reporting a day ago
		}
		offFrom := end.Add(time.Hour) // never, unless the unit is off now
		if p.State.Power != nil && !*p.State.Power {
			offFrom = end.Add(-3 * time.Hour)
		}
		zeroFrom := end.Add(time.Hour)
		if p.PowerSignal == "off" {
			zeroFrom = end.Add(-6 * time.Hour)
		}
		unit := ID(devicesim.UnitName(p))
		var power, temperature devicesim.Sensor
		for _, s := range devicesim.Sensors {
			switch s.Metric {
			case "power":
				power = s
			case "temperature":
				temperature = s
			}
		}
		powerID, tempID := ID(devicesim.SensorName(p, power)), ID(devicesim.SensorName(p, temperature))
		// on: the KL schedule, and not in the switched-off window; jitter: a stable 0..1 per sensor and minute
		const on = `((extract(isodow from t AT TIME ZONE 'Asia/Kuala_Lumpur') < 6 AND extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') BETWEEN 8 AND 21)
			OR (extract(isodow from t AT TIME ZONE 'Asia/Kuala_Lumpur') >= 6 AND extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') BETWEEN 10 AND 22)) AND t < $6`
		const jitter = `(abs(hashtext($9 || t::text)) % 1000) / 1000.0` // $9: the sensor ID as text
		tag, err := tx.Exec(ctx, `WITH s AS (
				SELECT t, CASE WHEN t >= $7 THEN 0
					WHEN `+on+` THEN round((0.9 + 0.35 * sin(pi() * ((extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') + extract(minute from t) / 60.0) - 8) / 14) + 0.15 * `+jitter+`)::numeric, 2)::float8
					ELSE round((0.02 + 0.01 * `+jitter+`)::numeric, 3)::float8 END AS v
				FROM generate_series($4::timestamptz, $5::timestamptz, interval '1 minute') AS t
				WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)),
			m AS (INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, boundary_id, received_at, event_id)
				SELECT $1, $3, $2, 'power', t, extract(epoch from t)::bigint / 60, v, 'kW', 'measured', 'valid', $8, t + interval '2 seconds', md5($9 || t::text)::uuid FROM s
				ON CONFLICT DO NOTHING)
			INSERT INTO energy.ref_power_samples (sensor_id, observed_at, sequence, tenant_id, unit_id, value, unit, origin, quality, boundary_id, event_id)
			SELECT $2, t, extract(epoch from t)::bigint / 60, $1, $3, v, 'kW', 'measured', 'valid', $8, md5($9 || t::text)::uuid FROM s
			ON CONFLICT DO NOTHING`,
			tenant, powerID, unit, start, last, offFrom, zeroFrom, power.Boundary, powerID.String())
		if err != nil {
			return total, fmt.Errorf("history power %s: %w", p.Key, err)
		}
		total += tag.RowsAffected()
		tag, err = tx.Exec(ctx, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
			SELECT $1, $3, $2, 'temperature', t, extract(epoch from t)::bigint / 60,
				CASE WHEN `+on+` THEN round((24.2 + 0.6 * sin(pi() * extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') / 12) + 0.5 * `+strings.ReplaceAll(jitter, "$9", "$7")+`)::numeric, 1)::float8
					ELSE round((28.0 + 1.0 * sin(pi() * (extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') - 6) / 12) + 0.5 * `+strings.ReplaceAll(jitter, "$9", "$7")+`)::numeric, 1)::float8 END,
				'°C', 'measured', 'valid', t + interval '2 seconds', md5($7 || t::text)::uuid
			FROM generate_series($4::timestamptz, $5::timestamptz, interval '15 minutes') AS t
			WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)
			ON CONFLICT DO NOTHING`,
			tenant, tempID, unit, start, last, offFrom, tempID.String())
		if err != nil {
			return total, fmt.Errorf("history temperature %s: %w", p.Key, err)
		}
		total += tag.RowsAffected()
		// the other air readings every 15 minutes: humidity drops while the AC runs, CO2 follows the people in the room,
		// PM2.5 drifts
		for _, a := range []struct{ metric, unit, on, off string }{
			{"humidity", "%", `52 + 4 * sin(pi() * extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') / 12) + 4 * j`, `66 + 5 * sin(pi() * (extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') - 6) / 12) + 4 * j`},
			{"co2", "ppm", `600 + 350 * sin(pi() * ((extract(hour from t AT TIME ZONE 'Asia/Kuala_Lumpur') + extract(minute from t) / 60.0) - 8) / 14) + 80 * j`, `430 + 40 * j`},
			{"pm25", "µg/m³", `12 + 10 * j`, `8 + 8 * j`},
		} {
			id := ID(devicesim.SensorName(p, devicesim.Sensor{Metric: a.metric}))
			tag, err = tx.Exec(ctx, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
				SELECT $1, $3, $2, $8, t, extract(epoch from t)::bigint / 60,
					round((CASE WHEN `+on+` THEN `+a.on+` ELSE `+a.off+` END)::numeric, 1)::float8, $9, 'measured', 'valid', t + interval '2 seconds', md5($7 || t::text)::uuid
				FROM (SELECT t, `+strings.ReplaceAll(jitter, "$9", "$7")+` AS j FROM generate_series($4::timestamptz, $5::timestamptz, interval '15 minutes') AS t
					WHERE NOT EXISTS (SELECT 1 FROM monitoring.measurements x WHERE x.sensor_id = $2 AND x.observed_at = t)) g
				ON CONFLICT DO NOTHING`,
				tenant, id, unit, start, last, offFrom, id.String(), a.metric, a.unit)
			if err != nil {
				return total, fmt.Errorf("history %s %s: %w", a.metric, p.Key, err)
			}
			total += tag.RowsAffected()
		}
		// the module was last heard from at its last reading
		if _, err := tx.Exec(ctx, `UPDATE devices.devices SET last_seen_at = $2 WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < $2)`, ID(devicesim.DeviceName(p)), last); err != nil {
			return total, err
		}
		if _, err := tx.Exec(ctx, `UPDATE assets.units SET last_seen_at = $2 WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < $2)`, unit, last); err != nil {
			return total, err
		}
	}
	return total, nil
}
