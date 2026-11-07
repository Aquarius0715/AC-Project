package integration_test

import (
	"testing"
	"time"

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
