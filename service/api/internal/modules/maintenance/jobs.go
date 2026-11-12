package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
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
	// UnitInstall is the install state, "planned" or "installed" ("" for no such unit), and the model of the unit: only a
	// planned unit has an installation job.
	UnitInstall(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (state string, model uuid.UUID, err error)
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
type MaintenanceJob struct { //nolint:revive // the name service-contracts.ts and the API description use
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

// The projections a caller reads a job in (Job.projection, IR23): JobDetail, JobSummary, JobOfferSummary and
// JobHistorySnapshot.
const (
	projectionDetail  = "detail"
	projectionSummary = "summary"
	projectionOffer   = "offer"
	projectionHistory = "history"
)

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

	return completeScan(job, preferred, schedStart, schedEnd)
}

// completeScan fills in what scanJob reads beside the columns: the scheduled slot, the preferred slots, and null for
// the unset documents.
func completeScan(job Job, preferred []byte, schedStart, schedEnd *time.Time) (Job, error) {
	if schedStart != nil && schedEnd != nil {
		job.ScheduledSlot = &Slot{*schedStart, *schedEnd}
	}

	unmarshalSlotsErr := unmarshalSlots(preferred, &job.PreferredSlots)
	if unmarshalSlotsErr != nil {
		return job, unmarshalSlotsErr
	}

	if job.TimeOnSite == nil {
		job.TimeOnSite = json.RawMessage(jsonNull)
	}

	if job.Rating == nil {
		job.Rating = json.RawMessage(jsonNull)
	}

	job.PartnerSlotProposal = json.RawMessage(jsonNull)
	job.ReportRefs = []any{}

	return job, nil
}

// ---- scope ----

// The roles of the callers (Principal.Role).
const (
	roleAdmin       = "admin"
	roleClient      = "client"
	jobInstallation = "installation"
	installPlanned  = "planned"
	roleContractor  = "contractor"
	roleTechnician  = "technician"
)

// employmentInternal is the employment of HQ's own technicians (Principal.Employment, identity.Technician.Employment).
const employmentInternal = "internal"

// SQL fragments the queries are built from: the condition every row meets and the row lock of a read for update.
const (
	sqlTrue   = "TRUE"
	forUpdate = " FOR UPDATE"
)

// jsonNull is the JSON of an absent value: what a nullable jsonb column reads as when it is unset, and is answered as.
const jsonNull = "null"

// The field names that field errors, sort specs and event payloads name: the contract's JSON names.
const (
	fieldAttachmentID   = "attachmentId"
	fieldCreatedAt      = "createdAt"
	fieldDueAt          = "dueAt"
	fieldEffectiveFrom  = "effectiveFrom"
	fieldFilters        = "filters"
	fieldFrom           = "from"
	fieldHold           = "hold"
	fieldItems          = "items"
	fieldEndAt          = "endAt"
	fieldJobID          = "jobId"
	fieldName           = "name"
	fieldNextAction     = "nextAction"
	fieldInstallation   = "installation"
	fieldWorkText       = "workText"
	fieldOrganizationID = "organizationId"
	fieldReason         = "reason"
	fieldRefrigerant    = "refrigerant"
	fieldReportVersion  = "reportVersion"
	fieldSlot           = "slot"
	fieldStatus         = "status"
	fieldUnitID         = "unitId"
	fieldUpdatedAt      = "updatedAt"
)

// jobScope applies D01 to job rows for the roles implemented here (client: own organization; admin: tenant).
// Contractor offer projections and technician assignment views are served by their own paths (IR23).
func jobScope(call *ops.Call, args *[]any) string {
	principal := call.Principal
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	switch principal.Role {
	case roleAdmin:
		return sqlTrue
	case roleClient:
		return "j.customer_org_id = " + add(principal.OrgID)
	case roleTechnician:
		// IR49 viewing window; external technicians also inside the accepted Offer's access window (IR124)
		now := add(call.Now)

		cond := "EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status = 'active' AND " +
			"a.technician_membership_id = " + add(
			principal.MembershipID,
		) +
			" AND a.created_at <= " + now + " AND " + now + " < upper(a.scheduled))"
		if principal.Employment != employmentInternal {
			cond += " AND EXISTS (SELECT 1 FROM maintenance.offers o WHERE o.job_id = j.id AND o.decision = 'accept' AND " +
				"o.access_valid_from <= " + now + " AND " + now + " < o.access_valid_until)"
		}

		return cond
	case roleContractor: // accepted Offer inside its access window (IR23 summary projection)
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

// jobTypes are the types of a job (MaintenanceJob.type).
func jobTypes() map[string]bool {
	return map[string]bool{
		"periodic":      true,
		"reactive":      true,
		"preventive":    true,
		jobInstallation: true,
	}
}

// maxContactWindowLength is the longest contact window, in characters after trimming.
const maxContactWindowLength = 200

// minPhoneDigits is the length of a run of digits that reads as a phone number.
const minPhoneDigits = 7

// contactSeparators are what a phone number is written with between its digits besides spaces: the ASCII characters and
// the full-width ones a Japanese input method produces.
const contactSeparators = "-()+\t－（）＋"

// ContactWindowOK applies IR64 and IR640: no '@' and no run of seven digits, once spaces, hyphens, parentheses and '+'
// are set aside. Digits are the digits of every script and the symbols include their full-width forms, so a number
// typed with a Japanese input method (０９０－１２３４－５６７８, ＠) is refused like the ASCII one.
func ContactWindowOK(contactWindow string) bool {
	if strings.ContainsAny(contactWindow, "@＠") {
		return false
	}

	digits := 0

	for _, char := range contactWindow {
		switch {
		case unicode.IsDigit(char):
			digits++
			if digits >= minPhoneDigits {
				return false
			}
		case unicode.IsSpace(char) || strings.ContainsRune(contactSeparators, char):
			// a separator between digits does not end the run
		default:
			digits = 0
		}
	}

	return true
}

// Validate implements ops.Validator (shape only; slot timing needs the clock).
func (in *CreateInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.UnitID == uuid.Nil {
		fieldErrs[fieldUnitID] = apperr.KeyRequired
	}

	if !jobTypes()[in.Type] {
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
		if utf8.RuneCountInString(*in.ContactWindow) > maxContactWindowLength {
			fieldErrs["contactWindow"] = apperr.KeyLength
		} else if !ContactWindowOK(*in.ContactWindow) {
			fieldErrs["contactWindow"] = "errors.contact_details_forbidden"
		}
	}

	return fieldErrs
}

// maxBatchJobs is how many jobs one batch read names at most.
const maxBatchJobs = 100

// photoMaxBytes is the largest photo, signature or site photo an upload takes: 5 MiB.
const photoMaxBytes = 5 << 20

// kualaLumpurOffset is the UTC offset of Malaysia Time in seconds, the zone when the zone database is missing.
const kualaLumpurOffset = 8 * 60 * 60

//nolint:gochecknoglobals // loaded once: time.LoadLocation reads the zone database on every call
var kualaLumpur = func() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", kualaLumpurOffset)
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
	org, err := m.unitToRequest(ctx, call, input)
	if err != nil {
		return Job{}, err
	}

	slots, due, err := requestedSlots(call, input)
	if err != nil {
		return Job{}, err
	}

	jobID, err := insertRequest(ctx, call, input, org, slots, due)
	if err != nil {
		return Job{}, err
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
			map[string]any{fieldUnitID: input.UnitID, "customerOrgId": org},
		),
	)
	call.Audit(auditEntry("jobs.create", "job", jobID, nil, &job.Version, ""))

	return job, nil
}

// unitToRequest checks the unit a job is requested for and answers its organization: a unit of the client's own
// organization that is not archived. Only HQ sets the due time (IR38).
func (m Jobs) unitToRequest(
	ctx context.Context,
	call *ops.Call,
	input *CreateInput,
) (uuid.UUID, error) {
	if call.Principal.Role == roleClient && input.DueAt != nil {
		return uuid.Nil, apperr.Fields(map[string]string{fieldDueAt: apperr.KeyNotAllowed})
	}

	org, archived, found, err := m.Units.UnitState(ctx, call, input.UnitID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "maintenance.JobUnits.UnitState")
	}

	if !found || (call.Principal.Role == roleClient && org != call.Principal.OrgID) {
		return uuid.Nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if archived {
		return uuid.Nil, apperr.Fields(map[string]string{fieldUnitID: apperr.KeyUnitArchived})
	}

	return org, m.checkInstallation(ctx, call, input)
}

// checkInstallation applies DEC-106 to a new job: an installation job is HQ's, for a planned unit and the only
// unfinished one of the unit, and a planned unit has no other kind of job.
func (m Jobs) checkInstallation(ctx context.Context, call *ops.Call, input *CreateInput) error {
	state, _, err := m.Units.UnitInstall(ctx, call, input.UnitID)
	if err != nil {
		return apperr.Wrap(err, "maintenance.JobUnits.UnitInstall")
	}

	planned := state == installPlanned

	switch {
	case input.Type == jobInstallation && call.Principal.Role == roleClient:
		return apperr.Fields(map[string]string{"type": apperr.KeyNotAllowed})
	case input.Type == jobInstallation && !planned, input.Type != jobInstallation && planned:
		return apperr.Fields(map[string]string{"type": apperr.KeyInvalidState})
	case input.Type != jobInstallation:
		return nil
	}

	var open bool

	err = call.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM maintenance.jobs WHERE unit_id = $1 AND type = 'installation'
			AND status NOT IN ('completed','cancelled'))`,
		input.UnitID,
	).Scan(&open)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.jobs")
	}

	if open {
		return apperr.E(apperr.Conflict, apperr.KeyConflict)
	}

	return nil
}

// requestedSlots answers the job's preferred slots, the requested slot and its alternatives (IR113 item 2: a client
// gives exactly two alternatives, HQ up to two), and its due time, the requested end unless HQ set one.
func requestedSlots(call *ops.Call, input *CreateInput) ([]Slot, time.Time, error) {
	slots := append([]Slot{{input.RequestedStart, input.RequestedEnd}}, input.AlternativeSlots...)

	alts := len(input.AlternativeSlots)
	if (call.Principal.Role == roleClient && alts != 2) || alts > 2 {
		return nil, time.Time{}, apperr.Fields(
			map[string]string{"alternativeSlots": apperr.KeyCount},
		)
	}

	if !PreferredSlotsOK(call.Now, slots) {
		return nil, time.Time{}, apperr.Fields(
			map[string]string{"alternativeSlots": apperr.KeySlotRules},
		)
	}

	due := input.RequestedEnd
	if input.DueAt != nil {
		if input.DueAt.Before(input.RequestedEnd) {
			return nil, time.Time{}, apperr.Fields(
				map[string]string{fieldDueAt: "error.beforeRequestedEnd"},
			)
		}

		due = *input.DueAt
	}

	return slots, due, nil
}

// insertRequest stores a requested job of the client's request and answers its ID.
func insertRequest(
	ctx context.Context,
	call *ops.Call,
	input *CreateInput,
	org uuid.UUID,
	slots []Slot,
	due time.Time,
) (uuid.UUID, error) {
	var contactWindow *string
	if input.ContactWindow != nil && *input.ContactWindow != "" {
		contactWindow = input.ContactWindow
	}

	pref, err := json.Marshal(slots)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "json.Marshal")
	}

	var jobID uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin,
		symptom, contact_window, requested_slot,
		preferred_slots, preference_round, due_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'requested',
		CASE WHEN $3 = 'installation' THEN 'hq_request' ELSE 'client_request' END, $4, $5,
		tstzrange($6, $7), $8, 1, $9) RETURNING
		id`, input.UnitID, org, input.Type, input.Symptom, contactWindow, input.RequestedStart, input.RequestedEnd, pref,
		due).
		Scan(&jobID)

	return jobID, apperr.Wrap(err, "insert maintenance.jobs")
}

