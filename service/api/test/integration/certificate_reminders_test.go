package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// BR-P09 / IR336: the maintenance scheduler reminds the contractor admins (partner.assign) of a valid certificate 60,
// 30 and 7 days before it expires, each reminder once and never an earlier one after a later one. Certificates that
// wait for HQ, were rejected, have expired, were replaced by a valid renewal or a later certificate of the same
// qualification, or whose technician left get none; a pending renewal does not stop them. The contractor reads the
// notifications of its own technicians' certificates only.
func TestCertificateReminders(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tenant, org := seed.ID("tenant-a"), seed.ID("org-contractor-a")
	day := 24 * time.Hour
	member := func(role string, from, until time.Time) string { // a new membership of contractor-a, without permissions
		t.Helper()
		id := uuid.NewString()
		owner(t, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from, valid_until) VALUES ($1,$2,$3,$4,$5::text,
			CASE WHEN $5::text = 'technician' THEN 'external' END,$6,$7)`, id, tenant, newUser(t), org, role, from, until)
		return id
	}
	tech := member("technician", clock.Add(-30*day), clock.Add(365*day))
	gone := member("technician", clock.Add(-30*day), clock.Add(-time.Hour)) // left the company
	viewer := member("contractor", clock.Add(-30*day), clock.Add(365*day))  // a contractor member without partner.assign
	// later tests see neither the memberships nor their reminders
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = valid_from + interval '1 minute' WHERE id = ANY($1::uuid[])`, []string{tech, viewer})
	})
	var certs []string
	cert := func(membership, code, status string, expires time.Time, renewalOf any) string {
		t.Helper()
		id := uuid.NewString()
		owner(t, `INSERT INTO maintenance.certificates (id, tenant_id, membership_id, organization_id, code, name, number, issued_at, expires_at, status, renewal_of)
			VALUES ($1,$2,$3,$4,$5,'Indoor AC','N-1',$6,$7,$8,$9)`, id, tenant, membership, org, code, clock.Add(-365*day), expires, status, renewalOf)
		certs = append(certs, id)
		return id
	}
	t.Cleanup(func() {
		owner(t, `UPDATE maintenance.certificates SET status = 'rejected' WHERE id = ANY($1::uuid[])`, certs)
	})
	tick := 0
	evaluate := func() { // a scheduler tick a minute later each time
		t.Helper()
		tick++
		drainEvents(t)
		if _, err := schedTick(ctx, s, clock.Add(time.Duration(tick)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	type reminder struct {
		days, notifications int
		severity, to        string
	}
	sent := func(id string) []reminder { // the certificate's reminders, the earliest first, with their notifications
		t.Helper()
		var n int
		ownerScan(t, `SELECT count(*) FROM maintenance.certificate_reminders WHERE certificate_id = $1`, []any{id}, &n)
		out := make([]reminder, n)
		for i := range out {
			ownerScan(t, `SELECT r.days_before, r.notifications, coalesce(string_agg(DISTINCT n.severity, ','), ''), coalesce(string_agg(n.recipient_membership_id::text, ','), '')
				FROM maintenance.certificate_reminders r LEFT JOIN notify.notifications n ON n.target->>'id' = r.certificate_id::text AND n.type = 'certificate_expiry'
				  AND n.template_key = 'certificate_expiry' AND n.occurred_at = r.sent_at
				WHERE r.certificate_id = $1 GROUP BY r.days_before, r.notifications ORDER BY r.days_before DESC OFFSET $2 LIMIT 1`, []any{id, i},
				&out[i].days, &out[i].notifications, &out[i].severity, &out[i].to)
		}
		return out
	}
	admin := seed.ID("contractor-a").String()

	// 60 days before: one reminder, to the contractor admin only
	main := cert(tech, "demo_indoor", "valid", clock.Add(59*day), nil)
	far := cert(tech, "demo_outdoor", "valid", clock.Add(61*day), nil)
	evaluate()
	if r := sent(main); len(r) != 1 || r[0].days != 60 || r[0].notifications != 1 || r[0].to != admin || r[0].severity != "normal" {
		t.Fatalf("60 days: %+v", r)
	}
	if r := sent(far); len(r) != 0 {
		t.Errorf("61 days: %+v", r)
	}
	var audits int
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'certificates.remind' AND target_id = $1 AND reason = '60 days before expiry'`, []any{main}, &audits)
	if audits != 1 {
		t.Errorf("audit: %d", audits)
	}
	// the same reminder is never repeated; 30 and 7 days follow (7 days is a warning); then nothing more
	evaluate()
	if r := sent(main); len(r) != 1 {
		t.Fatalf("repeated: %+v", r)
	}
	owner(t, `UPDATE maintenance.certificates SET expires_at = $2 WHERE id = $1`, main, clock.Add(29*day))
	evaluate()
	owner(t, `UPDATE maintenance.certificates SET expires_at = $2 WHERE id = $1`, main, clock.Add(6*day))
	evaluate()
	evaluate()
	if r := sent(main); len(r) != 3 || r[1].days != 30 || r[1].severity != "normal" || r[2].days != 7 || r[2].severity != "warning" || r[2].to != admin {
		t.Fatalf("30 and 7 days: %+v", r)
	}
	// a later reminder already sent: no earlier one (the expiry moved back)
	owner(t, `UPDATE maintenance.certificates SET expires_at = $2 WHERE id = $1`, main, clock.Add(59*day))
	evaluate()
	if r := sent(main); len(r) != 3 {
		t.Fatalf("earlier after later: %+v", r)
	}

	// a certificate approved with 20 days left gets the 30-day reminder only
	late := cert(tech, "demo_electrical", "valid", clock.Add(20*day), nil)
	// none for a pending, rejected or expired certificate, a replaced one or a technician who left
	pending := cert(tech, "other", "pending_verification", clock.Add(20*day), nil)
	rejected := cert(tech, "other", "rejected", clock.Add(20*day), nil)
	expired := cert(tech, "other", "valid", clock, nil)
	left := cert(gone, "demo_indoor", "valid", clock.Add(20*day), nil)
	renewed := cert(tech, "other", "valid", clock.Add(10*day), nil)
	cert(tech, "other", "valid", clock.Add(400*day), renewed)          // its approved renewal
	older := cert(tech, "demo_indoor", "valid", clock.Add(3*day), nil) // main expires later
	// a pending renewal does not stop the reminders; another "other" certificate is not a renewal
	waiting := cert(tech, "other", "valid", clock.Add(25*day), nil)
	cert(tech, "other", "pending_verification", clock.Add(400*day), waiting)
	unrelated := cert(tech, "other", "valid", clock.Add(5*day), nil)
	evaluate()
	if r := sent(late); len(r) != 1 || r[0].days != 30 {
		t.Errorf("approved late: %+v", r)
	}
	for name, id := range map[string]string{"pending": pending, "rejected": rejected, "expired": expired, "left": left, "renewed": renewed, "older": older} {
		if r := sent(id); len(r) != 0 {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r := sent(waiting); len(r) != 1 || r[0].days != 30 {
		t.Errorf("pending renewal: %+v", r)
	}
	if r := sent(unrelated); len(r) != 1 || r[0].days != 7 {
		t.Errorf("another other certificate: %+v", r)
	}

	// the contractor's inbox: titled by the certificate, its expiry in params.at; contractor-b reads none of them
	code, m := post(s, &contrA, "notifications.list", `{"filters":{"type":"certificate_expiry"},"limit":100}`)
	if code != 200 {
		t.Fatalf("list: %d %v", code, m)
	}
	var mine map[string]any
	for _, it := range items(m) {
		if target := it["target"].(map[string]any); target["kind"] == "certificate" && target["id"] == late {
			mine = it
		}
	}
	if mine == nil {
		t.Fatalf("the reminder is not listed: %v", m)
	}
	params := mine["params"].(map[string]any)
	if at, _ := time.Parse(time.RFC3339, params["at"].(string)); params["targetName"] != "Test user · Indoor AC" || !at.Equal(clock.Add(20*day)) || params["status"] != "expiring" ||
		mine["severity"] != "normal" || mine["templateKey"] != "certificate_expiry" {
		t.Errorf("notification: %v", mine)
	}
	if code, m := write(s, &contrA, "notifications.markRead", `{"id":"`+mine["id"].(string)+`"}`, int(mine["version"].(float64))); code != 200 || data(m)["readAt"] == nil {
		t.Errorf("markRead: %d %v", code, m)
	}
	// IR58: a notification of contractor-a's certificate addressed to contractor-b stays hidden from contractor-b
	stray := uuid.NewString()
	owner(t, `INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, template_key, target, params, severity, occurred_at)
		VALUES ($1,$2,$3,1,'certificate_expiry','certificate_expiry',jsonb_build_object('kind','certificate','id',$4::text),'{}','normal',$5)`,
		stray, tenant, seed.ID("contractor-b"), late, clock)
	t.Cleanup(func() { owner(t, `DELETE FROM notify.notifications WHERE id = $1`, stray) })
	_, m = post(s, &contrB, "notifications.list", `{"filters":{"type":"certificate_expiry"},"limit":100}`)
	for _, it := range items(m) {
		if it["id"] == stray {
			t.Error("contractor-b reads contractor-a's certificate")
		}
	}
	if code, _ := write(s, &contrB, "notifications.markRead", `{"id":"`+stray+`"}`, 1); code != 404 {
		t.Errorf("contractor-b marks it read: %d", code)
	}
}
