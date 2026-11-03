package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const refusedLinesInvalid = "lines=" + apperr.KeyInvalid

func TestContractorRegister(t *testing.T) {
	srv := server(t)
	org := seed.ID("org-contractor-b").String()
	owner(t, `DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org)
	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)

	body := func(extra string) string {
		return `{"organizationId":"` + org + `","registrationNo":" REG-1 ","serviceAreas":["Kuala Lumpur"," Selangor "],` +
			`"contactEmail":"ops@partner.example","insuranceValidUntil":null` + extra + `}`
	}
	want := map[string]string{ // the field each case is refused for
		caseBadEmail:     "contactEmail=error.invalid",
		"blank reg":      "registrationNo=error.length",
		"customer org":   "organizationId=error.notContractor",
		"duplicate area": "serviceAreas=error.invalid",
		"no areas":       "serviceAreas=error.count",
	}

	for name, b := range map[string]string{
		"no areas": `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":[],` +
			`"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		"duplicate area": `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":["KL","kl"],` +
			`"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		caseBadEmail: `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":["KL"],` +
			`"contactEmail":"not-an-email","insuranceValidUntil":null}`,
		"blank reg": `{"organizationId":"` + org + `","registrationNo":" ","serviceAreas":["KL"],` +
			`"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		"customer org": `{"organizationId":"` + seed.ID("org-customer-a").String() + `","registrationNo":"R",` +
			`"serviceAreas":["KL"],"contactEmail":"a@b.example","insuranceValidUntil":null}`,
	} {
		code, m := write(srv, hq(), "contractors.save", b, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		hq(),
		"contractors.save",
		`{"organizationId":"`+uuid.NewString()+`","registrationNo":"R","serviceAreas":["KL"],"contactEmail":"a@b.example",`+
			`"insuranceValidUntil":null}`,
		0,
	); code != 404 {
		t.Error("unknown organization")
	}

	code, resp := write(srv, hq(), "contractors.save", body(""), 0)

	p := data(resp)
	if code != 200 || p["status"] != statusActive || p["registrationNo"] != "REG-1" ||
		p["name"] == "" ||
		p["kpis"] == nil ||
		p["rateCardId"] != nil {
		t.Fatalf("create: %d %v", code, resp)
	}

	id := asStr(t, p["id"])

	if code, _ := write(srv, hq(), "contractors.save", body(""), 0); code != 409 {
		t.Error("second profile")
	}

	until := clock().Add(200 * 24 * time.Hour).Format(time.RFC3339)
	if code, res := write(
		srv,
		hq(),
		"contractors.save",
		body(`,"id":"` + id + `"`)[:len(body(""))-len(`"insuranceValidUntil":null}`)]+`"insuranceValidUntil":"`+until+
			`","id":"`+id+`"}`,
		1,
	); code != 200 ||
		asObj(t, data(res)["delegation"])["to"] != until {
		t.Fatalf("update: %d %v", code, res)
	}

	other := `{"organizationId":"` + seed.ID("org-contractor-a").
		String() +
		`","registrationNo":"R","serviceAreas":["KL"],"contactEmail":"a@b.example","insuranceValidUntil":null,` +
		`"id":"` + id + `"}`
	if code, res := write(
		srv,
		hq(),
		"contractors.save",
		other,
		2,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["organizationId"] != "error.organizationFixed" {
		t.Errorf("a profile keeps its organization: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"contractors.save",
		body(`,"id":"`+id+`"`),
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("update of a stale version: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"contractors.save",
		body(`,"id":"`+uuid.NewString()+`"`),
		1,
	); code != 404 {
		t.Errorf("update of an unknown profile: %d", code)
	}
	// suspension
	if code, res := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+org+`","status":"suspended","reason":"insurance lapsed"}`,
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("suspend a stale version: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+uuid.NewString()+`","status":"suspended","reason":"insurance lapsed"}`,
		1,
	); code != 404 {
		t.Errorf("suspend an unknown contractor: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+org+`","status":"suspended","reason":" "}`,
		2,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("suspend without reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+org+`","status":"suspended","reason":"insurance lapsed"}`,
		2,
	); code != 200 ||
		data(res)["suspendedReason"] != "insurance lapsed" {
		t.Fatalf("suspend: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+org+`","status":"suspended","reason":"again"}`,
		3,
	); code != 409 {
		t.Error("suspend twice")
	}

	_, resp = post(srv, hq(), "contractors.list", `{"filters":{"status":"suspended"},"limit":100}`)

	found := false
	for _, it := range items(t, resp) {
		found = found || it["id"] == id
	}

	if !found {
		t.Error("list suspended")
	}

	if code, res := write(
		srv,
		hq(),
		"contractors.setOfferStatus",
		`{"contractorOrgId":"`+org+`","status":"active","reason":"renewed"}`,
		3,
	); code != 200 ||
		data(res)["suspendedReason"] != nil {
		t.Fatalf("reactivate: %d", code)
	}

	if code, res := post(
		srv,
		hq(),
		"contractors.list",
		`{"filters":{"status":"x"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad status filter: %d %s", code, fields(res))
	}

	if code, _ := post(srv, contrA(), "contractors.list", `{}`); code != 403 {
		t.Error("contractor lists register")
	}
	// rate cards
	rc := func(from string, lines string) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"rateCards.save",
			`{"contractorOrgId":"`+org+`","effectiveFrom":"`+from+`","currency":"MYR","lines":`+lines+`}`,
			0,
		)
	}
	line := `[{"workType":"repair_base","amountMinor":45000,"note":" base "}]`
	want = map[string]string{ // the field each case is refused for
		"bad worktype": refusedLinesInvalid,
		"dup worktype": refusedLinesInvalid,
		"negative":     refusedLinesInvalid,
		"no lines":     "lines=error.count",
		casePast:       "effectiveFrom=error.past",
	}

	for name, tc := range map[string][2]string{
		casePast:   {clock().Add(-time.Hour).Format(time.RFC3339), line},
		"no lines": {until, `[]`},
		"dup worktype": {until, `[{"workType":"emergency","amountMinor":1,"note":null},{"workType":"emergency",` +
			`"amountMinor":2,"note":null}]`},
		"bad worktype": {until, `[{"workType":"travel","amountMinor":1,"note":null}]`},
		"negative":     {until, `[{"workType":"emergency","amountMinor":-1,"note":null}]`},
	} {
		code, m := rc(tc[0], tc[1])
		refusal(t, name, code, m, want[name])
	}

	code, resp = rc(clock().Add(24*time.Hour).Format(time.RFC3339), line)
	if code != 200 || asNum(t, data(resp)["version"]) != 1 ||
		asObj(t, asArr(t, data(resp)["lines"])[0])["note"] != "base" {
		t.Fatalf("rate card: %d %v", code, resp)
	}

	if code, res := rc(
		clock().Add(48*time.Hour).Format(time.RFC3339),
		`[{"workType":"repair_base","amountMinor":45000,"note":"   "}]`,
	); code != 200 ||
		asNum(t, data(res)["version"]) != 2 ||
		asObj(t, asArr(t, data(res)["lines"])[0])["note"] != nil {
		t.Fatalf("rate card v2, a blank note is none: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"rateCards.save",
		`{"contractorOrgId":"`+uuid.NewString()+`","effectiveFrom":"`+until+`","currency":"MYR","lines":`+line+`}`,
		0,
	); code != 404 {
		t.Error("rate card for unknown contractor")
	}

	if _, res := post(
		srv,
		hq(),
		"rateCards.list",
		`{"filters":{"contractorOrgId":"`+org+`"}}`,
	); len(items(t, res)) != 2 ||
		asNum(t, items(t, res)[0]["version"]) != 2 {
		t.Fatalf("rate card list: %v", res)
	}

	if code, res := post(
		srv,
		contrA(),
		"rateCards.list",
		`{"filters":{"contractorOrgId":"`+org+`"}}`,
	); code != 200 ||
		len(items(t, res)) != 0 && items(t, res)[0]["contractorOrgId"] == org {
		t.Error("contractor sees another company's rate cards")
	}

	if code, res := post(
		srv,
		hq(),
		"rateCards.list",
		`{"filters":{"x":1}}`,
	); code != 422 ||
		fields(res) != refusedUnknownFieldX {
		t.Errorf("bad rate card filter: %d %s", code, fields(res))
	}
}

func TestSLAScorecard(t *testing.T) {
	srv := server(t)
	owner(t, `DELETE FROM maintenance.sla_targets`) // test-only rows
	// earlier runs leave jobs in the scorecard window (created_at set back by this test); move them out
	owner(
		t,
		`UPDATE maintenance.jobs SET created_at = created_at - interval '1000 days' WHERE created_at >= $1 AND created_at <
			$2`,
		clock().Add(-48*time.Hour),
		clock(),
	)

	want := map[string]string{ // the field each case is refused for
		"arrival 120":  "arrivalInWindowPercent=error.range",
		"bad plan":     "planType=error.invalid",
		casePast:       "effectiveFrom=error.past",
		"response 0":   "responseHours=error.range",
		"response 169": "responseHours=error.range",
	}

	for name, b := range map[string]string{
		"arrival 120": `{"planType":"general","responseHours":4,"arrivalInWindowPercent":120,"firstTimeFixPercent":80,` +
			`"effectiveFrom":"` + clock().Format(time.RFC3339) + `"}`,
		"response 0": `{"planType":"general","responseHours":0,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,` +
			`"effectiveFrom":"` + clock().Format(time.RFC3339) + `"}`,
		"response 169": `{"planType":"general","responseHours":169,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,` +
			`"effectiveFrom":"` + clock().Format(time.RFC3339) + `"}`,
		"bad plan": `{"planType":"vip","responseHours":4,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,` +
			`"effectiveFrom":"` + clock().Format(time.RFC3339) + `"}`,
		casePast: `{"planType":"general","responseHours":4,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,` +
			`"effectiveFrom":"` + clock().Add(-time.Hour).Format(time.RFC3339) + `"}`,
	} {
		code, m := write(srv, hq(), "sla.saveTargets", b, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, resp := write(
		srv,
		hq(),
		"sla.saveTargets",
		`{"planType":"general","responseHours":2,"arrivalInWindowPercent":95,"firstTimeFixPercent":90,`+
			`"effectiveFrom":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	); code != 200 ||
		asNum(t, data(resp)["version"]) != 1 {
		t.Fatalf("save targets: %d %v", code, resp)
	}

	if code, resp := write(
		srv,
		hq(),
		"sla.saveTargets",
		`{"planType":"general","responseHours":3,"arrivalInWindowPercent":90,"firstTimeFixPercent":85,`+
			`"effectiveFrom":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`"}`,
		0,
	); code != 409 ||
		resp["messageKey"] != "errors.targets_exist" {
		t.Errorf("targets of the same plan type and start: %d %v", code, resp)
	}

	// a job created 10 h before the clock, accepted after 6 h 10 min → response breach against the default 4 h
	unit := seed.ID("unit-non-rto").String()
	_, res := write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	job := asStr(t, data(res)["id"])
	created := clock().Add(-10 * time.Hour)
	owner(
		t,
		`UPDATE maintenance.jobs SET created_at = $2, status = 'completed', completed_at = $3, contractor_org_id = $4, rating
			= '{"stars":4}'::jsonb WHERE id = $1`,
		job,
		created,
		clock().Add(-time.Hour),
		seed.ID("org-contractor-a"),
	)
	owner(
		t,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at,
			offer_expires_at, access_valid_from, access_valid_until, decision, decided_at)
		VALUES ($1,$2,$3,'t',tstzrange($4,$5),$4,$5,$4,$5,'accept',$6)`,
		seed.ID("tenant-a"),
		job,
		seed.ID("org-contractor-a"),
		created,
		clock(),
		created.Add(6*time.Hour+10*time.Minute),
	)
	// a job accepted 1 h after its creation answered within the default 4 h: counted within the target, no breach
	_, res = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	onTime := asStr(t, data(res)["id"])
	owner(
		t,
		`UPDATE maintenance.jobs SET created_at = $2, contractor_org_id = $3 WHERE id = $1`,
		onTime,
		created,
		seed.ID("org-contractor-a"),
	)
	owner(
		t,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at,
			offer_expires_at, access_valid_from, access_valid_until, decision, decided_at)
		VALUES ($1,$2,$3,'t',tstzrange($4,$5),$4,$5,$4,$5,'accept',$6)`,
		seed.ID("tenant-a"),
		onTime,
		seed.ID("org-contractor-a"),
		created,
		clock(),
		created.Add(time.Hour),
	)
	// an open overdue job
	_, res = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	overdue := asStr(t, data(res)["id"])
	owner(
		t,
		`UPDATE maintenance.jobs SET created_at = $2, due_at = $3 WHERE id = $1`,
		overdue,
		clock().Add(-5*time.Hour),
		clock().Add(-time.Hour),
	)

	// IR291: a job cancelled before its response was due is neither within the target nor a miss; one cancelled later
	// without a response is a miss
	cancelled := map[string]time.Duration{}

	for _, after := range []time.Duration{time.Hour, 6 * time.Hour} {
		_, res = write(
			srv,
			hq(),
			"jobs.create",
			jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
			0,
		)
		id := asStr(t, data(res)["id"])
		cancelled[id] = after
		owner(
			t,
			`UPDATE maintenance.jobs SET created_at = $2, status = 'cancelled' WHERE id = $1`,
			id,
			clock().Add(-10*time.Hour),
		)
		owner(
			t,
			`INSERT INTO maintenance.job_events (id, tenant_id, job_id, action, occurred_at) VALUES (gen_random_uuid(), $1, $2,
				'job.cancelled', $3)`,
			seed.ID("tenant-a"),
			id,
			clock().Add(-10*time.Hour+after),
		)
	}

	period := `{"period":{"from":"` + clock().Add(-48*time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Format(
		time.RFC3339,
	) + `"}`

	code, res := post(srv, hq(), "sla.scorecard", period+`}`)
	if code != 200 {
		t.Fatalf("scorecard: %d %v", code, res)
	}

	sc := data(res)
	kinds := map[string]map[string]any{}

	for _, b := range asArr(t, sc["breaches"]) {
		bm := asObj(t, b)
		kinds[asStr(t, bm["jobId"])+":"+asStr(t, bm["kind"])] = bm
	}
	// the screens phrase a breach from its kind and minutes (IR291); detail stays English
	if r := kinds[job+":response"]; r == nil || r["detail"] != "response 6 h 10 min vs 4 h" ||
		r["tookMinutes"] != float64(370) ||
		r["limitMinutes"] != float64(240) {
		t.Fatalf("response breach: %v", sc["breaches"])
	}

	if r := kinds[overdue+":response"]; r == nil || r["detail"] != "no response within 4 h" ||
		r["tookMinutes"] != nil ||
		r["limitMinutes"] != float64(240) {
		t.Fatalf("unanswered breach: %v", sc["breaches"])
	}

	if o := kinds[overdue+":overdue"]; o == nil || o["detail"] == "" || o["tookMinutes"] != nil ||
		o["limitMinutes"] != nil {
		t.Fatalf("overdue breach: %v", sc["breaches"])
	}

	if kinds[onTime+":response"] != nil {
		t.Errorf("a response within the target is a breach: %v", kinds[onTime+":response"])
	}

	if share := asObj(t, sc["totals"])["responseWithinTarget"]; share == nil ||
		asNum(t, share) <= 0 ||
		asNum(t, share) >= 100 {
		t.Errorf("response share with one answer in time and misses: %v", share)
	}

	for id, after := range cancelled {
		if miss := kinds[id+":response"] != nil; miss != (after > 4*time.Hour) {
			t.Errorf("cancelled %v after creation: response miss %v", after, miss)
		}
	}

	if asNum(t, asObj(t, sc["totals"])["ratingCount"]) < 1 ||
		len(asArr(t, sc["customers"])) == 0 {
		t.Fatalf("totals: %v", sc)
	}
	// IR236: the targets per plan type in effect now (general's saved row starts in an hour: default until then) and the
	// scheduled row; each customer row names its plan type
	var general, scheduled map[string]any

	for _, x := range asArr(t, sc["targets"]) {
		tv := asObj(t, x)
		switch {
		case tv["planType"] == "general" && tv["state"] != statusScheduled:
			general = tv
		case tv["planType"] == "general" && tv["state"] == statusScheduled:
			scheduled = tv
		}
	}

	if general == nil || general["state"] != "default" || asNum(t, general["responseHours"]) != 4 ||
		general["effectiveFrom"] != nil ||
		len(asArr(t, sc["targets"])) != 5 {
		t.Fatalf("targets in effect: %v", sc["targets"])
	}

	if scheduled == nil || asNum(t, scheduled["responseHours"]) != 2 ||
		asNum(t, scheduled["version"]) != 1 ||
		scheduled["effectiveFrom"] == nil {
		t.Fatalf("scheduled targets: %v", sc["targets"])
	}

	if cm := asObj(t, asArr(t, sc["customers"])[0]); cm["planType"] == nil ||
		cm["planType"] == "" {
		t.Errorf("customer plan type: %v", cm)
	}

	for _, cm := range asArr(t, sc["customers"]) {
		if asObj(t, cm)["customerId"] == seed.ID("cust-a").String() &&
			asObj(t, cm)["status"] != "breached" {
			t.Errorf("customer-a status: %v", cm)
		}
	}
	// contractor filter
	_, res = post(
		srv,
		hq(),
		"sla.scorecard",
		period+`,"contractorOrgId":"`+seed.ID("org-contractor-b").String()+`"}`,
	)
	for _, b := range asArr(t, data(res)["breaches"]) {
		if asObj(t, b)["jobId"] == job {
			t.Error("contractor filter")
		}
	}

	if code, resp := post(
		srv,
		hq(),
		"sla.scorecard",
		`{"period":{"from":"`+clock().Format(
			time.RFC3339,
		)+`","to":"`+clock().Add(-time.Hour).
			Format(time.RFC3339)+
			`"}}`,
	); code != 422 ||
		fields(resp) != "period=error.range" {
		t.Errorf("reversed period: %d %s", code, fields(resp))
	}

	if code, _ := post(srv, customerA(), "sla.scorecard", period+`}`); code != 403 {
		t.Error("client scorecard")
	}
	// KPIs include the accepted offer and rating
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation)
		SELECT $1, $2, 'A', 'active', 'R-A', 'a@partner.example', tstzrange($3, $4) WHERE NOT EXISTS (SELECT 1 FROM
		maintenance.contractors WHERE organization_id = $2)`,
		seed.ID(
			"tenant-a",
		),
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
		clock().Add(1000*time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.contractors SET status = 'active' WHERE organization_id = $1`,
		seed.ID("org-contractor-a"),
	)

	_, res = post(srv, hq(), "contractors.list", `{"limit":100}`)
	seen := false

	for _, it := range items(t, res) {
		if it["organizationId"] == seed.ID("org-contractor-a").String() {
			seen = true

			k := asObj(t, it["kpis"])
			if asNum(t, k["ratingCount"]) < 1 || k["offerAcceptance"] == nil {
				t.Errorf("kpis: %v", k)
			}
		}
	}

	if !seen {
		t.Error("contractor-a profile listed")
	}
}

