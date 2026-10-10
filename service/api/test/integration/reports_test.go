package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// draftBody builds a WorkReportDraft with all 18 components (result as given) and overrides.
func draftBody(job, result string, extra map[string]string) string {
	var items []string
	for _, g := range maintenance.ComponentGroups {
		for _, k := range g.Keys {
			r := `null`
			if result != "" {
				r = `"` + result + `"`
			}
			items = append(items, `{"componentGroup":"`+g.Group+`","componentKey":"`+k+`","result":`+r+`,"reason":null,"evidenceIds":[]}`)
		}
	}
	f := map[string]string{"jobId": `"` + job + `"`, "items": "[" + strings.Join(items, ",") + "]", "measurements": "[]", "parts": "[]", "refrigerant": "[]",
		"workText": `"` + strings.Repeat("w", 50) + `"`, "nextAction": `{"kind":"none"}`, "attachmentIds": "[]"}
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

func TestWorkReports(t *testing.T) {
	s := server(t)
	job := assignedNow(t, s)
	write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2)
	now := clock.Format("2006-01-02T15:04:05Z")

	// first draft (all results null)
	code, m := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", nil), 0)
	if code != 200 || data(m)["version"].(float64) != 1 || len(data(m)["items"].([]any)) != 18 {
		t.Fatalf("first draft: %d %v", code, m)
	}
	rep := data(m)["id"].(string)
	firstFilter := data(m)["items"].([]any)[0].(map[string]any)
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", nil), 0); code != 409 {
		t.Error("second draft without reportId")
	}
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", map[string]string{"reportId": `"` + rep + `"`}), 2); code != 409 {
		t.Error("stale report version")
	}
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", map[string]string{"reportId": `"` + uuid.NewString() + `"`}), 1); code != 404 {
		t.Error("unknown report")
	}
	bad := map[string]map[string]string{
		"duplicate key":    {"items": `[{"componentGroup":"indoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]},{"componentGroup":"indoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]}]`},
		"wrong group":      {"items": `[{"componentGroup":"outdoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":[]}]`},
		"bad result":       {"items": `[{"componentGroup":"indoor","componentKey":"filter","result":"ok","reason":null,"evidenceIds":[]}]`},
		"measurement key":  {"items": `[]`, "measurements": `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"°C","observedAt":"` + now + `"}]`},
		"bad symbol":       {"measurements": `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"F","observedAt":"` + now + `"}]`},
		"part quantity":    {"parts": `[{"name":"Filter","quantity":1000,"catalogCode":null,"source":"van_stock","lotSerial":null,"replacesComponentKey":null,"oldPartDisposal":null,"receiptAttachmentId":null}]`},
		"part source":      {"parts": `[{"name":"Filter","quantity":1,"catalogCode":null,"source":"amazon","lotSerial":null,"replacesComponentKey":null,"oldPartDisposal":null,"receiptAttachmentId":null}]`},
		"refrigerant":      {"refrigerant": `[{"refrigerant":"R22","cylinderId":"C1","recoveredKg":0,"chargedKg":0,"leakCheck":"pass","leakCheckMethod":null}]`},
		"work text":        {"workText": `"` + strings.Repeat("w", 4001) + `"`},
		"next action":      {"nextAction": `{"kind":"later"}`},
		"none with note":   {"nextAction": `{"kind":"none","note":"x"}`},
		"other attachment": {"attachmentIds": `["` + uuid.NewString() + `"]`},
		"missing parts":    {"parts": ""},
		"unknown field":    {"author": `"me"`},
	}
	for name, e := range bad {
		e["reportId"] = `"` + rep + `"`
		if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", e), 1); code != 422 {
			t.Errorf("draft %s: %d", name, code)
		}
	}
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", map[string]string{"reportId": `"` + rep + `"`, "measurements": `[{"id":"` + uuid.NewString() + `","componentKey":"filter","metric":"temperature","value":20,"unit":"°C","observedAt":"` + now + `"}]`}), 1); code != 404 {
		t.Error("unknown measurement id")
	}
	// second save: all normal, a mismatched measurement unit is stored as suspect; unchanged items keep id
	meas := `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"kPa","observedAt":"` + now + `"},{"componentKey":"compressor","metric":"vibration","value":null,"unit":"mm/s","observedAt":"` + now + `"}]`
	code, m = write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`, "measurements": meas,
		"parts":       `[{"name":" Filter ","quantity":1,"catalogCode":null,"source":"van_stock","lotSerial":null,"replacesComponentKey":"filter","oldPartDisposal":"returned","receiptAttachmentId":null}]`,
		"refrigerant": `[{"refrigerant":"R32","cylinderId":"C-1","recoveredKg":0.2,"chargedKg":0.3,"leakCheck":"pass","leakCheckMethod":"soap"}]`}), 1)
	if code != 200 || data(m)["version"].(float64) != 2 || data(m)["items"].([]any)[0].(map[string]any)["id"] != firstFilter["id"] {
		t.Fatalf("second save: %d %v", code, m)
	}
	ms := data(m)["measurements"].([]any)
	if ms[0].(map[string]any)["quality"] != "suspect" || ms[1].(map[string]any)["quality"] != "missing" || ms[0].(map[string]any)["sensorId"] != firstFilter["id"] {
		t.Fatalf("measurement quality: %v", ms)
	}
	if data(m)["parts"].([]any)[0].(map[string]any)["name"] != "Filter" {
		t.Error("part name trimmed")
	}
	// submit: unit mismatch and version checks
	if code, m := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":2}`, 3); code != 422 || m["fieldErrors"].(map[string]any)["measurements"] == nil {
		t.Fatalf("submit with unit mismatch: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":1}`, 3); code != 409 {
		t.Error("submit stale report version")
	}
	reasonItems := strings.Replace(draftBody(job, "normal", nil), `"componentKey":"filter","result":"normal","reason":null`, `"componentKey":"filter","result":"attention","reason":null`, 1)
	reasonItems = strings.Replace(reasonItems, "{", `{"reportId":"`+rep+`",`, 1)
	if code, _ := write(s, &techInt, "jobs.saveDraft", reasonItems, 2); code != 200 {
		t.Fatal("save attention item")
	}
	if code, m := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":3}`, 3); code != 422 || m["fieldErrors"].(map[string]any)["items"] == nil {
		t.Fatalf("attention without reason: %d %v", code, m)
	}
	write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`, "workText": `"short"`, "nextAction": "null"}), 3)
	if code, m := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":4}`, 3); code != 422 || len(m["fieldErrors"].(map[string]any)) < 2 {
		t.Fatalf("short text and no next action: %d %v", code, m)
	}
	write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`}), 4)
	code, m = write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":5}`, 3)
	if code != 200 || data(m)["status"] != "submitted" || data(m)["timeOnSite"].(map[string]any)["finishedAt"] == nil || len(data(m)["reportRefs"].([]any)) != 1 {
		t.Fatalf("submit: %d %v", code, m)
	}
	// reads
	getRep := func(a *actor, v int) (int, map[string]any) {
		return post(s, a, "reports.get", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":`+itoa(v)+`}`)
	}
	if code, m := getRep(&hq, 5); code != 200 || data(m)["reviewAvailability"].(map[string]any)["allowed"] != true {
		t.Fatalf("hq report: %d %v", code, m)
	}
	if code, m := getRep(&techInt, 5); code != 200 || data(m)["reviewAvailability"].(map[string]any)["reason"] != "permission_denied" {
		t.Fatalf("technician report: %d %v", code, m)
	}
	// a reviewer never accepts a report they wrote (IR31): the same user through another membership is still its author
	var author string
	ownerScan(t, `SELECT author_id::text FROM maintenance.work_reports WHERE id = $1 AND version = 5`, []any{rep}, &author)
	owner(t, `UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 5`, rep, seed.ID("user-hq-operator"))
	if code, m := getRep(&hq, 5); code != 200 || data(m)["reviewAvailability"].(map[string]any)["allowed"] != false || data(m)["reviewAvailability"].(map[string]any)["reason"] != "self_authored" {
		t.Errorf("self-authored report: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 5`, rep, author)
	if code, _ := getRep(&customerA, 5); code != 404 {
		t.Error("client reads submitted (not accepted) report")
	}
	if code, _ := getRep(&hq, 9); code != 404 {
		t.Error("unknown version")
	}
	// review: escalation mode is for outsourced jobs; return needs a reason
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":5,"decision":"accept","reviewMode":"hq_escalation","reason":"x"}`, 4); code != 403 {
		t.Error("hq_escalation on an internal job")
	}
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":5,"decision":"return","reviewMode":"normal"}`, 4); code != 422 {
		t.Error("return without reason")
	}
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":4,"decision":"accept","reviewMode":"normal"}`, 4); code != 409 {
		t.Error("review an older version")
	}
	if code, m := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":5,"decision":"return","reviewMode":"normal","reason":"photo missing"}`, 4); code != 200 || data(m)["status"] != "rework_requested" {
		t.Fatalf("return: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`}), 5); code != 409 {
		t.Error("draft while rework requested")
	}
	if code, m := write(s, &techInt, "jobs.resumeRework", `{"jobId":"`+job+`"}`, 5); code != 200 || data(m)["status"] != "in_progress" ||
		data(m)["draftReportRef"].(map[string]any)["reportVersion"].(float64) != 6 {
		t.Fatalf("resumeRework: %d %v", code, m)
	}
	if code, _ := write(s, &techInt, "jobs.resumeRework", `{"jobId":"`+job+`"}`, 6); code != 409 {
		t.Error("resume twice")
	}
	if _, m := getRep(&hq, 5); len(data(m)["reviewHistory"].([]any)) != 1 || data(m)["reviewAvailability"].(map[string]any)["reason"] != "not_current" {
		t.Fatalf("history after return: %v", m)
	}
	write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`, "workText": `"` + strings.Repeat("v", 60) + `"`}), 6)
	write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":7}`, 6)
	// another job at the same time: the technician is busy while the assignment is active (DEC-73 checks it after completion)
	_, m = write(s, &hq, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), map[string]string{"alternativeSlots": "[]"}), 0)
	other := data(m)["id"].(string)
	free := func() bool {
		_, m := post(s, &hq, "members.eligible", `{"jobId":"`+other+`","startAt":"`+clock.Add(-30*time.Minute).Format(time.RFC3339)+`","endAt":"`+clock.Add(30*time.Minute).Format(time.RFC3339)+`","query":{"limit":50}}`)
		for _, it := range items(m) {
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
	owner(t, `UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 7`, rep, seed.ID("user-hq-operator"))
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`, 7); code != 403 {
		t.Error("self review")
	}
	owner(t, `UPDATE maintenance.work_reports SET author_id = $2 WHERE id = $1 AND version = 7`, rep, seed.ID("user-tech-internal-a"))
	if code, m := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`, 7); code != 200 || data(m)["status"] != "completed" || data(m)["completedAt"] == nil {
		t.Fatalf("accept: %d %v", code, m)
	}
	// DEC-73 / IR234: completion releases the assignment — the technician is free again, the detail still names who did
	// the work, the technician keeps the job as a history snapshot at once (no worker tick), writes end, and the
	// completion notification still reaches the technician
	var status, reason string
	ownerScan(t, `SELECT status, reason FROM maintenance.assignments WHERE job_id = $1 ORDER BY updated_at DESC LIMIT 1`, []any{job}, &status, &reason)
	if status != "completed" || reason != "job_completed" {
		t.Fatalf("assignment after completion: %s / %s", status, reason)
	}
	if !free() {
		t.Error("a completed job's assignment still books the technician")
	}
	if code, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["assignment"] == nil || data(m)["assignment"].(map[string]any)["status"] != "completed" {
		t.Fatalf("the detail keeps the completed assignment: %d %v", code, m)
	}
	if _, m := post(s, &hq, "jobs.list", `{"filters":{"membershipId":"`+seed.ID("tech-internal-a").String()+`","status":"completed"},"limit":100}`); byJob(m, job) == nil || byJob(m, job)["technicianMembershipId"] != seed.ID("tech-internal-a").String() {
		t.Errorf("the assignee filter and the summary keep the completed job's technician: %v", byJob(m, job))
	}
	if code, m := post(s, &techInt, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["projection"] != "history" || data(m)["status"] != "completed" {
		t.Fatalf("technician history right after completion: %d %v", code, m)
	}
	if code, m := write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`}), 7); code != 403 || m["messageKey"] != "errors.assignment_ended" {
		t.Errorf("a technician write after completion: %d %v", code, m)
	}
	var notified int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE recipient_membership_id = $1 AND template_key = 'completion' AND target->>'id' = $2`, []any{seed.ID("tech-internal-a"), job}, &notified)
	if notified != 1 {
		t.Errorf("completion notification to the technician: %d", notified)
	}
	if code, m := getRep(&customerA, 7); code != 200 || data(m)["acceptedAt"] == nil {
		t.Fatalf("client reads the accepted report: %d %v", code, m)
	}
	if code, _ := getRep(&customerB, 7); code != 404 {
		t.Error("other client")
	}
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+job+`","reportVersion":7,"decision":"accept","reviewMode":"normal"}`, 8); code != 409 {
		t.Error("review completed job")
	}
	_, m = post(s, &hq, "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
	var refs int
	for _, e := range items(m) {
		if e["reportRef"] != nil {
			refs++
		}
	}
	if refs != 4 { // submitted v5, reviewed v5, submitted v7, reviewed v7
		t.Errorf("report events: %d", refs)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestContractorReview(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, seed.ID("tech-external-a"))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(0)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)
	write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`, 2)
	if code, m := write(s, &contrA, "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+seed.ID("tech-external-a").String()+`","startAt":"`+ts(49)+`","endAt":"`+ts(51)+`"}`, 3); code != 200 {
		t.Fatalf("assign: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3), created_at = $2 WHERE job_id = $1 AND status = 'active'`,
		job, clock.Add(-time.Hour), clock.Add(time.Hour))
	write(s, &techA, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 4)
	_, m = write(s, &techA, "jobs.saveDraft", draftBody(job, "normal", nil), 0)
	rep := data(m)["id"].(string)
	if code, m := write(s, &techA, "jobs.submit", `{"jobId":"`+job+`","reportVersion":1}`, 5); code != 200 {
		t.Fatalf("submit: %d %v", code, m)
	}
	if code, m := post(s, &contrA, "reports.get", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":1}`); code != 200 || data(m)["reviewAvailability"].(map[string]any)["allowed"] != true {
		t.Fatalf("contractor reads submitted report: %d %v", code, m)
	}
	if code, _ := post(s, &contrB, "reports.get", `{"jobId":"`+job+`","reportId":"`+rep+`","reportVersion":1}`); code != 404 {
		t.Error("other contractor reads report")
	}
	review := func(a *actor, mode, reason string) int {
		r := ""
		if reason != "" {
			r = `,"reason":"` + reason + `"`
		}
		code, _ := write(s, a, "jobs.review", `{"jobId":"`+job+`","reportVersion":1,"decision":"accept","reviewMode":"`+mode+`"`+r+`}`, 6)
		return code
	}
	if code := review(&contrB, "normal", ""); code != 404 {
		t.Errorf("other contractor review: %d", code)
	}
	if code := review(&contrA, "hq_escalation", "x"); code != 422 {
		t.Errorf("contractor escalation mode: %d", code)
	}
	if code := review(&hq, "normal", ""); code != 403 {
		t.Errorf("HQ normal review of outsourced job: %d", code)
	}
	if code := review(&hq, "hq_escalation", ""); code != 422 {
		t.Errorf("escalation without reason: %d", code)
	}
	if code := review(&contrA, "normal", ""); code != 200 {
		t.Fatalf("contractor accepts: %d", code)
	}
	var status string
	ownerScan(t, `SELECT status FROM maintenance.assignments WHERE job_id = $1 ORDER BY updated_at DESC LIMIT 1`, []any{job}, &status)
	if status != "completed" {
		t.Errorf("the contractor's acceptance releases the assignment too: %s", status)
	}
	if code, m := post(s, &techA, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["projection"] != "history" {
		t.Errorf("external technician history after completion: %d %v", code, m)
	}
}
