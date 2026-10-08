package summaries

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/modules/energy"
	"github.com/pradita/ac-project/service/internal/ops"
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
	// customers: Customer and Organization both active (IR40); filters narrow to the matching customer
	cargs := []any{}
	cadd := func(v any) string { cargs = append(cargs, v); return fmt.Sprintf("$%d", len(cargs)) }
	cconds := []string{"cu.status = 'active'", "o.status = 'active'"}
	if in.CustomerID != nil {
		cconds = append(cconds, "cu.id = "+cadd(*in.CustomerID))
	}
	if in.PropertyID != nil {
		cconds = append(cconds, "cu.organization_id = (SELECT customer_org_id FROM assets.properties WHERE id = "+cadd(*in.PropertyID)+")")
	}
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM assets.customers cu JOIN identity.organizations o ON o.id = cu.organization_id WHERE "+strings.Join(cconds, " AND "), cargs...).
		Scan(&out.CustomerCount); err != nil {
		return out, err
	}
	// jobs whose requested slot starts in the period, by status
	rows, err = c.Tx.Query(ctx, `SELECT status, count(*) FROM maintenance.jobs WHERE unit_id = ANY($1) AND lower(requested_slot) >= $2 AND lower(requested_slot) < $3 GROUP BY status`,
		units, in.From, in.To)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var s string
		var k int
		if err := rows.Scan(&s, &k); err != nil {
			rows.Close()
			return out, err
		}
		out.JobCounts[s] = k
	}
	rows.Close()
	// billing (billing.read; IR115): unpaid invoices past due on contracts covering the units
	if c.Principal.Permissions["billing.read"] {
		out.BillingVisibility, out.AmountsByCurrency = "allowed", []Money{}
		rows, err := c.Tx.Query(ctx, `SELECT i.currency, count(*), sum(i.amount_minor) FROM billing.invoices i
			WHERE i.status = 'unpaid' AND i.due_at < $2
			  AND EXISTS (SELECT 1 FROM billing.contract_units cu WHERE cu.contract_id = i.contract_id AND cu.contract_version = i.contract_version AND cu.unit_id = ANY($1))
			GROUP BY i.currency ORDER BY i.currency`, units, c.Now)
		if err != nil {
			return out, err
		}
		total := 0
		for rows.Next() {
			var mny Money
			var k int
			if err := rows.Scan(&mny.Currency, &k, &mny.AmountMinor); err != nil {
				rows.Close()
				return out, err
			}
			total += k
			out.AmountsByCurrency = append(out.AmountsByCurrency, mny)
		}
		rows.Close()
		out.OverdueInvoiceCount = &total
	}
	// energy: actuals only (no baseline) and the IR78 forecast
	f, err := energy.DefaultFactor(ctx, c)
	if err != nil {
		return out, err
	}
	s, integ, err := energy.Compute(ctx, c, units, in.From, in.To, f, nil)
	if err != nil {
		return out, err
	}
	out.EnergySummary = &s
	out.EnergyForecast, err = energy.ForecastFor(ctx, c, units, in.From, in.To, integ)
	return out, err
}

// RegisterAdmin binds admin.summary.
func RegisterAdmin(r *ops.Registry, m Summaries) {
	ops.Register(r, "admin.summary", m.admin)
}
