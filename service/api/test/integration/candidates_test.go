package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestContractorPicksCandidateTime (IR636): an offer carries all the client's agreed times, the contractor settles one
// when it assigns a technician, and after "can't make it" it takes another candidate, which the client is told.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractorPicksCandidateTime(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	// the seed assignment of tech-external-a (09-14..09-20) would overlap the candidate times
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE id = $1`,
		seed.ID("assignment-contractor-a"),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM
				maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND
			b.scheduled && a.scheduled)`,
			seed.ID("assignment-contractor-a"),
		)
	})

	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	job := asStr(t, data(resp)["id"])
	version := func() int {
		t.Helper()

		_, res := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		return int(asNum(t, data(res)["version"]))
	}

	// HQ offers without choosing a time: the offer holds the three preferred times
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+
			`","offerExpiresAt":"`+stamp(12)+`","accessValidFrom":"`+stamp(0)+`","accessValidUntil":"`+stamp(100)+
			`","termsVersion":"t"}`,
		version(),
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	_, detail := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

	candidates, ok := asObj(t, data(detail)["offer"])["candidateSlots"].([]any)
	if !ok || len(candidates) != 3 || asObj(t, candidates[2])["startAt"] != stamp(73) {
		t.Fatalf("the offer holds the client's three times: %v", data(detail)["offer"])
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
		version(),
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	assign := func(slot string) (int, map[string]any) {
		return write(
			srv,
			contrA(),
			"jobs.assign",
			`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").String()+
				`","startAt":"`+slot[12:32]+`","endAt":"`+slot[43:63]+`"}`,
			version(),
		)
	}

	// a stale version is refused after the scope check, before the slot is looked at (IR366)
	if code, res := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").String()+
			`","startAt":"`+stamp(25)+`","endAt":"`+stamp(27)+`"}`,
		version()-1,
	); code != 409 || res["messageKey"] != "error.versionConflict" {
		t.Fatalf("assign with a stale version: %d %v", code, res)
	}

	// the contractor settles the first candidate; the client hears of the confirmed time
	if code, res := assign(slotJSON(25, 2)); code != 200 || data(res)["status"] != statusAssigned {
		t.Fatalf("assign the first candidate: %d %v", code, res)
	}

	if countNotes(t, job, "customer-a", "schedule_change") != 1 {
		t.Errorf(
			"the client is told the confirmed time: %d",
			countNotes(t, job, "customer-a", "schedule_change"),
		)
	}
	// the technician cannot make it: the contractor takes the third candidate, not a time of its own
	if code, res := write(
		srv,
		techA(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"cant_make","reason":"another call"}`,
		version(),
	); code != 200 {
		t.Fatalf("cant_make: %d %v", code, res)
	}

	if code, res := assign(slotJSON(30, 2)); code != 422 || fields(res) != refusedSlotNotAgreed {
		t.Errorf("a time that is not a candidate: %d %s", code, fields(res))
	}

	code, res := assign(slotJSON(73, 1))
	if code != 200 || asObj(t, data(res)["scheduledSlot"])["startAt"] != stamp(73) {
		t.Fatalf("assign the third candidate: %d %v", code, res)
	}

	if countNotes(t, job, "customer-a", "schedule_change") != 2 {
		t.Errorf(
			"the client is told the new time: %d",
			countNotes(t, job, "customer-a", "schedule_change"),
		)
	}
}

// TestOfferAccessCoversEarliestCandidate (IR636 item 2): the client ranks a later time first, so the access window
// must start by the earliest candidate, not by the first-ranked one.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestOfferAccessCoversEarliestCandidate(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(
			seed.ID("unit-non-rto").String(),
			map[string]string{
				fieldAlternativeSlots: "[" + slotJSON(49, 2) + "," + slotJSON(16, 2) + "]",
			},
		),
		0,
	)
	job := asStr(t, data(resp)["id"])
	_, detail := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

	code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+
			`","offerExpiresAt":"`+stamp(12)+`","accessValidFrom":"`+stamp(20)+`","accessValidUntil":"`+stamp(100)+
			`","termsVersion":"t"}`,
		int(asNum(t, data(detail)["version"])),
	)
	if code != 422 || fields(res) != refusedAccessCover {
		t.Errorf("access from +20 h misses the +16 h candidate: %d %s", code, fields(res))
	}
}

