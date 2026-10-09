package summaries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/billing"
	"github.com/pradita/ac-project/service/api/internal/modules/energy"
	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// AdminInput is admin.summary input (Range & {customerId?, propertyId?}).
type AdminInput struct {
	From       time.Time  `json:"from"`
	To         time.Time  `json:"to"`
	CustomerID *uuid.UUID `json:"customerId,omitempty"`
	PropertyID *uuid.UUID `json:"propertyId,omitempty"`
}

// Validate implements ops.Validator: an energy range (minute-aligned, from < to, at most 366 days; SR17, IR74).
func (in *AdminInput) Validate() map[string]string {
	if energy.CheckRange(in.From, in.To) != nil {
		return map[string]string{"from": "errors.energy_range"}
	}
	return nil
}

// Money is Money.
type Money struct {
	AmountMinor int64  `json:"amountMinor"`
	Currency    string `json:"currency"`
}

// AdminSummary is AdminSummary of service-contracts.ts.
type AdminSummary struct {
	CustomerCount       int             `json:"customerCount"`
	Total               int             `json:"total"`
	Online              int             `json:"online"`
	Offline             int             `json:"offline"`
	Unknown             int             `json:"unknown"`
	PowerOn             int             `json:"powerOn"`
	PowerOff            int             `json:"powerOff"`
	PowerUnknown        int             `json:"powerUnknown"`
	OperatingRate       *float64        `json:"operatingRate"`
	AlertCount          int             `json:"alertCount"`
	JobCounts           map[string]int  `json:"jobCounts"`
	OverdueInvoiceCount *int            `json:"overdueInvoiceCount"`
	AmountsByCurrency   []Money         `json:"amountsByCurrency"`
	BillingVisibility   string          `json:"billingVisibility"`
	EnergySummary       *energy.Summary `json:"energySummary"`
	EnergyForecast      energy.Forecast `json:"energyForecast"`
	AsOf                time.Time       `json:"asOf"`
}

var jobStatuses = []string{"requested", "offered", "accepted", "assigned", "in_progress", "submitted", "completed", "rework_requested", "cancelled", "on_hold"}

func collectIDs(rows pgx.Rows) ([]uuid.UUID, error) {
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// admin is admin.summary (DD-A01, IR40, IR51, IR78, IR115, IR148).
//
//	@Summary		admin.summary (read)
//	@ID				admin.summary
//	@Description	Authorization: admin:dashboard.read; billing fields require billing.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR51 alertCount critical/warning; IR78 energyForecast (prorated modeled baseline); energySummary savings null
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A01
//	@Tags			admin
//	@Accept			json
//	@Produce		json
//	@Param			request	body		AdminInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=AdminSummary}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/ops/admin.summary [post]
func (m Summaries) admin(ctx context.Context, c *ops.Call, in *AdminInput) (AdminSummary, error) {
	out := AdminSummary{AsOf: c.Now, JobCounts: map[string]int{}, BillingVisibility: "forbidden"}
	for _, s := range jobStatuses {
		out.JobCounts[s] = 0
	}
	// target units U: not archived, matching the filters
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT u.archived"}
	if in.CustomerID != nil {
		conds = append(conds, "u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(*in.CustomerID)+")")
	}
	if in.PropertyID != nil {
		conds = append(conds, "u.property_id = "+add(*in.PropertyID))
	}
	rows, err := c.Tx.Query(ctx, "SELECT u.id FROM assets.units u WHERE "+strings.Join(conds, " AND ")+" ORDER BY u.id", args...)
	if err != nil {
		return out, err
	}
	units, err := collectIDs(rows)
	if err != nil {
		return out, err
	}
	n, err := m.Units.CountUnits(ctx, c, true, nil, in.CustomerID, in.PropertyID, nil)
	if err != nil {
		return out, err
	}
	out.Total, out.Online, out.Offline, out.Unknown = n.Total, n.Online, n.Offline, n.Unknown
	out.PowerOn, out.PowerOff, out.PowerUnknown, out.AlertCount = n.PowerOn, n.PowerOff, n.PowerUnknown, n.AlertCount
	if d := n.PowerOn + n.PowerOff; d > 0 { // unknown power never counts as inactive (DD-A01)
		r := float64(n.PowerOn) / float64(d) * 100
		out.OperatingRate = &r
	}
	// customers: Customer and Organization both active (IR40; organization status from identity); filters narrow to the
	// matching customer
	cargs := []any{}
	cadd := func(v any) string { cargs = append(cargs, v); return fmt.Sprintf("$%d", len(cargs)) }
	cconds := []string{"cu.status = 'active'"}
	if in.CustomerID != nil {
		cconds = append(cconds, "cu.id = "+cadd(*in.CustomerID))
	}
	if in.PropertyID != nil {
		cconds = append(cconds, "cu.organization_id = (SELECT customer_org_id FROM assets.properties WHERE id = "+cadd(*in.PropertyID)+")")
	}
	rows, err = c.Tx.Query(ctx, "SELECT DISTINCT cu.organization_id FROM assets.customers cu WHERE "+strings.Join(cconds, " AND "), cargs...)
	if err != nil {
		return out, err
	}
	orgs, err := collectIDs(rows)
	if err != nil {
		return out, err
	}
	active, err := ops.Ask[[]uuid.UUID](ctx, m.Registry, c, identity.QueryActiveOrganizations, identity.OrganizationsInput{IDs: orgs})
	if err != nil {
		return out, err
	}
	if len(active) > 0 {
		if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM assets.customers cu WHERE cu.organization_id = ANY("+cadd(active)+") AND "+strings.Join(cconds, " AND "), cargs...).
			Scan(&out.CustomerCount); err != nil {
			return out, err
		}
	}
	// jobs whose requested slot starts in the period, by status (maintenance)
	counts, err := ops.Ask[map[string]int](ctx, m.Registry, c, maintenance.QueryStatusCounts, maintenance.StatusCountsInput{UnitIDs: units, From: in.From, To: in.To})
	if err != nil {
		return out, err
	}
	for st, k := range counts {
		out.JobCounts[st] = k
	}
	// billing (billing.read; IR115): unpaid invoices past due on contracts covering the units
	if c.Principal.Permissions["billing.read"] {
		overdue, err := ops.Ask[[]billing.Overdue](ctx, m.Registry, c, billing.QueryOverdue, billing.OverdueInput{UnitIDs: units})
		if err != nil {
			return out, err
		}
		out.BillingVisibility, out.AmountsByCurrency = "allowed", []Money{}
		total := 0
		for _, o := range overdue {
			total += o.Count
			out.AmountsByCurrency = append(out.AmountsByCurrency, Money{AmountMinor: o.AmountMinor, Currency: o.Currency})
		}
		out.OverdueInvoiceCount = &total
	}
	// energy: actuals only (no baseline) and the IR78 forecast
	en, err := ops.Ask[energy.Actuals](ctx, m.Registry, c, energy.QueryActuals, energy.ActualsInput{UnitIDs: units, From: in.From, To: in.To})
	if err != nil {
		return out, err
	}
	out.EnergySummary, out.EnergyForecast = &en.Summary, en.Forecast
	return out, nil
}

// RegisterAdmin binds admin.summary.
func RegisterAdmin(r *ops.Registry, m Summaries) {
	ops.Register(r, "admin.summary", m.admin)
}
