package maintenance

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// JobCounts is the job part of Summary.counts for partners and technicians (IR26, IR49, IR146).
type JobCounts struct {
	Offer, Active, Review, Scheduled, InProgress, Overdue, Assigned int
	// Units are the distinct units of the current (non-terminal) summary rows.
	Units []uuid.UUID
}

// CountJobs applies the jobs.list filters and projections (IR26) and counts the caller's rows: offers are open
// Offer projections (contractors); among current summary rows scheduled = accepted+assigned, inProgress =
// in_progress, active = accepted+assigned+in_progress, review = submitted (D07), overdue = dueAt < now while not
// completed/cancelled, assigned = rows with an active Assignment.
func (m Jobs) CountJobs(ctx context.Context, c *ops.Call, raw json.RawMessage) (JobCounts, error) {
	var f listFilters
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return JobCounts{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if err := f.check(); err != nil {
		return JobCounts{}, err
	}
	items, err := m.collect(ctx, c, &f)
	if err != nil {
		return JobCounts{}, err
	}
	var n JobCounts
	seen := map[uuid.UUID]bool{}
	for _, it := range items {
		switch x := it.item.(type) {
		case OfferSummary:
			if x.Status == "offered" {
				n.Offer++
			}
		case Summary:
			if x.Status == "completed" || x.Status == "cancelled" {
				continue
			}
			switch x.Status { // D07 / SR KPI definitions
			case "accepted", "assigned":
				n.Active++
				n.Scheduled++
			case "in_progress":
				n.Active++
				n.InProgress++
			case "submitted":
				n.Review++
			}
			if x.DueAt.Before(c.Now) {
				n.Overdue++
			}
			if x.AssignmentID != nil {
				n.Assigned++
			}
			if !seen[x.UnitID] {
				seen[x.UnitID] = true
				n.Units = append(n.Units, x.UnitID)
			}
		}
	}
	return n, nil
}