// TestOfferCandidateSources (IR636): besides the client's preferred times, an offer's candidates are the slots of the
// job's accepted proposals, and a periodic job with no preferred time offers its plan occurrence.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestOfferCandidateSources(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	contractor := seed.ID("org-contractor-a").String()
	version := func(job string) int {
		t.Helper()

		_, res := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		return int(asNum(t, data(res)["version"]))
	}
	offer := func(job string) []any {
		t.Helper()

		if code, res := write(
			srv,
			hq(),
			"jobs.offer",
			`{"jobId":"`+job+`","contractorOrgId":"`+contractor+`","offerExpiresAt":"`+stamp(12)+
				`","accessValidFrom":"`+stamp(0)+`","accessValidUntil":"`+stamp(200)+`","termsVersion":"t"}`,
			version(job),
		); code != 200 {
			t.Fatalf("offer %s: %d %v", job, code, res)
		}

		_, detail := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		candidates, _ := asObj(t, data(detail)["offer"])["candidateSlots"].([]any)

		return candidates
	}
	openOffer := func(job string) string {
		var id string
		ownerScan(
			t,
			`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
			[]any{job},
			&id,
		)

		return id
	}

	// the client accepts HQ's contractor-held time, the contractor declines that offer: HQ's next offer holds the
	// three preferred times and then the accepted one
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
		"jobs.proposeSlot",
		`{"jobId":"`+job+`","slot":`+slotJSON(
			98,
			2,
		)+`,"hold":{"kind":"contractor","contractorOrgId":"`+contractor+
			`","technicianMembershipId":null},"message":"Can we come then?","replyBy":"`+stamp(
			48,
		)+`"}`,
		version(job),
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

	if code, res := write(
		srv,
		customerA(),
		"jobs.respondProposal",
		`{"jobId":"`+job+`","proposalId":"`+proposal+`","decision":"accept"}`,
		version(job),
	); code != 200 || data(res)["status"] != statusOffered {
		t.Fatalf("accept the proposal: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		opJobsDecline,
		`{"jobId":"`+job+`","offerId":"`+openOffer(job)+`","reason":"no capacity"}`,
		version(job),
	); code != 200 {
		t.Fatalf("decline: %d %v", code, res)
	}

	candidates := offer(job)
	if len(candidates) != 4 || asObj(t, candidates[0])["startAt"] != stamp(25) ||
		asObj(t, candidates[2])["startAt"] != stamp(
			73,
		) || asObj(t, candidates[3])["startAt"] != stamp(98) {
		t.Errorf("the preferred times and then the accepted proposal: %v", candidates)
	}

	// a periodic job without preferred times offers its plan occurrence (+70 h, not the requested +60 h), at the
	// requested length (3 h)
	plan, planJob := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,
			8,$4)`,
		plan,
		seed.ID("tenant-a"),
		seed.ID("unit-non-rto"),
		clock(),
	)
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin,
			requested_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','requested','periodic_plan',tstzrange($7,$8),$8)`,
		planJob,
		seed.ID("tenant-a"),
		seed.ID("unit-non-rto"),
		seed.ID("org-customer-a"),
		plan,
		clock().Add(70*time.Hour),
		clock().Add(60*time.Hour),
		clock().Add(63*time.Hour),
	)

	candidates = offer(planJob)
	if len(candidates) != 1 || asObj(t, candidates[0])["startAt"] != stamp(70) ||
		asObj(t, candidates[0])["endAt"] != stamp(73) {
		t.Errorf("the plan occurrence: %v", candidates)
	}
}

// TestInstallationRequestStatusFilter: installationRequests.list refuses a status outside the four request states as
// VALIDATION naming the filter (an unknown filter is refused earlier, by the REST binding).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestInstallationRequestStatusFilter(t *testing.T) {
	srv := server(t)

	code, res := post(
		srv,
		customerA(),
		"installationRequests.list",
		`{"filters":{"status":"lost"},"limit":10}`,
	)
	refusal(t, "an unknown status", code, res, "filters.status=error.invalid")
}
