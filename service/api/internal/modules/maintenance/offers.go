package maintenance

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Directory is what offers and assignments need from Identity & access.
type Directory interface {
	OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (kind, status string, found bool, err error)
	Technician(ctx context.Context, c *ops.Call, membership uuid.UUID) (identity.Technician, bool, error)
	Qualified(ctx context.Context, c *ops.Call, membership uuid.UUID, codes []string, start, end time.Time) (bool, error)
}

// ServiceUnits returns organization, property and service scope of a unit (Assets).
type ServiceUnits interface {
	UnitService(ctx context.Context, c *ops.Call, unit uuid.UUID) (org, property uuid.UUID, scope []string, err error)
}

// QualificationRequirements maps Unit.serviceScope entries to QualificationCodes (fixture qualificationRequirements).
var QualificationRequirements = map[string]string{"indoor": "demo_indoor", "outdoor": "demo_outdoor", "electrical": "demo_electrical"}

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
}

// lock loads a job for update without the role scope (callers apply their own rules) and checks the version.
func (m Delivery) lock(ctx context.Context, c *ops.Call, id uuid.UUID) (jobRow, error) {
	var r jobRow
	var v int
	var pref []byte
	var ss, se *time.Time
	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id, contractor_org_id, plan_id, occurrence_at, lower(requested_slot), upper(requested_slot), preferred_slots,
		lower(scheduled_slot), upper(scheduled_slot), assignment_id, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, id).
		Scan(&r.status, &r.unit, &r.contractor, &r.planID, &r.occurrence, &r.requested.StartAt, &r.requested.EndAt, &pref, &ss, &se, &r.assignment, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return r, err
	}
	if ss != nil && se != nil {
		r.scheduled = &Slot{*ss, *se}
	}
	if err := unmarshalSlots(pref, &r.preferred); err != nil {
		return r, err
	}
	if v != *c.ExpectedVersion {
		return r, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return r, nil
}

func sameSlot(a, b Slot) bool { return a.StartAt.Equal(b.StartAt) && a.EndAt.Equal(b.EndAt) }

