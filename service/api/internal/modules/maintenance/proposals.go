package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// DefaultTermsVersion is used for Offers created from an accepted proposal when the contractor has no earlier terms.
const DefaultTermsVersion = "terms-demo-v1"

// Hold is SlotHold of service-contracts.ts.
type Hold struct {
	Kind                   string     `json:"kind"`
	MembershipID           *uuid.UUID `json:"membershipId,omitempty"`
	ContractorOrgID        *uuid.UUID `json:"contractorOrgId,omitempty"`
	TechnicianMembershipID *uuid.UUID `json:"technicianMembershipId"`
}

// The kinds of a slot hold (SlotHold.kind): an HQ technician, or a contractor with its technician when known.
const (
	holdInternal   = "internal"
	holdContractor = "contractor"
)

// proposalPending is the status of a slot proposal, and of a contractor's time change, that waits for an answer.
const proposalPending = "pending"

// Proposals is the IR128 operation set.
type Proposals struct {
	Delivery Delivery
}

func (m Proposals) jobs() Jobs { return m.Delivery.Jobs }

// replyByOK: after now, at most 7 days ahead, and not after the proposed slot starts (IR455), so the client cannot
// accept a time that has begun.
func replyByOK(now, replyBy, slotStart time.Time) bool {
	return replyBy.After(now) && !replyBy.After(now.Add(7*24*time.Hour)) &&
		!replyBy.After(slotStart)
}

func (Proposals) pending(ctx context.Context, c *ops.Call, job uuid.UUID) (bool, error) {
	var found bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.slot_proposals WHERE job_id = $1 AND status =
		'pending')`, job).
		Scan(&found)

	return found, apperr.Wrap(err, "select maintenance.slot_proposals")
}

func (m Proposals) finish(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	action, operation, event, reason string,
) (Job, error) {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		job,
	)
	if err != nil {
		return Job{}, apperr.Wrap(err, "update maintenance.jobs")
	}

	err = m.jobs().event(ctx, call, job, action, nil)
	if err != nil {
		return Job{}, err
	}

	return m.jobs().finishScoped(ctx, call, job, operation, event, reason)
}

// checkHold validates a SlotHold for a slot (IR128 item 1).
func (m Proposals) checkHold(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	hold Hold,
	slot Slot,
) error {
	delivery := m.Delivery

	switch hold.Kind {
	case holdInternal:
		if hold.MembershipID == nil || hold.ContractorOrgID != nil {
			return apperr.Fields(map[string]string{fieldHold: apperr.KeyInvalid})
		}

		err := delivery.checkTechnician(ctx, call, unit, *hold.MembershipID, false, nil)
		if err != nil {
			return err
		}

		return delivery.checkQualified(ctx, call, unit, *hold.MembershipID, slot)
	case holdContractor:
		if hold.ContractorOrgID == nil || hold.MembershipID != nil {
			return apperr.Fields(map[string]string{fieldHold: apperr.KeyInvalid})
		}

		org, err := delivery.Directory.OrgState(ctx, call, *hold.ContractorOrgID)
		if err != nil {
			return apperr.Wrap(err, "maintenance.Directory.OrgState")
		}

		if !org.Found || org.Kind != orgKindContractor || org.Status != orgStatusActive {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		var suspended bool

		scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1 AND
			status = 'suspended')`, *hold.ContractorOrgID).
			Scan(&suspended)
		if scanErr != nil {
			return apperr.Wrap(scanErr, "select maintenance.contractors")
		}

		if suspended {
			return apperr.E(apperr.Conflict, "errors.contractor_suspended")
		}

		if hold.TechnicianMembershipID != nil {
			err := delivery.checkTechnician(
				ctx,
				call,
				unit,
				*hold.TechnicianMembershipID,
				true,
				hold.ContractorOrgID,
			)
			if err != nil {
				return err
			}

			return delivery.checkQualified(ctx, call, unit, *hold.TechnicianMembershipID, slot)
		}

		return nil
	}

	return apperr.Fields(map[string]string{fieldHold: apperr.KeyInvalid})
}

// ---- jobs.proposeSlot ----

// ProposeInput is jobs.proposeSlot input.
type ProposeInput struct {
	JobID   uuid.UUID `json:"jobId"`
	Slot    Slot      `json:"slot"`
	Hold    Hold      `json:"hold"`
	Message string    `json:"message"`
	ReplyBy time.Time `json:"replyBy"`
}

// Validate implements ops.Validator.
func (in *ProposeInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	trimmedReason(&in.Message, maxReasonLength, "message", fieldErrs)

	if in.ReplyBy.IsZero() {
		fieldErrs["replyBy"] = apperr.KeyRequired
	}

	return fieldErrs
}

// proposeSlot serves jobs.proposeSlot.
//
//	@Summary		jobs.proposeSlot (write)
//	@ID				jobs.proposeSlot
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 status requested (or offered/accepted/assigned after cant_make/decline); slot not one of \
//	@Description	preferredSlots (errors.slot_is_preferred); no pending proposal; message 1–1000; replyBy future ≤7 \
//	@Description	days and not after the slot start (IR455)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			request				body		ProposeInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/slot-proposals [post]
func (m Proposals) proposeSlot(
	ctx context.Context,
	c *ops.Call,
	in *ProposeInput,
) (MaintenanceJob, error) {
	return written(m.proposeSlotJob(ctx, c, in))
}

