package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// JobUnits is what the job operations need from Assets.
type JobUnits interface {
	UnitState(ctx context.Context, c *ops.Call, unit uuid.UUID) (org uuid.UUID, archived bool, found bool, err error)
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error)
	UnitsOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error)
}

// UnitSeverity returns the IR23 job severity per unit (highest open/acknowledged alert, normal without one).
type UnitSeverity interface {
	UnitSeverities(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]string, error)
}

// Jobs is the job operation set.
type Jobs struct {
	Units    JobUnits
	Severity UnitSeverity
	// HQOrg returns the operator organization of the tenant (internal delivery, IR122 item 6).
	HQOrg func(c *ops.Call) uuid.UUID
	Sites SiteAddresses
}

// Slot is Slot of service-contracts.ts.
type Slot struct {
	StartAt time.Time `json:"startAt"`
	EndAt   time.Time `json:"endAt"`
}

// SlotProposal is SlotProposal of service-contracts.ts.
type SlotProposal struct {
	ID             uuid.UUID       `json:"id"`
	Source         string          `json:"source"`
	Slot           Slot            `json:"slot"`
	Hold           json.RawMessage `json:"hold" swaggertype:"object"`
	Message        string          `json:"message"`
	ReplyBy        time.Time       `json:"replyBy"`
	Status         string          `json:"status"`
	DecidedAt      *time.Time      `json:"decidedAt"`
	DeclineReason  *string         `json:"declineReason"`
	DeclineComment *string         `json:"declineComment"`
}

// Assignment is Assignment of service-contracts.ts.
type Assignment struct {
	ID                     uuid.UUID  `json:"id"`
	TenantID               uuid.UUID  `json:"tenantId"`
	Version                int        `json:"version"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
	JobID                  uuid.UUID  `json:"jobId"`
	TechnicianMembershipID uuid.UUID  `json:"technicianMembershipId"`
	ValidFrom              time.Time  `json:"validFrom"`
	ValidUntil             time.Time  `json:"validUntil"`
	ScheduledStart         time.Time  `json:"scheduledStart"`
	ScheduledEnd           time.Time  `json:"scheduledEnd"`
	Status                 string     `json:"status"`
	Reason                 *string    `json:"reason"`
	Acknowledgement        string     `json:"acknowledgement"`
	AcknowledgedAt         *time.Time `json:"acknowledgedAt"`
	CantMakeReason         *string    `json:"cantMakeReason"`
	AlternativeSlot        *Slot      `json:"alternativeSlot"`
}

// Job is MaintenanceJob of service-contracts.ts plus the JobDetail / JobSummary extras.
type Job struct {
	ID                  uuid.UUID       `json:"id"`
	TenantID            uuid.UUID       `json:"tenantId"`
	Version             int             `json:"version"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
	PlanID              *uuid.UUID      `json:"planId"`
	OccurrenceAt        *time.Time      `json:"occurrenceAt"`
	UnitID              uuid.UUID       `json:"unitId"`
	AlertIDs            []uuid.UUID     `json:"alertIds"`
	Type                string          `json:"type"`
	Status              string          `json:"status"`
	Symptom             string          `json:"symptom"`
	ContactWindow       *string         `json:"contactWindow"`
	RequestedSlot       Slot            `json:"requestedSlot"`
	Origin              string          `json:"origin"`
	PreferredSlots      []Slot          `json:"preferredSlots"`
	PreferenceRound     int             `json:"preferenceRound"`
	SlotProposal        *SlotProposal   `json:"slotProposal"`
	PartnerSlotProposal json.RawMessage `json:"partnerSlotProposal" swaggertype:"object"`
	ScheduledSlot       *Slot           `json:"scheduledSlot"`
	DueAt               time.Time       `json:"dueAt"`
	StartedAt           *time.Time      `json:"startedAt"`
	CompletedAt         *time.Time      `json:"completedAt"`
	ContractorOrgID     *uuid.UUID      `json:"contractorOrgId"`
	AssignmentID        *uuid.UUID      `json:"assignmentId"`
	DraftReportRef      any             `json:"draftReportRef"`
	ReportRefs          []any           `json:"reportRefs"`
	Costs               json.RawMessage `json:"costs" swaggertype:"object"`
	FollowUpOfJobID     *uuid.UUID      `json:"followUpOfJobId"`
	FollowUpClass       *string         `json:"followUpClass"`
	TimeOnSite          json.RawMessage `json:"timeOnSite" swaggertype:"object"`
	Rating              json.RawMessage `json:"rating" swaggertype:"object"`
	CustomerConfirmedAt *time.Time      `json:"customerConfirmedAt"`
	WarrantyClaims      json.RawMessage `json:"warrantyClaims" swaggertype:"object"`
	// detail
	Projection  string      `json:"projection"`
	Assignment  *Assignment `json:"assignment"`
	Offer       any         `json:"offer"`
	EventCursor int         `json:"eventCursor"`
	// summary-only
	customerOrg uuid.UUID
}

const jobCols = `j.id, j.tenant_id, j.version, j.created_at, j.updated_at, j.plan_id, j.occurrence_at, j.unit_id, j.type, j.status, j.symptom, j.contact_window,
	lower(j.requested_slot), upper(j.requested_slot), j.origin, j.preferred_slots, j.preference_round, lower(j.scheduled_slot), upper(j.scheduled_slot), j.due_at,
	j.started_at, j.completed_at, j.contractor_org_id, j.assignment_id, j.costs, j.follow_up_of_job_id, j.follow_up_class, j.time_on_site, j.rating,
	j.customer_confirmed_at, j.warranty_claims, j.customer_org_id,
	COALESCE((SELECT array_agg(ja.alert_id ORDER BY ja.alert_id) FROM maintenance.job_alerts ja WHERE ja.job_id = j.id), '{}')`