func (Jobs) event(
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
		return map[string]string{fieldJobID: apperr.KeyRequired}
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
	return m.load(ctx, c, id, sqlTrue, []any{id})
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

	job.Projection = projectionDetail

	err = m.detailExtras(ctx, call, jobID, &job)
	if err != nil {
		return job, err
	}

	err = m.project(ctx, call, &job)

	return job, err
}

// detailExtras reads what JobDetail adds to the job's row: the slot proposal, the assignment, the contractor's time
// change, the event cursor, the problem photos and the report refs.
func (m Jobs) detailExtras(ctx context.Context, call *ops.Call, jobID uuid.UUID, job *Job) error {
	proposal, found, err := m.pendingOrLastProposal(ctx, call, jobID)
	if err != nil {
		return err
	}

	if found {
		job.SlotProposal = &proposal
	}

	assignment, found, err := m.shownAssignment(ctx, call, jobID)
	if err != nil {
		return err
	}

	if found {
		job.Assignment = &assignment
	}

	err = partnerProposalOf(ctx, call, jobID, job)
	if err != nil {
		return err
	}

	job.EventCursor, err = eventCursor(ctx, call, jobID)
	if err != nil {
		return err
	}

	job.ProblemPhotos, err = problemPhotos(ctx, call, jobID)
	if err != nil {
		return err
	}

	return reportRefs(ctx, call, jobID, job)
}

