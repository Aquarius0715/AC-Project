package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A unit is saved with a model only while the model row is read with a share lock, so the delete of the model (it
// locks the row, then looks for units) and the save never both pass: the save waits for the delete and finds no model,
// instead of leaving a unit whose model no longer exists.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestModelDeleteWaitsForAUnitSavedWithIt(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	code, resp := write(srv, hq(), "capabilities.save", capBody("", "TA-"+suffix, ""), 0)
	if code != 200 {
		t.Fatalf("model: %d %v", code, resp)
	}

	model := asStr(t, data(resp)["id"])

	t.Cleanup(func() {
		owner(t, `DELETE FROM assets.units WHERE display_name LIKE '%'||$1`, suffix)
		owner(t, `DELETE FROM devices.capabilities WHERE id = $1`, model)
	})

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// the model delete, half done: it holds the model row and has removed it
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM devices.capabilities WHERE id = $1`, model)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"units.save",
			`{"customerOrgId":"`+seed.ID("org-customer-b").
				String()+
				`","propertyId":"`+seed.ID("property-home-b").
				String()+
				`","spaceId":null,"displayName":"Late unit `+suffix+`","modelId":"`+model+
				`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
			0,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the save did not wait for the model delete: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 422 || fields(got.resp) != "modelId=error.unknownModel" {
		t.Errorf("a save with a deleted model: %d %v", got.code, got.resp)
	}
}
