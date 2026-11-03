package integration_test

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
	defer func() { _ = conn.Close(ctx) }()

	txn, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txn.Rollback(ctx) }() // the test never commits

	n, applyFleetErr := seed.ApplyFleet(ctx, txn, fleet, clock())
	if applyFleetErr != nil || n != len(fleet.Patterns) {
		t.Fatalf("apply: %d units, err %v", n, applyFleetErr)
	}

	n, applyFleetErr = seed.ApplyFleet(ctx, txn, fleet, clock())
	if applyFleetErr != nil || n != len(fleet.Patterns) {
		t.Fatalf("second apply: %d, err %v", n, applyFleetErr)
	}

	count := func(sql string, args ...any) int {
		var n int

		err := txn.QueryRow(ctx, sql, args...).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}

		return n
	}

	property := seed.ID(fleet.Property.ID)
	if n := count(
		`SELECT count(*) FROM assets.units WHERE property_id = $1`,
		property,
	); n != len(
		fleet.Patterns,
	) {
		t.Errorf(
			"%d units on the fleet property, want %d (a second apply adds nothing)",
			n,
			len(fleet.Patterns),
		)
	}

	modules := 0

	for _, pattern := range fleet.Patterns {
		if pattern.NoDevice {
			if n := count(
				`SELECT count(*) FROM devices.devices WHERE unit_id = $1`,
				seed.ID(devicesim.UnitName(pattern)),
			); n != 0 {
				t.Errorf("%s has a module", pattern.Key)
			}

			continue
		}

		modules++

		var conn, power, tamper, firmware string

		err := txn.QueryRow(ctx, `SELECT connection, power_signal, tamper, firmware_version FROM devices.devices WHERE id =
			$1`, seed.ID(devicesim.DeviceName(pattern))).
			Scan(&conn, &power, &tamper, &firmware)
		if err != nil {
			t.Fatalf("%s module: %v", pattern.Key, err)
		}

		if conn != pattern.Connection || power != pattern.PowerSignal || tamper != pattern.Tamper ||
			firmware != fleet.FirmwareOf(pattern) {
			t.Errorf("%s module is %s/%s/%s/%s", pattern.Key, conn, power, tamper, firmware)
		}

		if n := count(
			`SELECT count(*) FROM devices.sensors WHERE device_id = $1`,
			seed.ID(devicesim.DeviceName(pattern)),
		); n != len(
			devicesim.Sensors,
		) {
			t.Errorf("%s has %d sensors", pattern.Key, n)
		}
	}

	if n := count(
		`SELECT count(*) FROM devices.devices d JOIN assets.units u ON u.id = d.unit_id WHERE u.property_id = $1`,
		property,
	); n != modules {
		t.Errorf("%d modules on the fleet property, want %d", n, modules)
	}

	if n := count(
		`SELECT count(*) FROM assets.units WHERE property_id = $1 AND archived`,
		property,
	); n != 1 {
		t.Errorf("%d archived fleet units, want 1", n)
	}

	if n := count(
		`SELECT count(*) FROM assets.spaces WHERE property_id = $1 AND kind = 'floor'`,
		property,
	); n != 3 {
		t.Errorf("%d floors, want 3", n)
	}

	if n := count(
		`SELECT count(*) FROM assets.spaces WHERE property_id = $1 AND kind = 'room'`,
		property,
	); n != 10 {
		t.Errorf("%d rooms, want 10", n)
	}

	for _, pattern := range fleet.Patterns {
		if n := count(
			`SELECT count(*) FROM assets.units WHERE id = $1 AND space_id = $2`,
			seed.ID(devicesim.UnitName(pattern)),
			seed.ID(devicesim.SpaceName(pattern.Room)),
		); n != 1 {
			t.Errorf("%s is not in its room %s", pattern.Key, pattern.Room)
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
	defer func() { _ = conn.Close(ctx) }()

	txn, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txn.Rollback(ctx) }() // the test never commits

	_, applyFleetErr := seed.ApplyFleet(ctx, txn, fleet, clock())
	if applyFleetErr != nil {
		t.Fatal(applyFleetErr)
	}

	until := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC) // 14:00 in Kuala Lumpur, a Monday

	_, applyFleetHistoryErr := seed.ApplyFleetHistory(ctx, txn, fleet, until, 36)
	if applyFleetHistoryErr == nil {
		t.Error("36 days of history are accepted")
	}
	// a live reading the simulator already sent half an hour before until: history leaves that minute alone
	live := until.Add(-30 * time.Minute)

	_, execErr := txn.Exec(
		ctx,
		`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
			origin, quality, received_at, event_id)
		SELECT u.tenant_id, u.id, $2, 'power', $3, 1, 1.5, 'kW', 'measured', 'valid', $3, gen_random_uuid() FROM assets.units
		u WHERE u.id = $1`,
		seed.ID("unit-mock-cool-on"),
		seed.ID("sensor-mock-cool-on-power"),
		live,
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, applyFleetHistoryErr = seed.ApplyFleetHistory(ctx, txn, fleet, until, 2)
	if applyFleetHistoryErr != nil {
		t.Fatal(applyFleetHistoryErr)
	}

	count := func(sql string, args ...any) int {
		var n int

		err := txn.QueryRow(ctx, sql, args...).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}

		return n
	}

	power := seed.ID("sensor-mock-cool-on-power")
	if n := count(
		`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at = $2`,
		power,
		live,
	); n != 1 {
		t.Errorf("%d power readings at the live minute, want only the live one", n)
	}

	if n := count(
		`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`,
		power,
	); n != 2*24*60+1 {
		t.Errorf("%d power readings, want one a minute for two days (%d)", n, 2*24*60+1)
	}
	// the live minute reaches energy through its own capture event, not through history
	if n := count(
		`SELECT count(*) FROM energy.ref_power_samples WHERE sensor_id = $1`,
		power,
	); n != 2*24*60 {
		t.Errorf("%d power samples for energy, want %d (every minute but the live one)", n, 2*24*60)
	}

	if n := count(
		`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`,
		seed.ID("sensor-mock-cool-on-temperature"),
	); n != 2*24*4+1 {
		t.Errorf("%d temperatures, want one each 15 minutes (%d)", n, 2*24*4+1)
	}

	var afternoon, night float64

	scanErr := txn.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at =
		$2`, power, until.Add(-time.Hour)).
		Scan(&afternoon)
	if scanErr != nil {
		t.Fatal(scanErr)
	}

	scanErr = txn.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at =
		$2`, power, until.Add(-12*time.Hour)).
		Scan(&night)
	if scanErr != nil {
		t.Fatal(scanErr)
	}

	if afternoon < 0.8 || night > 0.05 {
		t.Errorf("power at 13:00 %.2f kW, at 02:00 %.3f kW", afternoon, night)
	}

	for _, metric := range []string{"humidity", "co2", "pm25"} {
		if n := count(
			`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`,
			seed.ID("sensor-mock-cool-on-"+metric),
		); n != 2*24*4+1 {
			t.Errorf("%d %s readings, want one each 15 minutes (%d)", n, metric, 2*24*4+1)
		}
	}

	var co2Day, co2Night float64

	scanErr = txn.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at =
		$2`, seed.ID("sensor-mock-cool-on-co2"), until.Add(-time.Hour)).
		Scan(&co2Day)
	if scanErr != nil {
		t.Fatal(scanErr)
	}

	scanErr = txn.QueryRow(ctx, `SELECT value FROM monitoring.measurements WHERE sensor_id = $1 AND observed_at =
		$2`, seed.ID("sensor-mock-cool-on-co2"), until.Add(-12*time.Hour)).
		Scan(&co2Night)
	if scanErr != nil {
		t.Fatal(scanErr)
	}

	if co2Day < 800 || co2Night > 500 {
		t.Errorf("CO2 at 13:00 %.0f ppm, at 02:00 %.0f ppm", co2Day, co2Night)
	}

	var seen time.Time

	scanErr = txn.QueryRow(ctx, `SELECT last_seen_at FROM devices.devices WHERE id = $1`, seed.ID("device-mock-cool-on")).
		Scan(&seen)
	if scanErr != nil ||
		!seen.Equal(until) {
		t.Errorf("the module was last seen at %v (%v), want %v", seen, scanErr, until)
	}
	// a later run (7 minutes on) keeps the quarter hours: no reading lands between them and none is doubled
	_, applyFleetHistoryErr = seed.ApplyFleetHistory(ctx, txn, fleet, until.Add(7*time.Minute), 2)
	if applyFleetHistoryErr != nil {
		t.Fatal(applyFleetHistoryErr)
	}

	temperature := seed.ID("sensor-mock-cool-on-temperature")
	if n := count(
		`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1 AND extract(minute from observed_at)::int % 15 <>
			0`,
		temperature,
	); n != 0 {
		t.Errorf("%d temperatures off the quarter hours", n)
	}

	if n := count(
		`SELECT count(*) FROM monitoring.measurements WHERE sensor_id = $1`,
		temperature,
	); n > 2*24*4+2 {
		t.Errorf("%d temperatures after a second run, want no doubling (at most %d)", n, 2*24*4+2)
	}

	var latest time.Time

	scanErr = txn.QueryRow(ctx, `SELECT max(observed_at) FROM monitoring.measurements WHERE sensor_id =
		$1`, seed.ID("sensor-mock-offline-power")).
		Scan(&latest)
	if scanErr != nil {
		t.Fatal(scanErr)
	}

	if !latest.Equal(until.Add(7*time.Minute - 24*time.Hour)) {
		t.Errorf(
			"the offline module's last reading is %v, want a day before %v",
			latest,
			until.Add(7*time.Minute),
		)
	}
}