func scanJob(r pgx.Row) (Job, error) {
	var j Job
	var preferred []byte
	var schedStart, schedEnd *time.Time
	err := r.Scan(&j.ID, &j.TenantID, &j.Version, &j.CreatedAt, &j.UpdatedAt, &j.PlanID, &j.OccurrenceAt, &j.UnitID, &j.Type, &j.Status, &j.Symptom, &j.ContactWindow,
		&j.RequestedSlot.StartAt, &j.RequestedSlot.EndAt, &j.Origin, &preferred, &j.PreferenceRound, &schedStart, &schedEnd, &j.DueAt,
		&j.StartedAt, &j.CompletedAt, &j.ContractorOrgID, &j.AssignmentID, &j.Costs, &j.FollowUpOfJobID, &j.FollowUpClass, &j.TimeOnSite, &j.Rating,
		&j.CustomerConfirmedAt, &j.WarrantyClaims, &j.customerOrg, &j.AlertIDs)
	if err != nil {
		return j, err
	}
	if schedStart != nil && schedEnd != nil {
		j.ScheduledSlot = &Slot{*schedStart, *schedEnd}
	}
	if err := unmarshalSlots(preferred, &j.PreferredSlots); err != nil {
		return j, err
	}
	if j.TimeOnSite == nil {
		j.TimeOnSite = json.RawMessage("null")
	}
	if j.Rating == nil {
		j.Rating = json.RawMessage("null")
	}
	j.PartnerSlotProposal = json.RawMessage("null")
	j.ReportRefs = []any{}
	return j, nil
}

// ---- scope ----

// jobScope applies D01 to job rows for the roles implemented here (client: own organization; admin: tenant).
// Contractor offer projections and technician assignment views are served by their own paths (IR23).
func jobScope(c *ops.Call, args *[]any) string {
	p := c.Principal
	add := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	switch p.Role {
	case "admin":
		return "TRUE"
	case "client":
		return "j.customer_org_id = " + add(p.OrgID)
	case "technician": // IR49 viewing window; external technicians also inside the accepted Offer's access window (IR124)
		now := add(c.Now)
		q := "EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active' AND a.technician_membership_id = " + add(p.MembershipID) +
			" AND a.created_at <= " + now + " AND " + now + " < upper(a.scheduled))"
		if p.Employment != "internal" {
			q += " AND EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision = 'accept' AND o.access_valid_from <= " + now + " AND " + now + " < o.access_valid_until)"
		}
		return q
	case "contractor": // accepted Offer inside its access window (IR23 summary projection)
		now := add(c.Now)
		return "EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = j.id AND o.contractor_org_id = " + add(p.OrgID) +
			" AND o.decision = 'accept' AND o.access_valid_from <= " + now + " AND " + now + " < o.access_valid_until)"
	}
	return "FALSE"
}

// ---- jobs.create ----

// CreateInput is jobs.create input.
type CreateInput struct {
	UnitID           uuid.UUID  `json:"unitId"`
	Type             string     `json:"type"`
	Symptom          string     `json:"symptom"`
	RequestedStart   time.Time  `json:"requestedStart"`
	RequestedEnd     time.Time  `json:"requestedEnd"`
	AlternativeSlots []Slot     `json:"alternativeSlots"`
	ContactWindow    *string    `json:"contactWindow,omitempty"`
	DueAt            *time.Time `json:"dueAt,omitempty"`
}

var jobTypes = map[string]bool{"periodic": true, "reactive": true, "preventive": true}

var contactDigits = regexp.MustCompile(`\d{7,}`)

// ContactWindowOK applies IR64: no '@' and no run of seven digits once spaces, hyphens, parentheses and '+' are removed.
func ContactWindowOK(s string) bool {
	if strings.Contains(s, "@") {
		return false
	}
	stripped := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '+' || r == '\t' {
			return -1
		}
		return r
	}, s)
	return !contactDigits.MatchString(stripped)
}

// Validate implements ops.Validator (shape only; slot timing needs the clock).
func (in *CreateInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	if !jobTypes[in.Type] {
		fe["type"] = "error.invalid"
	}
	in.Symptom = strings.TrimSpace(in.Symptom)
	if n := utf8.RuneCountInString(in.Symptom); n < 10 || n > 2000 {
		fe["symptom"] = "error.length"
	}
	if in.AlternativeSlots == nil {
		fe["alternativeSlots"] = "error.required"
	}
	if in.ContactWindow != nil {
		*in.ContactWindow = strings.TrimSpace(*in.ContactWindow)
		if utf8.RuneCountInString(*in.ContactWindow) > 200 {
			fe["contactWindow"] = "error.length"
		} else if !ContactWindowOK(*in.ContactWindow) {
			fe["contactWindow"] = "errors.contact_details_forbidden"
		}
	}
	return fe
}

var kualaLumpur = func() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", 8*3600)
	}
	return l
}()

