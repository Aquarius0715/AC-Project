package maintenance

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// Access implements the IR94 technician write table for every module (alerts, devices, commands, diagnostics,
// jobs, attachments).
type Access struct{}

func forbidden(key string) error { return apperr.E(apperr.Forbidden, key) }
func notFound() error            { return apperr.E(apperr.NotFound, "error.notFound") }

type assignment struct {
	unit                uuid.UUID
	status              string
	validFrom, validTil time.Time
	jobStatus           string
}

// TechnicianJob checks a technician write that names jobId: the user's active Assignment for the job, Job.unitId =
// target unit, and now inside the work window. Before the window → FORBIDDEN errors.assignment_not_started; ended or
// revoked after the start → FORBIDDEN errors.assignment_ended; never assigned / revoked before the start → NOT_FOUND.
func (Access) TechnicianJob(ctx context.Context, c *ops.Call, jobID uuid.UUID, unit uuid.UUID) error {
	rows, err := c.Tx.Query(ctx, `SELECT j.unit_id, a.status, a.valid_from, a.valid_until, j.status
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id
		WHERE a.job_id = $1 AND a.technician_membership_id = $2 ORDER BY a.created_at DESC`, jobID, c.Principal.MembershipID)
	if err != nil {
		return err
	}
	var as []assignment
	for rows.Next() {
		var a assignment
		if err := rows.Scan(&a.unit, &a.status, &a.validFrom, &a.validTil, &a.jobStatus); err != nil {
			rows.Close()
			return err
		}
		as = append(as, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := c.Now
	var started bool
	for _, a := range as {
		if a.unit != unit {
			return notFound() // the job is for another unit
		}
		if a.status == "active" {
			switch {
			case now.Before(a.validFrom):
				return forbidden("errors.assignment_not_started")
			case now.Before(a.validTil):
				return nil
			default:
				return forbidden("errors.assignment_ended")
			}
		}
		if !now.Before(a.validFrom) {
			started = true // revoked after its window started
		}
	}
	if started {
		return forbidden("errors.assignment_ended")
	}
	return notFound()
}

// TechnicianUnit checks a technician write without jobId (alerts.acknowledge/resolve, devices.addResponseNote): an
// active Assignment for the unit whose window contains now. Internal technicians with the unit in scope but no
// assignment get FORBIDDEN errors.assignment_required; everyone else NOT_FOUND.
func (Access) TechnicianUnit(ctx context.Context, c *ops.Call, unit uuid.UUID, inScope bool, internal bool) error {
	var any, inWindow, notStarted bool
	err := c.Tx.QueryRow(ctx, `SELECT count(*) > 0,
		COALESCE(bool_or(a.status = 'active' AND a.valid_from <= $3 AND $3 < a.valid_until), false),
		COALESCE(bool_or(a.status = 'active' AND $3 < a.valid_from), false)
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id
		WHERE j.unit_id = $1 AND a.technician_membership_id = $2`, unit, c.Principal.MembershipID, c.Now).Scan(&any, &inWindow, &notStarted)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	switch {
	case inWindow:
		return nil
	case notStarted:
		return forbidden("errors.assignment_not_started")
	case any:
		return forbidden("errors.assignment_ended")
	case internal && inScope:
		return forbidden("errors.assignment_required")
	}
	return notFound()
}

// InScope reports whether the unit is inside the membership scopes (organization, property or unit IDs); the caller
// passes the unit's organization and property.
func InScope(p *ops.Principal, unit, property, org uuid.UUID) bool {
	return slices.Contains(p.Scopes["unit"], unit) || slices.Contains(p.Scopes["property"], property) || slices.Contains(p.Scopes["organization"], org)
}

// JobVersion returns a job's version and unit (Control diagnostic runs).
func (Access) JobVersion(ctx context.Context, c *ops.Call, job uuid.UUID) (int, uuid.UUID, bool, error) {
	var v int
	var unit uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT version, unit_id FROM maintenance.jobs WHERE id = $1`, job).Scan(&v, &unit)
	if err != nil && err.Error() == "no rows in result set" {
		return 0, unit, false, nil
	}
	return v, unit, err == nil, err
}
