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

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// JobUnits is what the job operations need from Assets.
type JobUnits interface {
	UnitState(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (org uuid.UUID, archived bool, found bool, err error)
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error)
	UnitsOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error)
}

// UnitSeverity returns the IR23 job severity per unit (highest open/acknowledged alert, normal without one).
type UnitSeverity interface {
	UnitSeverities(
		ctx context.Context,
		c *ops.Call,
		units []uuid.UUID,
	) (map[uuid.UUID]string, error)
}

// Jobs is the job operation set.
type Jobs struct {
	Units    JobUnits
	Severity UnitSeverity
	// HQOrg returns the operator organization of the tenant (internal delivery, IR122 item 6).
	HQOrg func(c *ops.Call) uuid.UUID
	Sites SiteAddresses
	// Blobs holds the problem photos that jobs.getAttachment answers (IR437)
	Blobs blob.Store
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
	Hold           json.RawMessage `json:"hold"           swaggertype:"object"`
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
	// TechnicianName is the technician's display name in the client's projection once the technician accepted the
	// assignment (DD-C09, IR237); null otherwise and for the other roles, which read names from members.list.
	TechnicianName *string `json:"technicianName"`
}

// MaintenanceJob is MaintenanceJob of service-contracts.ts: what a job write answers (IR392).
type MaintenanceJob struct {
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
	Costs               json.RawMessage `json:"costs"               swaggertype:"object"`
	FollowUpOfJobID     *uuid.UUID      `json:"followUpOfJobId"`
	FollowUpClass       *string         `json:"followUpClass"`
	TimeOnSite          json.RawMessage `json:"timeOnSite"          swaggertype:"object"`
	Rating              json.RawMessage `json:"rating"              swaggertype:"object"`
	CustomerConfirmedAt *time.Time      `json:"customerConfirmedAt"`
	WarrantyClaims      json.RawMessage `json:"warrantyClaims"      swaggertype:"object"`
}

// Job is a MaintenanceJob with the JobDetail / JobSummary extras, as jobs.get and the job projections read it.
type Job struct {
	MaintenanceJob

	// detail
	Projection  string      `json:"projection"`
	Assignment  *Assignment `json:"assignment"`
	Offer       any         `json:"offer"`
	EventCursor int         `json:"eventCursor"`
	// the photos the customer sent with jobs.reportProblem, kept with this follow-up job (IR437)
	ProblemPhotos []JobAttachment `json:"problemPhotos"`
	// summary-only
	customerOrg uuid.UUID
}

// written is the answer of a job write: the job in the caller's projection without the JobDetail extras, as the
// contract's MaintenanceJob (IR392).
func written(j Job, err error) (MaintenanceJob, error) { return j.MaintenanceJob, err }

const jobCols = `j.id, j.tenant_id, j.version, j.created_at, j.updated_at, j.plan_id, j.occurrence_at, j.unit_id, ` +
	`j.type, j.status, j.symptom, j.contact_window,
	lower(j.requested_slot), upper(j.requested_slot), j.origin, j.preferred_slots, j.preference_round, ` +
	`lower(j.scheduled_slot), upper(j.scheduled_slot), j.due_at,
	j.started_at, j.completed_at, j.contractor_org_id, j.assignment_id, j.costs, j.follow_up_of_job_id, ` +
	`j.follow_up_class, j.time_on_site, j.rating,
	j.customer_confirmed_at, j.warranty_claims, j.customer_org_id,
	COALESCE((SELECT array_agg(ja.alert_id ORDER BY ja.alert_id) FROM maintenance.job_alerts ja WHERE ja.job_id = j.id),` +
	` '{}')`

func scanJob(row pgx.Row) (Job, error) {
	var (
		job                  Job
		preferred            []byte
		schedStart, schedEnd *time.Time
	)

	err := row.Scan(
		&job.ID,
		&job.TenantID,
		&job.Version,
		&job.CreatedAt,
		&job.UpdatedAt,
		&job.PlanID,
		&job.OccurrenceAt,
		&job.UnitID,
		&job.Type,
		&job.Status,
		&job.Symptom,
		&job.ContactWindow,
		&job.RequestedSlot.StartAt,
		&job.RequestedSlot.EndAt,
		&job.Origin,
		&preferred,
		&job.PreferenceRound,
		&schedStart,
		&schedEnd,
		&job.DueAt,
		&job.StartedAt,
		&job.CompletedAt,
		&job.ContractorOrgID,
		&job.AssignmentID,
		&job.Costs,
		&job.FollowUpOfJobID,
		&job.FollowUpClass,
		&job.TimeOnSite,
		&job.Rating,
		&job.CustomerConfirmedAt,
		&job.WarrantyClaims,
		&job.customerOrg,
		&job.AlertIDs,
	)
	if err != nil {
		return job, apperr.Wrap(err, "scan (scanJob)")
	}

	if schedStart != nil && schedEnd != nil {
		job.ScheduledSlot = &Slot{*schedStart, *schedEnd}
	}

	unmarshalSlotsErr := unmarshalSlots(preferred, &job.PreferredSlots)
	if unmarshalSlotsErr != nil {
		return job, unmarshalSlotsErr
	}

	if job.TimeOnSite == nil {
		job.TimeOnSite = json.RawMessage("null")
	}

	if job.Rating == nil {
		job.Rating = json.RawMessage("null")
	}

	job.PartnerSlotProposal = json.RawMessage("null")
	job.ReportRefs = []any{}

	return job, nil
}

// ---- scope ----

// jobScope applies D01 to job rows for the roles implemented here (client: own organization; admin: tenant).
// Contractor offer projections and technician assignment views are served by their own paths (IR23).
func jobScope(call *ops.Call, args *[]any) string {
	principal := call.Principal
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	switch principal.Role {
	case "admin":
		return "TRUE"
	case "client":
		return "j.customer_org_id = " + add(principal.OrgID)
	case "technician": // IR49 viewing window; external technicians also inside the accepted Offer's access window (IR124)
		now := add(call.Now)

		cond := "EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active' AND " +
			"a.technician_membership_id = " + add(
			principal.MembershipID,
		) +
			" AND a.created_at <= " + now + " AND " + now + " < upper(a.scheduled))"
		if principal.Employment != "internal" {
			cond += " AND EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision = 'accept' AND " +
				"o.access_valid_from <= " + now + " AND " + now + " < o.access_valid_until)"
		}

		return cond
	case "contractor": // accepted Offer inside its access window (IR23 summary projection)
		now := add(call.Now)

		return "EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = j.id AND o.contractor_org_id = " + add(
			principal.OrgID,
		) +
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

// ContactWindowOK applies IR64: no '@' and no run of seven digits once spaces, hyphens, parentheses and '+' are
// removed.
func ContactWindowOK(contactWindow string) bool {
	if strings.Contains(contactWindow, "@") {
		return false
	}

	stripped := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '+' || r == '\t' {
			return -1
		}

		return r
	}, contactWindow)

	return !contactDigits.MatchString(stripped)
}

