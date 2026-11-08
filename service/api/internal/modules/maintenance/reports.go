package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ComponentGroup is one Unit.serviceScope group and its inspection components in order.
type ComponentGroup struct {
	Group string
	Keys  []string
}

// ComponentGroups lists the inspection components per Unit.serviceScope group in DD-T04–T06 order (IR100 / IR126).
func ComponentGroups() []ComponentGroup {
	return []ComponentGroup{
		{
			"indoor",
			[]string{
				"filter",
				"evaporator_coil",
				"blower_motor",
				"blower_fan",
				"drain_pipe",
				"drain_pan",
				"outlet",
				"louver",
			},
		},
		{"outdoor", []string{"condenser_coil", "compressor", "fan", "blade", "refrigerant_pipe"}},
		{"electrical", []string{"thermostat", "sensor", "capacitor", "contactor", "wiring"}},
	}
}

func groupOf(key string) string {
	for _, g := range ComponentGroups() {
		if slices.Contains(g.Keys, key) {
			return g.Group
		}
	}

	return ""
}

// Components returns the component keys of a unit's service scope in canonical order.
func Components(scope []string) []string {
	var out []string

	for _, g := range ComponentGroups() {
		if slices.Contains(scope, g.Group) {
			out = append(out, g.Keys...)
		}
	}

	return out
}

// metricUnit is the expected UnitSymbol of a Metric (IR126 item 2), "" for a metric that is not one.
func metricUnit(metric string) string {
	switch metric {
	case "temperature":
		return "°C"
	case "humidity", "airflow_drop":
		return "%"
	case "co2":
		return "ppm"
	case "pm25":
		return "µg/m³"
	case "power":
		return "kW"
	case "vibration":
		return "mm/s"
	case "refrigerant_pressure":
		return "kPa"
	case "compressor_cycles":
		return "cycles/h"
	case "heartbeat_gap":
		return "min"
	}

	return ""
}

// Item is InspectionItem.
type Item struct {
	ID             uuid.UUID   `json:"id"`
	ComponentGroup string      `json:"componentGroup"`
	ComponentKey   string      `json:"componentKey"`
	Result         *string     `json:"result"`
	Reason         *string     `json:"reason"`
	EvidenceIDs    []uuid.UUID `json:"evidenceIds"`
	AuthorID       uuid.UUID   `json:"authorId"`
	ObservedAt     time.Time   `json:"observedAt"`
}

// ReportMeasurement is a Measurement recorded in a report (origin inspection).
type ReportMeasurement struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenantId"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	BoundaryID    *string   `json:"boundaryId"`
	QualityReason *string   `json:"qualityReason"`
	RawUnit       *string   `json:"rawUnit"`
	UnitID        uuid.UUID `json:"unitId"`
	SensorID      uuid.UUID `json:"sensorId"`
	ComponentKey  string    `json:"componentKey"`
	Metric        string    `json:"metric"`
	Value         *float64  `json:"value"`
	Unit          string    `json:"unit"`
	ObservedAt    time.Time `json:"observedAt"`
	ReceivedAt    time.Time `json:"receivedAt"`
	Origin        string    `json:"origin"`
	Quality       string    `json:"quality"`
	IsDemo        bool      `json:"isDemo"`
	Sequence      int       `json:"sequence"`
	EventID       uuid.UUID `json:"eventId"`
}

// Part is Part of service-contracts.ts.
type Part struct {
	Name                 string     `json:"name"`
	Quantity             int        `json:"quantity"`
	CatalogCode          *string    `json:"catalogCode"`
	Source               string     `json:"source"`
	LotSerial            *string    `json:"lotSerial"`
	ReplacesComponentKey *string    `json:"replacesComponentKey"`
	OldPartDisposal      *string    `json:"oldPartDisposal"`
	ReceiptAttachmentID  *uuid.UUID `json:"receiptAttachmentId"`
}

// Refrigerant is RefrigerantRecord.
type Refrigerant struct {
	Refrigerant     string  `json:"refrigerant"`
	CylinderID      string  `json:"cylinderId"`
	RecoveredKg     float64 `json:"recoveredKg"`
	ChargedKg       float64 `json:"chargedKg"`
	LeakCheck       string  `json:"leakCheck"`
	LeakCheckMethod *string `json:"leakCheckMethod"`
}

// NextAction is NextAction.
type NextAction struct {
	Kind string     `json:"kind"`
	Date *time.Time `json:"date,omitempty"`
	Note *string    `json:"note,omitempty"`
}

// The kinds of a next action: none, or a follow-up with a date and a note.
const (
	nextActionNone     = "none"
	nextActionFollowUp = "follow_up"
)

// Review is Review.
type Review struct {
	ReviewerUserID uuid.UUID `json:"reviewerUserId"`
	ReportVersion  int       `json:"reportVersion"`
	Decision       string    `json:"decision"`
	Reason         *string   `json:"reason"`
	OccurredAt     time.Time `json:"occurredAt"`
}

// AttachmentRef is Attachment.
type AttachmentRef struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenantId"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	JobID     uuid.UUID `json:"jobId"`
	ReportID  uuid.UUID `json:"reportId"`
	BlobID    uuid.UUID `json:"blobId"`
	Name      string    `json:"name"`
	Mime      string    `json:"mime"`
	Size      int       `json:"size"`
	Status    string    `json:"status"`
}

