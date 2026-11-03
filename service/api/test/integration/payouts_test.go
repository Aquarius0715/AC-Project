package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPayouts(t *testing.T) {
	srv := server(t)
	tenant, org := seed.ID("tenant-a"), seed.ID("org-contractor-b")
	owner(
		t,
		`DELETE FROM billing.payout_queries WHERE statement_id IN (SELECT id FROM billing.payout_statements WHERE
			contractor_org_id = $1)`,
		org,
	)
	owner(t, `DELETE FROM billing.payout_statements WHERE contractor_org_id = $1`, org)
	owner(t, `DELETE FROM maintenance.rate_cards WHERE contractor_org_id = $1`, org)
	owner(
		t,
		`UPDATE maintenance.work_reports SET accepted_at = accepted_at - interval '3000 days' WHERE accepted_at >=
			'2026-06-01' AND accepted_at < '2026-09-01'`,
	)
	owner(
		t,
		`INSERT INTO maintenance.rate_cards (tenant_id, contractor_org_id, effective_from, currency, lines, version) VALUES
			($1,$2,'2026-06-01','MYR',
		'[{"workType":"periodic_inspection","amountMinor":30000,"note":null},{"workType":"repair_base","amountMinor":45000,`+
			`"note":null},{"workType":"rework_deduction","amountMinor":5000,"note":null}]',1)`,
		tenant,
		org,
	)
	unit := newUnit(t, srv, "Payout AC")
	accepted := func(typ string, when time.Time, returned bool) string {
		job, rep := uuid.NewString(), uuid.NewString()
		owner(
			t,
			`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at,
				contractor_org_id, completed_at)
			VALUES ($1,$2,$3,$4,$5,'completed','client_request',tstzrange($6,$7),$7,$8,$7)`,
			job,
			tenant,
			unit,
			seed.ID("org-customer-b"),
			typ,
			when.Add(-2*time.Hour),
			when,
			org,
		)
		owner(
			t,
			`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, accepted_at) VALUES ($1, 2,
				$2, $3, $4, 'accepted', $5)`,
			rep,
			tenant,
			job,
			seed.ID("user-tech-external-b"),
			when,
		)

		if returned {
			owner(
				t,
				`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state) VALUES ($1, 1, $2, $3, $4,
					'returned')`,
				rep,
				tenant,
				job,
				seed.ID("user-tech-external-b"),
			)
			owner(
				t,
				`INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, reason,
					occurred_at) VALUES ($1,$2,1,$3,'return','redo',$4)`,
				tenant,
				rep,
				seed.ID("user-hq-operator"),
				when.Add(-time.Hour),
			)
		}

		return job
	}
	julyJob := accepted("reactive", time.Date(2026, 7, 20, 3, 0, 0, 0, time.UTC), false)
	accepted("periodic", time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC), true)
	accepted("reactive", time.Date(2026, 8, 20, 3, 0, 0, 0, time.UTC), false)
	accepted(
		"reactive",
		time.Date(2026, 9, 2, 3, 0, 0, 0, time.UTC),
		false,
	) // not yet in a closed month

	gen := func(period string) (int, map[string]any) {
		return write(srv, hq(), "payouts.generate", `{"period":"`+period+`"}`, 0)
	}
	if code, m := gen("2026-09"); code != 422 || fields(m) != "period=errors.period_not_ended" {
		t.Errorf("month not ended: %d %s", code, fields(m))
	}

	if code, m := gen("2026/07"); code != 422 || fields(m) != "period=error.invalid" {
		t.Errorf("bad period: %d %s", code, fields(m))
	}

	stmtOf := func(m map[string]any) map[string]any {
		for _, x := range asArr(t, m["data"]) {
			if asObj(t, x)["contractorOrgId"] == org.String() {
				return asObj(t, x)
			}
		}

		return nil
	}
	_, resp := gen("2026-07")

	july := stmtOf(resp)
	if july == nil || asNum(t, july["netMinor"]) != 45000 ||
		july["payDate"] != "2026-08-15T00:00:00Z" {
		t.Fatalf("july: %v", resp)
	}

	jid := asStr(t, july["id"])
	if code, _ := post(srv, contrB(), "payouts.get", `{"id":"`+jid+`"}`); code != 404 {
		t.Error("contractor sees a draft")
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"approve"}`,
		1,
	); code != 200 ||
		data(res)["status"] != "approved" {
		t.Fatalf("approve: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"approve"}`,
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("approve on a stale version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"approve"}`,
		2,
	); code != 409 ||
		res["messageKey"] != apperr.KeyInvalidState {
		t.Errorf("approve twice: %d %v", code, res)
	}
	// question on the July line, answered with an adjustment
	line := asStr(t, asObj(t, asArr(t, july["lines"])[0])["id"])

	if code, _ := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+uuid.NewString()+`","topic":"amount","message":"why"}`,
		2,
	); code != 404 {
		t.Error("unknown line")
	}

	if code, res := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"price","message":"why"}`,
		2,
	); code != 422 ||
		fields(res) != "topic=error.invalid" {
		t.Errorf("bad topic: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"why"}`,
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("question on a stale version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"Travel was not included"}`,
		2,
	); code != 200 ||
		asObj(t, asArr(t, data(res)["queries"])[0])["state"] != statusOpen {
		t.Fatalf("query: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		contrA(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"x"}`,
		3,
	); code != 404 {
		t.Error("other contractor asks")
	}

	_, resp = post(srv, hq(), "payouts.get", `{"id":"`+jid+`"}`)

	q := asStr(t, asObj(t, asArr(t, data(resp)["queries"])[0])["id"])
	if code, res := write(
		srv,
		hq(),
		"payouts.resolveQuery",
		`{"statementId":"`+jid+`","queryId":"`+q+`","reply":"ok","adjustmentMinor":0}`,
		3,
	); code != 422 ||
		fields(res) != "adjustmentMinor=error.nonZero" {
		t.Errorf("zero adjustment: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.resolveQuery",
		`{"statementId":"`+jid+`","queryId":"`+q+`","reply":"Added travel","adjustmentMinor":12000}`,
		2,
	); code != 409 ||
		res["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("answer on a stale version: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.resolveQuery",
		`{"statementId":"`+jid+`","queryId":"`+q+`","reply":"Added travel","adjustmentMinor":12000}`,
		3,
	); code != 200 ||
		asObj(t, asArr(t, data(res)["queries"])[0])["state"] != "adjusted" {
		t.Fatalf("resolve: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"payouts.resolveQuery",
		`{"statementId":"`+jid+`","queryId":"`+q+`","reply":"again"}`,
		4,
	); code != 409 {
		t.Error("resolve twice")
	}

	_, resp = post(srv, hq(), "jobs.events", `{"jobId":"`+julyJob+`","query":{"limit":100}}`)
	notes := 0

	for _, e := range items(t, resp) {
		if e["action"] == "note.added" {
			notes++
		}
	}

	if notes != 2 {
		t.Errorf("question and reply in job history: %d", notes)
	}
	// a second question answered with a reduction: a negative adjustment is a deduction of the next statement
	if code, res := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"Parts were charged twice"}`,
		4,
	); code != 200 {
		t.Fatalf("second query: %d %v", code, res)
	}

	_, resp = post(srv, hq(), "payouts.get", `{"id":"`+jid+`"}`)

	var q2 string

	for _, x := range asArr(t, data(resp)["queries"]) { // both were asked at the test clock: pick the open one
		if x := asObj(t, x); x["state"] == statusOpen {
			q2 = asStr(t, x["id"])
		}
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.resolveQuery",
		`{"statementId":"`+jid+`","queryId":"`+q2+`","reply":"Refund the double charge","adjustmentMinor":-3000}`,
		5,
	); code != 200 {
		t.Fatalf("resolve with a reduction: %d %v", code, res)
	}
	// August draft: periodic + rework deduction + repair + the July adjustments (+12000, −3000)
	_, resp = gen("2026-08")

	aug := stmtOf(resp)
	if aug == nil || asNum(t, aug["grossMinor"]) != 30000+45000+12000 ||
		asNum(t, aug["deductionsMinor"]) != 5000+3000 ||
		len(asArr(t, aug["lines"])) != 5 {
		t.Fatalf("august: %v", aug)
	}
	// regenerating keeps the approved July statement and replaces the August draft
	_, resp = gen("2026-07")
	if stmtOf(resp) != nil {
		t.Error("approved statement regenerated")
	}

	_, resp = gen("2026-08")
	if a2 := stmtOf(
		resp,
	); a2 == nil || a2["id"] == aug["id"] ||
		asNum(t, a2["grossMinor"]) != 87000 {
		t.Fatalf("august regenerated: %v", a2)
	}
	// paying: not before the pay date, not a draft
	_, resp = post(srv, hq(), "payouts.get", `{"id":"`+jid+`"}`)
	owner(t, `UPDATE billing.payout_statements SET pay_date = '2026-09-20' WHERE id = $1`, jid)

	if code, _ := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"mark_paid"}`,
		ver(t, resp),
	); code != 409 {
		t.Error("mark paid before the pay date")
	}

	owner(t, `UPDATE billing.payout_statements SET pay_date = '2026-09-14' WHERE id = $1`, jid)

	code, resp := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"mark_paid","reason":"bank transfer"}`,
		ver(t, resp),
	)
	if code != 200 || data(resp)["paidAt"] == nil {
		t.Fatalf("mark paid: %d %v", code, resp)
	}

	if code, res := write(
		srv,
		contrB(),
		"payouts.query",
		`{"statementId":"`+jid+`","lineId":"`+line+`","topic":"amount","message":"late question"}`,
		ver(t, resp),
	); code != 409 ||
		res["messageKey"] != apperr.KeyInvalidState {
		t.Errorf("question on a paid statement: %d %v", code, res)
	}
	// contractor sees approved/paid only
	_, resp = post(srv, contrB(), "payouts.list", `{"limit":100}`)
	for _, it := range items(t, resp) {
		if it["status"] == statusDraft || it["contractorOrgId"] != org.String() {
			t.Fatalf("contractor list: %v", it)
		}
	}

	if len(items(t, resp)) == 0 {
		t.Error("contractor list empty")
	}

	if _, res := post(
		srv,
		hq(),
		"payouts.list",
		`{"filters":{"period":"2026-08","status":"draft","contractorOrgId":"`+org.String()+`"}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Error("hq list filters")
	}

	if code, res := post(
		srv,
		hq(),
		"payouts.list",
		`{"filters":{"status":"void"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad status filter: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"payouts.transition",
		`{"statementId":"`+jid+`","action":"refund"}`,
		6,
	); code != 422 ||
		fields(res) != "action=error.pathMismatch" {
		t.Errorf("bad action: %d %s", code, fields(res))
	}
}

// TestPayoutWithoutRateCard: a contractor without a rate card in effect still gets a statement for its accepted work,
// with each line noted "no rate card" and no amount, for HQ to settle by hand.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPayoutWithoutRateCard(t *testing.T) {
	srv := server(t)
	tenant, org := seed.ID(
		"tenant-a",
	), uuid.New() // a contractor organization without any rate card
	when := time.Date(2026, 5, 20, 3, 0, 0, 0, time.UTC)
	job, rep := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at,
			contractor_org_id, completed_at)
		VALUES ($1,$2,$3,$4,'reactive','completed','client_request',tstzrange($5,$6),$6,$7,$6)`,
		job,
		tenant,
		newUnit(t, srv, "No rate AC"),
		seed.ID("org-customer-b"),
		when.Add(-2*time.Hour),
		when,
		org,
	)
	owner(
		t,
		`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, accepted_at) VALUES ($1, 1,
			$2, $3, $4, 'accepted', $5)`,
		rep,
		tenant,
		job,
		uuid.New(),
		when,
	)

	code, res := write(srv, hq(), "payouts.generate", `{"period":"2026-05"}`, 0)
	if code != 200 {
		t.Fatalf("generate: %d %v", code, res)
	}

	for _, x := range asArr(t, res["data"]) {
		st := asObj(t, x)
		if st["contractorOrgId"] != org.String() {
			continue
		}

		lines := asArr(t, st["lines"])
		if l := asObj(t, lines[0]); len(lines) != 1 || l["note"] != "no rate card" ||
			asNum(t, l["amountMinor"]) != 0 ||
			asNum(t, st["netMinor"]) != 0 {
			t.Errorf("statement without a rate card: %v", st)
		}

		return
	}

	t.Errorf("no statement for the contractor: %v", res["data"])
}
