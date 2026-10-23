package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestOfferAcceptAssign(t *testing.T) {
	s := server(t)
	// earlier runs leave active assignments for these technicians in the same slots
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active' AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, clock.Add(2*time.Hour))
	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, seed.ID("org-contractor-b"))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	s2 := slotJSON(49, 2)
	offerBody := func(extra map[string]string) string {
		f := map[string]string{"jobId": `"` + job + `"`, "contractorOrgId": `"` + seed.ID("org-contractor-a").String() + `"`, "visitSlot": s2,
			"offerExpiresAt": `"` + ts(12) + `"`, "accessValidFrom": `"` + ts(0) + `"`, "accessValidUntil": `"` + ts(100) + `"`, "termsVersion": `"terms-v1"`}
		for k, v := range extra {
			f[k] = v
		}
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
	for name, tc := range map[string]struct {
		extra map[string]string
		code  int
	}{
		"expiry in the past":    {map[string]string{"offerExpiresAt": `"` + ts(-1) + `"`}, 422},
		"expiry after access":   {map[string]string{"offerExpiresAt": `"` + ts(101) + `"`}, 422},
		"access does not cover": {map[string]string{"accessValidUntil": `"` + ts(50) + `"`}, 422},
		"slot not agreed":       {map[string]string{"visitSlot": slotJSON(30, 2)}, 422},
		"blank terms":           {map[string]string{"termsVersion": `""`}, 422},
		"customer organization": {map[string]string{"contractorOrgId": `"` + seed.ID("org-customer-a").String() + `"`}, 404},
		"unknown organization":  {map[string]string{"contractorOrgId": `"` + uuid.NewString() + `"`}, 404},
	} {
		code, m := write(s, &hq, "jobs.offer", offerBody(tc.extra), 1)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("offer %s: %d want %d", name, code, tc.code)
		}
	}
	owner(t, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email, delegation)
		VALUES ($1,$2,'B','suspended','R-1','b@example.com',tstzrange($3,$4))`, seed.ID("tenant-a"), seed.ID("org-contractor-b"), clock.Add(-time.Hour), clock.Add(1000*time.Hour))
	if code, _ := write(s, &hq, "jobs.offer", offerBody(map[string]string{"contractorOrgId": `"` + seed.ID("org-contractor-b").String() + `"`}), 1); code != 409 {
		t.Error("suspended contractor")
	}
	if code, _ := write(s, &customerA, "jobs.offer", offerBody(nil), 1); code != 403 {
		t.Error("client offer")
	}
	code, m := write(s, &hq, "jobs.offer", offerBody(nil), 1)
	if code != 200 || data(m)["status"] != "offered" || data(m)["contractorOrgId"] != seed.ID("org-contractor-a").String() {
		t.Fatalf("offer: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "jobs.offer", offerBody(nil), 2); code != 409 {
		t.Error("offer twice")
	}
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)

	acc := func(a *actor, terms string, v int) (int, map[string]any) {
		return write(s, a, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"`+terms+`"}`, v)
	}
	if code, _ := acc(&contrB, "terms-v1", 2); code != 404 {
		t.Error("other contractor")
	}
	if code, _ := acc(&contrA, "terms-v0", 2); code != 409 {
		t.Error("terms changed")
	}
	if code, _ := acc(&contrA, "terms-v1", 1); code != 409 {
		t.Error("stale job version")
	}
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`"}`, 2); code != 422 || fields(m) != "termsVersion=error.required" {
		t.Errorf("accept without terms: %d %s", code, fields(m))
	}
	code, m = acc(&contrA, "terms-v1", 2)
	if code != 200 || data(m)["decision"] != "accept" || data(m)["jobVersion"].(float64) != 3 || len(data(m)) != 4 {
		t.Fatalf("accept: %d %v", code, m)
	}
	if code, _ := acc(&contrA, "terms-v1", 3); code != 404 {
		t.Error("decided offer")
	}
	// contractor assigns its own technician at the visit slot
	asg := func(a *actor, j, tech, slot string, v int, reason string) (int, map[string]any) {
		start, end := slot[12:32], slot[43:63]
		r := ""
		if reason != "" {
			r = `,"reason":"` + reason + `"`
		}
		return write(s, a, "jobs.assign", `{"jobId":"`+j+`","technicianMembershipId":"`+seed.ID(tech).String()+`","startAt":"`+start+`","endAt":"`+end+`"`+r+`}`, v)
	}
	// the seed assignment of tech-external-a (09-14..09-20) would overlap: revoke it for this test
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE id = $1`, seed.ID("assignment-contractor-a"))
	t.Cleanup(func() { // restore unless a test assignment now holds the window
		owner(t, `UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND b.scheduled && a.scheduled)`, seed.ID("assignment-contractor-a"))
	})
	if code, _ := asg(&contrA, job, "tech-internal-a", s2, 3, ""); code != 403 {
		t.Error("contractor assigns another organization's technician")
	}
	if code, m := asg(&contrA, job, "tech-external-a", slotJSON(25, 2), 3, ""); code != 422 || fields(m) != "startAt=errors.slot_not_agreed" {
		t.Errorf("contractor slot must be the visit slot: %d %s", code, fields(m))
	}
	if code, _ := asg(&contrB, job, "tech-external-b", s2, 3, ""); code != 404 {
		t.Error("other contractor assign")
	}
	code, m = asg(&contrA, job, "tech-external-a", s2, 3, "")
	if code != 200 || data(m)["status"] != "assigned" || data(m)["assignmentId"] == nil || data(m)["scheduledSlot"] == nil {
		t.Fatalf("contractor assign: %d %v", code, m)
	}
	if _, extra := data(m)["assignment"]; extra { // the write answers MaintenanceJob, the detail is jobs.get's (IR392)
		t.Errorf("jobs.assign answers the detail: %v", data(m))
	}
	if _, g := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); data(g)["assignment"].(map[string]any)["acknowledgement"] != "pending" {
		t.Errorf("the assignment waits for the technician: %v", data(g)["assignment"])
	}
	if code, _ := asg(&hq, job, "tech-internal-a", s2, 4, ""); code != 403 {
		t.Error("HQ assigns internal jobs only")
	}

	// internal job: HQ assigns
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	k := data(m)["id"].(string)
	s1 := slotJSON(25, 2)
	if code, _ := asg(&hq, k, "tech-external-a", s1, 1, ""); code != 403 {
		t.Error("HQ assigns external technician")
	}
	if code, m := asg(&hq, k, "tech-internal-a", slotJSON(26, 2), 1, ""); code != 422 || fields(m) != "startAt=errors.slot_not_agreed" {
		t.Errorf("slot not agreed: %d %s", code, fields(m))
	}
	if code, _ := asg(&hq, k, "customer-a", s1, 1, ""); code != 404 {
		t.Error("not a technician")
	}
	if code, _ := asg(&hq, k, "tech-internal-a", s1, 1, ""); code != 200 {
		t.Fatal("hq assign")
	}
	if code, m := asg(&hq, k, "tech-internal-a", slotJSON(49, 2), 2, ""); code != 422 || fields(m) != "startAt=errors.slot_not_agreed" {
		t.Errorf("reassignment keeps the agreed slot: %d %s", code, fields(m))
	}
	if code, m := asg(&hq, k, "tech-internal-a", s1, 2, "swap"); code != 200 || data(m)["status"] != "assigned" {
		t.Fatalf("reassign: %d", code)
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, k)
	ext := `{"startAt":"` + ts(25) + `","endAt":"` + ts(28) + `"}`
	if code, m := asg(&hq, k, "tech-internal-a", ext, 3, ""); code != 422 || fields(m) != "reason=error.required" {
		t.Errorf("extension needs a reason: %d %s", code, fields(m))
	}
	if code, m := asg(&hq, k, "tech-internal-a", slotJSON(24, 4), 3, "x"); code != 422 || fields(m) != "endAt=errors.extension_only" {
		t.Errorf("extension keeps the start: %d %s", code, fields(m))
	}
	if code, m := asg(&hq, k, "tech-internal-a", ext, 3, "more work"); code != 200 || data(m)["status"] != "in_progress" {
		t.Fatalf("extend: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, k)
	if code, _ := asg(&hq, k, "tech-internal-a", ext, 4, "x"); code != 409 {
		t.Error("assign completed job")
	}
	// overlap with the active assignment of the same technician
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	l := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'requested' WHERE id = $1`, k)
	owner(t, `UPDATE maintenance.assignments SET status = 'active' WHERE job_id = $1 AND upper(scheduled) = $2`, k, clock.Add(28*time.Hour))
	if code, _ := asg(&hq, l, "tech-internal-a", s1, 1, ""); code != 409 {
		t.Error("overlapping assignment")
	}
	// out of scope and qualification
	other := newUnit(t, s, "Scope AC")
	_, m = write(s, &hq, "jobs.create", jobBody(other, map[string]string{"alternativeSlots": "[]"}), 0)
	if code, _ := asg(&hq, data(m)["id"].(string), "tech-internal-a", s1, 1, ""); code != 403 {
		t.Error("technician out of scope")
	}
	late := clock.Add(20 * 24 * time.Hour)
	lateSlot := `{"startAt":"` + late.Format(time.RFC3339) + `","endAt":"` + late.Add(2*time.Hour).Format(time.RFC3339) + `"}`
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]", "requestedStart": `"` + late.Format(time.RFC3339) + `"`,
		"requestedEnd": `"` + late.Add(2*time.Hour).Format(time.RFC3339) + `"`}), 0)
	if code, _ := asg(&hq, data(m)["id"].(string), "tech-internal-a", lateSlot, 1, ""); code != 403 {
		t.Error("qualification expires before the slot")
	}

	// decline and expiry
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job = data(m)["id"].(string)
	write(s, &hq, "jobs.offer", offerBody(nil), 1)
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	if code, m := write(s, &contrA, "jobs.decline", `{"jobId":"`+job+`","offerId":"`+offer+`","reason":"  "}`, 2); code != 422 || fields(m) != "reason=error.length" {
		t.Errorf("blank decline reason: %d %s", code, fields(m))
	}
	if code, m := write(s, &contrA, "jobs.decline", `{"jobId":"`+job+`","offerId":"`+offer+`","reason":"no capacity"}`, 2); code != 200 || data(m)["decision"] != "decline" {
		t.Fatalf("decline: %d %v", code, m)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["status"] != "requested" || data(m)["contractorOrgId"] != nil {
		t.Fatal("decline returns the job to requested")
	}
	// the company's own decision stays readable as offer.declined (history snapshots keep accept / decline records)
	var action string
	ownerScan(t, `SELECT action FROM maintenance.job_events WHERE job_id = $1 AND action LIKE 'offer.%' ORDER BY occurred_at DESC, id DESC LIMIT 1`, []any{job}, &action)
	if action != "offer.declined" {
		t.Fatalf("decline event action %q", action)
	}
	write(s, &hq, "jobs.offer", offerBody(nil), 3)
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE id = $1`, offer, clock)
	if code, _ := acc(&contrA, "terms-v1", 4); code != 409 {
		t.Error("expired offer")
	}
}

func TestAgreedSlotsAndInputs(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND lower(scheduled) > $2`,
		seed.ID("tech-internal-a"), clock.Add(2*time.Hour))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	// an accepted SlotProposal makes its slot agreed
	_, m := write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	job := data(m)["id"].(string)
	owner(t, `INSERT INTO maintenance.slot_proposals (tenant_id, job_id, source, slot, hold, message, reply_by, status)
		VALUES ($1,$2,'hq',tstzrange($3,$4),'{"kind":"internal"}','m',$4,'accepted')`, seed.ID("tenant-a"), job, clock.Add(55*time.Hour), clock.Add(57*time.Hour))
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(55)+`","endAt":"`+ts(57)+`"}`, 1); code != 200 {
		t.Fatalf("accepted proposal slot: %d %v", code, m)
	}
	// a plan occurrence is agreed for periodic jobs
	plan, pj := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO maintenance.plans (id, tenant_id, unit_id, interval_months, anchor_day, next_due_at) VALUES ($1,$2,$3,3,8,$4)`, plan, seed.ID("tenant-a"), unit, clock)
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,$5,$6,'periodic','requested','periodic_plan',tstzrange($6,$7),$7)`, pj, seed.ID("tenant-a"), unit, seed.ID("org-customer-a"), plan,
		clock.Add(60*time.Hour), clock.Add(62*time.Hour))
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+pj+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(60)+`","endAt":"`+ts(62)+`"}`, 1); code != 200 {
		t.Fatalf("plan occurrence slot: %d %v", code, m)
	}
	// input shape errors: reversed times, and a field of the other decision (not an input field: malformed)
	for op, c := range map[string]struct {
		body   string
		fields []string
	}{
		"jobs.offer": {`{"jobId":"` + job + `","contractorOrgId":"` + seed.ID("org-contractor-a").String() + `","visitSlot":{"startAt":"` + ts(3) + `","endAt":"` + ts(2) +
			`"},"offerExpiresAt":"` + ts(5) + `","accessValidFrom":"` + ts(6) + `","accessValidUntil":"` + ts(4) + `","termsVersion":"t"}`, []string{"visitSlot", "accessValidUntil"}},
		"jobs.accept":  {`{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","reason":"x"}`, []string{"_"}},
		"jobs.decline": {`{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t"}`, []string{"_"}},
		"jobs.assign":  {`{"jobId":"` + job + `","technicianMembershipId":"` + seed.ID("tech-internal-a").String() + `","startAt":"` + ts(3) + `","endAt":"` + ts(2) + `"}`, []string{"endAt"}},
	} {
		a := &hq
		if op == "jobs.accept" || op == "jobs.decline" {
			a = &contrA
		}
		code, m := write(s, a, op, c.body, 1)
		fe, _ := m["fieldErrors"].(map[string]any)
		for _, f := range c.fields {
			if code != 422 || fe[f] == nil {
				t.Errorf("%s shape: %d %v, want %s", op, code, fe, f)
			}
		}
	}
	// both terms and a reason: the reason is not an input of accept (it was refused for the missing jobId before)
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`","termsVersion":"t","reason":"x"}`, 1); code != 422 || fields(m) != "_=error.malformedInput" {
		t.Errorf("both terms and reason: %d %s", code, fields(m))
	}
	// each decision takes only its own fields (service-contracts: accept termsVersion, decline reason)
	for op, b := range map[string]string{
		"jobs.accept":  `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t","reason":"x"}`,
		"jobs.decline": `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t","reason":"no capacity"}`,
	} {
		if code, m := write(s, &contrA, op, b, 1); code != 422 || m["fieldErrors"].(map[string]any)["_"] != "error.malformedInput" {
			t.Errorf("%s with the other decision's field: %d %v", op, code, m)
		}
	}
	if code, m := write(s, &contrA, "jobs.decline", `{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`"}`, 1); code != 422 || m["fieldErrors"].(map[string]any)["reason"] != "error.length" {
		t.Errorf("decline without a reason: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+uuid.NewString()+`"}`, 1); code != 422 || m["fieldErrors"].(map[string]any)["termsVersion"] != "error.required" {
		t.Errorf("accept without terms: %d %v", code, m)
	}
	long := make([]byte, 1001)
	for i := range long {
		long[i] = 'r'
	}
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(55)+`","endAt":"`+ts(57)+`","reason":"`+string(long)+`"}`, 2); code != 422 || fields(m) != "reason=error.length" {
		t.Errorf("reason too long: %d %s", code, fields(m))
	}
	// contractor outside the access window
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+j2+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{j2}, &offer)
	write(s, &contrA, "jobs.accept", `{"jobId":"`+j2+`","offerId":"`+offer+`","termsVersion":"t"}`, 2)
	if code, _ := write(s, &contrA, "jobs.assign", `{"jobId":"`+j2+`","technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","startAt":"`+ts(49)+`","endAt":"`+ts(51)+`"}`, 3); code != 403 {
		t.Error("before the access window")
	}
}

