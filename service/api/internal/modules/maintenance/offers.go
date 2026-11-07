package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// Directory is what offers and assignments need from Identity & access.
type Directory interface {
	OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (identity.OrgStateResult, error)
	Technician(
		ctx context.Context,
		c *ops.Call,
		membership uuid.UUID,
	) (identity.Technician, bool, error)
	Qualified(
		ctx context.Context,
		c *ops.Call,
		membership uuid.UUID,
		codes []string,
		start, end time.Time,
	) (bool, error)
}

// ServiceUnits returns organization, property and service scope of a unit (Assets).
type ServiceUnits interface {
	UnitService(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (org, property uuid.UUID, scope []string, err error)
}

// The organization kind and status a contractor needs to be offered a job or to hold a proposed slot
// (Directory.OrgState).
const (
	orgKindContractor = "contractor"
	orgStatusActive   = "active"
)

// qualificationFor is the QualificationCode a Unit.serviceScope entry requires (fixture qualificationRequirements).
func qualificationFor(scope string) (string, bool) {
	switch scope {
	case "indoor":
		return "demo_indoor", true
	case "outdoor":
		return "demo_outdoor", true
	case "electrical":
		return "demo_electrical", true
	}

	return "", false
}

// Delivery is the offer / decision / assignment operation set (IR123).
type Delivery struct {
	Jobs      Jobs
	Directory Directory
	Units     ServiceUnits
}

type jobRow struct {
	status     string
	unit       uuid.UUID
	contractor *uuid.UUID
	planID     *uuid.UUID
	occurrence *time.Time
	requested  Slot
	preferred  []Slot
	scheduled  *Slot
	assignment *uuid.UUID
	version    int
}

// lock loads a job for update without the role scope (callers apply their own rules) and checks the version.
func (m Delivery) lock(ctx context.Context, call *ops.Call, id uuid.UUID) (jobRow, error) {
	r, err := m.load(ctx, call, id)
	if err != nil {
		return r, err
	}

	return r, r.current(call)
}

// current checks the expected version of a loaded job (CONFLICT).
func (r jobRow) current(c *ops.Call) error {
	if r.version != *c.ExpectedVersion {
		return apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return nil
}

// load locks a job for update like lock, without the version. A handler whose scope check needs the row checks the
// scope first and the version after it (current), so a caller outside the scope gets NOT_FOUND / FORBIDDEN for any
// version and learns nothing of the job (IR366).
func (Delivery) load(ctx context.Context, call *ops.Call, jobID uuid.UUID) (jobRow, error) {
	var (
		job                  jobRow
		pref                 []byte
		schedStart, schedEnd *time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id, contractor_org_id, plan_id, occurrence_at, lower(requested_slot),
		upper(requested_slot), preferred_slots,
		lower(scheduled_slot), upper(scheduled_slot), assignment_id, version FROM maintenance.jobs WHERE id = $1 FOR
		UPDATE`, jobID).
		Scan(&job.status, &job.unit, &job.contractor, &job.planID, &job.occurrence, &job.requested.StartAt,
			&job.requested.EndAt, &pref,
			&schedStart, &schedEnd, &job.assignment, &job.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return job, apperr.Wrap(err, "select maintenance.jobs")
	}

	if schedStart != nil && schedEnd != nil {
		job.scheduled = &Slot{*schedStart, *schedEnd}
	}

	unmarshalSlotsErr := unmarshalSlots(pref, &job.preferred)
	if unmarshalSlotsErr != nil {
		return job, unmarshalSlotsErr
	}

	return job, nil
}

func sameSlot(a, b Slot) bool { return a.StartAt.Equal(b.StartAt) && a.EndAt.Equal(b.EndAt) }

// slotIn reports whether slots holds slot.
func slotIn(slots []Slot, slot Slot) bool {
	return slices.ContainsFunc(slots, func(s Slot) bool { return sameSlot(s, slot) })
}

// agreed reports whether a slot is an agreed slot of the job (IR113 item 3).
func (Delivery) agreed(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	job jobRow,
	slot Slot,
) (bool, error) {
	for _, p := range job.preferred {
		if sameSlot(p, slot) {
			return true, nil
		}
	}

	var count int

	err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'accepted'
		AND slot = tstzrange($2, $3)`,
		jobID, slot.StartAt, slot.EndAt).
		Scan(&count)
	if err != nil {
		return false, apperr.Wrap(err, "select maintenance.slot_proposals")
	}

	if count > 0 {
		return true, nil
	}

	if job.planID != nil && job.occurrence != nil {
		dur := job.requested.EndAt.Sub(job.requested.StartAt)

		return sameSlot(Slot{*job.occurrence, job.occurrence.Add(dur)}, slot), nil
	}

	return false, nil
}

// candidates are the times a job is offered for (IR636 item 1): the agreed times that have not ended — the client's
// preferred times and the slots of accepted proposals, or else the plan occurrence — in the order the client ranked
// them. The contractor picks one of them when it assigns a technician.
func (Delivery) candidates(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	job jobRow,
) ([]Slot, error) {
	out := make([]Slot, 0, len(job.preferred)+1)
	add := func(slot Slot) {
		if slot.EndAt.After(call.Now) && !slotIn(out, slot) {
			out = append(out, slot)
		}
	}

	for _, p := range job.preferred {
		add(p)
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT lower(slot), upper(slot) FROM maintenance.slot_proposals WHERE job_id = $1
		AND status = 'accepted' ORDER BY decided_at`,
		jobID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.slot_proposals")
	}
	defer rows.Close()

	for rows.Next() {
		var slot Slot

		scanErr := rows.Scan(&slot.StartAt, &slot.EndAt)
		if scanErr != nil {
			return nil, apperr.Wrap(scanErr, "scan maintenance.slot_proposals")
		}

		add(slot)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select maintenance.slot_proposals")
	}

	if len(out) == 0 && job.planID != nil && job.occurrence != nil {
		add(
			Slot{
				*job.occurrence,
				job.occurrence.Add(job.requested.EndAt.Sub(job.requested.StartAt)),
			},
		)
	}

	return out, nil
}

// offerWindowErrors are the field errors of an offer for its candidate times (IR636 item 2): there is a time left, the
// access window covers every candidate, and the offer does not outlast the last candidate's start (IR457).
func offerWindowErrors(input *OfferInput, candidates []Slot) map[string]string {
	fieldErrs := map[string]string{}
	if len(candidates) == 0 {
		fieldErrs["candidateSlots"] = apperr.KeyPast

		return fieldErrs
	}

	first, last := candidates[0].StartAt, candidates[0].StartAt
	end := candidates[0].EndAt

	for _, slot := range candidates {
		if slot.StartAt.Before(first) {
			first = slot.StartAt
		}

		if slot.StartAt.After(last) {
			last = slot.StartAt
		}

		if slot.EndAt.After(end) {
			end = slot.EndAt
		}
	}

	// an answer after the last visit began could not be worked (IR457)
	if input.OfferExpiresAt.After(last) && !input.OfferExpiresAt.After(input.AccessValidUntil) {
		fieldErrs["offerExpiresAt"] = "error.afterVisit"
	}

	if input.AccessValidFrom.After(first) || end.After(input.AccessValidUntil) {
		fieldErrs["accessValidFrom"] = "error.accessMustCoverVisit"
	}

	return fieldErrs
}

// ---- jobs.offer ----

// keySlotNotAgreed refuses a visit or an assignment slot that is not an agreed slot of the job (IR113 item 3).
const keySlotNotAgreed = "errors.slot_not_agreed"

// slotNotAgreed is the VALIDATION error of an assignment whose slot is not an agreed one.
func slotNotAgreed() error { return apperr.Fields(map[string]string{"startAt": keySlotNotAgreed}) }

// OfferInput is jobs.offer input.
type OfferInput struct {
	JobID            uuid.UUID `json:"jobId"`
	ContractorOrgID  uuid.UUID `json:"contractorOrgId"`
	OfferExpiresAt   time.Time `json:"offerExpiresAt"`
	AccessValidFrom  time.Time `json:"accessValidFrom"`
	AccessValidUntil time.Time `json:"accessValidUntil"`
	TermsVersion     string    `json:"termsVersion"`
}

// Validate implements ops.Validator (IR48 / IR123 item 1, clock-free parts). The offered times are the job's agreed
// times, which the server reads (IR636), so the rules that need them are applied by offerJob.
func (in *OfferInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ContractorOrgID == uuid.Nil {
		fieldErrs["contractorOrgId"] = apperr.KeyRequired
	}

	if !in.AccessValidFrom.Before(in.AccessValidUntil) {
		fieldErrs["accessValidUntil"] = apperr.KeyRange
	}

	if in.OfferExpiresAt.After(in.AccessValidUntil) {
		fieldErrs["offerExpiresAt"] = "error.afterAccess"
	}

	if n := utf8.RuneCountInString(in.TermsVersion); n < 1 || n > maxCodeLength {
		fieldErrs["termsVersion"] = apperr.KeyLength
	}

	return fieldErrs
}

// offer serves jobs.offer.
//
//	@Summary		jobs.offer (write)
//	@ID				jobs.offer
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR48 requested only; offerExpiresAt<=accessValidUntil and <= the last candidate start (IR457); \
//	@Description	expiry returns requested; IR636 the offer carries the job's agreed times that have not ended \
//	@Description	(candidateSlots: preferred times or accepted proposals or else the plan occurrence); none left is \
//	@Description	VALIDATION candidateSlots=error.past; the access window covers every candidate; the contractor picks \
//	@Description	one when it assigns a technician
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		OfferInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/offers [post]
func (m Delivery) offer(ctx context.Context, c *ops.Call, in *OfferInput) (MaintenanceJob, error) {
	return written(m.offerJob(ctx, c, in))
}

// offerJob is offer answering the job with its detail extras.
func (m Delivery) offerJob(ctx context.Context, call *ops.Call, input *OfferInput) (Job, error) {
	if !call.Now.Before(input.OfferExpiresAt) {
		return Job{}, apperr.Fields(map[string]string{"offerExpiresAt": apperr.KeyPast})
	}

	org, err := m.Directory.OrgState(ctx, call, input.ContractorOrgID)
	if err != nil {
		return Job{}, apperr.Wrap(err, "maintenance.Directory.OrgState")
	}

	if !org.Found || org.Kind != orgKindContractor || org.Status != orgStatusActive {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	var suspended bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1 AND
		status = 'suspended')`, input.ContractorOrgID).
		Scan(&suspended)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.contractors")
	}

	job, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if job.status != statusRequested {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	if suspended {
		return Job{}, apperr.E(apperr.Conflict, "errors.contractor_suspended")
	}

	candidates, err := m.candidates(ctx, call, input.JobID, job)
	if err != nil {
		return Job{}, err
	}

	fieldErrs := offerWindowErrors(input, candidates)
	if len(fieldErrs) > 0 {
		return Job{}, apperr.Fields(fieldErrs)
	}

	encoded, marshalErr := json.Marshal(candidates)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, candidate_slots, offered_at,
			offer_expires_at,
		access_valid_from, access_valid_until) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4,
		$5, $6, $7, $8)`,
		input.JobID,
		input.ContractorOrgID,
		input.TermsVersion,
		encoded,
		call.Now,
		input.OfferExpiresAt,
		input.AccessValidFrom,
		input.AccessValidUntil,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "insert maintenance.offers")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'offered', contractor_org_id = $2, version = version + 1, updated_at =
			platform.app_now() WHERE id = $1`,
		input.JobID,
		input.ContractorOrgID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.Jobs.event(ctx, call, input.JobID, "job.offered", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.Jobs.finish(ctx, call, input.JobID, "jobs.offer", "JobOffered", "")
}

// ---- jobs.accept / jobs.decline ----

// The decisions on an offer, an assignment, a slot proposal and a work report review.
const (
	decisionAccept  = "accept"
	decisionDecline = "decline"
)

// AcceptInput is jobs.accept input: the company takes the offer under the terms version it read.
type AcceptInput struct {
	JobID        uuid.UUID `json:"jobId"`
	OfferID      uuid.UUID `json:"offerId"`
	TermsVersion string    `json:"termsVersion"`
}

// Validate implements ops.Validator.
func (in *AcceptInput) Validate() map[string]string {
	fieldErrs := decisionRefs(in.JobID, in.OfferID)
	if in.TermsVersion == "" {
		fieldErrs["termsVersion"] = apperr.KeyRequired
	} else if utf8.RuneCountInString(in.TermsVersion) > maxCodeLength {
		fieldErrs["termsVersion"] = apperr.KeyLength
	}

	return fieldErrs
}

// DeclineInput is jobs.decline input: the job returns to HQ with the reason (trimmed, 1–1000 characters).
type DeclineInput struct {
	JobID   uuid.UUID `json:"jobId"`
	OfferID uuid.UUID `json:"offerId"`
	Reason  string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *DeclineInput) Validate() map[string]string {
	fe := decisionRefs(in.JobID, in.OfferID)
	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fe)

	return fe
}

