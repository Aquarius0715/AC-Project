package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestAlertEscalation (IR784, DD-A05): an alert opened by a policy that nobody acknowledges within the policy's
// escalation time tells the HQ members who hold alert.policy.write, once; an alert acknowledged in time is left alone.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAlertEscalation(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	tenant, unit, org := seed.ID("tenant-a"), seed.ID("unit-non-rto"), seed.ID("org-customer-a")

	var (
		policy string
		before *int
	)
	ownerScan(
		t,
		`SELECT id::text, escalate_after_minutes FROM monitoring.alert_policies WHERE kind = 'default_alert' LIMIT 1`,
		nil,
		&policy,
		&before,
	)
	owner(
		t,
		`UPDATE monitoring.alert_policies SET escalate_after_minutes = 30 WHERE id = $1`,
		policy,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE monitoring.alert_policies SET escalate_after_minutes = $2 WHERE id = $1`,
			policy,
			before,
		)
		owner(t, `DELETE FROM monitoring.alerts WHERE rule_key LIKE 'escalation-test-%'`)
	})

	open := func(status string) string {
		id := uuid.NewString()
		owner(
			t,
			`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, policy_id, rule_key, type, severity,
				status, cause_code, evidence_kind, evidence_text, observed_at, detected_at)
			VALUES ($1,$2,$3,$4,$5,$6,'sensor','warning',$7,'unknown','demo_observation','test',$8,$8)`,
			id,
			tenant,
			unit,
			org,
			policy,
			"escalation-test-"+id,
			status,
			clock(),
		)

		return id
	}
	pending, acknowledged := open("open"), open("acknowledged")
	escalations := func(alert string) int {
		drainEvents(t)

		var n int
		ownerScan(
			t,
			`SELECT count(*) FROM notify.notifications WHERE source_alert_id = $1 AND params->>'status' = 'escalated'`,
			[]any{alert},
			&n,
		)

		return n
	}

	_, err := schedTick(ctx, srv, clock().Add(20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if escalations(pending) != 0 {
		t.Error("escalated before its time")
	}

	_, err = schedTick(ctx, srv, clock().Add(31*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	first := escalations(pending)
	if first == 0 {
		t.Fatal("the open alert was not escalated after its time")
	}

	_, err = schedTick(ctx, srv, clock().Add(40*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if escalations(pending) != first {
		t.Errorf("the alert was escalated again: %d, then %d", first, escalations(pending))
	}

	if escalations(acknowledged) != 0 {
		t.Error("an acknowledged alert was escalated")
	}

	var hqGot int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE source_alert_id = $1 AND recipient_membership_id = $2`,
		[]any{pending, seed.ID(membershipHQOperator)},
		&hqGot,
	)

	if hqGot == 0 {
		t.Error("HQ (alert.policy.write) was not told")
	}
}
