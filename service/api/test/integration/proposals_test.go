package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	refusedHoldInvalid  = "hold=" + apperr.KeyInvalid
	refusedReplyByRange = "replyBy=" + apperr.KeyRange
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSlotProposals(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active'
			AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")},
		clock().Add(2*time.Hour),
	)
	owner(
		t,
		`DELETE FROM maintenance.contractors WHERE organization_id = $1`,
		seed.ID("org-contractor-a"),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	internalHold := `{"kind":"internal","membershipId":"` + seed.ID("tech-internal-a").
		String() +
		`"}`
	contractorHold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").
		String() +
		`","technicianMembershipId":null}`
	propose := func(job, slot, hold, replyBy string, version int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"jobs.proposeSlot",
			`{"jobId":"`+job+`","slot":`+slot+`,"hold":`+hold+`,"message":"Can we come then?","replyBy":"`+replyBy+`"}`,
			version,
		)
	}
	proposalOf := func(job string) string {
		var proposalID string
		ownerScan(
			t,
			`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 ORDER BY (status = 'pending') DESC, created_at
				DESC LIMIT 1`,
			[]any{job},
			&proposalID,
		)

		return proposalID
	}

	_, resp := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(resp)["id"])
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"bad hold":                    refusedHoldInvalid,
		"contractor with a member":    refusedHoldInvalid,
		"internal with a contractor":  refusedHoldInvalid,
		"internal without technician": refusedHoldInvalid,
		"preferred slot":              "slot=errors.slot_is_preferred",
		"reply after 7 days":          refusedReplyByRange,
		"reply in the past":           refusedReplyByRange,
		"today":                       "slot=error.slotRules",
	}

	for name, testCase := range map[string]struct {
		slot, hold, reply string
		code              int
	}{
		"preferred slot":     {slotJSON(25, 2), internalHold, stamp(48), 422},
		"today":              {slotJSON(3, 1), internalHold, stamp(48), 422},
		"reply after 7 days": {slotJSON(97, 2), internalHold, stamp(24 * 8), 422},
		"reply in the past":  {slotJSON(97, 2), internalHold, stamp(-1), 422},
		"external as internal": {slotJSON(97, 2), `{"kind":"internal",` +
			`"membershipId":"` + seed.ID("tech-external-a").String() + `"}`, stamp(48), 403},
		"bad hold": {slotJSON(97, 2), `{"kind":"robot"}`, stamp(48), 422},
		"unknown contractor": {slotJSON(97, 2), `{"kind":"contractor",` +
			`"contractorOrgId":"` + uuid.NewString() + `","technicianMembershipId":null}`, stamp(48), 404},
		// a hold names exactly the side it holds (IR128 item 1)
		"internal without technician": {slotJSON(97, 2), `{"kind":"internal"}`, stamp(48), 422},
		"internal with a contractor": {slotJSON(97, 2), `{"kind":"internal",` +
			`"membershipId":"` + seed.ID("tech-internal-a").String() + `","contractorOrgId":"` +
			seed.ID("org-contractor-a").String() + `"}`, stamp(48), 422},
		"contractor with a member": {slotJSON(97, 2), `{"kind":"contractor",` +
			`"contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","membershipId":"` +
			seed.ID("tech-internal-a").String() + `"}`, stamp(48), 422},
		"another company's technician": {slotJSON(97, 2), `{"kind":"contractor",` +
			`"contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","technicianMembershipId":"` +
			seed.ID("tech-external-b").String() + `"}`, stamp(48), 403},
	} {
		code, m := propose(job, testCase.slot, testCase.hold, testCase.reply, 1)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != testCase.code {
			t.Errorf("propose %s: %d want %d", name, code, testCase.code)
		}
	}
	// a contractor whose offers are suspended cannot be held
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation, suspended_reason)
		VALUES ($1, $2, 'Demo Contractor A', 'suspended', 'SSM-TEST', 'ops@contractor-a.example', tstzrange($3, $4), 'test')`,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
		clock().Add(24*365*time.Hour),
	)

	if code, res := propose(
		job,
		slotJSON(97, 2),
		contractorHold,
		stamp(48),
		1,
	); code != 409 ||
		res["messageKey"] != "errors.contractor_suspended" {
		t.Errorf("suspended contractor: %d %v", code, res)
	}

	owner(
		t,
		`DELETE FROM maintenance.contractors WHERE organization_id = $1`,
		seed.ID("org-contractor-a"),
	)
	// a contractor hold may name the contractor's own technician, who must be qualified for the slot; withdrawn again
	{
		_, m := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
		other := asStr(t, data(m)["id"])

		named := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").
			String() +
			`","technicianMembershipId":"` + seed.ID("tech-external-a").
			String() +
			`"}`
		if code, res := propose(
			other,
			slotJSON(97, 2),
			named,
			stamp(48),
			1,
		); code != 200 ||
			asObj(t, data(res)["slotProposal"])["status"] != statusPending {
			t.Fatalf("contractor hold with its technician: %d %v", code, res)
		}

		if code, _ := write(
			srv,
			hq(),
			"jobs.withdrawProposal",
			`{"jobId":"`+other+`","proposalId":"`+proposalOf(other)+`"}`,
			2,
		); code != 200 {
			t.Error("withdraw the named hold")
		}
	}

	if code, res := propose(
		job,
		slotJSON(97, 2),
		internalHold,
		stamp(48),
		1,
	); code != 200 ||
		asObj(t, data(res)["slotProposal"])["status"] != statusPending {
		t.Fatalf("propose: %d %v", code, res)
	}

	if _, m := post(
		srv,
		customerA(),
		"jobs.list",
		`{"filters":{"unitId":"`+unit+`","proposalPending":true},"limit":100}`,
	); byJob(t, m, job)["displayStatus"] != "time_proposed" {
		t.Fatal("time_proposed display status")
	}

	if code, _ := propose(job, slotJSON(121, 2), internalHold, stamp(48), 2); code != 409 {
		t.Error("second pending proposal")
	}

	proposal1 := proposalOf(job)

	respond := func(a *actor, j, p, body string, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"jobs.respondProposal",
			`{"jobId":"`+j+`","proposalId":"`+p+`",`+body+`}`,
			version,
		)
	}
	if code, _ := respond(customerB(), job, proposal1, `"decision":"accept"`, 2); code != 404 {
		t.Error("other customer responds")
	}

	if code, res := respond(
		customerA(),
		job,
		proposal1,
		`"decision":"decline"`,
		2,
	); code != 422 ||
		fields(res) != "declineReason=error.invalid" {
		t.Errorf("decline without reason: %d %s", code, fields(res))
	}

	if code, res := respond(
		customerA(),
		job,
		proposal1,
		`"decision":"decline","declineReason":"not_home","preferredSlots":[`+slotJSON(30, 1)+`]`,
		2,
	); code != 422 ||
		fields(res) != "preferredSlots=error.count" {
		t.Errorf("decline with one slot: %d %s", code, fields(res))
	}

	if code, res := respond(
		customerA(),
		job,
		proposal1,
		`"decision":"accept","declineReason":"other"`,
		2,
	); code != 422 ||
		fields(res) != "decision=error.fieldsNotAllowed" {
		t.Errorf("accept with decline fields: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.withdrawProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal1+`"}`,
		2,
	); code != 200 ||
		asObj(t, data(res)["slotProposal"])["status"] != statusWithdrawn {
		t.Fatalf("withdraw: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.withdrawProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal1+`"}`,
		3,
	); code != 409 {
		t.Error("withdraw twice")
	}

	propose(job, slotJSON(97, 2), internalHold, stamp(48), 3)

	proposal2 := proposalOf(job)
	if code, res := respond(
		customerA(),
		job,
		proposal2,
		`"decision":"accept"`,
		4,
	); code != 200 ||
		data(res)["status"] != statusAssigned ||
		asObj(t, data(res)["scheduledSlot"])["startAt"] != stamp(97) {
		t.Fatalf("accept internal hold: %d %v", code, res)
	}

	if code, _ := respond(customerA(), job, proposal2, `"decision":"accept"`, 6); code != 409 {
		t.Error("answer twice")
	}

	// contractor hold without an Offer: accepting creates the Offer at the slot
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job2 := asStr(t, data(resp)["id"])
	propose(job2, slotJSON(98, 2), contractorHold, stamp(48), 1)

	p3 := proposalOf(job2)
	if code, res := respond(
		customerA(),
		job2,
		p3,
		`"decision":"accept"`,
		2,
	); code != 200 ||
		data(res)["status"] != statusOffered {
		t.Fatalf("accept contractor hold: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job2+`"}`,
	); firstCandidate(t, data(res)["offer"]) != stamp(
		98,
	) ||
		asObj(t, data(res)["offer"])["offerExpiresAt"] != stamp(24) {
		t.Fatalf("offer at the proposal slot, open for 24 hours: %v", data(res)["offer"])
	}
	// a slot that starts sooner than that closes the offer at its start
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	soon := asStr(t, data(resp)["id"])
	propose(soon, slotJSON(98, 2), contractorHold, stamp(48), 1)
	owner(
		t,
		`UPDATE maintenance.slot_proposals SET slot = tstzrange($2, $3) WHERE job_id = $1 AND status = 'pending'`,
		soon,
		clock().Add(10*time.Hour),
		clock().Add(12*time.Hour),
	)

	if code, res := respond(
		customerA(),
		soon,
		proposalOf(soon),
		`"decision":"accept"`,
		2,
	); code != 200 ||
		data(res)["status"] != statusOffered {
		t.Fatalf("accept a slot in 10 hours: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+soon+`"}`,
	); asObj(t, data(res)["offer"])["offerExpiresAt"] != stamp(
		10,
	) {
		t.Errorf("offer closes at the slot start: %v", data(res)["offer"])
	}

	// decline with three new preferred times
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job3 := asStr(t, data(resp)["id"])
	propose(job3, slotJSON(99, 2), internalHold, stamp(48), 1)
	newSlots := `[` + slotJSON(120, 2) + `,` + slotJSON(144, 2) + `,` + slotJSON(168, 2) + `]`
	// the new times follow the request's slot rules (IR113): a time today is refused
	today := `[` + slotJSON(1, 2) + `,` + slotJSON(144, 2) + `,` + slotJSON(168, 2) + `]`
	if code, res := respond(
		customerA(),
		job3,
		proposalOf(job3),
		`"decision":"decline","declineReason":"too_late","preferredSlots":`+today,
		2,
	); code != 422 ||
		fields(res) != "preferredSlots=error.slotRules" {
		t.Errorf("decline with a time today: %d %s", code, fields(res))
	}

	if code, res := respond(
		customerA(),
		job3,
		proposalOf(job3),
		`"decision":"decline","declineReason":"too_late","comment":" evenings only ","preferredSlots":`+newSlots,
		2,
	); code != 200 ||
		asNum(t, data(res)["preferenceRound"]) != 2 ||
		data(res)["status"] != statusRequested {
		t.Fatalf("decline with slots: %d %v", code, res)
	}

	// expiry by the worker
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job4 := asStr(t, data(resp)["id"])
	propose(job4, slotJSON(100, 2), internalHold, stamp(2), 1)

	_, err := schedTick(context.Background(), srv, clock().Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if _, m := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job4+`"}`,
	); asObj(t, data(m)["slotProposal"])["status"] != statusExpired {
		t.Fatal("expired proposal")
	}

	if code, _ := respond(
		customerA(),
		job4,
		proposalOf(job4),
		`"decision":"accept"`,
		2,
	); code != 409 {
		t.Error("accept an expired proposal")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerProposalsAndReschedule(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active'
			AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")},
		clock().Add(2*time.Hour),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(resp)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		1,
	)

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	proposePartner := func(a *actor, slot, tech, reason string, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"jobs.proposePartnerSlot",
			`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slot+`,"technicianMembershipId":"`+seed.ID(tech).
				String()+
				`","reason":"`+reason+`"}`,
			version,
		)
	}
	if code, _ := proposePartner(
		contrA(),
		slotJSON(101, 2),
		"tech-internal-a",
		"busy",
		2,
	); code != 403 {
		t.Error("other organization's technician")
	}

	if code, res := proposePartner(
		contrA(),
		slotJSON(101, 2),
		"tech-external-a",
		" ",
		2,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("blank reason: %d %s", code, fields(res))
	}

	if code, _ := proposePartner(
		contrB(),
		slotJSON(101, 2),
		"tech-external-b",
		"busy",
		2,
	); code != 404 {
		t.Error("other contractor")
	}

	if code, res := proposePartner(
		contrA(),
		slotJSON(101, 2),
		"tech-external-a",
		"team busy that day",
		2,
	); code != 200 ||
		data(res)["partnerSlotProposal"] == nil {
		t.Fatalf("partner proposal: %d %v", code, res)
	}

	if code, _ := proposePartner(
		contrA(),
		slotJSON(125, 2),
		"tech-external-a",
		"again",
		3,
	); code != 409 {
		t.Error("second partner proposal")
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		3,
	); code != 409 ||
		res["messageKey"] != "errors.partner_proposal_pending" {
		t.Errorf("accept while partner proposal pending: %d %v", code, res)
	}

	var partner string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 ORDER BY sent_at DESC LIMIT 1`,
		[]any{offer},
		&partner,
	)
	// the contractor's offer projection shows the pending proposal, when HQ offered and the access period (IR226)
	if code, res := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 ||
		data(res)["projection"] != projectionOffer {
		t.Fatalf("offer projection: %d %v", code, res)
	} else if offerView := data(
		res,
	); offerView["partnerSlotProposal"] == nil || asObj(t, offerView["partnerSlotProposal"])["id"] != partner ||
		asObj(t, offerView["partnerSlotProposal"])["status"] != statusPending ||
		offerView["offeredAt"] == nil || offerView["accessValidFrom"] != stamp(0) ||
		offerView["accessValidUntil"] != stamp(200) {
		t.Fatalf("offer projection fields: %v", offerView)
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client"}`,
		3,
	); code != 422 ||
		fields(res) != "replyBy=error.required" {
		t.Errorf("send without replyBy: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"keep","replyBy":"`+stamp(5)+`"}`,
		3,
	); code != 422 ||
		fields(res) != "replyBy=error.notAllowed" {
		t.Errorf("keep with replyBy: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+stamp(
			24,
		)+`"}`,
		3,
	); code != 200 ||
		asObj(t, data(res)["slotProposal"])["source"] != roleContractor {
		t.Fatalf("send to client: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"keep"}`,
		4,
	); code != 409 {
		t.Error("resolve twice")
	}

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		4,
	); code != 200 ||
		data(res)["status"] != statusOffered {
		t.Fatalf("client accepts partner time: %d %v", code, res)
	}

	if _, m := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); firstCandidate(t, data(m)["offer"]) != stamp(
		101,
	) {
		t.Fatal("offer visit slot moved")
	}

	if code, _ := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		5,
	); code != 200 {
		t.Error("accept after approval")
	}

	// keep and withdraw paths on another offer
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job2 := asStr(t, data(resp)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job2+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		1,
	)
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job2},
		&offer,
	)
	job = job2

	proposePartner(contrA(), slotJSON(102, 2), "tech-external-a", "van in repair", 2)
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 ORDER BY sent_at DESC LIMIT 1`,
		[]any{offer},
		&partner,
	)

	if code, _ := write(
		srv,
		contrB(),
		"jobs.withdrawPartnerSlot",
		`{"jobId":"`+job2+`","proposalId":"`+partner+`"}`,
		3,
	); code != 404 {
		t.Error("other contractor withdraws")
	}

	if code, _ := write(
		srv,
		contrA(),
		"jobs.withdrawPartnerSlot",
		`{"jobId":"`+job2+`","proposalId":"`+partner+`"}`,
		3,
	); code != 200 {
		t.Fatal("withdraw partner proposal")
	}

	if code, _ := write(
		srv,
		contrA(),
		"jobs.withdrawPartnerSlot",
		`{"jobId":"`+job2+`","proposalId":"`+partner+`"}`,
		4,
	); code != 409 {
		t.Error("withdraw twice")
	}

	proposePartner(contrA(), slotJSON(103, 2), "tech-external-a", "another reason", 4)
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status = 'pending'`,
		[]any{offer},
		&partner,
	)

	if code, _ := write(
		srv,
		hq(),
		"jobs.resolvePartnerSlot",
		`{"jobId":"`+job2+`","proposalId":"`+partner+`","decision":"keep"}`,
		5,
	); code != 200 {
		t.Fatal("keep")
	}

	// periodic visit reschedule
	plan, planJob := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,
			8,$4)`,
		plan,
		seed.ID("tenant-a"),
		unit,
		clock(),
	)
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin,
			requested_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','requested','periodic_plan',tstzrange($6,$7),$7)`,
		planJob,
		seed.ID("tenant-a"),
		unit,
		seed.ID("org-customer-a"),
		plan,
		clock().Add(150*time.Hour),
		clock().Add(152*time.Hour),
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+planJob+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			150,
		)+`","endAt":"`+stamp(
			152,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("assign plan job: %d %v", code, res)
	}

	three := `[` + slotJSON(170, 2) + `,` + slotJSON(194, 2) + `,` + slotJSON(218, 2) + `]`
	if code, res := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+planJob+`","preferredSlots":[`+slotJSON(170, 2)+`]}`,
		2,
	); code != 422 ||
		fields(res) != "preferredSlots=error.count" {
		t.Errorf("reschedule with one slot: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		customerB(),
		"jobs.requestReschedule",
		`{"jobId":"`+planJob+`","preferredSlots":`+three+`}`,
		2,
	); code != 404 {
		t.Error("other customer reschedules")
	}

	if code, res := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+planJob+`","preferredSlots":[`+slotJSON(
			1,
			2,
		)+`,`+slotJSON(
			194,
			2,
		)+`,`+slotJSON(
			218,
			2,
		)+`]}`,
		2,
	); code != 422 ||
		fields(res) != "preferredSlots=error.slotRules" {
		t.Errorf("reschedule to a time today: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+planJob+`","preferredSlots":`+three+`,"comment":"travelling"}`,
		2,
	); code != 200 ||
		data(res)["status"] != statusRequested || data(res)["assignmentId"] != nil ||
		asNum(t, data(res)["preferenceRound"]) != 2 {
		t.Fatalf("reschedule: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+planJob+`","preferredSlots":`+three+`}`,
		3,
	); code != 409 {
		t.Error("reschedule a requested job")
	}

	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	if code, _ := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+asStr(t, data(resp)["id"])+`","preferredSlots":`+three+`}`,
		1,
	); code != 409 {
		t.Error("client_request jobs are not rescheduled")
	}
	// within 48 hours of the visit
	pj2 := uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin,
			requested_slot, scheduled_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','assigned','periodic_plan',tstzrange($6,$7),tstzrange($6,$7),$7)`,
		pj2,
		seed.ID("tenant-a"),
		unit,
		seed.ID("org-customer-a"),
		plan,
		clock().Add(30*time.Hour),
		clock().Add(32*time.Hour),
	)

	if code, _ := write(
		srv,
		customerA(),
		"jobs.requestReschedule",
		`{"jobId":"`+pj2+`","preferredSlots":`+three+`}`,
		1,
	); code != 409 {
		t.Error("reschedule within 48 hours")
	}
}

