package app

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
	// an open overdue job
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	overdue := data(m)["id"].(string)
	owner(t, `UPDATE maintenance.jobs SET created_at = $2, due_at = $3 WHERE id = $1`, overdue, clock.Add(-5*time.Hour), clock.Add(-time.Hour))

	period := `{"period":{"from":"` + clock.Add(-48*time.Hour).Format(time.RFC3339) + `","to":"` + clock.Format(time.RFC3339) + `"}`
	code, m := post(s, &hq, "sla.scorecard", period+`}`)
	if code != 200 {
		t.Fatalf("scorecard: %d %v", code, m)
	}
	sc := data(m)
	kinds := map[string]string{}
	for _, b := range sc["breaches"].([]any) {
		bm := b.(map[string]any)
		if bm["jobId"] == job || bm["jobId"] == overdue {
			kinds[bm["kind"].(string)] = bm["detail"].(string)
		}
	}
	if kinds["response"] != "response 6 h 10 min vs 4 h" || kinds["overdue"] == "" {
		t.Fatalf("breaches: %v", sc["breaches"])
	}
	if sc["totals"].(map[string]any)["ratingCount"].(float64) < 1 || len(sc["customers"].([]any)) == 0 {
		t.Fatalf("totals: %v", sc)
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
