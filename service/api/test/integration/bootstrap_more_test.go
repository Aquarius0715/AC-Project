package integration_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
)

// bootstrapTx runs body in a transaction of the database owner (as cmd/migrate bootstrap-admin does) and rolls it back,
// so a demo clock set inside it is seen by no other test.
func bootstrapTx(t *testing.T, body func(ctx context.Context, tx pgx.Tx)) {
	t.Helper()

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	body(ctx, tx)
}

// TestBootstrapExistingTenantOnBusinessTime (IR509): cmd/migrate bootstrap-admin passes no start, so the membership
// starts at the business time, the wall clock with a demo clock's offset (IR168), not the wall clock; a tenant of that
// name without an administrator is reused with its HQ organization.
//
//nolint:paralleltest // the integration tests share the seeded test database
func TestBootstrapExistingTenantOnBusinessTime(t *testing.T) {
	bootstrapTx(t, func(ctx context.Context, tx pgx.Tx) {
		tag := uuid.NewString()[:8]

		var tenantID, orgID uuid.UUID

		err := tx.QueryRow(ctx, `INSERT INTO platform.tenants (name) VALUES ($1) RETURNING id`, "Reused "+tag).
			Scan(&tenantID)
		if err != nil {
			t.Fatal(err)
		}

		err = tx.QueryRow(ctx, `INSERT INTO identity.organizations (tenant_id, name, kind) VALUES ($1, 'Existing HQ',
			'operator') RETURNING id`, tenantID).
			Scan(&orgID)
		if err != nil {
			t.Fatal(err)
		}

		const offset = 30 * 24 * time.Hour

		_, err = tx.Exec(ctx, `INSERT INTO platform.demo_clock (offset_ms) VALUES ($1)
			ON CONFLICT (id) DO UPDATE SET offset_ms = EXCLUDED.offset_ms`, offset.Milliseconds())
		if err != nil {
			t.Fatal(err)
		}

		var wall time.Time

		err = tx.QueryRow(ctx, `SELECT now()`).Scan(&wall)
		if err != nil {
			t.Fatal(err)
		}

		result, err := bootstrap.Admin(ctx, tx, bootstrap.Input{
			Tenant:       "Reused " + tag,
			Organization: "Not used",
			Email:        "reused-admin-" + tag + "@example.com",
			Name:         "Reused Admin",
			At:           time.Time{},
		})
		if err != nil {
			t.Fatal(err)
		}

		if result.NewTenant || result.NewOrganization || result.TenantID != tenantID ||
			result.OrganizationID != orgID {
			t.Errorf("not reused: %+v, want tenant %s and organization %s", result, tenantID, orgID)
		}

		if !result.At.Equal(wall.Add(offset)) {
			t.Errorf("start %s, want the business time %s", result.At, wall.Add(offset))
		}

		var validFrom time.Time

		err = tx.QueryRow(ctx, `SELECT valid_from FROM identity.memberships WHERE id = $1`, result.MembershipID).
			Scan(&validFrom)
		if err != nil || !validFrom.Equal(result.At) {
			t.Errorf("membership valid_from %s %v, want %s", validFrom, err, result.At)
		}
	})
}

// TestBootstrapAmbiguousTenant: a name more than one tenant has is refused rather than picking one of them.
//
//nolint:paralleltest // the integration tests share the seeded test database
func TestBootstrapAmbiguousTenant(t *testing.T) {
	bootstrapTx(t, func(ctx context.Context, tx pgx.Tx) {
		name := "Twin " + uuid.NewString()[:8]

		_, err := tx.Exec(ctx, `INSERT INTO platform.tenants (name) VALUES ($1), ($1)`, name)
		if err != nil {
			t.Fatal(err)
		}

		_, err = bootstrap.Admin(ctx, tx, bootstrap.Input{
			Tenant:       name,
			Organization: "HQ",
			Email:        "twin-" + uuid.NewString()[:8] + "@example.com",
			Name:         "Twin Admin",
			At:           clock(),
		})
		if err == nil || !strings.Contains(err.Error(), "2 tenants are named") {
			t.Fatalf("Admin = %v, want the ambiguous tenant refused", err)
		}
	})
}
