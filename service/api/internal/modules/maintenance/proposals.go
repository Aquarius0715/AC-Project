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

// Proposals is the IR128 operation set.
type Proposals struct {
	Delivery Delivery
}

func (m Proposals) jobs() Jobs { return m.Delivery.Jobs }

func replyByOK(now, replyBy time.Time) bool {
	return replyBy.After(now) && !replyBy.After(now.Add(7*24*time.Hour))
}

func (m Proposals) pending(ctx context.Context, c *ops.Call, job uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'pending')`, job).Scan(&b)
	return b, err
}

func (m Proposals) finish(ctx context.Context, c *ops.Call, job uuid.UUID, action, op, event, reason string) (Job, error) {
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`, job); err != nil {
		return Job{}, err
	}
	if err := m.jobs().event(ctx, c, job, action, nil); err != nil {
		return Job{}, err
	}
	return m.jobs().finishScoped(ctx, c, job, op, event, reason)
}

// checkHold validates a SlotHold for a slot (IR128 item 1).
func (m Proposals) checkHold(ctx context.Context, c *ops.Call, unit uuid.UUID, h Hold, slot Slot) error {
	d := m.Delivery
	switch h.Kind {
	case "internal":
		if h.MembershipID == nil || h.ContractorOrgID != nil {
			return apperr.Fields(map[string]string{"hold": "error.invalid"})
		}
		if err := d.checkTechnician(ctx, c, unit, *h.MembershipID, false, nil); err != nil {
			return err
		}
		return d.checkQualified(ctx, c, unit, *h.MembershipID, slot)
	case "contractor":
		if h.ContractorOrgID == nil || h.MembershipID != nil {
			return apperr.Fields(map[string]string{"hold": "error.invalid"})
		}
		kind, status, found, err := d.Directory.OrgState(ctx, c, *h.ContractorOrgID)
		if err != nil {
			return err
		}
		if !found || kind != "contractor" || status != "active" {
			return apperr.E(apperr.NotFound, "error.notFound")
		}
		var suspended bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1 AND status = 'suspended')`, *h.ContractorOrgID).Scan(&suspended); err != nil {
			return err
		}
		if suspended {
			return apperr.E(apperr.Conflict, "errors.contractor_suspended")
		}
		if h.TechnicianMembershipID != nil {
			if err := d.checkTechnician(ctx, c, unit, *h.TechnicianMembershipID, true, h.ContractorOrgID); err != nil {
				return err
			}
			return d.checkQualified(ctx, c, unit, *h.TechnicianMembershipID, slot)
		}
		return nil
	}
	return apperr.Fields(map[string]string{"hold": "error.invalid"})
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	trimmedReason(&in.Message, 1000, "message", fe)
	if in.ReplyBy.IsZero() {
		fe["replyBy"] = "error.required"
	}
	return fe
}

// @Summary		jobs.proposeSlot (write)
// @ID				jobs.proposeSlot
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 status requested (or offered/accepted/assigned after cant_make/decline); slot not one of preferredSlots (errors.slot_is_preferred); no pending proposal; message 1–1000; replyBy future ≤7 days
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string			true	"input field jobId"
// @Param			request				body		ProposeInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/slot-proposals [post]
func (m Proposals) proposeSlot(ctx context.Context, c *ops.Call, in *ProposeInput) (Job, error) {
	r, err := m.Delivery.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	allowed := r.status == "requested"
	switch r.status {
	case "offered", "accepted":
		var n int
		if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.partner_slot_proposals p JOIN maintenance.offers o ON o.id = p.offer_id
			WHERE o.job_id = $1 AND p.status <> 'withdrawn'`, in.JobID).Scan(&n); err != nil {
			return Job{}, err
		}
		allowed = n > 0
	case "assigned":
		var ack string
		_ = c.Tx.QueryRow(ctx, `SELECT acknowledgement FROM maintenance.assignments WHERE job_id = $1 AND status = 'active'`, in.JobID).Scan(&ack)
		allowed = ack == "cant_make"
	}
	if !allowed {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if p, err := m.pending(ctx, c, in.JobID); err != nil || p {
		if err != nil {
			return Job{}, err
		}
		return Job{}, apperr.E(apperr.Conflict, "errors.proposal_pending")
	}
	if !PreferredSlotsOK(c.Now, []Slot{in.Slot}) {
		return Job{}, apperr.Fields(map[string]string{"slot": "error.slotRules"})
	}
	for _, p := range r.preferred {
		if sameSlot(p, in.Slot) {
			return Job{}, apperr.Fields(map[string]string{"slot": "errors.slot_is_preferred"})
		}
	}
	if !replyByOK(c.Now, in.ReplyBy) {
		return Job{}, apperr.Fields(map[string]string{"replyBy": "error.range"})
	}
	if err := m.checkHold(ctx, c, r.unit, in.Hold, in.Slot); err != nil {
		return Job{}, err
	}
	if err := m.insertProposal(ctx, c, in.JobID, "hq", in.Slot, in.Hold, in.Message, in.ReplyBy); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "proposal.sent", "jobs.proposeSlot", "SlotProposed", "")
}

