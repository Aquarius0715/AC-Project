package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// statusRecipients are the memberships that got a schedule_change notice of the job with this status word.
func statusRecipients(t *testing.T, job, status string) map[string]bool {
	t.Helper()
	drainEvents(t)

	out := map[string]bool{}

	for _, name := range []string{
		membershipHQOperator, membershipCustomerA, membershipContractorA, membershipContractorB,
		"tech-internal-a", membershipTechExternalA,
	} {
		var n int
		ownerScan(
			t,
			`SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2 AND
				template_key = 'schedule_change' AND params->>'status' = $3`,
			[]any{job, seed.ID(name), status},
			&n,
		)

		out[name] = n > 0
	}

	return out
}

// TestTimeProposalNotices (IR783, IR113 item 9): each step of the time-proposal flow tells the role that has to act,
// never the actor: a contractor's time change → HQ; HQ's proposal of it → the client; the client's answer → HQ; an
// unanswered proposal → HQ.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestTimeProposalNotices(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	check := func(step string, got map[string]bool, want, not []string) {
		t.Helper()

		for _, name := range want {
			if !got[name] {
				t.Errorf("%s: %s got no notice (%v)", step, name, got)
			}
		}

		for _, name := range not {
			if got[name] {
				t.Errorf("%s: %s got a notice (%v)", step, name, got)
			}
		}
	}

	// the client declines
	opened := openProposals(t, srv)
	check(
		"contractor's time change",
		statusRecipients(t, opened.job, "time_change_proposed"),
		[]string{membershipHQOperator},
		[]string{membershipContractorA, membershipCustomerA},
	)
	check(
		"HQ sends it to the client",
		statusRecipients(t, opened.job, "time_proposed"),
		[]string{membershipCustomerA},
		[]string{membershipHQOperator, membershipContractorA},
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+opened.job+`","proposalId":"`+opened.proposal+`","decision":"decline","declineReason":"too_late"}`,
		4,
	); code != 200 {
		t.Fatalf("decline: %d %v", code, res)
	}

	check(
		"client declines",
		statusRecipients(t, opened.job, "declined"),
		[]string{membershipHQOperator},
		[]string{membershipCustomerA},
	)

	// the client accepts
	opened = openProposals(t, srv)
	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+opened.job+`","proposalId":"`+opened.proposal+`","decision":"accept"}`,
		4,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	check(
		"client accepts",
		statusRecipients(t, opened.job, "accepted"),
		[]string{membershipHQOperator},
		[]string{membershipCustomerA},
	)

	// the accepted time hands the job to another contractor, which is told of its offer
	opened = openProposals(t, srv)
	jobVersion := func() int {
		t.Helper()

		_, res := post(srv, hq(), "jobs.get", `{"jobId":"`+opened.job+`"}`)

		return int(asNum(t, data(res)["version"]))
	}
	hold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-b").String() + `"}`

	if code, res := write(
		srv,
		hq(),
		"jobs.withdrawProposal",
		`{"jobId":"`+opened.job+`","proposalId":"`+opened.proposal+`"}`,
		jobVersion(),
	); code != 200 {
		t.Fatalf("withdraw the first proposal: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.proposeSlot",
		`{"jobId":"`+opened.job+`","slot":`+slotJSON(98, 2)+`,"hold":`+hold+
			`,"message":"Another partner can come then","replyBy":"`+
			clock().Add(48*time.Hour).Format(time.RFC3339)+`"}`,
		jobVersion(),
	); code != 200 {
		t.Fatalf("propose contractor-b: %d %v", code, res)
	}

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{opened.job},
		&proposal,
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+opened.job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		jobVersion(),
	); code != 200 {
		t.Fatalf("accept for contractor-b: %d %v", code, res)
	}

	check(
		"acceptance offers the job to contractor-b",
		statusRecipients(t, opened.job, "accepted"),
		[]string{membershipHQOperator, membershipContractorB},
		[]string{membershipCustomerA},
	)

	// nobody answers: the scheduler tells HQ
	opened = openProposals(t, srv)

	_, err := schedTick(ctx, srv, clock().Add(11*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	check(
		"proposal expires",
		statusRecipients(t, opened.job, "expired"),
		[]string{membershipHQOperator},
		[]string{membershipCustomerA},
	)
}

// TestTimeChangeKeptNotice: HQ keeps the agreed time against a contractor's time change, and the contractor is told.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestTimeChangeKeptNotice(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID(membershipTechExternalA),
		clock().Add(2*time.Hour),
	)

	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := asStr(t, data(resp)["id"])

	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+
			`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer, partner string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.proposePartnerSlot",
		`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slotJSON(146, 2)+
			`,"technicianMembershipId":"`+seed.ID(membershipTechExternalA).String()+`","reason":"team busy"}`,
		2,
	); code != 200 {
		t.Fatalf("partner proposal: %d %v", code, res)
	}

	ownerScan(
		t,
		`SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status = 'pending'`,
		[]any{offer},
		&partner,
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"keep"}`,
		3,
	); code != 200 {
		t.Fatalf("keep: %d %v", code, res)
	}

	got := statusRecipients(t, job, "time_change_kept")
	if !got[membershipContractorA] || got[membershipHQOperator] {
		t.Errorf("HQ keeps the agreed time: %v", got)
	}
}

// TestAcknowledgementNotices (IR113 items 8 and 9): the technician's answer goes to the coordinator, HQ for an internal
// job, and a Yes also to the client.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAcknowledgementNotices(t *testing.T) {
	srv := server(t)

	job := assignedNow(t, srv)
	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"cant_make","reason":"sick"}`,
		2,
	); code != 200 {
		t.Fatalf("cant_make: %d %v", code, res)
	}

	got := statusRecipients(t, job, "cant_make")
	if !got[membershipHQOperator] || got[membershipCustomerA] || got["tech-internal-a"] {
		t.Errorf("cant_make: %v", got)
	}

	job = assignedNow(t, srv)
	if code, res := write(
		srv,
		techInt(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	got = statusRecipients(t, job, "accepted")
	if !got[membershipHQOperator] || !got[membershipCustomerA] || got["tech-internal-a"] {
		t.Errorf("accepted: %v", got)
	}
}
