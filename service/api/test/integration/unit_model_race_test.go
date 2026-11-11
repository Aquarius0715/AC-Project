package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A unit saved while its model gets a new version waits for it and then takes that version (IR767): the model is read
// under a share lock of every version, so the row it waited for no longer being current does not make the model look
// unknown.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestUnitSaveWaitsForANewModelVersion(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	model := "TA-unit-" + uuid.NewString()[:6]

	code, resp := write(srv, hq(), "capabilities.save", capBody("", model, ""), 0)
	if code != 200 {
		t.Fatalf("model: %d %v", code, resp)
	}

	modelID := asStr(t, data(resp)["id"])

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

	holdNextVersion(ctx, t, tx, "devices.capabilities", modelID)

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"units.save",
			`{"customerOrgId":"`+seed.ID("org-customer-b").String()+
				`","propertyId":"`+seed.ID("property-home-b").String()+`","spaceId":null,"displayName":"Model race `+
				uuid.NewString()[:8]+`","modelId":"`+modelID+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
			0,
		)
		done <- saveResult{status, reply}
	}()

	waited := false

	var got saveResult
	select {
	case got = <-done:
		t.Errorf("the unit save did not wait for the model: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
		waited = true
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if waited {
		got = <-done
	}

	if got.code != 200 {
		t.Fatalf("a unit saved while its model got a new version: %d %v", got.code, got.resp)
	}

	t.Cleanup(
		func() { owner(t, `DELETE FROM assets.units WHERE id = $1`, asStr(t, data(got.resp)["id"])) },
	)
}
