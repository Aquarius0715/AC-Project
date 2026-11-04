package seed

import (
	"context"
	"os"
	"testing"
	"time"
)

const fixturePath = "../../../../docs/04-agentic-sdlc/fixture-contract.json"

// TestIDDeterministic: a fixture ID maps to one UUIDv5, and two IDs to two.
func TestIDDeterministic(t *testing.T) {
	t.Parallel()

	if a, b := ID(
		"unit-online-rto",
	), ID(
		"unit-online-rto",
	); a != b || ID("a") == ID("b") ||
		ID("x").Version() != 5 { // the same name maps to the same ID
		t.Fatal("UUIDv5 mapping")
	}
}

// TestLoadErrors: a missing file and a file that is no JSON are errors.
func TestLoadErrors(t *testing.T) {
	t.Parallel()

	_, err := Load("nope.json")
	if err == nil {
		t.Fatal("missing file")
	}

	bad := t.TempDir() + "/bad.json"

	err = os.WriteFile(bad, []byte("{"), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Load(bad)
	if err == nil {
		t.Fatal("bad json")
	}
}

// Every demo actor has its own display name: screens list memberships by name (members.list displayName, IR172), so a
// copied name shows two people as one (R293: the override-only actor was named "hq-operator").
func TestActorNamesUnique(t *testing.T) {
	t.Parallel()

	fixture, err := Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}

	for _, actor := range fixture.Actors {
		if actor.DisplayName == "" {
			t.Errorf("%s has no display name", actor.MembershipID)
		}

		if other, ok := seen[actor.DisplayName]; ok {
			t.Errorf(
				"%s and %s share the display name %q",
				other,
				actor.MembershipID,
				actor.DisplayName,
			)
		}

		seen[actor.DisplayName] = actor.MembershipID
	}
}

// Applies the fixture twice in a rolled-back transaction and checks the row counts.
func TestApplyFixture(t *testing.T) {
	t.Parallel()

	fixture, err := Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	// its own database: the integration suite commits users, units and modules to ac_test meanwhile (CI run
	// 41846091547)
	conn := scratchDB(t, "ac_seed_fixture_test")
	ctx := context.Background()

	txn, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = txn.Rollback(ctx) }()

	for i := range 2 {
		err := Apply(ctx, txn, fixture)
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}

	ids := func(n int, get func(int) string) []any {
		out := make([]any, n)
		for i := range out {
			out[i] = ID(get(i))
		}

		return out
	}

	counts := map[string][]any{
		"identity.memberships": ids(
			len(fixture.Actors),
			func(i int) string { return fixture.Actors[i].MembershipID },
		),
		"assets.customers": ids(
			len(fixture.DemoSeed.Customers),
			func(i int) string { return fixture.DemoSeed.Customers[i].ID },
		),
		"assets.properties": ids(
			len(fixture.DemoSeed.Properties),
			func(i int) string { return fixture.DemoSeed.Properties[i].ID },
		),
		"assets.spaces": ids(
			len(fixture.DemoSeed.Spaces),
			func(i int) string { return fixture.DemoSeed.Spaces[i].ID },
		),
		"assets.units": ids(
			len(fixture.DemoSeed.Units),
			func(i int) string { return fixture.DemoSeed.Units[i].ID },
		),
		"devices.capabilities": ids(
			len(fixture.DemoSeed.Capabilities),
			func(i int) string { return fixture.DemoSeed.Capabilities[i].ID },
		),
		"maintenance.filter_cleanings": ids(
			len(fixture.DemoSeed.FilterCleanings),
			func(i int) string { return str(fixture.DemoSeed.FilterCleanings[i], "id") },
		),
	}
	for table, want := range counts {
		var got int

		err := txn.QueryRow(ctx, "SELECT count(DISTINCT id) FROM "+table+" WHERE id = ANY($1::uuid[])", want).
			Scan(&got)
		if err != nil {
			t.Fatal(err)
		}

		if got != len(want) || got == 0 {
			t.Errorf("%s: got %d want %d", table, got, len(want))
		}
	}

	var perms int

	err = txn.QueryRow(ctx, `SELECT count(*) FROM identity.membership_permissions WHERE
		membership_id=$1`, ID("hq-operator")).
		Scan(&perms)
	if err != nil || perms == 0 {
		t.Error("hq-operator has no permissions")
	}

	var name string

	scanErr := txn.QueryRow(ctx, `SELECT display_name FROM assets.units WHERE id=$1`, ID("unit-online-rto")).
		Scan(&name)
	if scanErr != nil ||
		name != "Bedroom AC" {
		t.Fatalf("unit-online-rto: %v %q", scanErr, name)
	}
	// IR91 item 2: rows without createdAt carry fixture.seedCreatedAt, not the wall clock (IR318); explicit ones keep
	// theirs
	for _, table := range []string{
		"identity.organizations", "identity.users", "identity.memberships",
		"devices.capabilities", "assets.customers", "assets.properties", "assets.spaces", "assets.units", "devices.devices",
		"monitoring.alert_policies", "billing.invoices",
	} {
		var later int

		err := txn.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE created_at <> $1", fixture.SeedCreatedAt).
			Scan(&later)
		if err != nil ||
			later != 0 {
			t.Errorf("%s: %d rows not created at seedCreatedAt (%v)", table, later, err)
		}
	}

	var now time.Time

	scanErr = txn.QueryRow(ctx, `SELECT platform.app_now()`).
		Scan(&now)
	if scanErr != nil ||
		now.Equal(fixture.SeedCreatedAt) {
		t.Errorf("app.now stays the seed time after Apply: %v %v", now, scanErr)
	}
}