// Report is WorkReport.
type Report struct {
	ID                 uuid.UUID           `json:"id"`
	TenantID           uuid.UUID           `json:"tenantId"`
	Version            int                 `json:"version"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
	ReviewAvailability map[string]any      `json:"reviewAvailability"`
	JobID              uuid.UUID           `json:"jobId"`
	AuthorID           uuid.UUID           `json:"authorId"`
	Items              []Item              `json:"items"`
	Measurements       []ReportMeasurement `json:"measurements"`
	Parts              []Part              `json:"parts"`
	Refrigerant        []Refrigerant       `json:"refrigerant"`
	SignOff            json.RawMessage     `json:"signOff"            swaggertype:"object"`
	WorkText           string              `json:"workText"`
	NextAction         *NextAction         `json:"nextAction"`
	AttachmentRefs     []AttachmentRef     `json:"attachmentRefs"`
	SubmittedAt        *time.Time          `json:"submittedAt"`
	AcceptedAt         *time.Time          `json:"acceptedAt"`
	ReviewHistory      []Review            `json:"reviewHistory"`
	State              string              `json:"-"`
}

// The states of a work report version: the draft being written, submitted for review, and accepted or returned by
// it.
const (
	reportDraft     = "draft"
	reportSubmitted = "submitted"
	reportAccepted  = "accepted"
	reportReturned  = "returned"
)

// resultNormal is the result of an inspection item without a finding: the only one that needs no reason.
const resultNormal = "normal"

// Reports is the work-report operation set (IR126).
type Reports struct {
	Jobs   Jobs
	Access Access
	Units  ServiceUnits
}

const reportCols = `id, tenant_id, version, created_at, updated_at, job_id, author_id, state, items, measurements, ` +
	`parts, refrigerant, sign_off, work_text,
	next_action, submitted_at, accepted_at`

func scanReport(row pgx.Row) (Report, error) {
	var (
		report                         Report
		items, meas, parts, refr, next []byte
	)

	err := row.Scan(
		&report.ID,
		&report.TenantID,
		&report.Version,
		&report.CreatedAt,
		&report.UpdatedAt,
		&report.JobID,
		&report.AuthorID,
		&report.State,
		&items,
		&meas,
		&parts,
		&refr,
		&report.SignOff,
		&report.WorkText,
		&next,
		&report.SubmittedAt,
		&report.AcceptedAt,
	)
	if err != nil {
		return report, apperr.Wrap(err, "scan (scanReport)")
	}

	report.Items = []Item{}
	report.Measurements = []ReportMeasurement{}
	report.Parts = []Part{}

	report.Refrigerant = []Refrigerant{}
	for _, p := range []struct {
		raw []byte
		dst any
	}{{items, &report.Items}, {meas, &report.Measurements}, {parts, &report.Parts}, {refr, &report.Refrigerant}} {
		if len(p.raw) > 0 {
			err := json.Unmarshal(p.raw, p.dst)
			if err != nil {
				return report, apperr.Wrap(err, "json.Unmarshal")
			}
		}
	}

	if len(next) > 0 && string(next) != jsonNull {
		report.NextAction = &NextAction{Kind: "", Date: nil, Note: nil}

		err := json.Unmarshal(next, report.NextAction)
		if err != nil {
			return report, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	if report.SignOff == nil {
		report.SignOff = json.RawMessage(jsonNull)
	}

	return report, nil
}

// decorate adds attachments, review history and the caller's review availability.
func (Reports) decorate(ctx context.Context, call *ops.Call, report *Report) error {
	err := attachmentRefs(ctx, call, report)
	if err != nil {
		return err
	}

	err = reviewHistory(ctx, call, report)
	if err != nil {
		return err
	}

	return reviewAvailability(ctx, call, report)
}

// attachmentRefs lists the report's attachments, the oldest first.
func attachmentRefs(ctx context.Context, call *ops.Call, report *Report) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, tenant_id, created_at, updated_at, job_id, name, mime, size_bytes, status FROM maintenance.attachments
		WHERE report_id = $1 ORDER BY created_at, id`,
		report.ID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.attachments")
	}

	report.AttachmentRefs = []AttachmentRef{}

	for rows.Next() {
		var attachmentRef AttachmentRef

		err := rows.Scan(
			&attachmentRef.ID,
			&attachmentRef.TenantID,
			&attachmentRef.CreatedAt,
			&attachmentRef.UpdatedAt,
			&attachmentRef.JobID,
			&attachmentRef.Name,
			&attachmentRef.Mime,
			&attachmentRef.Size,
			&attachmentRef.Status,
		)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select maintenance.attachments")
		}

		attachmentRef.Version, attachmentRef.ReportID, attachmentRef.BlobID = 1, report.ID, attachmentRef.ID
		report.AttachmentRefs = append(report.AttachmentRefs, attachmentRef)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select maintenance.attachments")
	}

	return nil
}

