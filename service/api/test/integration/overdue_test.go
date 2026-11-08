package integration_test

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// overdueJob offers a client's job to contractor-a, which accepts it; it returns the job and a helper that reads its
// version.
func overdueJob(t *testing.T, srv *apiserver.Server) (string, func() int) {
	t.Helper()

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

	return job, version
}

// proposeOverdue is HQ proposing a time held for contractor-a, a week after the fixture clock.
func proposeOverdue(srv *apiserver.Server, job string, version int) (int, map[string]any) {
	return write(
		srv,
		hq(),
		"jobs.proposeSlot",
		`{"jobId":"`+job+`","slot":`+slotJSON(
			97,
			2,
		)+`,"hold":{"kind":"contractor","contractorOrgId":"`+
			seed.ID("org-contractor-a").
				String()+
			`","technicianMembershipId":null},"message":"The booked time passed",`+
			`"replyBy":"`+clock().Add(48*time.Hour).
			Format(time.RFC3339)+
			`"}`,
		version,
	)
}

// TestOverdueAssignedJobIsRescheduled (IR642): a job still "assigned" after its booked time cannot be given to a
// technician for that time again, and HQ may propose another time although the technician accepted; once the client
// accepts it, the contractor assigns again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestOverdueAssignedJobIsRescheduled(t *testing.T) {
	srv := server(t)
	job, version := overdueJob(t, srv)

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

	if code, res := assign(slotJSON(25, 2)); code != 200 || data(res)["status"] != statusAssigned {
		t.Fatalf("assign the first candidate: %d %v", code, res)
	}

	if code, res := write(
		srv,
		techA(),
		"jobs.acknowledgeAssignment",
		`{"jobId":"`+job+`","decision":"accept"}`,
		version(),
	); code != 200 {
		t.Fatalf("the technician accepts: %d %v", code, res)
	}
	// the booked time is still ahead and the technician accepted: nothing to propose
	if code, res := proposeOverdue(srv, job, version()); code != 409 {
		t.Fatalf("a proposal before the booked time ended: %d %v", code, res)
	}
	// nobody started and the booked time passes
	owner(
		t,
		`UPDATE maintenance.jobs SET scheduled_slot = tstzrange($2::timestamptz, $3::timestamptz) WHERE id = $1`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2::timestamptz,
			$3::timestamptz) WHERE job_id = $1 AND status = 'active'`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)
	// the same time cannot be assigned again: the new technician could never open it
	past := `{"startAt":"` + clock().Add(-3*time.Hour).Format(time.RFC3339) + `","endAt":"` +
		clock().Add(-time.Hour).Format(time.RFC3339) + `"}`
	if code, res := assign(past); code != 422 || fields(res) != "endAt=error.past" {
		t.Errorf("assign the time that is over: %d %s", code, fields(res))
	}
	// HQ proposes another time, the client accepts, and the job goes back to the contractor
	if code, res := proposeOverdue(srv, job, version()); code != 200 {
		t.Fatalf("propose after the booked time ended: %d %v", code, res)
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
		version(),
	); code != 200 {
		t.Fatalf("the client accepts: %d %v", code, res)
	}

	var status string

	var active int
	ownerScan(
		t,
		`SELECT j.status, (SELECT count(*) FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active')
		FROM maintenance.jobs j WHERE j.id = $1`,
		[]any{job},
		&status,
		&active,
	)

	if status != statusAccepted || active != 0 {
		t.Fatalf("after the client accepted: status %s, active assignments %d", status, active)
	}

	if code, res := assign(slotJSON(97, 2)); code != 200 || data(res)["status"] != statusAssigned {
		t.Errorf("the contractor assigns the new time: %d %v", code, res)
	}
}

// TestAcceptedJobWithNoTimeLeftIsRescheduled (IR642): when every time the contractor was offered is over and it
// assigned nobody, HQ may propose another time (before that only a time change of the contractor allowed it).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAcceptedJobWithNoTimeLeftIsRescheduled(t *testing.T) {
	srv := server(t)
	job, version := overdueJob(t, srv)

	if code, res := proposeOverdue(srv, job, version()); code != 409 {
		t.Fatalf("a proposal while the offered times are ahead: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE maintenance.offers SET candidate_slots = jsonb_build_array(jsonb_build_object('startAt', $2::timestamptz,
			'endAt', $3::timestamptz)) WHERE job_id = $1 AND decision = 'accept'`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	if code, res := proposeOverdue(srv, job, version()); code != 200 {
		t.Errorf("a proposal when every offered time is over: %d %v", code, res)
	}
}