// TestAssignWithinAccess (IR433): a technician is assigned only to a slot that ends before their access does; a slot
// past the access end is FORBIDDEN errors.technician_access_ends, and the same slot is fine once the access lasts.
func TestAssignWithinAccess(t *testing.T) {
	s := server(t)
	tech := seed.ID("tech-internal-a")
	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	t.Cleanup(func() { owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until) })
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, tech)
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(26*time.Hour))
	code, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	assign := func(from, to int) (int, map[string]any) {
		return write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(from)+`","endAt":"`+ts(to)+`"}`, 1)
	}
	if code, m := assign(25, 27); code != 403 || m["messageKey"] != "errors.technician_access_ends" {
		t.Errorf("a slot past the access end: %d %v", code, m)
	}
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(28*time.Hour)) // the agreed slot is fixed: extend the access
	if code, m := assign(25, 27); code != 200 {
		t.Errorf("the same slot within the access: %d %v", code, m)
	}
}

// TestReleaseEndedAccess (OPEN-BE-09, IR439): HQ shortens a booked technician's access to end before the visit; the
// maintenance scheduler releases the assignment of the job that has not started, the job goes back to be assigned
// (requested for HQ's own delivery), and HQ's assigners get a job_update notice.
func TestReleaseEndedAccess(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tech := seed.ID("tech-internal-a")
	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until)
		owner(t, `UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`, tech, until)
	})
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, tech)
	code, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(25)+`","endAt":"`+ts(27)+`"}`, 1); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	before := countNotes(t, job, "hq-operator", "job_update")
	// HQ ends the access tomorrow, before the visit ends (Access & roles); maintenance holds its copy of the membership
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(24*time.Hour))
	owner(t, `UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(24*time.Hour))
	if _, err := schedTick(ctx, s, clock.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	var assignment *string
	ownerScan(t, `SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id = j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
		[]any{job}, &status, &assignment, &reason)
	if status != "requested" || assignment != nil || reason != "access_ended" {
		t.Errorf("after the release: status %s, assignment %v, reason %s", status, assignment, reason)
	}
	if n := countNotes(t, job, "hq-operator", "job_update"); n != before+1 {
		t.Errorf("HQ notices: %d, want %d", n, before+1)
	}
	// nothing more to release on the next tick
	if _, err := schedTick(ctx, s, clock.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := countNotes(t, job, "hq-operator", "job_update"); n != before+1 {
		t.Errorf("released twice: %d notices", n)
	}
}

