package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestMockFleetSeeds: the mock fleet adds one unit per pattern on its own property, a module for each pattern that has
// one with the pattern's connection, power signal, tamper and firmware, and runs again without adding anything. It runs
// in a transaction that is rolled back, so the shared test database keeps only the fixture.
func TestMockFleetSeeds(t *testing.T) {
	server(t) // the test database with the fixture
	fleet, err := devicesim.MockFleet()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // the test never commits

	if n, err := seed.ApplyFleet(ctx, tx, fleet, clock); err != nil || n != len(fleet.Patterns) {
		t.Fatalf("apply: %d units, err %v", n, err)
	}
	if n, err := seed.ApplyFleet(ctx, tx, fleet, clock); err != nil || n != len(fleet.Patterns) {
		t.Fatalf("second apply: %d, err %v", n, err)
	}
	count := func(sql string, args ...any) int {
		var n int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	property := seed.ID(fleet.Property.ID)
	if n := count(`SELECT count(*) FROM assets.units WHERE property_id = $1`, property); n != len(fleet.Patterns) {
		t.Errorf("%d units on the fleet property, want %d (a second apply adds nothing)", n, len(fleet.Patterns))
	}
	modules := 0
	for _, p := range fleet.Patterns {
		if p.NoDevice {
			if n := count(`SELECT count(*) FROM devices.devices WHERE unit_id = $1`, seed.ID(devicesim.UnitName(p))); n != 0 {
				t.Errorf("%s has a module", p.Key)
			}
			continue
		}
		modules++
		var conn, power, tamper, firmware string
		if err := tx.QueryRow(ctx, `SELECT connection, power_signal, tamper, firmware_version FROM devices.devices WHERE id = $1`, seed.ID(devicesim.DeviceName(p))).
			Scan(&conn, &power, &tamper, &firmware); err != nil {
			t.Fatalf("%s module: %v", p.Key, err)
		}
		if conn != p.Connection || power != p.PowerSignal || tamper != p.Tamper || firmware != fleet.FirmwareOf(p) {
			t.Errorf("%s module is %s/%s/%s/%s", p.Key, conn, power, tamper, firmware)
		}
		if n := count(`SELECT count(*) FROM devices.sensors WHERE device_id = $1`, seed.ID(devicesim.DeviceName(p))); n != len(devicesim.Sensors) {
			t.Errorf("%s has %d sensors", p.Key, n)
		}
	}
	if n := count(`SELECT count(*) FROM devices.devices d JOIN assets.units u ON u.id = d.unit_id WHERE u.property_id = $1`, property); n != modules {
		t.Errorf("%d modules on the fleet property, want %d", n, modules)
	}
	if n := count(`SELECT count(*) FROM assets.units WHERE property_id = $1 AND archived`, property); n != 1 {
		t.Errorf("%d archived fleet units, want 1", n)
	}
	if n := count(`SELECT count(*) FROM assets.spaces WHERE property_id = $1 AND kind = 'floor'`, property); n != 3 {
		t.Errorf("%d floors, want 3", n)
	}
	if n := count(`SELECT count(*) FROM assets.spaces WHERE property_id = $1 AND kind = 'room'`, property); n != 10 {
		t.Errorf("%d rooms, want 10", n)
	}
	for _, p := range fleet.Patterns {
		if n := count(`SELECT count(*) FROM assets.units WHERE id = $1 AND space_id = $2`, seed.ID(devicesim.UnitName(p)), seed.ID(devicesim.SpaceName(p.Room))); n != 1 {
			t.Errorf("%s is not in its room %s", p.Key, p.Room)
		}
	}
}

// TestMockFleetHistory: two days of history give each reporting module a power reading every minute (copied to
// energy's power samples) and a temperature every 15 minutes; power follows the Kuala Lumpur day (running in the
// afternoon, standby at night); a module that is not reporting stopped a day earlier; more than 35 days is refused.
func TestMockFleetHistory(t *testing.T) {
	server(t)
	fleet, err := devicesim.MockFleet()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // the test never commits
	if _, err := seed.ApplyFleet(ctx, tx, fleet, clock); err != nil {
		t.Fatal(err)
	}
	until := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC) // 14:00 in Kuala Lumpur, a Monday
	if _, err := seed.ApplyFleetHistory(ctx, tx, fleet, until, 36); err == nil {
		t.Error("36 days of history are accepted")
	}
	// a live reading the simulator already sent half an hour before until: history leaves that minute alone
	live := until.Add(-30 * time.Minute)
	if _, err := tx.Exec(ctx, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
		SELECT u.tenant_id, u.id, $2, 'power', $3, 1, 1.5, 'kW', 'measured', 'valid', $3, gen_random_uuid() FROM assets.units u WHERE u.id = $1`,
		seed.ID("unit-mock-cool-on"), seed.ID("sensor-mock-cool-on-power"), live); err != nil {
		t.Fatal(err)
	}
	if _, err := seed.ApplyFleetHistory(ctx, tx, fleet, until, 2); err != nil {
		t.Fatal(err)
	}
	count := func(sql string, args ...any) int {
		var n int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	power := seed.ID("sensor-mock-cool-on-power")
	if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`, power, live); n != 1 {
		t.Errorf("%d power readings at the live minute, want only the live one", n)
	}
	if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`, power); n != 2*24*60+1 {
		t.Errorf("%d power readings, want one a minute for two days (%d)", n, 2*24*60+1)
	}
	// the live minute reaches energy through its own capture event, not through history
	if n := count(`SELECT count(*) FROM energy.ref_power_samples WHERE sensor_id = $1`, power); n != 2*24*60 {
		t.Errorf("%d power samples for energy, want %d (every minute but the live one)", n, 2*24*60)
	}
	if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`, seed.ID("sensor-mock-cool-on-temperature")); n != 2*24*4+1 {
		t.Errorf("%d temperatures, want one each 15 minutes (%d)", n, 2*24*4+1)
	}
	var afternoon, night float64
	if err := tx.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`, power, until.Add(-time.Hour)).Scan(&afternoon); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`, power, until.Add(-12*time.Hour)).Scan(&night); err != nil {
		t.Fatal(err)
	}
	if afternoon < 0.8 || night > 0.05 {
		t.Errorf("power at 13:00 %.2f kW, at 02:00 %.3f kW", afternoon, night)
	}
	for _, metric := range []string{"humidity", "co2", "pm25"} {
		if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`, seed.ID("sensor-mock-cool-on-"+metric)); n != 2*24*4+1 {
			t.Errorf("%d %s readings, want one each 15 minutes (%d)", n, metric, 2*24*4+1)
		}
	}
	var co2Day, co2Night float64
	if err := tx.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`, seed.ID("sensor-mock-cool-on-co2"), until.Add(-time.Hour)).Scan(&co2Day); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`, seed.ID("sensor-mock-cool-on-co2"), until.Add(-12*time.Hour)).Scan(&co2Night); err != nil {
		t.Fatal(err)
	}
	if co2Day < 800 || co2Night > 500 {
		t.Errorf("CO2 at 13:00 %.0f ppm, at 02:00 %.0f ppm", co2Day, co2Night)
	}
	var seen time.Time
	if err := tx.QueryRow(ctx, `SELECT last_seen_at FROM devices.devices WHERE id = $1`, seed.ID("device-mock-cool-on")).Scan(&seen); err != nil || !seen.Equal(until) {
		t.Errorf("the module was last seen at %v (%v), want %v", seen, err, until)
	}
	// a later run (7 minutes on) keeps the quarter hours: no reading lands between them and none is doubled
	if _, err := seed.ApplyFleetHistory(ctx, tx, fleet, until.Add(7*time.Minute), 2); err != nil {
		t.Fatal(err)
	}
	temperature := seed.ID("sensor-mock-cool-on-temperature")
	if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1 AND extract(minute from observed_at)::int % 15 <> 0`, temperature); n != 0 {
		t.Errorf("%d temperatures off the quarter hours", n)
	}
	if n := count(`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`, temperature); n > 2*24*4+2 {
		t.Errorf("%d temperatures after a second run, want no doubling (at most %d)", n, 2*24*4+2)
	}
	var latest time.Time
	if err := tx.QueryRow(ctx, `SELECT max(observed_at) FROM monitoring.measurements WHERE sensor_id = $1`, seed.ID("sensor-mock-offline-power")).Scan(&latest); err != nil {
		t.Fatal(err)
	}
	if !latest.Equal(until.Add(7*time.Minute - 24*time.Hour)) {
		t.Errorf("the offline module's last reading is %v, want a day before %v", latest, until.Add(7*time.Minute))
	}
}