// TestContractorQualityMetrics covers the quality figures of a contractor's jobs (IR131 item 6, IR236): arrival in the
// scheduled window, reports accepted the first time, rework follow-ups, and the customer status they give. Two
// completed jobs of contractor B on two units: one on time and accepted at once, one late and returned once with a
// rework visit. A later job on the same unit within 30 days also counts as not fixed the first time, so the units
// differ.
func TestContractorQualityMetrics(t *testing.T) {
	srv := server(t)
	units := []string{newUnit(t, srv, "Quality AC"), newUnit(t, srv, "Quality AC")}
	at := func(h int) time.Time { return clock().Add(time.Duration(h) * time.Hour) }
	tenant, contractor := seed.ID("tenant-a"), seed.ID("org-contractor-b")
	job := func(unit string, arrived time.Time) string {
		id := uuid.NewString()
		owner(
			t,
			`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, contractor_org_id, type, status, origin,
				symptom, requested_slot, scheduled_slot,
			due_at, created_at, completed_at, time_on_site) VALUES ($1,$2,$3,$4,$5,'reactive','completed','client_request',
			'quality metrics',tstzrange($6,$7),tstzrange($6,$7),
			$7,$8,$9,jsonb_build_object('arrivedAt', $10::timestamptz))`,
			id,
			tenant,
			unit,
			seed.ID("org-customer-b"),
			contractor,
			at(-24),
			at(-22),
			at(-25),
			at(-21),
			arrived,
		)

		return id
	}
	review := func(job string, version int, decision string, when time.Time) {
		report := uuid.NewString()
		owner(
			t,
			`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state) VALUES ($1,$2,$3,$4,$5,
				'submitted')`,
			report,
			version,
			tenant,
			job,
			uuid.New(),
		)
		owner(
			t,
			`INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision,
				occurred_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			tenant,
			report,
			version,
			uuid.New(),
			decision,
			when,
		)
	}
	good, late := job(units[0], at(-23)), job(units[1], at(-21))
	review(good, 1, "accept", at(-21))
	review(late, 1, "return", at(-21))
	review(late, 2, "accept", at(-20))
	owner(
		t,
		`INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at,
			created_at, follow_up_of_job_id, follow_up_class)
		VALUES ($1,$2,$3,'reactive','requested','client_request',tstzrange($4,$5),$5,$6,$7,'rework')`,
		tenant,
		units[1],
		seed.ID("org-customer-b"),
		at(24),
		at(26),
		at(-19),
		late,
	)

	_, res := post(
		srv,
		hq(),
		"sla.scorecard",
		`{"period":{"from":"`+at(
			-26,
		).Format(time.RFC3339)+
			`","to":"`+at(
			-24,
		).Format(time.RFC3339)+
			`"},"contractorOrgId":"`+contractor.String()+`"}`,
	)
	sc := data(res)
	breaches := map[string]bool{}

	for _, b := range asArr(t, sc["breaches"]) {
		bm := asObj(t, b)
		breaches[asStr(t, bm["jobId"])+":"+asStr(t, bm["kind"])] = true
	}

	if !breaches[late+":arrival"] || breaches[good+":arrival"] ||
		!breaches[late+":first_time_fix"] ||
		breaches[good+":first_time_fix"] {
		t.Errorf("arrival and first-time-fix breaches: %v", sc["breaches"])
	}

	totals := asObj(t, sc["totals"])
	if totals["arrivalInWindow"] != 50.0 || totals["firstTimeFix"] != 50.0 {
		t.Errorf("totals: %v", totals)
	}

	if cm := asArr(t, sc["customers"]); len(cm) != 1 ||
		asObj(t, cm[0])["status"] != "breached" {
		t.Errorf("customer status: %v", cm)
	}
	// the contractor's 90-day KPIs count the same jobs
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation)
		SELECT $1, $2, 'B', 'active', 'R-B', 'b@partner.example', tstzrange($3, $4) WHERE NOT EXISTS (SELECT 1 FROM
		maintenance.contractors WHERE organization_id = $2)`,
		tenant,
		contractor,
		at(-1),
		at(1000),
	)

	_, res = post(srv, hq(), "contractors.list", `{"limit":100}`)
	for _, it := range items(t, res) {
		if it["organizationId"] != contractor.String() {
			continue
		}

		k := asObj(t, it["kpis"])

		share := func(name string) float64 {
			v, _ := k[name].(float64)

			return v
		}
		if a, f, r := share(
			"arrivalInWindow",
		), share(
			"firstTimeAccepted",
		), share(
			"reworkRate",
		); a <= 0 || a >= 100 || f <= 0 || f >= 100 || r <= 0 ||
			r >= 100 {
			t.Errorf("contractor B KPIs: %v", k)
		}

		return
	}

	t.Error("contractor B listed")
}

// TestFirstTimeFixIgnoresPlanVisits (IR450): a plan's next visit on the unit is no follow-up, so it does not make the
// completed job a first-time-fix miss; a client's new request within 30 days does.
func TestFirstTimeFixIgnoresPlanVisits(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "FTF AC")
	_, res := write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	job := asStr(t, data(res)["id"])
	owner(
		t,
		`UPDATE maintenance.jobs SET created_at = $2, status = 'completed', completed_at = $3 WHERE id = $1`,
		job,
		clock().Add(-10*time.Hour),
		clock().Add(-9*time.Hour),
	)

	var tenant, org string
	ownerScan(
		t,
		`SELECT tenant_id::text, customer_org_id::text FROM maintenance.jobs WHERE id = $1`,
		[]any{job},
		&tenant,
		&org,
	)
	// the plan's next visit, generated 90 minutes after the completion for a date three months ahead: no response target
	// either (IR451), though nobody has taken it after the default 4 h
	visit := uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at,
			created_at)
		VALUES ($7, $1, $2, $3, 'periodic', 'requested', 'periodic_plan', tstzrange($4, $5), $5, $6)`,
		tenant,
		unit,
		org,
		clock().Add(90*24*time.Hour),
		clock().Add(90*24*time.Hour+time.Hour),
		clock().Add(-7*time.Hour-30*time.Minute),
		visit,
	)

	miss := func() bool {
		t.Helper()

		code, resp := post(
			srv,
			hq(),
			"sla.scorecard",
			`{"period":{"from":"`+clock().Add(-48*time.Hour).
				Format(time.RFC3339)+
				`","to":"`+clock().Format(
				time.RFC3339,
			)+`"}}`,
		)
		if code != 200 {
			t.Fatalf("scorecard: %d %v", code, resp)
		}

		found := false

		for _, b := range asArr(t, data(resp)["breaches"]) {
			b := asObj(t, b)
			if b["jobId"] == visit {
				t.Errorf("the plan's visit is a %v breach", b["kind"])
			}

			if b["jobId"] == job && b["kind"] == "first_time_fix" {
				found = true
			}
		}

		return found
	}
	if miss() {
		t.Error("the plan's next visit made the job a first-time-fix miss")
	}

	if code, m := write(srv, customerB(), "jobs.create", jobBody(unit, nil), 0); code != 200 {
		t.Fatalf("client request: %d %v", code, m)
	}

	if !miss() {
		t.Error("a client's new request within 30 days is no miss")
	}
}

