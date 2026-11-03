package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// notifyingRecorder creates the IR95 business-event notifications from the committed transitions of a write before
// the audit / outbox rows are recorded (same transaction). Each recipient membership gets one notification; the
// acting membership and memberships that are not currently active never receive one (IR95, IR104, IR156).
type notifyingRecorder struct {
	inner ops.Recorder
	store notify.Store
}

func (r notifyingRecorder) Record(
	ctx context.Context,
	txn pgx.Tx,
	call *ops.Call,
	operation string,
) error {
	if call.Principal != nil {
		err := r.dispatch(ctx, call, operation)
		if err != nil {
			return err
		}
	}

	return apperr.Wrap(r.inner.Record(ctx, txn, call, operation), "ops.Recorder.Record")
}

// RecordDenied passes a refused write's audit entry to the inner recorder (ops.DeniedRecorder, IR603); a refusal
// notifies nobody.
func (r notifyingRecorder) RecordDenied(
	ctx context.Context,
	txn pgx.Tx,
	c *ops.Call,
	op string,
	d ops.Denial,
) error {
	if dr, ok := r.inner.(ops.DeniedRecorder); ok {
		return apperr.Wrap(dr.RecordDenied(ctx, txn, c, op, d), "ops.DeniedRecorder.RecordDenied")
	}

	return nil
}

type recipientSet struct {
	ids []uuid.UUID
}

func (s *recipientSet) add(ids ...uuid.UUID) {
	for _, id := range ids {
		if id != uuid.Nil && !slices.Contains(s.ids, id) {
			s.ids = append(s.ids, id)
		}
	}
}

// members asks identity for the active memberships matching f (IR191).
func members(ctx context.Context, c *ops.Call, f identity.MembersInput) ([]uuid.UUID, error) {
	return ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, f)
}

func (r notifyingRecorder) clients(
	ctx context.Context,
	c *ops.Call,
	org uuid.UUID,
) ([]uuid.UUID, error) {
	return members(
		ctx,
		c,
		identity.MembersInput{
			Role:           "client",
			OrganizationID: &org,
			IDs:            nil,
			Permission:     "",
			ClientRole:     "",
		},
	)
}

func (r notifyingRecorder) hq(ctx context.Context, c *ops.Call, perm string) ([]uuid.UUID, error) {
	return members(
		ctx,
		c,
		identity.MembersInput{
			Role:           "admin",
			Permission:     perm,
			IDs:            nil,
			OrganizationID: nil,
			ClientRole:     "",
		},
	)
}

func (r notifyingRecorder) partners(
	ctx context.Context,
	call *ops.Call,
	org *uuid.UUID,
	perm string,
) ([]uuid.UUID, error) {
	if org == nil {
		return nil, nil
	}

	return members(
		ctx,
		call,
		identity.MembersInput{
			Role:           "contractor",
			OrganizationID: org,
			Permission:     perm,
			IDs:            nil,
			ClientRole:     "",
		},
	)
}