// proposeSlotJob is proposeSlot answering the job with its detail extras.
func (m Proposals) proposeSlotJob(
	ctx context.Context,
	call *ops.Call,
	input *ProposeInput,
) (Job, error) {
	job, err := m.Delivery.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	allowed := job.status == statusRequested
	switch job.status {
	case statusOffered, statusAccepted:
		var count int

		err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.partner_slot_proposals p JOIN maintenance.offers o ON
			o.id
			= p.offer_id
			WHERE o.job_id = $1 AND p.status <> 'withdrawn'`, input.JobID).
			Scan(&count)
		if err != nil {
			return Job{}, apperr.Wrap(err, "select maintenance.partner_slot_proposals")
		}

		allowed = count > 0

		if !allowed && job.status == statusAccepted {
			over, overErr := candidatesOver(ctx, call, input.JobID)
			if overErr != nil {
				return Job{}, overErr
			}

			allowed = over // the contractor can no longer assign any time it was offered (IR642)
		}
	case statusAssigned:
		var ack string // no active assignment: not allowed

		err := call.Tx.QueryRow(ctx, `SELECT acknowledgement FROM maintenance.assignments WHERE job_id = $1 AND status =
			'active'`, input.JobID).
			Scan(&ack)
		if err != nil &&
			!errors.Is(err, pgx.ErrNoRows) {
			return Job{}, apperr.Wrap(err, "select maintenance.assignments")
		}

		// the technician cannot make it, or the booked time is over and nobody started (IR642)
		allowed = ack == ackCantMake ||
			(job.scheduled != nil && !job.scheduled.EndAt.After(call.Now))
	}

	if !allowed {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	p, pendingErr := m.pending(ctx, call, input.JobID)
	if pendingErr != nil || p {
		if pendingErr != nil {
			return Job{}, pendingErr
		}

		return Job{}, apperr.E(apperr.Conflict, "errors.proposal_pending")
	}

	if !PreferredSlotsOK(call.Now, []Slot{input.Slot}) {
		return Job{}, apperr.Fields(map[string]string{fieldSlot: apperr.KeySlotRules})
	}

	for _, p := range job.preferred {
		if sameSlot(p, input.Slot) {
			return Job{}, apperr.Fields(map[string]string{fieldSlot: "errors.slot_is_preferred"})
		}
	}

	if !replyByOK(call.Now, input.ReplyBy, input.Slot.StartAt) {
		return Job{}, apperr.Fields(map[string]string{"replyBy": apperr.KeyRange})
	}

	checkHoldErr := m.checkHold(ctx, call, job.unit, input.Hold, input.Slot)
	if checkHoldErr != nil {
		return Job{}, checkHoldErr
	}

	insertProposalErr := m.insertProposal(
		ctx,
		call,
		input.JobID,
		"hq",
		input.Slot,
		input.Hold,
		input.Message,
		input.ReplyBy,
	)
	if insertProposalErr != nil {
		return Job{}, insertProposalErr
	}

	return m.finish(ctx, call, input.JobID, "proposal.sent", "jobs.proposeSlot", "SlotProposed", "")
}

func (Proposals) insertProposal(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	source string,
	slot Slot,
	hold Hold,
	msg string,
	replyBy time.Time,
) error {
	holdJSON, marshalErr := json.Marshal(hold)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.slot_proposals (tenant_id, job_id, source, slot, hold, message, reply_by, status, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, tstzrange($3, $4), $5, $6, $7, 'pending', $8)`,
		job,
		source,
		slot.StartAt,
		slot.EndAt,
		holdJSON,
		msg,
		replyBy,
		call.Now,
	)

	return apperr.Wrap(err, "insert maintenance.slot_proposals")
}

// ---- jobs.withdrawProposal / jobs.withdrawPartnerSlot ----

// ProposalRef is the {jobId, proposalId} input.
type ProposalRef struct {
	JobID      uuid.UUID `json:"jobId"`
	ProposalID uuid.UUID `json:"proposalId"`
}

// Validate implements ops.Validator.
func (in *ProposalRef) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ProposalID == uuid.Nil {
		fieldErrs["proposalId"] = apperr.KeyRequired
	}

	return fieldErrs
}

// withdrawProposal serves jobs.withdrawProposal.
//
//	@Summary		jobs.withdrawProposal (write)
//	@ID				jobs.withdrawProposal
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 pending own-tenant proposal only; releases hold
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			proposalId			path		string		true	"input field proposalId"
//	@Param			request				body		ProposalRef	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/slot-proposals/{proposalId}/withdraw [post]
func (m Proposals) withdrawProposal(
	ctx context.Context,
	c *ops.Call,
	in *ProposalRef,
) (MaintenanceJob, error) {
	return written(m.withdrawProposalJob(ctx, c, in))
}