// TestRescheduleAfterContractorAssignment: HQ may propose another time for an assigned job only after the technician
// answered that they can't make it. When the customer accepts it, the contractor's accepted offer moves to the new
// visit, the assignment is revoked as rescheduled, and the job goes back to accepted, so the contractor assigns again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRescheduleAfterContractorAssignment(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		seed.ID("tech-external-a"),
	)
	t.Cleanup(
		func() { // the seed assignment comes back unless a test assignment now holds its window
			owner(
				t,
				`UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM
					maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND
			b.scheduled && a.scheduled)`,
				seed.ID("assignment-contractor-a"),
			)
		},
	)
	owner(
		t,
		`DELETE FROM maintenance.contractors WHERE organization_id = $1`,
		seed.ID("org-contractor-a"),
	)

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	version := func(job string) int {
		_, m := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		return ver(t, m)
	}
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
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		version(job),
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		version(job),
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").
			String()+
			`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		version(job),
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	contractorHold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").
		String() +
		`","technicianMembershipId":null}`

	propose := func() (int, map[string]any) {
		return write(
			srv,
			hq(),
			"jobs.proposeSlot",
			`{"jobId":"`+job+`","slot":`+slotJSON(
				97,
				2,
			)+`,"hold":`+contractorHold+`,"message":"Can we come then?","replyBy":"`+stamp(
				48,
			)+`"}`,
			version(job),
		)
	}
	if code, m := propose(); code != 409 || m["messageKey"] != apperr.KeyInvalidState {
		t.Errorf("propose while the technician has not answered: %d %v", code, m)
	}

	if code, res := write(
		srv,
		techA(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"cant_make","reason":"another call"}`,
		version(job),
	); code != 200 ||
		data(res)["acknowledgement"] != "cant_make" {
		t.Fatalf("can't make it: %d %v", code, res)
	}

	if code, m := propose(); code != 200 {
		t.Fatalf("propose after can't make: %d %v", code, m)
	}

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)

	code, resp := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		version(job),
	)
	if code != 200 || data(resp)["status"] != statusAccepted || data(resp)["assignmentId"] != nil ||
		data(resp)["scheduledSlot"] != nil {
		t.Fatalf("accept the new time: %d %v", code, resp)
	}

	var status, reason string
	ownerScan(
		t,
		`SELECT status, reason FROM maintenance.assignments WHERE job_id = $1 ORDER BY created_at DESC LIMIT 1`,
		[]any{job},
		&status,
		&reason,
	)

	if status != "revoked" || reason != "rescheduled" {
		t.Errorf("assignment after the new time: %s %s", status, reason)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); firstCandidate(t, data(res)["offer"]) != stamp(
		97,
	) {
		t.Errorf("the accepted offer's visit: %v", data(res)["offer"])
	}

	write(srv, hq(), "jobs.cancel", `{"jobId":"`+job+`","reason":"test done"}`, version(job))
}

// TestPartnerProposalRefusals covers the refusals around a contractor's time proposal: a time that breaks the slot
// rules, an offer that has expired, sending it to the customer while HQ's own proposal is pending, and a reply-by
// outside the allowed range.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerProposalRefusals(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-external-a"),
		clock().Add(2*time.Hour),
	)

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	version := func(job string) int {
		_, m := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		return ver(t, m)
	}
	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := asStr(t, data(resp)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		version(job),
	)

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	proposePartner := func(slot string) (int, map[string]any) {
		return write(
			srv,
			contrA(),
			"jobs.proposePartnerSlot",
			`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slot+`,"technicianMembershipId":"`+seed.ID("tech-external-a").
				String()+
				`","reason":"van in repair"}`,
			version(job),
		)
	}
	if code, m := proposePartner(
		slotJSON(3, 1),
	); code != 422 ||
		asObj(t, m["fieldErrors"])["slot"] != apperr.KeySlotRules {
		t.Errorf("a time today: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`,
		offer,
		clock().Add(-time.Minute),
	)

	if code, m := proposePartner(
		slotJSON(101, 2),
	); code != 409 ||
		m["messageKey"] != "errors.offer_expired" {
		t.Errorf("an expired offer: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`,
		offer,
		clock().Add(12*time.Hour),
	)

	if code, m := proposePartner(slotJSON(101, 2)); code != 200 {
		t.Fatalf("partner proposal: %d %v", code, m)
	}

	var partner string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status = 'pending'`,
		[]any{offer},
		&partner,
	)

	resolve := func(replyBy string) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"jobs.resolvePartnerSlot",
			`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+replyBy+`"}`,
			version(job),
		)
	}
	if code, m := resolve(
		stamp(24 * 8),
	); code != 422 ||
		asObj(t, m["fieldErrors"])["replyBy"] != apperr.KeyRange {
		t.Errorf("reply by in 8 days: %d %v", code, m)
	}
	// HQ's own proposal is pending: the contractor's time waits
	hold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").
		String() +
		`","technicianMembershipId":null}`
	if code, res := write(
		srv,
		hq(),
		"jobs.proposeSlot",
		`{"jobId":"`+job+`","slot":`+slotJSON(
			121,
			2,
		)+`,"hold":`+hold+`,"message":"Or then?","replyBy":"`+stamp(
			48,
		)+`"}`,
		version(job),
	); code != 200 {
		t.Fatalf("HQ proposal: %d %v", code, res)
	}

	if code, m := resolve(stamp(24)); code != 409 || m["messageKey"] != "errors.proposal_pending" {
		t.Errorf("send while HQ's proposal is pending: %d %v", code, m)
	}

	write(srv, hq(), "jobs.cancel", `{"jobId":"`+job+`","reason":"test done"}`, version(job))
}

