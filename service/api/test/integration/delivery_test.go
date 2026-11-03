package integration_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	fieldOfferExpiresAt  = "offerExpiresAt"
	refusedSlotNotAgreed = "startAt=errors.slot_not_agreed"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestOfferAcceptAssign(t *testing.T) {
	srv := server(t)
	// earlier runs leave active assignments for these technicians in the same slots
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
		seed.ID("org-contractor-b"),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(resp)["id"])
	slot2 := slotJSON(49, 2)
	offerBody := func(extra map[string]string) string {
		f := map[string]string{
			fieldJobID:           `"` + job + `"`,
			fieldContractorOrgID: `"` + seed.ID("org-contractor-a").String() + `"`,
			fieldVisitSlot:       slot2,
			fieldOfferExpiresAt: `"` + stamp(
				12,
			) + `"`,
			"accessValidFrom":     `"` + stamp(0) + `"`,
			fieldAccessValidUntil: `"` + stamp(100) + `"`,
			fieldTermsVersion:     `"terms-v1"`,
		}
		maps.Copy(f, extra)

		out := "{"
		for k, v := range f {
			if len(out) > 1 {
				out += ","
			}

			out += `"` + k + `":` + v
		}

		return out + "}"
	}
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"access does not cover": "accessValidFrom=error.accessMustCoverVisit",
		"blank terms":           "termsVersion=error.length",
		"expiry after access":   "offerExpiresAt=error.afterAccess",
		"expiry in the past":    "offerExpiresAt=error.past",
		"slot not agreed":       "visitSlot=errors.slot_not_agreed",
	}

	for name, testCase := range map[string]struct {
		extra map[string]string
		code  int
	}{
		"expiry in the past":    {map[string]string{fieldOfferExpiresAt: `"` + stamp(-1) + `"`}, 422},
		"expiry after access":   {map[string]string{fieldOfferExpiresAt: `"` + stamp(101) + `"`}, 422},
		"access does not cover": {map[string]string{fieldAccessValidUntil: `"` + stamp(50) + `"`}, 422},
		"slot not agreed":       {map[string]string{fieldVisitSlot: slotJSON(30, 2)}, 422},
		"blank terms":           {map[string]string{fieldTermsVersion: `""`}, 422},
		"customer organization": {
			map[string]string{fieldContractorOrgID: `"` + seed.ID("org-customer-a").String() + `"`},
			404,
		},
		"unknown organization": {map[string]string{fieldContractorOrgID: `"` + uuid.NewString() + `"`}, 404},
	} {
		code, m := write(srv, hq(), "jobs.offer", offerBody(testCase.extra), 1)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != testCase.code {
			t.Errorf("offer %s: %d want %d", name, code, testCase.code)
		}
	}

	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation)
		VALUES ($1,$2,'B','suspended','R-1','b@example.com',tstzrange($3,$4))`,
		seed.ID("tenant-a"),
		seed.ID("org-contractor-b"),
		clock().Add(-time.Hour),
		clock().Add(1000*time.Hour),
	)

	if code, _ := write(
		srv,
		hq(),
		"jobs.offer",
		offerBody(
			map[string]string{
				fieldContractorOrgID: `"` + seed.ID("org-contractor-b").String() + `"`,
			},
		),
		1,
	); code != 409 {
		t.Error("suspended contractor")
	}

	if code, _ := write(srv, customerA(), "jobs.offer", offerBody(nil), 1); code != 403 {
		t.Error("client offer")
	}

	code, resp := write(srv, hq(), "jobs.offer", offerBody(nil), 1)
	if code != 200 || data(resp)["status"] != statusOffered ||
		data(resp)["contractorOrgId"] != seed.ID("org-contractor-a").String() {
		t.Fatalf("offer: %d %v", code, resp)
	}

	if code, _ := write(srv, hq(), "jobs.offer", offerBody(nil), 2); code != 409 {
		t.Error("offer twice")
	}

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	acc := func(a *actor, terms string, version int) (int, map[string]any) {
		return write(
			srv,
			a,
			"jobs.accept",
			`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"`+terms+`"}`,
			version,
		)
	}
	if code, _ := acc(contrB(), "terms-v1", 2); code != 404 {
		t.Error("other contractor")
	}

	if code, _ := acc(contrA(), "terms-v0", 2); code != 409 {
		t.Error("terms changed")
	}

	if code, _ := acc(contrA(), "terms-v1", 1); code != 409 {
		t.Error("stale job version")
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`"}`,
		2,
	); code != 422 ||
		fields(res) != "termsVersion=error.required" {
		t.Errorf("accept without terms: %d %s", code, fields(res))
	}

	code, resp = acc(contrA(), "terms-v1", 2)
	if code != 200 || data(resp)["decision"] != "accept" ||
		asNum(t, data(resp)["jobVersion"]) != 3 ||
		len(data(resp)) != 4 {
		t.Fatalf("accept: %d %v", code, resp)
	}

	if code, _ := acc(contrA(), "terms-v1", 3); code != 404 {
		t.Error("decided offer")
	}
	// contractor assigns its own technician at the visit slot
	asg := func(who *actor, jobID, tech, slot string, version int, reason string) (int, map[string]any) {
		start, end := slot[12:32], slot[43:63]

		reasonField := ""
		if reason != "" {
			reasonField = `,"reason":"` + reason + `"`
		}

		return write(
			srv,
			who,
			"jobs.assign",
			`{"jobId":"`+jobID+`","technicianMembershipId":"`+seed.ID(tech).
				String()+
				`","startAt":"`+start+`","endAt":"`+end+`"`+reasonField+`}`,
			version,
		)
	}
	// the seed assignment of tech-external-a (09-14..09-20) would overlap: revoke it for this test
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE id = $1`,
		seed.ID("assignment-contractor-a"),
	)
	t.Cleanup(func() { // restore unless a test assignment now holds the window
		owner(
			t,
			`UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM
				maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND
			b.scheduled && a.scheduled)`,
			seed.ID("assignment-contractor-a"),
		)
	})

	if code, _ := asg(contrA(), job, "tech-internal-a", slot2, 3, ""); code != 403 {
		t.Error("contractor assigns another organization's technician")
	}

	if code, res := asg(
		contrA(),
		job,
		"tech-external-a",
		slotJSON(25, 2),
		3,
		"",
	); code != 422 ||
		fields(res) != refusedSlotNotAgreed {
		t.Errorf("contractor slot must be the visit slot: %d %s", code, fields(res))
	}

	if code, _ := asg(contrB(), job, "tech-external-b", slot2, 3, ""); code != 404 {
		t.Error("other contractor assign")
	}

	code, resp = asg(contrA(), job, "tech-external-a", slot2, 3, "")
	if code != 200 || data(resp)["status"] != statusAssigned || data(resp)["assignmentId"] == nil ||
		data(resp)["scheduledSlot"] == nil {
		t.Fatalf("contractor assign: %d %v", code, resp)
	}

	if _, extra := data(resp)["assignment"]; extra { // the write answers MaintenanceJob, the detail is jobs.get's (IR392)
		t.Errorf("jobs.assign answers the detail: %v", data(resp))
	}

	if _, detail := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); asObj(t, data(detail)["assignment"])["acknowledgement"] != statusPending {
		t.Errorf("the assignment waits for the technician: %v", data(detail)["assignment"])
	}

	if code, _ := asg(hq(), job, "tech-internal-a", slot2, 4, ""); code != 403 {
		t.Error("HQ assigns internal jobs only")
	}

	// internal job: HQ assigns
	_, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	key := asStr(t, data(resp)["id"])

	slot1 := slotJSON(25, 2)
	if code, _ := asg(hq(), key, "tech-external-a", slot1, 1, ""); code != 403 {
		t.Error("HQ assigns external technician")
	}

	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		slotJSON(26, 2),
		1,
		"",
	); code != 422 ||
		fields(res) != refusedSlotNotAgreed {
		t.Errorf("slot not agreed: %d %s", code, fields(res))
	}

	if code, _ := asg(hq(), key, "customer-a", slot1, 1, ""); code != 404 {
		t.Error("not a technician")
	}

	if code, _ := asg(hq(), key, "tech-internal-a", slot1, 1, ""); code != 200 {
		t.Fatal("hq assign")
	}

	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		slotJSON(49, 2),
		2,
		"",
	); code != 422 ||
		fields(res) != refusedSlotNotAgreed {
		t.Errorf("reassignment keeps the agreed slot: %d %s", code, fields(res))
	}

	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		slot1,
		2,
		"swap",
	); code != 200 ||
		data(res)["status"] != statusAssigned {
		t.Fatalf("reassign: %d", code)
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, key)

	ext := `{"startAt":"` + stamp(25) + `","endAt":"` + stamp(28) + `"}`
	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		ext,
		3,
		"",
	); code != 422 ||
		fields(res) != refusedReasonRequired {
		t.Errorf("extension needs a reason: %d %s", code, fields(res))
	}

	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		slotJSON(24, 4),
		3,
		"x",
	); code != 422 ||
		fields(res) != "endAt=errors.extension_only" {
		t.Errorf("extension keeps the start: %d %s", code, fields(res))
	}

	if code, res := asg(
		hq(),
		key,
		"tech-internal-a",
		ext,
		3,
		"more work",
	); code != 200 ||
		data(res)["status"] != statusInProgress {
		t.Fatalf("extend: %d %v", code, res)
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, key)

	if code, _ := asg(hq(), key, "tech-internal-a", ext, 4, "x"); code != 409 {
		t.Error("assign completed job")
	}
	// overlap with the active assignment of the same technician
	_, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	overlapJob := asStr(t, data(resp)["id"])

	owner(t, `UPDATE maintenance.jobs SET status = 'requested' WHERE id = $1`, key)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'active' WHERE job_id = $1 AND upper(scheduled) = $2`,
		key,
		clock().Add(28*time.Hour),
	)

	if code, _ := asg(hq(), overlapJob, "tech-internal-a", slot1, 1, ""); code != 409 {
		t.Error("overlapping assignment")
	}
	// out of scope and qualification
	other := newUnit(t, srv, "Scope AC")

	_, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(other, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	if code, _ := asg(hq(), asStr(t, data(resp)["id"]), "tech-internal-a", slot1, 1, ""); code != 403 {
		t.Error("technician out of scope")
	}

	late := clock().Add(20 * 24 * time.Hour)
	lateSlot := `{"startAt":"` + late.Format(
		time.RFC3339,
	) + `","endAt":"` + late.Add(2*time.Hour).
		Format(time.RFC3339) +
		`"}`

	_, resp = write(srv, hq(), "jobs.create", jobBody(unit, map[string]string{
		fieldAlternativeSlots: "[]", fieldRequestedStart: `"` + late.Format(time.RFC3339) + `"`,
		fieldRequestedEnd: `"` + late.Add(2*time.Hour).Format(time.RFC3339) + `"`,
	}), 0)
	if code, _ := asg(
		hq(),
		asStr(t, data(resp)["id"]),
		"tech-internal-a",
		lateSlot,
		1,
		"",
	); code != 403 {
		t.Error("qualification expires before the slot")
	}

	// decline and expiry
	_, resp = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job = asStr(t, data(resp)["id"])

	write(srv, hq(), "jobs.offer", offerBody(nil), 1)
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.decline",
		`{"jobId":"`+job+`","offerId":"`+offer+`","reason":"  "}`,
		2,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("blank decline reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.decline",
		`{"jobId":"`+job+`","offerId":"`+offer+`","reason":"no capacity"}`,
		2,
	); code != 200 ||
		data(res)["decision"] != "decline" {
		t.Fatalf("decline: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); data(res)["status"] != statusRequested ||
		data(res)["contractorOrgId"] != nil {
		t.Fatal("decline returns the job to requested")
	}
	// the company's own decision stays readable as offer.declined (history snapshots keep accept / decline records)
	var action string
	ownerScan(
		t,
		`SELECT action FROM maintenance.job_events WHERE job_id = $1 AND action LIKE 'offer.%' ORDER BY occurred_at DESC, id
			DESC LIMIT 1`,
		[]any{job},
		&action,
	)

	if action != "offer.declined" {
		t.Fatalf("decline event action %q", action)
	}

	write(srv, hq(), "jobs.offer", offerBody(nil), 3)
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`, offer, clock())

	if code, _ := acc(contrA(), "terms-v1", 4); code != 409 {
		t.Error("expired offer")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAgreedSlotsAndInputs(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-internal-a"),
		clock().Add(2*time.Hour),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	// an accepted SlotProposal makes its slot agreed
	_, res := write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	job := asStr(t, data(res)["id"])
	owner(
		t,
		`INSERT INTO maintenance.slot_proposals (tenant_id, job_id, source, slot, hold, message, reply_by, status)
		VALUES ($1,$2,'hq',tstzrange($3,$4),'{"kind":"internal"}','m',$4,'accepted')`,
		seed.ID("tenant-a"),
		job,
		clock().Add(55*time.Hour),
		clock().Add(57*time.Hour),
	)

	if code, resp := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			55,
		)+`","endAt":"`+stamp(
			57,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("accepted proposal slot: %d %v", code, resp)
	}
	// a plan occurrence is agreed for periodic jobs
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
		clock().Add(60*time.Hour),
		clock().Add(62*time.Hour),
	)

	if code, resp := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+planJob+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			60,
		)+`","endAt":"`+stamp(
			62,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("plan occurrence slot: %d %v", code, resp)
	}
	// input shape errors: reversed times, and a field of the other decision (not an input field: malformed)
	for operation, testCase := range map[string]struct {
		body   string
		fields []string
	}{
		"jobs.offer": {`{"jobId":"` + job + `","contractorOrgId":"` + seed.ID("org-contractor-a").String() +
			`","visitSlot":{"startAt":"` + stamp(3) + `","endAt":"` + stamp(2) +
			`"},"offerExpiresAt":"` + stamp(5) + `","accessValidFrom":"` + stamp(6) + `","accessValidUntil":"` + stamp(4) +
			`","termsVersion":"t"}`, []string{fieldVisitSlot, fieldAccessValidUntil}},
		opJobsAccept:  {`{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","reason":"x"}`, []string{"_"}},
		opJobsDecline: {`{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t"}`, []string{"_"}},
		opJobsAssign: {`{"jobId":"` + job + `","technicianMembershipId":"` + seed.ID("tech-internal-a").String() +
			`","startAt":"` + stamp(3) + `","endAt":"` + stamp(2) + `"}`, []string{fieldEndAt}},
	} {
		a := hq()
		if operation == opJobsAccept || operation == opJobsDecline {
			a = contrA()
		}

		code, m := write(srv, a, operation, testCase.body, 1)

		fe, _ := m["fieldErrors"].(map[string]any)
		for _, f := range testCase.fields {
			if code != 422 || fe[f] == nil {
				t.Errorf("%s shape: %d %v, want %s", operation, code, fe, f)
			}
		}
	}
	// both terms and a reason: the reason is not an input of accept (it was refused for the missing jobId before)
	if code, resp := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`","termsVersion":"t","reason":"x"}`,
		1,
	); code != 422 ||
		fields(resp) != refusedMalformedInput {
		t.Errorf("both terms and reason: %d %s", code, fields(resp))
	}
	// each decision takes only its own fields (service-contracts: accept termsVersion, decline reason)
	for operation, body := range map[string]string{
		opJobsAccept: `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t","reason":"x"}`,
		opJobsDecline: `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t","reason":"no ` +
			`capacity"}`,
	} {
		if code, resp := write(
			srv,
			contrA(),
			operation,
			body,
			1,
		); code != 422 ||
			asObj(t, resp["fieldErrors"])["_"] != apperr.KeyMalformedInput {
			t.Errorf("%s with the other decision's field: %d %v", operation, code, resp)
		}
	}

	if code, resp := write(
		srv,
		contrA(),
		"jobs.decline",
		`{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`"}`,
		1,
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["reason"] != apperr.KeyLength {
		t.Errorf("decline without a reason: %d %v", code, resp)
	}

	if code, resp := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`"}`,
		1,
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["termsVersion"] != apperr.KeyRequired {
		t.Errorf("accept without terms: %d %v", code, resp)
	}

	long := make([]byte, 1001)
	for i := range long {
		long[i] = 'r'
	}

	if code, resp := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			55,
		)+`","endAt":"`+stamp(
			57,
		)+`","reason":"`+string(
			long,
		)+`"}`,
		2,
	); code != 422 ||
		fields(resp) != refusedReasonLength {
		t.Errorf("reason too long: %d %s", code, fields(resp))
	}
	// contractor outside the access window
	_, res = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job2 := asStr(t, data(res)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job2+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	)

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job2},
		&offer,
	)
	write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job2+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	)

	if code, _ := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job2+`","technicianMembershipId":"`+seed.ID("tech-external-a").
			String()+
			`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		3,
	); code != 403 {
		t.Error("before the access window")
	}
}

// TestAssignWithinAccess (IR433): a technician is assigned only to a slot that ends before their access does; a slot
// past the access end is FORBIDDEN errors.technician_access_ends, and the same slot is fine once the access lasts.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAssignWithinAccess(t *testing.T) {
	srv := server(t)
	tech := seed.ID("tech-internal-a")

	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	t.Cleanup(
		func() { owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until) },
	)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)
	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(26*time.Hour),
	)

	code, res := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	if code != 200 {
		t.Fatalf("create: %d %v", code, res)
	}

	job := asStr(t, data(res)["id"])
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	assign := func(from, end int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"jobs.assign",
			`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
				from,
			)+`","endAt":"`+stamp(
				end,
			)+`"}`,
			1,
		)
	}
	if code, m := assign(
		25,
		27,
	); code != 403 ||
		m["messageKey"] != "errors.technician_access_ends" {
		t.Errorf("a slot past the access end: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(28*time.Hour),
	) // the agreed slot is fixed: extend the access

	if code, m := assign(25, 27); code != 200 {
		t.Errorf("the same slot within the access: %d %v", code, m)
	}
}