func (m Proposals) insertProposal(ctx context.Context, c *ops.Call, job uuid.UUID, source string, slot Slot, hold Hold, msg string, replyBy time.Time) error {
	h, _ := json.Marshal(hold)
	_, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.slot_proposals (tenant_id, job_id, source, slot, hold, message, reply_by, status, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, tstzrange($3, $4), $5, $6, $7, 'pending', $8)`, job, source, slot.StartAt, slot.EndAt, h, msg, replyBy, c.Now)
	return err
}

// ---- jobs.withdrawProposal / jobs.withdrawPartnerSlot ----

// ProposalRef is the {jobId, proposalId} input.
type ProposalRef struct {
	JobID      uuid.UUID `json:"jobId"`
	ProposalID uuid.UUID `json:"proposalId"`
}

// Validate implements ops.Validator.
func (in *ProposalRef) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ProposalID == uuid.Nil {
		fe["proposalId"] = "error.required"
	}
	return fe
}

// @Summary		jobs.withdrawProposal (write)
// @ID				jobs.withdrawProposal
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 pending own-tenant proposal only; releases hold
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string		true	"input field jobId"
// @Param			proposalId			path		string		true	"input field proposalId"
// @Param			request				body		ProposalRef	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/slot-proposals/{proposalId}/withdraw [post]
func (m Proposals) withdrawProposal(ctx context.Context, c *ops.Call, in *ProposalRef) (Job, error) {
	if _, err := m.Delivery.lock(ctx, c, in.JobID); err != nil {
		return Job{}, err
	}
	var status string
	err := c.Tx.QueryRow(ctx, `SELECT status FROM maintenance.slot_proposals WHERE id = $1 AND job_id = $2 FOR UPDATE`, in.ProposalID, in.JobID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Job{}, err
	}
	if status != "pending" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $2 WHERE id = $1`, in.ProposalID, c.Now); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "proposal.withdrawn", "jobs.withdrawProposal", "ProposalWithdrawn", "")
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ProposalID == uuid.Nil {
		fe["proposalId"] = "error.required"
	}
	switch in.Decision {
	case "accept":
		if in.DeclineReason != nil || in.PreferredSlots != nil {
			fe["decision"] = "error.fieldsNotAllowed"
		}
	case "decline":
		if in.DeclineReason == nil || (*in.DeclineReason != "not_home" && *in.DeclineReason != "too_late" && *in.DeclineReason != "other") {
			fe["declineReason"] = "error.invalid"
		}
		if n := len(in.PreferredSlots); n != 0 && n != 3 {
			fe["preferredSlots"] = "error.count"
		}
	default:
		fe["decision"] = "error.invalid"
	}
	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > 1000 {
			fe["comment"] = "error.length"
		}
	}
	return fe
}