// partnerProposalOf sets the job's latest time-change proposal of a contractor, withdrawn ones excluded, in the detail
// of HQ and the contractor; a client and a technician read null.
func partnerProposalOf(ctx context.Context, call *ops.Call, jobID uuid.UUID, job *Job) error {
	var partnerSlotProposal PartnerSlotProposal

	err := call.Tx.QueryRow(ctx, `SELECT p.id, p.offer_id, lower(p.slot), upper(p.slot), p.technician_membership_id,
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
	case err == nil && call.Principal.Role != roleClient && call.Principal.Role != roleTechnician:
		job.PartnerSlotProposal, err = json.Marshal(partnerSlotProposal)

		return apperr.Wrap(err, "json.Marshal")
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return apperr.Wrap(err, "select maintenance.partner_slot_proposals")
	}

	return nil
}

// eventCursor counts the job's events the caller reads with jobs.events.
func eventCursor(ctx context.Context, call *ops.Call, jobID uuid.UUID) (int, error) {
	cursorSQL := `SELECT count(*) FROM maintenance.job_events e LEFT JOIN maintenance.job_notes n ON n.id = e.note_id WHERE
		e.job_id = $1`
	if call.Principal.Role == roleClient {
		// the cursor counts what the client's jobs.events lists, so it reveals no hidden event (IR42, IR442)
		cursorSQL += clientEventFilter
	}

	var cursor int

	err := call.Tx.QueryRow(ctx, cursorSQL, jobID).Scan(&cursor)

	return cursor, apperr.Wrap(err, "queryrow (load)")
}

// reportRefs sets the job's work report versions as its draft and its frozen report refs; a client reads neither the
// draft nor a version that was not accepted (IR42).
func reportRefs(ctx context.Context, call *ops.Call, jobID uuid.UUID, job *Job) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, version, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY version`,
		jobID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.work_reports")
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

			return apperr.Wrap(err, "select maintenance.work_reports")
		}

		ref := map[string]any{"reportId": rid, fieldReportVersion: version}

		switch {
		case state == reportDraft:
			if call.Principal.Role != roleClient { // IR42
				job.DraftReportRef = ref
			}
		case call.Principal.Role != roleClient || state == reportAccepted:
			job.ReportRefs = append(job.ReportRefs, ref)
		}
	}

	rows.Close()

	return apperr.Wrap(rows.Err(), "select maintenance.work_reports")
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
	CandidateSlots   []Slot     `json:"candidateSlots"` // the client's agreed times; the contractor picks one (IR636)
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
func (Jobs) project(ctx context.Context, call *ops.Call, job *Job) error {
	role := call.Principal.Role
	if role == roleAdmin || role == roleContractor {
		err := currentOffer(ctx, call, job)
		if err != nil {
			return err
		}
	}

	switch role {
	case roleClient:
		return clientProjection(ctx, call, job)
	case roleTechnician, roleContractor:
		job.Costs = json.RawMessage("[]")
		if role == roleTechnician && job.Assignment != nil &&
			job.Assignment.TechnicianMembershipID != call.Principal.MembershipID {
			job.Assignment.Reason = nil
		}
	}

	return nil
}

