package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const refusedRecurrenceInvalid = "recurrence=" + apperr.KeyInvalid

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPlans(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Plan AC")
	next := clock().Add(10 * 24 * time.Hour) // 2026-09-24 01:00 UTC
	body := func(extra string) string {
		return `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"` + next.Format(
			time.RFC3339,
		) + `"` + extra + `}`
	}
	want := map[string]string{ // the field each case is refused for
		"interval 0":  refusedRecurrenceInvalid,
		"interval 13": refusedRecurrenceInvalid,
		casePast:      "nextDueAt=error.past",
		"weekly":      refusedRecurrenceInvalid,
	}

	for name, payload := range map[string]string{
		"interval 0": `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":0},` +
			`"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		"interval 13": `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":13},` +
			`"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		"weekly": `{"unitId":"` + unit + `","recurrence":{"kind":"weekly","intervalMonths":1},` +
			`"nextDueAt":"` + next.Format(time.RFC3339) + `"}`,
		casePast: `{"unitId":"` + unit + `","recurrence":{"kind":"monthly","intervalMonths":3},` +
			`"nextDueAt":"` + clock().Add(-time.Hour).Format(time.RFC3339) + `"}`,
	} {
		code, m := write(srv, hq(), "plans.save", payload, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		hq(),
		"plans.save",
		`{"unitId":"`+uuid.NewString()+`","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"`+next.Format(
			time.RFC3339,
		)+`"}`,
		0,
	); code != 404 {
		t.Error("unknown unit")
	}

	if code, _ := write(srv, customerA(), "plans.save", body(""), 0); code != 403 {
		t.Error("client saves plans")
	}

	code, resp := write(srv, hq(), "plans.save", body(""), 0)
	if code != 200 || asNum(t, data(resp)["anchorDay"]) != 24 || data(resp)["timezone"] != "UTC" ||
		len(asArr(t, data(resp)["generatedOccurrences"])) != 0 {
		t.Fatalf("create: %d %v", code, resp)
	}

	plan := asStr(t, data(resp)["id"])
	// update: interval only keeps the anchor; unit is fixed; version
	if code, res := write(
		srv,
		hq(),
		"plans.save",
		`{"id":"`+plan+`","unitId":"`+unit+`","recurrence":{"kind":"monthly","intervalMonths":1},"nextDueAt":"`+next.Format(
			time.RFC3339,
		)+`"}`,
		1,
	); code != 200 ||
		asNum(t, asObj(t, data(res)["recurrence"])["intervalMonths"]) != 1 ||
		asNum(t, data(res)["version"]) != 2 {
		t.Fatalf("update: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"plans.save",
		`{"id":"`+plan+`","unitId":"`+seed.ID("unit-non-rto").
			String()+
			`","recurrence":{"kind":"monthly","intervalMonths":1},"nextDueAt":"`+next.Format(
			time.RFC3339,
		)+`"}`,
		2,
	); code != 422 ||
		fields(res) != "unitId=error.unitFixed" {
		t.Errorf("unit fixed: %d %s", code, fields(res))
	}

	if code, _ := write(srv, hq(), "plans.save", body(`,"id":"`+plan+`"`), 1); code != 409 {
		t.Error("stale plan version")
	}
	// another next date re-anchors the recurrence on its day; an unchanged one keeps the anchor
	{
		other := newUnit(t, srv, "Plan AC 2")
		later := next.Add(5 * 24 * time.Hour) // the 29th
		save := func(id string, months int, when time.Time, version int) map[string]any {
			idField := ""
			if id != "" {
				idField = `"id":"` + id + `",`
			}

			code, res := write(
				srv,
				hq(),
				"plans.save",
				`{`+idField+`"unitId":"`+other+`","recurrence":{"kind":"monthly","intervalMonths":`+itoa(
					months,
				)+`},"nextDueAt":"`+when.Format(
					time.RFC3339,
				)+`"}`,
				version,
			)
			if code != 200 {
				t.Fatalf("save: %d %v", code, res)
			}

			return data(res)
		}

		created := save("", 3, next, 0)
		if created = save(
			asStr(t, created["id"]),
			3,
			later,
			1,
		); asNum(
			t,
			created["anchorDay"],
		) != 29 {
			t.Errorf("re-anchored: %v", created)
		}

		if created = save(
			asStr(t, created["id"]),
			6,
			later,
			2,
		); asNum(
			t,
			created["anchorDay"],
		) != 29 {
			t.Errorf("anchor kept: %v", created)
		}
	}
	// generateNext
	gen := func(at time.Time, version int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"plans.generateNext",
			`{"id":"`+plan+`","occurrenceDate":"`+at.Format(time.RFC3339)+`"}`,
			version,
		)
	}
	if code, _ := gen(next.Add(time.Hour), 2); code != 409 {
		t.Error("occurrence must be the saved next date")
	}

	code, resp = gen(next, 2)
	if code != 200 || data(resp)["type"] != "periodic" || data(resp)["origin"] != "periodic_plan" ||
		data(resp)["symptom"] != "Scheduled periodic maintenance" ||
		asObj(t, data(resp)["requestedSlot"])["endAt"] != next.Add(time.Hour).
			Format(time.RFC3339) ||
		len(asArr(t, data(resp)["preferredSlots"])) != 0 {
		t.Fatalf("generate: %d %v", code, resp)
	}

	job := asStr(t, data(resp)["id"])
	// the generated visit is a new request for the customer's clients, as a client's own request is (IR95 job.requested,
	// IR752)
	if n := countNotes(t, job, "customer-b", "job_update"); n != 1 {
		t.Errorf("client notices of the generated plan job: %d", n)
	}

	_, resp = post(srv, hq(), "plans.get", `{"id":"`+plan+`"}`)
	if data(resp)["nextDueAt"] != next.AddDate(0, 1, 0).Format(time.RFC3339) ||
		asObj(t, asArr(t, data(resp)["generatedOccurrences"])[0])["jobId"] != job ||
		ver(t, resp) != 3 {
		t.Fatalf("plan after generate: %v", resp)
	}

	if code, m := gen(next, 3); code != 409 || m["messageKey"] != "errors.occurrence_mismatch" {
		t.Errorf("old occurrence again: %d %v", code, m)
	}
	// an occurrence is generated once, even when the next date is moved back to it
	owner(t, `UPDATE maintenance.plans SET next_due_at = $2 WHERE id = $1`, plan, next)

	if code, m := gen(next, 3); code != 409 || m["messageKey"] != "errors.occurrence_generated" {
		t.Errorf("generated occurrence again: %d %v", code, m)
	}
	// an archived unit gets no new job
	owner(
		t,
		`UPDATE maintenance.plans SET next_due_at = $2 WHERE id = $1`,
		plan,
		next.AddDate(0, 1, 0),
	)
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, unit)

	code, resp = gen(next.AddDate(0, 1, 0), 3)

	owner(t, `UPDATE assets.units SET archived = false WHERE id = $1`, unit)

	if code != 409 || resp["messageKey"] != apperr.KeyUnitArchived {
		t.Errorf("archived unit: %d %v", code, resp)
	}
	// a past next date must be corrected first
	owner(
		t,
		`UPDATE maintenance.plans SET next_due_at = $2 WHERE id = $1`,
		plan,
		clock().Add(-time.Hour),
	)

	if code, _ := gen(clock().Add(-time.Hour), 3); code != 409 {
		t.Error("past occurrence")
	}
	// reads and filters
	if code, _ := post(srv, hq(), "plans.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown plan")
	}

	for body, want := range map[string]bool{
		`{"filters":{"unitId":"` + unit + `"}}`:                                                            true,
		`{"filters":{"unitId":"` + uuid.NewString() + `"}}`:                                                false,
		`{"filters":{"customerId":"` + seed.ID("cust-b").String() + `","unitId":"` + unit + `"}}`:          true,
		`{"filters":{"customerId":"` + seed.ID("cust-a").String() + `","unitId":"` + unit + `"}}`:          false,
		`{"filters":{"propertyId":"` + seed.ID("property-home-b").String() + `","unitId":"` + unit + `"}}`: true,
	} {
		_, m := post(srv, hq(), "plans.list", body)

		found := false
		for _, it := range items(t, m) {
			found = found || it["id"] == plan
		}

		if found != want {
			t.Errorf("%s: %v", body, found)
		}
	}

	if code, res := post(
		srv,
		hq(),
		"plans.list",
		`{"filters":{"x":1}}`,
	); code != 422 ||
		fields(res) != refusedUnknownFieldX {
		t.Errorf("bad filter: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		hq(),
		"plans.list",
		`{"sort":{"field":"updatedAt","direction":"desc"}}`,
	); code != 200 {
		t.Error("sort")
	}

	if code, _ := post(srv, customerA(), "plans.list", `{}`); code != 403 {
		t.Error("client lists plans")
	}
	// archived unit
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, unit)

	if code, res := write(
		srv,
		hq(),
		"plans.save",
		body(""),
		0,
	); code != 422 ||
		fields(res) != refusedUnitArchived {
		t.Errorf("archived unit: %d %s", code, fields(res))
	}
}
