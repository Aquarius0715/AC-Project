package maintenance

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Access implements the IR94 technician write table for every module (alerts, devices, commands, diagnostics,
// jobs, attachments). Equipment calls it; every method asks maintenance (IR193).
type Access struct{}

// Internal queries behind Access (IR193).
const (
	QueryTechnicianJob  = "maintenance.technicianJob"
	QueryTechnicianUnit = "maintenance.technicianUnit"
	QueryJobVersion     = "maintenance.jobVersion"
)

// TechnicianJobInput is QueryTechnicianJob input.
type TechnicianJobInput struct {
	JobID  uuid.UUID `json:"jobId"`
	UnitID uuid.UUID `json:"unitId"`
}

// TechnicianUnitInput is QueryTechnicianUnit input.
type TechnicianUnitInput struct {
	UnitID   uuid.UUID `json:"unitId"`
	InScope  bool      `json:"inScope"`
	Internal bool      `json:"internal"`
}

// JobInput names a job.
type JobInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// JobVersionResult is QueryJobVersion output.
type JobVersionResult struct {
	Version int       `json:"version"`
	UnitID  uuid.UUID `json:"unitId"`
	Found   bool      `json:"found"`
}

func registerAccess(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryTechnicianJob, technicianJob)
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryTechnicianUnit, technicianUnit)
	ops.RegisterQuery(r, ops.DomainMaintenance, QueryJobVersion, jobVersion)
}

// TechnicianJob checks a technician write that names jobId (see technicianJob).
func (Access) TechnicianJob(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	unit uuid.UUID,
) error {
	_, err := ops.Delegate(
		ctx,
		call,
		QueryTechnicianJob,
		TechnicianJobInput{JobID: jobID, UnitID: unit},
		technicianJob,
	)

	return apperr.Wrap(err, "delegate maintenance.technicianJob")
}

// TechnicianUnit checks a technician write without jobId (see technicianUnit).
func (Access) TechnicianUnit(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	inScope bool,
	internal bool,
) error {
	_, err := ops.Delegate(
		ctx,
		call,
		QueryTechnicianUnit,
		TechnicianUnitInput{UnitID: unit, InScope: inScope, Internal: internal},
		technicianUnit,
	)

	return apperr.Wrap(err, "delegate maintenance.technicianUnit")
}

// JobVersion returns a job's version and unit (Control diagnostic runs).
func (Access) JobVersion(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (int, uuid.UUID, bool, error) {
	r, err := ops.Delegate(ctx, c, QueryJobVersion, JobInput{JobID: job}, jobVersion)

	return r.Version, r.UnitID, r.Found, apperr.Wrap(err, "delegate maintenance.jobVersion")
}

func forbidden(key string) error { return apperr.E(apperr.Forbidden, key) }
func notFound() error            { return apperr.E(apperr.NotFound, apperr.KeyNotFound) }

type assignment struct {
	unit                uuid.UUID
	status              string
	validFrom, validTil time.Time
	jobStatus           string
}

// technicianJob checks a technician write that names jobId: the user's active Assignment for the job, Job.unitId =
// target unit, and now inside the work window. Before the window → FORBIDDEN errors.assignment_not_started; ended or
// revoked after the start → FORBIDDEN errors.assignment_ended; never assigned / revoked before the start → NOT_FOUND.
func technicianJob(ctx context.Context, c *ops.Call, in *TechnicianJobInput) (struct{}, error) {
	return struct{}{}, checkTechnicianJob(ctx, c, in.JobID, in.UnitID)
}

func checkTechnicianJob(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	unit uuid.UUID,
) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT j.unit_id, a.status, a.valid_from, a.valid_until, j.status
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id
		WHERE a.job_id = $1 AND a.technician_membership_id = $2 ORDER BY a.created_at DESC`,
		jobID,
		call.Principal.MembershipID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.assignments")
	}

	var assignments []assignment

	for rows.Next() {
		var entry assignment

		err := rows.Scan(
			&entry.unit,
			&entry.status,
			&entry.validFrom,
			&entry.validTil,
			&entry.jobStatus,
		)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select maintenance.assignments")
		}

		assignments = append(assignments, entry)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select maintenance.assignments")
	}

	now := call.Now

	var started bool

	for _, entry := range assignments {
		if entry.unit != unit {
			return notFound() // the job is for another unit
		}

		if entry.status == "active" {
			switch {
			case now.Before(entry.validFrom):
				return forbidden("errors.assignment_not_started")
			case now.Before(entry.validTil):
				return nil
			default:
				return forbidden("errors.assignment_ended")
			}
		}

		if !now.Before(entry.validFrom) {
			started = true // revoked after its window started
		}
	}

	if started {
		return forbidden("errors.assignment_ended")
	}

	return notFound()
}

// technicianUnit checks a technician write without jobId (alerts.acknowledge/resolve, devices.addResponseNote): an
// active Assignment for the unit whose window contains now. Internal technicians with the unit in scope but no
// assignment get FORBIDDEN errors.assignment_required; everyone else NOT_FOUND.
func technicianUnit(ctx context.Context, c *ops.Call, in *TechnicianUnitInput) (struct{}, error) {
	return struct{}{}, checkTechnicianUnit(ctx, c, in.UnitID, in.InScope, in.Internal)
}

func checkTechnicianUnit(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	inScope bool,
	internal bool,
) error {
	var assigned, inWindow, notStarted bool

	err := call.Tx.QueryRow(ctx, `SELECT count(*) > 0,
		COALESCE(bool_or(a.status = 'active' AND a.valid_from <= $3 AND $3 < a.valid_until), false),
		COALESCE(bool_or(a.status = 'active' AND $3 < a.valid_from), false)
		FROM maintenance.assignments a JOIN maintenance.jobs j ON j.id = a.job_id
		WHERE j.unit_id = $1 AND a.technician_membership_id =
		$2`, unit, call.Principal.MembershipID, call.Now).Scan(&assigned, &inWindow, &notStarted)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(err, "select maintenance.assignments")
	}

	switch {
	case inWindow:
		return nil
	case notStarted:
		return forbidden("errors.assignment_not_started")
	case assigned:
		return forbidden("errors.assignment_ended")
	case internal && inScope:
		return forbidden("errors.assignment_required")
	}

	return notFound()
}

// InScope reports whether the unit is inside the membership scopes (organization, property or unit IDs); the caller
// passes the unit's organization and property.
func InScope(p *ops.Principal, unit, property, org uuid.UUID) bool {
	return slices.Contains(p.Scopes["unit"], unit) ||
		slices.Contains(p.Scopes["property"], property) ||
		slices.Contains(p.Scopes["organization"], org)
}

func jobVersion(ctx context.Context, c *ops.Call, in *JobInput) (JobVersionResult, error) {
	var jobVersionResult JobVersionResult

	err := c.Tx.QueryRow(ctx, `SELECT version, unit_id FROM maintenance.jobs WHERE id = $1`, in.JobID).
		Scan(&jobVersionResult.Version, &jobVersionResult.UnitID)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobVersionResult{Version: 0, UnitID: uuid.UUID{}, Found: false}, nil
	}

	jobVersionResult.Found = err == nil

	return jobVersionResult, apperr.Wrap(err, "select maintenance.jobs")
}
