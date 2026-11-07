package integration_test

import (
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const refusedPhotosInvalid = "photos=" + apperr.KeyInvalidFile

// completedJob creates a customer-a job completed one hour before the fixture clock.
func completedJob(t *testing.T, s *apiserver.Server) string {
	t.Helper()

	_, m := write(s, customerA(), "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := asStr(t, data(m)["id"])
	owner(
		t,
		`UPDATE maintenance.jobs SET status = 'completed', completed_at = $2 WHERE id = $1`,
		job,
		clock().Add(-time.Hour),
	)

	return job
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRateAndReportProblem(t *testing.T) {
	srv := server(t)
	job := completedJob(t, srv)
	rate := func(a *actor, body string, v int) (int, map[string]any) {
		return write(srv, a, "jobs.rate", `{"jobId":"`+job+`",`+body+`}`, v)
	}
	want := map[string]string{ // the field each case is refused for
		"duplicate tag": "tags=error.invalid",
		"long comment":  "comment=error.length",
		"no tags":       "tags=error.required",
		"stars 0":       "stars=error.range",
		"stars 6":       "stars=error.range",
		"unknown tag":   "tags=error.invalid",
	}

	for name, body := range map[string]string{
		"stars 0":       `"stars":0,"tags":[]`,
		"stars 6":       `"stars":6,"tags":[]`,
		"unknown tag":   `"stars":4,"tags":["Fast"]`,
		"duplicate tag": `"stars":4,"tags":["Polite","Polite"]`,
		"long comment":  `"stars":4,"tags":[],"comment":"` + strings.Repeat("c", 1001) + `"`,
		"no tags":       `"stars":4`,
	} {
		code, m := rate(customerA(), body, 1)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := rate(customerB(), `"stars":5,"tags":[]`, 1); code != 404 {
		t.Error("other customer rates")
	}

	code, resp := rate(customerA(), `"stars":2,"tags":["On time"],"comment":"  slow  "`, 1)

	rating, _ := data(resp)["rating"].(map[string]any)
	if code != 200 || asNum(t, rating["stars"]) != 2 || rating["comment"] != "slow" ||
		data(resp)["customerConfirmedAt"] == nil {
		t.Fatalf("rate: %d %v", code, resp)
	}

	if code, res := rate(
		customerA(),
		`"stars":3,"tags":[]`,
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("re-rate a stale version: %d %v", code, res)
	}

	if code, res := rate(
		customerA(),
		`"stars":3,"tags":[],"comment":"   "`,
		2,
	); code != 200 ||
		asObj(t, data(res)["rating"])["ratedAt"] != rating["ratedAt"] ||
		asObj(t, data(res)["rating"])["comment"] != nil {
		t.Fatalf("re-rate keeps ratedAt, a blank comment is none: %d %v", code, res)
	}

	open := completedJob(t, srv)
	owner(
		t,
		`UPDATE maintenance.jobs SET status = 'in_progress', completed_at = NULL WHERE id = $1`,
		open,
	)

	if code, res := write(
		srv,
		customerA(),
		"jobs.rate",
		`{"jobId":"`+open+`","stars":5,"tags":[]}`,
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyInvalidState {
		t.Errorf("rate a job not completed: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE maintenance.jobs SET rating = jsonb_set(rating, '{editableUntil}', to_jsonb($2::text)) WHERE id = $1`,
		job,
		clock().Format(time.RFC3339),
	)

	if code, _ := rate(customerA(), `"stars":4,"tags":[]`, 3); code != 409 {
		t.Error("rating locked after 7 days")
	}
	// report a problem → follow-up job
	img, flood := tinyPNG(t), pngHeader(50000, 50000)
	png, size := base64.StdEncoding.EncodeToString(img), strconv.Itoa(len(img))
	photo := `{"name":"leak.png","mime":"image/png","size":` + size + `,"bytes":"` + png + `"}`
	prob := func(body string, v int) (int, map[string]any) {
		return write(srv, customerA(), "jobs.reportProblem", `{"jobId":"`+job+`",`+body+`}`, v)
	}
	want = map[string]string{ // the field each case is refused for
		caseBadCode:      "reasonCode=error.invalid",
		"bad mime":       refusedPhotosInvalid,
		casePNGAsJPEG:    refusedPhotosInvalid,
		"short details":  "details=error.length",
		"six photos":     "photos=error.count",
		caseSizeMismatch: refusedPhotosInvalid,
		casePixelFlood:   refusedPhotosInvalid,
		"today slot":     "preferredSlot=error.slotRules",
	}

	for name, body := range map[string]string{
		"short details": `"reasonCode":"same_problem","details":"short","photos":[],"preferredSlot":null`,
		caseBadCode:     `"reasonCode":"angry","details":"still dripping water","photos":[],"preferredSlot":null`,
		"bad mime": `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.gif",` +
			`"mime":"image/gif","size":` + size + `,"bytes":"` + png + `"}],"preferredSlot":null`,
		casePNGAsJPEG: `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.jpg",` +
			`"mime":"image/jpeg","size":` + size + `,"bytes":"` + png + `"}],"preferredSlot":null`,
		caseSizeMismatch: `"reasonCode":"same_problem","details":"still dripping water","photos":[{"name":"a.png",` +
			`"mime":"image/png","size":3,"bytes":"` + png + `"}],"preferredSlot":null`,
		casePixelFlood: `"reasonCode":"same_problem","details":"still dripping water",` +
			`"photos":[` + blobJSON("flood.png", "image/png", flood, len(flood)) + `],"preferredSlot":null`,
		"six photos": `"reasonCode":"same_problem","details":"still dripping water",` +
			`"photos":[` + strings.Repeat(photo+",", 5) + photo + `],"preferredSlot":null`,
		"today slot": `"reasonCode":"same_problem","details":"still dripping water","photos":[],` +
			`"preferredSlot":` + slotJSON(2, 1),
	} {
		code, m := prob(body, 3)
		refusal(t, name, code, m, want[name])
	}

	code, resp = prob(
		`"reasonCode":"same_problem","details":"Still dripping water after the visit",`+
			`"photos":[`+photo+`],"preferredSlot":`+slotJSON(
			49,
			2,
		),
		3,
	)
	if code != 200 || data(resp)["followUpOfJobId"] != job ||
		data(resp)["followUpClass"] != statusPending ||
		data(resp)["status"] != statusRequested ||
		len(asArr(t, data(resp)["preferredSlots"])) != 1 {
		t.Fatalf("reportProblem: %d %v", code, resp)
	}

	follow := asStr(t, data(resp)["id"])

	var attachments int
	ownerScan(
		t,
		`SELECT count(*) FROM maintenance.attachments WHERE job_id = $1 AND status = 'ready'`,
		[]any{follow},
		&attachments,
	)

	if attachments != 1 {
		t.Errorf("photos stored: %d", attachments)
	}
	// HQ hears of the follow-up as of any new request: it has 1 business day to classify it (IR114)
	if n := countNotes(t, follow, "hq-operator", "job_update"); n != 1 {
		t.Errorf("HQ notifications of the follow-up: %d", n)
	}
	// the photo is listed with the follow-up and opens for HQ and the customer (IR437)
	_, g := post(srv, hq(), "jobs.get", `{"jobId":"`+follow+`"}`)

	photos, _ := data(g)["problemPhotos"].([]any)
	if len(photos) != 1 || asObj(t, photos[0])["name"] != "leak.png" {
		t.Fatalf("problemPhotos: %v", data(g)["problemPhotos"])
	}

	pid := asStr(t, asObj(t, photos[0])["id"])
	for _, a := range []*actor{hq(), customerA()} {
		if code, res := post(
			srv,
			a,
			"jobs.getAttachment",
			`{"jobId":"`+follow+`","attachmentId":"`+pid+`"}`,
		); code != 200 || data(res)["mime"] != mimePNG ||
			data(res)["bytes"] != png {
			t.Errorf("jobs.getAttachment: %d %v", code, res)
		}
	}

	if code, _ := post(
		srv,
		customerB(),
		"jobs.getAttachment",
		`{"jobId":"`+follow+`","attachmentId":"`+pid+`"}`,
	); code != 404 {
		t.Errorf("another customer opens the photo: %d", code)
	}

	if code, m := prob(
		`"reasonCode":"other","details":"Another issue with the louver","photos":[],"preferredSlot":null`,
		4,
	); code != 200 ||
		len(asArr(t, data(m)["preferredSlots"])) != 0 {
		t.Fatalf("reportProblem without slot: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE maintenance.jobs SET completed_at = $2 WHERE id = $1`,
		job,
		clock().Add(-8*24*time.Hour),
	)

	if code, _ := prob(
		`"reasonCode":"other","details":"Too late to report this","photos":[],"preferredSlot":null`,
		5,
	); code != 409 {
		t.Error("problem after 7 days")
	}
	// the list rows name the follow-up, its class and when it was created: HQ's classify-by time (IR114, IR338), and
	// the client's own row
	summary := func(who *actor) map[string]any { // a follow-up stays requested (IR114); later tests' jobs may fill pages
		t.Helper()

		cursor := ""
		for range 50 {
			_, res := post(
				srv,
				who,
				"jobs.list",
				`{"filters":{"unitId":"`+seed.ID("unit-non-rto").
					String()+
					`","status":"requested"},"limit":100`+cursor+`}`,
			)
			for _, it := range items(t, res) {
				if it["id"] == follow {
					return it
				}
			}

			next, _ := data(res)["nextCursor"].(string)
			if next == "" {
				break
			}

			cursor = `,"cursor":"` + next + `"`
		}

		t.Fatal("follow-up not listed")

		return nil
	}

	var created time.Time
	ownerScan(t, `SELECT created_at FROM maintenance.jobs WHERE id = $1`, []any{follow}, &created)

	for name, a := range map[string]*actor{"hq": hq(), roleClient: customerA()} {
		row := summary(a)

		at, _ := time.Parse(time.RFC3339, asStr(t, row["createdAt"]))
		if row["projection"] != projectionSummary || row["followUpOfJobId"] != job ||
			row["followUpClass"] != statusPending ||
			!at.Equal(created) {
			t.Errorf("%s row: %v", name, row)
		}
	}
	// HQ classifies once
	if code, res := write(
		srv,
		hq(),
		"jobs.classifyFollowUp",
		`{"jobId":"`+follow+`","classification":"redo","reason":"x"}`,
		1,
	); code != 422 ||
		fields(res) != "classification=error.invalid" {
		t.Errorf("bad classification: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.classifyFollowUp",
		`{"jobId":"`+follow+`","classification":"rework","reason":"same fault"}`,
		1,
	); code != 200 ||
		data(res)["followUpClass"] != "rework" {
		t.Fatalf("classify: %d %v", code, res)
	}

	if row := summary(hq()); row["followUpClass"] != "rework" || row["followUpOfJobId"] != job {
		t.Errorf("classified row: %v", row)
	}

	if code, _ := write(
		srv,
		hq(),
		"jobs.classifyFollowUp",
		`{"jobId":"`+follow+`","classification":"new_request","reason":"again"}`,
		2,
	); code != 409 {
		t.Error("classify twice")
	}

	if code, _ := write(
		srv,
		customerA(),
		"jobs.classifyFollowUp",
		`{"jobId":"`+follow+`","classification":"rework","reason":"x"}`,
		2,
	); code != 403 {
		t.Error("client classifies")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestCostsAccessWarrantyAndProjections(t *testing.T) {
	srv := server(t)
	job := completedJob(t, srv)
	lines := `[{"kind":"estimate","amountMinor":5000,"currency":"MYR","description":" Parts ","visibility":"customer"},` +
		`{"kind":"actual","amountMinor":800,"currency":"MYR","description":"Margin","visibility":"internal"}]`

	if code, res := write(
		srv,
		hq(),
		"jobs.saveCost",
		`{"jobId":"`+job+`","costLines":[{"kind":"guess","amountMinor":1,"currency":"MYR","description":"x",`+
			`"visibility":"internal"}]}`,
		1,
	); code != 422 ||
		fields(res) != "costLines=error.invalid" {
		t.Errorf("bad cost line: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.saveCost",
		`{"jobId":"`+job+`","costLines":[{"kind":"actual","amountMinor":-1,"currency":"EUR","description":"",`+
			`"visibility":"internal"}]}`,
		1,
	); code != 422 ||
		fields(res) != "costLines=error.invalid" {
		t.Errorf("bad amount/currency: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.saveCost",
		`{"jobId":"`+job+`","costLines":`+lines+`}`,
		1,
	); code != 200 ||
		len(asArr(t, data(res)["costs"])) != 2 {
		t.Fatalf("saveCost: %d %v", code, res)
	}
	// IR42: clients see customer lines only
	if _, res := post(
		srv,
		customerA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); len(asArr(t, data(res)["costs"])) != 1 ||
		asObj(t, asArr(t, data(res)["costs"])[0])["description"] != "Parts" {
		t.Fatalf("client costs: %v", data(res)["costs"])
	}
	// IR442: the client's history leaves out HQ's cost record (counted out of total too); HQ sees it
	actions := func(a *actor) ([]string, float64) {
		t.Helper()

		code, res := post(srv, a, "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
		if code != 200 {
			t.Fatalf("jobs.events: %d %v", code, res)
		}

		events := asArr(t, data(res)["items"])

		out := make([]string, 0, len(events))
		for _, e := range events {
			out = append(out, asStr(t, asObj(t, e)["action"]))
		}

		return out, asNum(t, data(res)["total"])
	}
	if got, total := actions(
		customerA(),
	); slices.Contains(got, "job.costs_saved") ||
		int(total) != len(got) {
		t.Errorf("client history: %v (total %v)", got, total)
	}

	clientEvents, clientTotal := actions(customerA())

	hqEvents, _ := actions(hq())
	if !slices.Contains(hqEvents, "job.costs_saved") {
		t.Errorf("HQ history lacks the cost record: %v", hqEvents)
	}
	// the detail's event cursor counts only what the caller's history lists, so it reveals no hidden event
	cursor := func(a *actor) float64 {
		t.Helper()

		_, m := post(srv, a, "jobs.get", `{"jobId":"`+job+`"}`)

		return asNum(t, data(m)["eventCursor"])
	}
	if got := cursor(customerA()); got != clientTotal || len(clientEvents) >= len(hqEvents) {
		t.Errorf(
			"client eventCursor %v, client events %d, HQ events %d",
			got,
			len(clientEvents),
			len(hqEvents),
		)
	}

	if got := cursor(hq()); int(got) != len(hqEvents) {
		t.Errorf("HQ eventCursor %v, HQ events %d", got, len(hqEvents))
	}
	// warranty claims need a warranty that covers completion
	claim := `{"jobId":"` + job + `","partLabel":"Compressor","amountMinor":12000,"currency":"MYR","reason":"failed in ` +
		`warranty"}`

	owner(
		t,
		`UPDATE assets.units SET warranty_ends_at = NULL WHERE id = $1`,
		seed.ID("unit-non-rto"),
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.recordWarrantyClaim",
		claim,
		2,
	); code != 422 ||
		fields(res) != "jobId=errors.outside_warranty" {
		t.Errorf("no warranty: %d %s", code, fields(res))
	}

	owner(
		t,
		`UPDATE assets.units SET warranty_ends_at = $2 WHERE id = $1`,
		seed.ID("unit-non-rto"),
		clock().Add(30*24*time.Hour),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE assets.units SET warranty_ends_at = NULL WHERE id = $1`,
			seed.ID("unit-non-rto"),
		)
	})

	if code, res := write(
		srv,
		hq(),
		"jobs.recordWarrantyClaim",
		`{"jobId":"`+job+`","partLabel":"","amountMinor":0,"currency":"JPY","reason":""}`,
		2,
	); code != 422 ||
		fields(
			res,
		) != "amountMinor=error.range,currency=error.invalid,partLabel=error.length,reason=error.length" {
		t.Errorf("bad claim input: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.recordWarrantyClaim",
		claim,
		2,
	); code != 200 ||
		asObj(t, asArr(t, data(res)["warrantyClaims"])[0])["state"] != "filed" {
		t.Fatalf("claim: %d %v", code, res)
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'cancelled' WHERE id = $1`, job)

	if code, _ := write(
		srv,
		hq(),
		"jobs.saveCost",
		`{"jobId":"`+job+`","costLines":[]}`,
		3,
	); code != 409 {
		t.Error("cost on cancelled job")
	}

	if code, _ := write(srv, hq(), "jobs.recordWarrantyClaim", claim, 3); code != 409 {
		t.Error("claim on non-completed job")
	}

	// extend access of an accepted offer; JobDetail.offer per role
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)

	job2 := asStr(t, data(m)["id"])
	if code, _ := write(
		srv,
		hq(),
		"jobs.extendAccess",
		`{"jobId":"`+job2+`","accessValidUntil":"`+stamp(200)+`","reason":"x"}`,
		1,
	); code != 409 {
		t.Error("extend without accepted offer")
	}

	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job2+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			-1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	)

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job2+`"}`,
	); data(res)["offer"] == nil ||
		asObj(t, data(res)["offer"])["decision"] != nil {
		t.Fatal("hq sees the open offer")
	}

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job2},
		&offer,
	)
	write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job2+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	)

	if code, res := write(
		srv,
		hq(),
		"jobs.extendAccess",
		`{"jobId":"`+job2+`","accessValidUntil":"`+stamp(50)+`","reason":"x"}`,
		3,
	); code != 422 ||
		fields(res) != "accessValidUntil=error.mustExtend" {
		t.Errorf("extension must be later: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.extendAccess",
		`{"jobId":"`+job2+`","accessValidUntil":"`+stamp(200)+`","reason":" "}`,
		3,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("extension reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.extendAccess",
		`{"jobId":"`+job2+`","accessValidUntil":"`+stamp(200)+`","reason":"parts delayed"}`,
		3,
	); code != 200 {
		t.Fatalf("extend: %d %v", code, res)
	}

	if _, res := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job2+`"}`,
	); asObj(t, data(res)["offer"])["accessValidUntil"] != stamp(
		200,
	) { // the write answers MaintenanceJob (IR392)
		t.Fatalf("extended access: %v", data(res)["offer"])
	}

	if _, res := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job2+`"}`,
	); data(res)["offer"] == nil ||
		len(asArr(t, data(res)["costs"])) != 0 {
		t.Fatalf("contractor detail: %v", res)
	}

	if code, res := post(
		srv,
		customerA(),
		"jobs.get",
		`{"jobId":"`+job2+`"}`,
	); code != 200 ||
		data(res)["offer"] != nil {
		t.Fatal("client sees the offer")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWorkerConfirmsUnratedJobs(t *testing.T) {
	srv := server(t)
	job := completedJob(t, srv)
	owner(
		t,
		`UPDATE maintenance.jobs SET completed_at = $2 WHERE id = $1`,
		job,
		clock().Add(-7*24*time.Hour),
	)

	_, err := schedTick(context.Background(), srv, clock())
	if err != nil {
		t.Fatal(err)
	}

	if _, res := post(
		srv,
		customerA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); data(res)["customerConfirmedAt"] == nil ||
		data(res)["rating"] != nil {
		t.Fatalf("auto confirm: %v", res)
	}
}

// TestExtendAccessRefreezesHistory (IR458): an access window that already ended was frozen into the contractor's
// history; extending it removes that snapshot, and the next end freezes it again with the new end.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestExtendAccessRefreezesHistory(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, resp := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)

	job := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			0,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	); code != 200 {
		t.Fatalf("offer: %d %v", code, res)
	}

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}
	// the access window ended an hour ago and the worker froze the contractor's history
	owner(
		t,
		`UPDATE maintenance.offers SET access_valid_from = $2, access_valid_until = $3 WHERE id = $1`,
		offer,
		clock().Add(-5*time.Hour),
		clock().Add(-time.Hour),
	)

	asOf := func() *time.Time {
		t.Helper()

		var asOf *time.Time
		ownerScan(
			t,
			`SELECT (SELECT as_of FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'organization')`,
			[]any{job},
			&asOf,
		)

		return asOf
	}

	_, err := schedTick(ctx, srv, clock())
	if err != nil {
		t.Fatal(err)
	}

	if at := asOf(); at == nil || !at.Equal(clock().Add(-time.Hour)) {
		t.Fatalf("frozen at the first end: %v", at)
	}

	var version int
	ownerScan(t, `SELECT version FROM maintenance.jobs WHERE id = $1`, []any{job}, &version)

	if code, res := write(
		srv,
		hq(),
		"jobs.extendAccess",
		`{"jobId":"`+job+`","accessValidUntil":"`+stamp(2)+`","reason":"parts arrived late"}`,
		version,
	); code != 200 {
		t.Fatalf("extend: %d %v", code, res)
	}

	if at := asOf(); at != nil {
		t.Errorf("the old snapshot stays while the access is open again: %v", at)
	}

	_, err = schedTick(ctx, srv, clock().Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	if at := asOf(); at == nil || !at.Equal(clock().Add(2*time.Hour)) {
		t.Errorf("frozen again at the new end: %v", at)
	}
}
