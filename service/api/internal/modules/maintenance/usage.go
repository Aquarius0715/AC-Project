// Package maintenance implements the Maintenance module.
package maintenance

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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
	// QueryTechnicianBooked answers whether a technician membership holds an active assignment whose access has not
	// ended: the contractor cannot disable or remove that account (IR421). Asked by identity by name.
	QueryTechnicianBooked = "maintenance.technicianBooked"
	// QueryBookedTechnicians answers which of the given technician memberships hold such an assignment: the
	// contractor's Users list marks them, so Disable and Remove are not offered (IR432).
	QueryBookedTechnicians = "maintenance.bookedTechnicians"
)

// MembershipsInput names memberships.
type MembershipsInput struct {
	MembershipIDs []uuid.UUID `json:"membershipIds"`
}

// MembershipInput names a membership.
type MembershipInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
}

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
	out, err := ops.Delegate(ctx, c, QueryUnitInUse, UnitInput{UnitID: unit}, unitInUse)

	return out, apperr.Wrap(err, "delegate maintenance.unitInUse")
}

// ClaimableJobs returns, per unit, the completed jobs whose accepted work report lists parts and that have no filed
// warranty claim yet, each with its completion time: Assets keeps those completed while the warranty ran (DD-A19,
// IR209).
func (Usage) ClaimableJobs(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]map[uuid.UUID]time.Time, error) {
	out, err := ops.Delegate(ctx, c, QueryClaimableJobs, UnitsInput{UnitIDs: units}, claimableJobs)

	return out, apperr.Wrap(err, "delegate maintenance.claimableJobs")
}

// UnitActive reports an open job on the unit (blocks units.archive, D05).
func (Usage) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	out, err := ops.Delegate(ctx, c, QueryUnitActive, UnitInput{UnitID: unit}, unitActive)

	return out, apperr.Wrap(err, "delegate maintenance.unitActive")
}

// OrgActive reports open jobs for a customer organization (blocks customer deactivation, D05).
func (Usage) OrgActive(ctx context.Context, c *ops.Call, org uuid.UUID) (bool, error) {
	out, err := ops.Delegate(ctx, c, QueryOrgActive, OrgInput{OrgID: org}, orgActive)

	return out, apperr.Wrap(err, "delegate maintenance.orgActive")
}

func registerUsage(reg *ops.Registry) {
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryUnitInUse, unitInUse)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryClaimableJobs, claimableJobs)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryUnitActive, unitActive)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryOrgActive, orgActive)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryTechnicianBooked, technicianBooked)
	ops.RegisterQuery(reg, ops.DomainMaintenance, QueryBookedTechnicians, bookedTechnicians)
}

func bookedTechnicians(
	ctx context.Context,
	call *ops.Call,
	input *MembershipsInput,
) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	if len(input.MembershipIDs) == 0 {
		return out, nil
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT DISTINCT technician_membership_id FROM maintenance.assignments
		WHERE technician_membership_id = ANY($1) AND status = 'active' AND valid_until > $2`,
		input.MembershipIDs,
		call.Now,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.assignments")
	}

	out2, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

	return out2, apperr.Wrap(err, "select maintenance.assignments")
}

func technicianBooked(ctx context.Context, c *ops.Call, in *MembershipInput) (bool, error) {
	var booked bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.assignments WHERE technician_membership_id = $1 AND
		status = 'active' AND valid_until > $2)`,
		in.MembershipID, c.Now).
		Scan(&booked)

	return booked, apperr.Wrap(err, "select maintenance.assignments")
}

func unitInUse(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1)`, in.UnitID).
		Scan(&b)

	return b, apperr.Wrap(err, "select maintenance.jobs")
}

func claimableJobs(
	ctx context.Context,
	c *ops.Call,
	input *UnitsInput,
) (map[uuid.UUID]map[uuid.UUID]time.Time, error) {
	out := map[uuid.UUID]map[uuid.UUID]time.Time{}

	rows, err := c.Tx.Query(
		ctx,
		`SELECT j.unit_id, j.id, j.completed_at FROM maintenance.jobs j WHERE j.unit_id = ANY($1) AND j.status = 'completed'
		AND j.completed_at IS NOT NULL AND EXISTS (SELECT 1 FROM maintenance.work_reports r WHERE r.job_id = j.id AND r.state
		= 'accepted' AND jsonb_array_length(r.parts) > 0)
		AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(j.warranty_claims) w WHERE w->>'state' = 'filed')`,
		input.UnitIDs,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.jobs")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			unit, job uuid.UUID
			when      time.Time
		)

		err := rows.Scan(&unit, &job, &when)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.jobs")
		}

		if out[unit] == nil {
			out[unit] = map[uuid.UUID]time.Time{}
		}

		out[unit][job] = when
	}

	return out, apperr.Wrap(rows.Err(), "select maintenance.jobs")
}

func unitActive(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var active bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1 AND status NOT IN
		('completed','cancelled'))`, in.UnitID).
		Scan(&active)

	return active, apperr.Wrap(err, "select maintenance.jobs")
}

func orgActive(ctx context.Context, c *ops.Call, in *OrgInput) (bool, error) {
	var active bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE customer_org_id = $1 AND status NOT IN
		('completed','cancelled'))`, in.OrgID).
		Scan(&active)

	return active, apperr.Wrap(err, "select maintenance.jobs")
}
