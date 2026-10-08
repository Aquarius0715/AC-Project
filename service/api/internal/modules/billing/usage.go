// Package billing implements the Billing module.
package billing

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Usage reports whether a unit is covered by a contract, which blocks physical deletion (DD-A02). Equipment calls
// it; every method asks billing (IR193).
type Usage struct{}

// Internal queries behind Usage (IR193).
const (
	QueryUnitInUse       = "billing.unitInUse"
	QueryActiveContracts = "billing.activeContracts"
	QueryUnitActive      = "billing.unitActive"
	QueryOrgActive       = "billing.orgActive"
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

// ActiveContracts returns, per unit, the IDs of current contracts whose term contains now (coverage, DD-A19).
func (Usage) ActiveContracts(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	return ops.Delegate(ctx, c, QueryActiveContracts, UnitsInput{UnitIDs: units}, activeContracts)
}

// UnitActive reports a current contract on the unit whose term has not ended (blocks units.archive, D05).
func (Usage) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryUnitActive, UnitInput{UnitID: unit}, unitActive)
}

// OrgActive reports an unexpired current contract of the customer organization.
func (Usage) OrgActive(ctx context.Context, c *ops.Call, org uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryOrgActive, OrgInput{OrgID: org}, orgActive)
}

func registerUsage(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitInUse, unitInUse)
	ops.RegisterQuery(r, ops.DomainBilling, QueryActiveContracts, activeContracts)
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitActive, unitActive)
	ops.RegisterQuery(r, ops.DomainBilling, QueryOrgActive, orgActive)
}

func unitInUse(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.contract_units WHERE unit_id = $1)`, in.UnitID).Scan(&b)
	return b, err
}

func activeContracts(ctx context.Context, c *ops.Call, in *UnitsInput) (map[uuid.UUID][]uuid.UUID, error) {
	out := map[uuid.UUID][]uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT cu.unit_id, k.id FROM billing.contract_units cu
		JOIN billing.contracts k ON k.id = cu.contract_id AND k.version = cu.contract_version
		WHERE k.is_current AND k.term @> $2::timestamptz AND cu.unit_id = ANY($1) ORDER BY k.id`, in.UnitIDs, c.Now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u, k uuid.UUID
		if err := rows.Scan(&u, &k); err != nil {
			return nil, err
		}
		out[u] = append(out[u], k)
	}
	return out, rows.Err()
}

func unitActive(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.contract_units cu JOIN billing.contracts k ON k.id = cu.contract_id AND k.version = cu.contract_version
		WHERE cu.unit_id = $1 AND k.is_current AND upper(k.term) > $2)`, in.UnitID, c.Now).Scan(&b)
	return b, err
}

func orgActive(ctx context.Context, c *ops.Call, in *OrgInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM billing.contracts WHERE customer_org_id = $1 AND is_current AND upper(term) > $2)`, in.OrgID, c.Now).Scan(&b)
	return b, err
}
