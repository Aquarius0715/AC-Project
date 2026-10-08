package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestEligibleCapacityUnavailability(t *testing.T) {
	s := server(t)
	tech := seed.ID("tech-internal-a")
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, tech)
	clean := func() {
		owner(t, `DELETE FROM maintenance.unavailability WHERE membership_id = $1 OR organization_id = ANY($2)`, tech, []uuid.UUID{seed.ID("org-operator-a"), seed.ID("org-contractor-a")})
	}
	clean()
	t.Cleanup(clean)
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	job := data(m)["id"].(string)
	elig := func(a *actor, start, end int) (int, map[string]any) {
		return post(s, a, "members.eligible", `{"jobId":"`+job+`","startAt":"`+ts(start)+`","endAt":"`+ts(end)+`","query":{}}`)
	}
	has := func(m map[string]any, id uuid.UUID) bool {
		for _, it := range items(m) {
			if it["id"] == id.String() {
				return true
			}
		}
		return false
	}
	if code, m := elig(&hq, 25, 27); code != 200 || !has(m, tech) || has(m, seed.ID("tech-external-a")) {
		t.Fatalf("eligible: %d %v", code, m)
	}
	if _, m := elig(&hq, 24*20, 24*20+2); has(m, tech) {
		t.Error("qualification expired for that slot")
	}
	if code, _ := elig(&hq, 27, 25); code != 422 {
		t.Error("reversed slot")
	}
	if code, _ := elig(&contrA, 25, 27); code != 404 {
		t.Error("contractor on an internal job")
	}
	// capacity: 2026-09-14 is a Monday; 09:00–17:00 KL = 01:00–09:00 UTC
	write(s, &hq, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(25)+`","endAt":"`+ts(27)+`"}`, 1)
	owner(t, `UPDATE maintenance.assignments SET scheduled = tstzrange($2, $3) WHERE job_id = $1 AND status = 'active'`, job, clock.Add(-2*time.Hour), clock.Add(4*time.Hour))
	capOf := func(a *actor, date string) map[string]any {
		_, m := post(s, a, "members.capacity", `{"date":"`+date+`","query":{"limit":100}}`)
		for _, it := range items(m) {
			if it["membershipId"] == tech.String() {
				return it
			}
		}
		return nil
	}
	c := capOf(&hq, "2026-09-14")
	if c == nil || c["availableMinutes"].(float64) != 480 || c["assignedMinutes"].(float64) != 240 || c["utilization"].(float64) != 50 {
		t.Fatalf("capacity: %v", c)
	}
	if c := capOf(&hq, "2026-09-13"); c["availableMinutes"].(float64) != 0 || c["utilization"] != nil {
		t.Fatalf("sunday: %v", c)
	}
	if code, _ := post(s, &hq, "members.capacity", `{"date":"14-09-2026","query":{}}`); code != 422 {
		t.Error("bad date")
	}
	if c := capOf(&contrA, "2026-09-14"); c != nil {
		t.Error("contractor sees internal technicians")
	}
	// unavailability: reports conflicts, removes availability and eligibility
	for name, b := range map[string]string{
		"to before from": `{"membershipId":"` + tech.String() + `","from":"2026-09-15","to":"2026-09-14","type":"sick"}`,
		"32 days":        `{"membershipId":"` + tech.String() + `","from":"2026-09-01","to":"2026-10-03","type":"sick"}`,
		"bad type":       `{"membershipId":"` + tech.String() + `","from":"2026-09-14","to":"2026-09-14","type":"holiday"}`,
	} {
		if code, _ := write(s, &hq, "members.setUnavailability", b, 0); code != 422 {
			t.Errorf("unavailability %s: %d", name, code)
		}
	}
	if code, _ := write(s, &contrA, "members.setUnavailability", `{"membershipId":"`+tech.String()+`","from":"2026-09-14","to":"2026-09-14","type":"sick"}`, 0); code != 404 {
		t.Error("contractor sets another organization's technician")
	}
	code, m := write(s, &hq, "members.setUnavailability", `{"membershipId":"`+tech.String()+`","from":"2026-09-14","to":"2026-09-15","type":"training","note":" course "}`, 0)
	if code != 200 || len(data(m)["conflictingAssignmentIds"].([]any)) != 1 || data(m)["note"] != "course" {
		t.Fatalf("unavailability: %d %v", code, m)
	}
	if c := capOf(&hq, "2026-09-14"); c["unavailability"] != "training" || c["availableMinutes"].(float64) != 0 {
		t.Fatalf("capacity while unavailable: %v", c)
	}
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	job = data(m)["id"].(string)
	if _, m := elig(&hq, 25, 27); has(m, tech) {
		t.Error("unavailable technician is not eligible")
	}
	if code, m := write(s, &contrA, "members.setUnavailability", `{"membershipId":null,"from":"2026-09-20","to":"2026-09-20","type":"public_holiday"}`, 0); code != 200 ||
		data(m)["organizationId"] != seed.ID("org-contractor-a").String() {
		t.Fatalf("organization-wide: %d %v", code, m)
	}
}
