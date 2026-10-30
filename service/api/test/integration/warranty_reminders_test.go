package integration

import (
	"context"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// BR-A19 / IR340: the equipment scheduler reminds HQ (asset.read) 90 and 30 days before a unit's warranty ends while
// no active contract covers it: the latest reminder reached, once per warranty end, and a changed warranty end gets
// its own. Archived units, covered units, ended warranties and ends beyond 90 days get none; clients get none.
func TestWarrantyReminders(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	day := 24 * time.Hour
	var units []string
	unit := func(ends time.Duration) string {
		t.Helper()
		u := newUnit(t, s, "Warranty AC")
		owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, u, clock.Add(ends))
		units = append(units, u)
		return u
	}
	t.Cleanup(func() { owner(t, `UPDATE assets.units SET archived = true WHERE id = ANY($1::uuid[])`, units) })
	tick := 0
	evaluate := func() {
		t.Helper()
		tick++
		if _, err := schedTick(ctx, s, clock.Add(time.Duration(tick)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	type reminder struct {
		days, notifications int
		severity            string
	}
	sent := func(u string) []reminder { // the unit's reminders for its current warranty end, the earliest first
		t.Helper()
		var n int
		ownerScan(t, `SELECT count(*) FROM assets.warranty_reminders r JOIN assets.units u ON u.id = r.unit_id AND u.warranty_ends_at = r.warranty_ends_at WHERE r.unit_id = $1`, []any{u}, &n)
		out := make([]reminder, n)
		for i := range out {
			ownerScan(t, `SELECT r.days_before, r.notifications, coalesce(string_agg(DISTINCT n.severity, ','), '') FROM assets.warranty_reminders r
				JOIN assets.units u ON u.id = r.unit_id AND u.warranty_ends_at = r.warranty_ends_at
				LEFT JOIN notify.notifications n ON n.target->>'id' = r.unit_id::text AND n.type = 'warranty_expiring' AND n.template_key = 'warranty_expiring' AND n.occurred_at = r.sent_at
				WHERE r.unit_id = $1 GROUP BY r.days_before, r.notifications ORDER BY r.days_before DESC OFFSET $2 LIMIT 1`, []any{u, i}, &out[i].days, &out[i].notifications, &out[i].severity)
		}
		return out
	}
	soon, near, far, ended := unit(59*day), unit(20*day), unit(100*day), unit(-day)
	archived := unit(20 * day)
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, archived)
	covered := unit(20 * day)
	if code, m := write(s, &hq, "contracts.save", `{"customerId":"`+seed.ID("cust-b").String()+`","unitIds":["`+covered+`"],"planType":"general","startAt":"`+
		clock.Add(-30*day).Format(time.RFC3339)+`","endAt":"`+clock.Add(365*day).Format(time.RFC3339)+`","priceMinor":5000,"currency":"MYR","restrictionEligible":false,"rulesVersion":null}`, 0); code != 200 {
		t.Fatalf("contract: %d %v", code, m)
	}
	evaluate()
	var admins int
	ownerScan(t, `SELECT count(*) FROM identity.memberships m WHERE m.role = 'admin' AND m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)
		AND EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND p.permission = 'asset.read') AND m.tenant_id = $2`, []any{clock.Add(time.Minute), seed.ID("tenant-a")}, &admins)
	if r := sent(soon); admins == 0 || len(r) != 1 || r[0].days != 90 || r[0].notifications != admins || r[0].severity != "normal" {
		t.Fatalf("90 days: %d admins, %+v", admins, r)
	}
	if r := sent(near); len(r) != 1 || r[0].days != 30 || r[0].severity != "warning" {
		t.Fatalf("20 days left: only the 30-day reminder: %+v", r)
	}
	for name, u := range map[string]string{"100 days": far, "ended": ended, "archived": archived, "covered by a contract": covered} {
		if r := sent(u); len(r) != 0 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	var audits int
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'units.remindWarranty' AND target_id = $1 AND reason = '90 days before the warranty ends'`, []any{soon}, &audits)
	if audits != 1 {
		t.Errorf("audit: %d", audits)
	}
	// once each; a moved warranty end gets its own reminders: the 30-day one after the old end's 90-day one
	evaluate()
	if r := sent(soon); len(r) != 1 {
		t.Fatalf("repeated: %+v", r)
	}
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, soon, clock.Add(29*day))
	evaluate()
	if r := sent(soon); len(r) != 1 || r[0].days != 30 {
		t.Fatalf("30 days: %+v", r)
	}
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, soon, clock.Add(29*day+time.Hour)) // a new end, still 30 days away
	evaluate()
	if r := sent(soon); len(r) != 1 || r[0].days != 30 {
		t.Fatalf("a changed end: %+v", r)
	}
	var total int
	ownerScan(t, `SELECT count(*) FROM assets.warranty_reminders WHERE unit_id = $1`, []any{soon}, &total)
	if total != 3 {
		t.Errorf("reminders for the unit's three warranty ends: %d", total)
	}
	// HQ reads the reminder about the unit in its inbox
	_, m := post(s, &hq, "notifications.list", `{"filters":{"type":"warranty_expiring"},"limit":100}`)
	var mine map[string]any
	for _, it := range items(m) {
		if it["target"].(map[string]any)["id"] == near {
			mine = it
		}
	}
	if mine == nil {
		t.Fatalf("not listed: %v", m)
	}
	params := mine["params"].(map[string]any)
	if at, _ := time.Parse(time.RFC3339, params["at"].(string)); !at.Equal(clock.Add(20*day)) || params["status"] != "expiring" || mine["templateKey"] != "warranty_expiring" {
		t.Errorf("notification: %v", mine)
	}
	if code, m := post(s, &customerB, "notifications.list", `{"filters":{"type":"warranty_expiring"},"limit":100}`); code != 200 || len(items(m)) != 0 {
		t.Errorf("a client receives none: %d %v", code, m)
	}
}