// Validate implements ops.Validator (shape only; slot timing needs the clock).
func (in *CreateInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.UnitID == uuid.Nil {
		fieldErrs["unitId"] = apperr.KeyRequired
	}

	if !jobTypes[in.Type] {
		fieldErrs["type"] = apperr.KeyInvalid
	}

	in.Symptom = strings.TrimSpace(in.Symptom)
	if n := utf8.RuneCountInString(in.Symptom); n < 10 || n > 2000 {
		fieldErrs["symptom"] = apperr.KeyLength
	}

	if in.AlternativeSlots == nil {
		fieldErrs["alternativeSlots"] = apperr.KeyRequired
	}

	if in.ContactWindow != nil {
		*in.ContactWindow = strings.TrimSpace(*in.ContactWindow)
		if utf8.RuneCountInString(*in.ContactWindow) > 200 {
			fieldErrs["contactWindow"] = apperr.KeyLength
		} else if !ContactWindowOK(*in.ContactWindow) {
			fieldErrs["contactWindow"] = "errors.contact_details_forbidden"
		}
	}

	return fieldErrs
}

var kualaLumpur = func() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", 8*3600)
	}

	return l
}()

// PreferredSlotsOK applies IR113 item 2: distinct, each 1–4 hours, starting on a later Kuala Lumpur calendar day than
// now.
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

// create serves jobs.create.
//
//	@Summary		jobs.create (write)
//	@ID				jobs.create
//	@Description	Authorization: client:self | admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; IR38 dueAt only for job.write; defaults \
//	@Description	to requestedEnd; IR113 client: requested slot + exactly 2 alternativeSlots, 3 distinct future slots \
//	@Description	≥1 day ahead; HQ on behalf: 0–2
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06, DD-C09
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		CreateInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs [post]
func (m Jobs) create(ctx context.Context, c *ops.Call, in *CreateInput) (MaintenanceJob, error) {
	return written(m.createJob(ctx, c, in))
}

