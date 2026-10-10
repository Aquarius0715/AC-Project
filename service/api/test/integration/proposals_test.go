package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestSlotProposals(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active' AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, clock.Add(2*time.Hour))
	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, seed.ID("org-contractor-a"))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	internalHold := `{"kind":"internal","membershipId":"` + seed.ID("tech-internal-a").String() + `"}`
	contractorHold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","technicianMembershipId":null}`
	propose := func(job, slot, hold, replyBy string, v int) (int, map[string]any) {
		return write(s, &hq, "jobs.proposeSlot", `{"jobId":"`+job+`","slot":`+slot+`,"hold":`+hold+`,"message":"Can we come then?","replyBy":"`+replyBy+`"}`, v)
	}
	proposalOf := func(job string) string {
		var id string
		ownerScan(t, `SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 ORDER BY (status = 'pending') DESC, created_at DESC LIMIT 1`, []any{job}, &id)
		return id
	}

	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	for name, tc := range map[string]struct {
		slot, hold, reply string
		code              int
	}{
		"preferred slot":       {slotJSON(25, 2), internalHold, ts(48), 422},
		"today":                {slotJSON(3, 1), internalHold, ts(48), 422},
		"reply after 7 days":   {slotJSON(97, 2), internalHold, ts(24 * 8), 422},
		"reply in the past":    {slotJSON(97, 2), internalHold, ts(-1), 422},
		"external as internal": {slotJSON(97, 2), `{"kind":"internal","membershipId":"` + seed.ID("tech-external-a").String() + `"}`, ts(48), 403},
		"bad hold":             {slotJSON(97, 2), `{"kind":"robot"}`, ts(48), 422},
		"unknown contractor":   {slotJSON(97, 2), `{"kind":"contractor","contractorOrgId":"` + uuid.NewString() + `","technicianMembershipId":null}`, ts(48), 404},
	} {
		if code, _ := propose(job, tc.slot, tc.hold, tc.reply, 1); code != tc.code {
			t.Errorf("propose %s: %d want %d", name, code, tc.code)
		}
	}
	if code, m := propose(job, slotJSON(97, 2), internalHold, ts(48), 1); code != 200 || data(m)["slotProposal"].(map[string]any)["status"] != "pending" {
		t.Fatalf("propose: %d %v", code, m)
	}
	if _, m := post(s, &customerA, "jobs.list", `{"filters":{"unitId":"`+unit+`","proposalPending":true},"limit":100}`); byJob(m, job)["displayStatus"] != "time_proposed" {
		t.Fatal("time_proposed display status")
	}
	if code, _ := propose(job, slotJSON(121, 2), internalHold, ts(48), 2); code != 409 {
		t.Error("second pending proposal")
	}
	p1 := proposalOf(job)
	respond := func(a *actor, j, p, body string, v int) (int, map[string]any) {
		return write(s, a, "jobs.respondProposal", `{"jobId":"`+j+`","proposalId":"`+p+`",`+body+`}`, v)
	}
	if code, _ := respond(&customerB, job, p1, `"decision":"accept"`, 2); code != 404 {
		t.Error("other customer responds")
	}
	if code, _ := respond(&customerA, job, p1, `"decision":"decline"`, 2); code != 422 {
		t.Error("decline without reason")
	}
	if code, _ := respond(&customerA, job, p1, `"decision":"decline","declineReason":"not_home","preferredSlots":[`+slotJSON(30, 1)+`]`, 2); code != 422 {
		t.Error("decline with one slot")
	}
	if code, _ := respond(&customerA, job, p1, `"decision":"accept","declineReason":"other"`, 2); code != 422 {
		t.Error("accept with decline fields")
	}
	if code, m := write(s, &hq, "jobs.withdrawProposal", `{"jobId":"`+job+`","proposalId":"`+p1+`"}`, 2); code != 200 || data(m)["slotProposal"].(map[string]any)["status"] != "withdrawn" {
		t.Fatalf("withdraw: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "jobs.withdrawProposal", `{"jobId":"`+job+`","proposalId":"`+p1+`"}`, 3); code != 409 {
		t.Error("withdraw twice")
	}
	propose(job, slotJSON(97, 2), internalHold, ts(48), 3)
	p2 := proposalOf(job)
	if code, m := respond(&customerA, job, p2, `"decision":"accept"`, 4); code != 200 || data(m)["status"] != "assigned" ||
		data(m)["scheduledSlot"].(map[string]any)["startAt"] != ts(97) {
		t.Fatalf("accept internal hold: %d %v", code, m)
	}
	if code, _ := respond(&customerA, job, p2, `"decision":"accept"`, 6); code != 409 {
		t.Error("answer twice")
	}

	// contractor hold without an Offer: accepting creates the Offer at the slot
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	propose(j2, slotJSON(98, 2), contractorHold, ts(48), 1)
	p3 := proposalOf(j2)
	if code, m := respond(&customerA, j2, p3, `"decision":"accept"`, 2); code != 200 || data(m)["status"] != "offered" {
		t.Fatalf("accept contractor hold: %d %v", code, m)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+j2+`"}`); data(m)["offer"].(map[string]any)["visitSlot"].(map[string]any)["startAt"] != ts(98) {
		t.Fatalf("offer at the proposal slot: %v", data(m)["offer"])
	}

	// decline with three new preferred times
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j3 := data(m)["id"].(string)
	propose(j3, slotJSON(99, 2), internalHold, ts(48), 1)
	newSlots := `[` + slotJSON(120, 2) + `,` + slotJSON(144, 2) + `,` + slotJSON(168, 2) + `]`
	if code, m := respond(&customerA, j3, proposalOf(j3), `"decision":"decline","declineReason":"too_late","comment":" evenings only ","preferredSlots":`+newSlots, 2); code != 200 ||
		data(m)["preferenceRound"].(float64) != 2 || data(m)["status"] != "requested" {
		t.Fatalf("decline with slots: %d %v", code, m)
	}

	// expiry by the worker
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j4 := data(m)["id"].(string)
	propose(j4, slotJSON(100, 2), internalHold, ts(2), 1)
	if _, err := schedTick(context.Background(), s, clock.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+j4+`"}`); data(m)["slotProposal"].(map[string]any)["status"] != "expired" {
		t.Fatal("expired proposal")
	}
	if code, _ := respond(&customerA, j4, proposalOf(j4), `"decision":"accept"`, 2); code != 409 {
		t.Error("accept an expired proposal")
	}
}