func decisionRefs(job, offer uuid.UUID) map[string]string {
	fieldErrs := map[string]string{}
	if job == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if offer == uuid.Nil {
		fieldErrs["offerId"] = apperr.KeyRequired
	}

	return fieldErrs
}

// Receipt is JobDecisionReceipt of service-contracts.ts.
type Receipt struct {
	JobID      uuid.UUID `json:"jobId"`
	JobVersion int       `json:"jobVersion"`
	OfferID    uuid.UUID `json:"offerId"`
	Decision   string    `json:"decision"`
}

// decide records the company's answer to its open offer: terms is the version read for an accept, reason the decline's.
func (m Delivery) decide(
	ctx context.Context,
	call *ops.Call,
	job, offer uuid.UUID,
	decision, readTerms, reason string,
) (Receipt, error) {
	terms, err := m.decidableOffer(ctx, call, job, offer)
	if err != nil {
		return Receipt{}, err
	}

	if decision == decisionAccept {
		err := acceptable(ctx, call, offer, readTerms, terms)
		if err != nil {
			return Receipt{}, err
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET decision = $2, decided_by = $3, decided_at = $4, decline_reason = NULLIF($5, ''),
		version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		offer,
		decision,
		call.Principal.UserID,
		call.Now,
		reason,
	)
	if execErr != nil {
		return Receipt{}, apperr.Wrap(execErr, "update maintenance.offers")
	}

	next, contractor := statusAccepted, "contractor_org_id"
	if decision == decisionDecline {
		next, contractor = statusRequested, "NULL"

		err := withdrawWithOffer(ctx, call, job, offer)
		if err != nil {
			return Receipt{}, err
		}
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE maintenance.jobs SET status = $2, contractor_org_id =
		`+contractor+`, version `+
		`= version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, job, next).
		Scan(&version)
	if scanErr != nil {
		return Receipt{}, apperr.Wrap(scanErr, "update maintenance.jobs")
	}

	// the past tense the history reads (offer.declined)
	done := map[string]string{decisionAccept: "accepted", decisionDecline: "declined"}[decision]

	eventErr := m.Jobs.event(ctx, call, job, "offer."+done, nil)
	if eventErr != nil {
		return Receipt{}, eventErr
	}

	call.Emit(
		outboxEvent(
			"job",
			job,
			"Offer"+strings.ToUpper(done[:1])+done[1:],
			map[string]any{"offerId": offer},
		),
	)
	call.Audit(auditEntry("jobs."+decision, "job", job, call.ExpectedVersion, &version, reason))

	return Receipt{JobID: job, JobVersion: version, OfferID: offer, Decision: decision}, nil
}

// decidableOffer is the company's offer of the job while it can still be decided: undecided (a decided one is reached
// only through its IR01 receipt), unexpired, and on a job that is still offered. It locks the job and returns the
// offer's terms version.
func (m Delivery) decidableOffer(
	ctx context.Context,
	call *ops.Call,
	job, offer uuid.UUID,
) (string, error) {
	var (
		terms   string
		expires time.Time
		decided *string
	)

	err := call.Tx.QueryRow(ctx, `SELECT terms_version, offer_expires_at, decision FROM maintenance.offers WHERE id = $1
		AND
		job_id = $2 AND contractor_org_id = $3 FOR UPDATE`,
		offer, job, call.Principal.OrgID).
		Scan(&terms, &expires, &decided)
	if errors.Is(err, pgx.ErrNoRows) {
		return terms, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return terms, apperr.Wrap(err, "select maintenance.offers")
	}

	if decided != nil {
		return terms, apperr.E(
			apperr.NotFound,
			apperr.KeyNotFound,
		) // declined / decided offers are only reachable through the IR01 receipt
	}

	locked, err := m.lock(ctx, call, job)
	if err != nil {
		return terms, err
	}

	if !call.Now.Before(expires) {
		return terms, apperr.E(apperr.Conflict, "errors.offer_expired")
	}

	if locked.status != statusOffered {
		return terms, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	return terms, nil
}

// acceptable checks an acceptance: no time change of the company's is open on the offer, and the terms read are the
// offer's.
func acceptable(
	ctx context.Context,
	call *ops.Call,
	offer uuid.UUID,
	readTerms, terms string,
) error {
	var open bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND
		status IN ('pending','sent_to_client'))`, offer).
		Scan(&open)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.partner_slot_proposals")
	}

	if open {
		return apperr.E(apperr.Conflict, "errors.partner_proposal_pending")
	}

	if readTerms != terms {
		return apperr.E(apperr.Conflict, "errors.terms_changed")
	}

	return nil
}

// withdrawWithOffer ends what a declined offer leaves open: the contractor's time change (IR444), and a client proposal
// that holds the contractor, which would offer the job again to the company that just declined it (IR445).
func withdrawWithOffer(ctx context.Context, call *ops.Call, job, offer uuid.UUID) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE offer_id = $1 AND status IN ('pending',
			'sent_to_client')`,
		offer,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.partner_slot_proposals")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $3 WHERE job_id = $1 AND status = 'pending'
		AND hold->>'contractorOrgId' = $2`,
		job,
		call.Principal.OrgID.String(),
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.slot_proposals")
	}

	return nil
}

// accept serves jobs.accept.
//
//	@Summary		jobs.accept (write)
//	@ID				jobs.accept
//	@Description	Authorization: contractor:partner.accept:own-valid-offer:first-attempt | \
//	@Description	IR01:same-key-receipt:original-user:current-own-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR86 expired undecided own offer: CONFLICT errors.offer_expired
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent; IR01 own valid \
//	@Description	membership receipt after decline/offer expiry
//	@Description	Design: DD-P02 · Input versions: none; options.expectedVersion=JobOfferSummary.jobVersion
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			offerId				path		string		true	"input field offerId"
//	@Param			request				body		AcceptInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Receipt}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/offers/{offerId}/accept [post]
func (m Delivery) accept(ctx context.Context, c *ops.Call, in *AcceptInput) (Receipt, error) {
	return m.decide(ctx, c, in.JobID, in.OfferID, decisionAccept, in.TermsVersion, "")
}

// decline serves jobs.decline.
//
//	@Summary		jobs.decline (write)
//	@ID				jobs.decline
//	@Description	Authorization: contractor:partner.accept:own-valid-offer:first-attempt | \
//	@Description	IR01:same-key-receipt:original-user:current-own-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR86 expired undecided own offer: CONFLICT errors.offer_expired
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent; IR01 own valid \
//	@Description	membership receipt after decline/offer expiry
//	@Description	Design: DD-P02 · Input versions: none; options.expectedVersion=JobOfferSummary.jobVersion
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			offerId				path		string			true	"input field offerId"
//	@Param			request				body		DeclineInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Receipt}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/offers/{offerId}/decline [post]
func (m Delivery) decline(ctx context.Context, c *ops.Call, in *DeclineInput) (Receipt, error) {
	return m.decide(ctx, c, in.JobID, in.OfferID, decisionDecline, "", in.Reason)
}

// ---- jobs.assign ----

// AssignInput is jobs.assign input.
type AssignInput struct {
	JobID                  uuid.UUID `json:"jobId"`
	TechnicianMembershipID uuid.UUID `json:"technicianMembershipId"`
	StartAt                time.Time `json:"startAt"`
	EndAt                  time.Time `json:"endAt"`
	Reason                 *string   `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *AssignInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.TechnicianMembershipID == uuid.Nil {
		fieldErrs["technicianMembershipId"] = apperr.KeyRequired
	}

	if !in.StartAt.Before(in.EndAt) {
		fieldErrs["endAt"] = apperr.KeyRange
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > maxReasonLength {
			fieldErrs[fieldReason] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// assign serves jobs.assign.
//
//	@Summary		jobs.assign (write)
//	@ID				jobs.assign
//	@Description	Authorization: contractor:partner.assign:own-valid-offer | admin:job.write:internal-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR89 success updates Job.assignmentId/scheduledSlot and version; IR94 technician scope must include \
//	@Description	Job.unitId; IR113 client_request/periodic_plan: [startAt,endAt) must equal an agreed slot (one of \
//	@Description	the offer candidateSlots for contractors, IR636, also to move an assigned job to another candidate \
//	@Description	that is not over) else VALIDATION errors.slot_not_agreed; IR433 the technician's access must last \
//	@Description	until endAt else FORBIDDEN errors.technician_access_ends; creates acknowledgement=pending
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06, DD-P03
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		AssignInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/assign [post]
func (m Delivery) assign(
	ctx context.Context,
	c *ops.Call,
	in *AssignInput,
) (MaintenanceJob, error) {
	return written(m.assignJob(ctx, c, in))
}

// assignJob is assign answering the job with its detail extras.
func (m Delivery) assignJob(ctx context.Context, call *ops.Call, input *AssignInput) (Job, error) {
	locked, err := m.load(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	slot := Slot{input.StartAt, input.EndAt}
	contractor := call.Principal.Role == roleContractor

	offered, err := assignmentScope(ctx, call, input.JobID, locked, contractor)
	if err != nil {
		return Job{}, err
	}

	currentErr := locked.current(call)
	if currentErr != nil {
		return Job{}, currentErr
	}

	checkTechnicianErr := m.checkTechnician(
		ctx,
		call,
		locked.unit,
		input.TechnicianMembershipID,
		contractor,
		nil,
	)
	if checkTechnicianErr != nil {
		return Job{}, checkTechnicianErr
	}

	err = m.slotRules(ctx, call, input, locked, slot, offered, contractor)
	if err != nil {
		return Job{}, err
	}

	checkQualifiedErr := m.checkQualified(
		ctx,
		call,
		locked.unit,
		input.TechnicianMembershipID,
		slot,
	)
	if checkQualifiedErr != nil {
		return Job{}, checkQualifiedErr
	}

	createAssignmentErr := m.createAssignment(
		ctx,
		call,
		input.JobID,
		input.TechnicianMembershipID,
		slot,
		deref(input.Reason),
	)
	if createAssignmentErr != nil {
		return Job{}, createAssignmentErr
	}

	job, err := m.Jobs.scoped(ctx, call, input.JobID)
	if err != nil {
		return job, err
	}

	call.Emit(
		outboxEvent(
			"job",
			input.JobID,
			"JobAssigned",
			map[string]any{"technicianMembershipId": input.TechnicianMembershipID},
		),
	)
	call.Audit(
		auditEntry(
			"jobs.assign",
			"job",
			input.JobID,
			call.ExpectedVersion,
			&job.Version,
			deref(input.Reason),
		),
	)

	return job, nil
}

// assignmentScope checks who may assign the job: HQ its internal jobs; a contractor the job of its accepted offer,
// within the offer's access window. It returns the candidate times of the contractor's offer (IR636).
func assignmentScope(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	locked jobRow,
	contractor bool,
) ([]Slot, error) {
	var offered []Slot

	if contractor {
		var from, until time.Time

		err := call.Tx.QueryRow(ctx, `SELECT candidate_slots, access_valid_from, access_valid_until FROM maintenance.offers
			WHERE job_id = $1 AND contractor_org_id = $2 AND decision = 'accept' ORDER BY decided_at DESC LIMIT
			1`, job, call.Principal.OrgID).
			Scan(&offered, &from, &until)
		if errors.Is(err, pgx.ErrNoRows) || locked.contractor == nil ||
			*locked.contractor != call.Principal.OrgID {
			return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.offers")
		}

		if call.Now.Before(from) || !call.Now.Before(until) {
			return nil, apperr.E(apperr.Forbidden, "errors.access_window")
		}
	} else if locked.contractor != nil {
		return nil, apperr.E(apperr.Forbidden, "errors.internal_job_only")
	}

	return offered, nil
}

// slotRules applies the state and slot rules of an assignment (IR113 / IR49 / IR123 item 4 / IR636): a first assignment
// takes an agreed time, which is not over (IR456), and a contractor picks one of its offer's candidate times; a
// reassignment keeps the scheduled time, and a contractor may move to another candidate that is not over; a job under
// way may only be extended, with a reason (D06).
func (m Delivery) slotRules(
	ctx context.Context,
	call *ops.Call,
	input *AssignInput,
	locked jobRow,
	slot Slot,
	offered []Slot,
	contractor bool,
) error {
	switch {
	case (locked.status == statusRequested && !contractor) || (locked.status == statusAccepted && contractor):
		if !slot.EndAt.After(
			call.Now,
		) { // a time already over cannot be worked (IR456): HQ proposes another
			return apperr.Fields(map[string]string{"endAt": apperr.KeyPast})
		}

		agreed := contractor && slotIn(offered, slot)
		if !contractor {
			var err error

			agreed, err = m.agreed(ctx, call, input.JobID, locked, slot)
			if err != nil {
				return err
			}
		}

		if !agreed {
			return slotNotAgreed()
		}
	case locked.status == statusAssigned:
		keeps := locked.scheduled != nil && sameSlot(*locked.scheduled, slot)
		// a technician who cannot make the time: the contractor takes another candidate that is not over (IR636 item 3)
		moves := locked.scheduled != nil && contractor && slot.EndAt.After(call.Now) &&
			slotIn(offered, slot)

		if !keeps && !moves {
			return slotNotAgreed()
		}
	case locked.status == statusInProgress || locked.status == statusReworkRequested || locked.status == statusOnHold:
		// D06 extension / reassignment
		if locked.scheduled == nil || !slot.StartAt.Equal(locked.scheduled.StartAt) ||
			slot.EndAt.Before(locked.scheduled.EndAt) {
			return apperr.Fields(map[string]string{"endAt": "errors.extension_only"})
		}

		if input.Reason == nil || *input.Reason == "" {
			return apperr.Fields(map[string]string{fieldReason: apperr.KeyRequired})
		}
	default:
		return apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// RegisterDelivery binds jobs.offer / accept / decline / assign.
func RegisterDelivery(r *ops.Registry, m Delivery) {
	ops.Register(r, "jobs.offer", m.offer)
	ops.Register(r, "jobs.accept", m.accept)
	ops.Register(r, "jobs.decline", m.decline)
	ops.Register(r, "jobs.assign", m.assign)
}

// checkTechnician applies IR123 item 4 to a technician candidate: active role=technician, the caller's own organization
// for contractors (or org when given) or an internal technician for HQ, and scopes containing the unit.
func (m Delivery) checkTechnician(
	ctx context.Context,
	call *ops.Call,
	unit, membership uuid.UUID,
	contractor bool,
	org *uuid.UUID,
) error {
	tech, found, err := m.Directory.Technician(ctx, call, membership)
	if err != nil {
		return apperr.Wrap(err, "maintenance.Directory.Technician")
	}

	if !found || !tech.Active {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	own := call.Principal.OrgID
	if org != nil {
		own = *org
	}

	if (contractor && tech.OrgID != own) || (!contractor && tech.Employment != employmentInternal) {
		return apperr.E(apperr.Forbidden, "errors.technician_not_allowed")
	}

	if contractor && len(tech.Scopes) == 0 {
		// a technician the contractor added has no unit scope (IR461): the contractor assigns them only on its own
		// accepted offer, which already bounds the unit, and they reach it only in the assignment's window (IR49)
		return nil
	}

	uorg, prop, _, err := m.Units.UnitService(ctx, call, unit)
	if err != nil {
		return apperr.Wrap(err, "maintenance.ServiceUnits.UnitService")
	}

	if !InScope(
		&ops.Principal{
			Scopes:            tech.Scopes,
			Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
			TenantID:          uuid.UUID{},
			MembershipID:      uuid.UUID{},
			UserID:            uuid.UUID{},
			OrgID:             uuid.UUID{},
			ScopeVersion:      0,
			Employment:        "",
			Timezone:          "",
			SessionsRevokedAt: nil,
		},
		unit,
		prop,
		uorg,
	) {
		return apperr.E(apperr.Forbidden, "errors.technician_out_of_scope")
	}

	return nil
}

// checkQualified applies IR123 item 3 for the unit's service scope over the slot, and requires the technician's access
// to last until the slot ends: a technician whose access ends sooner could not open the job then (IR433).
func (m Delivery) checkQualified(
	ctx context.Context,
	call *ops.Call,
	unit, membership uuid.UUID,
	slot Slot,
) error {
	tech, found, err := m.Directory.Technician(ctx, call, membership)
	if err != nil {
		return apperr.Wrap(err, "maintenance.Directory.Technician")
	}

	if found && tech.ValidUntil != nil && tech.ValidUntil.Before(slot.EndAt) {
		return apperr.E(apperr.Forbidden, "errors.technician_access_ends")
	}

	_, _, scope, err := m.Units.UnitService(ctx, call, unit)
	if err != nil {
		return apperr.Wrap(err, "maintenance.ServiceUnits.UnitService")
	}

	qualified, err := m.Directory.Qualified(
		ctx,
		call,
		membership,
		requiredCodes(scope),
		slot.StartAt,
		slot.EndAt,
	)
	if err != nil {
		return apperr.Wrap(err, "maintenance.Directory.Qualified")
	}

	if !qualified {
		return apperr.E(apperr.Forbidden, "errors.qualification_missing")
	}

	return nil
}

// createAssignment revokes the active Assignment, creates the new one (acknowledgement pending) and updates the job
// (IR89 / IR113 item 8); callers have checked access, slot and technician.
func (m Delivery) createAssignment(
	ctx context.Context,
	call *ops.Call,
	job, tech uuid.UUID,
	slot Slot,
	reason string,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.assignments SET status = 'revoked', reason = COALESCE(NULLIF($2, ''), 'reassigned'), version =
			version + 1, updated_at = platform.app_now()
		WHERE job_id = $1 AND status = 'active'`,
		job,
		reason,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.assignments")
	}

	var aid uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.assignments (tenant_id, job_id, technician_membership_id,
		valid_from,
		valid_until, scheduled, status, reason, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, tstzrange($3, $4), 'active', NULLIF($5, ''), $6, $6)
		RETURNING id`,
		job, tech, slot.StartAt, slot.EndAt, reason, call.Now).
		Scan(&aid)
	if err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return apperr.E(apperr.Conflict, "errors.assignment_overlap")
		}

		return apperr.Wrap(err, "insert maintenance.assignments")
	}
	// a window of this technician that had ended was frozen into their history; it is frozen again when the new one ends
	// (IR459)
	_, err = call.Tx.Exec(
		ctx,
		`DELETE FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'membership' AND owner_id = $2`,
		job,
		tech,
	)
	if err != nil {
		return apperr.Wrap(err, "delete maintenance.job_history_snapshots")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = CASE WHEN status IN ('requested','accepted') THEN 'assigned' ELSE status END,
		assignment_id = $2, scheduled_slot = tstzrange($3, $4), version = version + 1, updated_at = platform.app_now() WHERE
		id = $1`,
		job,
		aid,
		slot.StartAt,
		slot.EndAt,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.jobs")
	}

	err = m.Jobs.event(ctx, call, job, "job.assigned", nil)
	if err != nil {
		return err
	}

	return nil
}
