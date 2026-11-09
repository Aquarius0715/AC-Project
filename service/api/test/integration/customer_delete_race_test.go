package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type saveResult struct {
	code int
	resp map[string]any
}

// A property or unit is saved into a customer only while the customer row is read with a share lock, so a delete of the
// customer (it locks the row, then looks for units and deletes the locations) and the save never both pass: the save
// waits for the delete and finds no customer, instead of leaving a property or unit of a deleted customer.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCustomerDeleteWaitsForASaveIntoTheCustomer(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	name := "Race Customer " + uuid.NewString()[:8]

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

	cust := asStr(t, data(resp)["id"])

	t.Cleanup(func() {
		owner(t, `DELETE FROM assets.properties WHERE customer_org_id = $1`, org)
		owner(t, `DELETE FROM assets.customers WHERE id = $1`, cust)
		owner(t, `DELETE FROM identity.organizations WHERE id = $1`, org)
	})

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// the customer delete, half done: it holds the customer row and has removed it
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM assets.customers WHERE id = $1`, cust)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"properties.save",
			`{"customerOrgId":"`+org+`","kind":"office","name":"Late site","address":null,"accessInstructions":null}`,
			0,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the save did not wait for the customer delete: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 404 {
		t.Errorf("a save into a deleted customer: %d %v", got.code, got.resp)
	}
}