// TestReleaseEndedAccess (OPEN-BE-09, IR439): HQ shortens a booked technician's access to end before the visit; the
// maintenance scheduler releases the assignment of the job that has not started, the job goes back to be assigned
// (requested for HQ's own delivery), and HQ's assigners get a job_update notice.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestReleaseEndedAccess(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	tech := seed.ID("tech-internal-a")

	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until)
		owner(
			t,
			`UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`,
			tech,
			until,
		)
	})
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)

	code, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	if code != 200 {
		t.Fatalf("create: %d %v", code, resp)
	}

	job := asStr(t, data(resp)["id"])

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, res := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
			25,
		)+`","endAt":"`+stamp(
			27,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	before := countNotes(t, job, "hq-operator", "job_update")
	// HQ ends the access tomorrow, before the visit ends (Access & roles); maintenance holds its copy of the membership
	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(24*time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(24*time.Hour),
	)

	_, err := schedTick(ctx, srv, clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	var (
		status, reason string
		assignment     *string
	)
	ownerScan(
		t,
		`SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id =
			j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
		[]any{job},
		&status,
		&assignment,
		&reason,
	)

	if status != statusRequested || assignment != nil || reason != "access_ended" {
		t.Errorf(
			"after the release: status %s, assignment %v, reason %s",
			status,
			assignment,
			reason,
		)
	}

	if n := countNotes(t, job, "hq-operator", "job_update"); n != before+1 {
		t.Errorf("HQ notices: %d, want %d", n, before+1)
	}
	// nothing more to release on the next tick
	_, err = schedTick(ctx, srv, clock().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if n := countNotes(t, job, "hq-operator", "job_update"); n != before+1 {
		t.Errorf("released twice: %d notices", n)
	}
}

// TestReleaseEndedAccessContractor (IR439): the contractor's job whose technician's access now ends before the visit
// goes back to accepted with its offer, and the contractor's assigners get the job_update notice.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestReleaseEndedAccessContractor(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	tech := seed.ID("tech-external-a")

	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	// the seed assignment of tech-external-a (09-14..09-20) would overlap the visit: revoke it for this test
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until)
		owner(
			t,
			`UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`,
			tech,
			until,
		)
		owner(
			t,
			`UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM
				maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND
			b.scheduled && a.scheduled)`,
			seed.ID("assignment-contractor-a"),
		)
	})

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	code, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	if code != 200 {
		t.Fatalf("create: %d %v", code, resp)
	}

	job := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"terms-v1"}`,
		1,
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
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"terms-v1"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	before, hqBefore := countNotes(
		t,
		job,
		"contractor-a",
		"job_update",
	), countNotes(
		t,
		job,
		"hq-operator",
		"job_update",
	)
	// HQ ends the access during the visit (Access & roles)
	owner(
		t,
		`UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(50*time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`,
		tech,
		clock().Add(50*time.Hour),
	)

	_, err := schedTick(ctx, srv, clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	var (
		status, reason string
		assignment     *string
	)
	ownerScan(
		t,
		`SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id =
			j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
		[]any{job},
		&status,
		&assignment,
		&reason,
	)

	if status != statusAccepted || assignment != nil || reason != "access_ended" {
		t.Errorf(
			"after the release: status %s, assignment %v, reason %s",
			status,
			assignment,
			reason,
		)
	}

	if n := countNotes(t, job, "contractor-a", "job_update"); n != before+1 {
		t.Errorf("contractor notices: %d, want %d", n, before+1)
	}

	if n := countNotes(t, job, "hq-operator", "job_update"); n != hqBefore {
		t.Errorf("HQ assigners notified of the contractor's job: %d", n)
	}
}

// TestReleaseQualificationEnded (IR440): HQ approves a technician's certificate whose window no longer covers an
// assigned visit on a unit that requires the code; the not-started assignment is released like an ended access
// (IR439). A certificate that still covers the visit releases nothing.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestReleaseQualificationEnded(t *testing.T) {
	srv := server(t)
	tech := seed.ID("tech-external-a")

	var from, until time.Time
	ownerScan(
		t,
		`SELECT valid_from, valid_until FROM identity.qualification_grants WHERE membership_id = $1 AND code = 'demo_indoor'`,
		[]any{tech},
		&from,
		&until,
	)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.qualification_grants SET valid_from = $2, valid_until = $3 WHERE membership_id = $1 AND code =
				'demo_indoor'`,
			tech,
			from,
			until,
		)
		owner(
			t,
			`UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM
				maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND
			b.scheduled && a.scheduled)`,
			seed.ID("assignment-contractor-a"),
		)
	})

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	code, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)
	if code != 200 {
		t.Fatalf("create: %d %v", code, resp)
	}

	job := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"terms-v1"}`,
		1,
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
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"terms-v1"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	pdf := blobJSON("cert.pdf", "application/pdf", []byte("%PDF-1.4 demo"), 13)
	approve := func(expires int) {
		t.Helper()

		code, res := write(
			srv,
			contrA(),
			"certificates.submit",
			`{"membershipId":"`+tech.String()+`","code":"demo_indoor","name":"Indoor AC","number":"N-2","issuedAt":"`+stamp(
				-240,
			)+
				`","expiresAt":"`+stamp(
				expires,
			)+`","file":`+pdf+`}`,
			0,
		)
		if code != 200 {
			t.Fatalf("submit: %d %v", code, res)
		}

		if code, reply := write(
			srv,
			hq(),
			"certificates.verify",
			`{"certificateId":"`+asStr(t, data(res)["id"])+`","decision":"approve"}`,
			1,
		); code != 200 {
			t.Fatalf("approve: %d %v", code, reply)
		}
	}
	state := func() (string, *string, *string) {
		var (
			status     string
			assignment *string
			reason     *string
		)

		ownerScan(
			t,
			`SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id
				= j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
			[]any{job},
			&status,
			&assignment,
			&reason,
		)

		return status, assignment, reason
	}
	before := countNotes(t, job, "contractor-a", "job_update")

	approve(100) // still covers the visit

	if status, assignment, _ := state(); status != statusAssigned || assignment == nil {
		t.Fatalf("a covering certificate released the job: %s %v", status, assignment)
	}

	approve(50) // ends during the visit

	if status, assignment, reason := state(); status != statusAccepted || assignment != nil ||
		reason == nil ||
		*reason != "qualification_ended" {
		t.Errorf(
			"after the release: status %s, assignment %v, reason %v",
			status,
			assignment,
			reason,
		)
	}

	if n := countNotes(t, job, "contractor-a", "job_update"); n != before+1 {
		t.Errorf("contractor notices: %d, want %d", n, before+1)
	}

	var events int
	ownerScan(
		t,
		`SELECT count(*) FROM maintenance.job_events WHERE job_id = $1 AND action = 'job.qualification_released'`,
		[]any{job},
		&events,
	)

	if events != 1 {
		t.Errorf("job.qualification_released events: %d", events)
	}
}

// TestPastSlotsRefused (IR456): a preferred time that is over can neither be assigned nor offered.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPastSlotsRefused(t *testing.T) {
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
	// the client's preferred times have all passed
	owner(
		t,
		`UPDATE maintenance.jobs SET preferred_slots = jsonb_build_array(jsonb_build_object('startAt', $2::timestamptz,
			'endAt', $3::timestamptz)),
		requested_slot = tstzrange($2, $3) WHERE id = $1`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			-3,
		)+`","endAt":"`+stamp(
			-1,
		)+`"}`,
		1,
	); code != 422 ||
		fields(res) != "endAt=error.past" {
		t.Errorf("assign a time that is over: %d %s", code, fields(res))
	}
	// an offer must be answered by the visit's start (IR457), so a visit that has begun cannot be offered either
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":{"startAt":"`+stamp(
			-3,
		)+`","endAt":"`+stamp(
			-1,
		)+`"}`+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			-4,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 422 ||
		fields(res) != "offerExpiresAt=error.afterVisit" {
		t.Errorf("offer a visit that is over: %d %s", code, fields(res))
	}
	// IR457: a future visit whose offer could still be answered after it begins
	_, resp = write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)

	later := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+later+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			50,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 422 ||
		fields(res) != "offerExpiresAt=error.afterVisit" {
		t.Errorf("offer answered after the visit began: %d %s", code, fields(res))
	}
}

