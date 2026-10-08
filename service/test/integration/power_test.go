package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

// TestEffectivePowerSR27 covers the SR27 boundaries: freshness of observation and measurement, quality, origin,
// null values, unknown sensors and no fallback to older valid rows.
func TestEffectivePowerSR27(t *testing.T) {
	s := server(t)
	tenant := seed.ID("tenant-a")
	power := func(u string) string {
		_, m := post(s, &hq, "units.get", `{"id":"`+u+`"}`)
		return data(m)["effectivePowerState"].(string)
	}
	setup := func(observedAgo time.Duration, connection string) (string, string) {
		u := newUnit(t, s, "Power AC")
		dev, sensor := uuid.NewString(), uuid.NewString()
		owner(t, `UPDATE assets.units SET connection = $2, observed_state = jsonb_build_object('power', true, 'celsius', 25, 'mode', 'cool', 'fanLevel', 'mid', 'observedAt', $3::timestamptz)
			WHERE id = $1`, u, connection, clock.Add(-observedAgo))
		owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`,
			dev, tenant, "PW-"+dev[:8], u, seed.ID("hq-operator"))
		owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, stale_after_seconds) VALUES ($1,$2,$3,'power','kW',120)`, sensor, tenant, dev)
		return u, sensor
	}
	reading := func(u, sensor string, ago time.Duration, seq int, origin, quality string, value any) {
		owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
			VALUES ($1,$2,$3,'power',$4,$5,$6,'kW',$7,$8,$4,$9)`, tenant, u, sensor, clock.Add(-ago), seq, value, origin, quality, uuid.NewString())
	}
	cases := []struct {
		name     string
		observed time.Duration
		conn     string
		prep     func(u, sensor string)
		want     string
	}{
		{"fresh valid at 120 s", 30 * time.Second, "online", func(u, x string) { reading(u, x, 120*time.Second, 1, "measured", "valid", 1.2) }, "on"},
		{"measurement 121 s old", 30 * time.Second, "online", func(u, x string) { reading(u, x, 121*time.Second, 1, "measured", "valid", 1.2) }, "unknown"},
		{"observation 121 s old", 121 * time.Second, "online", func(u, x string) { reading(u, x, time.Second, 1, "measured", "valid", 1.2) }, "unknown"},
		{"offline", 10 * time.Second, "offline", func(u, x string) { reading(u, x, time.Second, 1, "measured", "valid", 1.2) }, "unknown"},
		{"estimated origin", 10 * time.Second, "online", func(u, x string) { reading(u, x, time.Second, 1, "estimated", "valid", 1.2) }, "unknown"},
		{"suspect quality", 10 * time.Second, "online", func(u, x string) { reading(u, x, time.Second, 1, "measured", "suspect", 1.2) }, "unknown"},
		{"null value", 10 * time.Second, "online", func(u, x string) { reading(u, x, time.Second, 1, "measured", "valid", nil) }, "unknown"},
		{"no measurement", 10 * time.Second, "online", func(u, x string) {}, "unknown"},
		{"no fallback to older valid", 10 * time.Second, "online", func(u, x string) {
			reading(u, x, 20*time.Second, 1, "measured", "valid", 1.2)
			reading(u, x, 5*time.Second, 2, "measured", "missing", nil)
		}, "unknown"},
		{"same time, higher sequence wins", 10 * time.Second, "online", func(u, x string) {
			reading(u, x, 5*time.Second, 1, "measured", "missing", nil)
			reading(u, x, 5*time.Second, 2, "measured", "valid", 0.9)
		}, "on"},
		{"future reading ignored", 10 * time.Second, "online", func(u, x string) {
			reading(u, x, 5*time.Second, 1, "measured", "valid", 1.0)
			reading(u, x, -time.Minute, 2, "measured", "missing", nil)
		}, "on"},
		{"reading from an unknown sensor", 10 * time.Second, "online", func(u, x string) { reading(u, uuid.NewString(), time.Second, 1, "measured", "valid", 1.0) }, "unknown"},
	}
	for _, tc := range cases {
		u, sensor := setup(tc.observed, tc.conn)
		tc.prep(u, sensor)
		if got := power(u); got != tc.want {
			t.Errorf("%s: %s want %s", tc.name, got, tc.want)
		}
	}
}