// PreferredSlotsOK applies IR113 item 2: distinct, each 1–4 hours, starting on a later Kuala Lumpur calendar day than now.
func PreferredSlotsOK(now time.Time, slots []Slot) bool {
	today := now.In(kualaLumpur)
	y, m, d := today.Date()
	tomorrow := time.Date(y, m, d+1, 0, 0, 0, 0, kualaLumpur)
	seen := map[[2]int64]bool{}
	for _, s := range slots {
		dur := s.EndAt.Sub(s.StartAt)
		key := [2]int64{s.StartAt.UnixNano(), s.EndAt.UnixNano()}
		if dur < time.Hour || dur > 4*time.Hour || s.StartAt.Before(tomorrow) || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// @Summary		jobs.create (write)
// @ID				jobs.create
// @Description	Authorization: client:self | admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; IR38 dueAt only for job.write; defaults to requestedEnd; IR113 client: requested slot + exactly 2 alternativeSlots, 3 distinct future slots ≥1 day ahead; HQ on behalf: 0–2
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06, DD-C09
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		CreateInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Job}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs [post]
func (m Jobs) create(ctx context.Context, c *ops.Call, in *CreateInput) (Job, error) {
	if c.Principal.Role == "client" && in.DueAt != nil {
		return Job{}, apperr.Fields(map[string]string{"dueAt": "error.notAllowed"})
	}
	org, archived, found, err := m.Units.UnitState(ctx, c, in.UnitID)
	if err != nil {
		return Job{}, err
	}
	if !found || (c.Principal.Role == "client" && org != c.Principal.OrgID) {
		return Job{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if archived {
		return Job{}, apperr.Fields(map[string]string{"unitId": "error.unitArchived"})
	}
	slots := append([]Slot{{in.RequestedStart, in.RequestedEnd}}, in.AlternativeSlots...)
	alts := len(in.AlternativeSlots)
	if (c.Principal.Role == "client" && alts != 2) || alts > 2 {
		return Job{}, apperr.Fields(map[string]string{"alternativeSlots": "error.count"})
	}
	if !PreferredSlotsOK(c.Now, slots) {
		return Job{}, apperr.Fields(map[string]string{"alternativeSlots": "error.slotRules"})
	}
	due := in.RequestedEnd
	if in.DueAt != nil {
		if in.DueAt.Before(in.RequestedEnd) {
			return Job{}, apperr.Fields(map[string]string{"dueAt": "error.beforeRequestedEnd"})
		}
		due = *in.DueAt
	}
	var cw *string
	if in.ContactWindow != nil && *in.ContactWindow != "" {
		cw = in.ContactWindow
	}
	pref, _ := json.Marshal(slots)
	var id uuid.UUID
	err = c.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin, symptom, contact_window, requested_slot,
		preferred_slots, preference_round, due_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'requested', 'client_request', $4, $5,
		tstzrange($6, $7), $8, 1, $9) RETURNING id`, in.UnitID, org, in.Type, in.Symptom, cw, in.RequestedStart, in.RequestedEnd, pref, due).Scan(&id)
	if err != nil {
		return Job{}, err
	}
	if err := m.event(ctx, c, id, "job.created", nil); err != nil {
		return Job{}, err
	}
	j, err := m.detail(ctx, c, id)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: id, Type: "JobCreated", Payload: map[string]any{"unitId": in.UnitID, "customerOrgId": org}})
	c.Audit(ops.AuditEntry{Action: "jobs.create", TargetKind: "job", TargetID: id.String(), NextVersion: &j.Version})
	return j, nil
}

func (m Jobs) event(ctx context.Context, c *ops.Call, job uuid.UUID, action string, note *uuid.UUID) error {
	// UUIDv7 IDs keep events of the same instant in write order (sort occurredAt asc, id asc)
	_, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, note_id, occurred_at)
		VALUES ($6, current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`, job, c.Principal.UserID, action, note, c.Now, uuid.Must(uuid.NewV7()))
	return err
}

// ---- jobs.get ----

// JobIDInput is the {jobId} input.
type JobIDInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// Validate implements ops.Validator.
func (in *JobIDInput) Validate() map[string]string {
	if in.JobID == uuid.Nil {
		return map[string]string{"jobId": "error.required"}
	}
	return nil
}

// detail loads the JobDetail projection in the caller's scope; contactWindow follows IR64.
func (m Jobs) detail(ctx context.Context, c *ops.Call, id uuid.UUID) (Job, error) {
	args := []any{id}
	return m.load(ctx, c, id, jobScope(c, &args), args)
}

// scoped loads JobDetail after the caller's operation already checked access (for example a contractor's assign).
func (m Jobs) scoped(ctx context.Context, c *ops.Call, id uuid.UUID) (Job, error) {
	return m.load(ctx, c, id, "TRUE", []any{id})
}

func (m Jobs) load(ctx context.Context, c *ops.Call, id uuid.UUID, scope string, args []any) (Job, error) {
	j, err := scanJob(c.Tx.QueryRow(ctx, "SELECT "+jobCols+" FROM maintenance.jobs j WHERE j.id = $1 AND "+scope, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return j, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return j, err
	}
	j.Projection = "detail"
	if c.Principal.Role == "admin" && !c.Principal.Permissions["job.write"] {
		j.ContactWindow = nil
	}
	if j.SlotProposal, err = m.pendingOrLastProposal(ctx, c, id); err != nil {
		return j, err
	}
	if j.Assignment, err = m.activeAssignment(ctx, c, id); err != nil {
		return j, err
	}
	var pp PartnerSlotProposal
	err = c.Tx.QueryRow(ctx, `SELECT p.id, p.offer_id, lower(p.slot), upper(p.slot), p.technician_membership_id, p.reason, p.sent_at, p.status
		FROM maintenance.partner_slot_proposals p JOIN maintenance.offers o ON o.id = p.offer_id WHERE o.job_id = $1 AND p.status <> 'withdrawn'
		ORDER BY p.sent_at DESC, p.id LIMIT 1`, id).Scan(&pp.ID, &pp.OfferID, &pp.Slot.StartAt, &pp.Slot.EndAt, &pp.TechnicianMembershipID, &pp.Reason, &pp.SentAt, &pp.Status)
	switch {
	case err == nil && c.Principal.Role != "client" && c.Principal.Role != "technician":
		j.PartnerSlotProposal, _ = json.Marshal(pp)
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return j, err
	}
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.job_events WHERE job_id = $1`, id).Scan(&j.EventCursor); err != nil {
		return j, err
	}
	rows, err := c.Tx.Query(ctx, `SELECT id, version, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY version`, id)
	if err != nil {
		return j, err
	}
	for rows.Next() {
		var rid uuid.UUID
		var v int
		var state string
		if err := rows.Scan(&rid, &v, &state); err != nil {
			rows.Close()
			return j, err
		}
		ref := map[string]any{"reportId": rid, "reportVersion": v}
		switch {
		case state == "draft":
			if c.Principal.Role != "client" { // IR42
				j.DraftReportRef = ref
			}
		case c.Principal.Role != "client" || state == "accepted":
			j.ReportRefs = append(j.ReportRefs, ref)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return j, err
	}
	return j, m.project(ctx, c, &j)
}

// OfferView is Offer of service-contracts.ts.
type OfferView struct {
	ID               uuid.UUID  `json:"id"`
	TenantID         uuid.UUID  `json:"tenantId"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	JobID            uuid.UUID  `json:"jobId"`
	ContractorOrgID  uuid.UUID  `json:"contractorOrgId"`
	TermsVersion     string     `json:"termsVersion"`
	VisitSlot        Slot       `json:"visitSlot"`
	OfferedAt        time.Time  `json:"offeredAt"`
	OfferExpiresAt   time.Time  `json:"offerExpiresAt"`
	AccessValidFrom  time.Time  `json:"accessValidFrom"`
	AccessValidUntil time.Time  `json:"accessValidUntil"`
	Decision         *string    `json:"decision"`
	DecidedBy        *uuid.UUID `json:"decidedBy"`
	DecidedAt        *time.Time `json:"decidedAt"`
	DeclineReason    *string    `json:"declineReason"`
}

// project applies the IR42 role projections (and IR127 item 7: the current Offer) to a JobDetail.
func (m Jobs) project(ctx context.Context, c *ops.Call, j *Job) error {
	role := c.Principal.Role
	if role == "admin" || role == "contractor" {
		q := `SELECT id, tenant_id, version, created_at, updated_at, job_id, contractor_org_id, terms_version, lower(visit_slot), upper(visit_slot), offered_at,
			offer_expires_at, access_valid_from, access_valid_until, decision, decided_by, decided_at, decline_reason
			FROM maintenance.offers WHERE job_id = $1 AND ((decision IS NULL AND expired_at IS NULL) OR decision = 'accept')`
		args := []any{j.ID}
		if role == "contractor" {
			q += " AND contractor_org_id = $2"
			args = append(args, c.Principal.OrgID)
		}
		var o OfferView
		err := c.Tx.QueryRow(ctx, q+" ORDER BY offered_at DESC LIMIT 1", args...).Scan(&o.ID, &o.TenantID, &o.Version, &o.CreatedAt, &o.UpdatedAt, &o.JobID,
			&o.ContractorOrgID, &o.TermsVersion, &o.VisitSlot.StartAt, &o.VisitSlot.EndAt, &o.OfferedAt, &o.OfferExpiresAt, &o.AccessValidFrom, &o.AccessValidUntil,
			&o.Decision, &o.DecidedBy, &o.DecidedAt, &o.DeclineReason)
		switch {
		case err == nil:
			if role == "contractor" {
				o.DeclineReason = nil
			}
			j.Offer = o
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
	}
	switch role {
	case "client":
		if j.Assignment != nil {
			j.Assignment.Reason = nil
		}
		var lines []map[string]any
		_ = json.Unmarshal(j.Costs, &lines)
		kept := []map[string]any{}
		for _, l := range lines {
			if l["visibility"] == "customer" {
				kept = append(kept, l)
			}
		}
		j.Costs, _ = json.Marshal(kept)
	case "technician", "contractor":
		j.Costs = json.RawMessage("[]")
		if role == "technician" && j.Assignment != nil && j.Assignment.TechnicianMembershipID != c.Principal.MembershipID {
			j.Assignment.Reason = nil
		}
	}
	return nil
}

func (m Jobs) pendingOrLastProposal(ctx context.Context, c *ops.Call, job uuid.UUID) (*SlotProposal, error) {
	var p SlotProposal
	err := c.Tx.QueryRow(ctx, `SELECT id, source, lower(slot), upper(slot), hold, message, reply_by, status, decided_at, decline_reason, decline_comment
		FROM maintenance.slot_proposals WHERE job_id = $1 ORDER BY (status = 'pending') DESC, created_at DESC, id LIMIT 1`, job).
		Scan(&p.ID, &p.Source, &p.Slot.StartAt, &p.Slot.EndAt, &p.Hold, &p.Message, &p.ReplyBy, &p.Status, &p.DecidedAt, &p.DeclineReason, &p.DeclineComment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (m Jobs) activeAssignment(ctx context.Context, c *ops.Call, job uuid.UUID) (*Assignment, error) {
	var a Assignment
	var altStart, altEnd *time.Time
	err := c.Tx.QueryRow(ctx, `SELECT id, tenant_id, version, created_at, updated_at, job_id, technician_membership_id, valid_from, valid_until,
		lower(scheduled), upper(scheduled), status, reason, acknowledgement, acknowledged_at, cant_make_reason, lower(alternative_slot), upper(alternative_slot)
		FROM maintenance.assignments WHERE job_id = $1 AND status = 'active'`, job).
		Scan(&a.ID, &a.TenantID, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.JobID, &a.TechnicianMembershipID, &a.ValidFrom, &a.ValidUntil,
			&a.ScheduledStart, &a.ScheduledEnd, &a.Status, &a.Reason, &a.Acknowledgement, &a.AcknowledgedAt, &a.CantMakeReason, &altStart, &altEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if altStart != nil && altEnd != nil {
		a.AlternativeSlot = &Slot{*altStart, *altEnd}
	}
	return &a, err
}

// @Summary		jobs.get (read)
// @ID				jobs.get
// @Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | technician:assigned-history | admin:job.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR86 expired unanswered offer: NOT_FOUND for contractor
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A06, DD-C09, DD-P01, DD-P02, DD-P05, DD-P08, DD-T04, DD-T05, DD-T06, DD-T08, DD-T09, DD-T10, DD-C17, DD-T13, DD-P10, DD-P03
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			jobId	path		string	true	"input field jobId"
// @Success		200		{object}	ops.Envelope
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId} [get]
func (m Jobs) get(ctx context.Context, c *ops.Call, in *JobIDInput) (any, error) {
	if c.Principal.Role == "contractor" || c.Principal.Role == "technician" {
		v, _, err := m.projection(ctx, c, in.JobID)
		return v, err
	}
	return m.detail(ctx, c, in.JobID)
}

// ---- jobs.list ----

// listFilters are the jobs.list filters (query catalog).
type listFilters struct {
	UnitID          *uuid.UUID   `json:"unitId,omitempty"`
	UnitIDs         *[]uuid.UUID `json:"unitIds,omitempty"`
	Status          *string      `json:"status,omitempty"`
	Statuses        *[]string    `json:"statuses,omitempty"`
	Severity        *string      `json:"severity,omitempty"`
	From            *time.Time   `json:"from,omitempty"`
	To              *time.Time   `json:"to,omitempty"`
	OrganizationID  *uuid.UUID   `json:"organizationId,omitempty"`
	MembershipID    *uuid.UUID   `json:"membershipId,omitempty"`
	CustomerID      *uuid.UUID   `json:"customerId,omitempty"`
	PropertyID      *uuid.UUID   `json:"propertyId,omitempty"`
	OverdueOnly     *bool        `json:"overdueOnly,omitempty"`
	Origin          *string      `json:"origin,omitempty"`
	ProposalPending *bool        `json:"proposalPending,omitempty"`
}

// Summary is JobSummary of service-contracts.ts.
type Summary struct {
	Projection                string     `json:"projection"`
	ID                        uuid.UUID  `json:"id"`
	Version                   int        `json:"version"`
	UnitID                    uuid.UUID  `json:"unitId"`
	Type                      string     `json:"type"`
	Status                    string     `json:"status"`
	DueAt                     time.Time  `json:"dueAt"`
	RequestedSlot             Slot       `json:"requestedSlot"`
	ScheduledSlot             *Slot      `json:"scheduledSlot"`
	AssignmentID              *uuid.UUID `json:"assignmentId"`
	Origin                    string     `json:"origin"`
	PreferredSlots            []Slot     `json:"preferredSlots"`
	Severity                  string     `json:"severity"`
	IsDemo                    bool       `json:"isDemo"`
	DisplayStatus             string     `json:"displayStatus"`
	AssignmentAcknowledgement *string    `json:"assignmentAcknowledgement"`
	TechnicianMembershipID    *uuid.UUID `json:"technicianMembershipId"` // the active Assignment's technician (IR225)
	AccessValidFrom           *time.Time `json:"accessValidFrom"`        // the accepted Offer's access window (IR227)
	AccessValidUntil          *time.Time `json:"accessValidUntil"`
}

var jobStatuses = []string{"requested", "offered", "accepted", "assigned", "in_progress", "on_hold", "submitted", "rework_requested", "completed", "cancelled"}

const statusRank = `array_position(ARRAY['requested','offered','accepted','assigned','in_progress','on_hold','submitted','rework_requested','completed','cancelled'], j.status)`

var severityRank = map[string]int{"normal": 0, "warning": 1, "critical": 2}

// @Summary		jobs.list (read)
// @ID				jobs.list
// @Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | technician:assigned-history | admin:job.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR23: project before filters/sort/total
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A06, DD-C09, DD-P01, DD-P03, DD-P06, DD-T01, DD-T11, DD-T12, DD-P10, DD-P07 · Query: filters unitId,unitIds,status,severity,from,to,organizationId,membershipId,customerId,propertyId,statuses,overdueOnly,origin,proposalPending · sort id,severity,dueAt,status (default status asc;id asc)
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			cursor			query		string		false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit			query		integer		false	"page size 1–100, default 25"
// @Param			sort			query		string		false	"field:direction — fields id,severity,dueAt,status; default status asc;id asc"
// @Param			unitId			query		string		false	"filter"
// @Param			unitIds			query		[]string	false	"filter (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
// @Param			status			query		string		false	"filter"
// @Param			severity		query		string		false	"filter"
// @Param			from			query		string		false	"filter"
// @Param			to				query		string		false	"filter"
// @Param			organizationId	query		string		false	"filter"
// @Param			membershipId	query		string		false	"filter"
// @Param			customerId		query		string		false	"filter"
// @Param			propertyId		query		string		false	"filter"
// @Param			statuses		query		[]string	false	"filter (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
// @Param			overdueOnly		query		boolean		false	"filter"
// @Param			origin			query		string		false	"filter"
// @Param			proposalPending	query		boolean		false	"filter"
// @Success		200				{object}	ops.Envelope{data=anyPage}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs [get]
func (m Jobs) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[any], error) {
	var f listFilters
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[any]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if err := f.check(); err != nil {
		return paging.Page[any]{}, err
	}
	sortField, desc := "status", false
	if in.Sort != nil {
		if !slices.Contains([]string{"id", "severity", "dueAt", "status"}, in.Sort.Field) {
			return paging.Page[any]{}, apperr.Fields(map[string]string{"sort.field": "error.invalid"})
		}
		sortField, desc = in.Sort.Field, in.Sort.Direction == "desc"
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[any]{}, err
	}
	merged, err := m.collect(ctx, c, &f)
	if err != nil {
		return paging.Page[any]{}, err
	}
	sortItems(merged, sortField, desc)
	total := len(merged)
	items := []any{}
	for i := w.Offset; i < total && i < w.Offset+w.Limit; i++ {
		items = append(items, merged[i].item)
	}
	return paging.Page[any]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// check validates the shared job filters (IR26).
func (f *listFilters) check() error {
	bad := func(k string) error {
		return apperr.Fields(map[string]string{"filters." + k: "error.invalid"})
	}
	switch {
	case f.Status != nil && f.Statuses != nil:
		return bad("statuses")
	case f.Status != nil && !slices.Contains(jobStatuses, *f.Status):
		return bad("status")
	case f.Statuses != nil && len(*f.Statuses) == 0:
		return bad("statuses")
	case f.Severity != nil && severityRank[*f.Severity] == 0 && *f.Severity != "normal":
		return bad("severity")
	case f.Origin != nil && *f.Origin != "client_request" && *f.Origin != "periodic_plan":
		return bad("origin")
	case f.From != nil && f.To != nil && !f.From.Before(*f.To):
		return apperr.Fields(map[string]string{"filters.to": "error.range"})
	}
	if f.Statuses != nil {
		for _, s := range *f.Statuses {
			if !slices.Contains(jobStatuses, s) {
				return bad("statuses")
			}
		}
	}
	return nil
}

// collect returns the projected job rows matching the filters for the caller (shared by jobs.list and
// summaries.get, IR26).
func (m Jobs) collect(ctx context.Context, c *ops.Call, fp *listFilters) ([]listItem, error) {
	f := *fp
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{jobScope(c, &args)}
	if f.UnitID != nil {
		conds = append(conds, "j.unit_id = "+add(*f.UnitID))
	}
	if f.UnitIDs != nil {
		conds = append(conds, "j.unit_id = ANY("+add(*f.UnitIDs)+")")
	}
	if f.Status != nil {
		conds = append(conds, "j.status = "+add(*f.Status))
	}
	if f.Statuses != nil {
		conds = append(conds, "j.status = ANY("+add(*f.Statuses)+")")
	}
	if f.From != nil {
		conds = append(conds, "lower(j.requested_slot) >= "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "lower(j.requested_slot) < "+add(*f.To))
	}
	if f.OrganizationID != nil {
		if c.Principal.Role == "admin" && m.HQOrg != nil && *f.OrganizationID == m.HQOrg(c) {
			conds = append(conds, "j.contractor_org_id IS NULL")
		} else {
			conds = append(conds, "j.contractor_org_id = "+add(*f.OrganizationID))
		}
	}
	if f.MembershipID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active' AND a.technician_membership_id = "+add(*f.MembershipID)+")")
	}
	if f.CustomerID != nil {
		org, ok, err := m.Units.OrgOfCustomer(ctx, c, *f.CustomerID)
		if err != nil {
			return nil, err
		}
		if !ok {
			org = uuid.Nil
		}
		conds = append(conds, "j.customer_org_id = "+add(org))
	}
	if f.PropertyID != nil {
		units, err := m.Units.UnitsOfProperty(ctx, c, *f.PropertyID)
		if err != nil {
			return nil, err
		}
		conds = append(conds, "j.unit_id = ANY("+add(units)+")")
	}
	if f.OverdueOnly != nil && *f.OverdueOnly {
		conds = append(conds, "j.due_at < "+add(c.Now)+" AND j.status NOT IN ('completed','cancelled')")
	}
	if f.Origin != nil {
		conds = append(conds, "j.origin = "+add(*f.Origin))
	}
	if f.ProposalPending != nil {
		q := "EXISTS (SELECT 1 FROM maintenance.slot_proposals p WHERE p.job_id = j.id AND p.status = 'pending')"
		if !*f.ProposalPending {
			q = "NOT " + q
		}
		conds = append(conds, q)
	}
	rows, err := c.Tx.Query(ctx, `SELECT j.id, j.version, j.unit_id, j.type, j.status, j.due_at, lower(j.requested_slot), upper(j.requested_slot),
		lower(j.scheduled_slot), upper(j.scheduled_slot), j.assignment_id, j.origin, j.preferred_slots, `+statusRank+`,
		EXISTS (SELECT 1 FROM maintenance.slot_proposals p WHERE p.job_id = j.id AND p.status = 'pending'),
		(SELECT a.acknowledgement FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active'),
		(SELECT a.technician_membership_id FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active'),
		ao.access_valid_from, ao.access_valid_until
		FROM maintenance.jobs j LEFT JOIN LATERAL (SELECT o.access_valid_from, o.access_valid_until FROM maintenance.offers o
			WHERE o.job_id = j.id AND o.decision = 'accept' ORDER BY o.decided_at DESC LIMIT 1) ao ON true
		WHERE `+strings.Join(conds, " AND "), args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		s    Summary
		rank int
	}
	var all []row
	var units []uuid.UUID
	for rows.Next() {
		var r row
		var ss, se *time.Time
		var pref []byte
		var pending bool
		if err := rows.Scan(&r.s.ID, &r.s.Version, &r.s.UnitID, &r.s.Type, &r.s.Status, &r.s.DueAt, &r.s.RequestedSlot.StartAt, &r.s.RequestedSlot.EndAt,
			&ss, &se, &r.s.AssignmentID, &r.s.Origin, &pref, &r.rank, &pending, &r.s.AssignmentAcknowledgement, &r.s.TechnicianMembershipID,
			&r.s.AccessValidFrom, &r.s.AccessValidUntil); err != nil {
			rows.Close()
			return nil, err
		}
		if ss != nil && se != nil {
			r.s.ScheduledSlot = &Slot{*ss, *se}
		}
		r.s.PreferredSlots = []Slot{}
		_ = json.Unmarshal(pref, &r.s.PreferredSlots)
		r.s.Projection, r.s.IsDemo, r.s.DisplayStatus = "summary", true, r.s.Status
		if pending {
			r.s.DisplayStatus = "time_proposed"
		}
		all, units = append(all, r), append(units, r.s.UnitID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sev, err := m.Severity.UnitSeverities(ctx, c, units)
	if err != nil {
		return nil, err
	}
	var merged []listItem
	for _, r := range all {
		r.s.Severity = sev[r.s.UnitID]
		if r.s.Severity == "" {
			r.s.Severity = "normal"
		}
		if f.Severity == nil || r.s.Severity == *f.Severity {
			due := r.s.DueAt
			merged = append(merged, listItem{rank: r.rank, sev: severityRank[r.s.Severity], due: &due, id: r.s.ID.String(), item: r.s})
		}
	}
	if c.Principal.Role == "contractor" || c.Principal.Role == "technician" {
		extra, err := m.extraProjections(ctx, c, &f)
		if err != nil {
			return nil, err
		}
		merged = append(merged, extra...)
	}
	return merged, nil
}

// ---- jobs.events ----

// EventsInput is jobs.events input.
type EventsInput struct {
	JobID uuid.UUID    `json:"jobId"`
	Query paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *EventsInput) Validate() map[string]string {
	if in.JobID == uuid.Nil {
		return map[string]string{"jobId": "error.required"}
	}
	return nil
}

// Note is JobNote of service-contracts.ts.
type Note struct {
	ID         uuid.UUID `json:"id"`
	TenantID   uuid.UUID `json:"tenantId"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	JobID      uuid.UUID `json:"jobId"`
	AuthorID   uuid.UUID `json:"authorId"`
	Visibility string    `json:"visibility"`
	Message    string    `json:"message"`
}

// Event is JobEvent of service-contracts.ts (IR122 item 5).
type Event struct {
	ID          uuid.UUID  `json:"id"`
	TenantID    uuid.UUID  `json:"tenantId"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	JobID       uuid.UUID  `json:"jobId"`
	ActorUserID *uuid.UUID `json:"actorUserId"`
	Action      string     `json:"action"`
	OccurredAt  time.Time  `json:"occurredAt"`
	Note        *Note      `json:"note"`
	ReportRef   any        `json:"reportRef"`
}

// @Summary		jobs.events (read)
// @ID				jobs.events
// @Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | technician:assigned-history | admin:job.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-P07, DD-P01, DD-P02 · Query: filters from,to · sort id,occurredAt (default occurredAt asc;id asc)
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			jobId	path		string	true	"input field jobId"
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,occurredAt; default occurredAt asc;id asc"
// @Param			from	query		string	false	"filter → [from,to) on occurredAt"
// @Param			to		query		string	false	"filter → [from,to) on occurredAt"
// @Success		200		{object}	ops.Envelope{data=EventPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/jobs/{jobId}/events [get]
func (m Jobs) events(ctx context.Context, c *ops.Call, in *EventsInput) (paging.Page[Event], error) {
	ownOnly := false
	if c.Principal.Role == "contractor" || c.Principal.Role == "technician" {
		_, kind, err := m.projection(ctx, c, in.JobID)
		if err != nil {
			return paging.Page[Event]{}, err
		}
		ownOnly = kind != "detail" // offer / history: only the caller's own decision events (IR124 item 5)
	} else if _, err := m.detail(ctx, c, in.JobID); err != nil {
		return paging.Page[Event]{}, err
	}
	var f struct {
		From *time.Time `json:"from,omitempty"`
		To   *time.Time `json:"to,omitempty"`
	}
	if len(in.Query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Query.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Event]{}, apperr.Fields(map[string]string{"query.filters": "error.invalid"})
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return paging.Page[Event]{}, apperr.Fields(map[string]string{"query.filters.to": "error.range"})
	}
	order, err := paging.OrderBy(in.Query.Sort, map[string]string{"id": "e.id", "occurredAt": "e.occurred_at"}, "e.occurred_at ASC, e.id ASC")
	if err != nil {
		return paging.Page[Event]{}, err
	}
	w, err := paging.Resolve(in.Query, struct {
		Job uuid.UUID
		F   any
	}{in.JobID, f}, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Event]{}, err
	}
	args := []any{in.JobID}
	where := "e.job_id = $1"
	if c.Principal.Role == "client" {
		where += " AND (n.id IS NULL OR n.visibility = 'customer')"
	}
	if ownOnly {
		args = append(args, c.Principal.UserID)
		where += fmt.Sprintf(" AND e.actor_user_id = $%d AND e.action IN ('offer.accepted','offer.declined')", len(args))
	}
	if f.From != nil {
		args = append(args, *f.From)
		where += fmt.Sprintf(" AND e.occurred_at >= $%d", len(args))
	}
	if f.To != nil {
		args = append(args, *f.To)
		where += fmt.Sprintf(" AND e.occurred_at < $%d", len(args))
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON n.id = e.note_id WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Event]{}, err
	}
	items, err := m.eventRows(ctx, c, where, args, order, w.Limit, w.Offset)
	if err != nil {
		return paging.Page[Event]{}, err
	}
	if ownOnly {
		for i := range items {
			items[i].Note, items[i].ReportRef = nil, nil
		}
	}
	return paging.Page[Event]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// eventRows loads JobEvents (with their notes) matching where.
func (m Jobs) eventRows(ctx context.Context, c *ops.Call, where string, args []any, order string, limit, offset int) ([]Event, error) {
	rows, err := c.Tx.Query(ctx, fmt.Sprintf(`SELECT e.id, e.tenant_id, e.job_id, e.actor_user_id, e.action, e.occurred_at, e.report_id, e.report_version,
		n.id, n.author_id, n.visibility, n.message, n.created_at FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON n.id = e.note_id
		WHERE %s ORDER BY %s LIMIT %d OFFSET %d`, where, order, limit, offset), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Event{}
	for rows.Next() {
		var e Event
		var reportID *uuid.UUID
		var reportVersion *int
		var noteID, author *uuid.UUID
		var vis, msg *string
		var noteAt *time.Time
		if err := rows.Scan(&e.ID, &e.TenantID, &e.JobID, &e.ActorUserID, &e.Action, &e.OccurredAt, &reportID, &reportVersion, &noteID, &author, &vis, &msg, &noteAt); err != nil {
			return nil, err
		}
		e.Version, e.CreatedAt, e.UpdatedAt = 1, e.OccurredAt, e.OccurredAt
		if reportID != nil && reportVersion != nil {
			e.ReportRef = map[string]any{"reportId": *reportID, "reportVersion": *reportVersion}
		}
		if noteID != nil {
			e.Note = &Note{ID: *noteID, TenantID: e.TenantID, Version: 1, CreatedAt: *noteAt, UpdatedAt: *noteAt, JobID: e.JobID, AuthorID: *author, Visibility: *vis, Message: *msg}
		}
		items = append(items, e)
	}
	return items, rows.Err()
}

// ---- state changes ----

// lockJob loads a job in scope for update and checks the expected version.
func (m Jobs) lockJob(ctx context.Context, c *ops.Call, id uuid.UUID) (string, error) {
	args := []any{id}
	var status string
	var v int
	err := c.Tx.QueryRow(ctx, "SELECT j.status, j.version FROM maintenance.jobs j WHERE j.id = $1 AND "+jobScope(c, &args)+" FOR UPDATE", args...).Scan(&status, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return "", err
	}
	if v != *c.ExpectedVersion {
		return status, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return status, nil
}

func trimmedReason(s *string, max int, key string, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > max {
		fe[key] = "error.length"
	}
}

// CancelInput is jobs.cancel input.
type CancelInput struct {
	JobID        uuid.UUID `json:"jobId"`
	CancelReason string    `json:"cancelReason"`
}

// Validate implements ops.Validator.
func (in *CancelInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	trimmedReason(&in.CancelReason, 1000, "cancelReason", fe)
	return fe
}

func (m Jobs) finish(ctx context.Context, c *ops.Call, id uuid.UUID, action, event string, reason string) (Job, error) {
	j, err := m.detail(ctx, c, id)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: id, Type: event, Payload: map[string]any{"status": j.Status, "unitId": j.UnitID}})
	c.Audit(ops.AuditEntry{Action: action, TargetKind: "job", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &j.Version, Reason: reason})
	return j, nil
}

// finishScoped is finish for callers whose access was checked by the operation (technician / contractor writes).
func (m Jobs) finishScoped(ctx context.Context, c *ops.Call, id uuid.UUID, action, event string, reason string) (Job, error) {
	j, err := m.scoped(ctx, c, id)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: id, Type: event, Payload: map[string]any{"status": j.Status, "unitId": j.UnitID}})
	c.Audit(ops.AuditEntry{Action: action, TargetKind: "job", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &j.Version, Reason: reason})
	return j, nil
}

// @Summary		jobs.cancel (write)
// @ID				jobs.cancel
// @Description	Authorization: client:self:requested-only | admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR56 state table
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06, DD-C09
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string		true	"input field jobId"
// @Param			request				body		CancelInput	true	"input; the path parameters come from the route"
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
// @Router			/v1/jobs/{jobId}/cancel [post]
func (m Jobs) cancel(ctx context.Context, c *ops.Call, in *CancelInput) (Job, error) {
	status, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	allowed := status == "requested"
	if c.Principal.Role == "admin" {
		allowed = slices.Contains([]string{"requested", "offered", "accepted", "assigned", "on_hold", "rework_requested"}, status)
	}
	if !allowed {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.offers SET decision = 'decline', decided_by = $2, decided_at = $3, decline_reason = 'job_cancelled',
		version = version + 1, updated_at = platform.app_now() WHERE job_id = $1 AND decision IS NULL`, in.JobID, c.Principal.MembershipID, c.Now); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.assignments SET status = 'revoked', reason = 'job_cancelled', version = version + 1, updated_at = $2
		WHERE job_id = $1 AND status = 'active'`, in.JobID, c.Now); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $2 WHERE job_id = $1 AND status = 'pending'`, in.JobID, c.Now); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'cancelled', hold_reason = NULL, version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID); err != nil {
		return Job{}, err
	}
	if err := m.event(ctx, c, in.JobID, "job.cancelled", nil); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "jobs.cancel", "JobCancelled", in.CancelReason)
}

// ReasonInput is the {jobId, reason} input of jobs.hold / jobs.resumeHold.
type ReasonInput struct {
	JobID  uuid.UUID `json:"jobId"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ReasonInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// @Summary		jobs.hold (write)
// @ID				jobs.hold
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string		true	"input field jobId"
// @Param			request				body		ReasonInput	true	"input; the path parameters come from the route"
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
// @Router			/v1/jobs/{jobId}/hold [post]
func (m Jobs) hold(ctx context.Context, c *ops.Call, in *ReasonInput) (Job, error) {
	status, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "in_progress" && status != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'on_hold', hold_reason = $2, version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID, in.Reason); err != nil {
		return Job{}, err
	}
	if err := m.event(ctx, c, in.JobID, "job.held", nil); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "jobs.hold", "JobHeld", in.Reason)
}

// @Summary		jobs.resumeHold (write)
// @ID				jobs.resumeHold
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			jobId				path		string		true	"input field jobId"
// @Param			request				body		ReasonInput	true	"input; the path parameters come from the route"
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
// @Router			/v1/jobs/{jobId}/resume-hold [post]
func (m Jobs) resumeHold(ctx context.Context, c *ops.Call, in *ReasonInput) (Job, error) {
	status, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "on_hold" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'in_progress', hold_reason = NULL, version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID); err != nil {
		return Job{}, err
	}
	if err := NewDraftFromLatest(ctx, c, in.JobID); err != nil { // a submitted report stays; work continues on a new draft (D06)
		return Job{}, err
	}
	if err := m.event(ctx, c, in.JobID, "job.resumed", nil); err != nil {
		return Job{}, err
	}
	return m.finish(ctx, c, in.JobID, "jobs.resumeHold", "JobResumed", in.Reason)
}