// createJob is create answering the job with its detail extras.
func (m Jobs) createJob(ctx context.Context, call *ops.Call, input *CreateInput) (Job, error) {
	if call.Principal.Role == "client" && input.DueAt != nil {
		return Job{}, apperr.Fields(map[string]string{"dueAt": apperr.KeyNotAllowed})
	}

	org, archived, found, err := m.Units.UnitState(ctx, call, input.UnitID)
	if err != nil {
		return Job{}, apperr.Wrap(err, "maintenance.JobUnits.UnitState")
	}

	if !found || (call.Principal.Role == "client" && org != call.Principal.OrgID) {
		return Job{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if archived {
		return Job{}, apperr.Fields(map[string]string{"unitId": apperr.KeyUnitArchived})
	}

	slots := append([]Slot{{input.RequestedStart, input.RequestedEnd}}, input.AlternativeSlots...)

	alts := len(input.AlternativeSlots)
	if (call.Principal.Role == "client" && alts != 2) || alts > 2 {
		return Job{}, apperr.Fields(map[string]string{"alternativeSlots": apperr.KeyCount})
	}

	if !PreferredSlotsOK(call.Now, slots) {
		return Job{}, apperr.Fields(map[string]string{"alternativeSlots": apperr.KeySlotRules})
	}

	due := input.RequestedEnd
	if input.DueAt != nil {
		if input.DueAt.Before(input.RequestedEnd) {
			return Job{}, apperr.Fields(map[string]string{"dueAt": "error.beforeRequestedEnd"})
		}

		due = *input.DueAt
	}

	var contactWindow *string
	if input.ContactWindow != nil && *input.ContactWindow != "" {
		contactWindow = input.ContactWindow
	}

	pref, _ := json.Marshal(slots)

	var jobID uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin,
		symptom, contact_window, requested_slot,
		preferred_slots, preference_round, due_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'requested',
		'client_request', $4, $5,
		tstzrange($6, $7), $8, 1, $9) RETURNING
		id`, input.UnitID, org, input.Type, input.Symptom, contactWindow, input.RequestedStart, input.RequestedEnd, pref, due).
		Scan(&jobID)
	if err != nil {
		return Job{}, apperr.Wrap(err, "insert maintenance.jobs")
	}

	eventErr := m.event(ctx, call, jobID, "job.created", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	job, err := m.detail(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	call.Emit(
		outboxEvent(
			"job",
			jobID,
			"JobCreated",
			map[string]any{"unitId": input.UnitID, "customerOrgId": org},
		),
	)
	call.Audit(auditEntry("jobs.create", "job", jobID, nil, &job.Version, ""))

	return job, nil
}

func (m Jobs) event(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	action string,
	note *uuid.UUID,
) error {
	// UUIDv7 IDs keep events of the same instant in write order (sort occurredAt asc, id asc)
	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, note_id, occurred_at)
		VALUES ($6, current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
		job,
		call.Principal.UserID,
		action,
		note,
		call.Now,
		uuid.Must(uuid.NewV7()),
	)

	return apperr.Wrap(err, "insert maintenance.job_events")
}

// outboxEvent is the outbox event of a write on the aggregate (backend architecture §8).
func outboxEvent(kind string, aggregate uuid.UUID, eventType string, payload any) ops.Event {
	return ops.Event{AggregateType: kind, AggregateID: aggregate, Type: eventType, Payload: payload}
}

// auditEntry is the audit entry of a write to the target: a success without field changes, with the target's version
// before and after it (nil where it has none).
func auditEntry(
	action, kind string,
	target uuid.UUID,
	previous, next *int,
	reason string,
) ops.AuditEntry {
	return ops.AuditEntry{
		Action:          action,
		TargetKind:      kind,
		TargetID:        target.String(),
		PreviousVersion: previous,
		NextVersion:     next,
		Reason:          reason,
		Result:          "",
		Before:          nil,
		After:           nil,
	}
}

// ---- jobs.get ----

// JobIDInput is the {jobId} input.
type JobIDInput struct {
	JobID uuid.UUID `json:"jobId"`
}

// Validate implements ops.Validator.
func (in *JobIDInput) Validate() map[string]string {
	if in.JobID == uuid.Nil {
		return map[string]string{"jobId": apperr.KeyRequired}
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

func (m Jobs) load(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	scope string,
	args []any,
) (Job, error) {
	job, err := scanJob(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+jobCols+" FROM maintenance.jobs j WHERE j.id = $1 AND "+scope,
			args...),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return job, err
	}

	job.Projection = "detail"

	job.SlotProposal, err = m.pendingOrLastProposal(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	job.Assignment, err = m.shownAssignment(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	var partnerSlotProposal PartnerSlotProposal

	err = call.Tx.QueryRow(ctx, `SELECT p.id, p.offer_id, lower(p.slot), upper(p.slot), p.technician_membership_id,
		p.reason,
		p.sent_at, p.status
		FROM maintenance.partner_slot_proposals p JOIN maintenance.offers o ON o.id = p.offer_id WHERE o.job_id = $1 AND
		p.status <> 'withdrawn'
		ORDER BY p.sent_at DESC, p.id LIMIT 1`, jobID).
		Scan(&partnerSlotProposal.ID, &partnerSlotProposal.OfferID, &partnerSlotProposal.Slot.StartAt,
			&partnerSlotProposal.Slot.EndAt, &partnerSlotProposal.TechnicianMembershipID, &partnerSlotProposal.Reason,
			&partnerSlotProposal.SentAt,
			&partnerSlotProposal.Status)
	switch {
	case err == nil && call.Principal.Role != "client" && call.Principal.Role != "technician":
		job.PartnerSlotProposal, _ = json.Marshal(partnerSlotProposal)
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return job, apperr.Wrap(err, "select maintenance.partner_slot_proposals")
	}

	cursorSQL := `SELECT count(*) FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON n.id = e.note_id WHERE
		e.job_id = $1`
	if call.Principal.Role == "client" {
		// the cursor counts what the client's jobs.events lists, so it reveals no hidden event (IR42, IR442)
		cursorSQL += clientEventFilter
	}

	scanErr := call.Tx.QueryRow(ctx, cursorSQL, jobID).Scan(&job.EventCursor)
	if scanErr != nil {
		return job, apperr.Wrap(scanErr, "queryrow (load)")
	}

	job.ProblemPhotos, err = problemPhotos(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, version, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY version`,
		jobID,
	)
	if err != nil {
		return job, apperr.Wrap(err, "select maintenance.work_reports")
	}

	for rows.Next() {
		var (
			rid     uuid.UUID
			version int
			state   string
		)

		err := rows.Scan(&rid, &version, &state)
		if err != nil {
			rows.Close()

			return job, apperr.Wrap(err, "select maintenance.work_reports")
		}

		ref := map[string]any{"reportId": rid, "reportVersion": version}

		switch {
		case state == "draft":
			if call.Principal.Role != "client" { // IR42
				job.DraftReportRef = ref
			}
		case call.Principal.Role != "client" || state == "accepted":
			job.ReportRefs = append(job.ReportRefs, ref)
		}
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return job, apperr.Wrap(rowsErr, "select maintenance.work_reports")
	}

	return job, m.project(ctx, call, &job)
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
func (m Jobs) project(ctx context.Context, call *ops.Call, job *Job) error {
	role := call.Principal.Role
	if role == "admin" || role == "contractor" {
		stmt := `SELECT id, tenant_id, version, created_at, updated_at, job_id, contractor_org_id, terms_version,
			lower(visit_slot), upper(visit_slot), offered_at,
			offer_expires_at, access_valid_from, access_valid_until, decision, decided_by, decided_at, decline_reason
			FROM maintenance.offers WHERE job_id = $1 AND ((decision IS NULL AND expired_at IS NULL) OR decision = 'accept')`
		args := []any{job.ID}

		if role == "contractor" {
			stmt += " AND contractor_org_id = $2"

			args = append(args, call.Principal.OrgID)
		}

		var offerView OfferView

		err := call.Tx.QueryRow(ctx, stmt+" ORDER BY offered_at DESC LIMIT 1", args...).
			Scan(&offerView.ID, &offerView.TenantID, &offerView.Version, &offerView.CreatedAt, &offerView.UpdatedAt,
				&offerView.JobID,
				&offerView.ContractorOrgID, &offerView.TermsVersion, &offerView.VisitSlot.StartAt, &offerView.VisitSlot.EndAt,
				&offerView.OfferedAt, &offerView.OfferExpiresAt,
				&offerView.AccessValidFrom, &offerView.AccessValidUntil,
				&offerView.Decision, &offerView.DecidedBy, &offerView.DecidedAt, &offerView.DeclineReason)
		switch {
		case err == nil:
			if role == "contractor" {
				offerView.DeclineReason = nil
			}

			job.Offer = offerView
		case !errors.Is(err, pgx.ErrNoRows):
			return apperr.Wrap(err, "queryrow (project)")
		}
	}

	switch role {
	case "client":
		if job.Assignment != nil {
			job.Assignment.Reason = nil
			if job.Assignment.Acknowledgement == "accepted" {
				var user uuid.UUID

				err := call.Tx.QueryRow(ctx, `SELECT user_id FROM maintenance.ref_memberships WHERE id =
					$1`, job.Assignment.TechnicianMembershipID).
					Scan(&user)
				if err == nil {
					members := []identity.Member{
						{
							UserID:         user,
							ID:             uuid.UUID{},
							TenantID:       uuid.UUID{},
							Version:        0,
							CreatedAt:      time.Time{},
							UpdatedAt:      time.Time{},
							DisplayName:    "",
							OrganizationID: uuid.UUID{},
							Role:           "",
							Employment:     nil,
							Permissions:    nil,
							Scopes:         nil,
							ScopeVersion:   0,
							Qualifications: nil,
							ValidFrom:      time.Time{},
							ValidUntil:     nil,
							ClientRole:     nil,
							RevokedAt:      nil,
						},
					}

					err := identity.LoadMembers(ctx, call, members)
					if err != nil {
						return apperr.Wrap(err, "identity.LoadMembers")
					}

					if members[0].DisplayName != "" {
						job.Assignment.TechnicianName = &members[0].DisplayName
					}
				} else if !errors.Is(
					err,
					pgx.ErrNoRows,
				) {
					return apperr.Wrap(err, "select maintenance.ref_memberships")
				}
			}
		}

		var lines []map[string]any

		_ = json.Unmarshal(job.Costs, &lines)
		kept := []map[string]any{}

		for _, l := range lines {
			if l["visibility"] == "customer" {
				kept = append(kept, l)
			}
		}

		job.Costs, _ = json.Marshal(kept)
	case "technician", "contractor":
		job.Costs = json.RawMessage("[]")
		if role == "technician" && job.Assignment != nil &&
			job.Assignment.TechnicianMembershipID != call.Principal.MembershipID {
			job.Assignment.Reason = nil
		}
	}

	return nil
}

func (m Jobs) pendingOrLastProposal(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (*SlotProposal, error) {
	var slotProposal SlotProposal

	err := c.Tx.QueryRow(ctx, `SELECT id, source, lower(slot), upper(slot), hold, message, reply_by, status, decided_at,
		decline_reason, decline_comment
		FROM maintenance.slot_proposals WHERE job_id = $1 ORDER BY (status = 'pending') DESC, created_at DESC, id LIMIT
		1`, job).
		Scan(&slotProposal.ID, &slotProposal.Source, &slotProposal.Slot.StartAt, &slotProposal.Slot.EndAt, &slotProposal.Hold,
			&slotProposal.Message, &slotProposal.ReplyBy, &slotProposal.Status, &slotProposal.DecidedAt,
			&slotProposal.DeclineReason, &slotProposal.DeclineComment)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &slotProposal, apperr.Wrap(err, "select maintenance.slot_proposals")
}

func (m Jobs) activeAssignment(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (*Assignment, error) {
	return m.assignmentWhere(ctx, c, job, "status = 'active'")
}

// shownAssignment is the job's Assignment for its projections: the active one, or for a completed job the one its
// completion released (IR234), so the detail still says who did the work.
func (m Jobs) shownAssignment(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (*Assignment, error) {
	return m.assignmentWhere(
		ctx,
		c,
		job,
		"status IN ('active', 'completed') ORDER BY status = 'active' DESC, updated_at DESC LIMIT 1",
	)
}

func (m Jobs) assignmentWhere(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	cond string,
) (*Assignment, error) {
	var (
		loaded           Assignment
		altStart, altEnd *time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, tenant_id, version, created_at, updated_at, job_id, technician_membership_id,
		valid_from, valid_until,
		lower(scheduled), upper(scheduled), status, reason, acknowledgement, acknowledged_at, cant_make_reason,
		lower(alternative_slot), upper(alternative_slot)
		FROM maintenance.assignments WHERE job_id = $1 AND `+cond, job).
		Scan(&loaded.ID, &loaded.TenantID, &loaded.Version, &loaded.CreatedAt, &loaded.UpdatedAt, &loaded.JobID, &loaded.TechnicianMembershipID, &loaded.ValidFrom,
			&loaded.ValidUntil,
			&loaded.ScheduledStart, &loaded.ScheduledEnd, &loaded.Status, &loaded.Reason, &loaded.Acknowledgement, &loaded.AcknowledgedAt, &loaded.CantMakeReason,
			&altStart, &altEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if altStart != nil && altEnd != nil {
		loaded.AlternativeSlot = &Slot{*altStart, *altEnd}
	}

	return &loaded, apperr.Wrap(err, "select maintenance.assignments")
}

// JobAttachment is JobAttachment of service-contracts.ts: a problem photo of a follow-up job (IR437).
type JobAttachment struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenantId"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	JobID     uuid.UUID `json:"jobId"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int       `json:"size"`
	Status    string    `json:"status"`
}

// problemPhotos lists the job's own attachments: those without a work report, which jobs.reportProblem stores.
func problemPhotos(ctx context.Context, c *ops.Call, job uuid.UUID) ([]JobAttachment, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, tenant_id, created_at, updated_at, job_id, name, mime, size_bytes, status FROM maintenance.attachments
		WHERE job_id = $1 AND report_id IS NULL ORDER BY created_at, id`,
		job,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.attachments")
	}
	defer rows.Close()

	out := []JobAttachment{}

	for rows.Next() {
		jobAttachment := JobAttachment{
			Version:   1,
			ID:        uuid.UUID{},
			TenantID:  uuid.UUID{},
			CreatedAt: time.Time{},
			UpdatedAt: time.Time{},
			JobID:     uuid.UUID{},
			Name:      "",
			Mime:      "",
			Size:      0,
			Status:    "",
		}

		err := rows.Scan(
			&jobAttachment.ID,
			&jobAttachment.TenantID,
			&jobAttachment.CreatedAt,
			&jobAttachment.UpdatedAt,
			&jobAttachment.JobID,
			&jobAttachment.Name,
			&jobAttachment.Mime,
			&jobAttachment.Size,
			&jobAttachment.Status,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.attachments")
		}

		out = append(out, jobAttachment)
	}

	return out, apperr.Wrap(rows.Err(), "select maintenance.attachments")
}

// JobAttachmentInput is jobs.getAttachment input.
type JobAttachmentInput struct {
	JobID        uuid.UUID `json:"jobId"`
	AttachmentID uuid.UUID `json:"attachmentId"`
}

// Validate implements ops.Validator.
func (in *JobAttachmentInput) Validate() map[string]string {
	if in.JobID == uuid.Nil || in.AttachmentID == uuid.Nil {
		return map[string]string{"attachmentId": apperr.KeyRequired}
	}

	return nil
}

// getAttachment answers a problem photo of a job to whoever may read the job (jobs.get decides, IR437).
//
//	@Summary		jobs.getAttachment (read)
//	@ID				jobs.getAttachment
//	@Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | \
//	@Description	technician:assigned-history | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; a problem photo of \
//	@Description	a follow-up job (IR437) for whoever may read the job
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-C17, DD-P02, DD-T04
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			jobId			path		string	true	"input field jobId"
//	@Param			attachmentId	path		string	true	"input field attachmentId"
//	@Success		200				{object}	ops.Envelope{data=BlobOut}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/attachments/{attachmentId}/content [get]
func (m Jobs) getAttachment(
	ctx context.Context,
	call *ops.Call,
	input *JobAttachmentInput,
) (BlobOut, error) {
	_, getErr := m.get(ctx, call, &JobIDInput{JobID: input.JobID})
	if getErr != nil {
		return BlobOut{}, getErr
	}

	var (
		key string
		out BlobOut
	)

	err := call.Tx.QueryRow(ctx, `SELECT object_key, name, mime, size_bytes FROM maintenance.attachments WHERE id = $1 AND
		job_id = $2 AND report_id IS NULL AND status = 'ready'`,
		input.AttachmentID, input.JobID).
		Scan(&key, &out.Name, &out.Mime, &out.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return out, apperr.Wrap(err, "select maintenance.attachments")
	}

	out.Bytes, err = m.Blobs.Get(ctx, key)
	if errors.Is(err, blob.ErrNotFound) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	} else if err != nil {
		return out, apperr.Wrap(err, "blob.Store.Get")
	}

	return out, nil
}

// get serves jobs.get.
//
//	@Summary		jobs.get (read)
//	@ID				jobs.get
//	@Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | \
//	@Description	technician:assigned-history | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR86 expired \
//	@Description	unanswered offer: NOT_FOUND for contractor
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-C09, DD-P01, DD-P02, DD-P05, DD-P08, DD-T04, DD-T05, DD-T06, DD-T08, DD-T09, \
//	@Description	DD-T10, DD-C17, DD-T13, DD-P10, DD-P03, DD-T02, DD-T07
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			jobId	path		string	true	"input field jobId"
//	@Success		200		{object}	ops.Envelope
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId} [get]
func (m Jobs) get(ctx context.Context, call *ops.Call, input *JobIDInput) (any, error) {
	if call.Principal.Role == "contractor" || call.Principal.Role == "technician" {
		v, _, err := m.projection(ctx, call, input.JobID)

		return v, err
	}

	return m.detail(ctx, call, input.JobID)
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
	// MaintenanceJob.type; offer and history rows match their public type (IR290)
	Type *string `json:"type,omitempty"`
}

// Summary is JobSummary of service-contracts.ts.
type Summary struct {
	Projection                string     `json:"projection"`
	ID                        uuid.UUID  `json:"id"`
	Version                   int        `json:"version"`
	CreatedAt                 time.Time  `json:"createdAt"`
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
	// a client's follow-up and its classification (IR114, IR338)
	FollowUpOfJobID *uuid.UUID   `json:"followUpOfJobId"`
	FollowUpClass   *string      `json:"followUpClass"`
	EventSummary    EventSummary `json:"eventSummary"` // jobs.list fills it for the page (IR506)
}

var jobStatuses = []string{
	"requested",
	"offered",
	"accepted",
	"assigned",
	"in_progress",
	"on_hold",
	"submitted",
	"rework_requested",
	"completed",
	"cancelled",
}

const statusRank = `array_position(ARRAY['requested','offered','accepted','assigned','in_progress','on_hold',` +
	`'submitted','rework_requested','completed','cancelled'], j.status)`

var severityRank = map[string]int{"normal": 0, "warning": 1, "critical": 2}

// list serves jobs.list.
//
//	@Summary		jobs.list (read)
//	@ID				jobs.list
//	@Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | \
//	@Description	technician:assigned-history | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR23: project \
//	@Description	before filters/sort/total
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-C09, DD-P01, DD-P03, DD-P06, DD-T01, DD-T11, DD-T12, DD-P10, DD-P07, DD-T13, \
//	@Description	DD-T02 · Query: filters \
//	@Description	unitId,unitIds,status,severity,from,to,organizationId,membershipId,customerId,propertyId,statuses,\
//	@Description	overdueOnly,origin,proposalPending,type · sort id,severity,dueAt,status (default status asc;id asc)
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer		false	"page size 1–100, default 25"
//	@Param			sort			query		string		false	"field:direction — fields id,severity,dueAt,status; default status asc;id asc"
//	@Param			unitId			query		string		false	"filter"
//	@Param			unitIds			query		[]string	false	"filter (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			status			query		string		false	"filter"
//	@Param			severity		query		string		false	"filter"
//	@Param			from			query		string		false	"filter → requestedSlot.startAt ≥ from and < to (IR245: the rule of admin.summary jobCounts)"
//	@Param			to				query		string		false	"filter → requestedSlot.startAt ≥ from and < to (IR245: the rule of admin.summary jobCounts)"
//	@Param			organizationId	query		string		false	"filter"
//	@Param			membershipId	query		string		false	"filter"
//	@Param			customerId		query		string		false	"filter"
//	@Param			propertyId		query		string		false	"filter"
//	@Param			statuses		query		[]string	false	"filter (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			overdueOnly		query		boolean		false	"filter"
//	@Param			origin			query		string		false	"filter"
//	@Param			proposalPending	query		boolean		false	"filter"
//	@Param			type			query		string		false	"filter"
//	@Success		200				{object}	ops.Envelope{data=anyPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs [get]
func (m Jobs) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[any], error) {
	var filters listFilters

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[any]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	err := filters.check()
	if err != nil {
		return paging.Page[any]{}, err
	}

	sortField, desc := "status", false

	if query.Sort != nil {
		if !slices.Contains([]string{"id", "severity", "dueAt", "status"}, query.Sort.Field) {
			return paging.Page[any]{}, apperr.Fields(
				map[string]string{"sort.field": apperr.KeyInvalid},
			)
		}

		sortField, desc = query.Sort.Field, query.Sort.Direction == "desc"
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[any]{}, apperr.Wrap(err, "paging.Resolve")
	}

	merged, err := m.collect(ctx, call, &filters)
	if err != nil {
		return paging.Page[any]{}, err
	}

	sortItems(merged, sortField, desc)
	total := len(merged)
	page := merged[min(window.Offset, total):min(window.Offset+window.Limit, total)]

	var jobs []uuid.UUID

	for _, x := range page {
		if s, ok := x.item.(Summary); ok {
			jobs = append(jobs, s.ID)
		}
	}

	sums, err := eventSummaries(ctx, call, jobs) // the page's event counts in one query (IR506)
	if err != nil {
		return paging.Page[any]{}, err
	}

	items := []any{}

	for _, entry := range page {
		if s, ok := entry.item.(Summary); ok {
			s.EventSummary = sums[s.ID]
			entry.item = s
		}

		items = append(items, entry.item)
	}

	return paging.Page[any]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// check validates the shared job filters (IR26).
func (f *listFilters) check() error {
	bad := func(k string) error {
		return apperr.Fields(map[string]string{"filters." + k: apperr.KeyInvalid})
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
	case f.Type != nil && !jobTypes[*f.Type]:
		return bad("type")
	case f.From != nil && f.To != nil && !f.From.Before(*f.To):
		return apperr.Fields(map[string]string{"filters.to": apperr.KeyRange})
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
func (m Jobs) collect(ctx context.Context, call *ops.Call, fp *listFilters) ([]listItem, error) {
	filters := *fp

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{jobScope(call, &args)}
	if filters.UnitID != nil {
		conds = append(conds, "j.unit_id = "+add(*filters.UnitID))
	}

	if filters.UnitIDs != nil {
		conds = append(conds, "j.unit_id = ANY("+add(*filters.UnitIDs)+")")
	}

	if filters.Status != nil {
		conds = append(conds, "j.status = "+add(*filters.Status))
	}

	if filters.Statuses != nil {
		conds = append(conds, "j.status = ANY("+add(*filters.Statuses)+")")
	}

	if filters.From != nil {
		conds = append(conds, "lower(j.requested_slot) >= "+add(*filters.From))
	}

	if filters.To != nil {
		conds = append(conds, "lower(j.requested_slot) < "+add(*filters.To))
	}

	if filters.OrganizationID != nil {
		if call.Principal.Role == "admin" && m.HQOrg != nil &&
			*filters.OrganizationID == m.HQOrg(call) {
			conds = append(conds, "j.contractor_org_id IS NULL")
		} else {
			conds = append(conds, "j.contractor_org_id = "+add(*filters.OrganizationID))
		}
	}

	if filters.MembershipID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status IN ('active', 'completed') "+
				"AND a.technician_membership_id = "+add(
				*filters.MembershipID,
			)+")",
		)
	}

	if filters.CustomerID != nil {
		org, ok, err := m.Units.OrgOfCustomer(ctx, call, *filters.CustomerID)
		if err != nil {
			return nil, apperr.Wrap(err, "maintenance.JobUnits.OrgOfCustomer")
		}

		if !ok {
			org = uuid.Nil
		}

		conds = append(conds, "j.customer_org_id = "+add(org))
	}

	if filters.PropertyID != nil {
		units, err := m.Units.UnitsOfProperty(ctx, call, *filters.PropertyID)
		if err != nil {
			return nil, apperr.Wrap(err, "maintenance.JobUnits.UnitsOfProperty")
		}

		conds = append(conds, "j.unit_id = ANY("+add(units)+")")
	}

	if filters.OverdueOnly != nil && *filters.OverdueOnly {
		conds = append(
			conds,
			"j.due_at < "+add(call.Now)+" AND j.status NOT IN ('completed','cancelled')",
		)
	}

	if filters.Origin != nil {
		conds = append(conds, "j.origin = "+add(*filters.Origin))
	}

	if filters.Type != nil {
		conds = append(conds, "j.type = "+add(*filters.Type))
	}

	if filters.ProposalPending != nil {
		q := "EXISTS (SELECT 1 FROM maintenance.slot_proposals p WHERE p.job_id = j.id AND p.status = 'pending')"
		if !*filters.ProposalPending {
			q = "NOT " + q
		}

		conds = append(conds, q)
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT j.id, j.version, j.unit_id, j.type, j.status, j.due_at, lower(j.requested_slot), upper(j.requested_slot),
		lower(j.scheduled_slot), upper(j.scheduled_slot), j.assignment_id, j.origin, j.preferred_slots, `+statusRank+`,
		EXISTS (SELECT 1 FROM maintenance.slot_proposals p WHERE p.job_id = j.id AND p.status = 'pending'),
		(SELECT a.acknowledgement FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status IN ('active', `+
			`'completed') ORDER BY a.status = 'active' DESC, a.updated_at DESC LIMIT 1),
		(SELECT a.technician_membership_id FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status IN ('active', `+
			`'completed') ORDER BY a.status = 'active' DESC, a.updated_at DESC LIMIT 1),
		ao.access_valid_from, ao.access_valid_until, j.created_at, j.follow_up_of_job_id, j.follow_up_class
		FROM maintenance.jobs j LEFT JOIN LATERAL (SELECT o.access_valid_from, o.access_valid_until FROM maintenance.offers o
			WHERE o.job_id = j.id AND o.decision = 'accept' ORDER BY o.decided_at DESC LIMIT 1) ao ON true
		WHERE `+strings.Join(
			conds,
			" AND ",
		),
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.jobs")
	}

	type row struct {
		s    Summary
		rank int
	}

	var (
		all   []row
		units []uuid.UUID
	)

	for rows.Next() {
		var (
			scanned              row
			schedStart, schedEnd *time.Time
			pref                 []byte
			pending              bool
		)

		err := rows.Scan(
			&scanned.s.ID,
			&scanned.s.Version,
			&scanned.s.UnitID,
			&scanned.s.Type,
			&scanned.s.Status,
			&scanned.s.DueAt,
			&scanned.s.RequestedSlot.StartAt,
			&scanned.s.RequestedSlot.EndAt,
			&schedStart,
			&schedEnd,
			&scanned.s.AssignmentID,
			&scanned.s.Origin,
			&pref,
			&scanned.rank,
			&pending,
			&scanned.s.AssignmentAcknowledgement,
			&scanned.s.TechnicianMembershipID,
			&scanned.s.AccessValidFrom,
			&scanned.s.AccessValidUntil,
			&scanned.s.CreatedAt,
			&scanned.s.FollowUpOfJobID,
			&scanned.s.FollowUpClass,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select maintenance.jobs")
		}

		if schedStart != nil && schedEnd != nil {
			scanned.s.ScheduledSlot = &Slot{*schedStart, *schedEnd}
		}

		scanned.s.PreferredSlots = []Slot{}
		_ = json.Unmarshal(pref, &scanned.s.PreferredSlots)

		scanned.s.Projection, scanned.s.IsDemo, scanned.s.DisplayStatus = "summary", true, scanned.s.Status
		if pending {
			scanned.s.DisplayStatus = "time_proposed"
		}

		all, units = append(all, scanned), append(units, scanned.s.UnitID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select maintenance.jobs")
	}

	sev, err := m.Severity.UnitSeverities(ctx, call, units)
	if err != nil {
		return nil, apperr.Wrap(err, "maintenance.UnitSeverity.UnitSeverities")
	}

	var merged []listItem

	for _, entry := range all {
		entry.s.Severity = sev[entry.s.UnitID]
		if entry.s.Severity == "" {
			entry.s.Severity = "normal"
		}

		if filters.Severity == nil || entry.s.Severity == *filters.Severity {
			due := entry.s.DueAt
			merged = append(
				merged,
				listItem{
					rank: entry.rank,
					sev:  severityRank[entry.s.Severity],
					due:  &due,
					id:   entry.s.ID.String(),
					item: entry.s,
				},
			)
		}
	}

	if call.Principal.Role == "contractor" || call.Principal.Role == "technician" {
		extra, err := m.extraProjections(ctx, call, &filters)
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
		return map[string]string{"jobId": apperr.KeyRequired}
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

// clientHiddenActions are the job events of HQ's and the contractor's own handling that the client's history leaves
// out, like internal notes (IR442): the costs and warranty records, and the contractor's time change while HQ alone
// decides on it (the change sent to the client shows as the client's proposal). Constants, so the query lists them
// inline.
var clientHiddenActions = []string{
	"job.costs_saved",
	"job.warranty_claim_filed",
	"partner_proposal.sent",
	"partner_proposal.withdrawn",
	"partner_proposal.kept",
}

// clientEventCond is the condition on job_events e (joined to job_notes n) of what a client may see: no internal note
// (IR42) and no hidden action. clientEventFilter appends it to a WHERE clause.
var clientEventCond = "(n.id IS NULL OR n.visibility = 'customer') AND e.action NOT IN ('" + strings.Join(
	clientHiddenActions,
	"','",
) + "')"

var clientEventFilter = " AND " + clientEventCond

// EventSummary is JobEventSummary of service-contracts.ts (IR506): the job's events the caller reads with jobs.events,
// counted for a list. customerVisible counts those the client reads (IR42, IR442), notes those with a note.
type EventSummary struct {
	Count           int          `json:"count"`
	CustomerVisible int          `json:"customerVisible"`
	Notes           int          `json:"notes"`
	Latest          *LatestEvent `json:"latest"`
}

// LatestEvent is the newest event of an EventSummary (occurredAt desc, then id desc).
type LatestEvent struct {
	Action     string    `json:"action"`
	OccurredAt time.Time `json:"occurredAt"`
}

// eventSummaries counts the events of the jobs in one query, with the caller's jobs.events visibility: a client's
// lists leave out internal notes and hidden actions; the other roles read the summaries of their summary projections,
// whose jobs.events lists every event (IR124 item 5).
func eventSummaries(
	ctx context.Context,
	call *ops.Call,
	jobs []uuid.UUID,
) (map[uuid.UUID]EventSummary, error) {
	out := map[uuid.UUID]EventSummary{}
	if len(jobs) == 0 {
		return out, nil
	}

	visible := "true"
	if call.Principal.Role == "client" {
		visible = clientEventCond
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT e.job_id, count(*) FILTER (WHERE
			`+visible+`), count(*) FILTER `+
			`(WHERE `+clientEventCond+`), count(n.id) `+
			`FILTER (WHERE `+visible+`),
		(array_agg(e.action ORDER BY e.occurred_at DESC, e.id DESC) FILTER `+
			`(WHERE `+visible+`))[1], max(e.occurred_at) `+
			`FILTER (WHERE `+visible+`)
		FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON n.id = e.note_id WHERE e.job_id = ANY($1) GROUP `+
			`BY e.job_id`,
		jobs,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select e.job_id")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			job          uuid.UUID
			eventSummary EventSummary
			action       *string
			latestAt     *time.Time
		)

		err := rows.Scan(
			&job,
			&eventSummary.Count,
			&eventSummary.CustomerVisible,
			&eventSummary.Notes,
			&action,
			&latestAt,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select e.job_id")
		}

		if action != nil && latestAt != nil {
			eventSummary.Latest = &LatestEvent{Action: *action, OccurredAt: *latestAt}
		}

		out[job] = eventSummary
	}

	return out, apperr.Wrap(rows.Err(), "select e.job_id")
}

// events serves jobs.events.
//
//	@Summary		jobs.events (read)
//	@ID				jobs.events
//	@Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | \
//	@Description	technician:assigned-history | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; clients do not get \
//	@Description	HQ's and the contractor's own handling events (IR442)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P07, DD-P01, DD-P02, DD-A06, DD-C09 · Query: filters from,to · sort id,occurredAt \
//	@Description	(default occurredAt asc;id asc)
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			jobId	path		string	true	"input field jobId"
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,occurredAt; default occurredAt asc;id asc"
//	@Param			from	query		string	false	"filter → [from,to) on occurredAt"
//	@Param			to		query		string	false	"filter → [from,to) on occurredAt"
//	@Success		200		{object}	ops.Envelope{data=EventPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/events [get]
func (m Jobs) events(
	ctx context.Context,
	call *ops.Call,
	input *EventsInput,
) (paging.Page[Event], error) {
	ownOnly := false

	if call.Principal.Role == "contractor" || call.Principal.Role == "technician" {
		_, kind, err := m.projection(ctx, call, input.JobID)
		if err != nil {
			return paging.Page[Event]{}, err
		}

		ownOnly = kind != "detail" // offer / history: only the decision events of the caller's side (IR124 item 5)
	} else {
		_, err := m.detail(ctx, call, input.JobID)
		if err != nil {
			return paging.Page[Event]{}, err
		}
	}

	var filters struct {
		From *time.Time `json:"from,omitempty"`
		To   *time.Time `json:"to,omitempty"`
	}

	if len(input.Query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(input.Query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[Event]{}, apperr.Fields(
				map[string]string{"query.filters": apperr.KeyInvalid},
			)
		}
	}

	if filters.From != nil && filters.To != nil && !filters.From.Before(*filters.To) {
		return paging.Page[Event]{}, apperr.Fields(
			map[string]string{"query.filters.to": apperr.KeyRange},
		)
	}

	order, err := paging.OrderBy(
		input.Query.Sort,
		map[string]string{"id": "e.id", "occurredAt": "e.occurred_at"},
		"e.occurred_at ASC, e.id ASC",
	)
	if err != nil {
		return paging.Page[Event]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(input.Query, struct {
		Job uuid.UUID
		F   any
	}{input.JobID, filters}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Event]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{input.JobID}

	where := "e.job_id = $1"
	if call.Principal.Role == "client" {
		where += clientEventFilter
	}

	if ownOnly {
		// the decision events of the caller's side: a contractor's organization, as its history snapshot stores them (IR506)
		where += " AND e.action IN ('offer.accepted','offer.declined')"

		if call.Principal.Role == "contractor" {
			args = append(args, call.Principal.OrgID)
			where += fmt.Sprintf(
				" AND EXISTS (SELECT 1 FROM maintenance.ref_memberships mb WHERE mb.user_id = e.actor_user_id AND "+
					"mb.organization_id = $%d)",
				len(args),
			)
		} else {
			args = append(args, call.Principal.UserID)
			where += fmt.Sprintf(" AND e.actor_user_id = $%d", len(args))
		}
	}

	if filters.From != nil {
		args = append(args, *filters.From)
		where += fmt.Sprintf(" AND e.occurred_at >= $%d", len(args))
	}

	if filters.To != nil {
		args = append(args, *filters.To)
		where += fmt.Sprintf(" AND e.occurred_at < $%d", len(args))
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON "+
		"n.id = e.note_id WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Event]{}, apperr.Wrap(scanErr, "select maintenance.job_events")
	}

	items, err := m.eventRows(ctx, call, where, args, order, window.Limit, window.Offset)
	if err != nil {
		return paging.Page[Event]{}, err
	}

	if ownOnly {
		for i := range items {
			items[i].Note, items[i].ReportRef = nil, nil
		}
	}

	return paging.Page[Event]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// eventRows loads JobEvents (with their notes) matching where.
func (m Jobs) eventRows(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	order string,
	limit, offset int,
) ([]Event, error) {
	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			`SELECT e.id, e.tenant_id, e.job_id, e.actor_user_id, e.action, e.occurred_at, e.report_id, e.report_version,
		n.id, n.author_id, n.visibility, n.message, n.created_at FROM maintenance.job_events e LEFT JOIN maintenance.job_notes
		n ON n.id = e.note_id
		WHERE %s ORDER BY %s LIMIT %d OFFSET %d`,
			where,
			order,
			limit,
			offset,
		),
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.job_events")
	}
	defer rows.Close()

	items := []Event{}

	for rows.Next() {
		var (
			event          Event
			reportID       *uuid.UUID
			reportVersion  *int
			noteID, author *uuid.UUID
			vis, msg       *string
			noteAt         *time.Time
		)

		err := rows.Scan(
			&event.ID,
			&event.TenantID,
			&event.JobID,
			&event.ActorUserID,
			&event.Action,
			&event.OccurredAt,
			&reportID,
			&reportVersion,
			&noteID,
			&author,
			&vis,
			&msg,
			&noteAt,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.job_events")
		}

		event.Version, event.CreatedAt, event.UpdatedAt = 1, event.OccurredAt, event.OccurredAt
		if reportID != nil && reportVersion != nil {
			event.ReportRef = map[string]any{"reportId": *reportID, "reportVersion": *reportVersion}
		}

		if noteID != nil {
			event.Note = &Note{
				ID:         *noteID,
				TenantID:   event.TenantID,
				Version:    1,
				CreatedAt:  *noteAt,
				UpdatedAt:  *noteAt,
				JobID:      event.JobID,
				AuthorID:   *author,
				Visibility: *vis,
				Message:    *msg,
			}
		}

		items = append(items, event)
	}

	return items, apperr.Wrap(rows.Err(), "select maintenance.job_events")
}

// ---- state changes ----

// lockJob loads a job in scope for update and checks the expected version.
func (m Jobs) lockJob(ctx context.Context, call *ops.Call, id uuid.UUID) (string, error) {
	args := []any{id}

	var (
		status  string
		version int
	)

	err := call.Tx.QueryRow(ctx, "SELECT j.status, j.version FROM maintenance.jobs j WHERE j.id = $1 AND "+
		jobScope(call, &args)+
		" FOR UPDATE", args...).
		Scan(&status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return "", apperr.Wrap(err, "select maintenance.jobs")
	}

	if version != *call.ExpectedVersion {
		return status, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return status, nil
}

func trimmedReason(s *string, max int, key string, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > max {
		fe[key] = apperr.KeyLength
	}
}

// CancelInput is jobs.cancel input.
type CancelInput struct {
	JobID        uuid.UUID `json:"jobId"`
	CancelReason string    `json:"cancelReason"`
}

// Validate implements ops.Validator.
func (in *CancelInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs["jobId"] = apperr.KeyRequired
	}

	trimmedReason(&in.CancelReason, 1000, "cancelReason", fieldErrs)

	return fieldErrs
}

func (m Jobs) finish(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	action, event string,
	reason string,
) (Job, error) {
	job, err := m.detail(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	call.Emit(
		outboxEvent(
			"job",
			jobID,
			event,
			map[string]any{"status": job.Status, "unitId": job.UnitID},
		),
	)
	call.Audit(auditEntry(action, "job", jobID, call.ExpectedVersion, &job.Version, reason))

	return job, nil
}

// finishScoped is finish for callers whose access was checked by the operation (technician / contractor writes).
func (m Jobs) finishScoped(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	action, event string,
	reason string,
) (Job, error) {
	job, err := m.scoped(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	call.Emit(
		outboxEvent(
			"job",
			jobID,
			event,
			map[string]any{"status": job.Status, "unitId": job.UnitID},
		),
	)
	call.Audit(auditEntry(action, "job", jobID, call.ExpectedVersion, &job.Version, reason))

	return job, nil
}

// cancel serves jobs.cancel.
//
//	@Summary		jobs.cancel (write)
//	@ID				jobs.cancel
//	@Description	Authorization: client:self:requested-only | admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR56 state table
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06, DD-C09
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		CancelInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/cancel [post]
func (m Jobs) cancel(ctx context.Context, c *ops.Call, in *CancelInput) (MaintenanceJob, error) {
	return written(m.cancelJob(ctx, c, in))
}

// cancelJob is cancel answering the job with its detail extras.
func (m Jobs) cancelJob(ctx context.Context, call *ops.Call, input *CancelInput) (Job, error) {
	status, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	allowed := status == "requested"
	if call.Principal.Role == "admin" {
		allowed = slices.Contains(
			[]string{"requested", "offered", "accepted", "assigned", "on_hold", "rework_requested"},
			status,
		)
	}

	if !allowed {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET decision = 'decline', decided_by = $2, decided_at = $3, decline_reason =
			'job_cancelled',
		version = version + 1, updated_at = platform.app_now() WHERE job_id = $1 AND decision IS NULL`,
		input.JobID,
		call.Principal.MembershipID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.offers")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.assignments SET status = 'revoked', reason = 'job_cancelled', version = version + 1, updated_at =
			$2
		WHERE job_id = $1 AND status = 'active'`,
		input.JobID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.assignments")
	}
	// the contractor's access to the unit ends with the job, as on a reschedule: a cancelled job leaves only its history
	// (BR-P08, IR588)
	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET access_valid_until = GREATEST(access_valid_from, $2), version = version + 1, updated_at
			= platform.app_now()
		WHERE job_id = $1 AND decision = 'accept' AND access_valid_until > $2`,
		input.JobID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.offers")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.slot_proposals SET status = 'withdrawn', decided_at = $2 WHERE job_id = $1 AND status =
			'pending'`,
		input.JobID,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.slot_proposals")
	}
	// the contractor's open time change ends with the job (IR444)
	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.partner_slot_proposals SET status = 'withdrawn' WHERE status IN ('pending','sent_to_client')
		AND offer_id IN (SELECT id FROM maintenance.offers WHERE job_id = $1)`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.partner_slot_proposals")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'cancelled', hold_reason = NULL, version = version + 1, updated_at =
			platform.app_now() WHERE id = $1`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.event(ctx, call, input.JobID, "job.cancelled", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.finish(ctx, call, input.JobID, "jobs.cancel", "JobCancelled", input.CancelReason)
}

// ReasonInput is the {jobId, reason} input of jobs.hold / jobs.resumeHold.
type ReasonInput struct {
	JobID  uuid.UUID `json:"jobId"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ReasonInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs["jobId"] = apperr.KeyRequired
	}

	trimmedReason(&in.Reason, 1000, "reason", fieldErrs)

	return fieldErrs
}

// hold serves jobs.hold.
//
//	@Summary		jobs.hold (write)
//	@ID				jobs.hold
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		ReasonInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/hold [post]
func (m Jobs) hold(ctx context.Context, c *ops.Call, in *ReasonInput) (MaintenanceJob, error) {
	return written(m.holdJob(ctx, c, in))
}

// holdJob is hold answering the job with its detail extras.
func (m Jobs) holdJob(ctx context.Context, call *ops.Call, input *ReasonInput) (Job, error) {
	status, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != "in_progress" && status != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'on_hold', hold_reason = $2, version = version + 1, updated_at =
			platform.app_now() WHERE id = $1`,
		input.JobID,
		input.Reason,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.event(ctx, call, input.JobID, "job.held", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.finish(ctx, call, input.JobID, "jobs.hold", "JobHeld", input.Reason)
}

// resumeHold serves jobs.resumeHold.
//
//	@Summary		jobs.resumeHold (write)
//	@ID				jobs.resumeHold
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		ReasonInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/resume-hold [post]
func (m Jobs) resumeHold(
	ctx context.Context,
	c *ops.Call,
	in *ReasonInput,
) (MaintenanceJob, error) {
	return written(m.resumeHoldJob(ctx, c, in))
}

// resumeHoldJob is resumeHold answering the job with its detail extras.
func (m Jobs) resumeHoldJob(ctx context.Context, call *ops.Call, input *ReasonInput) (Job, error) {
	status, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != "on_hold" {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'in_progress', hold_reason = NULL, version = version + 1, updated_at =
			platform.app_now() WHERE id = $1`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	newDraftFromLatestErr := NewDraftFromLatest(
		ctx,
		call,
		input.JobID,
	)
	if newDraftFromLatestErr != nil { // a submitted report stays; work continues on a new draft (D06)
		return Job{}, newDraftFromLatestErr
	}

	eventErr := m.event(ctx, call, input.JobID, "job.resumed", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.finish(ctx, call, input.JobID, "jobs.resumeHold", "JobResumed", input.Reason)
}

// NoteInput is jobs.addNote input.
type NoteInput struct {
	JobID      uuid.UUID `json:"jobId"`
	Message    string    `json:"message"`
	Visibility string    `json:"visibility"`
}

// Validate implements ops.Validator.
func (in *NoteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs["jobId"] = apperr.KeyRequired
	}

	trimmedReason(&in.Message, 2000, "message", fieldErrs)

	if in.Visibility != "internal" && in.Visibility != "customer" {
		fieldErrs["visibility"] = apperr.KeyInvalid
	}

	return fieldErrs
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
func (m Jobs) addNote(ctx context.Context, call *ops.Call, input *NoteInput) (Note, error) {
	if call.Principal.Role == "client" && input.Visibility != "customer" {
		return Note{}, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	status, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Note{}, err
	}

	if status == "completed" || status == "cancelled" {
		return Note{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	var note uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO maintenance.job_notes (tenant_id, job_id, author_id, visibility, message,
		created_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5) RETURNING
		id`, input.JobID, call.Principal.UserID, input.Visibility, input.Message, call.Now).
		Scan(&note)
	if scanErr != nil {
		return Note{}, apperr.Wrap(scanErr, "insert maintenance.job_notes")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		input.JobID,
	)
	if execErr != nil {
		return Note{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.event(ctx, call, input.JobID, "note.added", &note)
	if eventErr != nil {
		return Note{}, eventErr
	}

	_, finishErr := m.finish(ctx, call, input.JobID, "jobs.addNote", "JobNoteAdded", "")
	if finishErr != nil {
		return Note{}, finishErr
	}

	created := Note{
		ID:         note,
		Version:    1,
		CreatedAt:  call.Now,
		UpdatedAt:  call.Now,
		JobID:      input.JobID,
		AuthorID:   call.Principal.UserID,
		Visibility: input.Visibility,
		Message:    input.Message, TenantID: uuid.UUID{},
	}
	err = call.Tx.QueryRow(ctx, `SELECT tenant_id, message FROM maintenance.job_notes WHERE id = $1`, note).
		Scan(&created.TenantID, &created.Message)

	return created, apperr.Wrap(err, "select maintenance.job_notes")
}

// RegisterJobs binds the job operations implemented so far.
func RegisterJobs(reg *ops.Registry, jobs Jobs) {
	ops.Register(reg, "jobs.create", jobs.create)
	ops.Register(reg, "jobs.get", jobs.get)
	ops.Register(reg, "jobs.getAttachment", jobs.getAttachment)
	ops.Register(reg, "jobs.list", jobs.list)
	ops.Register(reg, "jobs.events", jobs.events)
	ops.Register(reg, "jobs.cancel", jobs.cancel)
	ops.Register(reg, "jobs.hold", jobs.hold)
	ops.Register(reg, "jobs.resumeHold", jobs.resumeHold)
	ops.Register(reg, "jobs.addNote", jobs.addNote)
	ops.RegisterQuery(reg, ops.DomainMaintenance, notify.QueryJobsOpen, jobs.jobsOpen)
}

// jobsOpen serves notify's QueryJobsOpen: of the jobs of one page of notices, those whose page opens for the caller —
// jobs.get answers them, live or as the caller's history (IR602). Only a contractor's and a technician's job pages
// can close while their notices stay readable; for the other roles every job is passed through.
func (m Jobs) jobsOpen(
	ctx context.Context,
	call *ops.Call,
	input *notify.JobsOpenInput,
) ([]uuid.UUID, error) {
	if len(input.JobIDs) > 100 {
		return nil, apperr.Fields(map[string]string{"jobIds": apperr.KeyRange})
	}

	out := []uuid.UUID{}

	for _, jobID := range input.JobIDs {
		if call.Principal.Role == "contractor" || call.Principal.Role == "technician" {
			_, _, err := m.projection(ctx, call, jobID)
			if err != nil {
				if code := apperr.From(
					err,
				).Code; code == apperr.NotFound ||
					code == apperr.Forbidden {
					continue
				}

				return nil, err
			}
		}

		out = append(out, jobID)
	}

	return out, nil
}

func unmarshalSlots(raw []byte, out *[]Slot) error {
	*out = []Slot{}

	if len(raw) == 0 {
		return nil
	}

	return apperr.Wrap(json.Unmarshal(raw, out), "json.Unmarshal")
}