// withdrawProposalJob is withdrawProposal answering the job with its detail extras.
func (m Proposals) withdrawProposalJob(
	ctx context.Context,
	call *ops.Call,
	input *ProposalRef,
) (Job, error) {
	_, lockErr := m.Delivery.lock(ctx, call, input.JobID)
	if lockErr != nil {
		return Job{}, lockErr
	}

	var status string

	err := call.Tx.QueryRow(ctx, `SELECT status FROM maintenance.slot_proposals WHERE id = $1 AND job_id = $2 FOR
		UPDATE`, input.ProposalID, input.JobID).
		Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Job{}, apperr.Wrap(err, "select maintenance.slot_proposals")
	}

	if status != proposalPending {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $2 WHERE id = $1`,
		input.ProposalID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.slot_proposals")
	}
	// the contractor's time change HQ had sent on: HQ keeps the agreed time, and the contractor may accept or propose
	// again (IR443)
	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'kept' WHERE status = 'sent_to_client'
		AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = $1)`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"proposal.withdrawn",
		"jobs.withdrawProposal",
		"ProposalWithdrawn",
		"",
	)
}

// ---- jobs.respondProposal ----

// RespondInput is jobs.respondProposal input.
type RespondInput struct {
	JobID          uuid.UUID `json:"jobId"`
	ProposalID     uuid.UUID `json:"proposalId"`
	Decision       string    `json:"decision"`
	DeclineReason  *string   `json:"declineReason,omitempty"`
	Comment        *string   `json:"comment,omitempty"`
	PreferredSlots []Slot    `json:"preferredSlots,omitempty"`
}