func (r notifyingRecorder) technician(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
) ([]uuid.UUID, error) {
	// the active assignment, or the one this very transition revoked (cancel / hold) or released (completion, IR234);
	// maintenance's own rows
	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.technician_membership_id FROM maintenance.assignments a
		WHERE a.job_id = $2 AND (a.status = 'active' OR (a.status IN ('revoked', 'completed') AND a.updated_at =
		$1))`,
		call.Now,
		job,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.assignments")
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(ids) == 0 {
		return nil, apperr.Wrap(err, "select maintenance.assignments")
	}

	return members(
		ctx,
		call,
		identity.MembersInput{
			IDs:            ids,
			Role:           "",
			OrganizationID: nil,
			Permission:     "",
			ClientRole:     "",
		},
	)
}

func (r notifyingRecorder) send(
	ctx context.Context,
	call *ops.Call,
	set *recipientSet,
	template, kind string,
	target uuid.UUID,
	severity, status string,
) error {
	for _, recipient := range set.ids {
		if recipient == call.Principal.MembershipID {
			continue
		}

		_, err := r.store.Create(ctx, call, notify.New{
			RecipientMembershipID: recipient,
			Type:                  template,
			TemplateKey:           template,
			TargetKind:            kind,
			TargetID:              target,
			Severity:              severity,
			Params: map[string]any{
				"status": status,
			},
			Channel:       "",
			SourceAlertID: nil,
		})
		if err != nil {
			return apperr.Wrap(err, "notify.Store.Create")
		}
	}

	return nil
}

// jobEvent maps a job write and the job status after it to the IR95 event, template and audience.
func jobEvent(operation, status string) (event, template string) {
	switch {
	case operation == "jobs.create" || operation == "jobs.reportProblem":
		return "requested", "job_update"
	case operation == "jobs.offer":
		return "offered", "job_update"
	case operation == "jobs.accept":
		return "accepted", "job_update"
	case operation == "jobs.decline":
		return "declined", "job_update"
	case operation == "jobs.assign":
		return "assigned", "schedule_change"
	case operation == "jobs.submit":
		return "submitted", "job_update"
	case operation == "jobs.review" && status == "rework_requested":
		return "returned", "report_return"
	case operation == "jobs.review" && status == "completed":
		return "completed", "completion"
	case operation == "jobs.cancel", operation == "jobs.hold", operation == "jobs.resumeHold":
		return status, "job_update"
	}

	return "", ""
}

func (r notifyingRecorder) dispatch(ctx context.Context, call *ops.Call, operation string) error {
	for _, auditEntry := range call.Audits {
		target, err := uuid.Parse(auditEntry.TargetID)
		if err != nil {
			continue
		}

		switch auditEntry.TargetKind {
		case "job":
			if operation == "jobs.reportProblem" { // the audit names the reported job; the request is the follow-up it created (IR438)
				err := call.Tx.QueryRow(ctx, `SELECT id FROM maintenance.jobs WHERE follow_up_of_job_id = $1 ORDER BY created_at
					DESC,
					id DESC LIMIT 1`, target).
					Scan(&target)
				if err != nil {
					return apperr.Wrap(err, "select maintenance.jobs")
				}
			}

			err := r.job(ctx, call, operation, target)
			if err != nil {
				return err
			}
		case "restriction":
			if operation == "restrictions.schedule" { // the advance notice is IR05's own notification
				continue
			}

			err := r.restriction(ctx, call, target)
			if err != nil {
				return err
			}
		}
	}

	for _, event := range call.Events { // payment.confirmed
		pl, _ := event.Payload.(map[string]any)
		if event.Type != "PaymentChanged" || fmt.Sprint(pl["status"]) != "confirmed" {
			continue
		}

		var org, invoice uuid.UUID

		err := call.Tx.QueryRow(ctx, `SELECT k.customer_org_id, i.id FROM billing.payments p JOIN billing.invoices i ON i.id =
			p.invoice_id
			JOIN billing.contracts k ON k.id = i.contract_id AND k.version = i.contract_version WHERE p.id =
			$1`, event.AggregateID).
			Scan(&org, &invoice)
		if err != nil {
			return apperr.Wrap(err, "select billing.payments")
		}

		set := &recipientSet{ids: nil}

		clientIDs, err := r.clients(ctx, call, org)
		if err != nil {
			return err
		}

		hqIDs, err := r.hq(ctx, call, "billing.write")
		if err != nil {
			return err
		}

		set.add(clientIDs...)
		set.add(hqIDs...)

		sendErr := r.send(
			ctx,
			call,
			set,
			"payment",
			"invoice",
			invoice,
			"normal",
			"confirmed",
		)
		if sendErr != nil {
			return sendErr
		}
	}

	return nil
}

func (r notifyingRecorder) job(
	ctx context.Context,
	call *ops.Call,
	operation string,
	job uuid.UUID,
) error {
	var (
		status     string
		org        uuid.UUID
		contractor *uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, customer_org_id, contractor_org_id FROM maintenance.jobs WHERE id =
		$1`, job).
		Scan(&status, &org, &contractor)
	if err != nil {
		return nil // not a job row (the audit target was resolved elsewhere)
	}

	event, template := jobEvent(operation, status)
	if event == "" {
		return nil
	}

	if event == "offered" { // the contractor of the latest open offer
		var offered uuid.UUID

		err := call.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.offers WHERE job_id = $1 ORDER BY offered_at
			DESC, id DESC LIMIT 1`, job).
			Scan(&offered)
		if err == nil {
			contractor = &offered
		}
	}

	set := &recipientSet{ids: nil}
	add := func(ids []uuid.UUID, err error) error {
		set.add(ids...)

		return err
	}
	clients := func() error { return add(r.clients(ctx, call, org)) }
	hqJob := func() error { return add(r.hq(ctx, call, "job.write")) }
	tech := func() error { return add(r.technician(ctx, call, job)) }
	partner := func(perm string) error { return add(r.partners(ctx, call, contractor, perm)) }

	var steps []func() error

	switch event {
	case "requested":
		steps = []func() error{clients, hqJob}
	case "offered":
		steps = []func() error{func() error { return partner("partner.accept") }, clients}
	case "accepted":
		steps = []func() error{hqJob, clients}
	case "declined":
		steps = []func() error{hqJob}
	case "assigned":
		steps = []func() error{
			tech,
			clients,
			hqJob,
			func() error { return partner("partner.assign") },
		}
	case "submitted":
		if contractor != nil {
			steps = []func() error{func() error { return partner("partner.review") }, clients}
		} else {
			steps = []func() error{hqJob, clients}
		}
	case "returned":
		steps = []func() error{tech, hqJob}
	case "completed":
		steps = []func() error{
			clients,
			hqJob,
			tech,
			func() error { return partner("partner.review") },
		}
	default: // cancelled / on_hold / resumed
		steps = []func() error{
			clients,
			tech,
			func() error { return partner("partner.assign") },
			hqJob,
		}
	}

	for _, s := range steps {
		err := s()
		if err != nil {
			return err
		}
	}

	return r.send(ctx, call, set, template, "job", job, "normal", event)
}

func (r notifyingRecorder) restriction(
	ctx context.Context,
	c *ops.Call,
	restrictionID uuid.UUID,
) error {
	var (
		state string
		org   uuid.UUID
	)

	err := c.Tx.QueryRow(ctx, `SELECT r.state, k.customer_org_id FROM restrictions.restrictions r
		JOIN billing.contracts k ON k.id = r.contract_id AND k.version = r.contract_version WHERE r.id = $1`, restrictionID).
		Scan(&state, &org)
	if err != nil {
		return apperr.Wrap(err, "select restrictions.restrictions")
	}

	set := &recipientSet{ids: nil}

	clientIDs, err := r.clients(ctx, c, org)
	if err != nil {
		return err
	}

	hqIDs, err := r.hq(ctx, c, "restriction.write")
	if err != nil {
		return err
	}

	set.add(clientIDs...)
	set.add(hqIDs...)

	severity := "warning"
	if state == "released" || state == "cancelled" {
		severity = "normal"
	}

	return r.send(ctx, c, set, "restriction", "restriction", restrictionID, severity, state)
}
