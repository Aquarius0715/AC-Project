package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestSlaTargetSavesNeverStoreOneVersionTwice: two saves of one plan type's targets at the same moment both read the
// same highest version, and the second used to store the same version number again; now the unique key refuses it and
// HQ saves again (IR778). A save is held half done on a second connection while the second one runs.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSlaTargetSavesNeverStoreOneVersionTwice(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	save := func(hours int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"sla.saveTargets",
			`{"planType":"environment","responseHours":6,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,`+
				`"effectiveFrom":"`+clock().Add(time.Duration(hours)*time.Hour).Format(time.RFC3339)+`"}`,
			0,
		)
	}

	if code, resp := save(3); code != http.StatusOK {
		t.Fatalf("first save: %d %v", code, resp)
	}

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

	// the other save, half done: the next version of the plan type at another start, not yet committed
	_, err = tx.Exec(
		ctx,
		`INSERT INTO maintenance.sla_targets (tenant_id, plan_type, response_hours, arrival_in_window_percent,
			first_time_fix_percent, effective_from, version)
		SELECT $1, 'environment', 5, 90, 80, $2, max(version) + 1 FROM maintenance.sla_targets
		WHERE tenant_id = $1 AND plan_type = 'environment'`,
		seed.ID("tenant-a"),
		clock().Add(5*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		owner(t, `DELETE FROM maintenance.sla_targets WHERE plan_type = 'environment' AND tenant_id = $1`, seed.ID("tenant-a"))
	})

	done := make(chan saveResult, 1)

	go func() {
		code, resp := save(7)
		done <- saveResult{code, resp}
	}()

	select {
	case got := <-done:
		t.Fatalf("the save did not wait for the other one: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != http.StatusConflict {
		t.Fatalf("the second save of the same version number: %d %v, want 409", got.code, got.resp)
	}

	// HQ saves again and gets the next number
	if code, resp := save(7); code != http.StatusOK {
		t.Errorf("save again: %d %v", code, resp)
	}
}

// TestRateCardSavesNeverStoreOneVersionTwice is the same race for a contractor's rate cards (IR778).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRateCardSavesNeverStoreOneVersionTwice(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	org, tenant := seed.ID("org-contractor-b"), seed.ID("tenant-a")

	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation) VALUES ($1,$2,'B','active','R-1','b@example.com',tstzrange($3,$4))`,
		tenant,
		org,
		clock().Add(-time.Hour),
		clock().Add(1000*time.Hour),
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org)
		owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)
	})

	save := func(hours int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"rateCards.save",
			`{"contractorOrgId":"`+org.String()+`","effectiveFrom":"`+clock().Add(time.Duration(hours)*time.Hour).
				Format(time.RFC3339)+`","currency":"MYR","lines":[{"workType":"repair_base","amountMinor":45000,"note":"base"}]}`,
			0,
		)
	}

	if code, resp := save(3); code != http.StatusOK {
		t.Fatalf("first save: %d %v", code, resp)
	}

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

	_, err = tx.Exec(
		ctx,
		`INSERT INTO maintenance.rate_cards (tenant_id, contractor_org_id, effective_from, currency, lines, version)
		SELECT $1, $2, $3, 'MYR', '[]', max(version) + 1 FROM maintenance.rate_cards WHERE contractor_org_id = $2`,
		tenant,
		org,
		clock().Add(5*time.Hour),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		code, resp := save(7)
		done <- saveResult{code, resp}
	}()

	select {
	case got := <-done:
		t.Fatalf("the save did not wait for the other one: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != http.StatusConflict {
		t.Fatalf("the second save of the same version number: %d %v, want 409", got.code, got.resp)
	}

	if code, resp := save(7); code != http.StatusOK {
		t.Errorf("save again: %d %v", code, resp)
	}
}