// Validate implements ops.Validator.
func (in *RespondInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ProposalID == uuid.Nil {
		fieldErrs["proposalId"] = apperr.KeyRequired
	}

	switch in.Decision {
	case decisionAccept:
		if in.DeclineReason != nil || in.PreferredSlots != nil {
			fieldErrs["decision"] = "error.fieldsNotAllowed"
		}
	case decisionDecline:
		if in.DeclineReason == nil ||
			(*in.DeclineReason != "not_home" && *in.DeclineReason != "too_late" && *in.DeclineReason != codeOther) {
			fieldErrs["declineReason"] = apperr.KeyInvalid
		}

		if n := len(in.PreferredSlots); n != 0 && n != preferredSlotCount {
			fieldErrs["preferredSlots"] = apperr.KeyCount
		}
	default:
		fieldErrs["decision"] = apperr.KeyInvalid
	}

	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > maxReasonLength {
			fieldErrs["comment"] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// respond serves jobs.respondProposal.
//
//	@Summary		jobs.respondProposal (write)
//	@ID				jobs.respondProposal
//	@Description	Authorization: client:self:pending-proposal
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 pending proposal of own customer job; accept checks the hold again (CONFLICT \
//	@Description	errors.proposal_unavailable; IR446); decline needs declineReason; preferredSlots empty or exactly 3 \
//	@Description	distinct future slots
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C09
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			proposalId			path		string			true	"input field proposalId"
//	@Param			request				body		RespondInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/slot-proposals/{proposalId}/respond [post]
func (m Proposals) respond(
	ctx context.Context,
	c *ops.Call,
	in *RespondInput,
) (MaintenanceJob, error) {
	return written(m.respondJob(ctx, c, in))
}

// respondJob is respond answering the job with its detail extras.
func (m Proposals) respondJob(
	ctx context.Context,
	call *ops.Call,
	input *RespondInput,
) (Job, error) {
	job, err := m.Delivery.load(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	var org uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `SELECT customer_org_id FROM maintenance.jobs WHERE id = $1`, input.JobID).
		Scan(&org)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.jobs")
	}

	if org != call.Principal.OrgID {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	currentErr := job.current(call)
	if currentErr != nil {
		return Job{}, currentErr
	}

	slot, hold, err := pendingProposal(ctx, call, input)
	if err != nil {
		return Job{}, err
	}

	if input.Decision == decisionDecline {
		return m.declineProposal(ctx, call, input)
	}

	// a proposal whose time has begun cannot be accepted any more (IR455)
	if !slot.StartAt.After(call.Now) {
		return Job{}, apperr.E(apperr.Conflict, "errors.proposal_expired")
	}
	// the hold is checked again: since the proposal, the contractor may have been suspended or the technician's access or
	// qualification may have ended (IR446); HQ withdraws the proposal and proposes again
	checkHoldErr := m.checkHold(ctx, call, job.unit, hold, slot)
	if checkHoldErr != nil {
		if _, ok := errors.AsType[*apperr.DomainError](checkHoldErr); ok {
			return Job{}, apperr.E(apperr.Conflict, "errors.proposal_unavailable")
		}

		return Job{}, checkHoldErr
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'accepted', decided_at = $2 WHERE id = $1`,
		input.ProposalID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.slot_proposals")
	}

	applyAcceptedErr := m.applyAccepted(ctx, call, input.JobID, hold, slot)
	if applyAcceptedErr != nil {
		return Job{}, applyAcceptedErr
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"proposal.accepted",
		"jobs.respondProposal",
		"ProposalAccepted",
		"",
	)
}

// pendingProposal is the job's pending slot proposal: its slot and the hold it keeps.
func pendingProposal(ctx context.Context, call *ops.Call, input *RespondInput) (Slot, Hold, error) {
	var (
		status, source string
		slot           Slot
		holdRaw        []byte
		hold           Hold
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, source, lower(slot), upper(slot), hold FROM maintenance.slot_proposals
		WHERE
		id = $1 AND job_id = $2 FOR UPDATE`,
		input.ProposalID, input.JobID).
		Scan(&status, &source, &slot.StartAt, &slot.EndAt, &holdRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return slot, hold, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return slot, hold, apperr.Wrap(err, "select maintenance.slot_proposals")
	}

	if status != proposalPending {
		return slot, hold, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	unmarshalErr := json.Unmarshal(holdRaw, &hold)

	return slot, hold, apperr.Wrap(unmarshalErr, "json.Unmarshal")
}

// declineProposal declines the proposal: three new preferred times, if the customer gives them, start the next round
// of preferences (IR113), and the contractor's time sent to the customer goes with it.
func (m Proposals) declineProposal(
	ctx context.Context,
	call *ops.Call,
	input *RespondInput,
) (Job, error) {
	if len(input.PreferredSlots) == preferredSlotCount &&
		!PreferredSlotsOK(call.Now, input.PreferredSlots) {
		return Job{}, apperr.Fields(map[string]string{"preferredSlots": apperr.KeySlotRules})
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'declined', decided_at = $2, decline_reason = $3, decline_comment =
			$4 WHERE id = $1`,
		input.ProposalID,
		call.Now,
		*input.DeclineReason,
		input.Comment,
	)
	if err != nil {
		return Job{}, apperr.Wrap(err, "update maintenance.slot_proposals")
	}

	if len(input.PreferredSlots) == preferredSlotCount {
		raw, marshalErr := json.Marshal(input.PreferredSlots)
		if marshalErr != nil {
			return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
		}

		_, err := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.jobs SET preferred_slots = $2, preference_round = preference_round + 1 WHERE id = $1`,
			input.JobID,
			raw,
		)
		if err != nil {
			return Job{}, apperr.Wrap(err, "update maintenance.jobs")
		}
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'declined' WHERE status = 'sent_to_client'
		AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = $1)`,
		input.JobID,
	)
	if err != nil {
		return Job{}, apperr.Wrap(err, "update maintenance.partner_slot_proposals")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"proposal.declined",
		"jobs.respondProposal",
		"ProposalDeclined",
		deref(input.DeclineReason),
	)
}

// An offer made from an accepted slot proposal stays open a day, or until the visit when that is sooner, and its
// access window reaches a day past the visit's end, as the window of an offer the acceptance creates.
const (
	proposalOfferOpen = 24 * time.Hour
	accessAfterVisit  = 24 * time.Hour
)

// applyAccepted performs the IR128 item 2 effects of an accepted proposal.
func (m Proposals) applyAccepted(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	hold Hold,
	slot Slot,
) error {
	d := m.Delivery
	if hold.Kind == holdInternal {
		return d.createAssignment(ctx, call, job, *hold.MembershipID, slot, "proposal accepted")
	}

	var (
		offer    uuid.UUID
		decision *string
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, decision FROM maintenance.offers WHERE job_id = $1 AND contractor_org_id = $2
		AND
		expired_at IS NULL
		AND (decision IS NULL OR decision = 'accept') ORDER BY offered_at DESC LIMIT 1 FOR
		UPDATE`, job, *hold.ContractorOrgID).
		Scan(&offer, &decision)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return offerFromProposal(ctx, call, job, *hold.ContractorOrgID, slot)
	case err != nil:
		return apperr.Wrap(err, "select maintenance.offers")
	}

	return moveOffer(ctx, call, job, *hold.ContractorOrgID, offer, decision, slot)
}

// offerFromProposal offers the job to the proposal's contractor for its slot under the company's latest terms (the
// demo terms for its first offer): open a day, or until the visit when that is sooner.
func offerFromProposal(
	ctx context.Context,
	call *ops.Call,
	job, contractor uuid.UUID,
	slot Slot,
) error {
	expires := call.Now.Add(proposalOfferOpen)
	if slot.StartAt.Before(expires) {
		expires = slot.StartAt
	}

	terms := DefaultTermsVersion // the company's first offer

	scanErr := call.Tx.QueryRow(ctx, `SELECT terms_version FROM maintenance.offers WHERE contractor_org_id = $1 ORDER BY
		offered_at DESC LIMIT 1`, contractor).
		Scan(&terms)
	if scanErr != nil &&
		!errors.Is(scanErr, pgx.ErrNoRows) {
		return apperr.Wrap(scanErr, "select maintenance.offers")
	}

	candidate, marshalErr := json.Marshal(
		[]Slot{slot},
	) // the client accepted this one time (IR636 item 4)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, candidate_slots, offered_at,
			offer_expires_at,
		access_valid_from, access_valid_until) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
		$5, $6, $5, $7)`,
		job,
		contractor,
		terms,
		candidate,
		call.Now,
		expires,
		slot.EndAt.Add(accessAfterVisit),
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "insert maintenance.offers")
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'offered', contractor_org_id = $2 WHERE id = $1`,
		job,
		contractor,
	)

	return apperr.Wrap(err, "update maintenance.jobs")
}

