// Package maintenance implements the Maintenance module.
package maintenance

import (
	"context"
	"time"

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

// ClaimableJobs returns, per unit, the completed jobs whose accepted work report lists parts and that have no filed warranty
// claim yet, each with its completion time: Assets keeps those completed while the warranty ran (DD-A19, IR209).
func (Usage) ClaimableJobs(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]map[uuid.UUID]time.Time, error) {
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

func claimableJobs(ctx context.Context, c *ops.Call, in *UnitsInput) (map[uuid.UUID]map[uuid.UUID]time.Time, error) {
	out := map[uuid.UUID]map[uuid.UUID]time.Time{}
	rows, err := c.Tx.Query(ctx, `SELECT j.unit_id, j.id, j.completed_at FROM maintenance.jobs j WHERE j.unit_id = ANY($1) AND j.status = 'completed'
		AND j.completed_at IS NOT NULL AND EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state = 'accepted' AND jsonb_array_length(r.parts) > 0)
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(j.warranty_claims) w WHERE w->>'state' = 'filed')`, in.UnitIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u, j uuid.UUID
		var at time.Time
		if err := rows.Scan(&u, &j, &at); err != nil {
			return nil, err
		}
		if out[u] == nil {
			out[u] = map[uuid.UUID]time.Time{}
		}
		out[u][j] = at
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