// TestRescheduleOfferedPeriodicVisit: a periodic visit that is offered but not yet assigned has no scheduled slot, so
// the 48-hour rule of jobs.requestReschedule reads the open offer's visit (IR113): within 48 hours it is too late,
// later the customer's three times go back to HQ.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRescheduleOfferedPeriodicVisit(t *testing.T) {
	srv := server(t)
	unit := seed.ID("unit-non-rto")
	plan, job := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,
			8,$4)`,
		plan,
		seed.ID("tenant-a"),
		unit,
		clock().Add(24*90*time.Hour),
	)
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin,
			requested_slot, due_at, contractor_org_id)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','offered','periodic_plan',tstzrange($6,$7),$7,$8)`,
		job,
		seed.ID("tenant-a"),
		unit,
		seed.ID("org-customer-a"),
		plan,
		clock().Add(30*time.Hour),
		clock().Add(32*time.Hour),
		seed.ID("org-contractor-a"),
	)
	owner(
		t,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, candidate_slots, offered_at,
			offer_expires_at, access_valid_from, access_valid_until)
		VALUES ($1,$2,$3,'t',`+oneSlotSQL(
			"$4",
			"$5",
		)+`,$6,$7,$6,$8)`,
		seed.ID("tenant-a"),
		job,
		seed.ID("org-contractor-a"),
		clock().Add(30*time.Hour),
		clock().Add(32*time.Hour),
		clock(),
		clock().Add(12*time.Hour),
		clock().Add(200*time.Hour),
	)

	three := `[` + slotJSON(170, 2) + `,` + slotJSON(194, 2) + `,` + slotJSON(218, 2) + `]`

	reschedule := func() (int, map[string]any) {
		_, res := post(srv, customerA(), "jobs.get", `{"jobId":"`+job+`"}`)

		return write(
			srv,
			customerA(),
			"jobs.requestReschedule",
			`{"jobId":"`+job+`","preferredSlots":`+three+`}`,
			ver(t, res),
		)
	}
	if code, m := reschedule(); code != 409 || m["messageKey"] != "errors.reschedule_too_late" {
		t.Errorf("an offered visit in 30 hours: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE maintenance.offers SET candidate_slots = `+oneSlotSQL(
			"$2",
			"$3",
		)+` WHERE job_id = $1`,
		job,
		clock().Add(100*time.Hour),
		clock().Add(102*time.Hour),
	)

	if code, m := reschedule(); code != 200 || data(m)["status"] != statusRequested {
		t.Errorf("an offered visit in 100 hours: %d %v", code, m)
	}
}

