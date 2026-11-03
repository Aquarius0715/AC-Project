package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestEligibleCapacityUnavailability(t *testing.T) {
	s := server(t)
	tech := seed.ID("tech-internal-a")
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		tech,
	)

	clean := func() {
		owner(
			t,
			`DELETE FROM maintenance.unavailability WHERE membership_id = $1 OR organization_id = ANY($2)`,
			tech,
			[]uuid.UUID{seed.ID("org-operator-a"), seed.ID("org-contractor-a")},
		)
	}
	clean()
	t.Cleanup(clean)

	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(
		s,
		&hq,
		"jobs.create",
		jobBody(unit, map[string]string{"alternativeSlots": "[]"}),
		0,
	)
	job := data(m)["id"].(string)
	elig := func(a *actor, start, end int) (int, map[string]any) {
		return post(
			s,
			a,
			"members.eligible",
			`{"jobId":"`+job+`","startAt":"`+ts(start)+`","endAt":"`+ts(end)+`","query":{}}`,
		)
	}

	has := func(m map[string]any, id uuid.UUID) bool {
		for _, it := range items(m) {
			if it["id"] == id.String() {
				return true
			}
		}

		return false
	}
	if code, m := elig(
		&hq,
		25,
		27,
	); code != 200 || !has(m, tech) ||
		has(m, seed.ID("tech-external-a")) {
		t.Fatalf("eligible: %d %v", code, m)
	}

	if _, m := elig(&hq, 24*20, 24*20+2); has(m, tech) {
		t.Error("qualification expired for that slot")
	}

	if code, m := elig(&hq, 27, 25); code != 422 || fields(m) != "endAt=error.range" {
		t.Errorf("reversed slot: %d %s", code, fields(m))
	}

	if code, _ := elig(&contrA, 25, 27); code != 404 {
		t.Error("contractor on an internal job")
	}
	// capacity: 2026-09-14 is a Monday; 09:00–17:00 KL = 01:00–09:00 UTC
	write(
		s,
		&hq,
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+tech.String()+`","startAt":"`+ts(
			25,
		)+`","endAt":"`+ts(
			27,
		)+`"}`,
		1,
	)
	owner(
		t,
		`UPDATE maintenance.assignments SET scheduled = tstzrange($2, $3) WHERE job_id = $1 AND status = 'active'`,
		job,
		clock.Add(-2*time.Hour),
		clock.Add(4*time.Hour),
	)

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
	if c == nil || c["availableMinutes"].(float64) != 480 ||
		c["assignedMinutes"].(float64) != 240 ||
		c["utilization"].(float64) != 50 {
		t.Fatalf("capacity: %v", c)
	}

	if c := capOf(
		&hq,
		"2026-09-13",
	); c["availableMinutes"].(float64) != 0 ||
		c["utilization"] != nil {
		t.Fatalf("sunday: %v", c)
	}

	if code, m := post(
		s,
		&hq,
		"members.capacity",
		`{"date":"14-09-2026","query":{}}`,
	); code != 422 ||
		fields(m) != "date=error.invalid" {
		t.Errorf("bad date: %d %s", code, fields(m))
	}

	if c := capOf(&contrA, "2026-09-14"); c != nil {
		t.Error("contractor sees internal technicians")
	}
	// unavailability: reports conflicts, removes availability and eligibility
	want := map[string]string{ // the field each case is refused for
		"32 days":        "to=error.range",
		"bad type":       "type=error.invalid",
		"to before from": "to=error.range",
	}

	for name, b := range map[string]string{
		"to before from": `{"membershipId":"` + tech.String() + `","from":"2026-09-15","to":"2026-09-14","type":"sick"}`,
		"32 days":        `{"membershipId":"` + tech.String() + `","from":"2026-09-01","to":"2026-10-03","type":"sick"}`,
		"bad type":       `{"membershipId":"` + tech.String() + `","from":"2026-09-14","to":"2026-09-14","type":"holiday"}`,
	} {
		code, m := write(s, &hq, "members.setUnavailability", b, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		s,
		&contrA,
		"members.setUnavailability",
		`{"membershipId":"`+tech.String()+`","from":"2026-09-14","to":"2026-09-14","type":"sick"}`,
		0,
	); code != 404 {
		t.Error("contractor sets another organization's technician")
	}

	code, m := write(
		s,
		&hq,
		"members.setUnavailability",
		`{"membershipId":"`+tech.String()+`","from":"2026-09-14","to":"2026-09-15","type":"training","note":" course "}`,
		0,
	)
	if code != 200 || len(data(m)["conflictingAssignmentIds"].([]any)) != 1 ||
		data(m)["note"] != "course" {
		t.Fatalf("unavailability: %d %v", code, m)
	}

	if c := capOf(
		&hq,
		"2026-09-14",
	); c["unavailability"] != "training" ||
		c["availableMinutes"].(float64) != 0 {
		t.Fatalf("capacity while unavailable: %v", c)
	}

	_, m = write(
		s,
		&hq,
		"jobs.create",
		jobBody(unit, map[string]string{"alternativeSlots": "[]"}),
		0,
	)

	job = data(m)["id"].(string)
	if _, m := elig(&hq, 25, 27); has(m, tech) {
		t.Error("unavailable technician is not eligible")
	}

	if code, m := write(
		s,
		&contrA,
		"members.setUnavailability",
		`{"membershipId":null,"from":"2026-09-20","to":"2026-09-20","type":"public_holiday","note":"   "}`,
		0,
	); code != 200 ||
		data(m)["organizationId"] != seed.ID("org-contractor-a").String() ||
		data(m)["note"] != nil {
		t.Fatalf("organization-wide: %d %v", code, m)
	}
}