// TestExtensionRefreezesTechnicianHistory (IR459): a technician whose window ended was frozen into their history; an
// extension for the same technician removes that snapshot, nothing is frozen while the new window is open, and its end
// freezes the history again.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestExtensionRefreezesTechnicianHistory(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	tech := seed.ID("tech-internal-a")
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
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
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
			25,
		)+`","endAt":"`+stamp(
			27,
		)+`"}`,
		1,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}
	// the work started and its window ended an hour ago
	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3) WHERE job_id = $1
			AND status = 'active'`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.jobs SET status = 'in_progress', scheduled_slot = tstzrange($2, $3) WHERE id = $1`,
		job,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	asOf := func() *time.Time {
		t.Helper()

		var asOf *time.Time
		ownerScan(
			t,
			`SELECT (SELECT as_of FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'membership' AND
				owner_id = $2)`,
			[]any{job, tech},
			&asOf,
		)

		return asOf
	}

	_, err := schedTick(ctx, srv, clock())
	if err != nil {
		t.Fatal(err)
	}

	if at := asOf(); at == nil || !at.Equal(clock().Add(-time.Hour)) {
		t.Fatalf("frozen at the first end: %v", at)
	}

	var version int
	ownerScan(t, `SELECT version FROM maintenance.jobs WHERE id = $1`, []any{job}, &version)

	if code, res := write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+stamp(
			-3,
		)+`","endAt":"`+stamp(
			2,
		)+`","reason":"more time on site"}`,
		version,
	); code != 200 {
		t.Fatalf("extension: %d %v", code, res)
	}

	_, err = schedTick(ctx, srv, clock().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	if at := asOf(); at != nil {
		t.Errorf("frozen while the extension is open: %v", at)
	}

	_, err = schedTick(ctx, srv, clock().Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if at := asOf(); at == nil || !at.Equal(clock().Add(2*time.Hour)) {
		t.Errorf("frozen again at the extension's end: %v", at)
	}
}
