// Package maintenance implements the Maintenance module.
package maintenance

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Usage reports whether a unit has maintenance jobs, which blocks physical deletion (DD-A02). Equipment calls it;
// every method asks maintenance (IR193).
type Usage struct{}

// Internal queries behind Usage (IR193).
const (
	QueryUnitInUse     = "maintenance.unitInUse"
	QueryClaimableJobs = "maintenance.claimableJobs"
	QueryUnitActive    = "maintenance.unitActive"
	QueryOrgActive     = "maintenance.orgActive"
)

// UnitInput names a unit.
type UnitInput struct {
	UnitID uuid.UUID `json:"unitId"`
}

// UnitsInput names units.
type UnitsInput struct {
	UnitIDs []uuid.UUID `json:"unitIds"`
}

// OrgInput names an organization.
type OrgInput struct {
	OrgID uuid.UUID `json:"orgId"`
}

// UnitInUse implements assets.UnitUsage.
func (Usage) UnitInUse(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryUnitInUse, UnitInput{UnitID: unit}, unitInUse)
}

// ClaimableJobs returns, per unit, jobs that hold a warranty claim still in state "claimable" (DD-A19).
func (Usage) ClaimableJobs(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return ops.Delegate(ctx, c, QueryClaimableJobs, UnitsInput{UnitIDs: units}, claimableJobs)
}

// UnitActive reports an open job on the unit (blocks units.archive, D05).
func (Usage) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryUnitActive, UnitInput{UnitID: unit}, unitActive)
}

// OrgActive reports open jobs for a customer organization (blocks customer deactivation, D05).
func (Usage) OrgActive(ctx context.Context, c *ops.Call, org uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryOrgActive, OrgInput{OrgID: org}, orgActive)
}

func registerUsage(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryUnitInUse, unitInUse)
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryClaimableJobs, claimableJobs)
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryUnitActive, unitActive)
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryOrgActive, orgActive)
}

func unitInUse(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1)`, in.UnitID).Scan(&b)
	return b, err
}

func claimableJobs(ctx context.Context, c *ops.Call, in *UnitsInput) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT unit_id, id FROM maintenance.jobs WHERE unit_id = ANY($1)
		AND EXISTS (SELECT 1 FROM jsonb_array_elements(warranty_claims) w WHERE w->>'state' = 'claimable') ORDER BY id`, in.UnitIDs)
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

func unitActive(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1 AND status NOT IN ('completed','cancelled'))`, in.UnitID).Scan(&b)
	return b, err
}

func orgActive(ctx context.Context, c *ops.Call, in *OrgInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE customer_org_id = $1 AND status NOT IN ('completed','cancelled'))`, in.OrgID).Scan(&b)
	return b, err
}