// reviewHistory lists the reviews of the report's versions in the order they happened.
func reviewHistory(ctx context.Context, call *ops.Call, report *Report) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT reviewer_user_id, report_version, decision, reason, occurred_at FROM maintenance.report_reviews WHERE
			report_id = $1 ORDER BY occurred_at, report_version`,
		report.ID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.report_reviews")
	}

	report.ReviewHistory = []Review{}

	for rows.Next() {
		var review Review

		err := rows.Scan(
			&review.ReviewerUserID,
			&review.ReportVersion,
			&review.Decision,
			&review.Reason,
			&review.OccurredAt,
		)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select maintenance.report_reviews")
		}

		report.ReviewHistory = append(report.ReviewHistory, review)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select maintenance.report_reviews")
	}

	return nil
}

// reviewAvailability says whether the caller may review the report, and why not: the permission, a report the caller
// wrote, a version that is not the latest, or a report that is not submitted.
func reviewAvailability(ctx context.Context, call *ops.Call, report *Report) error {
	var latest int

	scanErr := call.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, report.ID).
		Scan(&latest)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select maintenance.work_reports")
	}

	reason := any(nil)

	switch {
	case (call.Principal.Role != roleContractor || !call.Principal.Permissions["partner.review"]) &&
		(call.Principal.Role != roleAdmin || !call.Principal.Permissions["job.write"]):
		reason = "permission_denied"
	case report.AuthorID == call.Principal.UserID:
		reason = "self_authored"
	case report.Version != latest:
		reason = "not_current"
	case report.State != reportSubmitted:
		reason = "not_submitted"
	}

	report.ReviewAvailability = map[string]any{"allowed": reason == nil, fieldReason: reason}

	return nil
}

// ---- jobs.saveDraft ----

// ItemInput is InspectionItemInput.
type ItemInput struct {
	ComponentGroup string      `json:"componentGroup"`
	ComponentKey   string      `json:"componentKey"`
	Result         *string     `json:"result"`
	Reason         *string     `json:"reason"`
	EvidenceIDs    []uuid.UUID `json:"evidenceIds"`
}

// MeasurementInput is InspectionMeasurementInput.
type MeasurementInput struct {
	ID           *uuid.UUID `json:"id,omitempty"`
	ComponentKey string     `json:"componentKey"`
	Metric       string     `json:"metric"`
	Value        *float64   `json:"value"`
	Unit         string     `json:"unit"`
	ObservedAt   time.Time  `json:"observedAt"`
}

// DraftInput is WorkReportDraft.
type DraftInput struct {
	JobID         uuid.UUID          `json:"jobId"`
	ReportID      *uuid.UUID         `json:"reportId,omitempty"`
	Items         []ItemInput        `json:"items"`
	Measurements  []MeasurementInput `json:"measurements"`
	Parts         []Part             `json:"parts"`
	Refrigerant   []Refrigerant      `json:"refrigerant"`
	WorkText      string             `json:"workText"`
	NextAction    *NextAction        `json:"nextAction"`
	AttachmentIDs []uuid.UUID        `json:"attachmentIds"`
}

// validItemResult reports whether value is an inspection item's result.
func validItemResult(value string) bool {
	switch value {
	case resultNormal, "attention", "not_inspected", "not_applicable":
		return true
	}

	return false
}

// validPartSource reports whether value is where a part came from.
func validPartSource(value string) bool {
	switch value {
	case "van_stock", "hq_warehouse", "bought_locally":
		return true
	}

	return false
}

// validDisposal reports whether value is what happened to an old part.
func validDisposal(value string) bool {
	switch value {
	case "disposed_on_site", "returned":
		return true
	}

	return false
}

// validRefrigerant reports whether value is a refrigerant the reports record.
func validRefrigerant(value string) bool {
	switch value {
	case "R32", "R410A":
		return true
	}

	return false
}

// validLeakCheck reports whether value is a leak check's result.
func validLeakCheck(value string) bool {
	switch value {
	case "pass", "fail", "not_done":
		return true
	}

	return false
}

// validUnitSymbol reports whether value is the symbol of a measurement unit.
func validUnitSymbol(value string) bool {
	switch value {
	case "°C", "%", "ppm", "µg/m³", "kW", "mm/s", "kPa", "cycles/h", "min":
		return true
	}

	return false
}

// Validate implements ops.Validator (IR126 item 2, drafting rules).
func (in *DraftInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.Items == nil || in.Measurements == nil || in.Parts == nil || in.Refrigerant == nil ||
		in.AttachmentIDs == nil {
		fieldErrs["draft"] = apperr.KeyRequired

		return fieldErrs
	}

	seen := in.validateItems(fieldErrs)
	in.validateRecords(seen, fieldErrs)

	if utf8.RuneCountInString(in.WorkText) > maxWorkTextLength {
		fieldErrs[fieldWorkText] = apperr.KeyLength
	}

	validateNextAction(in.NextAction, fieldErrs)

	return fieldErrs
}

// validateItems checks the inspection items: known components in their own group, each once, with a result that is
// one and a reason within its length; it returns the components the draft covers.
func (in *DraftInput) validateItems(fieldErrs map[string]string) map[string]bool {
	seen := map[string]bool{}

	for i := range in.Items {
		itemInput := &in.Items[i]
		if groupOf(itemInput.ComponentKey) == "" ||
			groupOf(itemInput.ComponentKey) != itemInput.ComponentGroup ||
			seen[itemInput.ComponentKey] {
			fieldErrs[fieldItems] = apperr.KeyInvalid
		}

		seen[itemInput.ComponentKey] = true
		if itemInput.Result != nil && !validItemResult(*itemInput.Result) {
			fieldErrs[fieldItems] = apperr.KeyInvalid
		}

		if itemInput.Reason != nil {
			*itemInput.Reason = strings.TrimSpace(*itemInput.Reason)
			if utf8.RuneCountInString(*itemInput.Reason) > maxReasonLength {
				fieldErrs[fieldItems] = apperr.KeyLength
			}
		}

		if itemInput.EvidenceIDs == nil {
			itemInput.EvidenceIDs = []uuid.UUID{}
		}
	}

	return seen
}

// validateRecords checks the measurements (of a component the draft covers, with the metric's unit), the parts and the
// refrigerant records.
func (in *DraftInput) validateRecords(seen map[string]bool, fieldErrs map[string]string) {
	for _, x := range in.Measurements {
		if !seen[x.ComponentKey] || metricUnit(x.Metric) == "" || !validUnitSymbol(x.Unit) ||
			x.ObservedAt.IsZero() {
			fieldErrs["measurements"] = apperr.KeyInvalid
		}
	}

	for _, p := range in.Parts {
		n := utf8.RuneCountInString(strings.TrimSpace(p.Name))
		if n < 1 || n > maxNameLength || p.Quantity < 0 || p.Quantity > 999 ||
			!validPartSource(p.Source) ||
			(p.OldPartDisposal != nil && !validDisposal(*p.OldPartDisposal)) ||
			(p.ReplacesComponentKey != nil && groupOf(*p.ReplacesComponentKey) == "") {
			fieldErrs["parts"] = apperr.KeyInvalid
		}
	}

	for _, r := range in.Refrigerant {
		n := utf8.RuneCountInString(r.CylinderID)
		if !validRefrigerant(r.Refrigerant) || n < 1 || n > maxCodeLength || r.RecoveredKg < 0 ||
			r.ChargedKg < 0 ||
			!validLeakCheck(r.LeakCheck) {
			fieldErrs["refrigerant"] = apperr.KeyInvalid
		}
	}
}

// validateNextAction checks the next action: none, or a follow-up with its date and a note within its length.
func validateNextAction(nextAction *NextAction, fieldErrs map[string]string) {
	if nextAction == nil {
		return
	}

	switch nextAction.Kind {
	case nextActionNone:
		if nextAction.Date != nil || nextAction.Note != nil {
			fieldErrs[fieldNextAction] = apperr.KeyInvalid
		}
	case nextActionFollowUp:
		if nextAction.Date == nil || nextAction.Note == nil ||
			utf8.RuneCountInString(*nextAction.Note) > maxReasonLength {
			fieldErrs[fieldNextAction] = apperr.KeyInvalid
		}
	default:
		fieldErrs[fieldNextAction] = apperr.KeyInvalid
	}
}

// currentDraft reads the job's current draft and whether it has one.
func (Reports) currentDraft(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	lock bool,
) (Report, bool, error) {
	q := "SELECT " + reportCols + " FROM maintenance.work_reports WHERE job_id = $1 AND state = 'draft'"
	if lock {
		q += forUpdate
	}

	draft, err := scanReport(call.Tx.QueryRow(ctx, q, job))
	if errors.Is(err, pgx.ErrNoRows) {
		return draft, false, nil
	}

	return draft, err == nil, err
}

// saveDraft serves jobs.saveDraft.
//
//	@Summary		jobs.saveDraft (write)
//	@ID				jobs.saveDraft
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed); IR100 partial \
//	@Description	items allowed
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T04, DD-T05, DD-T06, DD-T09, DD-T14
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		false	"reportId omitted: omit (target none, read jobs.get); reportId present: required (target report, read reports.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		DraftInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Report}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/report-draft [put]
func (m Reports) saveDraft(ctx context.Context, call *ops.Call, input *DraftInput) (Report, error) {
	unit, err := m.draftJob(ctx, call, input.JobID)
	if err != nil {
		return Report{}, err
	}

	prev, err := m.previousDraft(ctx, call, input)
	if err != nil {
		return Report{}, err
	}

	allowed, err := reportAttachments(ctx, call, prev.ID)
	if err != nil {
		return Report{}, err
	}

	for _, id := range input.AttachmentIDs {
		if !allowed[id] {
			return Report{}, apperr.Fields(
				map[string]string{"attachmentIds": apperr.KeyOtherReportAttachment},
			)
		}
	}

	items, err := mergeItems(call, input, prev, allowed)
	if err != nil {
		return Report{}, err
	}

	meas, err := mergeMeasurements(call, input, prev, unit, items)
	if err != nil {
		return Report{}, err
	}

	err = writeDraft(ctx, call, input, prev, items, meas)
	if err != nil {
		return Report{}, err
	}

	report, err := m.load(ctx, call, prev.ID, prev.Version+1)
	if err != nil {
		return report, err
	}

	call.Emit(
		outboxEvent(
			"report",
			report.ID,
			"ReportDraftSaved",
			map[string]any{fieldJobID: input.JobID, "version": report.Version},
		),
	)
	call.Audit(
		auditEntry(
			"jobs.saveDraft",
			"report",
			report.ID,
			call.ExpectedVersion,
			&report.Version,
			"",
		),
	)

	return report, nil
}

// draftJob checks the job a draft is saved for: it exists, the caller is its technician now, and it is in progress.
// It returns the job's unit.
func (m Reports) draftJob(ctx context.Context, call *ops.Call, job uuid.UUID) (uuid.UUID, error) {
	var (
		status string
		unit   uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, job).
		Scan(&status, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return unit, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return unit, apperr.Wrap(err, "select maintenance.jobs")
	}

	technicianJobErr := m.Access.TechnicianJob(ctx, call, job, unit)
	if technicianJobErr != nil {
		return unit, technicianJobErr
	}

	if status != statusInProgress {
		return unit, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	return unit, nil
}

// previousDraft is the report the draft replaces: a new one when the input names none (the job has no report yet), or
// the job's current draft at the version shown.
func (m Reports) previousDraft(
	ctx context.Context,
	call *ops.Call,
	input *DraftInput,
) (Report, error) {
	draft, found, err := m.currentDraft(ctx, call, input.JobID, true)
	if err != nil {
		return draft, err
	}

	var prev Report

	switch {
	case input.ReportID == nil:
		var count int

		err := call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.work_reports WHERE job_id = $1`, input.JobID).
			Scan(&count)
		if err != nil {
			return Report{}, apperr.Wrap(err, "select maintenance.work_reports")
		}

		if count > 0 {
			return Report{}, apperr.E(apperr.Conflict, "errors.draft_exists")
		}

		prev = Report{
			ID:                 uuid.New(),
			Version:            0,
			TenantID:           uuid.UUID{},
			CreatedAt:          time.Time{},
			UpdatedAt:          time.Time{},
			ReviewAvailability: nil,
			JobID:              uuid.UUID{},
			AuthorID:           uuid.UUID{},
			Items:              nil,
			Measurements:       nil,
			Parts:              nil,
			Refrigerant:        nil,
			SignOff:            nil,
			WorkText:           "",
			NextAction:         nil,
			AttachmentRefs:     nil,
			SubmittedAt:        nil,
			AcceptedAt:         nil,
			ReviewHistory:      nil,
			State:              "",
		}
	case !found || draft.ID != *input.ReportID:
		return Report{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	case draft.Version != *call.ExpectedVersion:
		return Report{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	default:
		prev = draft
	}

	return prev, nil
}

