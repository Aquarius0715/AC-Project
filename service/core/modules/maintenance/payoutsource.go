package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/ops"
)

// AcceptedJob is a delivered job whose report was accepted (Billing payouts, IR137).
type AcceptedJob struct {
	JobID, ContractorOrgID uuid.UUID
	Type                   string
	AcceptedAt             time.Time
	Returned               bool
}

// PayoutSource answers payout questions for Billing.
type PayoutSource struct {
	Jobs Jobs
}

// AcceptedJobs returns contractor-delivered jobs whose latest report was accepted in [from, to).
func (PayoutSource) AcceptedJobs(ctx context.Context, c *ops.Call, from, to time.Time) ([]AcceptedJob, error) {
	rows, err := c.Tx.Query(ctx, `SELECT j.id, j.contractor_org_id, j.type, w.accepted_at,
		EXISTS (SELECT 1 FROM maintenance.report_reviews rr WHERE rr.report_id = w.id AND rr.decision = 'return')
		FROM maintenance.jobs j JOIN maintenance.work_reports w ON w.job_id = j.id AND w.state = 'accepted'
		WHERE j.contractor_org_id IS NOT NULL AND w.accepted_at >= $1 AND w.accepted_at < $2 ORDER BY j.contractor_org_id, w.accepted_at, j.id`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AcceptedJob
	for rows.Next() {
		var a AcceptedJob
		if err := rows.Scan(&a.JobID, &a.ContractorOrgID, &a.Type, &a.AcceptedAt, &a.Returned); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RateAt returns the rate card lines (workType → amount) and currency effective for a contractor at a time.
func (PayoutSource) RateAt(ctx context.Context, c *ops.Call, contractor uuid.UUID, at time.Time) (map[string]int64, string, bool, error) {
	var raw []byte
	var currency string
	err := c.Tx.QueryRow(ctx, `SELECT lines, currency FROM maintenance.rate_cards WHERE contractor_org_id = $1 AND effective_from <= $2 ORDER BY effective_from DESC LIMIT 1`,
		contractor, at).Scan(&raw, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	var lines []RateLine
	if err := json.Unmarshal(raw, &lines); err != nil {
		return nil, "", false, err
	}
	out := map[string]int64{}
	for _, l := range lines {
		out[l.WorkType] = int64(l.AmountMinor)
	}
	return out, currency, true, nil
}

// AddJobNote adds an internal note with a note.added event to a job's history (payout questions and replies).
func (s PayoutSource) AddJobNote(ctx context.Context, c *ops.Call, job uuid.UUID, message string) error {
	var note uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.job_notes (tenant_id, job_id, author_id, visibility, message, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'internal', $3, $4) RETURNING id`, job, c.Principal.UserID, message, c.Now).Scan(&note); err != nil {
		return err
	}
	return s.Jobs.event(ctx, c, job, "note.added", &note)
}
