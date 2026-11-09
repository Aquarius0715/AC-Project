package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A rule switch that names version 0 (no stored setting yet) is refused when another switch of the same rule stored the
// setting at the same moment: the second one answers CONFLICT error.versionConflict instead of overwriting the first.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDefaultRuleFirstSwitchesAtTheSameMoment(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	def := newPolicy(t, "cust-a", "default_alert")
	cust := seed.ID("cust-b")
	where := `policy_id = $1 AND rule_key = 'dust_pm25' AND customer_id = $2`

	owner(t, `DELETE FROM monitoring.default_rule_settings WHERE `+where, def, cust)
	t.Cleanup(
		func() { owner(t, `DELETE FROM monitoring.default_rule_settings WHERE `+where, def, cust) },
	)

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// the first switch, half done: it has stored the setting and has not committed
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(
		ctx,
		`INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled,
			changed_by_membership_id) VALUES ($1,$2,'dust_pm25',$3,false,$4)`,
		seed.ID("tenant-a"),
		def,
		cust,
		seed.ID("hq-operator"),
	)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan saveResult, 1)

	go func() {
		status, reply := write(
			srv,
			hq(),
			"policies.setDefaultRule",
			`{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+cust.String()+`","enabled":true}`,
			versionZero,
		)
		done <- saveResult{status, reply}
	}()

	select {
	case got := <-done:
		t.Fatalf("the second switch did not wait for the first: %d %v", got.code, got.resp)
	case <-time.After(700 * time.Millisecond):
	}

	err = tx.Commit(ctx)
	if err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.code != 409 || got.resp["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("a second switch at version 0: %d %v", got.code, got.resp)
	}
}
