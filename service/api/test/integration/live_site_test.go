package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestLiveSiteSeeds: the live site (DEC-86, DEC-87) adds a customer of its own with an invited owner whose sign-in
// account is asked for once, a monitoring-only model and, on the customer's property and room, one air conditioner
// whose online module has the live sensors and an active binding; the unit mirrors the module. A second run adds
// nothing, asks for no second account and leaves a connection that changed since alone. It runs in a transaction that
// is rolled back, so the shared test database keeps only the fixture.
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
	org, owner := seed.ID(s.CustomerOrgID), seed.ID(s.Owner.Membership)
	if n := count(`SELECT count(*) FROM identity.organizations o JOIN assets.customers c ON c.organization_id = o.id
		WHERE o.id = $1 AND o.kind = 'customer' AND o.status = 'active' AND c.id = $2 AND c.name = $3 AND o.tenant_id = $4`,
		org, seed.ID(s.Customer), s.CustomerName, seed.ID(s.Tenant)); n != 1 {
		t.Errorf("the live customer is missing (%d)", n)
	}
	if n := count(`SELECT count(*) FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
		JOIN identity.client_users x ON x.membership_id = m.id AND x.customer_id = $3 AND x.client_role = 'owner' AND x.status = 'invited'
		WHERE m.id = $1 AND m.organization_id = $2 AND m.role = 'client' AND m.client_role = 'owner' AND m.valid_until IS NULL AND u.email = $4`,
		owner, org, seed.ID(s.Customer), s.Owner.Email); n != 1 {
		t.Errorf("the live customer's invited owner is missing (%d)", n)
	}
	if n := count(`SELECT count(*) FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'control.execute'`, owner); n != 1 {
		t.Error("the owner lacks a client's permissions")
	}
	if n := count(`SELECT count(*) FROM identity.membership_scopes WHERE membership_id = $1 AND kind = 'organization' AND ref_id = $2`, owner, org); n != 1 {
		t.Error("the owner is not scoped to the live customer")
	}
	if n := count(`SELECT count(*) FROM identity.consents WHERE membership_id = $1 AND purpose = 'location_automation' AND NOT granted`, owner); n != 1 {
		t.Error("the owner has no initial location consent")
	}
	accounts := func() int {
		return count(`SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type = 'AccountRequested'
			AND payload->>'role' = 'client' AND payload->>'userId' = $2`, owner, seed.ID(s.Owner.User).String())
	}
	if n := accounts(); n != 1 {
		t.Errorf("%d sign-in accounts asked for, want 1", n)
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
	if n := accounts(); n != 1 {
		t.Errorf("the second start asked for the sign-in account again (%d)", n)
	}
	if n := count(`SELECT count(*) FROM identity.client_users WHERE customer_id = $1`, seed.ID(s.Customer)); n != 1 {
		t.Errorf("%d users on the live customer's list, want 1", n)
	}
}

// TestLiveSiteRefusesATakenAddress: the owner's address held by another person stops the seed instead of giving the
// live customer someone else's account.
func TestLiveSiteRefusesATakenAddress(t *testing.T) {
	server(t)
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
	if _, err := tx.Exec(ctx, `INSERT INTO identity.users (email, display_name) VALUES ($1, 'Someone else')`, seed.LiveSite.Owner.Email); err != nil {
		t.Fatal(err)
	}
	if err := seed.ApplyLive(ctx, tx, clock); err == nil {
		t.Fatal("the seed took an address another person holds")
	}
}