// @Summary		jobs.respondProposal (write)
// @ID				jobs.respondProposal
// @Description	Authorization: client:self:pending-proposal
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 pending proposal of own customer job; decline needs declineReason; preferredSlots empty or exactly 3 distinct future slots
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C09
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string			true	"input field jobId"
// @Param			proposalId			path		string			true	"input field proposalId"
// @Param			request				body		RespondInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/slot-proposals/{proposalId}/respond [post]
func (m Proposals) respond(ctx context.Context, c *ops.Call, in *RespondInput) (Job, error) {
	r, err := m.Delivery.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	var org uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT customer_org_id FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&org); err != nil {
		return Job{}, err
	}
	if org != c.Principal.OrgID {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	var status, source string
	var slot Slot
	var holdRaw []byte
	err = c.Tx.QueryRow(ctx, `SELECT status, source, lower(slot), upper(slot), hold FROM maintenance.slot_proposals WHERE id = $1 AND job_id = $2 FOR UPDATE`,
		in.ProposalID, in.JobID).Scan(&status, &source, &slot.StartAt, &slot.EndAt, &holdRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Job{}, err
	}
	if status != "pending" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var hold Hold
	if err := json.Unmarshal(holdRaw, &hold); err != nil {
		return Job{}, err
	}
	if in.Decision == "decline" {
		if len(in.PreferredSlots) == 3 && !PreferredSlotsOK(c.Now, in.PreferredSlots) {
			return Job{}, apperr.Fields(map[string]string{"preferredSlots": "error.slotRules"})
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.slot_proposals SET status = 'declined', decided_at = $2, decline_reason = $3, decline_comment = $4 WHERE id = $1`,
			in.ProposalID, c.Now, *in.DeclineReason, in.Comment); err != nil {
			return Job{}, err
		}
		if len(in.PreferredSlots) == 3 {
			raw, _ := json.Marshal(in.PreferredSlots)
			if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET preferred_slots = $2, preference_round = preference_round + 1 WHERE id = $1`, in.JobID, raw); err != nil {
				return Job{}, err
			}
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.partner_slot_proposals SET status = 'declined' WHERE status = 'sent_to_client'
			AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = $1)`, in.JobID); err != nil {
			return Job{}, err
		}
		return m.finish(ctx, c, in.JobID, "proposal.declined", "jobs.respondProposal", "ProposalDeclined", deref(in.DeclineReason))
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.slot_proposals SET status = 'accepted', decided_at = $2 WHERE id = $1`, in.ProposalID, c.Now); err != nil {
		return Job{}, err
	}
	if err := m.applyAccepted(ctx, c, in.JobID, r, hold, slot); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "proposal.accepted", "jobs.respondProposal", "ProposalAccepted", "")
}

// applyAccepted performs the IR128 item 2 effects of an accepted proposal.
func (m Proposals) applyAccepted(ctx context.Context, c *ops.Call, job uuid.UUID, r jobRow, h Hold, slot Slot) error {
	d := m.Delivery
	if h.Kind == "internal" {
		return d.createAssignment(ctx, c, job, *h.MembershipID, slot, "proposal accepted")
	}
	var offer uuid.UUID
	var decision *string
	err := c.Tx.QueryRow(ctx, `SELECT id, decision FROM maintenance.offers WHERE job_id = $1 AND contractor_org_id = $2 AND expired_at IS NULL
		AND (decision IS NULL OR decision = 'accept') ORDER BY offered_at DESC LIMIT 1 FOR UPDATE`, job, *h.ContractorOrgID).Scan(&offer, &decision)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		expires := c.Now.Add(24 * time.Hour)
		if slot.StartAt.Before(expires) {
			expires = slot.StartAt
		}
		terms := DefaultTermsVersion
		_ = c.Tx.QueryRow(ctx, `SELECT terms_version FROM maintenance.offers WHERE contractor_org_id = $1 ORDER BY offered_at DESC LIMIT 1`, *h.ContractorOrgID).Scan(&terms)
		if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at,
			access_valid_from, access_valid_until) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, tstzrange($4, $5), $6, $7, $6, $8)`,
			job, *h.ContractorOrgID, terms, slot.StartAt, slot.EndAt, c.Now, expires, slot.EndAt.Add(24*time.Hour)); err != nil {
			return err
		}
		_, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'offered', contractor_org_id = $2 WHERE id = $1`, job, *h.ContractorOrgID)
		return err
	case err != nil:
		return err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.offers SET visit_slot = tstzrange($2, $3), version = version + 1, updated_at = platform.app_now() WHERE id = $1`, offer, slot.StartAt, slot.EndAt); err != nil {
		return err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.partner_slot_proposals SET status = 'approved' WHERE offer_id = $1 AND status IN ('pending','sent_to_client')`, offer); err != nil {
		return err
	}
	if decision != nil { // accepted Offer: the contractor assigns again for the new visit slot
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.assignments SET status = 'revoked', reason = 'rescheduled', version = version + 1, updated_at = platform.app_now()
			WHERE job_id = $1 AND status = 'active'`, job); err != nil {
			return err
		}
		_, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'accepted', assignment_id = NULL, scheduled_slot = NULL WHERE id = $1`, job)
		return err
	}
	return nil
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if len(in.PreferredSlots) != 3 {
		fe["preferredSlots"] = "error.count"
	}
	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > 1000 {
			fe["comment"] = "error.length"
		}
	}
	return fe
}