func TestPartnerProposalsAndReschedule(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active' AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, clock.Add(2*time.Hour))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(200)+`","termsVersion":"t"}`, 1)
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	pp := func(a *actor, slot, tech, reason string, v int) (int, map[string]any) {
		return write(s, a, "jobs.proposePartnerSlot", `{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slot+`,"technicianMembershipId":"`+seed.ID(tech).String()+`","reason":"`+reason+`"}`, v)
	}
	if code, _ := pp(&contrA, slotJSON(101, 2), "tech-internal-a", "busy", 2); code != 403 {
		t.Error("other organization's technician")
	}
	if code, _ := pp(&contrA, slotJSON(101, 2), "tech-external-a", " ", 2); code != 422 {
		t.Error("blank reason")
	}
	if code, _ := pp(&contrB, slotJSON(101, 2), "tech-external-b", "busy", 2); code != 404 {
		t.Error("other contractor")
	}
	if code, m := pp(&contrA, slotJSON(101, 2), "tech-external-a", "team busy that day", 2); code != 200 || data(m)["partnerSlotProposal"] == nil {
		t.Fatalf("partner proposal: %d %v", code, m)
	}
	if code, _ := pp(&contrA, slotJSON(125, 2), "tech-external-a", "again", 3); code != 409 {
		t.Error("second partner proposal")
	}
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`, 3); code != 409 || m["messageKey"] != "errors.partner_proposal_pending" {
		t.Errorf("accept while partner proposal pending: %d %v", code, m)
	}
	var partner string
	ownerScan(t, `SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 ORDER BY sent_at DESC LIMIT 1`, []any{offer}, &partner)
	// the contractor's offer projection shows the pending proposal, when HQ offered and the access period (IR226)
	if code, m := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["projection"] != "offer" {
		t.Fatalf("offer projection: %d %v", code, m)
	} else if o := data(m); o["partnerSlotProposal"] == nil || o["partnerSlotProposal"].(map[string]any)["id"] != partner || o["partnerSlotProposal"].(map[string]any)["status"] != "pending" ||
		o["offeredAt"] == nil || o["accessValidFrom"] != ts(0) || o["accessValidUntil"] != ts(200) {
		t.Fatalf("offer projection fields: %v", o)
	}
	if code, _ := write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client"}`, 3); code != 422 {
		t.Error("send without replyBy")
	}
	if code, _ := write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"keep","replyBy":"`+ts(5)+`"}`, 3); code != 422 {
		t.Error("keep with replyBy")
	}
	if code, m := write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+ts(24)+`"}`, 3); code != 200 ||
		data(m)["slotProposal"].(map[string]any)["source"] != "contractor" {
		t.Fatalf("send to client: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"keep"}`, 4); code != 409 {
		t.Error("resolve twice")
	}
	var proposal string
	ownerScan(t, `SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`, []any{job}, &proposal)
	if code, m := write(s, &customerA, "jobs.respondProposal", `{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`, 4); code != 200 || data(m)["status"] != "offered" {
		t.Fatalf("client accepts partner time: %d %v", code, m)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["offer"].(map[string]any)["visitSlot"].(map[string]any)["startAt"] != ts(101) {
		t.Fatal("offer visit slot moved")
	}
	if code, _ := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`, 5); code != 200 {
		t.Error("accept after approval")
	}

	// keep and withdraw paths on another offer
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+j2+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(200)+`","termsVersion":"t"}`, 1)
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{j2}, &offer)
	job = j2
	pp(&contrA, slotJSON(102, 2), "tech-external-a", "van in repair", 2)
	ownerScan(t, `SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 ORDER BY sent_at DESC LIMIT 1`, []any{offer}, &partner)
	if code, _ := write(s, &contrB, "jobs.withdrawPartnerSlot", `{"jobId":"`+j2+`","proposalId":"`+partner+`"}`, 3); code != 404 {
		t.Error("other contractor withdraws")
	}
	if code, _ := write(s, &contrA, "jobs.withdrawPartnerSlot", `{"jobId":"`+j2+`","proposalId":"`+partner+`"}`, 3); code != 200 {
		t.Fatal("withdraw partner proposal")
	}
	if code, _ := write(s, &contrA, "jobs.withdrawPartnerSlot", `{"jobId":"`+j2+`","proposalId":"`+partner+`"}`, 4); code != 409 {
		t.Error("withdraw twice")
	}
	pp(&contrA, slotJSON(103, 2), "tech-external-a", "another reason", 4)
	ownerScan(t, `SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status = 'pending'`, []any{offer}, &partner)
	if code, _ := write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+j2+`","proposalId":"`+partner+`","decision":"keep"}`, 5); code != 200 {
		t.Fatal("keep")
	}

	// periodic visit reschedule
	plan, pj := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,8,$4)`, plan, seed.ID("tenant-a"), unit, clock)
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','requested','periodic_plan',tstzrange($6,$7),$7)`, pj, seed.ID("tenant-a"), unit, seed.ID("org-customer-a"), plan,
		clock.Add(150*time.Hour), clock.Add(152*time.Hour))
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+pj+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(150)+`","endAt":"`+ts(152)+`"}`, 1); code != 200 {
		t.Fatalf("assign plan job: %d %v", code, m)
	}
	three := `[` + slotJSON(170, 2) + `,` + slotJSON(194, 2) + `,` + slotJSON(218, 2) + `]`
	if code, _ := write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+pj+`","preferredSlots":[`+slotJSON(170, 2)+`]}`, 2); code != 422 {
		t.Error("reschedule with one slot")
	}
	if code, _ := write(s, &customerB, "jobs.requestReschedule", `{"jobId":"`+pj+`","preferredSlots":`+three+`}`, 2); code != 404 {
		t.Error("other customer reschedules")
	}
	if code, m := write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+pj+`","preferredSlots":`+three+`,"comment":"travelling"}`, 2); code != 200 ||
		data(m)["status"] != "requested" || data(m)["assignmentId"] != nil || data(m)["preferenceRound"].(float64) != 2 {
		t.Fatalf("reschedule: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+pj+`","preferredSlots":`+three+`}`, 3); code != 409 {
		t.Error("reschedule a requested job")
	}
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	if code, _ := write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+data(m)["id"].(string)+`","preferredSlots":`+three+`}`, 1); code != 409 {
		t.Error("client_request jobs are not rescheduled")
	}
	// within 48 hours of the visit
	pj2 := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin, requested_slot, scheduled_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','assigned','periodic_plan',tstzrange($6,$7),tstzrange($6,$7),$7)`, pj2, seed.ID("tenant-a"), unit, seed.ID("org-customer-a"), plan,
		clock.Add(30*time.Hour), clock.Add(32*time.Hour))
	if code, _ := write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+pj2+`","preferredSlots":`+three+`}`, 1); code != 409 {
		t.Error("reschedule within 48 hours")
	}
}

