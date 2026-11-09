package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A customer is deactivated only after the customer row is locked, so a unit that is being added at the same moment
// (units.save holds the row with a share lock until it commits) is seen by the check for units: the deactivation waits
// for it and is CONFLICT error.customerActive, instead of leaving an active unit under an inactive customer.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCustomerDeactivateWaitsForAUnitBeingAdded(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	name := "Race Deactivate " + uuid.NewString()[:8]

	code, resp := write(
		srv,
		hq(),
		"organizations.save",
		`{"name":"`+name+`","kind":"customer","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("organization: %d %v", code, resp)
	}

	org := asStr(t, data(resp)["id"])

	code, resp = write(
		srv,
		hq(),
		"customers.save",
		`{"name":"`+name+`","organizationId":"`+org+`","serviceProfile":"general","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("customer: %d %v", code, resp)
	}

	cust, version := asStr(t, data(resp)["id"]), ver(t, resp)

	code, resp = write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"office","name":"Race site","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("property: %d %v", code, resp)
	}

	prop := asStr(t, data(resp)["id"])

	t.Cleanup(func() {
		owner(t, `DELETE FROM assets.units WHERE customer_org_id = $1`, org)
		owner(t, `DELETE FROM assets.properties WHERE customer_org_id = $1`, org)
		owner(t, `DELETE FROM assets.customers WHERE id = $1`, cust)
		owner(t, `DELETE FROM identity.organizations WHERE id = $1`, org)
	})

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// the unit save, half done: it has read the customer as active (share lock) and has inserted its unit
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `SELECT 1 FROM assets.customers WHERE id = $1 FOR SHARE`, cust)
	if err != nil {
		t.Fatal(err)
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO assets.units (tenant_id, customer_org_id, property_id, display_name, model_id, service_scope,
			capability_version) SELECT tenant_id, customer_org_id, id, 'Late unit', $2, '{indoor}', 1 FROM assets.properties
			WHERE id = $1`,
		prop,
		seed.ID("ventilation-demo"),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"customers.save",
			`{"id":"`+cust+`","name":"`+name+`","organizationId":"`+org+`","serviceProfile":"general","status":"inactive"}`,
			version,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the deactivation did not wait for the unit save: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 409 || got.resp["messageKey"] != apperr.KeyCustomerActive {
		t.Errorf("a deactivation with a unit added at the same moment: %d %v", got.code, got.resp)
	}
}
