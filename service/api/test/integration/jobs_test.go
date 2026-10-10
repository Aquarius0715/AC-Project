package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// slotJSON returns a Slot starting h hours after the fixture clock lasting d hours.
func slotJSON(h, d int) string {
	s := clock.Add(time.Duration(h) * time.Hour)
	return `{"startAt":"` + s.Format(time.RFC3339) + `","endAt":"` + s.Add(time.Duration(d)*time.Hour).Format(time.RFC3339) + `"}`
}

func jobBody(unit string, extra map[string]string) string {
	// clock = 2026-09-14 09:00 Kuala Lumpur; +25 h is the next local day
	f := map[string]string{"unitId": `"` + unit + `"`, "type": `"reactive"`, "symptom": `"Water dripping from the indoor unit"`,
		"requestedStart": `"` + clock.Add(25*time.Hour).Format(time.RFC3339) + `"`, "requestedEnd": `"` + clock.Add(27*time.Hour).Format(time.RFC3339) + `"`,
		"alternativeSlots": "[" + slotJSON(49, 2) + "," + slotJSON(73, 1) + "]"}
	for k, v := range extra {
		if v == "" {
			delete(f, k)
			continue
		}
		f[k] = v
	}
	parts := []string{}
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestJobsCreate(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-non-rto").String()
	code, m := write(s, &customerA, "jobs.create", jobBody(unit, map[string]string{"contactWindow": `" Weekdays 09:00-18:00 "`}), 0)
	j := data(m)
	if code != 200 || j["status"] != "requested" || j["origin"] != "client_request" || len(j["preferredSlots"].([]any)) != 3 ||
		j["dueAt"] != clock.Add(27*time.Hour).Format(time.RFC3339) || j["contactWindow"] != "Weekdays 09:00-18:00" || j["projection"] != "detail" || j["preferenceRound"].(float64) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	// HQ on behalf: 0 alternatives, explicit dueAt
	code, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]", "dueAt": `"` + clock.Add(48*time.Hour).Format(time.RFC3339) + `"`}), 0)
	if code != 200 || len(data(m)["preferredSlots"].([]any)) != 1 || data(m)["dueAt"] != clock.Add(48*time.Hour).Format(time.RFC3339) {
		t.Fatalf("hq create: %d %v", code, m)
	}
	bad := map[string]struct {
		a     *actor
		extra map[string]string
		code  int
	}{
		"symptom 9":              {&customerA, map[string]string{"symptom": `"too short"`}, 422},
		"symptom 2001":           {&customerA, map[string]string{"symptom": `"` + strings.Repeat("s", 2001) + `"`}, 422},
		"bad type":               {&customerA, map[string]string{"type": `"emergency"`}, 422},
		"client one alternative": {&customerA, map[string]string{"alternativeSlots": "[" + slotJSON(49, 2) + "]"}, 422},
		"hq three alternatives":  {&hq, map[string]string{"alternativeSlots": "[" + slotJSON(49, 2) + "," + slotJSON(73, 1) + "," + slotJSON(97, 1) + "]"}, 422},
		"today (KL)":             {&customerA, map[string]string{"requestedStart": `"` + clock.Add(5*time.Hour).Format(time.RFC3339) + `"`, "requestedEnd": `"` + clock.Add(7*time.Hour).Format(time.RFC3339) + `"`}, 422},
		"slot 5 hours":           {&customerA, map[string]string{"alternativeSlots": "[" + slotJSON(49, 5) + "," + slotJSON(73, 1) + "]"}, 422},
		"duplicate slots":        {&customerA, map[string]string{"alternativeSlots": "[" + slotJSON(25, 2) + "," + slotJSON(73, 1) + "]"}, 422},
		"client dueAt":           {&customerA, map[string]string{"dueAt": `"` + clock.Add(48*time.Hour).Format(time.RFC3339) + `"`}, 422},
		"hq dueAt before end":    {&hq, map[string]string{"dueAt": `"` + clock.Add(26*time.Hour).Format(time.RFC3339) + `"`}, 422},
		"contact email":          {&customerA, map[string]string{"contactWindow": `"call me at a@b.c"`}, 422},
		"contact phone":          {&customerA, map[string]string{"contactWindow": `"+60 (12) 345-6789"`}, 422},
		"contact 201":            {&customerA, map[string]string{"contactWindow": `"` + strings.Repeat("w", 201) + `"`}, 422},
		"other customer's unit":  {&customerB, nil, 404},
		"unknown unit":           {&hq, map[string]string{"unitId": `"` + uuid.NewString() + `"`}, 404},
		"missing alternatives":   {&customerA, map[string]string{"alternativeSlots": ""}, 422},
		"technician":             {&techB, nil, 403},
	}
	for name, tc := range bad {
		if code, _ := write(s, tc.a, "jobs.create", jobBody(unit, tc.extra), 0); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	archived := newUnit(t, s, "Archived AC")
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, archived)
	if code, _ := write(s, &hq, "jobs.create", jobBody(archived, nil), 0); code != 422 {
		t.Error("archived unit")
	}
}