// TestRescheduleAfterContractorAssignment: HQ may propose another time for an assigned job only after the technician
// answered that they can't make it. When the customer accepts it, the contractor's accepted offer moves to the new
// visit, the assignment is revoked as rescheduled, and the job goes back to accepted, so the contractor assigns again.
func TestRescheduleAfterContractorAssignment(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, seed.ID("tech-external-a"))
	t.Cleanup(func() { // the seed assignment comes back unless a test assignment now holds its window
		owner(t, `UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND b.scheduled && a.scheduled)`, seed.ID("assignment-contractor-a"))
	})
	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, seed.ID("org-contractor-a"))
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	version := func(job string) int {
		_, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
		return ver(m)
	}
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(200)+`","termsVersion":"t"}`, version(job)); code != 200 {
		t.Fatalf("offer: %d %v", code, m)
	}
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`, version(job)); code != 200 {
		t.Fatalf("accept: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","startAt":"`+ts(49)+`","endAt":"`+ts(51)+`"}`, version(job)); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	contractorHold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","technicianMembershipId":null}`
	propose := func() (int, map[string]any) {
		return write(s, &hq, "jobs.proposeSlot", `{"jobId":"`+job+`","slot":`+slotJSON(97, 2)+`,"hold":`+contractorHold+`,"message":"Can we come then?","replyBy":"`+ts(48)+`"}`, version(job))
	}
	if code, m := propose(); code != 409 || m["messageKey"] != "error.invalidState" {
		t.Errorf("propose while the technician has not answered: %d %v", code, m)
	}
	if code, m := write(s, &techA, "jobs.acknowledgeAssignment", `{"jobId":"`+job+`","decision":"cant_make","reason":"another call"}`, version(job)); code != 200 || data(m)["acknowledgement"] != "cant_make" {
		t.Fatalf("can't make it: %d %v", code, m)
	}
	if code, m := propose(); code != 200 {
		t.Fatalf("propose after can't make: %d %v", code, m)
	}
	var proposal string
	ownerScan(t, `SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`, []any{job}, &proposal)
	code, m := write(s, &customerA, "jobs.respondProposal", `{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`, version(job))
	if code != 200 || data(m)["status"] != "accepted" || data(m)["assignmentId"] != nil || data(m)["scheduledSlot"] != nil {
		t.Fatalf("accept the new time: %d %v", code, m)
	}
	var status, reason string
	ownerScan(t, `SELECT status, reason FROM maintenance.assignments WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`, []any{job}, &status, &reason)
	if status != "revoked" || reason != "rescheduled" {
		t.Errorf("assignment after the new time: %s %s", status, reason)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["offer"].(map[string]any)["visitSlot"].(map[string]any)["startAt"] != ts(97) {
		t.Errorf("the accepted offer's visit: %v", data(m)["offer"])
	}
	write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","reason":"test done"}`, version(job))
}

