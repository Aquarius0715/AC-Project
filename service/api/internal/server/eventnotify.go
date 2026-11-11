package server

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
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

func (notifyingRecorder) clients(
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

func (notifyingRecorder) hq(ctx context.Context, c *ops.Call, perm string) ([]uuid.UUID, error) {
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

func (notifyingRecorder) partners(
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

func (notifyingRecorder) technician(
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

// jobEvent maps a job write and the job status after it to the IR95 event and its template; a write without an
// event gives "".
func jobEvent(operation, status string) (string, string) {
	switch operation {
	case "jobs.create", "jobs.reportProblem", "plans.generateNext":
		return "requested", noticeJobUpdate
	case "jobs.offer":
		return eventOffered, noticeJobUpdate
	case "jobs.accept":
		return eventAccepted, noticeJobUpdate
	case "jobs.decline":
		return eventDeclined, noticeJobUpdate
	case "jobs.assign":
		return eventAssigned, noticeScheduleChange
	case "jobs.submit":
		return "submitted", noticeJobUpdate
	case "jobs.review": // a returned report, or a completed job
		switch status {
		case "rework_requested":
			return "returned", "report_return"
		case statusCompleted:
			return statusCompleted, "completion"
		}
	case "jobs.cancel", "jobs.hold", "jobs.resumeHold":
		return status, noticeJobUpdate
	}

	return "", ""
}

func (r notifyingRecorder) dispatch(ctx context.Context, call *ops.Call, operation string) error {
	for _, auditEntry := range call.Audits {
		err := r.audited(ctx, call, operation, auditEntry)
		if err != nil {
			return err
		}
	}

	for _, event := range call.Events { // payment.confirmed
		pl, _ := event.Payload.(map[string]any)
		if event.Type != "PaymentChanged" || fmt.Sprint(pl["status"]) != "confirmed" {
			continue
		}

		err := r.paymentConfirmed(ctx, call, event.AggregateID)
		if err != nil {
			return err
		}
	}

	return nil
}

// audited notifies the audience of one audited write: a job event, or a restriction's change.
func (r notifyingRecorder) audited(
	ctx context.Context,
	call *ops.Call,
	operation string,
	entry ops.AuditEntry,
) error {
	target, err := uuid.Parse(entry.TargetID)
	if err != nil {
		return nil //nolint:nilerr // an audit target that is not a UUID names no row to notify about
	}

	switch entry.TargetKind {
	case "job":
		if operation == "jobs.reportProblem" {
			// the audit names the reported job; the request is the follow-up it created (IR438)
			err := call.Tx.QueryRow(ctx, `SELECT id FROM maintenance.jobs WHERE follow_up_of_job_id = $1 ORDER BY created_at
				DESC,
				id DESC LIMIT 1`, target).
				Scan(&target)
			if err != nil {
				return apperr.Wrap(err, "select maintenance.jobs")
			}
		}

		return r.job(ctx, call, operation, target)
	case "plan":
		if operation != "plans.generateNext" {
			return nil
		}
		// the audit names the plan; the request is the job it generated, which its PlanOccurrenceGenerated event names
		// (IR95 job.requested, IR752)
		job, ok := generatedJob(call, target)
		if !ok {
			return nil
		}

		return r.job(ctx, call, operation, job)
	case "restriction":
		if operation == "restrictions.schedule" { // the advance notice is IR05's own notification
			return nil
		}

		return r.restriction(ctx, call, target)
	}

	return nil
}

// generatedJob is the job a plans.generateNext call generated for the plan, read from its PlanOccurrenceGenerated
// event: the plan's newest job would be ambiguous when two jobs share a creation time (a frozen demo clock).
func generatedJob(call *ops.Call, plan uuid.UUID) (uuid.UUID, bool) {
	for _, event := range call.Events {
		if event.Type != "PlanOccurrenceGenerated" || event.AggregateID != plan {
			continue
		}

		payload, _ := event.Payload.(map[string]any)
		if id, ok := payload["jobId"].(uuid.UUID); ok {
			return id, true
		}
	}

	return uuid.Nil, false
}

// paymentConfirmed tells the customer's users and HQ that a payment was confirmed.
func (r notifyingRecorder) paymentConfirmed(
	ctx context.Context,
	call *ops.Call,
	payment uuid.UUID,
) error {
	var org, invoice uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT k.customer_org_id, i.id FROM billing.payments p JOIN billing.invoices i ON i.id =
		p.invoice_id
		JOIN billing.contracts k ON k.id = i.contract_id AND k.version = i.contract_version WHERE p.id =
		$1`, payment).
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

	return r.send(
		ctx,
		call,
		set,
		"payment",
		"invoice",
		invoice,
		"normal",
		"confirmed",
	)
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
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // not a job row (the audit target was resolved elsewhere)
	}

	if err != nil {
		return apperr.Wrap(err, "select maintenance.jobs")
	}

	event, template := jobEvent(operation, status)
	if event == "" {
		var proposalErr error

		event, proposalErr = proposalEvent(ctx, call, operation, job)
		if proposalErr != nil {
			return proposalErr
		}

		template = noticeScheduleChange
	}

	if event == "" {
		return nil
	}

	if event == eventOffered { // the contractor of the latest open offer
		var offered uuid.UUID

		err := call.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.offers WHERE job_id = $1 ORDER BY offered_at
			DESC, id DESC LIMIT 1`, job).
			Scan(&offered)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return apperr.Wrap(err, "select maintenance.offers")
		}

		if err == nil {
			contractor = &offered
		}
	}

	set := &recipientSet{ids: nil}

	for _, audience := range jobAudiences(event, contractor != nil, status) {
		ids, err := r.jobRecipients(
			ctx,
			call,
			audience,
			jobParties{job: job, org: org, contractor: contractor},
		)
		if err != nil {
			return err
		}

		set.add(ids...)
	}

	return r.send(ctx, call, set, template, "job", job, "normal", noticeStatus(event))
}

// The events of IR113 item 9 beyond IR95's own table (IR783): the time proposals and the technician's answer.
const (
	eventTimeProposed       = "time_proposed"
	eventProposalAccepted   = "proposal_accepted"
	eventProposalDeclined   = "proposal_declined"
	eventProposalExpired    = "proposal_expired"
	eventRescheduleAsked    = "reschedule_requested"
	eventTimeChangeProposed = "time_change_proposed"
	eventTimeChangeKept     = "time_change_kept"
	eventCantMake           = "cant_make"
	eventVisitAccepted      = "visit_accepted"
)

// proposalEvent maps the time-proposal and acknowledgement writes of IR113 item 9 to their event; a write that is
// none of them gives "". The outcome of a write that has two (an answer to a proposal, the technician's answer, HQ's
// decision on a contractor's time) is read from what the write emitted or stored.
func proposalEvent(
	ctx context.Context,
	call *ops.Call,
	operation string,
	job uuid.UUID,
) (string, error) {
	switch operation {
	case "jobs.proposeSlot":
		return eventTimeProposed, nil
	case "jobs.respondProposal":
		if hasEvent(call, "ProposalAccepted") {
			return eventProposalAccepted, nil
		}

		return eventProposalDeclined, nil
	case maintenance.OperationProposalExpiry:
		return eventProposalExpired, nil
	case "jobs.requestReschedule":
		return eventRescheduleAsked, nil
	case "jobs.proposePartnerSlot":
		return eventTimeChangeProposed, nil
	case "jobs.resolvePartnerSlot":
		var status string

		err := call.Tx.QueryRow(
			ctx,
			`SELECT p.status FROM maintenance.partner_slot_proposals p JOIN maintenance.offers o ON o.id = p.offer_id
			WHERE o.job_id = $1 AND p.status IN ('kept', 'sent_to_client') ORDER BY p.sent_at DESC, p.id DESC LIMIT 1`,
			job,
		).Scan(&status)
		if err != nil {
			return "", apperr.Wrap(err, "select maintenance.partner_slot_proposals")
		}

		if status == "sent_to_client" {
			return eventTimeProposed, nil
		}

		return eventTimeChangeKept, nil
	case "jobs.acknowledgeAssignment":
		if hasEvent(call, "AssignmentAcknowledged") && acknowledged(call) == "cant_make" {
			return eventCantMake, nil
		}

		return eventVisitAccepted, nil
	}

	return "", nil
}

// hasEvent reports whether the write emitted an event of the type.
func hasEvent(call *ops.Call, eventType string) bool {
	return slices.ContainsFunc(
		call.Events,
		func(event ops.Event) bool { return event.Type == eventType },
	)
}

// acknowledged is the technician's answer a jobs.acknowledgeAssignment write emitted.
func acknowledged(call *ops.Call) string {
	for _, event := range call.Events {
		if event.Type != "AssignmentAcknowledged" {
			continue
		}

		payload, _ := event.Payload.(map[string]any)

		return fmt.Sprint(payload["acknowledgement"])
	}

	return ""
}

// noticeStatus is the status word a notice shows for an event: IR95's own events are their own words, the events of
// IR113 item 9 use the words of the job statuses where there is one.
func noticeStatus(event string) string {
	switch event {
	case eventProposalAccepted, eventVisitAccepted:
		return eventAccepted
	case eventProposalDeclined:
		return eventDeclined
	case eventProposalExpired:
		return "expired"
	}

	return event
}

// jobAudiences are the audiences of a job event in the order IR95 lists them: the customer's users, HQ, the assigned
// technician, or the contractor's users who hold a partner permission (the audience is that permission).
func jobAudiences(event string, contracted bool, status string) []string {
	switch event {
	case eventTimeProposed: // IR113 item 9: a proposal sent to the client
		return []string{audienceClients}
	case eventProposalAccepted: // HQ, and who the accepted time put to work: the offered contractor, the technician
		audiences := []string{audienceHQ}
		if contracted && status == eventOffered {
			audiences = append(audiences, permPartnerAccept)
		}

		if status == eventAssigned {
			audiences = append(audiences, audienceTechnician)
		}

		return audiences
	case eventTimeChangeKept: // HQ kept the agreed time: the contractor that asked for another is told
		return []string{permPartnerAssign}
	case eventCantMake: // the coordinator: HQ for an internal job, the receiving contractor otherwise
		if contracted {
			return []string{permPartnerAssign}
		}

		return []string{audienceHQ}
	case eventVisitAccepted: // the coordinator and the client
		if contracted {
			return []string{permPartnerAssign, audienceClients}
		}

		return []string{audienceHQ, audienceClients}
	case "requested":
		return []string{audienceClients, audienceHQ}
	case eventOffered:
		return []string{permPartnerAccept, audienceClients}
	case eventAccepted:
		return []string{audienceHQ, audienceClients}
	case eventDeclined,
		eventProposalDeclined,
		eventProposalExpired,
		eventRescheduleAsked,
		eventTimeChangeProposed:
		return []string{audienceHQ} // IR113 item 9: the answers and requests HQ has to act on
	case eventAssigned:
		return []string{audienceTechnician, audienceClients, audienceHQ, permPartnerAssign}
	case "submitted":
		if contracted {
			return []string{permPartnerReview, audienceClients}
		}

		return []string{audienceHQ, audienceClients}
	case "returned":
		return []string{audienceTechnician, audienceHQ}
	case statusCompleted:
		return []string{audienceClients, audienceHQ, audienceTechnician, permPartnerReview}
	}

	return []string{
		audienceClients,
		audienceTechnician,
		permPartnerAssign,
		audienceHQ,
	} // cancelled / on_hold / resumed
}

// jobParties are the job of an event, its customer and its contractor (nil when HQ handles it).
type jobParties struct {
	job, org   uuid.UUID
	contractor *uuid.UUID
}

// jobRecipients are the users of one audience of a job event.
func (r notifyingRecorder) jobRecipients(
	ctx context.Context,
	call *ops.Call,
	audience string,
	parties jobParties,
) ([]uuid.UUID, error) {
	switch audience {
	case audienceClients:
		return r.clients(ctx, call, parties.org)
	case audienceHQ:
		return r.hq(ctx, call, "job.write")
	case audienceTechnician:
		return r.technician(ctx, call, parties.job)
	}

	return r.partners(ctx, call, parties.contractor, audience)
}

func (r notifyingRecorder) restriction(
	ctx context.Context,
	call *ops.Call,
	restrictionID uuid.UUID,
) error {
	var (
		state string
		org   uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT r.state, k.customer_org_id FROM restrictions.restrictions r
		JOIN billing.contracts k ON k.id = r.contract_id AND k.version = r.contract_version WHERE r.id = $1`, restrictionID).
		Scan(&state, &org)
	if err != nil {
		return apperr.Wrap(err, "select restrictions.restrictions")
	}

	set := &recipientSet{ids: nil}

	clientIDs, err := r.clients(ctx, call, org)
	if err != nil {
		return err
	}

	hqIDs, err := r.hq(ctx, call, "restriction.write")
	if err != nil {
		return err
	}

	set.add(clientIDs...)
	set.add(hqIDs...)

	severity := "warning"
	if state == "released" || state == "cancelled" {
		severity = "normal"
	}

	return r.send(ctx, call, set, "restriction", "restriction", restrictionID, severity, state)
}
