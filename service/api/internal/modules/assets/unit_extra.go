package assets

import (
	"context"
	"errors"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// UnitUsage is implemented by modules whose records block physical deletion of a unit (contracts, jobs, IoT).
type UnitUsage interface {
	UnitInUse(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// UnitActivity is implemented by modules whose active records block units.archive (D05: active job, device binding or
// operation, restriction, command or run, unexpired contract).
type UnitActivity interface {
	UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// OrgActivity is implemented by modules whose active records block customer deactivation (D05).
type OrgActivity interface {
	OrgActive(ctx context.Context, c *ops.Call, org uuid.UUID) (bool, error)
}

// PolicyReader is what Assets needs from Monitoring for units.setAlertPolicies.
type PolicyReader interface {
	Owners(ctx context.Context, c *ops.Call, ids []uuid.UUID) (map[uuid.UUID]monitoring.PolicyOwner, error)
}

// DeleteInput is units.delete input.
type DeleteInput = ArchiveInput

// DeletedResource is DeletedResource of service-contracts.ts.
type DeletedResource struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// unitsDelete physically deletes a unit that was never linked to contracts, jobs or IoT (DD-A02); otherwise CONFLICT
// (archive it instead).
func (m *Module) unitsDelete(ctx context.Context, c *ops.Call, in *DeleteInput) (DeletedResource, error) {
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT version FROM assets.units WHERE id = $1 FOR UPDATE`, in.ID).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, notFound()
	}
	if err != nil {
		return DeletedResource{}, err
	}
	if v != *c.ExpectedVersion {
		return DeletedResource{}, conflict("error.versionConflict")
	}
	for _, u := range m.Usage {
		busy, err := u.UnitInUse(ctx, c, in.ID)
		if err != nil {
			return DeletedResource{}, err
		}
		if busy {
			return DeletedResource{}, conflict("error.unitInUse")
		}
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM assets.unit_alert_policies WHERE unit_id = $1`, in.ID); err != nil {
		return DeletedResource{}, err
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM assets.units WHERE id = $1`, in.ID); err != nil {
		return DeletedResource{}, err
	}
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: in.ID, Type: "UnitDeleted"})
	c.Audit(ops.AuditEntry{Action: "units.delete", TargetKind: "unit", TargetID: in.ID.String(), PreviousVersion: c.ExpectedVersion, Reason: in.Reason})
	return DeletedResource{ID: in.ID, Deleted: true}, nil
}

// SetPoliciesInput is units.setAlertPolicies input.
type SetPoliciesInput struct {
	UnitID         uuid.UUID   `json:"unitId"`
	AlertPolicyIDs []uuid.UUID `json:"alertPolicyIds"`
}

// Validate implements ops.Validator.
func (in *SetPoliciesInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	if in.AlertPolicyIDs == nil {
		fe["alertPolicyIds"] = "error.required"
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range in.AlertPolicyIDs {
		if seen[id] {
			fe["alertPolicyIds"] = "error.duplicate"
		}
		seen[id] = true
	}
	return fe
}

// unitsSetAlertPolicies replaces the unit's attached policies (IR108: only alert policies of the unit's customer;
// the default policy is implicit and cannot be listed or removed).
func (m *Module) unitsSetAlertPolicies(ctx context.Context, c *ops.Call, in *SetPoliciesInput) (Unit, error) {
	var org uuid.UUID
	var customerID *uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT u.customer_org_id, (SELECT id FROM assets.customers WHERE organization_id = u.customer_org_id)
		FROM assets.units u WHERE u.id = $1 AND NOT u.archived FOR UPDATE`, in.UnitID).Scan(&org, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, notFound()
	}
	if err != nil {
		return Unit{}, err
	}
	if c.Principal.Role == "client" && org != c.Principal.OrgID {
		return Unit{}, notFound()
	}
	owners, err := m.Policies.Owners(ctx, c, in.AlertPolicyIDs)
	if err != nil {
		return Unit{}, err
	}
	for _, id := range in.AlertPolicyIDs {
		o, ok := owners[id]
		switch { // IR108 / IR120 item 8
		case ok && o.Kind != "alert":
			return Unit{}, apperr.Fields(map[string]string{"alertPolicyIds": "error.defaultPolicyImplicit"})
		case !ok || o.CustomerID == nil || customerID == nil || *o.CustomerID != *customerID:
			return Unit{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	var v int
	err = c.Tx.QueryRow(ctx, `UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = $1 AND version = $2 RETURNING version`,
		in.UnitID, *c.ExpectedVersion).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, conflict("error.versionConflict")
	}
	if err != nil {
		return Unit{}, err
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM assets.unit_alert_policies WHERE unit_id = $1 AND NOT (policy_id = ANY($2))`, in.UnitID, in.AlertPolicyIDs); err != nil {
		return Unit{}, err
	}
	ids := append([]uuid.UUID(nil), in.AlertPolicyIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	for _, id := range ids {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)
			ON CONFLICT DO NOTHING`, in.UnitID, id); err != nil {
			return Unit{}, err
		}
	}
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: in.UnitID, Type: "UnitChanged"})
	c.Audit(ops.AuditEntry{Action: "units.setAlertPolicies", TargetKind: "unit", TargetID: in.UnitID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v})
	return m.load(ctx, c, in.UnitID)
}
