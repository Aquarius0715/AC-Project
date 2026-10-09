package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// poweredUnit is a new customer-b unit, online, whose power sensor reported "on" onAgo before clock and "off" offAgo
// before clock (IR134 item 1: it ran onAgo − offAgo hours).
func poweredUnit(t *testing.T, name string, onAgo, offAgo time.Duration) string {
	t.Helper()
	tenant := seed.ID("tenant-a")
	u := newUnit(t, lastServer, name)
	owner(t, `UPDATE assets.units SET connection = 'online' WHERE id = $1`, u)
	dev, sensor := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'v1')`, dev, tenant, "FR-"+dev[:8], u, seed.ID("hq-operator"))
	owner(t, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, stale_after_seconds) VALUES ($1,$2,$3,'power','kW',1000000)`, sensor, tenant, dev)
	for i, r := range []struct {
		ago time.Duration
		v   float64
	}{{onAgo, 1.2}, {offAgo, 0}} {
		owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
			VALUES ($1,$2,$3,'power',$4,$5,$6,'kW','measured','valid',$4,$7)`, tenant, u, sensor, clock.Add(-r.ago), i+1, r.v, uuid.NewString())
	}
	return u
}

// IR134 item 5 / IR239 / BR-C18: the maintenance scheduler reminds once per cleaning cycle — a maintenance Alert
// (normal, “Cleaning due … Not a fault.”) and one cleaning_due notification per recipient and channel of the customer's
// settings; Mark cleaned resolves it at once, a technician cleaning at the next evaluation; no recipient leaves a
// no_recipient DeliveryFailure on the Alert.
func TestFilterReminders(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tenant, org := seed.ID("tenant-a"), seed.ID("org-customer-b")
	evaluate := func(at time.Time) { // the next tick evaluates the filters (the job runs every 15 minutes of business time)
		t.Helper()
		owner(t, `DELETE FROM maintenance.filter_reminder_watermarks WHERE tenant_id = $1`, tenant)
		if _, err := schedTick(ctx, s, at); err != nil {
			t.Fatal(err)
		}
	}
	type alert struct {
		id, status, severity, ruleKey, evidence, reason string
		failures                                        int
	}
	alertsOf := func(u string) []alert {
		t.Helper()
		var n int
		ownerScan(t, `SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'maintenance'`, []any{u}, &n)
		out := make([]alert, n)
		for i := range out {
			ownerScan(t, `SELECT id::text, status, severity, coalesce(rule_key, ''), evidence_text, coalesce(resolution_reason, ''), jsonb_array_length(delivery_failures)
				FROM monitoring.alerts WHERE unit_id = $1 AND type = 'maintenance' ORDER BY created_at, id OFFSET $2 LIMIT 1`, []any{u, i},
				&out[i].id, &out[i].status, &out[i].severity, &out[i].ruleKey, &out[i].evidence, &out[i].reason, &out[i].failures)
		}
		return out
	}
	notes := func(alertID string) (total, recipients, email int) {
		t.Helper()
		ownerScan(t, `SELECT count(*), count(DISTINCT recipient_membership_id), count(*) FILTER (WHERE channel = 'email') FROM notify.notifications
			WHERE source_alert_id = $1 AND type = 'cleaning_due' AND template_key = 'alert' AND severity = 'normal'`, []any{alertID}, &total, &recipients, &email)
		return
	}
	settings := func(body string) {
		t.Helper()
		if code, m := write(s, &customerB, "filterCare.saveSettings", body, 0); code != 200 {
			t.Fatalf("settings: %d %v", code, m)
		}
	}
	owner(t, `DELETE FROM maintenance.filter_care_settings WHERE customer_id = $1`, seed.ID("cust-b"))
	settings(`{"thresholdHours":100,"fallbackDays":30,"recipients":"owners","channels":["inApp"]}`)
	u := poweredUnit(t, "Reminder AC", 210*time.Hour, 10*time.Hour) // 200 h, never cleaned
	var units []string
	units = append(units, u)
	t.Cleanup(func() { // later tests see neither the units nor their open reminders
		owner(t, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = $2 WHERE unit_id = ANY($1::uuid[]) AND status <> 'resolved'`, units, clock)
		owner(t, `UPDATE assets.units SET archived = true WHERE id = ANY($1::uuid[])`, units)
	})

	// overdue → one reminder: the Alert opens and the owner gets one in-app notification
	evaluate(clock.Add(time.Minute))
	as := alertsOf(u)
	if len(as) != 1 || as[0].status != "open" || as[0].severity != "normal" || as[0].ruleKey != "filter_cleaning" ||
		as[0].evidence != "Cleaning due (200 h of run time, no cleaning recorded). Not a fault." || as[0].failures != 0 {
		t.Fatalf("raised: %+v", as)
	}
	first := as[0].id
	if total, recipients, email := notes(first); total != 1 || recipients != 1 || email != 0 {
		t.Fatalf("owner notification: %d %d %d", total, recipients, email)
	}
	var to string
	ownerScan(t, `SELECT recipient_membership_id::text FROM notify.notifications WHERE source_alert_id = $1`, []any{first}, &to)
	if to != seed.ID("customer-b").String() {
		t.Errorf("recipient %s", to)
	}
	if code, m := post(s, &customerB, "alerts.list", `{"filters":{"unitId":"`+u+`"}}`); code != 200 || len(items(m)) != 1 || items(m)[0]["type"] != "maintenance" {
		t.Fatalf("alerts.list: %d %v", code, m)
	}
	var audits int
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'filterCare.remind' AND target_id = $1`, []any{u}, &audits)
	if audits != 1 {
		t.Errorf("audit: %d", audits)
	}
	// the same cycle is reminded once; within 15 minutes nothing is evaluated
	if _, err := schedTick(ctx, s, clock.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	evaluate(clock.Add(20 * time.Minute))
	if as := alertsOf(u); len(as) != 1 {
		t.Fatalf("repeated: %+v", as)
	}
	if total, _, _ := notes(first); total != 1 {
		t.Fatalf("repeated notifications: %d", total)
	}

	// Mark cleaned ends the cycle and resolves the Alert at once
	if code, m := write(s, &customerB, "filterCare.markCleaned", `{"unitId":"`+u+`"}`, 0); code != 200 {
		t.Fatalf("mark cleaned: %d %v", code, m)
	}
	drainEvents(t)
	if as := alertsOf(u); as[0].status != "resolved" || as[0].reason != "filter cleaned" {
		t.Fatalf("resolved: %+v", as)
	}

	// a new cycle (cleaned 150 h ago: 140 h run) reminds again — now all client users of the customer by app and e-mail
	member, user := uuid.NewString(), newUser(t)
	owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,$2,$3,$4,'client','member',$5)`,
		member, tenant, user, org, clock.Add(-time.Hour))
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = valid_from + interval '1 minute' WHERE id = $1`, member)
	})
	settings(`{"thresholdHours":100,"fallbackDays":30,"recipients":"all_users","channels":["inApp","email"]}`)
	owner(t, `UPDATE maintenance.filter_cleanings SET cleaned_at = $2 WHERE unit_id = $1`, u, clock.Add(-150*time.Hour))
	at := clock.Add(30 * time.Minute)
	evaluate(at)
	as = alertsOf(u)
	if len(as) != 2 || as[1].status != "open" || as[1].evidence != "Cleaning due (140 h of run time since the last cleaning). Not a fault." {
		t.Fatalf("second cycle: %+v", as)
	}
	var clients int
	ownerScan(t, `SELECT count(*) FROM identity.memberships WHERE organization_id = $1 AND role = 'client' AND valid_from <= $2 AND (valid_until IS NULL OR valid_until > $2)`,
		[]any{org, at}, &clients)
	if total, recipients, email := notes(as[1].id); clients < 2 || recipients != clients || total != 2*clients || email != clients {
		t.Fatalf("all users by app and e-mail: %d clients → %d %d %d", clients, total, recipients, email)
	}

	// a technician cleaning (accepted report, filter normal) ends the cycle at the next evaluation
	job := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at, completed_at)
		VALUES ($1,$2,$3,$4,'preventive','completed','client_request',tstzrange($5,$6),$6,$6)`, job, tenant, u, org, clock.Add(-2*time.Hour), clock.Add(-time.Hour))
	owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items) VALUES ($1, 1, $2, $3, $4, 'accepted', '[{"componentKey":"filter","result":"normal"}]')`,
		uuid.NewString(), tenant, job, seed.ID("user-tech-internal-a"))
	evaluate(clock.Add(45 * time.Minute))
	if as := alertsOf(u); len(as) != 2 || as[1].status != "resolved" || as[1].reason != "filter cleaned" {
		t.Fatalf("technician cleaning: %+v", as)
	}

	// no recipient (no owner left): the Alert opens with a no_recipient DeliveryFailure and no notification
	settings(`{"thresholdHours":null,"fallbackDays":30,"recipients":"owners","channels":["inApp"]}`)
	offline := newUnit(t, s, "Offline reminder AC")
	units = append(units, offline)
	owner(t, `INSERT INTO maintenance.filter_cleanings (tenant_id, unit_id, cleaned_at, marked_by_membership_id) VALUES ($1, $2, $3, $4)`,
		tenant, offline, clock.Add(-40*24*time.Hour), seed.ID("customer-b"))
	owner(t, `UPDATE identity.memberships SET client_role = 'member' WHERE id = $1`, seed.ID("customer-b"))
	defer owner(t, `UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`, seed.ID("customer-b"))
	evaluate(clock.Add(time.Hour))
	as = alertsOf(offline)
	if len(as) != 1 || as[0].evidence != "Cleaning due (40 days since the last cleaning, run time unknown). Not a fault." || as[0].failures != 1 {
		t.Fatalf("no recipient: %+v", as)
	}
	if total, _, _ := notes(as[0].id); total != 0 {
		t.Fatalf("notifications without recipients: %d", total)
	}
	var sent int
	ownerScan(t, `SELECT notifications FROM maintenance.filter_reminders WHERE id = $1`, []any{as[0].id}, &sent)
	if sent != 0 {
		t.Errorf("reminder notifications %d", sent)
	}
}
