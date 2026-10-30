package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestLiveSiteSeeds: the live site (DEC-86 to DEC-88) adds a customer of its own with an invited owner whose sign-in
// account is asked for once and, per node, a monitoring-only model with the node's sensors and, on the customer's
// property and room, an air conditioner whose online module (serial: the node's name) has those sensors and an active
// binding; each unit mirrors its module. A second run adds nothing, asks for no second account and leaves a
// connection that changed since alone. It runs in a transaction that is rolled back, so the shared test database keeps
// only the fixture.
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
	type state struct {
		devConn, unitConn string
		devSeen, unitSeen *time.Time
	}
	read := func(n seed.LiveNode) state {
		t.Helper()
		var st state
		if err := tx.QueryRow(ctx, `SELECT d.connection, d.last_seen_at, u.connection, u.last_seen_at FROM devices.devices d JOIN assets.units u ON u.id = d.unit_id
			WHERE d.id = $1`, seed.ID(n.Device)).Scan(&st.devConn, &st.devSeen, &st.unitConn, &st.unitSeen); err != nil {
			t.Fatalf("%s module: %v", n.Node, err)
		}
		return st
	}
	for _, n := range s.Nodes {
		var control, modeControl, fanControl bool
		var model string
		if err := tx.QueryRow(ctx, `SELECT control, mode_control, fan_control, model FROM devices.capabilities WHERE id = $1 AND is_current`, seed.ID(n.Capability)).
			Scan(&control, &modeControl, &fanControl, &model); err != nil {
			t.Fatalf("%s model: %v", n.Node, err)
		}
		if control || modeControl || fanControl || model != n.Model {
			t.Errorf("%s's model %q offers control %t/%t/%t; the nodes only measure", n.Node, model, control, modeControl, fanControl)
		}
		if c := count(`SELECT count(*) FROM devices.capabilities c, jsonb_array_elements(c.sensors) e WHERE c.id = $1`, seed.ID(n.Capability)); c != len(n.Sensors) {
			t.Errorf("%s's model lists %d sensors, its module has %d", n.Node, c, len(n.Sensors))
		}
		if c := count(`SELECT count(*) FROM assets.units u JOIN assets.properties p ON p.id = u.property_id
			WHERE u.id = $1 AND u.customer_org_id = $2 AND p.customer_org_id = $2 AND u.space_id = $3 AND u.model_id = $4 AND u.display_name = $5 AND NOT u.archived`,
			seed.ID(n.Unit), org, seed.ID(s.Room), seed.ID(n.Capability), n.UnitName); c != 1 {
			t.Errorf("%s's unit is not the live customer's in its room on its model (%d)", n.Node, c)
		}
		st := read(n)
		if st.devConn != "online" || st.unitConn != "online" || st.devSeen == nil || st.unitSeen == nil || !st.devSeen.Equal(clock) || !st.unitSeen.Equal(clock) {
			t.Errorf("%s: module %s seen %v, unit %s seen %v; want both online, seen at %s", n.Node, st.devConn, st.devSeen, st.unitConn, st.unitSeen, clock)
		}
		if c := count(`SELECT count(*) FROM devices.devices WHERE id = $1 AND serial = $2`, seed.ID(n.Device), n.Serial); c != 1 {
			t.Errorf("%s's module is not serial %s", n.Node, n.Serial)
		}
		for _, x := range n.Sensors {
			if c := count(`SELECT count(*) FROM devices.sensors WHERE id = $1 AND device_id = $2 AND metric = $3 AND unit = $4`,
				seed.ID(x.Name), seed.ID(n.Device), x.Metric, x.Unit); c != 1 {
				t.Errorf("%s sensor %s missing", n.Node, x.Name)
			}
		}
		if c := count(`SELECT count(*) FROM devices.device_bindings WHERE device_id = $1 AND unit_id = $2 AND unbound_at IS NULL`, seed.ID(n.Device), seed.ID(n.Unit)); c != 1 {
			t.Errorf("%s: %d active bindings, want 1", n.Node, c)
		}
	}
	if c := count(`SELECT count(*) FROM devices.sensors s JOIN devices.devices d ON d.id = s.device_id
		WHERE d.unit_id = $1 AND s.metric = 'power' AND s.boundary_id = 'ac_input_electricity'`, seed.ID("unit-live-node-2")); c != 1 {
		t.Error("node_2's power sensor does not count the air conditioner's electricity")
	}

	// node_1's module went offline since: the next start keeps that and adds nothing
	first := s.Nodes[0]
	if _, err := tx.Exec(ctx, `UPDATE devices.devices SET connection = 'offline' WHERE id = $1`, seed.ID(first.Device)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE assets.units SET connection = 'offline' WHERE id = $1`, seed.ID(first.Unit)); err != nil {
		t.Fatal(err)
	}
	if err := seed.ApplyLive(ctx, tx, clock.Add(time.Hour)); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if st := read(first); st.devConn != "offline" || st.unitConn != "offline" || !st.unitSeen.Equal(clock) {
		t.Errorf("the second start changed node_1's module to %s, its unit to %s seen %v", st.devConn, st.unitConn, st.unitSeen)
	}
	if c := count(`SELECT count(*) FROM assets.units WHERE property_id = $1`, seed.ID(s.Property)); c != len(s.Nodes) {
		t.Errorf("%d units on the live property, want one per node (%d)", c, len(s.Nodes))
	}
	for _, n := range s.Nodes {
		if c := count(`SELECT count(*) FROM devices.sensors WHERE device_id = $1`, seed.ID(n.Device)); c != len(n.Sensors) {
			t.Errorf("%s: %d sensors, want %d", n.Node, c, len(n.Sensors))
		}
		if c := count(`SELECT count(*) FROM devices.capabilities WHERE id = $1`, seed.ID(n.Capability)); c != 1 {
			t.Errorf("%s: %d versions of its model, want 1", n.Node, c)
		}
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
