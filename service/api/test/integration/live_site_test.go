package integration_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestLiveSiteSeeds: the live site (DEC-86, DEC-89, DEC-91, DEC-92) adds, on a property of the fixture's Demo Customer
// B with one room, the air conditioner on a model that offers the settings the nodes' firmware takes (power,
// setpoint, mode and fan) and node_2's sensor unit on one with no control, each with an online module that has its
// sensors and an active binding; a unit mirrors its module. The customer's client, customer-b, keeps the access
// without the fixture's end date, and the site adds no person. A second run adds nothing, clears an end date set
// since, leaves a connection that changed since alone, and brings in a unit the site gained since the first, online.
// It runs in a transaction that is rolled back, so the shared test database keeps only the fixture.
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

	site := seed.LiveSite()
	count := func(sql string, args ...any) int {
		var total int

		err := txn.QueryRow(ctx, sql, args...).Scan(&total)
		if err != nil {
			t.Fatal(err)
		}

		return total
	}

	org, client := seed.ID(site.CustomerOrgID), seed.ID(site.Client)
	ends := func() int { // the client's membership with an end date
		return count(
			`SELECT count(*) FROM identity.memberships WHERE id = $1 AND valid_until IS NOT NULL`,
			client,
		)
	}
	people := func() int {
		return count(
			`SELECT count(*) FROM identity.client_users WHERE customer_id = $1`,
			seed.ID(site.Customer),
		)
	}

	if ends() != 1 {
		t.Fatal("the fixture's customer-b has no end of access to clear")
	}

	listed := people()

	applyLiveErr := seed.ApplyLive(ctx, txn, site, clock())
	if applyLiveErr != nil {
		t.Fatalf("apply: %v", applyLiveErr)
	}

	if total := count(
		`SELECT count(*) FROM identity.organizations o JOIN assets.customers c ON c.organization_id = o.id
		WHERE o.id = $1 AND o.kind = 'customer' AND o.status = 'active' AND c.id = $2 AND c.name = $3 AND o.tenant_id = $4`,
		org,
		seed.ID(site.Customer),
		site.CustomerName,
		seed.ID(site.Tenant),
	); total != 1 {
		t.Errorf("the live site's customer is not the fixture's %s (%d)", site.CustomerName, total)
	}

	if total := count(
		`SELECT count(*) FROM identity.memberships WHERE id = $1 AND organization_id = $2 AND role = 'client'
		AND valid_until IS NULL AND revoked_at IS NULL`,
		client,
		org,
	); total != 1 {
		t.Errorf(
			"the client %s does not keep an open access to the customer (%d)",
			site.Client,
			total,
		)
	}

	if n := people(); n != listed {
		t.Errorf("the site changed the customer's user list: %d people, before %d", n, listed)
	}

	for _, module := range site.Modules() {
		var (
			control, modeControl, fanControl bool
			manufacturer, model, settings    string
		)

		scanErr := txn.QueryRow(ctx, `SELECT control, mode_control, fan_control, manufacturer, model,
			concat_ws(' ', temperature->>'min', temperature->>'max', temperature->>'step', array_to_string(modes, ','),
				array_to_string(fan_levels, ','))
			FROM devices.capabilities WHERE id = $1 AND is_current`,
			seed.ID(module.Capability),
		).
			Scan(&control, &modeControl, &fanControl, &manufacturer, &model, &settings)
		if scanErr != nil {
			t.Fatalf("model of %s: %v", module.UnitName, scanErr)
		}

		wantSettings := " " // no temperature, no modes, no fan levels
		if s := module.Settings; s != nil {
			wantSettings = fmt.Sprintf("%g %g %g %s %s", s.TempMin, s.TempMax, s.TempStep,
				strings.Join(s.Modes, ","), strings.Join(s.FanLevels, ","))
		}

		if control != module.Control || modeControl != (module.Settings != nil) ||
			fanControl != (module.Settings != nil) ||
			settings != wantSettings ||
			manufacturer != module.Manufacturer ||
			model != module.Model {
			t.Errorf(
				"the model of %s, %s %s, offers control %t, mode %t, fan %t, settings %q; want control %t, settings %q",
				module.UnitName,
				manufacturer,
				model,
				control,
				modeControl,
				fanControl,
				settings,
				module.Control,
				wantSettings,
			)
		}

		if total := count(
			`SELECT count(*) FROM devices.capabilities c, jsonb_array_elements(c.sensors) e WHERE c.id = $1`,
			seed.ID(module.Capability),
		); total != len(module.Sensors) {
			t.Errorf(
				"the model of %s lists %d sensors, the module has %d",
				module.UnitName,
				total,
				len(module.Sensors),
			)
		}

		if total := count(
			`SELECT count(*) FROM assets.units u JOIN assets.properties p ON p.id = u.property_id
			WHERE u.id = $1 AND u.customer_org_id = $2 AND p.customer_org_id = $2 AND u.space_id = $3 AND u.model_id = $4
			AND u.display_name = $5 AND NOT u.archived`,
			seed.ID(module.Unit),
			org,
			seed.ID(site.Room),
			seed.ID(module.Capability),
			module.UnitName,
		); total != 1 {
			t.Errorf(
				"%s is not the live customer's in its room on its model (%d)",
				module.UnitName,
				total,
			)
		}

		got := liveConnection(ctx, t, txn, seed.ID(module.Device))
		if got.module != connectionOnline || got.unit != connectionOnline ||
			got.moduleSeen == nil ||
			got.unitSeen == nil ||
			!got.moduleSeen.Equal(clock()) ||
			!got.unitSeen.Equal(clock()) {
			t.Errorf(
				"%s: module %s seen %v, unit %s seen %v; want both online, seen at %s",
				module.UnitName,
				got.module,
				got.moduleSeen,
				got.unit,
				got.unitSeen,
				clock(),
			)
		}

		if c := count(
			`SELECT count(*) FROM devices.devices WHERE id = $1 AND serial = $2 AND mqtt_node = $3`,
			seed.ID(module.Device),
			module.Serial,
			module.Node,
		); c != 1 {
			t.Errorf(
				"the module of %s is not serial %s with MQTT node %s",
				module.UnitName,
				module.Serial,
				module.Node,
			)
		}

		for _, liveSensor := range module.Sensors {
			if total := count(
				`SELECT count(*) FROM devices.sensors WHERE id = $1 AND device_id = $2 AND metric = $3 AND unit = $4`,
				seed.ID(liveSensor.Name),
				seed.ID(module.Device),
				liveSensor.Metric,
				liveSensor.Unit,
			); total != 1 {
				t.Errorf("sensor %s missing", liveSensor.Name)
			}
		}

		if c := count(
			`SELECT count(*) FROM devices.device_bindings WHERE device_id = $1 AND unit_id = $2 AND unbound_at IS NULL`,
			seed.ID(module.Device),
			seed.ID(module.Unit),
		); c != 1 {
			t.Errorf("%s: %d active bindings, want 1", module.UnitName, c)
		}
	}

	if c := count(
		`SELECT count(*) FROM devices.sensors WHERE device_id = $1 AND metric = 'power' AND boundary_id =
			'ac_input_electricity'`,
		seed.ID(site.SensorUnits[0].Device),
	); c != 1 {
		t.Error("the power sensor of node_2 does not count the air conditioner's electricity")
	}

	// the air conditioner's module went offline since, and the sensor unit is one the site gained since (as an
	// installation seeded before DEC-91): the next start keeps the one's connection and brings in the other, online
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

	gained := site.SensorUnits[0]
	for _, statement := range []string{
		`DELETE FROM devices.sensors WHERE device_id = $1`,
		`DELETE FROM devices.device_bindings WHERE device_id = $1`,
		`DELETE FROM devices.devices WHERE id = $1`,
	} {
		_, execErr = txn.Exec(ctx, statement, seed.ID(gained.Device))
		if execErr != nil {
			t.Fatal(execErr)
		}
	}

	_, execErr = txn.Exec(ctx, `DELETE FROM assets.units WHERE id = $1`, seed.ID(gained.Unit))
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, execErr = txn.Exec(
		ctx,
		`DELETE FROM devices.capabilities WHERE id = $1`,
		seed.ID(gained.Capability),
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, execErr = txn.Exec(
		ctx,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		client,
		clock().Add(24*time.Hour),
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	applyLiveErr = seed.ApplyLive(ctx, txn, site, clock().Add(time.Hour))
	if applyLiveErr != nil {
		t.Fatalf("second apply: %v", applyLiveErr)
	}

	if ends() != 0 {
		t.Error("the second start left the client's end date set since")
	}

	kept := liveConnection(ctx, t, txn, seed.ID(site.Device))
	if kept.module != connectionOffline || kept.unit != connectionOffline || kept.unitSeen == nil ||
		!kept.unitSeen.Equal(clock()) {
		t.Errorf(
			"the second start changed the module to %s, the unit to %s seen %v",
			kept.module,
			kept.unit,
			kept.unitSeen,
		)
	}

	added := liveConnection(ctx, t, txn, seed.ID(gained.Device))
	if added.module != connectionOnline || added.unit != connectionOnline ||
		added.moduleSeen == nil ||
		added.unitSeen == nil ||
		!added.moduleSeen.Equal(clock().Add(time.Hour)) ||
		!added.unitSeen.Equal(clock().Add(time.Hour)) {
		t.Errorf(
			"the gained %s: module %s seen %v, unit %s seen %v; want both online as of the second start",
			gained.UnitName,
			added.module,
			added.moduleSeen,
			added.unit,
			added.unitSeen,
		)
	}

	if c := count(
		`SELECT count(*) FROM assets.units WHERE property_id = $1`,
		seed.ID(site.Property),
	); c != len(site.Modules()) {
		t.Errorf("%d units on the live property, want %d", c, len(site.Modules()))
	}

	for _, module := range site.Modules() {
		if total := count(
			`SELECT count(*) FROM devices.sensors WHERE device_id = $1`,
			seed.ID(module.Device),
		); total != len(module.Sensors) {
			t.Errorf("%s: %d live sensors, want %d", module.UnitName, total, len(module.Sensors))
		}

		if c := count(
			`SELECT count(*) FROM devices.capabilities WHERE id = $1`,
			seed.ID(module.Capability),
		); c != 1 {
			t.Errorf("%d versions of the model of %s, want 1", c, module.UnitName)
		}
	}

	if n := people(); n != listed {
		t.Errorf(
			"the second start changed the customer's user list: %d people, before %d",
			n,
			listed,
		)
	}
}

// moduleConnection is a live module's connection and last seen, with its unit's.
type moduleConnection struct {
	module, unit         string
	moduleSeen, unitSeen *time.Time
}

// liveConnection reads a live module's connection and last seen with its unit's.
func liveConnection(
	ctx context.Context,
	t *testing.T,
	txn pgx.Tx,
	device uuid.UUID,
) moduleConnection {
	t.Helper()

	var got moduleConnection

	err := txn.QueryRow(ctx, `SELECT d.connection, d.last_seen_at, u.connection, u.last_seen_at FROM devices.devices d
		JOIN assets.units u ON u.id = d.unit_id
		WHERE d.id = $1`, device).
		Scan(&got.module, &got.moduleSeen, &got.unit, &got.unitSeen)
	if err != nil {
		t.Fatalf("module %s: %v", device, err)
	}

	return got
}