// currentOffer sets the job's current Offer, an open or the accepted one (IR127 item 7): any for HQ, its own without
// the decline reason for a contractor.
func currentOffer(ctx context.Context, call *ops.Call, job *Job) error {
	role := call.Principal.Role
	stmt := `SELECT id, tenant_id, version, created_at, updated_at, job_id, contractor_org_id, terms_version,
			candidate_slots, offered_at,
			offer_expires_at, access_valid_from, access_valid_until, decision, decided_by, decided_at, decline_reason
			FROM maintenance.offers WHERE job_id = $1 AND ((decision IS NULL AND expired_at IS NULL) OR decision = 'accept')`
	args := []any{job.ID}

	if role == roleContractor {
		stmt += " AND contractor_org_id = $2"

		args = append(args, call.Principal.OrgID)
	}

	var offerView OfferView

	err := call.Tx.QueryRow(ctx, stmt+" ORDER BY offered_at DESC LIMIT 1", args...).
		Scan(&offerView.ID, &offerView.TenantID, &offerView.Version, &offerView.CreatedAt, &offerView.UpdatedAt,
			&offerView.JobID,
			&offerView.ContractorOrgID, &offerView.TermsVersion, &offerView.CandidateSlots,
			&offerView.OfferedAt, &offerView.OfferExpiresAt,
			&offerView.AccessValidFrom, &offerView.AccessValidUntil,
			&offerView.Decision, &offerView.DecidedBy, &offerView.DecidedAt, &offerView.DeclineReason)
	switch {
	case err == nil:
		if role == roleContractor {
			offerView.DeclineReason = nil
		}

		job.Offer = offerView
	case !errors.Is(err, pgx.ErrNoRows):
		return apperr.Wrap(err, "queryrow (project)")
	}

	return nil
}

// clientProjection applies IR42 to a client's JobDetail: the assignment without its reason and, once the technician
// accepted it, with their name; the cost lines the customer may see.
func clientProjection(ctx context.Context, call *ops.Call, job *Job) error {
	if job.Assignment != nil {
		job.Assignment.Reason = nil
		if job.Assignment.Acknowledgement == ackAccepted {
			err := technicianName(ctx, call, job.Assignment)
			if err != nil {
				return err
			}
		}
	}

	var lines []map[string]any

	_ = json.Unmarshal(job.Costs, &lines)
	kept := []map[string]any{}

	for _, l := range lines {
		if l["visibility"] == visibilityCustomer {
			kept = append(kept, l)
		}
	}

	costs, err := json.Marshal(kept)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	job.Costs = costs

	return nil
}

// technicianName sets the display name of the assignment's technician (DD-C09, IR237).
func technicianName(ctx context.Context, call *ops.Call, assignment *Assignment) error {
	var user uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT user_id FROM maintenance.ref_memberships WHERE id =
					$1`, assignment.TechnicianMembershipID).
		Scan(&user)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return apperr.Wrap(err, "select maintenance.ref_memberships")
	}

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

	err = identity.LoadMembers(ctx, call, members)
	if err != nil {
		return apperr.Wrap(err, "identity.LoadMembers")
	}

	if members[0].DisplayName != "" {
		assignment.TechnicianName = &members[0].DisplayName
	}

	return nil
}

// pendingOrLastProposal is the job's pending slot proposal, or else its latest one; found is false for a job without
// one.
func (Jobs) pendingOrLastProposal(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (SlotProposal, bool, error) {
	var slotProposal SlotProposal

	err := c.Tx.QueryRow(ctx, `SELECT id, source, lower(slot), upper(slot), hold, message, reply_by, status, decided_at,
		decline_reason, decline_comment
		FROM maintenance.slot_proposals WHERE job_id = $1 ORDER BY (status = 'pending') DESC, created_at DESC, id LIMIT
		1`, job).
		Scan(&slotProposal.ID, &slotProposal.Source, &slotProposal.Slot.StartAt, &slotProposal.Slot.EndAt, &slotProposal.Hold,
			&slotProposal.Message, &slotProposal.ReplyBy, &slotProposal.Status, &slotProposal.DecidedAt,
			&slotProposal.DeclineReason, &slotProposal.DeclineComment)
	if errors.Is(err, pgx.ErrNoRows) {
		return slotProposal, false, nil
	}

	return slotProposal, true, apperr.Wrap(err, "select maintenance.slot_proposals")
}

// activeAssignment is the job's active Assignment; found is false when it has none.
func (m Jobs) activeAssignment(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (Assignment, bool, error) {
	return m.assignmentWhere(ctx, c, job, "status = 'active'")
}

// shownAssignment is the job's Assignment for its projections: the active one, or for a completed job the one its
// completion released (IR234), so the detail still says who did the work.
func (m Jobs) shownAssignment(
	ctx context.Context,
	c *ops.Call,
	job uuid.UUID,
) (Assignment, bool, error) {
	return m.assignmentWhere(
		ctx,
		c,
		job,
		"status IN ('active', 'completed') ORDER BY status = 'active' DESC, updated_at DESC LIMIT 1",
	)
}

// assignmentWhere is the job's Assignment that meets cond; found is false when none does.
func (Jobs) assignmentWhere(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	cond string,
) (Assignment, bool, error) {
	var (
		loaded           Assignment
		altStart, altEnd *time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, tenant_id, version, created_at, updated_at, job_id, technician_membership_id,
		valid_from, valid_until,
		lower(scheduled), upper(scheduled), status, reason, acknowledgement, acknowledged_at, cant_make_reason,
		lower(alternative_slot), upper(alternative_slot)
		FROM maintenance.assignments WHERE job_id = $1 AND `+cond, job).
		Scan(&loaded.ID, &loaded.TenantID, &loaded.Version, &loaded.CreatedAt, &loaded.UpdatedAt, &loaded.JobID,
			&loaded.TechnicianMembershipID, &loaded.ValidFrom,
			&loaded.ValidUntil,
			&loaded.ScheduledStart, &loaded.ScheduledEnd, &loaded.Status, &loaded.Reason, &loaded.Acknowledgement,
			&loaded.AcknowledgedAt, &loaded.CantMakeReason,
			&altStart, &altEnd)
	if errors.Is(err, pgx.ErrNoRows) {
		return loaded, false, nil
	}

	if altStart != nil && altEnd != nil {
		loaded.AlternativeSlot = &Slot{*altStart, *altEnd}
	}

	return loaded, true, apperr.Wrap(err, "select maintenance.assignments")
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
		return map[string]string{fieldAttachmentID: apperr.KeyRequired}
	}

	return nil
}

