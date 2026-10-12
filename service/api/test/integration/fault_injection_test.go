package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

var errInjected = errors.New("injected database failure")

// failing is a transaction whose statements containing match fail, as a database error would; the rest run normally.
type failing struct {
	pgx.Tx
	match string
}

func (f failing) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(sql, f.match) {
		return pgconn.CommandTag{}, errInjected
	}
	return f.Tx.Exec(ctx, sql, args...)
}

// injectAt returns a server whose operations fail at the first statement containing match.
func injectAt(t *testing.T, match string) *apiserver.Server {
	t.Helper()
	return serverCfg(t, func(c *apiserver.Config) {
		c.WrapTx = func(tx pgx.Tx) pgx.Tx { return failing{Tx: tx, match: match} }
	})
}

// TestWithdrawRollsBackWhenTheJobCannotBeUpdated: a withdrawal whose job version update fails is not half done. The
// operation fails as a server error, and the proposal stays pending, because the database rolls the transaction back.
func TestWithdrawRollsBackWhenTheJobCannotBeUpdated(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = ANY($1) AND status = 'active' AND lower(scheduled) > $2`,
		[]uuid.UUID{seed.ID("tech-internal-a"), seed.ID("tech-external-a")}, clock.Add(2*time.Hour))
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	internalHold := `{"kind":"internal","membershipId":"` + seed.ID("tech-internal-a").String() + `"}`
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	if code, m := write(s, &hq, "jobs.proposeSlot", `{"jobId":"`+job+`","slot":`+slotJSON(97, 2)+`,"hold":`+internalHold+`,"message":"Can we come then?","replyBy":"`+ts(48)+`"}`, 1); code != 200 {
		t.Fatalf("propose: %d %v", code, m)
	}
	var proposal string
	ownerScan(t, `SELECT id::text FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending'`, []any{job}, &proposal)

	broken := injectAt(t, "UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1")
	code, m := write(broken, &hq, "jobs.withdrawProposal", `{"jobId":"`+job+`","proposalId":"`+proposal+`"}`, 2)
	if code < 500 {
		t.Fatalf("withdraw with a failing job update: %d %v, want a server error", code, m)
	}
	var status string
	ownerScan(t, `SELECT status FROM maintenance.slot_proposals WHERE id = $1`, []any{proposal}, &status)
	if status != "pending" {
		t.Errorf("a failed withdrawal left the proposal %s, want pending", status)
	}
}
