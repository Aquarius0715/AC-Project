package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestLiveSiteSeeds: the live site (DEC-86) adds a monitoring-only model and, on its own property and room of
// customer-b, one air conditioner whose online module has the live sensors and an active binding; the unit mirrors the
// module. A second run adds nothing and leaves a connection that changed since alone. It runs in a transaction that is
// rolled back, so the shared test database keeps only the fixture.
func TestLiveSiteSeeds(t *testing.T) {
	server(t) // the test database with the fixture
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

	if err := seed.ApplyLive(ctx, tx, clock); err != nil {
		t.Fatalf("apply: %v", err)
	}
	s := seed.LiveSite
	count := func(sql string, args ...any) int {
		var n int
		if err := tx.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var control, modeControl, fanControl bool
	if err := tx.QueryRow(ctx, `SELECT control, mode_control, fan_control FROM devices.capabilities WHERE id = $1 AND is_current`, seed.ID(s.Capability)).
		Scan(&control, &modeControl, &fanControl); err != nil {
		t.Fatalf("model: %v", err)
	}
	if control || modeControl || fanControl {
		t.Errorf("the live model offers control %t/%t/%t; the nodes only measure", control, modeControl, fanControl)
	}
	if n := count(`SELECT count(*) FROM assets.units u JOIN assets.properties p ON p.id = u.property_id
		WHERE u.id = $1 AND u.customer_org_id = $2 AND p.customer_org_id = $2 AND u.space_id = $3 AND u.model_id = $4 AND NOT u.archived`,
		seed.ID(s.Unit), seed.ID(s.CustomerOrgID), seed.ID(s.Room), seed.ID(s.Capability)); n != 1 {
		t.Errorf("the live unit is not customer-b's in its room on the live model (%d)", n)
	}
	var devConn, unitConn string
	var devSeen, unitSeen *time.Time
	read := func() {
		t.Helper()
		if err := tx.QueryRow(ctx, `SELECT d.connection, d.last_seen_at, u.connection, u.last_seen_at FROM devices.devices d JOIN assets.units u ON u.id = d.unit_id
			WHERE d.id = $1`, seed.ID(s.Device)).Scan(&devConn, &devSeen, &unitConn, &unitSeen); err != nil {
			t.Fatalf("module: %v", err)
		}
	}
	read()
	if devConn != "online" || unitConn != "online" || devSeen == nil || unitSeen == nil || !devSeen.Equal(clock) || !unitSeen.Equal(clock) {
		t.Errorf("module %s seen %v, unit %s seen %v; want both online, seen at %s", devConn, devSeen, unitConn, unitSeen, clock)
	}
	for _, x := range s.Sensors {
		if n := count(`SELECT count(*) FROM devices.sensors WHERE id = $1 AND device_id = $2 AND metric = $3 AND unit = $4`,
			seed.ID(x.Name), seed.ID(s.Device), x.Metric, x.Unit); n != 1 {
			t.Errorf("sensor %s missing", x.Name)
		}
	}
	if n := count(`SELECT count(*) FROM devices.sensors WHERE device_id = $1 AND metric = 'power' AND boundary_id = 'ac_input_electricity'`, seed.ID(s.Device)); n != 1 {
		t.Error("the power sensor does not count the air conditioner's electricity")
	}
	if n := count(`SELECT count(*) FROM devices.device_bindings WHERE device_id = $1 AND unit_id = $2 AND unbound_at IS NULL`, seed.ID(s.Device), seed.ID(s.Unit)); n != 1 {
		t.Errorf("%d active bindings, want 1", n)
	}

	// the module went offline since: the next start keeps that and adds nothing
	if _, err := tx.Exec(ctx, `UPDATE devices.devices SET connection = 'offline' WHERE id = $1`, seed.ID(s.Device)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE assets.units SET connection = 'offline' WHERE id = $1`, seed.ID(s.Unit)); err != nil {
		t.Fatal(err)
	}
	if err := seed.ApplyLive(ctx, tx, clock.Add(time.Hour)); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	read()
	if devConn != "offline" || unitConn != "offline" || !unitSeen.Equal(clock) {
		t.Errorf("the second start changed the module to %s, the unit to %s seen %v", devConn, unitConn, unitSeen)
	}
	if n := count(`SELECT count(*) FROM assets.units WHERE property_id = $1`, seed.ID(s.Property)); n != 1 {
		t.Errorf("%d units on the live property, want 1", n)
	}
	if n := count(`SELECT count(*) FROM devices.sensors WHERE device_id = $1`, seed.ID(s.Device)); n != len(s.Sensors) {
		t.Errorf("%d live sensors, want %d", n, len(s.Sensors))
	}
	if n := count(`SELECT count(*) FROM devices.capabilities WHERE id = $1`, seed.ID(s.Capability)); n != 1 {
		t.Errorf("%d versions of the live model, want 1", n)
	}
}
