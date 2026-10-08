package app

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func countNotes(t *testing.T, target, recipient, template string) int {
	t.Helper()
	var n int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2 AND template_key = $3`,
		[]any{target, seed.ID(recipient), template}, &n)
	return n
}

func TestBusinessEventNotifications(t *testing.T) {
	s := server(t)
	// job.requested by the client: HQ job.write holders, not the acting client
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, seed.ID("tech-internal-a"))
	code, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	if countNotes(t, job, "hq-operator", "job_update") != 1 || countNotes(t, job, "customer-a", "job_update") != 0 {
		t.Errorf("job.requested: hq %d, actor %d", countNotes(t, job, "hq-operator", "job_update"), countNotes(t, job, "customer-a", "job_update"))
	}
	// job.assigned: technician and client get schedule_change; the acting HQ member does not
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(25)+`","endAt":"`+ts(27)+`"}`, 1); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	if countNotes(t, job, "tech-internal-a", "schedule_change") != 1 || countNotes(t, job, "customer-a", "schedule_change") != 1 || countNotes(t, job, "hq-operator", "schedule_change") != 0 ||
		countNotes(t, job, "hq-restriction-manager", "schedule_change") != 1 {
		t.Error("job.assigned recipients")
	}
	// job.cancelled
	_, g := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if code, m := write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"no longer needed"}`, ver(g)); code != 200 {
		t.Fatalf("cancel: %d %v", code, m)
	}
	if countNotes(t, job, "tech-internal-a", "job_update") != 1 || countNotes(t, job, "customer-a", "job_update") != 1 || countNotes(t, job, "hq-operator", "job_update") != 1 ||
		countNotes(t, job, "hq-restriction-manager", "job_update") != 2 {
		t.Errorf("job.cancelled: tech %d client %d hq %d", countNotes(t, job, "tech-internal-a", "job_update"), countNotes(t, job, "customer-a", "job_update"), countNotes(t, job, "hq-operator", "job_update"))
	}
	// restriction transition and payment confirmation
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	rid, _ := executed(t, s, k, inv, []string{u})
	if countNotes(t, rid, "customer-b", "restriction") != 2 || countNotes(t, rid, "hq-operator", "restriction") != 0 || countNotes(t, rid, "hq-restriction-manager", "restriction") != 0 {
		t.Errorf("restriction notices: client %d hq %d actor %d", countNotes(t, rid, "customer-b", "restriction"), countNotes(t, rid, "hq-operator", "restriction"),
			countNotes(t, rid, "hq-restriction-manager", "restriction"))
	}
	if code, m := write(s, &hq, "payments.recordManual", `{"invoiceId":"`+inv+`","paymentReference":"BANK-`+rid[:8]+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"bank"}`, 1); code != 200 {
		t.Fatalf("payment: %d %v", code, m)
	}
	if countNotes(t, inv, "customer-b", "payment") != 1 || countNotes(t, inv, "hq-restriction-manager", "payment") != 1 || countNotes(t, inv, "hq-operator", "payment") != 0 {
		t.Error("payment.confirmed recipients")
	}
	if countNotes(t, rid, "customer-b", "restriction") != 3 {
		t.Error("release_requested after payment notifies the client")
	}
	var sev string
	ownerScan(t, `SELECT severity FROM notify.notifications WHERE target->>'id' = $1 AND template_key = 'restriction' ORDER BY created_at DESC LIMIT 1`, []any{rid}, &sev)
	if sev != "warning" {
		t.Errorf("restriction severity %s", sev)
	}
}