// TestOfferAcceptanceIgnoresCancelledJobs (IR452): an offer HQ's cancellation closes is no decline of the contractor,
// so the contractor's offer acceptance stays as it was.
func TestOfferAcceptanceIgnoresCancelledJobs(t *testing.T) {
	srv := server(t)
	org := seed.ID("org-contractor-a")
	owner(
		t,
		`INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email,
			delegation)
		SELECT $1, $2, 'A', 'active', 'R-A', 'a@partner.example', tstzrange($3, $4) WHERE NOT EXISTS (SELECT 1 FROM
		maintenance.contractors WHERE organization_id = $2)`,
		seed.ID("tenant-a"),
		org,
		clock().Add(-time.Hour),
		clock().Add(1000*time.Hour),
	)
	owner(t, `UPDATE maintenance.contractors SET status = 'active' WHERE organization_id = $1`, org)

	acceptance := func() any {
		t.Helper()

		_, m := post(srv, hq(), "contractors.list", `{"limit":100}`)
		for _, it := range items(t, m) {
			if it["organizationId"] == org.String() {
				return asObj(t, it["kpis"])["offerAcceptance"]
			}
		}

		t.Fatal("contractor-a not listed")

		return nil
	}
	// one accepted offer, so the share is defined
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	offer := func() string {
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
			`{"jobId":"`+job+`","contractorOrgId":"`+org.String()+`","visitSlot":`+slotJSON(49, 2)+
				`,"offerExpiresAt":"`+stamp(12)+`","accessValidFrom":"`+stamp(0)+`","accessValidUntil":"`+stamp(100)+
				`","termsVersion":"t"}`,
			1,
		); code != 200 {
			t.Fatalf("offer: %d %v", code, res)
		}
		// the KPI window ends now (exclusive): an offer made at the clock time is not in it yet
		owner(
			t,
			`UPDATE maintenance.offers SET offered_at = offered_at - interval '1 hour' WHERE job_id = $1`,
			job,
		)

		return job
	}
	accepted := offer()

	var oid string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{accepted},
		&oid,
	)

	if code, res := write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+accepted+`","offerId":"`+oid+`","termsVersion":"t"}`,
		2,
	); code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	before := acceptance()
	if before == nil {
		t.Fatal("no offer acceptance after an accepted offer")
	}

	cancelled := offer()
	if code, res := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+cancelled+`","cancelReason":"client moved out"}`,
		2,
	); code != 200 {
		t.Fatalf("cancel: %d %v", code, res)
	}

	if after := acceptance(); after != before {
		t.Errorf("offer acceptance moved from %v to %v by HQ's cancellation", before, after)
	}
}

