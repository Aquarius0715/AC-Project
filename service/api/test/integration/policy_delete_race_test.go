package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A unit is given an alert policy only while the policy row is read with a share lock, so the delete of the policy (it
// locks the row, then detaches the units) and the attach never both pass: the attach waits for the delete and finds no
// policy, instead of leaving a unit that carries the ID of a policy that no longer exists.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPolicyDeleteWaitsForAnAttachToAUnit(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	code, resp := write(srv, hq(), "policies.save", alertPolicyBody(nil), 0)
	if code != 200 {
		t.Fatalf("policy: %d %v", code, resp)
	}

	policy := asStr(t, data(resp)["id"])
	unit := seed.ID("unit-non-rto").String()

	t.Cleanup(func() {
		owner(t, `DELETE FROM assets.unit_alert_policies WHERE policy_id = $1`, policy)
		owner(t, `DELETE FROM monitoring.alert_policies WHERE id = $1`, policy)
	})

	_, detail := post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)
	version := ver(t, detail)

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// the policy delete, half done: it holds the policy row and has removed it
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `DELETE FROM monitoring.alert_policies WHERE id = $1`, policy)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			customerA(),
			"units.setAlertPolicies",
			`{"unitId":"`+unit+`","alertPolicyIds":["`+policy+`"]}`,
			version,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the attach did not wait for the policy delete: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 404 {
		t.Errorf("an attach of a deleted policy: %d %v", got.code, got.resp)
	}
}
