package maintenance

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
)

// ReleaseEndedAccess is the maintenance scheduler job of OPEN-BE-09 (IR439). A technician whose access ends before an
// assigned visit ends could not open the job: HQ ended or shortened the membership (Access & roles), which the
// contractor cannot do while the technician is booked (IR421). The assignment of such a job that has not started is
// released at once, as completion releases assignments (DEC-73): the job goes back to the contractor (accepted) or,
// for HQ's own delivery, to requested, keeping its agreed visit, and the people who assign get a job_update notice so
// they choose another technician. A job already started keeps its assignment for HQ to handle.
func (m Jobs) ReleaseEndedAccess(ctx context.Context, c *ops.Call) (int, error) {
	rows, err := c.Tx.Query(ctx, `SELECT a.id, a.job_id, j.contractor_org_id FROM maintenance.assignments a
		JOIN maintenance.jobs j ON j.id = a.job_id AND j.assignment_id = a.id AND j.status = 'assigned'
		JOIN maintenance.ref_memberships rm ON rm.id = a.technician_membership_id
		WHERE a.status = 'active' AND rm.valid_until IS NOT NULL AND rm.valid_until < a.valid_until
		ORDER BY a.job_id FOR UPDATE OF a, j SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	type ended struct {
		assignment, job uuid.UUID
		contractor      *uuid.UUID
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ended, error) {
		var e ended
		return e, r.Scan(&e.assignment, &e.job, &e.contractor)
	})
	if err != nil {
		return 0, err
	}
	for _, e := range list {
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.assignments SET status = 'revoked', reason = 'access_ended', version = version + 1, updated_at = $2 WHERE id = $1`,
			e.assignment, c.Now); err != nil {
			return 0, err
		}
		status := "requested" // HQ's own delivery assigns from requested
		if e.contractor != nil {
			status = "accepted" // the contractor assigns again on its accepted offer
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = $2, assignment_id = NULL, scheduled_slot = NULL, version = version + 1, updated_at = $3 WHERE id = $1`,
			e.job, status, c.Now); err != nil {
			return 0, err
		}
		if err := m.event(ctx, c, e.job, "job.assignment_released", nil); err != nil {
			return 0, err
		}
		in := identity.MembersInput{Role: "admin", Permission: "job.write"}
		if e.contractor != nil {
			in = identity.MembersInput{Role: "contractor", OrganizationID: e.contractor, Permission: "partner.assign"}
		}
		recipients, err := identity.Members(ctx, c, in)
		if err != nil {
			return 0, err
		}
		for _, r := range recipients {
			if _, err := (notify.Store{}).Create(ctx, c, notify.New{RecipientMembershipID: r, Type: "job_update", TemplateKey: "job_update",
				TargetKind: "job", TargetID: e.job, Severity: "warning", Params: map[string]any{"status": status, "reason": "technician_access_ended"}}); err != nil {
				return 0, err
			}
		}
		c.Audit(ops.AuditEntry{Action: "jobs.releaseAssignment", TargetKind: "job", TargetID: e.job.String(), Reason: "technician access ended before the visit"})
	}
	return len(list), nil
}