// TestReleaseEndedAccessContractor (IR439): the contractor's job whose technician's access now ends before the visit
// goes back to accepted with its offer, and the contractor's assigners get the job_update notice.
func TestReleaseEndedAccessContractor(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tech := seed.ID("tech-external-a")
	var until *time.Time
	ownerScan(t, `SELECT valid_until FROM identity.memberships WHERE id = $1`, []any{tech}, &until)
	// the seed assignment of tech-external-a (09-14..09-20) would overlap the visit: revoke it for this test
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, tech)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, until)
		owner(t, `UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`, tech, until)
		owner(t, `UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND b.scheduled && a.scheduled)`, seed.ID("assignment-contractor-a"))
	})
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	code, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"terms-v1"}`, 1); code != 200 {
		t.Fatalf("offer: %d %v", code, m)
	}
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"terms-v1"}`, 2); code != 200 {
		t.Fatalf("accept: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(49)+`","endAt":"`+ts(51)+`"}`, 3); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	before, hqBefore := countNotes(t, job, "contractor-a", "job_update"), countNotes(t, job, "hq-operator", "job_update")
	// HQ ends the access during the visit (Access & roles)
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(50*time.Hour))
	owner(t, `UPDATE maintenance.ref_memberships SET valid_until = $2 WHERE id = $1`, tech, clock.Add(50*time.Hour))
	if _, err := schedTick(ctx, s, clock.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var status, reason string
	var assignment *string
	ownerScan(t, `SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id = j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
		[]any{job}, &status, &assignment, &reason)
	if status != "accepted" || assignment != nil || reason != "access_ended" {
		t.Errorf("after the release: status %s, assignment %v, reason %s", status, assignment, reason)
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
func TestReleaseQualificationEnded(t *testing.T) {
	s := server(t)
	tech := seed.ID("tech-external-a")
	var from, until time.Time
	ownerScan(t, `SELECT valid_from, valid_until FROM identity.qualification_grants WHERE membership_id = $1 AND code = 'demo_indoor'`, []any{tech}, &from, &until)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, tech)
	t.Cleanup(func() {
		owner(t, `UPDATE identity.qualification_grants SET valid_from = $2, valid_until = $3 WHERE membership_id = $1 AND code = 'demo_indoor'`, tech, from, until)
		owner(t, `UPDATE maintenance.assignments a SET status = 'active' WHERE a.id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.assignments b
			WHERE b.id <> a.id AND b.status = 'active' AND b.technician_membership_id = a.technician_membership_id AND b.scheduled && a.scheduled)`, seed.ID("assignment-contractor-a"))
	})
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	code, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	if code != 200 {
		t.Fatalf("create: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"terms-v1"}`, 1); code != 200 {
		t.Fatalf("offer: %d %v", code, m)
	}
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	if code, m := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"terms-v1"}`, 2); code != 200 {
		t.Fatalf("accept: %d %v", code, m)
	}
	if code, m := write(s, &contrA, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(49)+`","endAt":"`+ts(51)+`"}`, 3); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	pdf := blobJSON("cert.pdf", "application/pdf", []byte("%PDF-1.4 demo"), 13)
	approve := func(expires int) {
		t.Helper()
		code, m := write(s, &contrA, "certificates.submit", `{"membershipId":"`+tech.String()+`","code":"demo_indoor","name":"Indoor AC","number":"N-2","issuedAt":"`+ts(-240)+
			`","expiresAt":"`+ts(expires)+`","file":`+pdf+`}`, 0)
		if code != 200 {
			t.Fatalf("submit: %d %v", code, m)
		}
		if code, m := write(s, &hq, "certificates.verify", `{"certificateId":"`+data(m)["id"].(string)+`","decision":"approve"}`, 1); code != 200 {
			t.Fatalf("approve: %d %v", code, m)
		}
	}
	state := func() (status string, assignment *string, reason *string) {
		ownerScan(t, `SELECT j.status, j.assignment_id::text, a.reason FROM maintenance.jobs j JOIN maintenance.assignments a ON a.job_id = j.id WHERE j.id = $1 ORDER BY a.created_at DESC LIMIT 1`,
			[]any{job}, &status, &assignment, &reason)
		return
	}
	before := countNotes(t, job, "contractor-a", "job_update")
	approve(100) // still covers the visit
	if status, assignment, _ := state(); status != "assigned" || assignment == nil {
		t.Fatalf("a covering certificate released the job: %s %v", status, assignment)
	}
	approve(50) // ends during the visit
	if status, assignment, reason := state(); status != "accepted" || assignment != nil || reason == nil || *reason != "qualification_ended" {
		t.Errorf("after the release: status %s, assignment %v, reason %v", status, assignment, reason)
	}
	if n := countNotes(t, job, "contractor-a", "job_update"); n != before+1 {
		t.Errorf("contractor notices: %d, want %d", n, before+1)
	}
	var events int
	ownerScan(t, `SELECT count(*) FROM maintenance.job_events WHERE job_id = $1 AND action = 'job.qualification_released'`, []any{job}, &events)
	if events != 1 {
		t.Errorf("job.qualification_released events: %d", events)
	}
}