// getAttachment answers a problem photo of a job to whoever may read the job in its detail projection (IR437, IR755).
//
//	@Summary		jobs.getAttachment (read)
//	@ID				jobs.getAttachment
//	@Description	Authorization: client:self | contractor:offer-projection-or-delegated-history | \
//	@Description	technician:assigned-history | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; a problem photo of \
//	@Description	a follow-up job (IR437) for whoever reads the job in its detail projection (IR755)
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
	// only the detail projection lists the photos: a contractor's offer or history projection, or a technician's
	// history, ends their sight of them with the access window (IR755)
	_, detailErr := m.detail(ctx, call, input.JobID)
	if detailErr != nil {
		return BlobOut{}, detailErr
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
//	@Description	DD-T10, DD-C17, DD-T13, DD-P10, DD-P03, DD-T02, DD-T07, DD-T16
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
	if projectedCaller(call) {
		v, _, err := m.projection(ctx, call, input.JobID)

		return v, err
	}

	return m.detail(ctx, call, input.JobID)
}

// ---- jobs.list ----

// sortDesc is the descending direction of a sort spec.
const sortDesc = "desc"

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

// The job statuses (MaintenanceJob.status).
const (
	statusRequested       = "requested"
	statusOffered         = "offered"
	statusAccepted        = "accepted"
	statusAssigned        = "assigned"
	statusInProgress      = "in_progress"
	statusOnHold          = "on_hold"
	statusSubmitted       = "submitted"
	statusReworkRequested = "rework_requested"
	statusCompleted       = "completed"
	statusCancelled       = "cancelled"
)

// jobStatuses lists the job statuses in the order of their status rank (IR23).
func jobStatuses() []string {
	return []string{
		statusRequested,
		statusOffered,
		statusAccepted,
		statusAssigned,
		statusInProgress,
		statusOnHold,
		statusSubmitted,
		statusReworkRequested,
		statusCompleted,
		statusCancelled,
	}
}

const statusRank = `array_position(ARRAY['requested','offered','accepted','assigned','in_progress','on_hold',` +
	`'submitted','rework_requested','completed','cancelled'], j.status)`

// The severities of a job (IR23) and of a notice.
const (
	severityNormal   = "normal"
	severityWarning  = "warning"
	severityCritical = "critical"
)

