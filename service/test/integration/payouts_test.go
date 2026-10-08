package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func TestPayouts(t *testing.T) {
	s := server(t)
	tenant, org := seed.ID("tenant-a"), seed.ID("org-contractor-b")
	owner(t, `DELETE FROM billing.payout_queries WHERE statement_id IN (SELECT id FROM billing.payout_statements WHERE contractor_org_id = $1)`, org)
	owner(t, `DELETE FROM billing.payout_statements WHERE contractor_org_id = $1`, org)
	owner(t, `DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org)
	owner(t, `UPDATE maintenance.work_reports SET accepted_at = accepted_at - interval '3000 days' WHERE accepted_at >= '2026-06-01' AND accepted_at < '2026-09-01'`)
	owner(t, `INSERT INTO maintenance.rate_cards (tenant_id, contractor_org_id, effective_from, currency, lines, version) VALUES ($1,$2,'2026-06-01','MYR',
		'[{"workType":"periodic_inspection","amountMinor":30000,"note":null},{"workType":"repair_base","amountMinor":45000,"note":null},{"workType":"rework_deduction","amountMinor":5000,"note":null}]',1)`, tenant, org)
	unit := newUnit(t, s, "Payout AC")
	accepted := func(typ string, at time.Time, returned bool) string {
		job, rep := uuid.NewString(), uuid.NewString()
		owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at, contractor_org_id, completed_at)
			VALUES ($1,$2,$3,$4,$5,'completed','client_request',tstzrange($6,$7),$7,$8,$7)`, job, tenant, unit, seed.ID("org-customer-b"), typ, at.Add(-2*time.Hour), at, org)
		owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, accepted_at) VALUES ($1, 2, $2, $3, $4, 'accepted', $5)`, rep, tenant, job, seed.ID("user-tech-external-b"), at)
		if returned {
			owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state) VALUES ($1, 1, $2, $3, $4, 'returned')`, rep, tenant, job, seed.ID("user-tech-external-b"))
			owner(t, `INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, reason, occurred_at) VALUES ($1,$2,1,$3,'return','redo',$4)`,
				tenant, rep, seed.ID("user-hq-operator"), at.Add(-time.Hour))
		}
		return job
	}
	julyJob := accepted("reactive", time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC), false)
	accepted("periodic", time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC), true)
	accepted("reactive", time.Date(2026, 8, 20, 3, 0, 0, 0, time.UTC), false)
	accepted("reactive", time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC), false) // not yet in a closed month

	gen := func(period string) (int, map[string]any) {
		return write(s, &hq, "payouts.generate", `{"period":"`+period+`"}`, 0)
	}
	if code, _ := gen("2026-09"); code != 422 {
		t.Error("month not ended")
	}
	if code, _ := gen("2026/07"); code != 422 {
		t.Error("bad period")
	}
	stmtOf := func(m map[string]any) map[string]any {
		for _, x := range m["data"].([]any) {
			if x.(map[string]any)["contractorOrgId"] == org.String() {
				return x.(map[string]any)
			}
		}
		return nil
	}
	_, m := gen("2026-07")
	july := stmtOf(m)
	if july == nil || july["netMinor"].(float64) != 45000 || july["payDate"] != "2026-08-15T00:00:00Z" {
		t.Fatalf("july: %v", m)
	}
	jid := july["id"].(string)
	if code, _ := post(s, &contrB, "payouts.get", `{"id":"`+jid+`"}`); code != 404 {
		t.Error("contractor sees a draft")
	}
	if code, m := write(s, &hq, "payouts.transition", `{"statementId":"`+jid+`","action":"approve"}`, 1); code != 200 || data(m)["status"] != "approved" {
		t.Fatalf("approve: %d %v", code, m)
	}
	// question on the July line, answered with an adjustment
	line := july["lines"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := write(s, &contrB, "payouts.query", `{"statementId":"`+jid+`","lineId":"`+uuid.NewString()+`","topic":"amount","message":"why"}`, 2); code != 404 {
		t.Error("unknown line")
	}
	if code, _ := write(s, &contrB, "payouts.query", `{"statementId":"`+jid+`","lineId":"`+line+`","topic":"price","message":"why"}`, 2); code != 422 {
		t.Error("bad topic")
	}
	if code, m := write(s, &contrB, "payouts.query", `{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"Travel was not included"}`, 2); code != 200 ||
		data(m)["queries"].([]any)[0].(map[string]any)["state"] != "open" {
		t.Fatalf("query: %d %v", code, m)
	}
	if code, _ := write(s, &contrA, "payouts.query", `{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"x"}`, 3); code != 404 {
		t.Error("other contractor asks")
	}
	_, m = post(s, &hq, "payouts.get", `{"id":"`+jid+`"}`)
	q := data(m)["queries"].([]any)[0].(map[string]any)["id"].(string)
	if code, _ := write(s, &hq, "payouts.resolveQuery", `{"statementId":"`+jid+`","queryId":"`+q+`","reply":"ok","adjustmentMinor":0}`, 3); code != 422 {
		t.Error("zero adjustment")
	}
	if code, m := write(s, &hq, "payouts.resolveQuery", `{"statementId":"`+jid+`","queryId":"`+q+`","reply":"Added travel","adjustmentMinor":12000}`, 3); code != 200 ||
		data(m)["queries"].([]any)[0].(map[string]any)["state"] != "adjusted" {
		t.Fatalf("resolve: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "payouts.resolveQuery", `{"statementId":"`+jid+`","queryId":"`+q+`","reply":"again"}`, 4); code != 409 {
		t.Error("resolve twice")
	}
	_, m = post(s, &hq, "jobs.events", `{"jobId":"`+julyJob+`","query":{"limit":100}}`)
	notes := 0
	for _, e := range items(m) {
		if e["action"] == "note.added" {
			notes++
		}
	}
	if notes != 2 {
		t.Errorf("question and reply in job history: %d", notes)
	}
	// August draft: periodic + rework deduction + repair + July adjustment
	_, m = gen("2026-08")
	aug := stmtOf(m)
	if aug == nil || aug["grossMinor"].(float64) != 30000+45000+12000 || aug["deductionsMinor"].(float64) != 5000 || len(aug["lines"].([]any)) != 4 {
		t.Fatalf("august: %v", aug)
	}
	// regenerating keeps the approved July statement and replaces the August draft
	_, m = gen("2026-07")
	if stmtOf(m) != nil {
		t.Error("approved statement regenerated")
	}
	_, m = gen("2026-08")
	if a2 := stmtOf(m); a2 == nil || a2["id"] == aug["id"] || a2["grossMinor"].(float64) != 87000 {
		t.Fatalf("august regenerated: %v", a2)
	}
	// paying: not before the pay date, not a draft
	_, m = post(s, &hq, "payouts.get", `{"id":"`+jid+`"}`)
	owner(t, `UPDATE billing.payout_statements SET pay_date = '2026-09-20' WHERE id = $1`, jid)
	if code, _ := write(s, &hq, "payouts.transition", `{"statementId":"`+jid+`","action":"mark_paid"}`, ver(m)); code != 409 {
		t.Error("mark paid before the pay date")
	}
	owner(t, `UPDATE billing.payout_statements SET pay_date = '2026-09-14' WHERE id = $1`, jid)
	if code, m := write(s, &hq, "payouts.transition", `{"statementId":"`+jid+`","action":"mark_paid","reason":"bank transfer"}`, ver(m)); code != 200 || data(m)["paidAt"] == nil {
		t.Fatalf("mark paid: %d %v", code, m)
	}
	// contractor sees approved/paid only
	_, m = post(s, &contrB, "payouts.list", `{"limit":100}`)
	for _, it := range items(m) {
		if it["status"] == "draft" || it["contractorOrgId"] != org.String() {
			t.Fatalf("contractor list: %v", it)
		}
	}
	if len(items(m)) == 0 {
		t.Error("contractor list empty")
	}
	if _, m := post(s, &hq, "payouts.list", `{"filters":{"period":"2026-08","status":"draft","contractorOrgId":"`+org.String()+`"}}`); len(items(m)) != 1 {
		t.Error("hq list filters")
	}
	if code, _ := post(s, &hq, "payouts.list", `{"filters":{"status":"void"}}`); code != 422 {
		t.Error("bad status filter")
	}
	if code, _ := write(s, &hq, "payouts.transition", `{"statementId":"`+jid+`","action":"refund"}`, 6); code != 422 {
		t.Error("bad action")
	}
}
