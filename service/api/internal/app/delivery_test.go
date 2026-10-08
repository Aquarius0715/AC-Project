package app

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/seed"
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
		if code, _ := write(s, &hq, "jobs.offer", offerBody(tc.extra), 1); code != tc.code {
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
	if code, _ := write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`"}`, 2); code != 422 {
		t.Error("accept without terms")
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
	if code, _ := asg(&contrA, job, "tech-external-a", slotJSON(25, 2), 3, ""); code != 422 {
		t.Error("contractor slot must be the visit slot")
	}
	if code, _ := asg(&contrB, job, "tech-external-b", s2, 3, ""); code != 404 {
		t.Error("other contractor assign")
	}
	code, m = asg(&contrA, job, "tech-external-a", s2, 3, "")
	if code != 200 || data(m)["status"] != "assigned" || data(m)["assignment"].(map[string]any)["acknowledgement"] != "pending" || data(m)["scheduledSlot"] == nil {
		t.Fatalf("contractor assign: %d %v", code, m)
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
	if code, _ := asg(&hq, k, "tech-internal-a", slotJSON(26, 2), 1, ""); code != 422 {
		t.Error("slot not agreed")
	}
	if code, _ := asg(&hq, k, "customer-a", s1, 1, ""); code != 404 {
		t.Error("not a technician")
	}
	if code, _ := asg(&hq, k, "tech-internal-a", s1, 1, ""); code != 200 {
		t.Fatal("hq assign")
	}
	if code, _ := asg(&hq, k, "tech-internal-a", slotJSON(49, 2), 2, ""); code != 422 {
		t.Error("reassignment keeps the agreed slot")
	}
	if code, m := asg(&hq, k, "tech-internal-a", s1, 2, "swap"); code != 200 || data(m)["status"] != "assigned" {
		t.Fatalf("reassign: %d", code)
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, k)
	ext := `{"startAt":"` + ts(25) + `","endAt":"` + ts(28) + `"}`
	if code, _ := asg(&hq, k, "tech-internal-a", ext, 3, ""); code != 422 {
		t.Error("extension needs a reason")
	}
	if code, _ := asg(&hq, k, "tech-internal-a", slotJSON(24, 4), 3, "x"); code != 422 {
		t.Error("extension keeps the start")
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
	if code, _ := write(s, &contrA, "jobs.decline", `{"jobId":"`+job+`","offerId":"`+offer+`","reason":"  "}`, 2); code != 422 {
		t.Error("blank decline reason")
	}
	if code, m := write(s, &contrA, "jobs.decline", `{"jobId":"`+job+`","offerId":"`+offer+`","reason":"no capacity"}`, 2); code != 200 || data(m)["decision"] != "decline" {
		t.Fatalf("decline: %d %v", code, m)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["status"] != "requested" || data(m)["contractorOrgId"] != nil {
		t.Fatal("decline returns the job to requested")
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
	// input shape errors
	for op, b := range map[string]string{
		"jobs.offer":   `{"visitSlot":{"startAt":"` + ts(3) + `","endAt":"` + ts(2) + `"},"offerExpiresAt":"` + ts(5) + `","accessValidFrom":"` + ts(6) + `","accessValidUntil":"` + ts(4) + `","termsVersion":"t"}`,
		"jobs.accept":  `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","reason":"x"}`,
		"jobs.decline": `{"jobId":"` + job + `","offerId":"` + uuid.NewString() + `","termsVersion":"t"}`,
		"jobs.assign":  `{"startAt":"` + ts(3) + `","endAt":"` + ts(2) + `","reason":"` + string(make([]byte, 0)) + `"}`,
	} {
		a := &hq
		if op == "jobs.accept" || op == "jobs.decline" {
			a = &contrA
		}
		if code, _ := write(s, a, op, b, 1); code != 422 {
			t.Errorf("%s shape: %d", op, code)
		}
	}
	if code, _ := write(s, &contrA, "jobs.accept", `{"offerId":"`+uuid.NewString()+`","termsVersion":"t","reason":"x"}`, 1); code != 422 {
		t.Error("both terms and reason")
	}
	long := make([]byte, 1001)
	for i := range long {
		long[i] = 'r'
	}
	if code, _ := write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(55)+`","endAt":"`+ts(57)+`","reason":"`+string(long)+`"}`, 2); code != 422 {
		t.Error("reason too long")
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