// @Summary		jobs.requestReschedule (write)
// @ID				jobs.requestReschedule
// @Description	Authorization: client:self:plan-visit
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 periodic_plan job assigned/offered/accepted; ≥48 h before scheduledSlot start; exactly 3 preferred slots
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C09
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string			true	"input field jobId"
// @Param			request				body		RescheduleInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/request-reschedule [post]
func (m Proposals) requestReschedule(ctx context.Context, c *ops.Call, in *RescheduleInput) (Job, error) {
	r, err := m.Delivery.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	var org uuid.UUID
	var origin string
	if err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, origin FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&org, &origin); err != nil {
		return Job{}, err
	}
	if org != c.Principal.OrgID {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if origin != "periodic_plan" || (r.status != "offered" && r.status != "accepted" && r.status != "assigned") {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var start *time.Time
	if r.scheduled != nil {
		start = &r.scheduled.StartAt
	} else {
		var s time.Time
		if err := c.Tx.QueryRow(ctx, `SELECT lower(visit_slot) FROM maintenance.offers WHERE job_id = $1 AND expired_at IS NULL AND (decision IS NULL OR decision = 'accept')
			ORDER BY offered_at DESC LIMIT 1`, in.JobID).Scan(&s); err == nil {
			start = &s
		}
	}
	if start != nil && start.Sub(c.Now) < 48*time.Hour {
		return Job{}, apperr.E(apperr.Conflict, "errors.reschedule_too_late")
	}
	if !PreferredSlotsOK(c.Now, in.PreferredSlots) {
		return Job{}, apperr.Fields(map[string]string{"preferredSlots": "error.slotRules"})
	}
	for _, q := range []string{
		`UPDATE maintenance.offers SET decision = 'decline', decided_by = $2, decided_at = $3, decline_reason = 'rescheduled', version = version + 1, updated_at = platform.app_now()
			WHERE job_id = $1 AND decision IS NULL AND expired_at IS NULL`,
		`UPDATE maintenance.offers SET access_valid_until = GREATEST(access_valid_from, $3), version = version + 1, updated_at = platform.app_now()
			WHERE job_id = $1 AND decision = 'accept' AND access_valid_until > $3 AND $2::uuid IS NOT NULL`,
		`UPDATE maintenance.assignments SET status = 'revoked', reason = 'rescheduled', version = version + 1, updated_at = platform.app_now() WHERE job_id = $1 AND status = 'active' AND $2::uuid IS NOT NULL AND $3::timestamptz IS NOT NULL`,
	} {
		if _, err := c.Tx.Exec(ctx, q, in.JobID, c.Principal.UserID, c.Now); err != nil {
			return Job{}, err
		}
	}
	raw, _ := json.Marshal(in.PreferredSlots)
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'requested', contractor_org_id = NULL, assignment_id = NULL, scheduled_slot = NULL,
		preferred_slots = $2, preference_round = preference_round + 1 WHERE id = $1`, in.JobID, raw); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "job.reschedule_requested", "jobs.requestReschedule", "RescheduleRequested", deref(in.Comment))
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil || in.OfferID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.TechnicianMembershipID == uuid.Nil {
		fe["technicianMembershipId"] = "error.required"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// @Summary		jobs.proposePartnerSlot (write)
// @ID				jobs.proposePartnerSlot
// @Description	Authorization: contractor:partner.accept:own-valid-offer
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 own open offer (status offered, not expired); no pending partner proposal; qualified own technician; reason 1–1000
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-P02
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer				true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string				true	"input field jobId"
// @Param			request				body		PartnerProposeInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/partner-slot-proposals [post]
func (m Proposals) proposePartner(ctx context.Context, c *ops.Call, in *PartnerProposeInput) (Job, error) {
	var expires time.Time
	err := c.Tx.QueryRow(ctx, `SELECT offer_expires_at FROM maintenance.offers WHERE id = $1 AND job_id = $2 AND contractor_org_id = $3 AND decision IS NULL AND expired_at IS NULL FOR UPDATE`,
		in.OfferID, in.JobID, c.Principal.OrgID).Scan(&expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Job{}, err
	}
	r, err := m.Delivery.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if r.status != "offered" || !c.Now.Before(expires) {
		return Job{}, apperr.E(apperr.Conflict, "errors.offer_expired")
	}
	var open bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status IN ('pending','sent_to_client'))`, in.OfferID).Scan(&open); err != nil {
		return Job{}, err
	}
	if open {
		return Job{}, apperr.E(apperr.Conflict, "errors.partner_proposal_pending")
	}
	if !PreferredSlotsOK(c.Now, []Slot{in.Slot}) {
		return Job{}, apperr.Fields(map[string]string{"slot": "error.slotRules"})
	}
	if err := m.Delivery.checkTechnician(ctx, c, r.unit, in.TechnicianMembershipID, true, nil); err != nil {
		return Job{}, err
	}
	if err := m.Delivery.checkQualified(ctx, c, r.unit, in.TechnicianMembershipID, in.Slot); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.partner_slot_proposals (tenant_id, offer_id, slot, technician_membership_id, reason, sent_at, status)
		VALUES (current_setting('app.tenant_id')::uuid, $1, tstzrange($2, $3), $4, $5, $6, 'pending')`, in.OfferID, in.Slot.StartAt, in.Slot.EndAt,
		in.TechnicianMembershipID, in.Reason, c.Now); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "partner_proposal.sent", "jobs.proposePartnerSlot", "PartnerSlotProposed", in.Reason)
}

func (m Proposals) partnerProposal(ctx context.Context, c *ops.Call, in *ProposalRef, ownOnly bool) (string, uuid.UUID, Slot, uuid.UUID, uuid.UUID, error) {
	var status string
	var offer, tech, org uuid.UUID
	var slot Slot
	q := `SELECT p.status, p.offer_id, lower(p.slot), upper(p.slot), p.technician_membership_id, o.contractor_org_id FROM maintenance.partner_slot_proposals p
		JOIN maintenance.offers o ON o.id = p.offer_id WHERE p.id = $1 AND o.job_id = $2`
	args := []any{in.ProposalID, in.JobID}
	if ownOnly {
		q += " AND o.contractor_org_id = $3"
		args = append(args, c.Principal.OrgID)
	}
	err := c.Tx.QueryRow(ctx, q+" FOR UPDATE OF p", args...).Scan(&status, &offer, &slot.StartAt, &slot.EndAt, &tech, &org)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", offer, slot, tech, org, apperr.E(apperr.NotFound, "error.notFound")
	}
	return status, offer, slot, tech, org, err
}

// @Summary		jobs.withdrawPartnerSlot (write)
// @ID				jobs.withdrawPartnerSlot
// @Description	Authorization: contractor:partner.accept:own-valid-offer
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 own pending partner proposal only
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-P02
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string		true	"input field jobId"
// @Param			proposalId			path		string		true	"input field proposalId"
// @Param			request				body		ProposalRef	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/partner-slot-proposals/{proposalId}/withdraw [post]
func (m Proposals) withdrawPartner(ctx context.Context, c *ops.Call, in *ProposalRef) (Job, error) {
	status, _, _, _, _, err := m.partnerProposal(ctx, c, in, true)
	if err != nil {
		return Job{}, err
	}
	if _, err := m.Delivery.lock(ctx, c, in.JobID); err != nil {
		return Job{}, err
	}
	if status != "pending" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE id = $1`, in.ProposalID); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "partner_proposal.withdrawn", "jobs.withdrawPartnerSlot", "PartnerSlotWithdrawn", "")
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil || in.ProposalID == uuid.Nil {
		fe["proposalId"] = "error.required"
	}
	switch in.Decision {
	case "send_to_client":
		if in.ReplyBy == nil {
			fe["replyBy"] = "error.required"
		}
	case "keep":
		if in.ReplyBy != nil {
			fe["replyBy"] = "error.notAllowed"
		}
	default:
		fe["decision"] = "error.invalid"
	}
	return fe
}