// TestPartnerProposalRefusals covers the refusals around a contractor's time proposal: a time that breaks the slot
// rules, an offer that has expired, sending it to the customer while HQ's own proposal is pending, and a reply-by
// outside the allowed range.
func TestPartnerProposalRefusals(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND lower(scheduled) > $2`, seed.ID("tech-external-a"), clock.Add(2*time.Hour))
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	version := func(job string) int {
		_, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
		return ver(m)
	}
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(200)+`","termsVersion":"t"}`, version(job))
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	pp := func(slot string) (int, map[string]any) {
		return write(s, &contrA, "jobs.proposePartnerSlot", `{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slot+`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"van in repair"}`, version(job))
	}
	if code, m := pp(slotJSON(3, 1)); code != 422 || m["fieldErrors"].(map[string]any)["slot"] != "error.slotRules" {
		t.Errorf("a time today: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`, offer, clock.Add(-time.Minute))
	if code, m := pp(slotJSON(101, 2)); code != 409 || m["messageKey"] != "errors.offer_expired" {
		t.Errorf("an expired offer: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`, offer, clock.Add(12*time.Hour))
	if code, m := pp(slotJSON(101, 2)); code != 200 {
		t.Fatalf("partner proposal: %d %v", code, m)
	}
	var partner string
	ownerScan(t, `SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status = 'pending'`, []any{offer}, &partner)
	resolve := func(replyBy string) (int, map[string]any) {
		return write(s, &hq, "jobs.resolvePartnerSlot", `{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+replyBy+`"}`, version(job))
	}
	if code, m := resolve(ts(24 * 8)); code != 422 || m["fieldErrors"].(map[string]any)["replyBy"] != "error.range" {
		t.Errorf("reply by in 8 days: %d %v", code, m)
	}
	// HQ's own proposal is pending: the contractor's time waits
	hold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","technicianMembershipId":null}`
	if code, m := write(s, &hq, "jobs.proposeSlot", `{"jobId":"`+job+`","slot":`+slotJSON(121, 2)+`,"hold":`+hold+`,"message":"Or then?","replyBy":"`+ts(48)+`"}`, version(job)); code != 200 {
		t.Fatalf("HQ proposal: %d %v", code, m)
	}
	if code, m := resolve(ts(24)); code != 409 || m["messageKey"] != "errors.proposal_pending" {
		t.Errorf("send while HQ's proposal is pending: %d %v", code, m)
	}
	write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","reason":"test done"}`, version(job))
}

// TestRescheduleOfferedPeriodicVisit: a periodic visit that is offered but not yet assigned has no scheduled slot, so
// the 48-hour rule of jobs.requestReschedule reads the open offer's visit (IR113): within 48 hours it is too late,
// later the customer's three times go back to HQ.
func TestRescheduleOfferedPeriodicVisit(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-non-rto")
	plan, job := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,8,$4)`, plan, seed.ID("tenant-a"), unit, clock.Add(24*90*time.Hour))
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin, requested_slot, due_at, contractor_org_id)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','offered','periodic_plan',tstzrange($6,$7),$7,$8)`, job, seed.ID("tenant-a"), unit, seed.ID("org-customer-a"), plan,
		clock.Add(30*time.Hour), clock.Add(32*time.Hour), seed.ID("org-contractor-a"))
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until)
		VALUES ($1,$2,$3,'t',tstzrange($4,$5),$6,$7,$6,$8)`, seed.ID("tenant-a"), job, seed.ID("org-contractor-a"), clock.Add(30*time.Hour), clock.Add(32*time.Hour), clock, clock.Add(12*time.Hour), clock.Add(200*time.Hour))
	three := `[` + slotJSON(170, 2) + `,` + slotJSON(194, 2) + `,` + slotJSON(218, 2) + `]`
	reschedule := func() (int, map[string]any) {
		_, m := post(s, &customerA, "jobs.get", `{"jobId":"`+job+`"}`)
		return write(s, &customerA, "jobs.requestReschedule", `{"jobId":"`+job+`","preferredSlots":`+three+`}`, ver(m))
	}
	if code, m := reschedule(); code != 409 || m["messageKey"] != "errors.reschedule_too_late" {
		t.Errorf("an offered visit in 30 hours: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.offers SET visit_slot = tstzrange($2, $3) WHERE job_id = $1`, job, clock.Add(100*time.Hour), clock.Add(102*time.Hour))
	if code, m := reschedule(); code != 200 || data(m)["status"] != "requested" {
		t.Errorf("an offered visit in 100 hours: %d %v", code, m)
	}
}
