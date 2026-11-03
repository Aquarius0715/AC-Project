package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	fieldContactWindow = "contactWindow"
	fieldSymptom       = "symptom"
	refusedSlotRules   = "alternativeSlots=" + apperr.KeySlotRules
)

// slotJSON returns a Slot starting h hours after the fixture clock lasting d hours.
func slotJSON(h, d int) string {
	s := clock().Add(time.Duration(h) * time.Hour)

	return `{"startAt":"` + s.Format(
		time.RFC3339,
	) + `","endAt":"` + s.Add(time.Duration(d)*time.Hour).
		Format(time.RFC3339) +
		`"}`
}

func jobBody(unit string, extra map[string]string) string {
	// clock = 2026-09-14 09:00 Kuala Lumpur; +25 h is the next local day
	f := map[string]string{
		fieldUnitID:  `"` + unit + `"`,
		fieldType:    `"reactive"`,
		fieldSymptom: `"Water dripping from the indoor unit"`,
		fieldRequestedStart: `"` + clock().Add(25*time.Hour).
			Format(time.RFC3339) +
			`"`,
		fieldRequestedEnd:     `"` + clock().Add(27*time.Hour).Format(time.RFC3339) + `"`,
		fieldAlternativeSlots: "[" + slotJSON(49, 2) + "," + slotJSON(73, 1) + "]",
	}

	for key, value := range extra {
		if value == "" {
			delete(f, key)

			continue
		}

		f[key] = value
	}

	parts := make([]string, 0, len(f))
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestJobsCreate(t *testing.T) {
	srv := server(t)
	unit := seed.ID("unit-non-rto").String()
	code, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldContactWindow: `" Weekdays 09:00-18:00 "`}),
		0,
	)

	job := data(resp)
	if code != 200 || job["status"] != statusRequested || job["origin"] != "client_request" ||
		len(asArr(t, job["preferredSlots"])) != 3 ||
		job["dueAt"] != clock().Add(27*time.Hour).Format(time.RFC3339) ||
		job["contactWindow"] != "Weekdays 09:00-18:00" ||
		asNum(t, job["preferenceRound"]) != 1 {
		t.Fatalf("create: %d %v", code, resp)
	}

	if _, detail := job["projection"]; detail { // the write answers MaintenanceJob; the projections are jobs.get's (IR392)
		t.Errorf("jobs.create answers a projection: %v", job)
	}
	// HQ receives every field (IR42): a reader without job.write sees the customer's contact window too (IR353)
	owner(
		t,
		`DELETE FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'job.write'`,
		seed.ID("hq-restriction-manager"),
	)
	t.Cleanup(func() {
		owner(
			t,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1, $2, 'job.write') ON
				CONFLICT DO NOTHING`,
			seed.ID("tenant-a"),
			seed.ID("hq-restriction-manager"),
		)
	})

	if code, res := post(
		srv,
		restrMgr(),
		"jobs.get",
		`{"jobId":"`+asStr(t, job["id"])+`"}`,
	); code != 200 ||
		data(res)["contactWindow"] != "Weekdays 09:00-18:00" {
		t.Errorf("HQ reader without job.write: %d %v", code, data(res)["contactWindow"])
	}
	// HQ on behalf: 0 alternatives, explicit dueAt
	code, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(
			unit,
			map[string]string{
				fieldAlternativeSlots: "[]",
				fieldDueAt:            `"` + clock().Add(48*time.Hour).Format(time.RFC3339) + `"`,
			},
		),
		0,
	)
	if code != 200 || len(asArr(t, data(resp)["preferredSlots"])) != 1 ||
		data(resp)["dueAt"] != clock().Add(48*time.Hour).Format(time.RFC3339) {
		t.Fatalf("hq create: %d %v", code, resp)
	}

	bad := map[string]struct {
		a     *actor
		extra map[string]string
		code  int
	}{
		"symptom 9": {customerA(), map[string]string{fieldSymptom: `"too short"`}, 422},
		"symptom 2001": {
			customerA(),
			map[string]string{fieldSymptom: `"` + strings.Repeat("s", 2001) + `"`},
			422,
		},
		caseBadType: {customerA(), map[string]string{fieldType: `"emergency"`}, 422},
		"client one alternative": {
			customerA(),
			map[string]string{fieldAlternativeSlots: "[" + slotJSON(49, 2) + "]"},
			422,
		},
		"hq three alternatives": {
			hq(),
			map[string]string{
				fieldAlternativeSlots: "[" + slotJSON(
					49,
					2,
				) + "," + slotJSON(
					73,
					1,
				) + "," + slotJSON(
					97,
					1,
				) + "]",
			},
			422,
		},
		"today (KL)": {
			customerA(),
			map[string]string{
				fieldRequestedStart: `"` + clock().Add(5*time.Hour).Format(time.RFC3339) + `"`,
				fieldRequestedEnd:   `"` + clock().Add(7*time.Hour).Format(time.RFC3339) + `"`,
			},
			422,
		},
		"slot 5 hours": {
			customerA(),
			map[string]string{
				fieldAlternativeSlots: "[" + slotJSON(49, 5) + "," + slotJSON(73, 1) + "]",
			},
			422,
		},
		"duplicate slots": {
			customerA(),
			map[string]string{
				fieldAlternativeSlots: "[" + slotJSON(25, 2) + "," + slotJSON(73, 1) + "]",
			},
			422,
		},
		"client dueAt": {
			customerA(),
			map[string]string{
				fieldDueAt: `"` + clock().Add(48*time.Hour).Format(time.RFC3339) + `"`,
			},
			422,
		},
		"hq dueAt before end": {
			hq(),
			map[string]string{
				fieldDueAt: `"` + clock().Add(26*time.Hour).Format(time.RFC3339) + `"`,
			},
			422,
		},
		"contact email": {
			customerA(),
			map[string]string{fieldContactWindow: `"call me at a@b.c"`},
			422,
		},
		"contact phone": {
			customerA(),
			map[string]string{fieldContactWindow: `"+60 (12) 345-6789"`},
			422,
		},
		"contact 201": {
			customerA(),
			map[string]string{fieldContactWindow: `"` + strings.Repeat("w", 201) + `"`},
			422,
		},
		"other customer's unit": {customerB(), nil, 404},
		caseUnknownUnit: {
			hq(),
			map[string]string{fieldUnitID: `"` + uuid.NewString() + `"`},
			404,
		},
		"missing alternatives": {customerA(), map[string]string{fieldAlternativeSlots: ""}, 422},
		roleTechnician:         {techB(), nil, 403},
	}
	refused := map[string]string{ // the field each VALIDATION case is refused for
		caseBadType:              "type=error.invalid",
		"client dueAt":           "dueAt=error.notAllowed",
		"client one alternative": "alternativeSlots=error.count",
		"contact 201":            "contactWindow=error.length",
		"contact email":          "contactWindow=errors.contact_details_forbidden",
		"contact phone":          "contactWindow=errors.contact_details_forbidden",
		"duplicate slots":        refusedSlotRules,
		"hq dueAt before end":    "dueAt=error.beforeRequestedEnd",
		"hq three alternatives":  "alternativeSlots=error.count",
		"missing alternatives":   "alternativeSlots=error.required",
		"slot 5 hours":           refusedSlotRules,
		"symptom 2001":           "symptom=error.length",
		"symptom 9":              "symptom=error.length",
		"today (KL)":             refusedSlotRules,
	}

	for name, tc := range bad {
		code, m := write(srv, tc.a, "jobs.create", jobBody(unit, tc.extra), 0)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}

	archived := newUnit(t, srv, "Archived AC")
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, archived)

	if code, res := write(
		srv,
		hq(),
		"jobs.create",
		jobBody(archived, nil),
		0,
	); code != 422 ||
		fields(res) != refusedUnitArchived {
		t.Errorf("archived unit: %d %s", code, fields(res))
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestJobsReadAndTransitions(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Job AC")
	_, resp := write(
		srv,
		customerB(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldContactWindow: `"Mornings"`}),
		0,
	)
	job := asStr(t, data(resp)["id"])

	newAlert(t, unit, seed.ID("org-customer-b").String(), "warning", clock().Add(-time.Minute))

	// list: summary projection with severity, display status, filters
	_, resp = post(srv, customerB(), "jobs.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if len(items(t, resp)) != 1 || items(t, resp)[0]["projection"] != projectionSummary ||
		items(t, resp)[0]["severity"] != severityWarning ||
		items(t, resp)[0]["displayStatus"] != statusRequested {
		t.Fatalf("list: %v", resp)
	}

	_, resp = post(srv, customerA(), "jobs.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if len(items(t, resp)) != 0 {
		t.Fatal("customer-a sees customer-b's job")
	}

	for body, want := range map[string]int{
		`{"filters":{"unitId":"` + unit + `","severity":"warning"}}`:  1,
		`{"filters":{"unitId":"` + unit + `","severity":"critical"}}`: 0,
		`{"filters":{"unitIds":["` + unit + `"],"statuses":["requested",` +
			`"offered"]}}`: 1,
		`{"filters":{"unitId":"` + unit + `","status":"assigned"}}`: 0,
		`{"filters":{"unitId":"` + unit + `","origin":"client_request",` +
			`"proposalPending":false}}`: 1,
		`{"filters":{"unitId":"` + unit + `","proposalPending":true}}`: 0,
		`{"filters":{"unitId":"` + unit + `","overdueOnly":true}}`:     0,
		// IR290
		`{"filters":{"unitId":"` + unit +
			`","type":"reactive"}}`: 1,
		`{"filters":{"unitId":"` + unit +
			`","type":"periodic"}}`: 0,
		`{"filters":{"unitId":"` + unit + `","customerId":"` + seed.ID("cust-b").String() +
			`"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","propertyId":"` + seed.ID("property-home-b").String() +
			`"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","organizationId":"` + seed.ID("org-operator-a").String() +
			`"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","organizationId":"` + seed.ID("org-contractor-a").String() +
			`"}}`: 0,
		`{"filters":{"unitId":"` + unit + `","membershipId":"` + seed.ID("tech-internal-a").String() +
			`"}}`: 0,
		`{"filters":{"unitId":"` + unit + `","from":"` + clock().Add(24*time.Hour).Format(time.RFC3339) + `","to":"` +
			clock().Add(26*time.Hour).Format(time.RFC3339) + `"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","from":"` + clock().Add(26*time.Hour).Format(time.RFC3339) +
			`"}}`: 0,
	} {
		if code, m := post(srv, hq(), "jobs.list", body); code != 200 || len(items(t, m)) != want {
			t.Errorf("%s: %d want %d", body, len(items(t, m)), want)
		}
	}
	// the field each case is refused for
	want := map[string]string{
		"{\"filters\":{\"from\":\"2026-09-15T00:00:00Z\",\"to\":\"2026-09-14T00:00:00Z\"}}": refusedFilterToRange,
		"{\"filters\":{\"origin\":\"phone\"}}":                                              "filters.origin=error.invalid",
		bodyInfoSeverity:                                                                    "filters.severity=error.invalid",
		"{\"filters\":{\"status\":\"done\"}}":                                               refusedFilterStatusInvalid,
		"{\"filters\":{\"status\":\"requested\",\"statuses\":[\"requested\"]}}":             refusedFilterStatusesInvalid,
		"{\"filters\":{\"statuses\":[\"done\"]}}":                                           refusedFilterStatusesInvalid,
		"{\"filters\":{\"statuses\":[]}}":                                                   refusedFilterStatusesInvalid,
		"{\"filters\":{\"type\":\"repair\"}}":                                               "filters.type=error.invalid",
		bodyUnknownFilter:                                                                   refusedUnknownFieldX,
		"{\"sort\":{\"field\":\"unitId\",\"direction\":\"asc\"}}":                           refusedSortFieldInvalid,
	}

	for _, body := range []string{
		`{"filters":{"status":"requested",` +
			`"statuses":["requested"]}}`, `{"filters":{"statuses":[]}}`, `{"filters":{"status":"done"}}`,
		`{"filters":{"statuses":["done"]}}`, bodyInfoSeverity, `{"filters":{"origin":"phone"}}`,
		`{"filters":{"type":"repair"}}`, bodyUnknownFilter,
		`{"filters":{"from":"2026-09-15T00:00:00Z",` +
			`"to":"2026-09-14T00:00:00Z"}}`, `{"sort":{"field":"unitId",` +
			`"direction":"asc"}}`,
	} {
		code, m := post(srv, hq(), "jobs.list", body)
		refusal(t, body, code, m, want[body])
	}

	for _, field := range []string{"id", fieldSeverity, fieldDueAt, fieldStatus} {
		for _, direction := range []string{"asc", "desc"} {
			if code, _ := post(
				srv,
				hq(),
				"jobs.list",
				`{"sort":{"field":"`+field+`","direction":"`+direction+`"},"limit":5}`,
			); code != 200 {
				t.Errorf("sort %s %s: %d", field, direction, code)
			}
		}
	}
	// status rank order (default sort): requested before cancelled
	_, resp = post(srv, hq(), "jobs.list", `{"limit":100}`)
	last := -1

	rank := map[string]int{
		statusRequested:    0,
		statusOffered:      1,
		statusAccepted:     2,
		statusAssigned:     3,
		statusInProgress:   4,
		statusOnHold:       5,
		"submitted":        6,
		"rework_requested": 7,
		statusCompleted:    8,
		statusCancelled:    9,
	}
	for _, it := range items(t, resp) {
		if r := rank[asStr(t, it["status"])]; r < last {
			t.Fatal("status rank order")
		} else {
			last = r
		}
	}

	// get / contact window visibility
	if code, res := post(
		srv,
		customerB(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 ||
		data(res)["contactWindow"] != "Mornings" {
		t.Fatalf("get: %d %v", code, res)
	}

	if code, _ := post(srv, customerA(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("other customer get")
	}

	if code, _ := post(srv, techB(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("unassigned technician")
	}

	// notes and events
	if code, _ := write(
		srv,
		customerB(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"internal?","visibility":"internal"}`,
		1,
	); code != 403 {
		t.Error("client internal note")
	}

	if code, res := write(
		srv,
		customerB(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"  Please call before arriving  ","visibility":"customer"}`,
		1,
	); code != 200 || data(res)["jobId"] != job || data(res)["message"] != "Please call before arriving" ||
		data(res)["visibility"] != "customer" {
		t.Fatalf("client note: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"Check parts stock","visibility":"internal"}`,
		2,
	); code != 200 {
		t.Fatal("hq internal note")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"   ","visibility":"internal"}`,
		3,
	); code != 422 ||
		fields(res) != refusedMessageLength {
		t.Errorf("blank note: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"x","visibility":"public"}`,
		3,
	); code != 422 ||
		fields(res) != "visibility=error.invalid" {
		t.Errorf("bad visibility: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"stale","visibility":"internal"}`,
		1,
	); code != 409 {
		t.Error("stale note")
	}

	_, resp = post(srv, customerB(), "jobs.events", `{"jobId":"`+job+`","query":{}}`)

	events := items(t, resp)

	acts := make([]string, 0, len(events))
	for _, e := range events {
		acts = append(acts, asStr(t, e["action"]))
		if n, ok := e["note"].(map[string]any); ok && n["visibility"] == "internal" {
			t.Fatal("client sees internal note")
		}
	}

	if strings.Join(acts, ",") != "job.created,note.added" {
		t.Fatalf("client events: %v", acts)
	}

	_, resp = post(
		srv,
		hq(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{"sort":{"field":"occurredAt","direction":"desc"},`+
			`"filters":{"from":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`","to":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`"}}}`,
	)
	if len(items(t, resp)) != 3 ||
		items(t, resp)[0]["createdAt"] != items(t, resp)[0]["occurredAt"] {
		t.Fatalf("hq events: %v", resp)
	}

	for body, want := range map[string]string{
		`{"jobId":"` + job + `","query":{"filters":{"x":1}}}`: refusedUnknownFieldX,
		`{"jobId":"` + job + `","query":{"filters":{"from":"2026-09-15T00:00:00Z",` +
			`"to":"2026-09-14T00:00:00Z"}}}`: "query.filters.to=error.range",
		bodyEmptyQuery: "jobId=error.invalid",
	} {
		code, m := post(srv, hq(), "jobs.events", body)
		refusal(t, body, code, m, want)
	}

	if code, _ := post(
		srv,
		customerA(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{}}`,
	); code != 404 {
		t.Error("events scope")
	}

	// hold / resume / cancel state table
	if code, _ := write(
		srv,
		hq(),
		"jobs.hold",
		`{"jobId":"`+uuid.NewString()+`","reason":"parts"}`,
		1,
	); code != 404 {
		t.Error("hold of an unknown job")
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.hold",
		`{"jobId":"`+job+`","reason":"parts"}`,
		3,
	); code != 409 {
		t.Error("hold from requested")
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'in_progress' WHERE id = $1`, job)

	if code, _ := write(
		srv,
		customerB(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"no longer needed"}`,
		3,
	); code != 409 {
		t.Error("client cancel in_progress")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.hold",
		`{"jobId":"`+job+`","reason":" waiting for parts "}`,
		3,
	); code != 200 ||
		data(res)["status"] != statusOnHold {
		t.Fatalf("hold: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.hold",
		`{"jobId":"`+job+`","reason":"again"}`,
		4,
	); code != 409 {
		t.Error("hold twice")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.resumeHold",
		`{"jobId":"`+job+`","reason":"parts arrived"}`,
		4,
	); code != 200 ||
		data(res)["status"] != statusInProgress {
		t.Fatalf("resume: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.resumeHold",
		`{"jobId":"`+job+`","reason":"x"}`,
		5,
	); code != 409 {
		t.Error("resume when not on hold")
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"x"}`,
		5,
	); code != 409 {
		t.Error("hq cancel in_progress")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.hold",
		`{"jobId":"`+job+`","reason":""}`,
		5,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("blank hold reason: %d %s", code, fields(res))
	}

	write(srv, hq(), "jobs.hold", `{"jobId":"`+job+`","reason":"stop"}`, 5)

	if code, res := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"customer withdrew"}`,
		6,
	); code != 200 ||
		data(res)["status"] != statusCancelled {
		t.Fatalf("cancel on_hold: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"late","visibility":"internal"}`,
		7,
	); code != 409 {
		t.Error("note on cancelled")
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"again"}`,
		7,
	); code != 409 {
		t.Error("cancel twice")
	}

	// client cancels a requested job; offers and assignments of an offered job are closed by HQ cancel
	_, resp = write(srv, customerB(), "jobs.create", jobBody(unit, nil), 0)

	job2 := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		customerB(),
		"jobs.cancel",
		`{"jobId":"`+job2+`","cancelReason":"   "}`,
		1,
	); code != 422 ||
		fields(res) != "cancelReason=error.length" {
		t.Errorf("blank cancel reason: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		customerB(),
		"jobs.cancel",
		`{"jobId":"`+job2+`","cancelReason":"fixed itself"}`,
		1,
	); code != 200 {
		t.Fatal("client cancel requested")
	}

	_, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	job3 := asStr(t, data(resp)["id"])
	owner(t, `UPDATE maintenance.jobs SET status = 'offered' WHERE id = $1`, job3)
	owner(
		t,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at,
			offer_expires_at, access_valid_from, access_valid_until)
		VALUES ($1,$2,$3,'t1',tstzrange($4,$5),$4,$5,$4,$5)`,
		seed.ID("tenant-a"),
		job3,
		seed.ID("org-contractor-a"),
		clock(),
		clock().Add(time.Hour),
	)

	if code, _ := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job3+`","cancelReason":"duplicate"}`,
		1,
	); code != 200 {
		t.Fatal("hq cancel offered")
	}

	if code, _ := write(
		srv,
		techB(),
		"jobs.cancel",
		`{"jobId":"`+job3+`","cancelReason":"x"}`,
		2,
	); code != 403 {
		t.Error("technician cancel")
	}
}