// TestUnavailabilityListAndClear: members.unavailability lists the rows whose last Kuala Lumpur day is today or later,
// first day first, a contractor's own company only; members.clearUnavailability clears a row's days from a day on —
// from its first day the row goes, from a later day the earlier days stay (version + 1) — never a day before today,
// with the row's version, and only within the caller's company (IR612). The clock's day is Monday 2026-09-14.
func TestUnavailabilityListAndClear(t *testing.T) {
	s := server(t)
	tech, orgA := seed.ID("tech-external-a"), seed.ID("org-contractor-a")
	clean := func() { owner(t, `DELETE FROM maintenance.unavailability WHERE organization_id = $1`, orgA) }
	clean()
	t.Cleanup(clean)

	set := func(body string) string {
		t.Helper()

		code, m := write(s, &contrA, "members.setUnavailability", body, 0)
		if code != 200 || ver(m) != 1 {
			t.Fatalf("set %s: %d %v", body, code, m)
		}

		return data(m)["id"].(string)
	}
	leave := set(
		`{"membershipId":"` + tech.String() + `","from":"2026-09-16","to":"2026-09-18","type":"annual_leave"}`,
	)
	sick := set(
		`{"membershipId":"` + tech.String() + `","from":"2026-09-10","to":"2026-09-15","type":"sick"}`,
	)
	set(
		`{"membershipId":null,"from":"2026-09-08","to":"2026-09-09","type":"public_holiday"}`,
	) // over on the 9th: never listed
	holiday := set(
		`{"membershipId":null,"from":"2026-09-20","to":"2026-09-20","type":"public_holiday"}`,
	)
	ids := func(a *actor, query string) []string {
		t.Helper()

		code, m := post(s, a, "members.unavailability", query)
		if code != 200 {
			t.Fatalf("list %s: %d %v", query, code, m)
		}

		out := []string{}

		for _, it := range items(m) {
			if it["organizationId"] == orgA.String() {
				out = append(out, it["id"].(string))
			}
		}

		return out
	}
	same := func(name string, got []string, want ...string) {
		t.Helper()

		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
	same(
		"contractor A",
		ids(&contrA, `{"limit":100}`),
		sick,
		leave,
		holiday,
	) // the row that ended on the 9th is not listed
	same(
		"by first day, descending",
		ids(&contrA, `{"limit":100,"sort":{"field":"from","direction":"desc"}}`),
		holiday,
		leave,
		sick,
	)
	same("contractor B", ids(&contrB, `{"limit":100}`))
	same(
		"HQ for the company",
		ids(&hq, `{"limit":100,"filters":{"organizationId":"`+orgA.String()+`"}}`),
		sick,
		leave,
		holiday,
	)
	same(
		"the technician with the whole company's days",
		ids(&contrA, `{"limit":100,"filters":{"membershipId":"`+tech.String()+`"}}`),
		sick,
		leave,
		holiday,
	)
	same(
		"another technician",
		ids(
			&contrA,
			`{"limit":100,"filters":{"membershipId":"`+seed.ID("tech-external-b").String()+`"}}`,
		),
	)

	if code, m := post(
		s,
		&contrA,
		"members.unavailability",
		`{"filters":{"type":"sick"}}`,
	); code != 422 ||
		fields(m) != "type=error.notAllowed" {
		t.Errorf("a filter outside the catalog: %d %s", code, fields(m))
	}

	clear := func(a *actor, id, from string, version int) (int, map[string]any) {
		return write(
			s,
			a,
			"members.clearUnavailability",
			`{"id":"`+id+`","from":"`+from+`"}`,
			version,
		)
	}
	if code, _ := clear(&contrA, leave, "2026-09-17", 0); code != 422 {
		t.Errorf("no expected version: %d", code)
	}

	if code, _ := clear(&contrB, leave, "2026-09-17", 1); code != 404 {
		t.Errorf("another company: %d", code)
	}

	if code, m := clear(&contrA, leave, "2026-09-17", 2); code != 409 {
		t.Errorf("stale version: %d %v", code, m)
	}

	for name, from := range map[string]string{"before today": "2026-09-13", "after its last day": "2026-09-19"} {
		code, m := clear(&contrA, leave, from, 1)
		refusal(t, name, code, m, "from=error.range")
	}

	code, m := clear(&contrA, leave, "2026-09-17", 1)

	r, _ := data(m)["remaining"].(map[string]any)
	if code != 200 || r == nil || r["from"] != "2026-09-16" || r["to"] != "2026-09-16" ||
		r["version"].(float64) != 2 {
		t.Fatalf("a new last day: %d %v", code, m)
	}

	if code, _ := clear(&contrA, leave, "2026-09-16", 1); code != 409 {
		t.Errorf("the version before the change: %d", code)
	}

	if code, m := clear(
		&contrA,
		leave,
		"2026-09-16",
		2,
	); code != 200 ||
		data(m)["remaining"] != nil {
		t.Fatalf("from its first day: %d %v", code, m)
	}

	if code, _ := clear(&contrA, leave, "2026-09-16", 3); code != 404 {
		t.Errorf("a removed row: %d", code)
	}
	// a row that started before today keeps its past days: cleared from today, it ends yesterday and leaves the list
	code, m = clear(&contrA, sick, "2026-09-10", 1)
	refusal(t, "its past first day", code, m, "from=error.range")

	code, m = clear(&contrA, sick, "2026-09-14", 1)
	if r, _ := data(m)["remaining"].(map[string]any); code != 200 || r == nil ||
		r["from"] != "2026-09-10" ||
		r["to"] != "2026-09-13" {
		t.Fatalf("from today: %d %v", code, m)
	}

	same("after the clears", ids(&contrA, `{"limit":100}`), holiday)

	var past, gone int
	ownerScan(
		t,
		`SELECT count(*) FILTER (WHERE id = $1 AND days = daterange('2026-09-10', '2026-09-13', '[]')), count(*) FILTER (WHERE id = $2)
		FROM maintenance.unavailability`,
		[]any{sick, leave},
		&past,
		&gone,
	)

	if past != 1 || gone != 0 {
		t.Errorf("rows: past days kept %d, removed row %d", past, gone)
	}

	var audited int
	ownerScan(
		t,
		`SELECT count(*) FROM audit.audit_log WHERE action = 'members.clearUnavailability' AND result = 'success' AND target_id IN ($1, $2)`,
		[]any{leave, sick},
		&audited,
	)

	if audited != 3 {
		t.Errorf("%d audited clears, want 3", audited)
	}
}