// TestPartnerProposalEndsWithClientProposal (IR443): when the client's proposal made from a contractor's time change
// ends without an answer, the time change ends too — kept when HQ withdraws the proposal, declined when the client lets
// it expire — so the contractor can accept the offer or propose again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerProposalEndsWithClientProposal(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-external-a"),
		clock().Add(2*time.Hour),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	sent := func(slotHour int) (string, string, string) {
		t.Helper()

		var job, offer, partner string

		_, m := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)

		job = asStr(t, data(m)["id"])
		if code, res := write(
			srv,
			hq(),
			"jobs.offer",
			`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
				String()+
				`","offerExpiresAt":"`+stamp(
				12,
			)+`","accessValidFrom":"`+stamp(
				0,
			)+`","accessValidUntil":"`+stamp(
				200,
			)+`","termsVersion":"t"}`,
			1,
		); code != 200 {
			t.Fatalf("offer: %d %v", code, res)
		}

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
			`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slotJSON(slotHour, 2)+
				`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"team busy"}`,
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
			`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+stamp(
				6,
			)+`"}`,
			3,
		); code != 200 {
			t.Fatalf("send to client: %d %v", code, res)
		}

		return job, offer, partner
	}
	status := func(partner string) string {
		var status string

		ownerScan(
			t,
			`SELECT status FROM maintenance.partner_slot_proposals WHERE id = $1`,
			[]any{partner},
			&status,
		)

		return status
	}

	// HQ withdraws the proposal: the agreed time stays, and the contractor accepts the offer
	job, offer, partner := sent(105)

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.withdrawProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`"}`,
		4,
	); code != 200 {
		t.Fatalf("withdraw: %d %v", code, res)
	}

	if st := status(partner); st != "kept" {
		t.Errorf("after HQ withdrew the client's proposal the time change is %s", st)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		5,
	); code != 200 {
		t.Errorf("accept after the withdrawal: %d %v", code, res)
	}

	// the client lets it expire: the contractor may propose again
	job, offer, partner = sent(129)

	_, err := schedTick(ctx, srv, clock().Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if st := status(partner); st != "declined" {
		t.Errorf("after the client's proposal expired the time change is %s", st)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.proposePartnerSlot",
		`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slotJSON(153, 2)+
			`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"another time"}`,
		4,
	); code != 200 {
		t.Errorf("propose again after the expiry: %d %v", code, res)
	}
}

// TestPartnerProposalEndsWithOffer (IR444): a contractor's time change still waiting for HQ ends (withdrawn) when its
// offer is declined or expires, so HQ's job no longer shows it once the job is offered again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPartnerProposalEndsWithOffer(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-external-a"),
		clock().Add(2*time.Hour),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	proposed := func(slotHour int) (string, string, string) {
		t.Helper()

		var job, offer, partner string

		_, m := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)

		job = asStr(t, data(m)["id"])
		if code, res := write(
			srv,
			hq(),
			"jobs.offer",
			`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
				String()+
				`","offerExpiresAt":"`+stamp(
				12,
			)+`","accessValidFrom":"`+stamp(
				0,
			)+`","accessValidUntil":"`+stamp(
				200,
			)+`","termsVersion":"t"}`,
			1,
		); code != 200 {
			t.Fatalf("offer: %d %v", code, res)
		}

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
			`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slotJSON(slotHour, 2)+
				`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"team busy"}`,
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

		return job, offer, partner
	}
	ended := func(job, partner string) {
		t.Helper()

		var state string
		ownerScan(
			t,
			`SELECT status FROM maintenance.partner_slot_proposals WHERE id = $1`,
			[]any{partner},
			&state,
		)

		if state != statusWithdrawn {
			t.Errorf("the time change is %s after its offer ended", state)
		}

		if _, res := post(
			srv,
			hq(),
			"jobs.get",
			`{"jobId":"`+job+`"}`,
		); data(res)["partnerSlotProposal"] != nil ||
			data(res)["status"] != statusRequested {
			t.Errorf(
				"HQ's job after the offer ended: status %v, partnerSlotProposal %v",
				data(res)["status"],
				data(res)["partnerSlotProposal"],
			)
		}
	}

	job, offer, partner := proposed(110)
	if code, res := write(
		srv,
		contrA(),
		"jobs.decline",
		`{"jobId":"`+job+`","offerId":"`+offer+`","reason":"no capacity"}`,
		3,
	); code != 200 {
		t.Fatalf("decline: %d %v", code, res)
	}

	ended(job, partner)

	// HQ cancels the job: the time change ends with it
	job, _, partner = proposed(122)
	if code, res := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"client moved out"}`,
		3,
	); code != 200 {
		t.Fatalf("cancel: %d %v", code, res)
	}

	var status string
	ownerScan(
		t,
		`SELECT status FROM maintenance.partner_slot_proposals WHERE id = $1`,
		[]any{partner},
		&status,
	)

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); status != statusWithdrawn ||
		data(res)["partnerSlotProposal"] != nil {
		t.Errorf(
			"after the cancellation the time change is %s, HQ's job shows %v",
			status,
			data(res)["partnerSlotProposal"],
		)
	}

	job, _, partner = proposed(134)

	_, err := schedTick(ctx, srv, clock().Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	ended(job, partner)
}