// agreed reports whether a slot is an agreed slot of the job (IR113 item 3).
func (m Delivery) agreed(ctx context.Context, c *ops.Call, id uuid.UUID, r jobRow, s Slot) (bool, error) {
	for _, p := range r.preferred {
		if sameSlot(p, s) {
			return true, nil
		}
	}
	var n int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.slot_proposals WHERE job_id = $1 AND status = 'accepted' AND slot = tstzrange($2, $3)`,
		id, s.StartAt, s.EndAt).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	if r.planID != nil && r.occurrence != nil {
		dur := r.requested.EndAt.Sub(r.requested.StartAt)
		return sameSlot(Slot{*r.occurrence, r.occurrence.Add(dur)}, s), nil
	}
	return false, nil
}

// ---- jobs.offer ----

// OfferInput is jobs.offer input.
type OfferInput struct {
	JobID            uuid.UUID `json:"jobId"`
	ContractorOrgID  uuid.UUID `json:"contractorOrgId"`
	VisitSlot        Slot      `json:"visitSlot"`
	OfferExpiresAt   time.Time `json:"offerExpiresAt"`
	AccessValidFrom  time.Time `json:"accessValidFrom"`
	AccessValidUntil time.Time `json:"accessValidUntil"`
	TermsVersion     string    `json:"termsVersion"`
}

// Validate implements ops.Validator (IR48 / IR123 item 1, clock-free parts).
func (in *OfferInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ContractorOrgID == uuid.Nil {
		fe["contractorOrgId"] = "error.required"
	}
	if !in.VisitSlot.StartAt.Before(in.VisitSlot.EndAt) {
		fe["visitSlot"] = "error.range"
	}
	if !in.AccessValidFrom.Before(in.AccessValidUntil) {
		fe["accessValidUntil"] = "error.range"
	}
	if in.OfferExpiresAt.After(in.AccessValidUntil) {
		fe["offerExpiresAt"] = "error.afterAccess"
	}
	if in.AccessValidFrom.After(in.VisitSlot.StartAt) || in.VisitSlot.EndAt.After(in.AccessValidUntil) {
		fe["accessValidFrom"] = "error.accessMustCoverVisit"
	}
	if n := utf8.RuneCountInString(in.TermsVersion); n < 1 || n > 64 {
		fe["termsVersion"] = "error.length"
	}
	return fe
}

func (m Delivery) offer(ctx context.Context, c *ops.Call, in *OfferInput) (Job, error) {
	if !c.Now.Before(in.OfferExpiresAt) {
		return Job{}, apperr.Fields(map[string]string{"offerExpiresAt": "error.past"})
	}
	kind, status, found, err := m.Directory.OrgState(ctx, c, in.ContractorOrgID)
	if err != nil {
		return Job{}, err
	}
	if !found || kind != "contractor" || status != "active" {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	var suspended bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.contractors WHERE organization_id = $1 AND status = 'suspended')`, in.ContractorOrgID).Scan(&suspended); err != nil {
		return Job{}, err
	}
	r, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if r.status != "requested" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if suspended {
		return Job{}, apperr.E(apperr.Conflict, "errors.contractor_suspended")
	}
	ok, err := m.agreed(ctx, c, in.JobID, r, in.VisitSlot)
	if err != nil {
		return Job{}, err
	}
	if !ok {
		return Job{}, apperr.Fields(map[string]string{"visitSlot": "errors.slot_not_agreed"})
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at, offer_expires_at,
		access_valid_from, access_valid_until) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, tstzrange($4, $5), $6, $7, $8, $9)`,
		in.JobID, in.ContractorOrgID, in.TermsVersion, in.VisitSlot.StartAt, in.VisitSlot.EndAt, c.Now, in.OfferExpiresAt, in.AccessValidFrom, in.AccessValidUntil); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'offered', contractor_org_id = $2, version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		in.JobID, in.ContractorOrgID); err != nil {
		return Job{}, err
	}
	if err := m.Jobs.event(ctx, c, in.JobID, "job.offered", nil); err != nil {
		return Job{}, err
	}
	return m.Jobs.finish(ctx, c, in.JobID, "jobs.offer", "JobOffered", "")
}

// ---- jobs.accept / jobs.decline ----

// DecisionInput is the input of jobs.accept (termsVersion) and jobs.decline (reason).
type DecisionInput struct {
	JobID        uuid.UUID `json:"jobId"`
	OfferID      uuid.UUID `json:"offerId"`
	TermsVersion *string   `json:"termsVersion,omitempty"`
	Reason       *string   `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *DecisionInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.OfferID == uuid.Nil {
		fe["offerId"] = "error.required"
	}
	if (in.TermsVersion == nil) == (in.Reason == nil) {
		fe["termsVersion"] = "error.required"
	}
	if in.Reason != nil {
		trimmedReason(in.Reason, 1000, "reason", fe)
	}
	return fe
}

// Receipt is JobDecisionReceipt of service-contracts.ts.
type Receipt struct {
	JobID      uuid.UUID `json:"jobId"`
	JobVersion int       `json:"jobVersion"`
	OfferID    uuid.UUID `json:"offerId"`
	Decision   string    `json:"decision"`
}

func (m Delivery) decide(ctx context.Context, c *ops.Call, in *DecisionInput, decision string) (Receipt, error) {
	var terms string
	var expires time.Time
	var decided *string
	err := c.Tx.QueryRow(ctx, `SELECT terms_version, offer_expires_at, decision FROM maintenance.offers WHERE id = $1 AND job_id = $2 AND contractor_org_id = $3 FOR UPDATE`,
		in.OfferID, in.JobID, c.Principal.OrgID).Scan(&terms, &expires, &decided)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Receipt{}, err
	}
	if decided != nil {
		return Receipt{}, apperr.E(apperr.NotFound, "error.notFound") // declined / decided offers are only reachable through the IR01 receipt
	}
	r, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Receipt{}, err
	}
	if !c.Now.Before(expires) {
		return Receipt{}, apperr.E(apperr.Conflict, "errors.offer_expired")
	}
	if r.status != "offered" {
		return Receipt{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if decision == "accept" {
		var open bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.partner_slot_proposals WHERE offer_id = $1 AND status IN ('pending','sent_to_client'))`, in.OfferID).Scan(&open); err != nil {
			return Receipt{}, err
		}
		if open {
			return Receipt{}, apperr.E(apperr.Conflict, "errors.partner_proposal_pending")
		}
	}
	if decision == "accept" && *in.TermsVersion != terms {
		return Receipt{}, apperr.E(apperr.Conflict, "errors.terms_changed")
	}
	reason := ""
	if in.Reason != nil {
		reason = *in.Reason
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.offers SET decision = $2, decided_by = $3, decided_at = $4, decline_reason = NULLIF($5, ''),
		version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.OfferID, decision, c.Principal.UserID, c.Now, reason); err != nil {
		return Receipt{}, err
	}
	next, contractor := "accepted", "contractor_org_id"
	if decision == "decline" {
		next, contractor = "requested", "NULL"
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `UPDATE maintenance.jobs SET status = $2, contractor_org_id = `+contractor+`, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, in.JobID, next).Scan(&v); err != nil {
		return Receipt{}, err
	}
	if err := m.Jobs.event(ctx, c, in.JobID, "offer."+decision+"ed", nil); err != nil {
		return Receipt{}, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: in.JobID, Type: "Offer" + strings.ToUpper(decision[:1]) + decision[1:] + "ed", Payload: map[string]any{"offerId": in.OfferID}})
	c.Audit(ops.AuditEntry{Action: "jobs." + decision, TargetKind: "job", TargetID: in.JobID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: reason})
	return Receipt{JobID: in.JobID, JobVersion: v, OfferID: in.OfferID, Decision: decision}, nil
}

