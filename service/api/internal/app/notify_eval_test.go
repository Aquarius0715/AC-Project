package app

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
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
