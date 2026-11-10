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

// A rename to a name that a sibling takes at the same moment is CONFLICT error.duplicateSiblingName, as a save is: the
// sibling check of locations.rename runs before the update and does not see a property that is not committed yet, so
// the unique index (properties_name) refuses the update when the other one commits.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRenameToANameTakenAtTheSameMomentIsAConflict(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	org := seed.ID("org-customer-b").String()
	suffix := uuid.NewString()[:8]

	code, resp := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"office","name":"Rename me `+suffix+`","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 {
		t.Fatalf("property: %d %v", code, resp)
	}

	prop, version := asStr(t, data(resp)["id"]), ver(t, resp)

	t.Cleanup(func() {
		owner(
			t,
			`DELETE FROM assets.properties WHERE customer_org_id = $1 AND name LIKE '%'||$2`,
			org,
			suffix,
		)
	})

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// a sibling with the new name, saved at the same moment: inserted, not committed
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(
		ctx,
		`INSERT INTO assets.properties (tenant_id, customer_org_id, kind, name)
			SELECT tenant_id, customer_org_id, kind, $2 FROM assets.properties WHERE id = $1`,
		prop,
		"Taken "+suffix,
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"locations.rename",
			`{"target":{"kind":"property","id":"`+prop+`"},"name":"taken `+suffix+`"}`,
			version,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the rename did not wait for the sibling: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 409 || got.resp["messageKey"] != apperr.KeyDuplicateSiblingName {
		t.Errorf("a rename to a name taken at the same moment: %d %v", got.code, got.resp)
	}
}