func (m Delivery) accept(ctx context.Context, c *ops.Call, in *DecisionInput) (Receipt, error) {
	if in.TermsVersion == nil {
		return Receipt{}, apperr.Fields(map[string]string{"termsVersion": "error.required"})
	}
	return m.decide(ctx, c, in, "accept")
}

func (m Delivery) decline(ctx context.Context, c *ops.Call, in *DecisionInput) (Receipt, error) {
	if in.Reason == nil {
		return Receipt{}, apperr.Fields(map[string]string{"reason": "error.required"})
	}
	return m.decide(ctx, c, in, "decline")
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.TechnicianMembershipID == uuid.Nil {
		fe["technicianMembershipId"] = "error.required"
	}
	if !in.StartAt.Before(in.EndAt) {
		fe["endAt"] = "error.range"
	}
	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > 1000 {
			fe["reason"] = "error.length"
		}
	}
	return fe
}

func (m Delivery) assign(ctx context.Context, c *ops.Call, in *AssignInput) (Job, error) {
	r, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	slot := Slot{in.StartAt, in.EndAt}
	contractor := c.Principal.Role == "contractor"
	// scope: HQ internal jobs; contractor its accepted Offer within the access window
	var visit Slot
	if contractor {
		var from, until time.Time
		err := c.Tx.QueryRow(ctx, `SELECT lower(visit_slot), upper(visit_slot), access_valid_from, access_valid_until FROM maintenance.offers
			WHERE job_id = $1 AND contractor_org_id = $2 AND decision = 'accept' ORDER BY decided_at DESC LIMIT 1`, in.JobID, c.Principal.OrgID).
			Scan(&visit.StartAt, &visit.EndAt, &from, &until)
		if errors.Is(err, pgx.ErrNoRows) || r.contractor == nil || *r.contractor != c.Principal.OrgID {
			return Job{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if err != nil {
			return Job{}, err
		}
		if c.Now.Before(from) || !c.Now.Before(until) {
			return Job{}, apperr.E(apperr.Forbidden, "errors.access_window")
		}
	} else if r.contractor != nil {
		return Job{}, apperr.E(apperr.Forbidden, "errors.internal_job_only")
	}
	if err := m.checkTechnician(ctx, c, r.unit, in.TechnicianMembershipID, contractor, nil); err != nil {
		return Job{}, err
	}
	// state and slot rules (IR113 / IR49 / IR123 item 4)
	switch {
	case (r.status == "requested" && !contractor) || (r.status == "accepted" && contractor):
		ok := contractor && sameSlot(visit, slot)
		if !contractor {
			if ok, err = m.agreed(ctx, c, in.JobID, r, slot); err != nil {
				return Job{}, err
			}
		}
		if !ok {
			return Job{}, apperr.Fields(map[string]string{"startAt": "errors.slot_not_agreed"})
		}
	case r.status == "assigned":
		if r.scheduled == nil || !sameSlot(*r.scheduled, slot) {
			return Job{}, apperr.Fields(map[string]string{"startAt": "errors.slot_not_agreed"})
		}
	case r.status == "in_progress" || r.status == "rework_requested" || r.status == "on_hold": // D06 extension / reassignment
		if r.scheduled == nil || !slot.StartAt.Equal(r.scheduled.StartAt) || slot.EndAt.Before(r.scheduled.EndAt) {
			return Job{}, apperr.Fields(map[string]string{"endAt": "errors.extension_only"})
		}
		if in.Reason == nil || *in.Reason == "" {
			return Job{}, apperr.Fields(map[string]string{"reason": "error.required"})
		}
	default:
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if err := m.checkQualified(ctx, c, r.unit, in.TechnicianMembershipID, slot); err != nil {
		return Job{}, err
	}
	if err := m.createAssignment(ctx, c, in.JobID, in.TechnicianMembershipID, slot, deref(in.Reason)); err != nil {
		return Job{}, err
	}
	j, err := m.Jobs.scoped(ctx, c, in.JobID)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: in.JobID, Type: "JobAssigned", Payload: map[string]any{"technicianMembershipId": in.TechnicianMembershipID}})
	c.Audit(ops.AuditEntry{Action: "jobs.assign", TargetKind: "job", TargetID: in.JobID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &j.Version, Reason: deref(in.Reason)})
	return j, nil
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
func (m Delivery) checkTechnician(ctx context.Context, c *ops.Call, unit, membership uuid.UUID, contractor bool, org *uuid.UUID) error {
	tech, found, err := m.Directory.Technician(ctx, c, membership)
	if err != nil {
		return err
	}
	if !found || !tech.Active {
		return apperr.E(apperr.NotFound, "error.notFound")
	}
	own := c.Principal.OrgID
	if org != nil {
		own = *org
	}
	if (contractor && tech.OrgID != own) || (!contractor && tech.Employment != "internal") {
		return apperr.E(apperr.Forbidden, "errors.technician_not_allowed")
	}
	uorg, prop, _, err := m.Units.UnitService(ctx, c, unit)
	if err != nil {
		return err
	}
	if !InScope(&ops.Principal{Scopes: tech.Scopes}, unit, prop, uorg) {
		return apperr.E(apperr.Forbidden, "errors.technician_out_of_scope")
	}
	return nil
}

// checkQualified applies IR123 item 3 for the unit's service scope over the slot.
func (m Delivery) checkQualified(ctx context.Context, c *ops.Call, unit, membership uuid.UUID, slot Slot) error {
	_, _, scope, err := m.Units.UnitService(ctx, c, unit)
	if err != nil {
		return err
	}
	ok, err := m.Directory.Qualified(ctx, c, membership, requiredCodes(scope), slot.StartAt, slot.EndAt)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.E(apperr.Forbidden, "errors.qualification_missing")
	}
	return nil
}

// createAssignment revokes the active Assignment, creates the new one (acknowledgement pending) and updates the job
// (IR89 / IR113 item 8); callers have checked access, slot and technician.
func (m Delivery) createAssignment(ctx context.Context, c *ops.Call, job, tech uuid.UUID, slot Slot, reason string) error {
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.assignments SET status = 'revoked', reason = COALESCE(NULLIF($2, ''), 'reassigned'), version = version + 1, updated_at = platform.app_now()
		WHERE job_id = $1 AND status = 'active'`, job, reason); err != nil {
		return err
	}
	var aid uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.assignments (tenant_id, job_id, technician_membership_id, valid_from, valid_until, scheduled, status, reason, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, tstzrange($3, $4), 'active', NULLIF($5, ''), $6, $6) RETURNING id`,
		job, tech, slot.StartAt, slot.EndAt, reason, c.Now).Scan(&aid); err != nil {
		if apperr.From(err).Code == apperr.Conflict {
			return apperr.E(apperr.Conflict, "errors.assignment_overlap")
		}
		return err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = CASE WHEN status IN ('requested','accepted') THEN 'assigned' ELSE status END,
		assignment_id = $2, scheduled_slot = tstzrange($3, $4), version = version + 1, updated_at = platform.app_now() WHERE id = $1`, job, aid, slot.StartAt, slot.EndAt); err != nil {
		return err
	}
	if err := m.Jobs.event(ctx, c, job, "job.assigned", nil); err != nil {
		return err
	}
	return nil
}