// TestCancelEndsContractorAccess (IR588, BR-P08): HQ's cancellation of a job the contractor accepted ends the
// contractor's access window at once, as a reschedule does, so the unit leaves the contractor's scope through that job
// instead of staying readable until the window's planned end; once the worker freezes it, the job reads as its history.
// The scope's own copy of the window (assets.unit_offer_access) is checked, since other jobs of the shared database
// may grant contractor-a the same unit.
func TestCancelEndsContractorAccess(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(
		srv,
		customerA(),
		"jobs.create",
		jobBody(seed.ID("unit-non-rto").String(), nil),
		0,
	)

	job := asStr(t, data(m)["id"])
	if code, res := write(
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
			60,
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

	access := func() time.Time {
		t.Helper()

		var until time.Time

		drainEvents(t)
		ownerScan(
			t,
			`SELECT access_valid_until FROM assets.unit_offer_access WHERE offer_id = $1`,
			[]any{offer},
			&until,
		)

		return until
	}

	planned := clock().Add(60 * time.Hour)
	if until := access(); !until.Equal(planned) {
		t.Fatalf("the accepted offer's access in the unit scope: until %v, want %v", until, planned)
	}

	if code, res := write(
		srv,
		hq(),
		"jobs.cancel",
		`{"jobId":"`+job+`","cancelReason":"the customer withdrew"}`,
		3,
	); code != 200 {
		t.Fatalf("cancel: %d %v", code, res)
	}

	until := access()
	if !until.Before(
		planned,
	) { // the cancellation time (the cluster suite's server clock runs ahead of clock)
		t.Fatalf(
			"the cancelled job keeps the contractor's unit access until its planned end %v",
			until,
		)
	}

	freeze(t, until)

	if code, res := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 || data(res)["projection"] != projectionHistory ||
		data(res)["status"] != statusCancelled {
		t.Errorf("the cancelled job for the contractor: %d %v", code, res)
	}
}