// moveOffer moves the contractor's open offer to the proposal's slot: its access window follows the visit (IR454), a
// frozen contractor history is frozen again (IR458), its partner proposals are approved, and an accepted offer goes
// back to be assigned again.
func moveOffer(
	ctx context.Context,
	call *ops.Call,
	job, contractor, offer uuid.UUID,
	decision *string,
	slot Slot,
) error {
	// the access window follows the moved visit (IR454): it still has to cover it, as jobs.offer requires, and reaches
	// slot end + 24 h like an offer the acceptance creates; it is never shortened. The client accepted this one time, so
	// it becomes the offer's only candidate (IR636 item 4).
	candidate, marshalErr := json.Marshal([]Slot{slot})
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET candidate_slots = $4, access_valid_from = LEAST(access_valid_from, $2),
		access_valid_until = GREATEST(access_valid_until, $3), version = version + 1, updated_at = platform.app_now() WHERE id
		= $1`,
		offer,
		slot.StartAt,
		slot.EndAt.Add(accessAfterVisit),
		candidate,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update maintenance.offers")
	}
	// a contractor history frozen at an earlier end is frozen again at the widened one (IR458)
	_, execErr = call.Tx.Exec(
		ctx,
		`DELETE FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'organization' AND owner_id = $2`,
		job,
		contractor,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "delete maintenance.job_history_snapshots")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'approved' WHERE offer_id = $1 AND status IN ('pending',
			'sent_to_client')`,
		offer,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	if decision != nil { // accepted Offer: the contractor assigns again for the new visit slot
		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.assignments SET status = 'revoked', reason = 'rescheduled', version = version + 1, updated_at =
				platform.app_now()
			WHERE job_id = $1 AND status = 'active'`,
			job,
		)
		if execErr != nil {
			return apperr.Wrap(execErr, "update maintenance.assignments")
		}

		_, err := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.jobs SET status = 'accepted', assignment_id = NULL, scheduled_slot = NULL WHERE id = $1`,
			job,
		)

		return apperr.Wrap(err, "update maintenance.jobs")
	}

	return nil
}

// candidatesOver reports whether every candidate time of the job's accepted offer has ended (IR642): the contractor can
// assign none of them, so HQ may propose another time like it does after a technician cannot make it.
func candidatesOver(ctx context.Context, call *ops.Call, jobID uuid.UUID) (bool, error) {
	var over bool

	err := call.Tx.QueryRow(ctx, `SELECT COALESCE(bool_and((c->>'endAt')::timestamptz <= $2), false)
		FROM jsonb_array_elements((SELECT candidate_slots FROM maintenance.offers WHERE job_id = $1 AND decision = 'accept'
			ORDER BY decided_at DESC LIMIT 1)) c`, jobID, call.Now).Scan(&over)
	if err != nil {
		return false, apperr.Wrap(err, "select maintenance.offers")
	}

	return over, nil
}

// ---- jobs.requestReschedule ----

// RescheduleInput is jobs.requestReschedule input.
type RescheduleInput struct {
	JobID          uuid.UUID `json:"jobId"`
	PreferredSlots []Slot    `json:"preferredSlots"`
	Comment        *string   `json:"comment,omitempty"`
}

// Validate implements ops.Validator.
func (in *RescheduleInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if len(in.PreferredSlots) != preferredSlotCount {
		fieldErrs["preferredSlots"] = apperr.KeyCount
	}

	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > maxReasonLength {
			fieldErrs["comment"] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// requestReschedule serves jobs.requestReschedule.
//
//	@Summary		jobs.requestReschedule (write)
//	@ID				jobs.requestReschedule
//	@Description	Authorization: client:self:plan-visit
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 periodic_plan job assigned/offered/accepted; ≥48 h before scheduledSlot start; exactly 3 \
//	@Description	preferred slots
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C09
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			request				body		RescheduleInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/request-reschedule [post]
func (m Proposals) requestReschedule(
	ctx context.Context,
	c *ops.Call,
	in *RescheduleInput,
) (MaintenanceJob, error) {
	return written(m.requestRescheduleJob(ctx, c, in))
}

// visitStart is when the job's visit begins, for the 48 hour rule of a reschedule: its scheduled slot or, until a
// technician is assigned, the earliest candidate time of its offer (IR636 item 5); nil when it has neither.
func (Proposals) visitStart(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	job jobRow,
) (*time.Time, error) {
	if job.scheduled != nil {
		return &job.scheduled.StartAt, nil
	}

	var start time.Time

	err := call.Tx.QueryRow(ctx, `SELECT (SELECT min((c->>'startAt')::timestamptz) FROM
		jsonb_array_elements(candidate_slots) c) FROM maintenance.offers WHERE job_id = $1 AND expired_at IS NULL
		AND (decision IS NULL OR decision = 'accept') ORDER BY offered_at DESC LIMIT 1`, jobID).
		Scan(&start)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // no offer yet: there is no visit start, which is not an error
	}

	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.offers")
	}

	return &start, nil
}

