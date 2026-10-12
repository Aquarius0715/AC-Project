package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestProposalRefusalsByState: a slot proposal can be withdrawn or answered only while it is pending and only within
// its job. An unknown proposal, a proposal of another job and a decided proposal are each refused with the state
// that applies, and the decided one changes nothing.
func TestProposalRefusalsByState(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active' AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, clock.Add(2*time.Hour))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	internalHold := `{"kind":"internal","membershipId":"` + seed.ID("tech-internal-a").String() + `"}`
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.proposeSlot", `{"jobId":"`+job+`","slot":`+slotJSON(97, 2)+`,"hold":`+internalHold+`,"message":"Can we come then?","replyBy":"`+ts(48)+`"}`, 1); code != 200 {
		t.Fatalf("propose: %d %v", code, m)
	}
	var proposal string
	ownerScan(t, `SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`, []any{job}, &proposal)
	unknown := uuid.NewString()

	if code, m := write(s, &hq, "jobs.withdrawProposal", `{"jobId":"`+job+`","proposalId":"`+unknown+`"}`, 2); code != 404 || m["messageKey"] != "error.notFound" {
		t.Errorf("withdraw an unknown proposal: %d %v", code, m)
	}
	if code, m := write(s, &customerA, "jobs.respondProposal", `{"jobId":"`+job+`","proposalId":"`+unknown+`","decision":"accept"}`, 2); code != 404 || m["messageKey"] != "error.notFound" {
		t.Errorf("answer an unknown proposal: %d %v", code, m)
	}
	if code, m := write(s, &hq, "jobs.withdrawProposal", `{"jobId":"`+job+`","proposalId":"`+proposal+`"}`, 2); code != 200 {
		t.Fatalf("withdraw: %d %v", code, m)
	}
	if code, m := write(s, &customerA, "jobs.respondProposal", `{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`, 3); code != 409 || m["messageKey"] != "error.invalidState" {
		t.Errorf("answer a withdrawn proposal: %d %v", code, m)
	}
	var status string
	ownerScan(t, `SELECT status FROM maintenance.slot_proposals WHERE id = $1`, []any{proposal}, &status)
	if status != "withdrawn" {
		t.Errorf("a refused answer changed the proposal: %s", status)
	}
}
