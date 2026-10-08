// Package summaries implements summaries.get (FR-C01/C08/P01/T01, IR26/IR44/IR49/IR51, IR146).
package summaries

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Units counts equipment (Assets).
type Units interface {
	CountUnits(ctx context.Context, c *ops.Call, scoped bool, ids *[]uuid.UUID, customer, property, unit *uuid.UUID) (assets.UnitCounts, error)
}

// Summaries is the summaries.get and admin.summary read models. Units are equipment's own; jobs, billing,
// energy and organization status come from their owners' internal queries through Registry (IR190).
type Summaries struct {
	Units    Units
	Registry *ops.Registry
}

// Input is summaries.get input.
type Input struct {
	Kind    string          `json:"kind"`
	Filters json.RawMessage `json:"filters"`
}

// Validate implements ops.Validator.
func (in *Input) Validate() map[string]string {
	if in.Kind != "customer" && in.Kind != "partner" && in.Kind != "technician" {
		return map[string]string{"kind": "error.invalid"}
	}
	return nil
}

// Counts is Summary.counts.
type Counts struct {
	Total           int `json:"total"`
	Online          int `json:"online"`
	Offline         int `json:"offline"`
	Unknown         int `json:"unknown"`
	PowerOn         int `json:"powerOn"`
	PowerOff        int `json:"powerOff"`
	PowerUnknown    int `json:"powerUnknown"`
	AlertCount      int `json:"alertCount"`
	OfferCount      int `json:"offerCount"`
	ActiveCount     int `json:"activeCount"`
	ReviewCount     int `json:"reviewCount"`
	ScheduledCount  int `json:"scheduledCount"`
	InProgressCount int `json:"inProgressCount"`
	OverdueCount    int `json:"overdueCount"`
	AssignedCount   int `json:"assignedCount"`
}

// Summary is Summary of service-contracts.ts.
type Summary struct {
	Kind   string    `json:"kind"`
	Counts Counts    `json:"counts"`
	AsOf   time.Time `json:"asOf"`
}

var roleKind = map[string]string{"client": "customer", "contractor": "partner", "technician": "technician"}

func (m Summaries) get(ctx context.Context, c *ops.Call, in *Input) (Summary, error) {
	if roleKind[c.Principal.Role] != in.Kind {
		return Summary{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	out := Summary{Kind: in.Kind, AsOf: c.Now}
	if in.Kind == "customer" { // equipment metrics only; job conditions are VALIDATION (IR26)
		var f struct {
			CustomerID *uuid.UUID   `json:"customerId,omitempty"`
			PropertyID *uuid.UUID   `json:"propertyId,omitempty"`
			UnitID     *uuid.UUID   `json:"unitId,omitempty"`
			UnitIDs    *[]uuid.UUID `json:"unitIds,omitempty"`
			From       *time.Time   `json:"from,omitempty"`
			To         *time.Time   `json:"to,omitempty"`
		}
		if len(in.Filters) > 0 && string(in.Filters) != "null" {
			dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
			dec.DisallowUnknownFields()
			if dec.Decode(&f) != nil {
				return Summary{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
			}
		}
		if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
			return Summary{}, apperr.Fields(map[string]string{"filters.to": "error.range"})
		}
		n, err := m.Units.CountUnits(ctx, c, true, f.UnitIDs, f.CustomerID, f.PropertyID, f.UnitID)
		if err != nil {
			return Summary{}, err
		}
		out.Counts = Counts{Total: n.Total, Online: n.Online, Offline: n.Offline, Unknown: n.Unknown, PowerOn: n.PowerOn, PowerOff: n.PowerOff,
			PowerUnknown: n.PowerUnknown, AlertCount: n.AlertCount}
		return out, nil
	}
	j, err := ops.Ask[maintenance.JobCounts](ctx, m.Registry, c, maintenance.QueryCountJobs, maintenance.CountJobsInput{Filters: in.Filters})
	if err != nil {
		return Summary{}, err
	}
	out.Counts = Counts{OfferCount: j.Offer, ActiveCount: j.Active, ReviewCount: j.Review, ScheduledCount: j.Scheduled, InProgressCount: j.InProgress,
		OverdueCount: j.Overdue, AssignedCount: j.Assigned}
	if in.Kind == "technician" { // assigned-equipment counts deduplicate the units of current rows (IR26, IR49)
		ids := j.Units
		if ids == nil {
			ids = []uuid.UUID{}
		}
		n, err := m.Units.CountUnits(ctx, c, false, &ids, nil, nil, nil)
		if err != nil {
			return Summary{}, err
		}
		out.Counts.Total, out.Counts.Online, out.Counts.Offline, out.Counts.Unknown = n.Total, n.Online, n.Offline, n.Unknown
	}
	return out, nil
}

// Register binds summaries.get.
func Register(r *ops.Registry, m Summaries) { ops.Register(r, "summaries.get", m.get) }
