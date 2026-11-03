package maintenance

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ReleaseEndedAccess is the maintenance scheduler job of OPEN-BE-09 (IR439). A technician whose access ends before an
// assigned visit ends could not open the job: HQ ended or shortened the membership (Access & roles), which the
// contractor cannot do while the technician is booked (IR421). The assignment of such a job that has not started is
// released at once, as completion releases assignments (DEC-73): the job goes back to the contractor (accepted) or,
// for HQ's own delivery, to requested, keeping its agreed visit, and the people who assign get a job_update notice so
// they choose another technician. A job already started keeps its assignment for HQ to handle.
func (m Jobs) ReleaseEndedAccess(ctx context.Context, call *ops.Call) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.id, a.job_id, j.contractor_org_id FROM maintenance.assignments a
		JOIN maintenance.jobs j ON j.id = a.job_id AND j.assignment_id = a.id AND j.status = 'assigned'
		JOIN maintenance.ref_memberships rm ON rm.id = a.technician_membership_id
		WHERE a.status = 'active' AND rm.valid_until IS NOT NULL AND rm.valid_until < a.valid_until
		ORDER BY a.job_id FOR UPDATE OF a, j SKIP LOCKED`,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select maintenance.assignments")
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (released, error) {
		var e released

		return e, r.Scan(&e.assignment, &e.job, &e.contractor)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select maintenance.assignments")
	}

	for _, e := range list {
		err := releaseAssignment(ctx, call, e, accessEnded)
		if err != nil {
			return 0, err
		}
	}

	return len(list), nil
}

// releaseQualificationEnded is IR440: certificates.verify approved a certificate whose window [from, until) becomes the
// technician's grant for code, replacing the earlier one (identity applies it from the event, IR192). Every not-started
// assignment of the technician on a unit that requires code and that the new window does not cover (IR123 item 3, the
// check of every assignment) is released like an ended access (IR439).
func releaseQualificationEnded(
	ctx context.Context,
	call *ops.Call,
	units ServiceUnits,
	membership uuid.UUID,
	code string,
	from, until time.Time,
) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.id, a.job_id, j.contractor_org_id, j.unit_id FROM maintenance.assignments a
		JOIN maintenance.jobs j ON j.id = a.job_id AND j.assignment_id = a.id AND j.status = 'assigned'
		WHERE a.status = 'active' AND a.technician_membership_id = $1 AND NOT (a.valid_from >= $2 AND a.valid_until <= $3)
		ORDER BY a.job_id FOR UPDATE OF a, j`,
		membership,
		from,
		until,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.assignments")
	}

	type candidate struct {
		released

		unit uuid.UUID
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (candidate, error) {
		var e candidate

		return e, r.Scan(&e.assignment, &e.job, &e.contractor, &e.unit)
	})
	if err != nil {
		return apperr.Wrap(err, "select maintenance.assignments")
	}

	for _, target := range list {
		_, _, scope, err := units.UnitService(ctx, call, target.unit)
		if err != nil {
			return apperr.Wrap(err, "maintenance.ServiceUnits.UnitService")
		}

		if !slices.Contains(requiredCodes(scope), code) {
			continue
		}

		releaseAssignmentErr := releaseAssignment(ctx, call, target.released, qualificationEnded)
		if releaseAssignmentErr != nil {
			return releaseAssignmentErr
		}
	}

	return nil
}

// released is an assignment to release and its job.
type released struct {
	assignment, job uuid.UUID
	contractor      *uuid.UUID
}

// releaseCause names why an assignment is released: the assignment's reason, the job event, the notice's reason
// parameter and the audit reason.
type releaseCause struct{ reason, event, notice, audit string }

var (
	accessEnded = releaseCause{
		"access_ended",
		"job.assignment_released",
		"technician_access_ended",
		"technician access ended before the visit",
	}
	qualificationEnded = releaseCause{
		"qualification_ended",
		"job.qualification_released",
		"technician_qualification_ended",
		"technician qualification does not cover the visit",
	}
)

// releaseAssignment revokes the assignment and returns its job to be assigned again with its agreed visit (IR439):
// accepted for a contractor's job, requested for HQ's own delivery. The job's assigners get a job_update notice.
func releaseAssignment(
	ctx context.Context,
	call *ops.Call,
	target released,
	why releaseCause,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.assignments SET status = 'revoked', reason = $3, version = version + 1, updated_at = $2 WHERE id =
			$1`,
		target.assignment,
		call.Now,
		why.reason,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.assignments")
	}

	status := "requested" // HQ's own delivery assigns from requested
	if target.contractor != nil {
		status = "accepted" // the contractor assigns again on its accepted offer
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = $2, assignment_id = NULL, scheduled_slot = NULL, version = version + 1,
			updated_at = $3 WHERE id = $1`,
		target.job,
		status,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.jobs")
	}

	err = (Jobs{Units: nil, Severity: nil, HQOrg: nil, Sites: nil, Blobs: nil}).event(
		ctx,
		call,
		target.job,
		why.event,
		nil,
	)
	if err != nil {
		return err
	}

	input := identity.MembersInput{
		Role:           "admin",
		Permission:     "job.write",
		IDs:            nil,
		OrganizationID: nil,
		ClientRole:     "",
	}
	if target.contractor != nil {
		input = identity.MembersInput{
			Role:           "contractor",
			OrganizationID: target.contractor,
			Permission:     "partner.assign", IDs: nil, ClientRole: "",
		}
	}

	recipients, err := identity.Members(ctx, call, input)
	if err != nil {
		return apperr.Wrap(err, "identity.Members")
	}

	for _, r := range recipients {
		_, err := (notify.Store{}).Create(ctx, call, notify.New{
			RecipientMembershipID: r,
			Type:                  "job_update",
			TemplateKey:           "job_update",
			TargetKind:            "job",
			TargetID:              target.job,
			Severity:              "warning",
			Params: map[string]any{
				"status": status,
				"reason": why.notice,
			},
			Channel:       "",
			SourceAlertID: nil,
		})
		if err != nil {
			return apperr.Wrap(err, "notify.Store.Create")
		}
	}

	call.Audit(auditEntry("jobs.releaseAssignment", "job", target.job, nil, nil, why.audit))

	return nil
}
