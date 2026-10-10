package integration

import (
	"context"
	"encoding/base64"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// completedJob creates a customer-a job completed one hour before the fixture clock.
func completedJob(t *testing.T, s *apiserver.Server) string {
	t.Helper()
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET status = 'completed', completed_at = $2 WHERE id = $1`, job, clock.Add(-time.Hour))
	return job
}

func TestRateAndReportProblem(t *testing.T) {
	s := server(t)
	job := completedJob(t, s)
	rate := func(a *actor, body string, v int) (int, map[string]any) {
		return write(s, a, "jobs.rate", `{"jobId":"`+job+`",`+body+`}`, v)
	}
	for name, b := range map[string]string{
		"stars 0":       `"stars":0,"tags":[]`,
		"stars 6":       `"stars":6,"tags":[]`,
		"unknown tag":   `"stars":4,"tags":["Fast"]`,
		"duplicate tag": `"stars":4,"tags":["Polite","Polite"]`,
		"long comment":  `"stars":4,"tags":[],"comment":"` + strings.Repeat("c", 1001) + `"`,
		"no tags":       `"stars":4`,
	} {
		if code, _ := rate(&customerA, b, 1); code != 422 {
			t.Errorf("rate %s: %d", name, code)
		}
	}
	if code, _ := rate(&customerB, `"stars":5,"tags":[]`, 1); code != 404 {
		t.Error("other customer rates")
	}
	code, m := rate(&customerA, `"stars":2,"tags":["On time"],"comment":"  slow  "`, 1)
	r, _ := data(m)["rating"].(map[string]any)
	if code != 200 || r["stars"].(float64) != 2 || r["comment"] != "slow" || data(m)["customerConfirmedAt"] == nil {
		t.Fatalf("rate: %d %v", code, m)
	}
	if code, m := rate(&customerA, `"stars":3,"tags":[]`, 2); code != 200 || data(m)["rating"].(map[string]any)["ratedAt"] != r["ratedAt"] {
		t.Fatalf("re-rate keeps ratedAt: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET rating = jsonb_set(rating, '{editableUntil}', to_jsonb($2::text)) WHERE id = $1`, job, clock.Format(time.RFC3339))
	if code, _ := rate(&customerA, `"stars":4,"tags":[]`, 3); code != 409 {
		t.Error("rating locked after 7 days")
	}
	// report a problem → follow-up job
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\ndemo")) // the PNG signature, then the demo bytes
	photo := `{"name":"leak.png","mime":"image/png","size":12,"bytes":"` + png + `"}`
	prob := func(body string, v int) (int, map[string]any) {
		return write(s, &customerA, "jobs.reportProblem", `{"jobId":"`+job+`",`+body+`}`, v)
	}
	for name, b := range map[string]string{
		"short details": `"reasonCode":"same_problem","details":"short","photos":[],"preferredSlot":null`,
		"bad code":      `"reasonCode":"angry","details":"still dripping water","photos":[],"preferredSlot":null`,
		"bad mime":      `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.gif","mime":"image/gif","size":12,"bytes":"` + png + `"}],"preferredSlot":null`,
		"png as jpeg":   `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.jpg","mime":"image/jpeg","size":12,"bytes":"` + png + `"}],"preferredSlot":null`,
		"size mismatch": `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.png","mime":"image/png","size":3,"bytes":"` + png + `"}],"preferredSlot":null`,
		"six photos":    `"reasonCode":"same_problem","details":"still dripping water","photos":[` + strings.Repeat(photo+",", 5) + photo + `],"preferredSlot":null`,
		"today slot":    `"reasonCode":"same_problem","details":"still dripping water","photos":[],"preferredSlot":` + slotJSON(2, 1),
	} {
		if code, _ := prob(b, 3); code != 422 {
			t.Errorf("problem %s: %d", name, code)
		}
	}
	code, m = prob(`"reasonCode":"same_problem","details":"Still dripping water after the visit","photos":[`+photo+`],"preferredSlot":`+slotJSON(49, 2), 3)
	if code != 200 || data(m)["followUpOfJobId"] != job || data(m)["followUpClass"] != "pending" || data(m)["status"] != "requested" || len(data(m)["preferredSlots"].([]any)) != 1 {
		t.Fatalf("reportProblem: %d %v", code, m)
	}
	follow := data(m)["id"].(string)
	var attachments int
	ownerScan(t, `SELECT count(*) FROM maintenance.attachments WHERE job_id = $1 AND status = 'ready'`, []any{follow}, &attachments)
	if attachments != 1 {
		t.Errorf("photos stored: %d", attachments)
	}
	if code, m := prob(`"reasonCode":"other","details":"Another issue with the louver","photos":[],"preferredSlot":null`, 4); code != 200 || len(data(m)["preferredSlots"].([]any)) != 0 {
		t.Fatalf("reportProblem without slot: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET completed_at = $2 WHERE id = $1`, job, clock.Add(-8*24*time.Hour))
	if code, _ := prob(`"reasonCode":"other","details":"Too late to report this","photos":[],"preferredSlot":null`, 5); code != 409 {
		t.Error("problem after 7 days")
	}
	// HQ classifies once
	if code, _ := write(s, &hq, "jobs.classifyFollowUp", `{"jobId":"`+follow+`","classification":"redo","reason":"x"}`, 1); code != 422 {
		t.Error("bad classification")
	}
	if code, m := write(s, &hq, "jobs.classifyFollowUp", `{"jobId":"`+follow+`","classification":"rework","reason":"same fault"}`, 1); code != 200 || data(m)["followUpClass"] != "rework" {
		t.Fatalf("classify: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "jobs.classifyFollowUp", `{"jobId":"`+follow+`","classification":"new_request","reason":"again"}`, 2); code != 409 {
		t.Error("classify twice")
	}
	if code, _ := write(s, &customerA, "jobs.classifyFollowUp", `{"jobId":"`+follow+`","classification":"rework","reason":"x"}`, 2); code != 403 {
		t.Error("client classifies")
	}
}

func TestCostsAccessWarrantyAndProjections(t *testing.T) {
	s := server(t)
	job := completedJob(t, s)
	lines := `[{"kind":"estimate","amountMinor":5000,"currency":"MYR","description":" Parts ","visibility":"customer"},{"kind":"actual","amountMinor":800,"currency":"MYR","description":"Margin","visibility":"internal"}]`
	if code, _ := write(s, &hq, "jobs.saveCost", `{"jobId":"`+job+`","costLines":[{"kind":"guess","amountMinor":1,"currency":"MYR","description":"x","visibility":"internal"}]}`, 1); code != 422 {
		t.Error("bad cost line")
	}
	if code, _ := write(s, &hq, "jobs.saveCost", `{"jobId":"`+job+`","costLines":[{"kind":"actual","amountMinor":-1,"currency":"EUR","description":"","visibility":"internal"}]}`, 1); code != 422 {
		t.Error("bad amount/currency")
	}
	if code, m := write(s, &hq, "jobs.saveCost", `{"jobId":"`+job+`","costLines":`+lines+`}`, 1); code != 200 || len(data(m)["costs"].([]any)) != 2 {
		t.Fatalf("saveCost: %d %v", code, m)
	}
	// IR42: clients see customer lines only
	if _, m := post(s, &customerA, "jobs.get", `{"jobId":"`+job+`"}`); len(data(m)["costs"].([]any)) != 1 || data(m)["costs"].([]any)[0].(map[string]any)["description"] != "Parts" {
		t.Fatalf("client costs: %v", data(m)["costs"])
	}
	// warranty claims need a warranty that covers completion
	claim := `{"jobId":"` + job + `","partLabel":"Compressor","amountMinor":12000,"currency":"MYR","reason":"failed in warranty"}`
	owner(t, `UPDATE assets.units SET warranty_ends_at = NULL WHERE id = $1`, seed.ID("unit-non-rto"))
	if code, _ := write(s, &hq, "jobs.recordWarrantyClaim", claim, 2); code != 422 {
		t.Error("no warranty")
	}
	owner(t, `UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`, seed.ID("unit-non-rto"), clock.Add(30*24*time.Hour))
	t.Cleanup(func() {
		owner(t, `UPDATE assets.units SET warranty_ends_at = NULL WHERE id = $1`, seed.ID("unit-non-rto"))
	})
	if code, _ := write(s, &hq, "jobs.recordWarrantyClaim", `{"jobId":"`+job+`","partLabel":"","amountMinor":0,"currency":"JPY","reason":""}`, 2); code != 422 {
		t.Error("bad claim input")
	}
	if code, m := write(s, &hq, "jobs.recordWarrantyClaim", claim, 2); code != 200 || data(m)["warrantyClaims"].([]any)[0].(map[string]any)["state"] != "filed" {
		t.Fatalf("claim: %d %v", code, m)
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'cancelled' WHERE id = $1`, job)
	if code, _ := write(s, &hq, "jobs.saveCost", `{"jobId":"`+job+`","costLines":[]}`, 3); code != 409 {
		t.Error("cost on cancelled job")
	}
	if code, _ := write(s, &hq, "jobs.recordWarrantyClaim", claim, 3); code != 409 {
		t.Error("claim on non-completed job")
	}

	// extend access of an accepted offer; JobDetail.offer per role
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	if code, _ := write(s, &hq, "jobs.extendAccess", `{"jobId":"`+j2+`","accessValidUntil":"`+ts(200)+`","reason":"x"}`, 1); code != 409 {
		t.Error("extend without accepted offer")
	}
	write(s, &hq, "jobs.offer", `{"jobId":"`+j2+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(-1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+j2+`"}`); data(m)["offer"] == nil || data(m)["offer"].(map[string]any)["decision"] != nil {
		t.Fatal("hq sees the open offer")
	}
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{j2}, &offer)
	write(s, &contrA, "jobs.accept", `{"jobId":"`+j2+`","offerId":"`+offer+`","termsVersion":"t"}`, 2)
	if code, _ := write(s, &hq, "jobs.extendAccess", `{"jobId":"`+j2+`","accessValidUntil":"`+ts(50)+`","reason":"x"}`, 3); code != 422 {
		t.Error("extension must be later")
	}
	if code, _ := write(s, &hq, "jobs.extendAccess", `{"jobId":"`+j2+`","accessValidUntil":"`+ts(200)+`","reason":" "}`, 3); code != 422 {
		t.Error("extension reason")
	}
	if code, m := write(s, &hq, "jobs.extendAccess", `{"jobId":"`+j2+`","accessValidUntil":"`+ts(200)+`","reason":"parts delayed"}`, 3); code != 200 ||
		data(m)["offer"].(map[string]any)["accessValidUntil"] != ts(200) {
		t.Fatalf("extend: %d %v", code, m)
	}
	if _, m := post(s, &contrA, "jobs.get", `{"jobId":"`+j2+`"}`); data(m)["offer"] == nil || len(data(m)["costs"].([]any)) != 0 {
		t.Fatalf("contractor detail: %v", m)
	}
	if _, m := post(s, &customerA, "jobs.get", `{"jobId":"`+j2+`"}`); data(m)["offer"] != nil {
		t.Fatal("client sees the offer")
	}
}

func TestWorkerConfirmsUnratedJobs(t *testing.T) {
	s := server(t)
	job := completedJob(t, s)
	owner(t, `UPDATE maintenance.jobs SET completed_at = $2 WHERE id = $1`, job, clock.Add(-7*24*time.Hour))
	if _, err := schedTick(context.Background(), s, clock); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &customerA, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["customerConfirmedAt"] == nil || data(m)["rating"] != nil {
		t.Fatalf("auto confirm: %v", m)
	}
}
