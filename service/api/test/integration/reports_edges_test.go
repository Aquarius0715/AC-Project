package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// The less common paths of a work report (IR100, SR07): a draft or submission of a job that does not exist, an item that
// cites an attachment of another report, an empty reason kept as none, a measurement corrected in place, a submission of
// a stale job version or of a job not in progress, the time on site without the ended pauses, and the reads of a report
// through another job.
func TestWorkReportEdges(t *testing.T) {
	s := server(t)
	job := assignedNow(t, s) // version 2
	now := clock.Format(time.RFC3339)
	if code, m := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":1}`, 2); code != 409 || m["messageKey"] != "error.invalidState" {
		t.Errorf("submit before the start: %d %v", code, m)
	}
	if code, m := write(s, &techInt, "jobs.start", `{"jobId":"`+job+`","startConfirmed":true}`, 2); code != 200 {
		t.Fatalf("start: %d %v", code, m)
	}
	unknown := uuid.NewString()
	if code, _ := write(s, &techInt, "jobs.saveDraft", draftBody(unknown, "", nil), 0); code != 404 {
		t.Errorf("draft of an unknown job: %d", code)
	}
	if code, _ := write(s, &techInt, "jobs.submit", `{"jobId":"`+unknown+`","reportVersion":1}`, 1); code != 404 {
		t.Errorf("submit of an unknown job: %d", code)
	}
	if code, m := write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":1}`, 2); code != 409 || m["messageKey"] != "error.versionConflict" {
		t.Errorf("submit of a stale job version: %d %v", code, m)
	}

	code, m := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", nil), 0)
	if code != 200 {
		t.Fatalf("first draft: %d %v", code, m)
	}
	rep := data(m)["id"].(string)
	elsewhere := `[{"componentGroup":"indoor","componentKey":"filter","result":null,"reason":null,"evidenceIds":["` + uuid.NewString() + `"]}]`
	if code, m := write(s, &techInt, "jobs.saveDraft", draftBody(job, "", map[string]string{"reportId": `"` + rep + `"`, "items": elsewhere}), 1); code != 422 ||
		m["fieldErrors"].(map[string]any)["items"] != "error.otherReportAttachment" {
		t.Errorf("an item citing another report's attachment: %d %v", code, m)
	}
	// an empty reason is no reason; a measurement saved again by its ID keeps it and counts a version
	body := strings.Replace(draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`,
		"measurements": `[{"componentKey":"filter","metric":"temperature","value":20,"unit":"°C","observedAt":"` + now + `"}]`}),
		`"componentKey":"filter","result":"normal","reason":null`, `"componentKey":"filter","result":"normal","reason":""`, 1)
	code, m = write(s, &techInt, "jobs.saveDraft", body, 1)
	if code != 200 {
		t.Fatalf("second draft: %d %v", code, m)
	}
	for _, it := range data(m)["items"].([]any) {
		if it := it.(map[string]any); it["componentKey"] == "filter" && it["reason"] != nil {
			t.Errorf("empty reason kept: %v", it)
		}
	}
	first := data(m)["measurements"].([]any)[0].(map[string]any)
	code, m = write(s, &techInt, "jobs.saveDraft", draftBody(job, "normal", map[string]string{"reportId": `"` + rep + `"`,
		"measurements": `[{"id":"` + first["id"].(string) + `","componentKey":"filter","metric":"temperature","value":21,"unit":"°C","observedAt":"` + now + `"}]`}), 2)
	if code != 200 {
		t.Fatalf("third draft: %d %v", code, m)
	}
	if again := data(m)["measurements"].([]any)[0].(map[string]any); again["id"] != first["id"] || again["version"].(float64) != 2 || again["value"].(float64) != 21 || again["createdAt"] != first["createdAt"] {
		t.Errorf("measurement corrected in place: %v → %v", first, again)
	}

	// on site from 60 minutes ago with a 10-minute pause that ended: 50 minutes
	owner(t, `UPDATE maintenance.jobs SET time_on_site = jsonb_build_object('arrivedAt', $2::timestamptz, 'startedAt', $2::timestamptz, 'pauses',
		jsonb_build_array(jsonb_build_object('from', $3::timestamptz, 'to', $4::timestamptz))) WHERE id = $1`,
		job, clock.Add(-time.Hour), clock.Add(-40*time.Minute), clock.Add(-30*time.Minute))
	code, m = write(s, &techInt, "jobs.submit", `{"jobId":"`+job+`","reportVersion":3}`, 3)
	if code != 200 {
		t.Fatalf("submit: %d %v", code, m)
	}
	if tos := data(m)["timeOnSite"].(map[string]any); tos["onSiteMinutes"].(float64) != 50 {
		t.Errorf("time on site without the pause: %v", tos)
	}

	// the report is read through its own job only; reviewing a job that does not exist says so
	if code, _ := post(s, &hq, "reports.get", `{"jobId":"`+unknown+`","reportId":"`+rep+`","reportVersion":3}`); code != 404 {
		t.Errorf("report through an unknown job: %d", code)
	}
	other := assignedNow(t, s)
	if code, _ := post(s, &hq, "reports.get", `{"jobId":"`+other+`","reportId":"`+rep+`","reportVersion":3}`); code != 404 {
		t.Errorf("report through another job: %d", code)
	}
	if code, _ := write(s, &hq, "jobs.review", `{"jobId":"`+unknown+`","reportVersion":3,"decision":"accept","reviewMode":"normal"}`, 1); code != 404 {
		t.Errorf("review of an unknown job: %d", code)
	}
}