// TestDeclineWithdrawsClientProposal (IR445): a contractor that declines its offer while HQ has sent its time change to
// the client ends that client proposal too, so the client's acceptance cannot offer the job again to that company.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDeclineWithdrawsClientProposal(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-external-a"),
		clock().Add(2*time.Hour),
	)

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
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
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			200,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer, partner, proposal string
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
			`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"team busy"}`,
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
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+stamp(
			10,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("send to client: %d %v", code, res)
	}

	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.decline",
		`{"jobId":"`+job+`","offerId":"`+offer+`","reason":"no capacity"}`,
		4,
	); code != 200 {
		t.Fatalf("decline: %d %v", code, res)
	}

	var clientStatus, partnerStatus string
	ownerScan(
		t,
		`SELECT (SELECT status FROM maintenance.slot_proposals WHERE id = $1), (SELECT status FROM
			maintenance.partner_slot_proposals WHERE id = $2)`,
		[]any{proposal, partner},
		&clientStatus,
		&partnerStatus,
	)

	if clientStatus != statusWithdrawn || partnerStatus != statusWithdrawn {
		t.Errorf(
			"after the decline: client proposal %s, time change %s",
			clientStatus,
			partnerStatus,
		)
	}

	if code, _ := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		5,
	); code != 409 {
		t.Errorf("the client accepted a proposal of the company that declined: %d", code)
	}

	var offers int
	ownerScan(t, `SELECT count(*) FROM maintenance.offers WHERE job_id = $1`, []any{job}, &offers)

	if offers != 1 {
		t.Errorf("offers after the decline: %d", offers)
	}
}