// reportAttachments are the attachments of the report, the only ones its draft may name.
func reportAttachments(
	ctx context.Context,
	call *ops.Call,
	report uuid.UUID,
) (map[uuid.UUID]bool, error) {
	allowed := map[uuid.UUID]bool{}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT id FROM maintenance.attachments WHERE report_id = $1`,
		report,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.attachments")
	}

	for rows.Next() {
		var attachmentID uuid.UUID

		err := rows.Scan(&attachmentID)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select maintenance.attachments")
		}

		allowed[attachmentID] = true
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select maintenance.attachments")
	}

	return allowed, nil
}

// mergeItems are the draft's inspection items, merged with the previous report's: an item keeps its ID, and its
// author and time while its result, reason and evidence are unchanged (SR07); evidence must be the report's own
// attachments.
func mergeItems(
	call *ops.Call,
	input *DraftInput,
	prev Report,
	allowed map[uuid.UUID]bool,
) ([]Item, error) {
	old := map[string]Item{}
	for _, it := range prev.Items {
		old[it.ComponentKey] = it
	}

	items := []Item{}

	for _, req := range input.Items {
		for _, e := range req.EvidenceIDs {
			if !allowed[e] {
				return nil, apperr.Fields(
					map[string]string{fieldItems: apperr.KeyOtherReportAttachment},
				)
			}
		}

		item := Item{
			ID:             uuid.New(),
			ComponentGroup: req.ComponentGroup,
			ComponentKey:   req.ComponentKey,
			Result:         req.Result,
			Reason:         req.Reason,
			EvidenceIDs:    req.EvidenceIDs,
			AuthorID:       call.Principal.UserID,
			ObservedAt:     call.Now,
		}
		if item.Reason != nil && *item.Reason == "" {
			item.Reason = nil
		}

		if o, ok := old[req.ComponentKey]; ok {
			item.ID = o.ID
			if eqStr(o.Result, item.Result) && eqStr(o.Reason, item.Reason) &&
				slices.Equal(o.EvidenceIDs, item.EvidenceIDs) {
				item.AuthorID, item.ObservedAt = o.AuthorID, o.ObservedAt
			}
		}

		items = append(items, item)
	}

	return items, nil
}

// mergeMeasurements are the draft's measurements on the unit, each on its item's sensor, merged with the previous
// report's: a measurement the draft had
// keeps its ID and creation and moves its version on; a value that is no number is missing, and a unit other than the
// metric's makes it suspect.
func mergeMeasurements(
	call *ops.Call,
	input *DraftInput,
	prev Report,
	unit uuid.UUID,
	items []Item,
) ([]ReportMeasurement, error) {
	itemID := map[string]uuid.UUID{}
	for _, it := range items {
		itemID[it.ComponentKey] = it.ID
	}

	oldM := map[uuid.UUID]ReportMeasurement{}
	for _, x := range prev.Measurements {
		oldM[x.ID] = x
	}

	meas := []ReportMeasurement{}

	for idx, measurement := range input.Measurements {
		reportMeasurement := ReportMeasurement{
			ID:            uuid.New(),
			TenantID:      call.Principal.TenantID,
			Version:       1,
			CreatedAt:     call.Now,
			UpdatedAt:     call.Now,
			UnitID:        unit,
			SensorID:      itemID[measurement.ComponentKey],
			ComponentKey:  measurement.ComponentKey,
			Metric:        measurement.Metric,
			Value:         measurement.Value,
			Unit:          measurement.Unit,
			ObservedAt:    measurement.ObservedAt,
			ReceivedAt:    call.Now,
			Origin:        "inspection",
			Quality:       "valid",
			IsDemo:        true,
			Sequence:      idx + 1,
			BoundaryID:    nil,
			QualityReason: nil,
			RawUnit:       nil,
			EventID:       uuid.UUID{},
		}
		if measurement.ID != nil {
			o, ok := oldM[*measurement.ID]
			if !ok {
				return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
			}

			reportMeasurement.ID, reportMeasurement.CreatedAt, reportMeasurement.Version = o.ID, o.CreatedAt, o.Version+1
		}

		reportMeasurement.EventID = reportMeasurement.ID

		switch {
		case measurement.Value == nil || math.IsNaN(*measurement.Value) || math.IsInf(*measurement.Value, 0):
			reportMeasurement.Quality, reportMeasurement.Value = "missing", nil
		case metricUnit(measurement.Metric) != measurement.Unit:
			reason := "unit_mismatch"
			reportMeasurement.Quality = "suspect"
			reportMeasurement.QualityReason = &reason
			reportMeasurement.RawUnit = &measurement.Unit
		}

		meas = append(meas, reportMeasurement)
	}

	return meas, nil
}

// writeDraft stores the draft: a new report or the next version of the previous one, with its attachments, and the
// job's draft version.
func writeDraft(
	ctx context.Context,
	call *ops.Call,
	input *DraftInput,
	prev Report,
	items []Item,
	meas []ReportMeasurement,
) error {
	for i := range input.Parts {
		input.Parts[i].Name = strings.TrimSpace(input.Parts[i].Name)
	}

	// the report's JSON columns, encoded once for either statement
	var cols [4][]byte

	for i, v := range []any{items, meas, input.Parts, input.Refrigerant} {
		encoded, marshalErr := json.Marshal(v)
		if marshalErr != nil {
			return apperr.Wrap(marshalErr, "json.Marshal")
		}

		cols[i] = encoded
	}

	var next any

	if input.NextAction != nil {
		encoded, marshalErr := json.Marshal(input.NextAction)
		if marshalErr != nil {
			return apperr.Wrap(marshalErr, "json.Marshal")
		}

		next = encoded
	}

	var err error

	if input.ReportID == nil {
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items, measurements, parts,
				refrigerant, work_text,
			next_action, created_at, updated_at) VALUES ($1, 1, current_setting('app.tenant_id')::uuid, $2, $3, 'draft', $4, $5,
			$6, $7, $8, $9, $10, $10)`,
			prev.ID,
			input.JobID,
			call.Principal.UserID,
			cols[0],
			cols[1],
			cols[2],
			cols[3],
			input.WorkText,
			next,
			call.Now,
		)
	} else {
		_, err = call.Tx.Exec(
			ctx,
			`UPDATE maintenance.work_reports SET version = version + 1, items = $3, measurements = $4, parts = $5, refrigerant =
				$6, work_text = $7,
			next_action = $8, sign_off = NULL, updated_at = $9 WHERE id = $1 AND version = $2`,
			prev.ID,
			prev.Version,
			cols[0],
			cols[1],
			cols[2],
			cols[3],
			input.WorkText,
			next,
			call.Now,
		)
	}

	if err != nil {
		return apperr.Wrap(err, "update maintenance.work_reports")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.attachments SET report_id = $2 WHERE id = ANY($1)`,
		input.AttachmentIDs,
		prev.ID,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update maintenance.attachments")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET draft_report_version = $2 WHERE id = $1`,
		input.JobID,
		prev.Version+1,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update maintenance.jobs")
	}

	return nil
}