// requestRescheduleJob is requestReschedule answering the job with its detail extras.
func (m Proposals) requestRescheduleJob(
	ctx context.Context,
	call *ops.Call,
	input *RescheduleInput,
) (Job, error) {
	job, err := m.Delivery.load(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	var (
		org    uuid.UUID
		origin string
	)

	scanErr := call.Tx.QueryRow(ctx, `SELECT customer_org_id, origin FROM maintenance.jobs WHERE id = $1`, input.JobID).
		Scan(&org, &origin)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.jobs")
	}

	if org != call.Principal.OrgID {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	currentErr := job.current(call)
	if currentErr != nil {
		return Job{}, currentErr
	}

	if origin != "periodic_plan" ||
		(job.status != statusOffered && job.status != statusAccepted && job.status != statusAssigned) {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	start, startErr := m.visitStart(ctx, call, input.JobID, job)
	if startErr != nil {
		return Job{}, startErr
	}

	if start != nil && start.Sub(call.Now) < 48*time.Hour {
		return Job{}, apperr.E(apperr.Conflict, "errors.reschedule_too_late")
	}

	if !PreferredSlotsOK(call.Now, input.PreferredSlots) {
		return Job{}, apperr.Fields(map[string]string{"preferredSlots": apperr.KeySlotRules})
	}

	for _, stmt := range []string{
		`UPDATE maintenance.offers SET decision = 'decline', decided_by = $2, decided_at = $3, decline_reason = 'rescheduled',
			version = version + 1, updated_at = platform.app_now()
			WHERE job_id = $1 AND decision IS NULL AND expired_at IS NULL`,
		`UPDATE maintenance.offers SET access_valid_until = GREATEST(access_valid_from, $3), version = version + 1, updated_at
			= platform.app_now()
			WHERE job_id = $1 AND decision = 'accept' AND access_valid_until > $3 AND $2::uuid IS NOT NULL`,
		`UPDATE maintenance.assignments SET status = 'revoked', reason = 'rescheduled', version = version + 1, updated_at =
			platform.app_now() WHERE job_id = $1 AND status = 'active' AND $2::uuid IS NOT NULL AND $3::timestamptz IS NOT NULL`,
	} {
		_, err := call.Tx.Exec(ctx, stmt, input.JobID, call.Principal.UserID, call.Now)
		if err != nil {
			return Job{}, apperr.Wrap(err, "exec (requestRescheduleJob)")
		}
	}

	withdrawErr := withdrawOpenProposals(ctx, call, input.JobID)
	if withdrawErr != nil {
		return Job{}, withdrawErr
	}

	raw, marshalErr := json.Marshal(input.PreferredSlots)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'requested', contractor_org_id = NULL, assignment_id = NULL, scheduled_slot =
			NULL,
		preferred_slots = $2, preference_round = preference_round + 1 WHERE id = $1`,
		input.JobID,
		raw,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"job.reschedule_requested",
		"jobs.requestReschedule",
		"RescheduleRequested",
		deref(input.Comment),
	)
}

// withdrawOpenProposals ends the proposals a reschedule leaves open, as a cancellation does (IR444, IR445, IR749): the
// client's pending proposal and the contractor's time change pending at HQ or sent to the client. The new preferred
// times start a new round, which HQ answers with a new offer or proposal.
func withdrawOpenProposals(ctx context.Context, call *ops.Call, job uuid.UUID) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $2 WHERE job_id = $1 AND status =
			'pending'`,
		job,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.slot_proposals")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE status IN ('pending','sent_to_client')
		AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = $1)`,
		job,
	)

	return apperr.Wrap(err, "update maintenance.partner_slot_proposals")
}

// ---- partner proposals ----

// PartnerProposeInput is jobs.proposePartnerSlot input.
type PartnerProposeInput struct {
	JobID                  uuid.UUID `json:"jobId"`
	OfferID                uuid.UUID `json:"offerId"`
	Slot                   Slot      `json:"slot"`
	TechnicianMembershipID uuid.UUID `json:"technicianMembershipId"`
	Reason                 string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *PartnerProposeInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil || in.OfferID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.TechnicianMembershipID == uuid.Nil {
		fieldErrs["technicianMembershipId"] = apperr.KeyRequired
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

	return fieldErrs
}

// proposePartner serves jobs.proposePartnerSlot.
//
//	@Summary		jobs.proposePartnerSlot (write)
//	@ID				jobs.proposePartnerSlot
//	@Description	Authorization: contractor:partner.accept:own-valid-offer
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 own open offer (status offered, not expired); no pending partner proposal; qualified own \
//	@Description	technician; reason 1–1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P02
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string				true	"input field jobId"
//	@Param			request				body		PartnerProposeInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/partner-slot-proposals [post]
func (m Proposals) proposePartner(
	ctx context.Context,
	c *ops.Call,
	in *PartnerProposeInput,
) (MaintenanceJob, error) {
	return written(m.proposePartnerJob(ctx, c, in))
}

// proposePartnerJob is proposePartner answering the job with its detail extras.
func (m Proposals) proposePartnerJob(
	ctx context.Context,
	call *ops.Call,
	input *PartnerProposeInput,
) (Job, error) {
	var (
		expires    time.Time
		candidates []Slot
	)

	err := call.Tx.QueryRow(ctx, `SELECT offer_expires_at, candidate_slots FROM maintenance.offers WHERE id = $1 AND
		job_id = $2 AND contractor_org_id = $3 AND decision IS NULL AND expired_at IS NULL FOR UPDATE`,
		input.OfferID, input.JobID, call.Principal.OrgID).
		Scan(&expires, &candidates)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Job{}, apperr.Wrap(err, "select maintenance.offers")
	}

	job, err := m.Delivery.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if job.status != statusOffered || !call.Now.Before(expires) {
		return Job{}, apperr.E(apperr.Conflict, "errors.offer_expired")
	}

	var open bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.partner_slot_proposals WHERE offer_id = $1
		AND
		status IN ('pending','sent_to_client'))`, input.OfferID).
		Scan(&open)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.partner_slot_proposals")
	}

	if open {
		return Job{}, apperr.E(apperr.Conflict, "errors.partner_proposal_pending")
	}

	if !PreferredSlotsOK(call.Now, []Slot{input.Slot}) {
		return Job{}, apperr.Fields(map[string]string{fieldSlot: apperr.KeySlotRules})
	}

	// a candidate time needs no proposal: the contractor assigns a technician for it directly (IR636 item 6)
	if slotIn(candidates, input.Slot) {
		return Job{}, apperr.Fields(map[string]string{fieldSlot: "errors.slot_is_preferred"})
	}

	checkTechnicianErr := m.Delivery.checkTechnician(
		ctx,
		call,
		job.unit,
		input.TechnicianMembershipID,
		true,
		nil,
	)
	if checkTechnicianErr != nil {
		return Job{}, checkTechnicianErr
	}

	checkQualifiedErr := m.Delivery.checkQualified(
		ctx,
		call,
		job.unit,
		input.TechnicianMembershipID,
		input.Slot,
	)
	if checkQualifiedErr != nil {
		return Job{}, checkQualifiedErr
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.partner_slot_proposals (tenant_id, offer_id, slot, technician_membership_id, reason, sent_at,
			status)
		VALUES (current_setting('app.tenant_id')::uuid, $1, tstzrange($2, $3), $4, $5, $6, 'pending')`,
		input.OfferID,
		input.Slot.StartAt,
		input.Slot.EndAt,
		input.TechnicianMembershipID,
		input.Reason,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "insert maintenance.partner_slot_proposals")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"partner_proposal.sent",
		"jobs.proposePartnerSlot",
		"PartnerSlotProposed",
		input.Reason,
	)
}

func (Proposals) partnerProposal(
	ctx context.Context,
	call *ops.Call,
	input *ProposalRef,
	ownOnly bool,
) (partnerSlotProposal, error) {
	var proposal partnerSlotProposal

	stmt := `SELECT p.status, lower(p.slot), upper(p.slot), p.technician_membership_id, o.contractor_org_id
		FROM
		maintenance.partner_slot_proposals p
		JOIN maintenance.offers o ON o.id = p.offer_id WHERE p.id = $1 AND o.job_id = $2`
	args := []any{input.ProposalID, input.JobID}

	if ownOnly {
		stmt += " AND o.contractor_org_id = $3"

		args = append(args, call.Principal.OrgID)
	}

	err := call.Tx.QueryRow(ctx, stmt+" FOR UPDATE OF p", args...).
		Scan(&proposal.status, &proposal.slot.StartAt, &proposal.slot.EndAt, &proposal.tech, &proposal.org)
	if errors.Is(err, pgx.ErrNoRows) {
		return proposal, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return proposal, apperr.Wrap(err, "queryrow (partnerProposal)")
}

// partnerSlotProposal is a contractor's slot proposal: its status, slot, technician and company.
type partnerSlotProposal struct {
	status    string
	slot      Slot
	tech, org uuid.UUID
}

// withdrawPartner serves jobs.withdrawPartnerSlot.
//
//	@Summary		jobs.withdrawPartnerSlot (write)
//	@ID				jobs.withdrawPartnerSlot
//	@Description	Authorization: contractor:partner.accept:own-valid-offer
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 own pending partner proposal only
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P02
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			proposalId			path		string		true	"input field proposalId"
//	@Param			request				body		ProposalRef	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/partner-slot-proposals/{proposalId}/withdraw [post]
func (m Proposals) withdrawPartner(
	ctx context.Context,
	c *ops.Call,
	in *ProposalRef,
) (MaintenanceJob, error) {
	return written(m.withdrawPartnerJob(ctx, c, in))
}

// withdrawPartnerJob is withdrawPartner answering the job with its detail extras.
func (m Proposals) withdrawPartnerJob(
	ctx context.Context,
	call *ops.Call,
	input *ProposalRef,
) (Job, error) {
	proposal, err := m.partnerProposal(ctx, call, input, true)
	if err != nil {
		return Job{}, err
	}

	_, lockErr := m.Delivery.lock(ctx, call, input.JobID)
	if lockErr != nil {
		return Job{}, lockErr
	}

	if proposal.status != proposalPending {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE id = $1`,
		input.ProposalID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"partner_proposal.withdrawn",
		"jobs.withdrawPartnerSlot",
		"PartnerSlotWithdrawn",
		"",
	)
}