// TestAcceptRechecksHold (IR446): the client's acceptance checks the proposal's hold again. HQ suspends the held
// contractor after proposing; the acceptance is CONFLICT errors.proposal_unavailable and no offer is created.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAcceptRechecksHold(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := asStr(t, data(resp)["id"])

	hold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `"}`
	if code, res := write(
		srv,
		hq(),
		"jobs.proposeSlot",
		`{"jobId":"`+job+`","slot":`+slotJSON(
			98,
			2,
		)+`,"hold":`+hold+`,"message":"A partner can come then","replyBy":"`+stamp(
			48,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("propose: %d %v", code, res)
	}

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation, suspended_reason)
		VALUES ($1, $2, 'Demo Contractor A', 'suspended', 'SSM-TEST', 'ops@contractor-a.example', tstzrange($3, $4), 'test')`,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
		clock().Add(24*365*time.Hour),
	)
	t.Cleanup(func() {
		owner(
			t,
			`DELETE FROM maintenance.contractors WHERE organization_id = $1`,
			seed.ID("org-contractor-a"),
		)
	})

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		2,
	); code != 409 ||
		res["messageKey"] != "errors.proposal_unavailable" {
		t.Errorf("accepting a suspended contractor's hold: %d %v", code, res)
	}

	var (
		offers int
		status string
	)
	ownerScan(
		t,
		`SELECT (SELECT count(*) FROM maintenance.offers WHERE job_id = $1), (SELECT status FROM maintenance.slot_proposals
			WHERE id = $2)`,
		[]any{job, proposal},
		&offers,
		&status,
	)

	if offers != 0 || status != statusPending {
		t.Errorf("after the refused acceptance: %d offers, proposal %s", offers, status)
	}
}

// TestAcceptedProposalExtendsAccess (IR454): a contractor's time change after the offer's access window ends moves the
// visit, and the access window follows it to the new slot end + 24 h.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAcceptedProposalExtendsAccess(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-external-a"),
		clock().Add(2*time.Hour),
	)

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
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
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer, partner, proposal string
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
		`{"jobId":"`+job+`","offerId":"`+offer+`","slot":`+slotJSON(101, 2)+
			`,"technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","reason":"team busy"}`,
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
		`{"jobId":"`+job+`","proposalId":"`+partner+`","decision":"send_to_client","replyBy":"`+stamp(
			24,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("send to client: %d %v", code, res)
	}

	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)
	// a contractor history frozen earlier (IR458) is removed when the window widens
	owner(
		t,
		`INSERT INTO maintenance.job_history_snapshots (tenant_id, job_id, owner_kind, owner_id, as_of, type, status,
			has_report, acceptance)
		SELECT tenant_id, id, 'organization', $2, $3, type, status, false, 'not_accepted' FROM maintenance.jobs WHERE id =
		$1`,
		job,
		seed.ID("org-contractor-a"),
		clock(),
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		4,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	var snapshots int
	ownerScan(
		t,
		`SELECT count(*) FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'organization'`,
		[]any{job},
		&snapshots,
	)

	if snapshots != 0 {
		t.Errorf("the contractor's frozen history stays after the window widened: %d", snapshots)
	}

	var until time.Time
	ownerScan(
		t,
		`SELECT access_valid_until FROM maintenance.offers WHERE id = $1`,
		[]any{offer},
		&until,
	)

	if want := clock().Add(103*time.Hour + 24*time.Hour); !until.Equal(want) {
		t.Errorf("access until %v, want %v (the moved visit's end + 24 h)", until, want)
	}
}

