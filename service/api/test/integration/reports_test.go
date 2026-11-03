package integration_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const (
	fieldParts          = "parts"
	fieldRefrigerant    = "refrigerant"
	fieldWorkText       = "workText"
	refusedItemsInvalid = "items=" + apperr.KeyInvalid
)

// draftBody builds a WorkReportDraft with all 18 components (result as given) and overrides.
func draftBody(job, result string, extra map[string]string) string {
	var items []string

	for _, group := range maintenance.ComponentGroups {
		for _, key := range group.Keys {
			resultJSON := jsonNull
			if result != "" {
				resultJSON = `"` + result + `"`
			}

			items = append(
				items,
				`{"componentGroup":"`+group.Group+`","componentKey":"`+key+`","result":`+resultJSON+`,"reason":null,`+
					`"evidenceIds":[]}`,
			)
		}
	}

	bodyFields := map[string]string{
		fieldJobID:        `"` + job + `"`,
		fieldItems:        "[" + strings.Join(items, ",") + "]",
		fieldMeasurements: "[]",
		fieldParts:        "[]",
		fieldRefrigerant:  "[]",
		fieldWorkText: `"` + strings.Repeat(
			"w",
			50,
		) + `"`,
		fieldNextAction:    `{"kind":"none"}`,
		fieldAttachmentIDs: "[]",
	}

	for key, value := range extra {
		if value == "" {
			delete(bodyFields, key)

			continue
		}

		bodyFields[key] = value
	}

	parts := make([]string, 0, len(bodyFields))
	for k, v := range bodyFields {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWorkReports(t *testing.T) {
	srv := server(t)
	job := assignedNow(t, srv)
	write(srv, techInt(), "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2)

	now := clock().Format("2006-01-02T15:04:05Z")

	// first draft (all results null)
	code, resp := write(srv, techInt(), "jobs.saveDraft", draftBody(job, "", nil), 0)
	if code != 200 || asNum(t, data(resp)["version"]) != 1 ||
		len(asArr(t, data(resp)["items"])) != 18 {
		t.Fatalf("first draft: %d %v", code, resp)
	}

	rep := asStr(t, data(resp)["id"])
	firstFilter := asObj(t, asArr(t, data(resp)["items"])[0])

	if code, _ := write(srv, techInt(), "jobs.saveDraft", draftBody(job, "", nil), 0); code != 409 {
		t.Error("second draft without reportId")
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "", map[string]string{fieldReportID: `"` + rep + `"`}),
		2,
	); code != 409 {
		t.Error("stale report version")
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "", map[string]string{fieldReportID: `"` + uuid.NewString() + `"`}),
		1,
	); code != 404 {
		t.Error("unknown report")
	}

	bad := map[string]map[string]string{
		"duplicate key": {
			fieldItems: `[{"componentGroup":"indoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]},` +
				`{"componentGroup":"indoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]}]`,
		},
		"wrong group": {
			fieldItems: `[{"componentGroup":"outdoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]}]`,
		},
		"bad result": {
			fieldItems: `[{"componentGroup":"indoor","componentKey":"filter","result":"ok","reason":null,"evidenceIds":[]}]`,
		},
		"measurement key": {
			fieldItems: `[]`,
			fieldMeasurements: `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"°C",` +
				`"observedAt":"` + now + `"}]`,
		},
		"bad symbol": {
			fieldMeasurements: `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"F",` +
				`"observedAt":"` + now + `"}]`,
		},
		"part quantity": {
			fieldParts: `[{"name":"Filter","quantity":1000,"catalogCode":null,"source":"van_stock","lotSerial":null,` +
				`"replacesComponentKey":null,"oldPartDisposal":null,"receiptAttachmentId":null}]`,
		},
		"part source": {
			fieldParts: `[{"name":"Filter","quantity":1,"catalogCode":null,"source":"amazon","lotSerial":null,` +
				`"replacesComponentKey":null,"oldPartDisposal":null,"receiptAttachmentId":null}]`,
		},
		fieldRefrigerant: {
			fieldRefrigerant: `[{"refrigerant":"R22","cylinderId":"C1","recoveredKg":0,"chargedKg":0,"leakCheck":"pass",` +
				`"leakCheckMethod":null}]`,
		},
		"work text":        {fieldWorkText: `"` + strings.Repeat("w", 4001) + `"`},
		"next action":      {fieldNextAction: `{"kind":"later"}`},
		"none with note":   {fieldNextAction: `{"kind":"none","note":"x"}`},
		"other attachment": {fieldAttachmentIDs: `["` + uuid.NewString() + `"]`},
		"missing parts":    {fieldParts: ""},
		caseUnknownField:   {"author": `"me"`},
	}
	// the field each case is refused for
	want := map[string]string{
		"bad result":       refusedItemsInvalid,
		"bad symbol":       "measurements=error.invalid",
		"duplicate key":    refusedItemsInvalid,
		"measurement key":  "measurements=error.invalid",
		"missing parts":    "draft=error.required",
		"next action":      "nextAction=error.invalid",
		"none with note":   "nextAction=error.invalid",
		"other attachment": "attachmentIds=error.otherReportAttachment",
		"part quantity":    "parts=error.invalid",
		"part source":      "parts=error.invalid",
		fieldRefrigerant:   "refrigerant=error.invalid",
		caseUnknownField:   refusedMalformedInput,
		"work text":        "workText=error.length",
		"wrong group":      refusedItemsInvalid,
	}

	for name, e := range bad {
		e["reportId"] = `"` + rep + `"`
		code, m := write(srv, techInt(), "jobs.saveDraft", draftBody(job, "", e), 1)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(
			job,
			"",
			map[string]string{
				fieldReportID: `"` + rep + `"`,
				fieldMeasurements: `[{"id":"` + uuid.NewString() + `","componentKey":"filter","metric":"temperature","value":20,` +
					`"unit":"°C","observedAt":"` + now + `"}]`,
			},
		),
		1,
	); code != 404 {
		t.Error("unknown measurement id")
	}
	// second save: all normal, a mismatched measurement unit is stored as suspect; unchanged items keep id
	meas := `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"kPa",` +
		`"observedAt":"` + now + `"},{"componentKey":"compressor","metric":"vibration","value":null,"unit":"mm/s",` +
		`"observedAt":"` + now + `"}]`

	code, resp = write(srv, techInt(), "jobs.saveDraft", draftBody(job, "normal", map[string]string{
		fieldReportID:     `"` + rep + `"`,
		fieldMeasurements: meas,
		fieldParts: `[{"name":" Filter ","quantity":1,"catalogCode":null,"source":"van_stock","lotSerial":null,` +
			`"replacesComponentKey":"filter","oldPartDisposal":"returned","receiptAttachmentId":null}]`,
		fieldRefrigerant: `[{"refrigerant":"R32","cylinderId":"C-1","recoveredKg":0.2,"chargedKg":0.3,"leakCheck":"pass",` +
			`"leakCheckMethod":"soap"}]`,
	}), 1)
	if code != 200 || asNum(t, data(resp)["version"]) != 2 ||
		asObj(t, asArr(t, data(resp)["items"])[0])["id"] != firstFilter["id"] {
		t.Fatalf("second save: %d %v", code, resp)
	}

	ms := asArr(t, data(resp)["measurements"])
	if asObj(t, ms[0])["quality"] != qualitySuspect ||
		asObj(t, ms[1])["quality"] != "missing" ||
		asObj(t, ms[0])["sensorId"] != firstFilter["id"] {
		t.Fatalf("measurement quality: %v", ms)
	}

	if asObj(t, asArr(t, data(resp)["parts"])[0])["name"] != "Filter" {
		t.Error("part name trimmed")
	}
	// submit: unit mismatch and version checks
	if code, res := write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":2}`,
		3,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["measurements"] == nil {
		t.Fatalf("submit with unit mismatch: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":1}`,
		3,
	); code != 409 {
		t.Error("submit stale report version")
	}

	reasonItems := strings.Replace(
		draftBody(job, "normal", nil),
		`"componentKey":"filter","result":"normal","reason":null`,
		`"componentKey":"filter","result":"attention","reason":null`,
		1,
	)

	reasonItems = strings.Replace(reasonItems, "{", `{"reportId":"`+rep+`",`, 1)
	if code, _ := write(srv, techInt(), "jobs.saveDraft", reasonItems, 2); code != 200 {
		t.Fatal("save attention item")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":3}`,
		3,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["items"] == nil {
		t.Fatalf("attention without reason: %d %v", code, res)
	}

	write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(
			job,
			"normal",
			map[string]string{
				fieldReportID:   `"` + rep + `"`,
				fieldWorkText:   `"short"`,
				fieldNextAction: jsonNull,
			},
		),
		3,
	)

	if code, res := write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":4}`,
		3,
	); code != 422 ||
		len(asObj(t, res["fieldErrors"])) < 2 {
		t.Fatalf("short text and no next action: %d %v", code, res)
	}

	write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "normal", map[string]string{fieldReportID: `"` + rep + `"`}),
		4,
	)

	code, resp = write(srv, techInt(), "jobs.submit", `{"jobId":"`+job+`","reportVersion":5}`, 3)
	if code != 200 || data(resp)["status"] != "submitted" ||
		asObj(t, data(resp)["timeOnSite"])["finishedAt"] == nil ||
		len(asArr(t, data(resp)["reportRefs"])) != 1 {
		t.Fatalf("submit: %d %v", code, resp)
	}
	// reads
	getRep := func(a *actor, v int) (int, map[string]any) {
		return post(
			srv,
			a,
			"reports.get",
			`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(v)+`}`,
		)
	}
	if code, m := getRep(
		hq(),
		5,
	); code != 200 ||
		asObj(t, data(m)["reviewAvailability"])["allowed"] != true {
		t.Fatalf("hq report: %d %v", code, m)
	}

	if code, m := getRep(
		techInt(),
		5,
	); code != 200 ||
		asObj(t, data(m)["reviewAvailability"])["reason"] != "permission_denied" {
		t.Fatalf("technician report: %d %v", code, m)
	}
	// a reviewer never accepts a report they wrote (IR31): the same user through another membership is still its author
	var author string
	ownerScan(
		t,
		`SELECT author_id::text FROM maintenance.work_reports WHERE id = $1 AND version = 5`,
		[]any{rep},
		&author,
	)
	owner(
		t,
		`UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 5`,
		rep,
		seed.ID("user-hq-operator"),
	)

	if code, m := getRep(
		hq(),
		5,
	); code != 200 || asObj(t, data(m)["reviewAvailability"])["allowed"] != false ||
		asObj(t, data(m)["reviewAvailability"])["reason"] != "self_authored" {
		t.Errorf("self-authored report: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 5`,
		rep,
		author,
	)

	if code, _ := getRep(customerA(), 5); code != 404 {
		t.Error("client reads submitted (not accepted) report")
	}

	if code, _ := getRep(hq(), 9); code != 404 {
		t.Error("unknown version")
	}
	// review: escalation mode is for outsourced jobs; return needs a reason
	if code, _ := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":5,"decision":"accept","reviewMode":"hq_escalation","reason":"x"}`,
		4,
	); code != 403 {
		t.Error("hq_escalation on an internal job")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":5,"decision":"return","reviewMode":"normal"}`,
		4,
	); code != 422 ||
		fields(res) != refusedReasonRequired {
		t.Errorf("return without reason: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":4,"decision":"accept","reviewMode":"normal"}`,
		4,
	); code != 409 {
		t.Error("review an older version")
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":5,"decision":"return","reviewMode":"normal","reason":"photo missing"}`,
		4,
	); code != 200 ||
		data(res)["status"] != "rework_requested" {
		t.Fatalf("return: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "normal", map[string]string{fieldReportID: `"` + rep + `"`}),
		5,
	); code != 409 {
		t.Error("draft while rework requested")
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.resumeRework",
		`{"jobId":"`+job+`"}`,
		5,
	); code != 200 ||
		data(res)["status"] != statusInProgress ||
		asNum(t, asObj(t, data(res)["draftReportRef"])["reportVersion"]) != 6 {
		t.Fatalf("resumeRework: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"jobs.resumeRework",
		`{"jobId":"`+job+`"}`,
		6,
	); code != 409 {
		t.Error("resume twice")
	}

	if _, m := getRep(
		hq(),
		5,
	); len(asArr(t, data(m)["reviewHistory"])) != 1 ||
		asObj(t, data(m)["reviewAvailability"])["reason"] != "not_current" {
		t.Fatalf("history after return: %v", m)
	}

	write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(
			job,
			"normal",
			map[string]string{
				fieldReportID: `"` + rep + `"`,
				fieldWorkText: `"` + strings.Repeat("v", 60) + `"`,
			},
		),
		6,
	)
	write(srv, techInt(), "jobs.submit", `{"jobId":"`+job+`","reportVersion":7}`, 6)
	// another job at the same time: the technician is busy while the assignment is active (DEC-73 checks it after
	// completion)
	_, resp = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	other := asStr(t, data(resp)["id"])

	free := func() bool {
		_, res := post(
			srv,
			hq(),
			"members.eligible",
			`{"jobId":"`+other+`","startAt":"`+clock().Add(-30*time.Minute).
				Format(time.RFC3339)+
				`","endAt":"`+clock().Add(30*time.Minute).
				Format(time.RFC3339)+
				`","query":{"limit":50}}`,
		)
		for _, it := range items(t, res) {
			if it["id"] == seed.ID("tech-internal-a").String() {
				return true
			}
		}

		return false
	}
	if free() {
		t.Fatal("an active assignment books the technician")
	}
	// self review is forbidden even for HQ
	owner(
		t,
		`UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 7`,
		rep,
		seed.ID("user-hq-operator"),
	)

	if code, _ := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`,
		7,
	); code != 403 {
		t.Error("self review")
	}

	owner(
		t,
		`UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 7`,
		rep,
		seed.ID("user-tech-internal-a"),
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`,
		7,
	); code != 200 || data(res)["status"] != statusCompleted ||
		data(res)["completedAt"] == nil {
		t.Fatalf("accept: %d %v", code, res)
	}
	// DEC-73 / IR234: completion releases the assignment — the technician is free again, the detail still names who did
	// the work, the technician keeps the job as a history snapshot at once (no worker tick), writes end, and the
	// completion notification still reaches the technician
	var status, reason string
	ownerScan(
		t,
		`SELECT status, reason FROM maintenance.assignments WHERE job_id = $1 ORDER BY updated_at DESC LIMIT 1`,
		[]any{job},
		&status,
		&reason,
	)

	if status != statusCompleted || reason != "job_completed" {
		t.Fatalf("assignment after completion: %s / %s", status, reason)
	}

	if !free() {
		t.Error("a completed job's assignment still books the technician")
	}

	if code, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 || data(res)["assignment"] == nil ||
		asObj(t, data(res)["assignment"])["status"] != statusCompleted {
		t.Fatalf("the detail keeps the completed assignment: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.list",
		`{"filters":{"membershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","status":"completed"},"limit":100}`,
	); byJob(t, res, job) == nil ||
		byJob(t, res, job)["technicianMembershipId"] != seed.ID("tech-internal-a").String() {
		t.Errorf(
			"the assignee filter and the summary keep the completed job's technician: %v",
			byJob(t, res, job),
		)
	}

	if code, res := post(
		srv,
		techInt(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 || data(res)["projection"] != projectionHistory ||
		data(res)["status"] != statusCompleted {
		t.Fatalf("technician history right after completion: %d %v", code, res)
	}

	if code, res := write(
		srv,
		techInt(),
		"jobs.saveDraft",
		draftBody(job, "normal", map[string]string{fieldReportID: `"` + rep + `"`}),
		7,
	); code != 403 ||
		res["messageKey"] != keyAssignmentEnded {
		t.Errorf("a technician write after completion: %d %v", code, res)
	}

	var notified int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE recipient_membership_id = $1 AND template_key = 'completion' AND
			target->>'id' = $2`,
		[]any{seed.ID("tech-internal-a"), job},
		&notified,
	)

	if notified != 1 {
		t.Errorf("completion notification to the technician: %d", notified)
	}

	if code, m := getRep(customerA(), 7); code != 200 || data(m)["acceptedAt"] == nil {
		t.Fatalf("client reads the accepted report: %d %v", code, m)
	}

	if code, _ := getRep(customerB(), 7); code != 404 {
		t.Error("other client")
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`,
		8,
	); code != 409 {
		t.Error("review completed job")
	}

	_, resp = post(srv, hq(), "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)

	var refs int

	for _, e := range items(t, resp) {
		if e["reportRef"] != nil {
			refs++
		}
	}

	if refs != 4 { // submitted v5, reviewed v5, submitted v7, reviewed v7
		t.Errorf("report events: %d", refs)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestContractorReview(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		seed.ID("tech-external-a"),
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, res := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(res)["id"])
	write(
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
		)+`","termsVersion":"t"}`,
		1,
	)

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)
	write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	)

	if code, resp := write(
		srv,
		contrA(),
		"jobs.assign",
		`{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").
			String()+
			`","startAt":"`+stamp(
			49,
		)+`","endAt":"`+stamp(
			51,
		)+`"}`,
		3,
	); code != 200 {
		t.Fatalf("assign: %d %v", code, resp)
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3), created_at = $2
			WHERE job_id = $1 AND status = 'active'`,
		job,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)
	write(srv, techA(), "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 4)
	_, res = write(srv, techA(), "jobs.saveDraft", draftBody(job, "normal", nil), 0)

	rep := asStr(t, data(res)["id"])

	if code, resp := write(
		srv,
		techA(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":1}`,
		5,
	); code != 200 {
		t.Fatalf("submit: %d %v", code, resp)
	}

	if code, resp := post(
		srv,
		contrA(),
		"reports.get",
		`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":1}`,
	); code != 200 ||
		asObj(t, data(resp)["reviewAvailability"])["allowed"] != true {
		t.Fatalf("contractor reads submitted report: %d %v", code, resp)
	}

	if code, _ := post(
		srv,
		contrB(),
		"reports.get",
		`{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":1}`,
	); code != 404 {
		t.Error("other contractor reads report")
	}

	review := func(who *actor, mode, reason string) int {
		reasonField := ""
		if reason != "" {
			reasonField = `,"reason":"` + reason + `"`
		}

		code, _ := write(
			srv,
			who,
			"jobs.review",
			`{"jobId":"`+job+`","reportVersion":1,"decision":"accept","reviewMode":"`+mode+`"`+reasonField+`}`,
			6,
		)

		return code
	}
	if code := review(contrB(), "normal", ""); code != 404 {
		t.Errorf("other contractor review: %d", code)
	}

	if code, resp := write(
		srv,
		contrA(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":1,"decision":"accept","reviewMode":"hq_escalation","reason":"x"}`,
		6,
	); code != 422 ||
		fields(resp) != "reviewMode=error.invalid" {
		t.Errorf("contractor escalation mode: %d %s", code, fields(resp))
	}

	if code := review(hq(), "normal", ""); code != 403 {
		t.Errorf("HQ normal review of outsourced job: %d", code)
	}

	if code, resp := write(
		srv,
		hq(),
		"jobs.review",
		`{"jobId":"`+job+`","reportVersion":1,"decision":"accept","reviewMode":"hq_escalation"}`,
		6,
	); code != 422 ||
		fields(resp) != refusedReasonRequired {
		t.Errorf("escalation without reason: %d %s", code, fields(resp))
	}

	if code := review(contrA(), "normal", ""); code != 200 {
		t.Fatalf("contractor accepts: %d", code)
	}

	var status string
	ownerScan(
		t,
		`SELECT status FROM maintenance.assignments WHERE job_id = $1 ORDER BY updated_at DESC LIMIT 1`,
		[]any{job},
		&status,
	)

	if status != statusCompleted {
		t.Errorf("the contractor's acceptance releases the assignment too: %s", status)
	}

	if code, resp := post(
		srv,
		techA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 ||
		data(resp)["projection"] != projectionHistory {
		t.Errorf("external technician history after completion: %d %v", code, resp)
	}
}