// NoteInput is jobs.addNote input.
type NoteInput struct {
	JobID      uuid.UUID `json:"jobId"`
	Message    string    `json:"message"`
	Visibility string    `json:"visibility"`
}

// Validate implements ops.Validator.
func (in *NoteInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	trimmedReason(&in.Message, 2000, "message", fe)
	if in.Visibility != "internal" && in.Visibility != "customer" {
		fe["visibility"] = "error.invalid"
	}
	return fe
}

// addNote returns the created JobNote (service-contracts jobs.addNote); the job version is bumped.
//
//	@Summary		jobs.addNote (write)
//	@ID				jobs.addNote
//	@Description	Authorization: client:self:customer-visibility | contractor:delegated | admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C09, DD-P07
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		NoteInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Note}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/notes [post]
func (m Jobs) addNote(ctx context.Context, c *ops.Call, in *NoteInput) (Note, error) {
	if c.Principal.Role == "client" && in.Visibility != "customer" {
		return Note{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	status, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Note{}, err
	}
	if status == "completed" || status == "cancelled" {
		return Note{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var note uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.job_notes (tenant_id, job_id, author_id, visibility, message, created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5) RETURNING id`, in.JobID, c.Principal.UserID, in.Visibility, in.Message, c.Now).Scan(&note); err != nil {
		return Note{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID); err != nil {
		return Note{}, err
	}
	if err := m.event(ctx, c, in.JobID, "note.added", &note); err != nil {
		return Note{}, err
	}
	if _, err := m.finish(ctx, c, in.JobID, "jobs.addNote", "JobNoteAdded", ""); err != nil {
		return Note{}, err
	}
	x := Note{ID: note, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, JobID: in.JobID, AuthorID: c.Principal.UserID, Visibility: in.Visibility, Message: in.Message}
	err = c.Tx.QueryRow(ctx, `SELECT tenant_id, message FROM maintenance.job_notes WHERE id = $1`, note).Scan(&x.TenantID, &x.Message)
	return x, err
}

// RegisterJobs binds the job operations implemented so far.
func RegisterJobs(r *ops.Registry, m Jobs) {
	ops.Register(r, "jobs.create", m.create)
	ops.Register(r, "jobs.get", m.get)
	ops.Register(r, "jobs.list", m.list)
	ops.Register(r, "jobs.events", m.events)
	ops.Register(r, "jobs.cancel", m.cancel)
	ops.Register(r, "jobs.hold", m.hold)
	ops.Register(r, "jobs.resumeHold", m.resumeHold)
	ops.Register(r, "jobs.addNote", m.addNote)
}

func unmarshalSlots(raw []byte, out *[]Slot) error {
	*out = []Slot{}
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