func eqStr(
	a, b *string,
) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (m Reports) load(
	ctx context.Context,
	call *ops.Call,
	reportID uuid.UUID,
	version int,
) (Report, error) {
	report, err := scanReport(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+reportCols+" FROM maintenance.work_reports WHERE id = $1 AND version = $2",
			reportID,
			version,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return report, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return report, err
	}

	err = m.decorate(ctx, call, &report)

	return report, err
}

// ---- jobs.submit ----

// SubmitInput is jobs.submit input.
type SubmitInput struct {
	JobID         uuid.UUID `json:"jobId"`
	ReportVersion int       `json:"reportVersion"`
}

// Validate implements ops.Validator.
func (in *SubmitInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ReportVersion < 1 {
		fieldErrs[fieldReportVersion] = apperr.KeyRequired
	}

	return fieldErrs
}

func (m Reports) lockJob(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
) (string, uuid.UUID, error) {
	var (
		status  string
		unit    uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, jobID).
		Scan(&status, &unit, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unit, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return "", unit, apperr.Wrap(err, "select maintenance.jobs")
	}

	if call.Principal.Role == roleTechnician {
		err := m.Access.TechnicianJob(ctx, call, jobID, unit)
		if err != nil {
			return "", unit, err
		}
	}

	if version != *call.ExpectedVersion {
		return "", unit, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return status, unit, nil
}

// submit serves jobs.submit.
//
//	@Summary		jobs.submit (write)
//	@ID				jobs.submit
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed); IR100 component \
//	@Description	set and report completeness
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T04, DD-T05, DD-T06, DD-T08, DD-T09 · Input versions: reportVersion=WorkReport.version
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;reports.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		SubmitInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/submit [post]
func (m Reports) submit(ctx context.Context, c *ops.Call, in *SubmitInput) (MaintenanceJob, error) {
	return written(m.submitJob(ctx, c, in))
}

