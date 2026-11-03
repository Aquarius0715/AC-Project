package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestLiveSiteSeeds: the live site (DEC-86, DEC-87, DEC-89) adds a customer of its own with an invited owner whose
// sign-in account is asked for once, a model that offers the remote power only and, on the customer's property and
// room, one air conditioner whose online module (both nodes) has the live sensors and an active binding; the unit
// mirrors the module. A second run adds nothing, asks for no second account and leaves a connection that changed
// since alone. It runs in a transaction that is rolled back, so the shared test database keeps only the fixture.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestLiveSiteSeeds(t *testing.T) {
	server(t) // the test database with the fixture

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

	applyLiveErr := seed.ApplyLive(ctx, txn, clock())
	if applyLiveErr != nil {
		t.Fatalf("apply: %v", applyLiveErr)
	}

	site := seed.LiveSite
	count := func(sql string, args ...any) int {
		var total int

		err := txn.QueryRow(ctx, sql, args...).Scan(&total)
		if err != nil {
			t.Fatal(err)
		}

		return total
	}

	org, owner := seed.ID(site.CustomerOrgID), seed.ID(site.Owner.Membership)
	if total := count(
		`SELECT count(*) FROM identity.organizations o JOIN assets.customers c ON c.organization_id = o.id
		WHERE o.id = $1 AND o.kind = 'customer' AND o.status = 'active' AND c.id = $2 AND c.name = $3 AND o.tenant_id = $4`,
		org,
		seed.ID(site.Customer),
		site.CustomerName,
		seed.ID(site.Tenant),
	); total != 1 {
		t.Errorf("the live customer is missing (%d)", total)
	}

	if total := count(
		`SELECT count(*) FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
		JOIN identity.client_users x ON x.membership_id = m.id AND x.customer_id = $3 AND x.client_role = 'owner' AND x.status
		= 'invited'
		WHERE m.id = $1 AND m.organization_id = $2 AND m.role = 'client' AND m.client_role = 'owner' AND m.valid_until IS NULL
		AND u.email = $4`,
		owner,
		org,
		seed.ID(site.Customer),
		site.Owner.Email,
	); total != 1 {
		t.Errorf("the live customer's invited owner is missing (%d)", total)
	}

	if n := count(
		`SELECT count(*) FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'control.execute'`,
		owner,
	); n != 1 {
		t.Error("the owner lacks a client's permissions")
	}

	if n := count(
		`SELECT count(*) FROM identity.membership_scopes WHERE membership_id = $1 AND kind = 'organization' AND ref_id = $2`,
		owner,
		org,
	); n != 1 {
		t.Error("the owner is not scoped to the live customer")
	}

	if n := count(
		`SELECT count(*) FROM identity.consents WHERE membership_id = $1 AND purpose = 'location_automation' AND NOT granted`,
		owner,
	); n != 1 {
		t.Error("the owner has no initial location consent")
	}

	accounts := func() int {
		return count(
			`SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type = 'AccountRequested'
			AND payload->>'role' = 'client' AND payload->>'userId' = $2`,
			owner,
			seed.ID(site.Owner.User).String(),
		)
	}
	if n := accounts(); n != 1 {
		t.Errorf("%d sign-in accounts asked for, want 1", n)
	}

	var (
		control, modeControl, fanControl, noTemperature bool
		manufacturer, model                             string
	)

	scanErr := txn.QueryRow(ctx, `SELECT control, mode_control, fan_control, temperature IS NULL, manufacturer, model FROM
		devices.capabilities WHERE id = $1 AND is_current`,
		seed.ID(site.Capability),
	).
		Scan(&control, &modeControl, &fanControl, &noTemperature, &manufacturer, &model)
	if scanErr != nil {
		t.Fatalf("model: %v", scanErr)
	}

	if !control || modeControl || fanControl || !noTemperature ||
		manufacturer != site.Manufacturer ||
		model != site.Model {
		t.Errorf(
			"the live model %s %s offers control %t, mode %t, fan %t, no temperature %t; want the power only",
			manufacturer,
			model,
			control,
			modeControl,
			fanControl,
			noTemperature,
		)
	}

	if total := count(
		`SELECT count(*) FROM devices.capabilities c, jsonb_array_elements(c.sensors) e WHERE c.id = $1`,
		seed.ID(site.Capability),
	); total != len(
		site.Sensors,
	) {
		t.Errorf("the model lists %d sensors, the module has %d", total, len(site.Sensors))
	}

	if total := count(
		`SELECT count(*) FROM assets.units u JOIN assets.properties p ON p.id = u.property_id
		WHERE u.id = $1 AND u.customer_org_id = $2 AND p.customer_org_id = $2 AND u.space_id = $3 AND u.model_id = $4 AND
		u.display_name = $5 AND NOT u.archived`,
		seed.ID(site.Unit),
		org,
		seed.ID(site.Room),
		seed.ID(site.Capability),
		site.UnitName,
	); total != 1 {
		t.Errorf(
			"the live unit is not the live customer's in its room on the live model (%d)",
			total,
		)
	}

	var (
		devConn, unitConn string
		devSeen, unitSeen *time.Time
	)

	read := func() {
		t.Helper()

		err := txn.QueryRow(ctx, `SELECT d.connection, d.last_seen_at, u.connection, u.last_seen_at FROM devices.devices d
			JOIN
			assets.units u ON u.id = d.unit_id
			WHERE d.id = $1`, seed.ID(site.Device)).
			Scan(&devConn, &devSeen, &unitConn, &unitSeen)
		if err != nil {
			t.Fatalf("module: %v", err)
		}
	}
	read()

	if devConn != connectionOnline || unitConn != connectionOnline || devSeen == nil ||
		unitSeen == nil ||
		!devSeen.Equal(clock()) ||
		!unitSeen.Equal(clock()) {
		t.Errorf(
			"module %s seen %v, unit %s seen %v; want both online, seen at %s",
			devConn,
			devSeen,
			unitConn,
			unitSeen,
			clock(),
		)
	}

	if c := count(
		`SELECT count(*) FROM devices.devices WHERE id = $1 AND serial = $2`,
		seed.ID(site.Device),
		site.Serial,
	); c != 1 {
		t.Errorf("the module is not serial %s", site.Serial)
	}

	for _, liveSensor := range site.Sensors {
		if total := count(
			`SELECT count(*) FROM devices.sensors WHERE id = $1 AND device_id = $2 AND metric = $3 AND unit = $4`,
			seed.ID(liveSensor.Name),
			seed.ID(site.Device),
			liveSensor.Metric,
			liveSensor.Unit,
		); total != 1 {
			t.Errorf("sensor %s missing", liveSensor.Name)
		}
	}

	if c := count(
		`SELECT count(*) FROM devices.sensors WHERE device_id = $1 AND metric = 'power' AND boundary_id =
			'ac_input_electricity'`,
		seed.ID(site.Device),
	); c != 1 {
		t.Error("the power sensor does not count the air conditioner's electricity")
	}

	if c := count(
		`SELECT count(*) FROM devices.device_bindings WHERE device_id = $1 AND unit_id = $2 AND unbound_at IS NULL`,
		seed.ID(site.Device),
		seed.ID(site.Unit),
	); c != 1 {
		t.Errorf("%d active bindings, want 1", c)
	}

	// the module went offline since: the next start keeps that and adds nothing
	_, execErr := txn.Exec(
		ctx,
		`UPDATE devices.devices SET connection = 'offline' WHERE id = $1`,
		seed.ID(site.Device),
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, execErr = txn.Exec(
		ctx,
		`UPDATE assets.units SET connection = 'offline' WHERE id = $1`,
		seed.ID(site.Unit),
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	applyLiveErr = seed.ApplyLive(ctx, txn, clock().Add(time.Hour))
	if applyLiveErr != nil {
		t.Fatalf("second apply: %v", applyLiveErr)
	}

	read()

	if devConn != connectionOffline || unitConn != connectionOffline || !unitSeen.Equal(clock()) {
		t.Errorf(
			"the second start changed the module to %s, the unit to %s seen %v",
			devConn,
			unitConn,
			unitSeen,
		)
	}

	if c := count(
		`SELECT count(*) FROM assets.units WHERE property_id = $1`,
		seed.ID(site.Property),
	); c != 1 {
		t.Errorf("%d units on the live property, want 1", c)
	}

	if total := count(
		`SELECT count(*) FROM devices.sensors WHERE device_id = $1`,
		seed.ID(site.Device),
	); total != len(
		site.Sensors,
	) {
		t.Errorf("%d live sensors, want %d", total, len(site.Sensors))
	}

	if c := count(
		`SELECT count(*) FROM devices.capabilities WHERE id = $1`,
		seed.ID(site.Capability),
	); c != 1 {
		t.Errorf("%d versions of the live model, want 1", c)
	}

	if n := accounts(); n != 1 {
		t.Errorf("the second start asked for the sign-in account again (%d)", n)
	}

	if n := count(
		`SELECT count(*) FROM identity.client_users WHERE customer_id = $1`,
		seed.ID(site.Customer),
	); n != 1 {
		t.Errorf("%d users on the live customer's list, want 1", n)
	}
}

// TestLiveSiteRefusesATakenAddress: the owner's address held by another person stops the seed instead of giving the
// live customer someone else's account.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestLiveSiteRefusesATakenAddress(t *testing.T) {
	server(t)

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

	_, execErr := txn.Exec(
		ctx,
		`INSERT INTO identity.users (email, display_name) VALUES ($1, 'Someone else')`,
		seed.LiveSite.Owner.Email,
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	applyLiveErr := seed.ApplyLive(ctx, txn, clock())
	if applyLiveErr == nil {
		t.Fatal("the seed took an address another person holds")
	}
}
