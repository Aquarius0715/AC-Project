package maintenance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Internal queries maintenance answers for the read models (IR190).
const (
	QueryCountJobs    = "maintenance.countJobs"
	QueryStatusCounts = "maintenance.statusCounts"
)

// CountJobsInput is QueryCountJobs input: the jobs.list filters of summaries.get.
type CountJobsInput struct {
	Filters json.RawMessage `json:"filters" swaggertype:"object"`
}

// StatusCountsInput is QueryStatusCounts input.
type StatusCountsInput struct {
	UnitIDs []uuid.UUID `json:"unitIds"`
	From    time.Time   `json:"from"`
	To      time.Time   `json:"to"`
}

// RegisterQueries binds maintenance's internal queries.
func RegisterQueries(reg *ops.Registry, m Jobs) {
	ops.RegisterQuery(
		reg,
		ops.DomainMaintenance,
		QueryCountJobs,
		func(ctx context.Context, c *ops.Call, in *CountJobsInput) (JobCounts, error) {
			return m.CountJobs(ctx, c, in.Filters)
		},
	)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryStatusCounts, statusCounts)
	registerUsage(reg)
	registerAccess(reg)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryQrAssignment, qrAssignment)
}

// statusCounts counts the jobs of the units whose requested slot starts in [from, to), by status (admin.summary).
func statusCounts(
	ctx context.Context,
	c *ops.Call,
	input *StatusCountsInput,
) (map[string]int, error) {
	out := map[string]int{}

	rows, err := c.Tx.Query(
		ctx,
		`SELECT status, count(*) FROM maintenance.jobs WHERE unit_id = ANY($1) AND lower(requested_slot) >= $2 AND
			lower(requested_slot) < $3 GROUP BY status`,
		input.UnitIDs,
		input.From,
		input.To,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.jobs")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			s string
			k int
		)

		err := rows.Scan(&s, &k)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.jobs")
		}

		out[s] = k
	}

	return out, apperr.Wrap(rows.Err(), "select maintenance.jobs")
}
