package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestPlans(t *testing.T) {
	s := server(t)
	unit := newUnit(t, s, "Plan AC")
	next := clock.Add(10 * 24 * time.Hour) // 2026-09-24 01:00 UTC
	body := func(extra string) string {
		return `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"` + next.Format(time.RFC3339) + `"` + extra + `}`
	}
	for name, b := range map[string]string{
		"interval 0":  `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":0},"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		"interval 13": `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":13},"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		"weekly":      `{"unitId":"` + unit + `","recurrence":{"kind":"weekly","intervalMonths":1},"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		"past":        `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"` + clock.Add(-time.Hour).Format(time.RFC3339) + `"}`,
	} {
		if code, _ := write(s, &hq, "plans.save", b, 0); code != 422 {
			t.Errorf("save %s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "plans.save", `{"unitId":"`+uuid.NewString()+`","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"`+next.Format(time.RFC3339)+`"}`, 0); code != 404 {
		t.Error("unknown unit")
	}
	if code, _ := write(s, &customerA, "plans.save", body(""), 0); code != 403 {
		t.Error("client saves plans")
	}
	code, m := write(s, &hq, "plans.save", body(""), 0)
	if code != 200 || data(m)["anchorDay"].(float64) != 24 || data(m)["timezone"] != "UTC" || len(data(m)["generatedOccurrences"].([]any)) != 0 {
		t.Fatalf("create: %d %v", code, m)
	}
	plan := data(m)["id"].(string)
	// update: interval only keeps the anchor; unit is fixed; version
	if code, m := write(s, &hq, "plans.save", `{"id":"`+plan+`","unitId":"`+unit+`","recurrence":{"kind":"monthly","intervalMonths":1},"nextDueAt":"`+next.Format(time.RFC3339)+`"}`, 1); code != 200 ||
		data(m)["recurrence"].(map[string]any)["intervalMonths"].(float64) != 1 || data(m)["version"].(float64) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "plans.save", `{"id":"`+plan+`","unitId":"`+seed.ID("unit-non-rto").String()+`","recurrence":{"kind":"monthly","intervalMonths":1},"nextDueAt":"`+next.Format(time.RFC3339)+`"}`, 2); code != 422 {
		t.Error("unit fixed")
	}
	if code, _ := write(s, &hq, "plans.save", body(`,"id":"`+plan+`"`), 1); code != 409 {
		t.Error("stale plan version")
	}
	// generateNext
	gen := func(at time.Time, v int) (int, map[string]any) {
		return write(s, &hq, "plans.generateNext", `{"id":"`+plan+`","occurrenceDate":"`+at.Format(time.RFC3339)+`"}`, v)
	}
	if code, _ := gen(next.Add(time.Hour), 2); code != 409 {
		t.Error("occurrence must be the saved next date")
	}
	code, m = gen(next, 2)
	if code != 200 || data(m)["type"] != "periodic" || data(m)["origin"] != "periodic_plan" || data(m)["symptom"] != "Scheduled periodic maintenance" ||
		data(m)["requestedSlot"].(map[string]any)["endAt"] != next.Add(time.Hour).Format(time.RFC3339) || len(data(m)["preferredSlots"].([]any)) != 0 {
		t.Fatalf("generate: %d %v", code, m)
	}
	job := data(m)["id"].(string)
	_, m = post(s, &hq, "plans.get", `{"id":"`+plan+`"}`)
	if data(m)["nextDueAt"] != next.AddDate(0, 1, 0).Format(time.RFC3339) || data(m)["generatedOccurrences"].([]any)[0].(map[string]any)["jobId"] != job || ver(m) != 3 {
		t.Fatalf("plan after generate: %v", m)
	}
	if code, _ := gen(next, 3); code != 409 {
		t.Error("old occurrence again")
	}
	// a past next date must be corrected first
	owner(t, `UPDATE maintenance.plans SET next_due_at = $2 WHERE id = $1`, plan, clock.Add(-time.Hour))
	if code, _ := gen(clock.Add(-time.Hour), 3); code != 409 {
		t.Error("past occurrence")
	}
	// reads and filters
	if code, _ := post(s, &hq, "plans.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown plan")
	}
	for body, want := range map[string]bool{
		`{"filters":{"unitId":"` + unit + `"}}`:                                                            true,
		`{"filters":{"unitId":"` + uuid.NewString() + `"}}`:                                                false,
		`{"filters":{"customerId":"` + seed.ID("cust-b").String() + `","unitId":"` + unit + `"}}`:          true,
		`{"filters":{"customerId":"` + seed.ID("cust-a").String() + `","unitId":"` + unit + `"}}`:          false,
		`{"filters":{"propertyId":"` + seed.ID("property-home-b").String() + `","unitId":"` + unit + `"}}`: true,
	} {
		_, m := post(s, &hq, "plans.list", body)
		found := false
		for _, it := range items(m) {
			found = found || it["id"] == plan
		}
		if found != want {
			t.Errorf("%s: %v", body, found)
		}
	}
	if code, _ := post(s, &hq, "plans.list", `{"filters":{"x":1}}`); code != 422 {
		t.Error("bad filter")
	}
	if code, _ := post(s, &hq, "plans.list", `{"sort":{"field":"updatedAt","direction":"desc"}}`); code != 200 {
		t.Error("sort")
	}
	if code, _ := post(s, &customerA, "plans.list", `{}`); code != 403 {
		t.Error("client lists plans")
	}
	// archived unit
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, unit)
	if code, _ := write(s, &hq, "plans.save", body(""), 0); code != 422 {
		t.Error("archived unit")
	}
}