// ResolveInput is jobs.resolvePartnerSlot input.
type ResolveInput struct {
	JobID      uuid.UUID  `json:"jobId"`
	ProposalID uuid.UUID  `json:"proposalId"`
	Decision   string     `json:"decision"`
	ReplyBy    *time.Time `json:"replyBy,omitempty"`
}

// Validate implements ops.Validator.
func (in *ResolveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil || in.ProposalID == uuid.Nil {
		fieldErrs["proposalId"] = apperr.KeyRequired
	}

	switch in.Decision {
	case "send_to_client":
		if in.ReplyBy == nil {
			fieldErrs["replyBy"] = apperr.KeyRequired
		}
	case "keep":
		if in.ReplyBy != nil {
			fieldErrs["replyBy"] = apperr.KeyNotAllowed
		}
	default:
		fieldErrs["decision"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// resolvePartner serves jobs.resolvePartnerSlot.
//
//	@Summary		jobs.resolvePartnerSlot (write)
//	@ID				jobs.resolvePartnerSlot
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 pending partner proposal; send_to_client creates a SlotProposal(source=contractor) and needs \
//	@Description	replyBy not after the slot start (IR455)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			proposalId			path		string			true	"input field proposalId"
//	@Param			request				body		ResolveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/partner-slot-proposals/{proposalId}/resolve [post]
func (m Proposals) resolvePartner(
	ctx context.Context,
	c *ops.Call,
	in *ResolveInput,
) (MaintenanceJob, error) {
	return written(m.resolvePartnerJob(ctx, c, in))
}

// resolvePartnerJob is resolvePartner answering the job with its detail extras.
func (m Proposals) resolvePartnerJob(
	ctx context.Context,
	call *ops.Call,
	input *ResolveInput,
) (Job, error) {
	proposal, err := m.partnerProposal(
		ctx,
		call,
		&ProposalRef{JobID: input.JobID, ProposalID: input.ProposalID},
		false,
	)
	if err != nil {
		return Job{}, err
	}

	_, lockErr := m.Delivery.lock(ctx, call, input.JobID)
	if lockErr != nil {
		return Job{}, lockErr
	}

	if proposal.status != proposalPending {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	next := "kept"

	if input.Decision == "send_to_client" {
		err := m.sendToClient(ctx, call, input, proposal)
		if err != nil {
			return Job{}, err
		}

		next = "sent_to_client"
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = $2 WHERE id = $1`,
		input.ProposalID,
		next,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	return m.finish(
		ctx,
		call,
		input.JobID,
		"partner_proposal."+next,
		"jobs.resolvePartnerSlot",
		"PartnerSlotResolved",
		"",
	)
}

// sendToClient puts the contractor's time to the customer as an HQ slot proposal held for the contractor and its
// technician; one pending proposal at a time, answered by a reply-by time the visit allows.
func (m Proposals) sendToClient(
	ctx context.Context,
	call *ops.Call,
	input *ResolveInput,
	proposal partnerSlotProposal,
) error {
	pending, err := m.pending(ctx, call, input.JobID)
	if err != nil {
		return err
	}

	if pending {
		return apperr.E(apperr.Conflict, "errors.proposal_pending")
	}

	if !replyByOK(call.Now, *input.ReplyBy, proposal.slot.StartAt) {
		return apperr.Fields(map[string]string{"replyBy": apperr.KeyRange})
	}

	hold := Hold{
		Kind:                   holdContractor,
		ContractorOrgID:        &proposal.org,
		TechnicianMembershipID: &proposal.tech,
		MembershipID:           nil,
	}

	err = m.insertProposal(
		ctx,
		call,
		input.JobID,
		"contractor",
		proposal.slot,
		hold,
		"Partner time proposal",
		*input.ReplyBy,
	)

	return err
}

// ExpireProposals applies IR128 item 4 in the worker tick.
func ExpireProposals(ctx context.Context, txn pgx.Tx, now time.Time) (int, error) {
	rows, err := txn.Query(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'expired', decided_at = $1 WHERE status = 'pending' AND reply_by <= $1
			RETURNING job_id`,
		now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update maintenance.slot_proposals")
	}

	jobs, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil || len(jobs) == 0 {
		return 0, apperr.Wrap(err, "update maintenance.slot_proposals")
	}
	// a contractor's time change the client left unanswered ends as declined, as when the client declines (IR443)
	_, execErr := txn.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'declined' WHERE status = 'sent_to_client'
		AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = ANY($1))`,
		jobs,
	)
	if execErr != nil {
		return 0, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	return len(jobs), nil
}

// RegisterProposals binds the IR128 operations.
func RegisterProposals(reg *ops.Registry, proposals Proposals) {
	ops.Register(reg, "jobs.proposeSlot", proposals.proposeSlot)
	ops.Register(reg, "jobs.withdrawProposal", proposals.withdrawProposal)
	ops.Register(reg, "jobs.respondProposal", proposals.respond)
	ops.Register(reg, "jobs.requestReschedule", proposals.requestReschedule)
	ops.Register(reg, "jobs.proposePartnerSlot", proposals.proposePartner)
	ops.Register(reg, "jobs.withdrawPartnerSlot", proposals.withdrawPartner)
	ops.Register(reg, "jobs.resolvePartnerSlot", proposals.resolvePartner)
}