// TestPastSlotsRefused (IR456): a preferred time that is over can neither be assigned nor offered.
func TestPastSlotsRefused(t *testing.T) {
	s := server(t)
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	// the client's preferred times have all passed
	owner(t, `UPDATE maintenance.jobs SET preferred_slots = jsonb_build_array(jsonb_build_object('startAt', $2::timestamptz, 'endAt', $3::timestamptz)),
		requested_slot = tstzrange($2, $3) WHERE id = $1`, job, clock.Add(-3*time.Hour), clock.Add(-time.Hour))
	if code, m := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(-3)+`","endAt":"`+ts(-1)+`"}`, 1); code != 422 || fields(m) != "endAt=error.past" {
		t.Errorf("assign a time that is over: %d %s", code, fields(m))
	}
	// an offer must be answered by the visit's start (IR457), so a visit that has begun cannot be offered either
	if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":{"startAt":"`+ts(-3)+`","endAt":"`+ts(-1)+`"}`+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(-4)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1); code != 422 || fields(m) != "offerExpiresAt=error.afterVisit" {
		t.Errorf("offer a visit that is over: %d %s", code, fields(m))
	}
	// IR457: a future visit whose offer could still be answered after it begins
	_, m = write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	later := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+later+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(50)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1); code != 422 || fields(m) != "offerExpiresAt=error.afterVisit" {
		t.Errorf("offer answered after the visit began: %d %s", code, fields(m))
	}
}