// severityRank orders the severities for the severity sort: normal, warning, critical.
func severityRank() map[string]int {
	rank := map[string]int{}
	for i, severity := range []string{severityNormal, severityWarning, severityCritical} {
		rank[severity] = i
	}

	return rank
}

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

	if !decodeFilters(query.Filters, &filters) {
		return paging.Page[any]{}, invalidFilters()
	}

	err := filters.check()
	if err != nil {
		return paging.Page[any]{}, err
	}

	sortField, desc, err := listSort(query.Sort)
	if err != nil {
		return paging.Page[any]{}, err
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

	items, err := withEventSummaries(ctx, call, page)
	if err != nil {
		return paging.Page[any]{}, err
	}

	return paging.Page[any]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// decodeFilters reads a query's filters into dst, refusing unknown fields; absent filters leave dst as it is. It
// reports whether the filters decoded.
func decodeFilters(raw json.RawMessage, dst any) bool {
	if len(raw) == 0 {
		return true
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	return dec.Decode(dst) == nil
}

// invalidFilters is VALIDATION on a query's filters.
func invalidFilters() error { return apperr.Fields(map[string]string{fieldFilters: apperr.KeyInvalid}) }

// listSort is the field and the direction of the jobs.list sort: status ascending unless the query sorts by id,
// severity, dueAt or status.
func listSort(sort *paging.SortSpec) (string, bool, error) {
	if sort == nil {
		return fieldStatus, false, nil
	}

	if !slices.Contains([]string{"id", "severity", fieldDueAt, fieldStatus}, sort.Field) {
		return "", false, apperr.Fields(map[string]string{"sort.field": apperr.KeyInvalid})
	}

	return sort.Field, sort.Direction == sortDesc, nil
}

// withEventSummaries answers the page's items with the event summary of every job summary, read in one query (IR506).
func withEventSummaries(ctx context.Context, call *ops.Call, page []listItem) ([]any, error) {
	var jobs []uuid.UUID

	for _, x := range page {
		if s, ok := x.item.(Summary); ok {
			jobs = append(jobs, s.ID)
		}
	}

	sums, err := eventSummaries(ctx, call, jobs) // the page's event counts in one query (IR506)
	if err != nil {
		return nil, err
	}

	items := []any{}

	for _, entry := range page {
		if s, ok := entry.item.(Summary); ok {
			s.EventSummary = sums[s.ID]
			entry.item = s
		}

		items = append(items, entry.item)
	}

	return items, nil
}

// invalidFilter is VALIDATION on one field of a query's filters.
func invalidFilter(field string) error {
	return apperr.Fields(map[string]string{"filters." + field: apperr.KeyInvalid})
}

// check validates the shared job filters (IR26), the first invalid one in the order of these checks.
func (f *listFilters) check() error {
	err := f.checkStatus()
	if err == nil {
		err = f.checkKinds()
	}

	if err != nil {
		return err
	}

	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return apperr.Fields(map[string]string{"filters.to": apperr.KeyRange})
	}

	if f.Statuses != nil {
		for _, s := range *f.Statuses {
			if !slices.Contains(jobStatuses(), s) {
				return invalidFilter("statuses")
			}
		}
	}

	return nil
}

// checkStatus validates the status filters: a job status, or a non-empty list of them, but not both.
func (f *listFilters) checkStatus() error {
	switch {
	case f.Status != nil && f.Statuses != nil:
		return invalidFilter("statuses")
	case f.Status != nil && !slices.Contains(jobStatuses(), *f.Status):
		return invalidFilter(fieldStatus)
	case f.Statuses != nil && len(*f.Statuses) == 0:
		return invalidFilter("statuses")
	}

	return nil
}

// checkKinds validates the severity, origin and type filters.
func (f *listFilters) checkKinds() error {
	switch {
	case f.Severity != nil && severityRank()[*f.Severity] == 0 && *f.Severity != severityNormal:
		return invalidFilter("severity")
	case f.Origin != nil && *f.Origin != "client_request" && *f.Origin != "periodic_plan":
		return invalidFilter("origin")
	case f.Type != nil && !jobTypes()[*f.Type]:
		return invalidFilter("type")
	}

	return nil
}

// collect returns the projected job rows matching the filters for the caller (shared by jobs.list and
// summaries.get, IR26).
func (m Jobs) collect(ctx context.Context, call *ops.Call, fp *listFilters) ([]listItem, error) {
	filters := *fp

	where, err := m.listConds(ctx, call, &filters)
	if err != nil {
		return nil, err
	}

	all, units, err := querySummaries(ctx, call, where)
	if err != nil {
		return nil, err
	}

	sev, err := m.Severity.UnitSeverities(ctx, call, units)
	if err != nil {
		return nil, apperr.Wrap(err, "maintenance.UnitSeverity.UnitSeverities")
	}

	merged := summaryItems(all, sev, filters.Severity)

	if projectedCaller(call) {
		extra, err := m.extraProjections(ctx, call, &filters)
		if err != nil {
			return nil, err
		}

		merged = append(merged, extra...)
	}

	return merged, nil
}

// projectedCaller reports a contractor or a technician: a caller whose job reads answer the IR23 projections.
func projectedCaller(call *ops.Call) bool {
	return call.Principal.Role == roleContractor || call.Principal.Role == roleTechnician
}

// jobConds are the conditions of a job query on maintenance.jobs j and their arguments, with the placeholders numbered
// in the order the conditions are added.
type jobConds struct {
	conds []string
	args  []any
}

// arg adds an argument and answers its placeholder.
func (w *jobConds) arg(v any) string {
	w.args = append(w.args, v)

	return fmt.Sprintf("$%d", len(w.args))
}

// and adds a condition.
func (w *jobConds) and(cond string) { w.conds = append(w.conds, cond) }

// slotFilters adds the conditions of the unit, status and requested slot filters.
func (w *jobConds) slotFilters(filters *listFilters) {
	if filters.UnitID != nil {
		w.and("j.unit_id = " + w.arg(*filters.UnitID))
	}

	if filters.UnitIDs != nil {
		w.and("j.unit_id = ANY(" + w.arg(*filters.UnitIDs) + ")")
	}

	if filters.Status != nil {
		w.and("j.status = " + w.arg(*filters.Status))
	}

	if filters.Statuses != nil {
		w.and("j.status = ANY(" + w.arg(*filters.Statuses) + ")")
	}

	if filters.From != nil {
		w.and("lower(j.requested_slot) >= " + w.arg(*filters.From))
	}

	if filters.To != nil {
		w.and("lower(j.requested_slot) < " + w.arg(*filters.To))
	}
}

// stateFilters adds the conditions of the overdue, origin, type and pending proposal filters.
func (w *jobConds) stateFilters(call *ops.Call, filters *listFilters) {
	if filters.OverdueOnly != nil && *filters.OverdueOnly {
		w.and("j.due_at < " + w.arg(call.Now) + " AND j.status NOT IN ('completed','cancelled')")
	}

	if filters.Origin != nil {
		w.and("j.origin = " + w.arg(*filters.Origin))
	}

	if filters.Type != nil {
		w.and("j.type = " + w.arg(*filters.Type))
	}

	if filters.ProposalPending != nil {
		q := "EXISTS (SELECT 1 FROM maintenance.slot_proposals p WHERE p.job_id = j.id AND p.status = 'pending')"
		if !*filters.ProposalPending {
			q = "NOT " + q
		}

		w.and(q)
	}
}

// listConds are the conditions of jobs.list: the caller's scope, then the filters in their catalog order.
func (m Jobs) listConds(
	ctx context.Context,
	call *ops.Call,
	filters *listFilters,
) (*jobConds, error) {
	where := &jobConds{conds: nil, args: nil}
	where.and(jobScope(call, &where.args))
	where.slotFilters(filters)

	if filters.OrganizationID != nil {
		if call.Principal.Role == roleAdmin && m.HQOrg != nil &&
			*filters.OrganizationID == m.HQOrg(call) {
			where.and("j.contractor_org_id IS NULL")
		} else {
			where.and("j.contractor_org_id = " + where.arg(*filters.OrganizationID))
		}
	}

	if filters.MembershipID != nil {
		where.and(
			"EXISTS (SELECT 1 FROM maintenance.assignments a WHERE a.job_id = j.id AND a.status IN ('active', 'completed') " +
				"AND a.technician_membership_id = " + where.arg(
				*filters.MembershipID,
			) + ")",
		)
	}

	err := m.customerFilters(ctx, call, filters, where)
	if err != nil {
		return nil, err
	}

	where.stateFilters(call, filters)

	return where, nil
}

// customerFilters adds the conditions of the customer and property filters, which Assets resolves to an organization
// and to units.
func (m Jobs) customerFilters(
	ctx context.Context,
	call *ops.Call,
	filters *listFilters,
	where *jobConds,
) error {
	if filters.CustomerID != nil {
		org, ok, err := m.Units.OrgOfCustomer(ctx, call, *filters.CustomerID)
		if err != nil {
			return apperr.Wrap(err, "maintenance.JobUnits.OrgOfCustomer")
		}

		if !ok {
			org = uuid.Nil
		}

		where.and("j.customer_org_id = " + where.arg(org))
	}

	if filters.PropertyID != nil {
		units, err := m.Units.UnitsOfProperty(ctx, call, *filters.PropertyID)
		if err != nil {
			return apperr.Wrap(err, "maintenance.JobUnits.UnitsOfProperty")
		}

		where.and("j.unit_id = ANY(" + where.arg(units) + ")")
	}

	return nil
}

// summaryRow is a JobSummary row with its status rank.
type summaryRow struct {
	s    Summary
	rank int
}

// querySummaries reads the job summaries that meet the conditions, and their units.
func querySummaries(
	ctx context.Context,
	call *ops.Call,
	where *jobConds,
) ([]summaryRow, []uuid.UUID, error) {
	conds, args := where.conds, where.args

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
		return nil, nil, apperr.Wrap(err, "select maintenance.jobs")
	}

	var (
		all   []summaryRow
		units []uuid.UUID
	)

	for rows.Next() {
		scanned, err := scanSummary(rows)
		if err != nil {
			rows.Close()

			return nil, nil, err
		}

		all, units = append(all, scanned), append(units, scanned.s.UnitID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, nil, apperr.Wrap(rowsErr, "select maintenance.jobs")
	}

	return all, units, nil
}

// scanSummary reads a job summary row of querySummaries; a pending slot proposal shows as time_proposed.
func scanSummary(rows pgx.Rows) (summaryRow, error) {
	var (
		scanned              summaryRow
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
		return scanned, apperr.Wrap(err, "select maintenance.jobs")
	}

	if schedStart != nil && schedEnd != nil {
		scanned.s.ScheduledSlot = &Slot{*schedStart, *schedEnd}
	}

	scanned.s.PreferredSlots = []Slot{}
	_ = json.Unmarshal(pref, &scanned.s.PreferredSlots)

	scanned.s.Projection, scanned.s.IsDemo, scanned.s.DisplayStatus = projectionSummary, true, scanned.s.Status
	if pending {
		scanned.s.DisplayStatus = "time_proposed"
	}

	return scanned, nil
}

// summaryItems are the job summaries as list items, each with its unit's severity (normal without an open alert),
// those of the severity filter only.
func summaryItems(all []summaryRow, sev map[uuid.UUID]string, severity *string) []listItem {
	ranks := severityRank()

	var merged []listItem

	for _, entry := range all {
		entry.s.Severity = sev[entry.s.UnitID]
		if entry.s.Severity == "" {
			entry.s.Severity = severityNormal
		}

		if severity == nil || entry.s.Severity == *severity {
			due := entry.s.DueAt
			merged = append(
				merged,
				listItem{
					rank: entry.rank,
					sev:  ranks[entry.s.Severity],
					due:  &due,
					id:   entry.s.ID.String(),
					item: entry.s,
				},
			)
		}
	}

	return merged
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
		return map[string]string{fieldJobID: apperr.KeyRequired}
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
const clientHiddenActions = "'job.costs_saved','job.warranty_claim_filed','partner_proposal.sent'," +
	"'partner_proposal.withdrawn','partner_proposal.kept'"

// clientEventCond is the condition on job_events e (joined to job_notes n) of what a client may see: no internal note
// (IR42) and no hidden action. clientEventFilter appends it to a WHERE clause.
const clientEventCond = "(n.id IS NULL OR n.visibility = 'customer') AND e.action NOT IN (" + clientHiddenActions + ")"

const clientEventFilter = " AND " + clientEventCond

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
	if call.Principal.Role == roleClient {
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
	ownOnly, err := m.ownEventsOnly(ctx, call, input.JobID)
	if err != nil {
		return paging.Page[Event]{}, err
	}

	period, err := decodeEventPeriod(input.Query.Filters)
	if err != nil {
		return paging.Page[Event]{}, err
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
	}{input.JobID, period}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Event]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where, args := eventsWhere(call, input.JobID, ownOnly, period)

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

// ownEventsOnly checks that the caller reads the job, and reports whether it reads only the decision events of its own
// side: a contractor or a technician whose job is an offer or a history (IR124 item 5).
func (m Jobs) ownEventsOnly(ctx context.Context, call *ops.Call, jobID uuid.UUID) (bool, error) {
	if projectedCaller(call) {
		_, kind, err := m.projection(ctx, call, jobID)
		if err != nil {
			return false, err
		}

		return kind != projectionDetail, nil // offer / history: only the decision events of the caller's side (IR124 item 5)
	}

	_, err := m.detail(ctx, call, jobID)

	return false, err
}

// eventPeriod is the jobs.events filter: the events that occurred in [from, to).
type eventPeriod struct {
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
}

// decodeEventPeriod reads and checks the jobs.events filters.
func decodeEventPeriod(raw json.RawMessage) (eventPeriod, error) {
	var period eventPeriod

	if !decodeFilters(raw, &period) {
		return period, apperr.Fields(map[string]string{"query.filters": apperr.KeyInvalid})
	}

	if period.From != nil && period.To != nil && !period.From.Before(*period.To) {
		return period, apperr.Fields(map[string]string{"query.filters.to": apperr.KeyRange})
	}

	return period, nil
}

// eventsWhere is the jobs.events condition on job_events e and its arguments: the job's events the caller reads, in
// the period.
func eventsWhere(
	call *ops.Call,
	jobID uuid.UUID,
	ownOnly bool,
	period eventPeriod,
) (string, []any) {
	args := []any{jobID}

	where := "e.job_id = $1"
	if call.Principal.Role == roleClient {
		where += clientEventFilter
	}

	if ownOnly {
		// the decision events of the caller's side: a contractor's organization, as its history snapshot stores them (IR506)
		where += " AND e.action IN ('offer.accepted','offer.declined')"

		if call.Principal.Role == roleContractor {
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

	if period.From != nil {
		args = append(args, *period.From)
		where += fmt.Sprintf(" AND e.occurred_at >= $%d", len(args))
	}

	if period.To != nil {
		args = append(args, *period.To)
		where += fmt.Sprintf(" AND e.occurred_at < $%d", len(args))
	}

	return where, args
}

// eventRows loads JobEvents (with their notes) matching where.
func (Jobs) eventRows(
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
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, event)
	}

	return items, apperr.Wrap(rows.Err(), "select maintenance.job_events")
}

// scanEvent reads a JobEvent row of eventRows, with its report ref and its note.
func scanEvent(rows pgx.Rows) (Event, error) {
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
		return event, apperr.Wrap(err, "select maintenance.job_events")
	}

	event.Version, event.CreatedAt, event.UpdatedAt = 1, event.OccurredAt, event.OccurredAt
	if reportID != nil && reportVersion != nil {
		event.ReportRef = map[string]any{
			"reportId":         *reportID,
			fieldReportVersion: *reportVersion,
		}
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

	return event, nil
}

// ---- state changes ----

// lockJob loads a job in scope for update and checks the expected version.
func (Jobs) lockJob(ctx context.Context, call *ops.Call, id uuid.UUID) (string, error) {
	args := []any{id}

	var (
		status  string
		version int
	)

	err := call.Tx.QueryRow(ctx, "SELECT j.status, j.version FROM maintenance.jobs j WHERE j.id = $1 AND "+
		jobScope(call, &args)+
		forUpdate, args...).
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

// The longest free texts, in characters after trimming.
const (
	maxReasonLength       = 1000 // a reason, a note, a comment or a message
	maxNameLength         = 120  // the name of a signer, a certificate or a part
	maxCodeLength         = 64   // a certificate number, a registration number or a terms version
	maxNoteLength         = 2000 // a job note
	maxWorkTextLength     = 4000 // a work report's text
	maxUnavailabilityNote = 500  // an unavailability's note
	maxRateNoteLength     = 200  // a rate card line's note
)

// preferredSlotCount is how many preferred times a request or a slot proposal's decline names (IR113).
const preferredSlotCount = 3

func trimmedReason(s *string, maxLength int, key string, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > maxLength {
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
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	trimmedReason(&in.CancelReason, maxReasonLength, "cancelReason", fieldErrs)

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
			map[string]any{fieldStatus: job.Status, fieldUnitID: job.UnitID},
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
			map[string]any{fieldStatus: job.Status, fieldUnitID: job.UnitID},
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

	allowed := status == statusRequested
	if call.Principal.Role == roleAdmin {
		allowed = slices.Contains(
			[]string{
				statusRequested,
				statusOffered,
				statusAccepted,
				statusAssigned,
				statusOnHold,
				statusReworkRequested,
			},
			status,
		)
	}

	if !allowed {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET decision = 'decline', decided_by = $2, decided_at = $3, decline_reason =
			'job_cancelled',
		version = version + 1, updated_at = platform.app_now() WHERE job_id = $1 AND decision IS NULL`,
		input.JobID,
		call.Principal.UserID, // Offer.decidedBy is the deciding user (IR747)
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
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

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

	if status != statusInProgress && status != statusSubmitted {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyConflict)
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

	if status != statusOnHold {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyConflict)
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

// The visibilities of a job note and of a cost line: internal to HQ and the contractor, or shown to the customer.
const (
	visibilityInternal = "internal"
	visibilityCustomer = "customer"
)

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
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	trimmedReason(&in.Message, maxNoteLength, "message", fieldErrs)

	if in.Visibility != visibilityInternal && in.Visibility != visibilityCustomer {
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
	if call.Principal.Role == roleClient && input.Visibility != visibilityCustomer {
		return Note{}, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	status, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Note{}, err
	}

	if status == statusCompleted || status == statusCancelled {
		return Note{}, apperr.E(apperr.Conflict, apperr.KeyConflict)
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
	if len(input.JobIDs) > maxBatchJobs {
		return nil, apperr.Fields(map[string]string{"jobIds": apperr.KeyRange})
	}

	out := []uuid.UUID{}

	for _, jobID := range input.JobIDs {
		if projectedCaller(call) {
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
