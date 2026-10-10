package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestAutomationNotifications(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	u, lonely := newUnit(t, s, "Notify AC"), newUnit(t, s, "Lonely AC")
	cust := seed.ID("cust-b")
	policy := func(unit string, recipients []uuid.UUID) string {
		id := uuid.NewString()
		cond, _ := json.Marshal(map[string]any{"metric": "co2", "operator": "gte", "threshold": 1000, "recoveryThreshold": 900, "durationSeconds": 60, "activeWindow": nil, "severity": "warning"})
		owner(t, `INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, enabled, priority, condition,
			recipient_membership_ids, channels, escalate_after_minutes, cooldown_minutes) VALUES ($1,$2,'alert',$3,'CO2 high',$4,$5,'Asia/Kuala_Lumpur',true,50,$6,$7,'{inApp,email}',60,30)`,
			id, seed.ID("tenant-a"), cust, seed.ID("customer-b"), seed.ID("user-customer-b"), cond, recipients)
		owner(t, `INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES ($1,$2,$3)`, seed.ID("tenant-a"), unit, id)
		return id
	}
	withRecipient := policy(u, []uuid.UUID{seed.ID("customer-b")})
	noRecipient := policy(lonely, []uuid.UUID{seed.ID("customer-a")}) // another customer's client cannot receive it
	def := defaultPolicyID(t)
	fact := func(unit, value string, observed time.Time, quality string) string {
		return `{"unitId":"` + unit + `","metric":"co2","value":` + value + `,"unit":"ppm","observedAt":"` + observed.Format(time.RFC3339) + `","quality":"` + quality + `"}`
	}
	input := func(facts string) string {
		return `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + clock.Format(time.RFC3339) + `","unitIds":["` + u + `","` + lonely + `"],"facts":[` + facts + `],"phase":"condition"}`
	}
	outcome := func(m map[string]any, unit, pol string) map[string]any {
		for _, n := range data(m)["notifications"].([]any) {
			if x := n.(map[string]any); x["unitId"] == unit && x["policyId"] == pol {
				return x
			}
		}
		return nil
	}
	high := fact(u, "1200", clock, "valid") + "," + fact(lonely, "1200", clock, "valid")
	code, m := post(s, &customerB, "automations.simulate", input(high))
	if code != 200 || outcome(m, u, withRecipient)["decision"] != "selected" || outcome(m, u, def)["decision"] != "selected" || outcome(m, lonely, noRecipient)["reason"] != "no_recipient" {
		t.Fatalf("simulate: %d %v", code, m)
	}
	for _, r := range data(m)["results"].([]any) {
		if x := r.(map[string]any); x["reason"] != "no_control_action" {
			t.Errorf("alert-only units: %v", x)
		}
	}
	if _, m := post(s, &customerB, "automations.simulate", input(fact(u, "500", clock, "valid"))); outcome(m, u, withRecipient)["reason"] != "not_due" {
		t.Errorf("below threshold: %v", m)
	}
	if _, m := post(s, &customerB, "automations.simulate", input(fact(u, "1200", clock, "suspect"))); outcome(m, u, withRecipient)["reason"] != "quality" {
		t.Errorf("quality: %v", m)
	}
	var before int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1`, []any{u}, &before)
	if before != 0 {
		t.Fatal("simulate saved notifications")
	}
	code, m = write(s, &customerB, "automations.fire", input(high), 0)
	created := outcome(m, u, withRecipient)
	if code != 200 || created["decision"] != "created" || len(created["notificationIds"].([]any)) != 2 || len(created["failureIds"].([]any)) != 0 {
		t.Fatalf("fire: %d %v", code, m)
	}
	failed := outcome(m, lonely, noRecipient)
	if failed["decision"] != "failed" || len(failed["failureIds"].([]any)) != 1 || len(failed["notificationIds"].([]any)) != 0 {
		t.Errorf("no recipient: %v", failed)
	}
	var alerts, failures int
	ownerScan(t, `SELECT count(*), COALESCE(sum(jsonb_array_length(delivery_failures)), 0) FROM monitoring.alerts WHERE unit_id IN ($1, $2) AND status = 'open'`, []any{u, lonely}, &alerts, &failures)
	if alerts < 2 || failures != 1 {
		t.Errorf("source alerts %d, failures %d", alerts, failures)
	}
	// cooldown after a created notification (30 minutes); the default policy has none
	if _, m := post(s, &customerB, "automations.simulate", input(high)); outcome(m, u, withRecipient)["reason"] != "cooldown" || outcome(m, u, def)["decision"] != "selected" {
		t.Errorf("cooldown: %v", m)
	}
	if _, m := post(s, &customerB, "notifications.list", `{"filters":{"type":"fault"},"limit":100}`); len(items(m)) == 0 {
		t.Error("customer receives the alert notification")
	}
	owner(t, `UPDATE monitoring.alert_policies SET enabled = false WHERE id = $1`, withRecipient)
	if _, m := post(s, &customerB, "automations.simulate", input(high)); outcome(m, u, withRecipient)["reason"] != "disabled" {
		t.Errorf("disabled: %v", m)
	}
}

// D08 / IR66: fired facts past the recovery threshold resolve the open policy alerts of the unit — the attached
// policy's and the default rule's — with the recovery as evidence; a fact between the thresholds, a suspect one and a
// simulation change nothing, an alert without a policy stays open, and the next breach opens a new alert linked to the
// resolved one.
func TestAlertRecoveryResolvesPolicyAlerts(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	u := newUnit(t, s, "Recovery AC")
	pol := uuid.NewString()
	cond, _ := json.Marshal(map[string]any{"metric": "co2", "operator": "gte", "threshold": 1000, "recoveryThreshold": 900, "durationSeconds": 60, "activeWindow": nil, "severity": "warning"})
	owner(t, `INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, enabled, priority, condition,
		recipient_membership_ids, channels) VALUES ($1,$2,'alert',$3,'CO2 high',$4,$5,'Asia/Kuala_Lumpur',true,50,$6,$7,'{inApp}')`,
		pol, seed.ID("tenant-a"), seed.ID("cust-b"), seed.ID("customer-b"), seed.ID("user-customer-b"), cond, []uuid.UUID{seed.ID("customer-b")})
	owner(t, `INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES ($1,$2,$3)`, seed.ID("tenant-a"), u, pol)
	def := defaultPolicyID(t)
	manual := uuid.NewString() // an alert without a policy resolves only by hand (IR66)
	owner(t, `INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at)
		VALUES ($1,$2,$3,$4,'tamper','warning','open','unknown','demo_observation','cover opened',$5,$5)`, manual, seed.ID("tenant-a"), u, seed.ID("org-customer-b"), clock.Add(-time.Hour))
	fire := func(value, quality string, simulate bool) string {
		owner(t, `DELETE FROM control.evaluation_events`) // one result per tick (D02): each call stands for a later tick of the fixed test clock
		ev := uuid.NewString()
		body := `{"eventId":"` + ev + `","occurredAt":"` + clock.Format(time.RFC3339) + `","unitIds":["` + u + `"],"facts":[{"unitId":"` + u + `","metric":"co2","value":` + value +
			`,"unit":"ppm","observedAt":"` + clock.Format(time.RFC3339) + `","quality":"` + quality + `"}],"phase":"condition"}`
		op, code := "automations.fire", 0
		if simulate {
			op = "automations.simulate"
			code, _ = post(s, &customerB, op, body)
		} else {
			code, _ = write(s, &customerB, op, body, 0)
		}
		if code != 200 {
			t.Fatalf("%s %s: %d", op, value, code)
		}
		return ev
	}
	type row struct {
		id, status string
		reason     *string
		evidence   []uuid.UUID
		previous   *uuid.UUID
	}
	of := func(policy string) []row {
		ctx := context.Background()
		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}
		defer conn.Close(ctx)
		rs, err := conn.Query(ctx, `SELECT id, status, resolution_reason, resolution_evidence_ids, previous_alert_id FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 ORDER BY detected_at, created_at`, u, policy)
		if err != nil {
			t.Fatal(err)
		}
		defer rs.Close()
		var out []row
		for rs.Next() {
			var r row
			var id uuid.UUID
			if err := rs.Scan(&id, &r.status, &r.reason, &r.evidence, &r.previous); err != nil {
				t.Fatal(err)
			}
			r.id = id.String()
			out = append(out, r)
		}
		return out
	}
	fire("1200", "valid", false)
	first := of(pol)
	if len(first) != 1 || first[0].status != "open" || len(of(def)) != 1 {
		t.Fatalf("the breach opens the policy's and the default rule's alerts: %v / %v", first, of(def))
	}
	fire("950", "valid", false)  // between the thresholds
	fire("850", "suspect", false) // not a valid reading
	fire("850", "valid", true)    // a simulation saves nothing
	if r := of(pol); r[0].status != "open" {
		t.Fatalf("nothing recovers yet: %v", r)
	}
	ev := fire("850", "valid", false)
	r := of(pol)[0]
	if r.status != "resolved" || r.reason == nil || *r.reason != "Recovered: co2 850 < 900 for 60 s (D08)" || len(r.evidence) != 1 || r.evidence[0].String() != ev {
		t.Fatalf("sustained recovery resolves with the recovery as evidence: %+v", r)
	}
	if d := of(def); d[0].status != "resolved" {
		t.Errorf("the default rule's alert recovers too: %+v", d)
	}
	_, m := post(s, &customerB, "alerts.get", `{"id":"`+manual+`"}`)
	if data(m)["status"] != "open" {
		t.Errorf("an alert without a policy stays open: %v", data(m)["status"])
	}
	fire("1300", "valid", false)
	again := of(pol)
	if len(again) != 2 || again[1].status != "open" || again[1].previous == nil || again[1].previous.String() != first[0].id {
		t.Fatalf("a recurrence opens a new alert linked to the resolved one: %+v", again)
	}
	_, m = post(s, &customerB, "alerts.get", `{"id":"`+again[1].id+`"}`)
	if data(m)["previousAlertId"] != first[0].id {
		t.Errorf("alerts.get shows the link: %v", data(m)["previousAlertId"])
	}
}
