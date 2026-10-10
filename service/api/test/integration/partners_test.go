package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestContractorRegister(t *testing.T) {
	s := server(t)
	org := seed.ID("org-contractor-b").String()
	owner(t, `DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org)
	owner(t, `DELETE FROM maintenance.contractors WHERE organization_id = $1`, org)
	body := func(extra string) string {
		return `{"organizationId":"` + org + `","registrationNo":" REG-1 ","serviceAreas":["Kuala Lumpur"," Selangor "],"contactEmail":"ops@partner.example","insuranceValidUntil":null` + extra + `}`
	}
	for name, b := range map[string]string{
		"no areas":       `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":[],"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		"duplicate area": `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":["KL","kl"],"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		"bad email":      `{"organizationId":"` + org + `","registrationNo":"R","serviceAreas":["KL"],"contactEmail":"not-an-email","insuranceValidUntil":null}`,
		"blank reg":      `{"organizationId":"` + org + `","registrationNo":" ","serviceAreas":["KL"],"contactEmail":"a@b.example","insuranceValidUntil":null}`,
		"customer org":   `{"organizationId":"` + seed.ID("org-customer-a").String() + `","registrationNo":"R","serviceAreas":["KL"],"contactEmail":"a@b.example","insuranceValidUntil":null}`,
	} {
		if code, _ := write(s, &hq, "contractors.save", b, 0); code != 422 {
			t.Errorf("save %s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "contractors.save", `{"organizationId":"`+uuid.NewString()+`","registrationNo":"R","serviceAreas":["KL"],"contactEmail":"a@b.example","insuranceValidUntil":null}`, 0); code != 404 {
		t.Error("unknown organization")
	}
	code, m := write(s, &hq, "contractors.save", body(""), 0)
	p := data(m)
	if code != 200 || p["status"] != "active" || p["registrationNo"] != "REG-1" || p["name"] == "" || p["kpis"] == nil || p["rateCardId"] != nil {
		t.Fatalf("create: %d %v", code, m)
	}
	id := p["id"].(string)
	if code, _ := write(s, &hq, "contractors.save", body(""), 0); code != 409 {
		t.Error("second profile")
	}
	until := clock.Add(200 * 24 * time.Hour).Format(time.RFC3339)
	if code, m := write(s, &hq, "contractors.save", body(`,"id":"` + id + `"`)[:len(body(""))-len(`"insuranceValidUntil":null}`)]+`"insuranceValidUntil":"`+until+`","id":"`+id+`"}`, 1); code != 200 ||
		data(m)["delegation"].(map[string]any)["to"] != until {
		t.Fatalf("update: %d %v", code, m)
	}
	// suspension
	if code, _ := write(s, &hq, "contractors.setOfferStatus", `{"contractorOrgId":"`+org+`","status":"suspended","reason":" "}`, 2); code != 422 {
		t.Error("suspend without reason")
	}
	if code, m := write(s, &hq, "contractors.setOfferStatus", `{"contractorOrgId":"`+org+`","status":"suspended","reason":"insurance lapsed"}`, 2); code != 200 ||
		data(m)["suspendedReason"] != "insurance lapsed" {
		t.Fatalf("suspend: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "contractors.setOfferStatus", `{"contractorOrgId":"`+org+`","status":"suspended","reason":"again"}`, 3); code != 409 {
		t.Error("suspend twice")
	}
	_, m = post(s, &hq, "contractors.list", `{"filters":{"status":"suspended"},"limit":100}`)
	found := false
	for _, it := range items(m) {
		found = found || it["id"] == id
	}
	if !found {
		t.Error("list suspended")
	}
	if code, m := write(s, &hq, "contractors.setOfferStatus", `{"contractorOrgId":"`+org+`","status":"active","reason":"renewed"}`, 3); code != 200 || data(m)["suspendedReason"] != nil {
		t.Fatalf("reactivate: %d", code)
	}
	if code, _ := post(s, &hq, "contractors.list", `{"filters":{"status":"x"}}`); code != 422 {
		t.Error("bad status filter")
	}
	if code, _ := post(s, &contrA, "contractors.list", `{}`); code != 403 {
		t.Error("contractor lists register")
	}
	// rate cards
	rc := func(from string, lines string) (int, map[string]any) {
		return write(s, &hq, "rateCards.save", `{"contractorOrgId":"`+org+`","effectiveFrom":"`+from+`","currency":"MYR","lines":`+lines+`}`, 0)
	}
	line := `[{"workType":"repair_base","amountMinor":45000,"note":" base "}]`
	for name, tc := range map[string][2]string{
		"past":         {clock.Add(-time.Hour).Format(time.RFC3339), line},
		"no lines":     {until, `[]`},
		"dup worktype": {until, `[{"workType":"emergency","amountMinor":1,"note":null},{"workType":"emergency","amountMinor":2,"note":null}]`},
		"bad worktype": {until, `[{"workType":"travel","amountMinor":1,"note":null}]`},
		"negative":     {until, `[{"workType":"emergency","amountMinor":-1,"note":null}]`},
	} {
		if code, _ := rc(tc[0], tc[1]); code != 422 {
			t.Errorf("rate card %s: %d", name, code)
		}
	}
	code, m = rc(clock.Add(24*time.Hour).Format(time.RFC3339), line)
	if code != 200 || data(m)["version"].(float64) != 1 || data(m)["lines"].([]any)[0].(map[string]any)["note"] != "base" {
		t.Fatalf("rate card: %d %v", code, m)
	}
	if code, m := rc(clock.Add(48*time.Hour).Format(time.RFC3339), line); code != 200 || data(m)["version"].(float64) != 2 {
		t.Fatalf("rate card v2: %d", code)
	}
	if code, _ := write(s, &hq, "rateCards.save", `{"contractorOrgId":"`+uuid.NewString()+`","effectiveFrom":"`+until+`","currency":"MYR","lines":`+line+`}`, 0); code != 404 {
		t.Error("rate card for unknown contractor")
	}
	if _, m := post(s, &hq, "rateCards.list", `{"filters":{"contractorOrgId":"`+org+`"}}`); len(items(m)) != 2 || items(m)[0]["version"].(float64) != 2 {
		t.Fatalf("rate card list: %v", m)
	}
	if _, m := post(s, &contrA, "rateCards.list", `{"filters":{"contractorOrgId":"`+org+`"}}`); len(items(m)) != 0 && items(m)[0]["contractorOrgId"] == org {
		t.Error("contractor sees another company's rate cards")
	}
	if code, _ := post(s, &hq, "rateCards.list", `{"filters":{"x":1}}`); code != 422 {
		t.Error("bad rate card filter")
	}
}

func TestSLAScorecard(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM maintenance.sla_targets`) // test-only rows
	// earlier runs leave jobs in the scorecard window (created_at set back by this test); move them out
	owner(t, `UPDATE maintenance.jobs SET created_at = created_at - interval '1000 days' WHERE created_at >= $1 AND created_at < $2`, clock.Add(-48*time.Hour), clock)
	for name, b := range map[string]string{
		"arrival 120":  `{"planType":"general","responseHours":4,"arrivalInWindowPercent":120,"firstTimeFixPercent":80,"effectiveFrom":"` + clock.Format(time.RFC3339) + `"}`,
		"response 0":   `{"planType":"general","responseHours":0,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,"effectiveFrom":"` + clock.Format(time.RFC3339) + `"}`,
		"response 169": `{"planType":"general","responseHours":169,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,"effectiveFrom":"` + clock.Format(time.RFC3339) + `"}`,
		"bad plan":     `{"planType":"vip","responseHours":4,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,"effectiveFrom":"` + clock.Format(time.RFC3339) + `"}`,
		"past":         `{"planType":"general","responseHours":4,"arrivalInWindowPercent":90,"firstTimeFixPercent":80,"effectiveFrom":"` + clock.Add(-time.Hour).Format(time.RFC3339) + `"}`,
	} {
		if code, _ := write(s, &hq, "sla.saveTargets", b, 0); code != 422 {
			t.Errorf("targets %s: %d", name, code)
		}
	}
	if code, m := write(s, &hq, "sla.saveTargets", `{"planType":"general","responseHours":2,"arrivalInWindowPercent":95,"firstTimeFixPercent":90,"effectiveFrom":"`+clock.Add(time.Hour).Format(time.RFC3339)+`"}`, 0); code != 200 || data(m)["version"].(float64) != 1 {
		t.Fatalf("save targets: %d %v", code, m)
	}

	// a job created 10 h before the clock, accepted after 6 h 10 min → response breach against the default 4 h
	unit := seed.ID("unit-non-rto").String()
	_, m := write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	job := data(m)["id"].(string)
	created := clock.Add(-10 * time.Hour)
	owner(t, `UPDATE maintenance.jobs SET created_at = $2, status = 'completed', completed_at = $3, contractor_org_id = $4, rating = '{"stars":4}'::jsonb WHERE id = $1`,
		job, created, clock.Add(-time.Hour), seed.ID("org-contractor-a"))
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until, decision, decided_at)
		VALUES ($1,$2,$3,'t',tstzrange($4,$5),$4,$5,$4,$5,'accept',$6)`, seed.ID("tenant-a"), job, seed.ID("org-contractor-a"), created, clock, created.Add(6*time.Hour+10*time.Minute))
	// a job accepted 1 h after its creation answered within the default 4 h: counted within the target, no breach
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	onTime := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET created_at = $2, contractor_org_id = $3 WHERE id = $1`, onTime, created, seed.ID("org-contractor-a"))
	owner(t, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at, access_valid_from, access_valid_until, decision, decided_at)
		VALUES ($1,$2,$3,'t',tstzrange($4,$5),$4,$5,$4,$5,'accept',$6)`, seed.ID("tenant-a"), onTime, seed.ID("org-contractor-a"), created, clock, created.Add(time.Hour))
	// an open overdue job
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	overdue := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET created_at = $2, due_at = $3 WHERE id = $1`, overdue, clock.Add(-5*time.Hour), clock.Add(-time.Hour))

	// IR291: a job cancelled before its response was due is neither within the target nor a miss; one cancelled later
	// without a response is a miss
	cancelled := map[string]time.Duration{}
	for _, after := range []time.Duration{time.Hour, 6 * time.Hour} {
		_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
		id := data(m)["id"].(string)
		cancelled[id] = after
		owner(t, `UPDATE maintenance.jobs SET created_at = $2, status = 'cancelled' WHERE id = $1`, id, clock.Add(-10*time.Hour))
		owner(t, `INSERT INTO maintenance.job_events (id, tenant_id, job_id, action, occurred_at) VALUES (gen_random_uuid(), $1, $2, 'job.cancelled', $3)`,
			seed.ID("tenant-a"), id, clock.Add(-10*time.Hour+after))
	}
	period := `{"period":{"from":"` + clock.Add(-48*time.Hour).Format(time.RFC3339) + `","to":"` + clock.Format(time.RFC3339) + `"}`
	code, m := post(s, &hq, "sla.scorecard", period+`}`)
	if code != 200 {
		t.Fatalf("scorecard: %d %v", code, m)
	}
	sc := data(m)
	kinds := map[string]map[string]any{}
	for _, b := range sc["breaches"].([]any) {
		bm := b.(map[string]any)
		kinds[bm["jobId"].(string)+":"+bm["kind"].(string)] = bm
	}
	// the screens phrase a breach from its kind and minutes (IR291); detail stays English
	if r := kinds[job+":response"]; r == nil || r["detail"] != "response 6 h 10 min vs 4 h" || r["tookMinutes"] != float64(370) || r["limitMinutes"] != float64(240) {
		t.Fatalf("response breach: %v", sc["breaches"])
	}
	if r := kinds[overdue+":response"]; r == nil || r["detail"] != "no response within 4 h" || r["tookMinutes"] != nil || r["limitMinutes"] != float64(240) {
		t.Fatalf("unanswered breach: %v", sc["breaches"])
	}
	if o := kinds[overdue+":overdue"]; o == nil || o["detail"] == "" || o["tookMinutes"] != nil || o["limitMinutes"] != nil {
		t.Fatalf("overdue breach: %v", sc["breaches"])
	}
	if kinds[onTime+":response"] != nil {
		t.Errorf("a response within the target is a breach: %v", kinds[onTime+":response"])
	}
	if share := sc["totals"].(map[string]any)["responseWithinTarget"]; share == nil || share.(float64) <= 0 || share.(float64) >= 100 {
		t.Errorf("response share with one answer in time and misses: %v", share)
	}
	for id, after := range cancelled {
		if miss := kinds[id+":response"] != nil; miss != (after > 4*time.Hour) {
			t.Errorf("cancelled %v after creation: response miss %v", after, miss)
		}
	}
	if sc["totals"].(map[string]any)["ratingCount"].(float64) < 1 || len(sc["customers"].([]any)) == 0 {
		t.Fatalf("totals: %v", sc)
	}
	// IR236: the targets per plan type in effect now (general's saved row starts in an hour: default until then) and the
	// scheduled row; each customer row names its plan type
	var general, scheduled map[string]any
	for _, x := range sc["targets"].([]any) {
		tv := x.(map[string]any)
		switch {
		case tv["planType"] == "general" && tv["state"] != "scheduled":
			general = tv
		case tv["planType"] == "general" && tv["state"] == "scheduled":
			scheduled = tv
		}
	}
	if general == nil || general["state"] != "default" || general["responseHours"].(float64) != 4 || general["effectiveFrom"] != nil || len(sc["targets"].([]any)) != 5 {
		t.Fatalf("targets in effect: %v", sc["targets"])
	}
	if scheduled == nil || scheduled["responseHours"].(float64) != 2 || scheduled["version"].(float64) != 1 || scheduled["effectiveFrom"] == nil {
		t.Fatalf("scheduled targets: %v", sc["targets"])
	}
	if cm := sc["customers"].([]any)[0].(map[string]any); cm["planType"] == nil || cm["planType"] == "" {
		t.Errorf("customer plan type: %v", cm)
	}
	for _, cm := range sc["customers"].([]any) {
		if cm.(map[string]any)["customerId"] == seed.ID("cust-a").String() && cm.(map[string]any)["status"] != "breached" {
			t.Errorf("customer-a status: %v", cm)
		}
	}
	// contractor filter
	_, m = post(s, &hq, "sla.scorecard", period+`,"contractorOrgId":"`+seed.ID("org-contractor-b").String()+`"}`)
	for _, b := range data(m)["breaches"].([]any) {
		if b.(map[string]any)["jobId"] == job {
			t.Error("contractor filter")
		}
	}
	if code, _ := post(s, &hq, "sla.scorecard", `{"period":{"from":"`+clock.Format(time.RFC3339)+`","to":"`+clock.Add(-time.Hour).Format(time.RFC3339)+`"}}`); code != 422 {
		t.Error("reversed period")
	}
	if code, _ := post(s, &customerA, "sla.scorecard", period+`}`); code != 403 {
		t.Error("client scorecard")
	}
	// KPIs include the accepted offer and rating
	owner(t, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email, delegation)
		SELECT $1, $2, 'A', 'active', 'R-A', 'a@partner.example', tstzrange($3, $4) WHERE NOT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $2)`,
		seed.ID("tenant-a"), seed.ID("org-contractor-a"), clock.Add(-time.Hour), clock.Add(1000*time.Hour))
	owner(t, `UPDATE maintenance.contractors SET status = 'active' WHERE organization_id = $1`, seed.ID("org-contractor-a"))
	_, m = post(s, &hq, "contractors.list", `{"limit":100}`)
	seen := false
	for _, it := range items(m) {
		if it["organizationId"] == seed.ID("org-contractor-a").String() {
			seen = true
			k := it["kpis"].(map[string]any)
			if k["ratingCount"].(float64) < 1 || k["offerAcceptance"] == nil {
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
// rework visit. A later job on the same unit within 30 days also counts as not fixed the first time, so the units differ.
func TestContractorQualityMetrics(t *testing.T) {
	s := server(t)
	units := []string{newUnit(t, s, "Quality AC"), newUnit(t, s, "Quality AC")}
	at := func(h int) time.Time { return clock.Add(time.Duration(h) * time.Hour) }
	tenant, contractor := seed.ID("tenant-a"), seed.ID("org-contractor-b")
	job := func(unit string, arrived time.Time) string {
		id := uuid.NewString()
		owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, contractor_org_id, type, status, origin, symptom, requested_slot, scheduled_slot,
			due_at, created_at, completed_at, time_on_site) VALUES ($1,$2,$3,$4,$5,'reactive','completed','client_request','quality metrics',tstzrange($6,$7),tstzrange($6,$7),
			$7,$8,$9,jsonb_build_object('arrivedAt', $10::timestamptz))`, id, tenant, unit, seed.ID("org-customer-b"), contractor, at(-24), at(-22), at(-25), at(-21), arrived)
		return id
	}
	review := func(job string, version int, decision string, when time.Time) {
		report := uuid.NewString()
		owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state) VALUES ($1,$2,$3,$4,$5,'submitted')`, report, version, tenant, job, uuid.New())
		owner(t, `INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, occurred_at) VALUES ($1,$2,$3,$4,$5,$6)`,
			tenant, report, version, uuid.New(), decision, when)
	}
	good, late := job(units[0], at(-23)), job(units[1], at(-21))
	review(good, 1, "accept", at(-21))
	review(late, 1, "return", at(-21))
	review(late, 2, "accept", at(-20))
	owner(t, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at, created_at, follow_up_of_job_id, follow_up_class)
		VALUES ($1,$2,$3,'reactive','requested','client_request',tstzrange($4,$5),$5,$6,$7,'rework')`, tenant, units[1], seed.ID("org-customer-b"), at(24), at(26), at(-19), late)

	_, m := post(s, &hq, "sla.scorecard", `{"period":{"from":"`+at(-26).Format(time.RFC3339)+`","to":"`+at(-24).Format(time.RFC3339)+`"},"contractorOrgId":"`+contractor.String()+`"}`)
	sc := data(m)
	breaches := map[string]bool{}
	for _, b := range sc["breaches"].([]any) {
		bm := b.(map[string]any)
		breaches[bm["jobId"].(string)+":"+bm["kind"].(string)] = true
	}
	if !breaches[late+":arrival"] || breaches[good+":arrival"] || !breaches[late+":first_time_fix"] || breaches[good+":first_time_fix"] {
		t.Errorf("arrival and first-time-fix breaches: %v", sc["breaches"])
	}
	totals := sc["totals"].(map[string]any)
	if totals["arrivalInWindow"] != 50.0 || totals["firstTimeFix"] != 50.0 {
		t.Errorf("totals: %v", totals)
	}
	if cm := sc["customers"].([]any); len(cm) != 1 || cm[0].(map[string]any)["status"] != "breached" {
		t.Errorf("customer status: %v", cm)
	}
	// the contractor's 90-day KPIs count the same jobs
	owner(t, `INSERT INTO maintenance.contractors (tenant_id, organization_id, name, status, registration_no, contact_email, delegation)
		SELECT $1, $2, 'B', 'active', 'R-B', 'b@partner.example', tstzrange($3, $4) WHERE NOT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $2)`,
		tenant, contractor, at(-1), at(1000))
	_, m = post(s, &hq, "contractors.list", `{"limit":100}`)
	for _, it := range items(m) {
		if it["organizationId"] != contractor.String() {
			continue
		}
		k := it["kpis"].(map[string]any)
		share := func(name string) float64 { v, _ := k[name].(float64); return v }
		if a, f, r := share("arrivalInWindow"), share("firstTimeAccepted"), share("reworkRate"); a <= 0 || a >= 100 || f <= 0 || f >= 100 || r <= 0 || r >= 100 {
			t.Errorf("contractor B KPIs: %v", k)
		}
		return
	}
	t.Error("contractor B listed")
}