func TestJobsReadAndTransitions(t *testing.T) {
	s := server(t)
	unit := newUnit(t, s, "Job AC")
	_, m := write(s, &customerB, "jobs.create", jobBody(unit, map[string]string{"contactWindow": `"Mornings"`}), 0)
	job := data(m)["id"].(string)
	newAlert(t, unit, seed.ID("org-customer-b").String(), "warning", clock.Add(-time.Minute))

	// list: summary projection with severity, display status, filters
	_, m = post(s, &customerB, "jobs.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if len(items(m)) != 1 || items(m)[0]["projection"] != "summary" || items(m)[0]["severity"] != "warning" || items(m)[0]["displayStatus"] != "requested" {
		t.Fatalf("list: %v", m)
	}
	_, m = post(s, &customerA, "jobs.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if len(items(m)) != 0 {
		t.Fatal("customer-a sees customer-b's job")
	}
	for body, n := range map[string]int{
		`{"filters":{"unitId":"` + unit + `","severity":"warning"}}`:                                                                                                      1,
		`{"filters":{"unitId":"` + unit + `","severity":"critical"}}`:                                                                                                     0,
		`{"filters":{"unitIds":["` + unit + `"],"statuses":["requested","offered"]}}`:                                                                                     1,
		`{"filters":{"unitId":"` + unit + `","status":"assigned"}}`:                                                                                                       0,
		`{"filters":{"unitId":"` + unit + `","origin":"client_request","proposalPending":false}}`:                                                                         1,
		`{"filters":{"unitId":"` + unit + `","proposalPending":true}}`:                                                                                                    0,
		`{"filters":{"unitId":"` + unit + `","overdueOnly":true}}`:                                                                                                        0,
		`{"filters":{"unitId":"` + unit + `","type":"reactive"}}`:                                                                                                         1, // IR290
		`{"filters":{"unitId":"` + unit + `","type":"periodic"}}`:                                                                                                         0,
		`{"filters":{"unitId":"` + unit + `","customerId":"` + seed.ID("cust-b").String() + `"}}`:                                                                         1,
		`{"filters":{"unitId":"` + unit + `","propertyId":"` + seed.ID("property-home-b").String() + `"}}`:                                                                1,
		`{"filters":{"unitId":"` + unit + `","organizationId":"` + seed.ID("org-operator-a").String() + `"}}`:                                                             1,
		`{"filters":{"unitId":"` + unit + `","organizationId":"` + seed.ID("org-contractor-a").String() + `"}}`:                                                           0,
		`{"filters":{"unitId":"` + unit + `","membershipId":"` + seed.ID("tech-internal-a").String() + `"}}`:                                                              0,
		`{"filters":{"unitId":"` + unit + `","from":"` + clock.Add(24*time.Hour).Format(time.RFC3339) + `","to":"` + clock.Add(26*time.Hour).Format(time.RFC3339) + `"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","from":"` + clock.Add(26*time.Hour).Format(time.RFC3339) + `"}}`:                                                             0,
	} {
		if _, m := post(s, &hq, "jobs.list", body); len(items(m)) != n {
			t.Errorf("%s: %d want %d", body, len(items(m)), n)
		}
	}
	for _, b := range []string{`{"filters":{"status":"requested","statuses":["requested"]}}`, `{"filters":{"statuses":[]}}`, `{"filters":{"status":"done"}}`,
		`{"filters":{"statuses":["done"]}}`, `{"filters":{"severity":"info"}}`, `{"filters":{"origin":"phone"}}`, `{"filters":{"type":"repair"}}`, `{"filters":{"x":1}}`,
		`{"filters":{"from":"2026-09-15T00:00:00Z","to":"2026-09-14T00:00:00Z"}}`, `{"sort":{"field":"unitId","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "jobs.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	for _, f := range []string{"id", "severity", "dueAt", "status"} {
		for _, d := range []string{"asc", "desc"} {
			if code, _ := post(s, &hq, "jobs.list", `{"sort":{"field":"`+f+`","direction":"`+d+`"},"limit":5}`); code != 200 {
				t.Errorf("sort %s %s: %d", f, d, code)
			}
		}
	}
	// status rank order (default sort): requested before cancelled
	_, m = post(s, &hq, "jobs.list", `{"limit":100}`)
	last := -1
	rank := map[string]int{"requested": 0, "offered": 1, "accepted": 2, "assigned": 3, "in_progress": 4, "on_hold": 5, "submitted": 6, "rework_requested": 7, "completed": 8, "cancelled": 9}
	for _, it := range items(m) {
		if r := rank[it["status"].(string)]; r < last {
			t.Fatal("status rank order")
		} else {
			last = r
		}
	}

	// get / contact window visibility
	if code, m := post(s, &customerB, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["contactWindow"] != "Mornings" {
		t.Fatalf("get: %d %v", code, m)
	}
	if code, _ := post(s, &customerA, "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("other customer get")
	}
	if code, _ := post(s, &techB, "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("unassigned technician")
	}

	// notes and events
	if code, _ := write(s, &customerB, "jobs.addNote", `{"jobId":"`+job+`","message":"internal?","visibility":"internal"}`, 1); code != 403 {
		t.Error("client internal note")
	}
	if code, m := write(s, &customerB, "jobs.addNote", `{"jobId":"`+job+`","message":"  Please call before arriving  ","visibility":"customer"}`, 1); code != 200 || data(m)["jobId"] != job || data(m)["message"] != "Please call before arriving" || data(m)["visibility"] != "customer" {
		t.Fatalf("client note: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "jobs.addNote", `{"jobId":"`+job+`","message":"Check parts stock","visibility":"internal"}`, 2); code != 200 {
		t.Fatal("hq internal note")
	}
	if code, _ := write(s, &hq, "jobs.addNote", `{"jobId":"`+job+`","message":"   ","visibility":"internal"}`, 3); code != 422 {
		t.Error("blank note")
	}
	if code, _ := write(s, &hq, "jobs.addNote", `{"jobId":"`+job+`","message":"x","visibility":"public"}`, 3); code != 422 {
		t.Error("bad visibility")
	}
	if code, _ := write(s, &hq, "jobs.addNote", `{"jobId":"`+job+`","message":"stale","visibility":"internal"}`, 1); code != 409 {
		t.Error("stale note")
	}
	_, m = post(s, &customerB, "jobs.events", `{"jobId":"`+job+`","query":{}}`)
	acts := []string{}
	for _, e := range items(m) {
		acts = append(acts, e["action"].(string))
		if n, ok := e["note"].(map[string]any); ok && n["visibility"] == "internal" {
			t.Fatal("client sees internal note")
		}
	}
	if strings.Join(acts, ",") != "job.created,note.added" {
		t.Fatalf("client events: %v", acts)
	}
	_, m = post(s, &hq, "jobs.events", `{"jobId":"`+job+`","query":{"sort":{"field":"occurredAt","direction":"desc"},"filters":{"from":"`+clock.Add(-time.Hour).Format(time.RFC3339)+`","to":"`+clock.Add(time.Hour).Format(time.RFC3339)+`"}}}`)
	if len(items(m)) != 3 || items(m)[0]["createdAt"] != items(m)[0]["occurredAt"] {
		t.Fatalf("hq events: %v", m)
	}
	for _, b := range []string{`{"jobId":"` + job + `","query":{"filters":{"x":1}}}`, `{"jobId":"` + job + `","query":{"filters":{"from":"2026-09-15T00:00:00Z","to":"2026-09-14T00:00:00Z"}}}`, `{"query":{}}`} {
		if code, _ := post(s, &hq, "jobs.events", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "jobs.events", `{"jobId":"`+job+`","query":{}}`); code != 404 {
		t.Error("events scope")
	}

	// hold / resume / cancel state table
	if code, _ := write(s, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":"parts"}`, 3); code != 409 {
		t.Error("hold from requested")
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, job)
	if code, _ := write(s, &customerB, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"no longer needed"}`, 3); code != 409 {
		t.Error("client cancel in_progress")
	}
	if code, m := write(s, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":" waiting for parts "}`, 3); code != 200 || data(m)["status"] != "on_hold" {
		t.Fatalf("hold: %d", code)
	}
	if code, _ := write(s, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":"again"}`, 4); code != 409 {
		t.Error("hold twice")
	}
	if code, m := write(s, &hq, "jobs.resumeHold", `{"jobId":"`+job+`","reason":"parts arrived"}`, 4); code != 200 || data(m)["status"] != "in_progress" {
		t.Fatalf("resume: %d", code)
	}
	if code, _ := write(s, &hq, "jobs.resumeHold", `{"jobId":"`+job+`","reason":"x"}`, 5); code != 409 {
		t.Error("resume when not on hold")
	}
	if code, _ := write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"x"}`, 5); code != 409 {
		t.Error("hq cancel in_progress")
	}
	if code, _ := write(s, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":""}`, 5); code != 422 {
		t.Error("blank hold reason")
	}
	write(s, &hq, "jobs.hold", `{"jobId":"`+job+`","reason":"stop"}`, 5)
	if code, m := write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"customer withdrew"}`, 6); code != 200 || data(m)["status"] != "cancelled" {
		t.Fatalf("cancel on_hold: %d", code)
	}
	if code, _ := write(s, &hq, "jobs.addNote", `{"jobId":"`+job+`","message":"late","visibility":"internal"}`, 7); code != 409 {
		t.Error("note on cancelled")
	}
	if code, _ := write(s, &hq, "jobs.cancel", `{"jobId":"`+job+`","cancelReason":"again"}`, 7); code != 409 {
		t.Error("cancel twice")
	}

	// client cancels a requested job; offers and assignments of an offered job are closed by HQ cancel
	_, m = write(s, &customerB, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	if code, _ := write(s, &customerB, "jobs.cancel", `{"jobId":"`+j2+`","cancelReason":"   "}`, 1); code != 422 {
		t.Error("blank cancel reason")
	}
	if code, _ := write(s, &customerB, "jobs.cancel", `{"jobId":"`+j2+`","cancelReason":"fixed itself"}`, 1); code != 200 {
		t.Fatal("client cancel requested")
	}
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	j3 := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'offered' WHERE id = $1`, j3)
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until)
		VALUES ($1,$2,$3,'t1',tstzrange($4,$5),$4,$5,$4,$5)`, seed.ID("tenant-a"), j3, seed.ID("org-contractor-a"), clock, clock.Add(time.Hour))
	if code, _ := write(s, &hq, "jobs.cancel", `{"jobId":"`+j3+`","cancelReason":"duplicate"}`, 1); code != 200 {
		t.Fatal("hq cancel offered")
	}
	if code, _ := write(s, &techB, "jobs.cancel", `{"jobId":"`+j3+`","cancelReason":"x"}`, 2); code != 403 {
		t.Error("technician cancel")
	}
}
