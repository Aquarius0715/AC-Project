package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/modules/notify"
	"github.com/pradita/ac-project/service/internal/ops"
)

// notifyingRecorder creates the IR95 business-event notifications from the committed transitions of a write before
// the audit / outbox rows are recorded (same transaction). Each recipient membership gets one notification; the
// acting membership and memberships that are not currently active never receive one (IR95, IR104, IR156).
type notifyingRecorder struct {
	inner ops.Recorder
	store notify.Store
}

func (r notifyingRecorder) Record(ctx context.Context, tx pgx.Tx, c *ops.Call, op string) error {
	if c.Principal != nil {
		if err := r.dispatch(ctx, c, op); err != nil {
			return err
		}
	}
	return r.inner.Record(ctx, tx, c, op)
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

const activeMember = `m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)`

func (r notifyingRecorder) query(ctx context.Context, c *ops.Call, sql string, args ...any) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, sql, append([]any{c.Now}, args...)...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func (r notifyingRecorder) clients(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return r.query(ctx, c, `SELECT m.id FROM identity.memberships m WHERE m.role = 'client' AND m.organization_id = $2 AND `+activeMember, org)
}

func (r notifyingRecorder) hq(ctx context.Context, c *ops.Call, perm string) ([]uuid.UUID, error) {
	return r.query(ctx, c, `SELECT m.id FROM identity.memberships m WHERE m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions p
		WHERE p.membership_id = m.id AND p.permission = $2) AND `+activeMember, perm)
}

func (r notifyingRecorder) partners(ctx context.Context, c *ops.Call, org *uuid.UUID, perm string) ([]uuid.UUID, error) {
	if org == nil {
		return nil, nil
	}
	return r.query(ctx, c, `SELECT m.id FROM identity.memberships m WHERE m.role = 'contractor' AND m.organization_id = $2 AND EXISTS (SELECT 1 FROM identity.membership_permissions p
		WHERE p.membership_id = m.id AND p.permission = $3) AND `+activeMember, *org, perm)
}

func (r notifyingRecorder) technician(ctx context.Context, c *ops.Call, job uuid.UUID) ([]uuid.UUID, error) {
	// the active assignment, or the one revoked by this very transition (cancel / hold)
	return r.query(ctx, c, `SELECT m.id FROM maintenance.assignments a JOIN identity.memberships m ON m.id = a.technician_membership_id
		WHERE a.job_id = $2 AND (a.status = 'active' OR (a.status = 'revoked' AND a.updated_at = $1)) AND `+activeMember, job)
}

func (r notifyingRecorder) send(ctx context.Context, c *ops.Call, set *recipientSet, template, kind string, target uuid.UUID, severity, status string) error {
	for _, id := range set.ids {
		if id == c.Principal.MembershipID {
			continue
		}
		if _, err := r.store.Create(ctx, c, notify.New{RecipientMembershipID: id, Type: template, TemplateKey: template, TargetKind: kind, TargetID: target,
			Severity: severity, Params: map[string]any{"status": status}}); err != nil {
			return err
		}
	}
	return nil
}

// jobEvent maps a job write and the job status after it to the IR95 event, template and audience.
func jobEvent(op, status string) (event, template string) {
	switch {
	case op == "jobs.create" || op == "jobs.reportProblem":
		return "requested", "job_update"
	case op == "jobs.offer":
		return "offered", "job_update"
	case op == "jobs.accept":
		return "accepted", "job_update"
	case op == "jobs.decline":
		return "declined", "job_update"
	case op == "jobs.assign":
		return "assigned", "schedule_change"
	case op == "jobs.submit":
		return "submitted", "job_update"
	case op == "jobs.review" && status == "rework_requested":
		return "returned", "report_return"
	case op == "jobs.review" && status == "completed":
		return "completed", "completion"
	case op == "jobs.cancel", op == "jobs.hold", op == "jobs.resumeHold":
		return status, "job_update"
	}
	return "", ""
}

func (r notifyingRecorder) dispatch(ctx context.Context, c *ops.Call, op string) error {
	for _, a := range c.Audits {
		id, err := uuid.Parse(a.TargetID)
		if err != nil {
			continue
		}
		switch a.TargetKind {
		case "job":
			if err := r.job(ctx, c, op, id); err != nil {
				return err
			}
		case "restriction":
			if op == "restrictions.schedule" { // the advance notice is IR05's own notification
				continue
			}
			if err := r.restriction(ctx, c, id); err != nil {
				return err
			}
		}
	}
	for _, e := range c.Events { // payment.confirmed
		pl, _ := e.Payload.(map[string]any)
		if e.Type != "PaymentChanged" || fmt.Sprint(pl["status"]) != "confirmed" {
			continue
		}
		var org, invoice uuid.UUID
		if err := c.Tx.QueryRow(ctx, `SELECT k.customer_org_id, i.id FROM billing.payments p JOIN billing.invoices i ON i.id = p.invoice_id
			JOIN billing.contracts k ON k.id = i.contract_id AND k.version = i.contract_version WHERE p.id = $1`, e.AggregateID).Scan(&org, &invoice); err != nil {
			return err
		}
		set := &recipientSet{}
		cl, err := r.clients(ctx, c, org)
		if err != nil {
			return err
		}
		hq, err := r.hq(ctx, c, "billing.write")
		if err != nil {
			return err
		}
		set.add(cl...)
		set.add(hq...)
		if err := r.send(ctx, c, set, "payment", "invoice", invoice, "normal", "confirmed"); err != nil {
			return err
		}
	}
	return nil
}

func (r notifyingRecorder) job(ctx context.Context, c *ops.Call, op string, job uuid.UUID) error {
	var status string
	var org uuid.UUID
	var contractor *uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT status, customer_org_id, contractor_org_id FROM maintenance.jobs WHERE id = $1`, job).Scan(&status, &org, &contractor)
	if err != nil {
		return nil // not a job row (the audit target was resolved elsewhere)
	}
	event, template := jobEvent(op, status)
	if event == "" {
		return nil
	}
	if event == "offered" { // the contractor of the latest open offer
		var offered uuid.UUID
		if err := c.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.offers WHERE job_id = $1 ORDER BY offered_at DESC, id DESC LIMIT 1`, job).Scan(&offered); err == nil {
			contractor = &offered
		}
	}
	set := &recipientSet{}
	add := func(ids []uuid.UUID, err error) error {
		set.add(ids...)
		return err
	}
	clients := func() error { return add(r.clients(ctx, c, org)) }
	hqJob := func() error { return add(r.hq(ctx, c, "job.write")) }
	tech := func() error { return add(r.technician(ctx, c, job)) }
	partner := func(perm string) error { return add(r.partners(ctx, c, contractor, perm)) }
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
		steps = []func() error{tech, clients, hqJob, func() error { return partner("partner.assign") }}
	case "submitted":
		if contractor != nil {
			steps = []func() error{func() error { return partner("partner.review") }, clients}
		} else {
			steps = []func() error{hqJob, clients}
		}
	case "returned":
		steps = []func() error{tech, hqJob}
	case "completed":
		steps = []func() error{clients, hqJob, tech, func() error { return partner("partner.review") }}
	default: // cancelled / on_hold / resumed
		steps = []func() error{clients, tech, func() error { return partner("partner.assign") }, hqJob}
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	return r.send(ctx, c, set, template, "job", job, "normal", event)
}

func (r notifyingRecorder) restriction(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	var state string
	var org uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT r.state, cu.organization_id FROM restrictions.restrictions r JOIN assets.customers cu ON cu.id = r.customer_id WHERE r.id = $1`, id).
		Scan(&state, &org); err != nil {
		return err
	}
	set := &recipientSet{}
	cl, err := r.clients(ctx, c, org)
	if err != nil {
		return err
	}
	hq, err := r.hq(ctx, c, "restriction.write")
	if err != nil {
		return err
	}
	set.add(cl...)
	set.add(hq...)
	severity := "warning"
	if state == "released" || state == "cancelled" {
		severity = "normal"
	}
	return r.send(ctx, c, set, "restriction", "restriction", id, severity, state)
}
