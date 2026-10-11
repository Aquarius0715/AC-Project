package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// BR-A23 / BR-P10 / IR341: the billing scheduler generates the drafts of the month that ended (August for the
// September clock), once per month. A later tick regenerates nothing, and a run after HQ approved the statement
// leaves it as it is.
func TestMonthlyPayoutDrafts(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	tenant, org := seed.ID("tenant-a"), uuid.New() // a contractor of its own, without a rate card
	at := time.Date(2026, 8, 20, 3, 0, 0, 0, time.UTC)
	job, rep := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at, contractor_org_id, completed_at)
		VALUES ($1,$2,$3,$4,'reactive','completed','client_request',tstzrange($5,$6),$6,$7,$6)`, job, tenant, newUnit(t, s, "Monthly payout AC"), seed.ID("org-customer-b"), at.Add(-2*time.Hour), at, org)
	owner(t, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, accepted_at) VALUES ($1, 1, $2, $3, $4, 'accepted', $5)`, rep, tenant, job, uuid.New(), at)
	t.Cleanup(func() { // later tests' payout lists stay as they were
		owner(t, `DELETE FROM billing.payout_lines WHERE statement_id IN (SELECT id FROM billing.payout_statements WHERE contractor_org_id = $1)`, org)
		owner(t, `DELETE FROM billing.payout_statements WHERE contractor_org_id = $1`, org)
		owner(t, `UPDATE maintenance.work_reports SET accepted_at = accepted_at - interval '3000 days' WHERE id = $1`, rep)
	})
	rerun := func() { // the next tick runs August again
		t.Helper()
		owner(t, `DELETE FROM billing.payout_runs WHERE tenant_id = $1 AND period = '2026-08'`, tenant)
	}
	tick := 0
	evaluate := func() {
		t.Helper()
		tick++
		if _, err := schedTick(ctx, s, clock.Add(time.Duration(tick)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	type statement struct {
		id, status string
		lines      int
	}
	august := func() []statement {
		t.Helper()
		var n int
		ownerScan(t, `SELECT count(*) FROM billing.payout_statements WHERE contractor_org_id = $1 AND period = '2026-08'`, []any{org}, &n)
		out := make([]statement, n)
		for i := range out {
			ownerScan(t, `SELECT s.id::text, s.status, (SELECT count(*) FROM billing.payout_lines l WHERE l.statement_id = s.id) FROM billing.payout_statements s
				WHERE s.contractor_org_id = $1 AND s.period = '2026-08' ORDER BY s.created_at, s.id OFFSET $2 LIMIT 1`, []any{org, i}, &out[i].id, &out[i].status, &out[i].lines)
		}
		return out
	}

	rerun()
	evaluate()
	got := august()
	if len(got) != 1 || got[0].status != "draft" || got[0].lines != 1 {
		t.Fatalf("the August draft: %+v", got)
	}
	draft := got[0].id
	var drafts, audits int
	ownerScan(t, `SELECT statements FROM billing.payout_runs WHERE tenant_id = $1 AND period = '2026-08'`, []any{tenant}, &drafts)
	ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = 'payouts.generate' AND target_id = $1`, []any{draft}, &audits)
	if drafts < 1 || audits != 1 {
		t.Errorf("the run: %d drafts, %d audits", drafts, audits)
	}
	// once per month: a later tick generates nothing
	evaluate()
	if got := august(); len(got) != 1 || got[0].id != draft {
		t.Fatalf("regenerated: %+v", got)
	}
	// HQ approves; a later run of August leaves the approved statement
	var v int
	ownerScan(t, `SELECT version FROM billing.payout_statements WHERE id = $1`, []any{draft}, &v)
	if code, m := write(s, &hq, "payouts.transition", `{"statementId":"`+draft+`","action":"approve"}`, v); code != 200 {
		t.Fatalf("approve: %d %v", code, m)
	}
	rerun()
	evaluate()
	if got := august(); len(got) != 1 || got[0].id != draft || got[0].status != "approved" {
		t.Fatalf("the approved statement: %+v", got)
	}
}