// submitJob is submit answering the job with its detail extras.
func (m Reports) submitJob(ctx context.Context, call *ops.Call, input *SubmitInput) (Job, error) {
	status, unit, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != statusInProgress {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	current, found, err := m.currentDraft(ctx, call, input.JobID, true)
	if err != nil {
		return Job{}, err
	}

	draft := &current
	if !found || draft.Version != input.ReportVersion {
		return Job{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}

	decorateErr := m.decorate(ctx, call, draft)
	if decorateErr != nil {
		return Job{}, decorateErr
	}

	_, _, scope, err := m.Units.UnitService(ctx, call, unit)
	if err != nil {
		return Job{}, apperr.Wrap(err, "maintenance.ServiceUnits.UnitService")
	}

	if fe := submitErrors(draft, Components(scope), call.Now); len(fe) > 0 {
		return Job{}, apperr.Fields(fe)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.work_reports SET state = 'submitted', submitted_at = $3, updated_at = $3 WHERE id = $1 AND version
			= $2`,
		draft.ID,
		draft.Version,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.work_reports")
	}

	tos, err := closedTimeOnSite(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'submitted', time_on_site = $2, draft_report_version = NULL, version = version +
			1, updated_at = platform.app_now()
		WHERE id = $1`,
		input.JobID,
		tos,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	reportEventErr := m.reportEvent(
		ctx,
		call,
		input.JobID,
		"job.submitted",
		draft.ID,
		draft.Version,
	)
	if reportEventErr != nil {
		return Job{}, reportEventErr
	}

	return m.Jobs.finishScoped(ctx, call, input.JobID, "jobs.submit", "ReportSubmitted", "")
}

// closedTimeOnSite is the job's time on site closed at the submission: it finishes now, a pause still open ends with
// it (IR448), and the minutes on site leave out the pauses and the time the job was out of progress (IR449).
func closedTimeOnSite(ctx context.Context, call *ops.Call, job uuid.UUID) ([]byte, error) {
	var raw []byte

	scanErr := call.Tx.QueryRow(ctx, `SELECT time_on_site FROM maintenance.jobs WHERE id = $1`, job).
		Scan(&raw)
	if scanErr != nil {
		return nil, apperr.Wrap(scanErr, "select maintenance.jobs")
	}

	timeOnSite := TimeOnSite{
		Pauses:        []Pause{},
		ArrivedAt:     nil,
		CheckInMethod: nil,
		CheckInReason: nil,
		Distance:      nil,
		StartedAt:     nil,
		FinishedAt:    nil,
		OnSiteMinutes: nil,
	}
	if len(raw) > 0 && string(raw) != jsonNull {
		err := json.Unmarshal(raw, &timeOnSite)
		if err != nil {
			return nil, apperr.Wrap(err, "decode maintenance.jobs time_on_site")
		}
	}

	now := call.Now
	timeOnSite.FinishedAt = &now
	// a pause still open ends with the submission, so its time is not counted as on site (IR448)
	if n := len(timeOnSite.Pauses); n > 0 && timeOnSite.Pauses[n-1].To == nil {
		timeOnSite.Pauses[n-1].To = &now
	}

	if from := firstNonNil(timeOnSite.ArrivedAt, timeOnSite.StartedAt); from != nil {
		off, err := offWork(ctx, call, job, *from, timeOnSite.Pauses)
		if err != nil {
			return nil, err
		}

		mins := int(workedFor(*from, now, off) / time.Minute)
		timeOnSite.OnSiteMinutes = &mins
	}

	tos, marshalErr := json.Marshal(timeOnSite)

	return tos, apperr.Wrap(marshalErr, "json.Marshal")
}

// offWork lists the intervals since from when the technician was not working on the job (IR449): the pauses and the
// periods the job left in_progress — held by HQ (job.held → job.resumed) or submitted and sent back (job.submitted →
// job.rework_started, or held and resumed).
func offWork(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	from time.Time,
	pauses []Pause,
) ([][2]time.Time, error) {
	var off [][2]time.Time

	for _, p := range pauses {
		if p.To != nil {
			off = append(off, [2]time.Time{p.From, *p.To})
		}
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT action, occurred_at FROM maintenance.job_events WHERE job_id = $1 AND occurred_at >= $2
		AND action IN ('job.held','job.submitted','job.resumed','job.rework_started') ORDER BY occurred_at, id`,
		job,
		from,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.job_events")
	}
	defer rows.Close()

	var since *time.Time

	for rows.Next() {
		var (
			action string
			when   time.Time
		)

		err := rows.Scan(&action, &when)
		if err != nil {
			return nil, apperr.Wrap(err, "select maintenance.job_events")
		}

		switch {
		case (action == "job.held" || action == "job.submitted") && since == nil:
			since = &when
		case (action == "job.resumed" || action == "job.rework_started") && since != nil:
			off = append(off, [2]time.Time{*since, when})
			since = nil
		}
	}

	return off, apperr.Wrap(rows.Err(), "select maintenance.job_events")
}

// workedFor is the time in [from, to) outside the off intervals, overlaps counted once.
func workedFor(from, until time.Time, off [][2]time.Time) time.Duration {
	slices.SortFunc(off, func(a, b [2]time.Time) int { return a[0].Compare(b[0]) })

	duration, cur := time.Duration(0), from

	for _, o := range off {
		start, end := o[0], o[1]
		if end.After(until) {
			end = until
		}

		if !end.After(cur) {
			continue
		}

		if start.After(cur) {
			duration += start.Sub(cur)
		}

		cur = end
	}

	if until.After(cur) {
		duration += until.Sub(cur)
	}

	return duration
}

func firstNonNil(ts ...*time.Time) *time.Time {
	for _, t := range ts {
		if t != nil {
			return t
		}
	}

	return nil
}

// submitErrors applies IR100 to a draft.
func submitErrors(report *Report, components []string, now time.Time) map[string]string {
	fieldErrs := map[string]string{}

	keys := make([]string, 0, len(report.Items))
	for _, it := range report.Items {
		keys = append(keys, it.ComponentKey)
		if it.Result == nil {
			fieldErrs[fieldItems] = "errors.result_required"
		} else if *it.Result != resultNormal && (it.Reason == nil || utf8.RuneCountInString(*it.Reason) < 1) {
			fieldErrs[fieldItems] = "errors.reason_required"
		}
	}

	a, b := slices.Clone(keys), slices.Clone(components)
	slices.Sort(a)
	slices.Sort(b)

	if !slices.Equal(a, b) {
		fieldErrs[fieldItems] = "errors.components_mismatch"
	}

	if n := utf8.RuneCountInString(strings.TrimSpace(report.WorkText)); n < 10 || n > 4000 {
		fieldErrs[fieldWorkText] = apperr.KeyLength
	}

	switch {
	case report.NextAction == nil:
		fieldErrs[fieldNextAction] = apperr.KeyRequired
	case report.NextAction.Kind == nextActionFollowUp &&
		(report.NextAction.Date == nil || !report.NextAction.Date.After(now) || report.NextAction.Note == nil ||
			strings.TrimSpace(*report.NextAction.Note) == ""):
		fieldErrs[fieldNextAction] = apperr.KeyInvalid
	}

	for _, p := range report.Parts {
		if p.Quantity < 1 || p.Quantity > 999 {
			fieldErrs["parts"] = apperr.KeyRange
		}
	}

	for _, a := range report.AttachmentRefs {
		if a.Status != "ready" {
			fieldErrs["attachmentIds"] = "errors.attachment_not_ready"
		}
	}

	for _, mm := range report.Measurements {
		if mm.QualityReason != nil && *mm.QualityReason == "unit_mismatch" {
			fieldErrs["measurements"] = "errors.unit_mismatch"
		}
	}

	return fieldErrs
}

func (Reports) reportEvent(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	action string,
	report uuid.UUID,
	version int,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, report_id, report_version,
			occurred_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7)`,
		uuid.Must(uuid.NewV7()),
		job,
		call.Principal.UserID,
		action,
		report,
		version,
		call.Now,
	)

	return apperr.Wrap(err, "insert maintenance.job_events")
}

// ---- jobs.review ----

// decisionReturn sends a submitted report back for rework (the other review decision is decisionAccept).
const decisionReturn = "return"

// The review modes: a contractor's or HQ's review of its own delivery, or HQ stepping in on a contractor's report.
const (
	reviewNormal       = "normal"
	reviewHQEscalation = "hq_escalation"
)

// ReviewInput is jobs.review input.
type ReviewInput struct {
	JobID         uuid.UUID `json:"jobId"`
	ReportVersion int       `json:"reportVersion"`
	Decision      string    `json:"decision"`
	Reason        *string   `json:"reason,omitempty"`
	ReviewMode    string    `json:"reviewMode"`
}

// Validate implements ops.Validator.
func (in *ReviewInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ReportVersion < 1 {
		fieldErrs[fieldReportVersion] = apperr.KeyRequired
	}

	if in.Decision != decisionAccept && in.Decision != decisionReturn {
		fieldErrs["decision"] = apperr.KeyInvalid
	}

	if in.ReviewMode != reviewNormal && in.ReviewMode != reviewHQEscalation {
		fieldErrs["reviewMode"] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		trimmedReason(in.Reason, maxReasonLength, fieldReason, fieldErrs)
	}

	if (in.Decision == decisionReturn || in.ReviewMode == reviewHQEscalation) && in.Reason == nil {
		fieldErrs[fieldReason] = apperr.KeyRequired
	}

	return fieldErrs
}

// review serves jobs.review.
//
//	@Summary		jobs.review (write)
//	@ID				jobs.review
//	@Description	Authorization: contractor:partner.review:own-offer | admin:job.write:internal-or-escalation; \
//	@Description	no-self-review
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reason 1-1000; IR234 accept releases the active assignment (status completed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06, DD-P05 · Input versions: reportVersion=WorkReport.version
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;reports.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		ReviewInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/review [post]
func (m Reports) review(ctx context.Context, c *ops.Call, in *ReviewInput) (MaintenanceJob, error) {
	return written(m.reviewJob(ctx, c, in))
}

// reviewJob is review answering the job with its detail extras.
func (m Reports) reviewJob(ctx context.Context, call *ops.Call, input *ReviewInput) (Job, error) {
	err := checkReviewer(ctx, call, input)
	if err != nil {
		return Job{}, err
	}

	status, _, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	reportID, latest, err := reportUnderReview(ctx, call, input, status)
	if err != nil {
		return Job{}, err
	}

	reportState, jobStatus := reportAccepted, statusCompleted
	if input.Decision == decisionReturn {
		reportState, jobStatus = reportReturned, statusReworkRequested
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.work_reports SET state = $3, accepted_at = CASE WHEN $3 = 'accepted' THEN $4::timestamptz END,
			updated_at = $4
		WHERE id = $1 AND version = $2`,
		reportID,
		latest,
		reportState,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.work_reports")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, reason,
			occurred_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6)`,
		reportID,
		latest,
		call.Principal.UserID,
		input.Decision,
		input.Reason,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "insert maintenance.report_reviews")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = $2, completed_at = CASE WHEN $2 = 'completed' THEN $3::timestamptz ELSE
			completed_at END,
		version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		input.JobID,
		jobStatus,
		call.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	if jobStatus == statusCompleted {
		// DEC-73 / IR234: completion releases the assignment; a return keeps it for the rework
		err := ReleaseOnCompletion(ctx, call.Tx, input.JobID, call.Now)
		if err != nil {
			return Job{}, err
		}
	}

	reportEventErr := m.reportEvent(ctx, call, input.JobID, "job.reviewed", reportID, latest)
	if reportEventErr != nil {
		return Job{}, reportEventErr
	}

	return m.Jobs.finishScoped(
		ctx,
		call,
		input.JobID,
		"jobs.review",
		"ReportReviewed",
		deref(input.Reason),
	)
}

// checkReviewer checks who reviews the job: its contractor, in the normal review only, or HQ, in the normal review of
// a job without a contractor and the HQ review of one with a contractor.
func checkReviewer(ctx context.Context, call *ops.Call, input *ReviewInput) error {
	var contractor *uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.jobs WHERE id = $1`, input.JobID).
		Scan(&contractor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		return apperr.Wrap(err, "select maintenance.jobs")
	}

	switch call.Principal.Role {
	case roleContractor:
		if contractor == nil || *contractor != call.Principal.OrgID {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if input.ReviewMode != reviewNormal {
			return apperr.Fields(map[string]string{"reviewMode": apperr.KeyInvalid})
		}
	default: // admin
		if (contractor == nil) != (input.ReviewMode == reviewNormal) {
			return apperr.E(apperr.Forbidden, "errors.review_mode")
		}
	}

	return nil
}

// reportUnderReview is the job's submitted report at the version shown, which someone other than its author reviews;
// it returns the report and its latest version.
func reportUnderReview(
	ctx context.Context,
	call *ops.Call,
	input *ReviewInput,
	status string,
) (uuid.UUID, int, error) {
	var (
		reportID uuid.UUID
		author   uuid.UUID
		state    string
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, author_id, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY
		version
		DESC LIMIT 1 FOR UPDATE`, input.JobID).
		Scan(&reportID, &author, &state)
	if errors.Is(err, pgx.ErrNoRows) || status != statusSubmitted {
		return reportID, 0, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	if err != nil {
		return reportID, 0, apperr.Wrap(err, "select maintenance.work_reports")
	}

	var latest int

	scanErr := call.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, reportID).
		Scan(&latest)
	if scanErr != nil {
		return reportID, 0, apperr.Wrap(scanErr, "select maintenance.work_reports")
	}

	if latest != input.ReportVersion || state != reportSubmitted {
		return reportID, 0, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}

	if author == call.Principal.UserID {
		return reportID, 0, apperr.E(apperr.Forbidden, "errors.self_review")
	}

	return reportID, latest, nil
}

// ---- jobs.resumeRework ----

// NewDraftFromLatest copies the latest frozen version into a new draft (version + 1) when the job has none.
func NewDraftFromLatest(ctx context.Context, call *ops.Call, job uuid.UUID) error {
	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items, measurements, parts,
			refrigerant, work_text,
			next_action, created_at, updated_at)
		SELECT id, version + 1, tenant_id, job_id, author_id, 'draft', items, measurements, parts, refrigerant, work_text,
		next_action, $2, $2
		FROM maintenance.work_reports r WHERE r.job_id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.work_reports d WHERE
		d.job_id = $1 AND d.state = 'draft')
		ORDER BY version DESC LIMIT 1`,
		job,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "insert maintenance.work_reports")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET draft_report_version = (SELECT version FROM maintenance.work_reports WHERE job_id = $1
			AND state = 'draft') WHERE id = $1`,
		job,
	)

	return apperr.Wrap(err, "update maintenance.jobs")
}