// @Summary		jobs.resolvePartnerSlot (write)
// @ID				jobs.resolvePartnerSlot
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR113 pending partner proposal; send_to_client creates a SlotProposal(source=contractor) and needs replyBy
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string			true	"input field jobId"
// @Param			proposalId			path		string			true	"input field proposalId"
// @Param			request				body		ResolveInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Job}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/partner-slot-proposals/{proposalId}/resolve [post]
func (m Proposals) resolvePartner(ctx context.Context, c *ops.Call, in *ResolveInput) (Job, error) {
	status, _, slot, tech, org, err := m.partnerProposal(ctx, c, &ProposalRef{JobID: in.JobID, ProposalID: in.ProposalID}, false)
	if err != nil {
		return Job{}, err
	}
	if _, err := m.Delivery.lock(ctx, c, in.JobID); err != nil {
		return Job{}, err
	}
	if status != "pending" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	next := "kept"
	if in.Decision == "send_to_client" {
		if p, err := m.pending(ctx, c, in.JobID); err != nil || p {
			if err != nil {
				return Job{}, err
			}
			return Job{}, apperr.E(apperr.Conflict, "errors.proposal_pending")
		}
		if !replyByOK(c.Now, *in.ReplyBy) {
			return Job{}, apperr.Fields(map[string]string{"replyBy": "error.range"})
		}
		hold := Hold{Kind: "contractor", ContractorOrgID: &org, TechnicianMembershipID: &tech}
		if err := m.insertProposal(ctx, c, in.JobID, "contractor", slot, hold, "Partner time proposal", *in.ReplyBy); err != nil {
			return Job{}, err
		}
		next = "sent_to_client"
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.partner_slot_proposals SET status = $2 WHERE id = $1`, in.ProposalID, next); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "partner_proposal."+next, "jobs.resolvePartnerSlot", "PartnerSlotResolved", "")
}

// ExpireProposals applies IR128 item 4 in the worker tick.
func ExpireProposals(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	tag, err := tx.Exec(ctx, `UPDATE maintenance.slot_proposals SET status = 'expired', decided_at = $1 WHERE status = 'pending' AND reply_by <= $1`, now)
	return int(tag.RowsAffected()), err
}

// RegisterProposals binds the IR128 operations.
func RegisterProposals(r *ops.Registry, m Proposals) {
	ops.Register(r, "jobs.proposeSlot", m.proposeSlot)
	ops.Register(r, "jobs.withdrawProposal", m.withdrawProposal)
	ops.Register(r, "jobs.respondProposal", m.respond)
	ops.Register(r, "jobs.requestReschedule", m.requestReschedule)
	ops.Register(r, "jobs.proposePartnerSlot", m.proposePartner)
	ops.Register(r, "jobs.withdrawPartnerSlot", m.withdrawPartner)
	ops.Register(r, "jobs.resolvePartnerSlot", m.resolvePartner)
}
