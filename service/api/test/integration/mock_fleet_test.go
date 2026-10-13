package integration

import (
	"context"
	"testing"

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
}