// resumeRework serves jobs.resumeRework.
//
//	@Summary		jobs.resumeRework (write)
//	@ID				jobs.resumeRework
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T08
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		JobIDInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/resume-rework [post]
func (m Reports) resumeRework(
	ctx context.Context,
	c *ops.Call,
	in *JobIDInput,
) (MaintenanceJob, error) {
	return written(m.resumeReworkJob(ctx, c, in))
}

// resumeReworkJob is resumeRework answering the job with its detail extras.
func (m Reports) resumeReworkJob(
	ctx context.Context,
	call *ops.Call,
	input *JobIDInput,
) (Job, error) {
	status, _, err := m.lockJob(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != statusReworkRequested {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	newDraftFromLatestErr := NewDraftFromLatest(ctx, call, input.JobID)
	if newDraftFromLatestErr != nil {
		return Job{}, newDraftFromLatestErr
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'in_progress', version = version + 1, updated_at = platform.app_now() WHERE id =
			$1`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.Jobs.event(ctx, call, input.JobID, "job.rework_started", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.Jobs.finishScoped(ctx, call, input.JobID, "jobs.resumeRework", "ReworkStarted", "")
}

// ---- reports.get ----

// GetReportInput is reports.get input.
type GetReportInput struct {
	JobID         uuid.UUID `json:"jobId"`
	ReportID      uuid.UUID `json:"reportId"`
	ReportVersion int       `json:"reportVersion"`
}

// Validate implements ops.Validator.
func (in *GetReportInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.ReportID == uuid.Nil {
		fieldErrs["reportId"] = apperr.KeyRequired
	}

	if in.ReportVersion < 1 {
		fieldErrs[fieldReportVersion] = apperr.KeyRequired
	}

	return fieldErrs
}

// get serves reports.get.
//
//	@Summary		reports.get (read)
//	@ID				reports.get
//	@Description	Authorization: client:accepted-report-only | contractor:partner.review:submitted | \
//	@Description	technician:assigned-valid-job | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-C09, DD-P05, DD-T04, DD-T05, DD-T06, DD-T08, DD-T09, DD-T15
//	@Tags			reports
//	@Accept			json
//	@Produce		json
//	@Param			jobId			path		string	true	"input field jobId"
//	@Param			reportId		path		string	true	"input field reportId"
//	@Param			reportVersion	query		integer	false	"input field reportVersion"
//	@Success		200				{object}	ops.Envelope{data=Report}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/reports/{reportId} [get]
func (m Reports) get(ctx context.Context, call *ops.Call, input *GetReportInput) (Report, error) {
	var (
		org        uuid.UUID
		unit       uuid.UUID
		contractor *uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT customer_org_id, unit_id, contractor_org_id FROM maintenance.jobs WHERE id =
		$1`, input.JobID).
		Scan(&org, &unit, &contractor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Report{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	report, err := m.load(ctx, call, input.ReportID, input.ReportVersion)
	if err != nil {
		return report, err
	}

	notFoundErr := apperr.E(apperr.NotFound, apperr.KeyNotFound)
	if report.JobID != input.JobID {
		return Report{}, notFoundErr
	}

	switch call.Principal.Role {
	case roleClient:
		if org != call.Principal.OrgID || report.State != reportAccepted {
			return Report{}, notFoundErr
		}
	case roleContractor:
		if contractor == nil || *contractor != call.Principal.OrgID || report.State == reportDraft {
			return Report{}, notFoundErr
		}
	case roleTechnician:
		err := m.Access.TechnicianJob(ctx, call, input.JobID, unit)
		if err != nil {
			return Report{}, err
		}
	}

	return report, nil
}

// RegisterReports binds the work-report operations.
func RegisterReports(r *ops.Registry, m Reports) {
	ops.Register(r, "jobs.saveDraft", m.saveDraft)
	ops.Register(r, "jobs.submit", m.submit)
	ops.Register(r, "jobs.review", m.review)
	ops.Register(r, "jobs.resumeRework", m.resumeRework)
	ops.Register(r, "reports.get", m.get)
}