// TestProposalReplyByBeforeSlot (IR455): the reply-by cannot fall after the proposed time starts, and a proposal whose
// time has begun cannot be accepted.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestProposalReplyByBeforeSlot(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := asStr(t, data(resp)["id"])
	hold := `{"kind":"contractor","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `"}`

	propose := func(replyBy string) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"jobs.proposeSlot",
			`{"jobId":"`+job+`","slot":`+slotJSON(
				30,
				2,
			)+`,"hold":`+hold+`,"message":"Can we come then?","replyBy":"`+replyBy+`"}`,
			1,
		)
	}
	if code, m := propose(stamp(31)); code != 422 || fields(m) != refusedReplyByRange {
		t.Errorf("reply-by after the slot starts: %d %s", code, fields(m))
	}

	if code, m := propose(stamp(30)); code != 200 {
		t.Fatalf("reply-by at the slot start: %d %v", code, m)
	}

	var proposal string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`,
		[]any{job},
		&proposal,
	)
	// the proposed time has begun (moved into the past directly): the acceptance is refused and nothing is booked
	owner(
		t,
		`UPDATE maintenance.slot_proposals SET slot = tstzrange($2, $3), reply_by = $4 WHERE id = $1`,
		proposal,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
		clock().Add(time.Hour),
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		2,
	); code != 409 ||
		res["messageKey"] != "errors.proposal_expired" {
		t.Errorf("accepting a time that has begun: %d %v", code, res)
	}

	var offers int
	ownerScan(t, `SELECT count(*) FROM maintenance.offers WHERE job_id = $1`, []any{job}, &offers)

	if offers != 0 {
		t.Errorf("offers after the refused acceptance: %d", offers)
	}
}

// firstCandidate is the start of the first candidate time of an Offer (IR636).
func firstCandidate(t *testing.T, offer any) any {
	t.Helper()

	slots, ok := asObj(t, offer)["candidateSlots"].([]any)
	if !ok || len(slots) == 0 {
		t.Fatalf("offer without candidateSlots: %v", offer)
	}

	return asObj(t, slots[0])["startAt"]
}
