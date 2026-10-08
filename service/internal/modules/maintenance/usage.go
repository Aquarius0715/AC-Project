// Package maintenance implements the Maintenance module.
package maintenance

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/ops"
)

// Usage reports whether a unit has maintenance jobs, which blocks physical deletion (DD-A02).
type Usage struct{}

// UnitInUse implements assets.UnitUsage.
func (Usage) UnitInUse(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1)`, unit).Scan(&b)
	return b, err
}

// ClaimableJobs returns, per unit, jobs that hold a warranty claim still in state "claimable" (DD-A19).
func (Usage) ClaimableJobs(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT unit_id, id FROM maintenance.jobs WHERE unit_id = ANY($1)
		AND EXISTS (SELECT 1 FROM jsonb_array_elements(warranty_claims) w WHERE w->>'state' = 'claimable') ORDER BY id`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u, j uuid.UUID
		if err := rows.Scan(&u, &j); err != nil {
			return nil, err
		}
		out[u] = append(out[u], j)
	}
	return out, rows.Err()
}

// UnitActive reports an open job on the unit (blocks units.archive, D05).
func (Usage) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1 AND status NOT IN ('completed','cancelled'))`, unit).Scan(&b)
	return b, err
}

// OrgActive reports open jobs for a customer organization (blocks customer deactivation, D05).
func (Usage) OrgActive(ctx context.Context, c *ops.Call, org uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE customer_org_id = $1 AND status NOT IN ('completed','cancelled'))`, org).Scan(&b)
	return b, err
}
