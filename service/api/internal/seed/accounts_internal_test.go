package seed

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/migrate"
	"github.com/pradita/ac-project/service/api/internal/migrations"
)

// scratchDB is an empty database with the schema, named name and removed after the test. The seed tests run on their
// own: the test databases hold the whole fixture, and the integration suite adds rows to ac_test while they run.
func scratchDB(t *testing.T, name string) *pgx.Conn {
	t.Helper()

	ctx := context.Background()

	admin := os.Getenv("AC_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		//nolint:gosec // G101: the local test database's password, not a secret
		admin = "postgres://postgres:local@localhost:5432/postgres?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	for _, statement := range []string{
		"DROP DATABASE IF EXISTS " + name + " WITH (FORCE)",
		"CREATE DATABASE " + name,
	} {
		_, err := conn.Exec(ctx, statement)
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := pgx.ParseConfig(admin)
	if err != nil {
		t.Fatal(err)
	}

	cfg.Database = name

	db, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = db.Close(ctx)
		_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = conn.Close(ctx)
	})

	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}

	_, err = migrate.Up(ctx, db, ms)
	if err != nil {
		t.Fatal(err)
	}

	return db
}

// TestLiveSiteAt: the live site goes to the customer of a client membership of the fixture with its own records
// unchanged; no client keeps customer-b's, and a membership that is not a client's, or none of the fixture, is refused.
func TestLiveSiteAt(t *testing.T) {
	t.Parallel()

	fixture, err := Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	site, err := LiveSiteAt(fixture, "")
	if err != nil || site.Client != "customer-b" || site.Customer != "cust-b" {
		t.Fatalf("no client: %+v, %v", site, err)
	}

	site, err = LiveSiteAt(fixture, "customer-a")
	if err != nil || site.Client != "customer-a" || site.CustomerOrgID != "org-customer-a" ||
		site.Customer != "cust-a" ||
		site.CustomerName != "Demo Customer A" ||
		site.Tenant != demoTenant ||
		site.Unit != LiveSite().Unit ||
		len(site.Modules()) != len(LiveSite().Modules()) {
		t.Fatalf("customer-a: %+v, %v", site, err)
	}

	for _, client := range []string{"contractor-a", "nobody"} {
		_, err := LiveSiteAt(fixture, client)
		if !errors.Is(err, errNoLiveClient) {
			t.Errorf("%s: %v, want a refusal", client, err)
		}
	}
}

// TestApplyAccounts (DEC-96): the accounts-only seed with the live site at customer-a builds the shared test
// environment's data on an empty database. Every account of the fixture is there with its organization, permissions
// and company user list, open-ended but for the external technicians, whose access ends far ahead as the schema needs,
// and every qualification holds as long; no scope names a place of the demo and no demo setting changes the alert
// rules. Of the demo's records only the live site is there, its two units customer-a's.
//
//nolint:paralleltest // it rebuilds the scratch database ac_seed_accounts_test
func TestApplyAccounts(t *testing.T) {
	ctx := context.Background()
	db := scratchDB(t, "ac_seed_accounts_test")

	fixture, err := Load(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	site, err := LiveSiteAt(fixture, "customer-a")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

	err = pgx.BeginFunc(ctx, db, func(txn pgx.Tx) error {
		applyErr := ApplyAccounts(ctx, txn, fixture)
		if applyErr != nil {
			return applyErr
		}

		return ApplyLive(ctx, txn, site, now)
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	count := func(sql string, args ...any) int {
		t.Helper()

		var total int

		scanErr := db.QueryRow(ctx, sql, args...).Scan(&total)
		if scanErr != nil {
			t.Fatalf("%s: %v", sql, scanErr)
		}

		return total
	}

	var externals, permissions, qualifications, accountScopes int

	for _, actor := range fixture.Actors {
		if actor.Employment != nil && *actor.Employment == "external" {
			externals++
		}

		permissions += len(actor.Permissions)
		qualifications += len(actor.Qualifications)

		for _, scope := range actor.Scopes {
			if scope.Kind == "organization" || scope.Kind == "tenant" {
				accountScopes++
			}
		}
	}

	far := accountsUntil()
	for _, check := range []struct {
		what, sql string
		args      []any
		want      int
	}{
		{
			"accounts", `SELECT count(*) FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id`, nil,
			len(fixture.Actors),
		},
		{
			"open accesses", `SELECT count(*) FROM identity.memberships WHERE valid_until IS NULL AND revoked_at IS NULL`,
			nil, len(fixture.Actors) - externals,
		},
		{"external technicians until the far end", `SELECT count(*) FROM identity.memberships
			WHERE employment = 'external' AND valid_until = $1`, []any{far}, externals},
		{"permissions", `SELECT count(*) FROM identity.membership_permissions`, nil, permissions},
		{
			"qualifications until the far end", `SELECT count(*) FROM identity.qualification_grants WHERE valid_until = $1`,
			[]any{far},
			qualifications,
		},
		{"organization and tenant scopes", `SELECT count(*) FROM identity.membership_scopes
			WHERE kind IN ('organization','tenant')`, nil, accountScopes},
		{"scopes to places", `SELECT count(*) FROM identity.membership_scopes WHERE kind IN ('property','unit')`, nil, 0},
		{"organizations", `SELECT count(*) FROM identity.organizations`, nil, len(fixture.DemoSeed.Organizations)},
		{"customers", `SELECT count(*) FROM assets.customers`, nil, len(fixture.DemoSeed.Customers)},
		{"customer users", `SELECT count(*) FROM identity.client_users`, nil, len(clientMemberships(fixture))},
		{"consents", `SELECT count(*) FROM identity.consents`, nil, len(fixture.DemoSeed.Consents)},
		{"default alert policies", `SELECT count(*) FROM monitoring.alert_policies WHERE kind = 'default_alert'`, nil, 1},
		{"units", `SELECT count(*) FROM assets.units`, nil, len(site.Modules())},
		{
			"customer-a's live units", `SELECT count(*) FROM assets.units WHERE customer_org_id = $1 AND property_id = $2`,
			[]any{ID("org-customer-a"), ID(site.Property)},
			len(site.Modules()),
		},
		{"properties", `SELECT count(*) FROM assets.properties`, nil, 1},
		{"models", `SELECT count(*) FROM devices.capabilities`, nil, len(site.Modules())},
		{"modules", `SELECT count(*) FROM devices.devices`, nil, len(site.Modules())},
	} {
		if got := count(check.sql, check.args...); got != check.want {
			t.Errorf("%s: %d, want %d", check.what, got, check.want)
		}
	}

	if n := count(`SELECT count(*) FROM identity.partner_users`); n == 0 {
		t.Error("no contractor company user lists")
	}
	// the records of the demo: none
	for _, table := range []string{
		"billing.contracts", "billing.contract_units", "billing.invoices", "control.commands", "energy.baselines",
		"energy.emission_factors", "maintenance.jobs", "maintenance.job_alerts", "maintenance.job_events",
		"maintenance.offers", "maintenance.assignments", "maintenance.rate_cards", "maintenance.filter_cleanings",
		"monitoring.alerts", "monitoring.allergen_observations", "monitoring.default_rule_settings",
		"monitoring.measurements", "notify.notifications", "restrictions.restrictions", "restrictions.restriction_units",
		"restrictions.restriction_commands", "restrictions.restriction_invoices",
	} {
		if n := count(`SELECT count(*) FROM ` + table); n != 0 {
			t.Errorf("%s: %d rows of the demo", table, n)
		}
	}
}
