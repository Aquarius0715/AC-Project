package billing

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// QueryOverdue is the internal query of unpaid invoices past due for the read models (IR190, IR115).
const QueryOverdue = "billing.overdue"

// OverdueInput is QueryOverdue input.
type OverdueInput struct {
	UnitIDs []uuid.UUID `json:"unitIds"`
}

// Overdue is one currency of QueryOverdue.
type Overdue struct {
	Currency    string `json:"currency"`
	Count       int    `json:"count"`
	AmountMinor int64  `json:"amountMinor"`
}

// RegisterQueries binds billing's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainBilling, QueryOverdue, overdue)
	registerUsage(r)
}

// overdue sums unpaid invoices past due on contracts covering the units, by currency.
func overdue(ctx context.Context, c *ops.Call, in *OverdueInput) ([]Overdue, error) {
	rows, err := c.Tx.Query(ctx, `SELECT i.currency, count(*), sum(i.amount_minor) FROM billing.invoices i
		WHERE i.status = 'unpaid' AND i.due_at < $2
		  AND EXISTS (SELECT 1 FROM billing.contract_units cu WHERE cu.contract_id = i.contract_id AND cu.contract_version = i.contract_version AND cu.unit_id = ANY($1))
		GROUP BY i.currency ORDER BY i.currency`, in.UnitIDs, c.Now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Overdue{}
	for rows.Next() {
		var o Overdue
		if err := rows.Scan(&o.Currency, &o.Count, &o.AmountMinor); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
