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

// ComponentGroups lists the inspection components per Unit.serviceScope group in DD-T04–T06 order (IR100 / IR126).
var ComponentGroups = []struct {
	Group string
	Keys  []string
}{
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

func groupOf(key string) string {
	for _, g := range ComponentGroups {
		if slices.Contains(g.Keys, key) {
			return g.Group
		}
	}

	return ""
}

// Components returns the component keys of a unit's service scope in canonical order.
func Components(scope []string) []string {
	var out []string

	for _, g := range ComponentGroups {
		if slices.Contains(scope, g.Group) {
			out = append(out, g.Keys...)
		}
	}

	return out
}

// MetricUnits is the expected UnitSymbol per Metric (IR126 item 2).
var MetricUnits = map[string]string{
	"temperature":          "°C",
	"humidity":             "%",
	"co2":                  "ppm",
	"pm25":                 "µg/m³",
	"power":                "kW",
	"vibration":            "mm/s",
	"refrigerant_pressure": "kPa",
	"compressor_cycles":    "cycles/h",
	"airflow_drop":         "%",
	"heartbeat_gap":        "min",
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

	if len(next) > 0 && string(next) != "null" {
		report.NextAction = &NextAction{Kind: "", Date: nil, Note: nil}

		err := json.Unmarshal(next, report.NextAction)
		if err != nil {
			return report, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	if report.SignOff == nil {
		report.SignOff = json.RawMessage("null")
	}

	return report, nil
}

// decorate adds attachments, review history and the caller's review availability.
func (m Reports) decorate(ctx context.Context, call *ops.Call, x *Report) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, tenant_id, created_at, updated_at, job_id, name, mime, size_bytes, status FROM maintenance.attachments
		WHERE report_id = $1 ORDER BY created_at, id`,
		x.ID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.attachments")
	}

	x.AttachmentRefs = []AttachmentRef{}

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

		attachmentRef.Version, attachmentRef.ReportID, attachmentRef.BlobID = 1, x.ID, attachmentRef.ID
		x.AttachmentRefs = append(x.AttachmentRefs, attachmentRef)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select maintenance.attachments")
	}

	rows, err = call.Tx.Query(
		ctx,
		`SELECT reviewer_user_id, report_version, decision, reason, occurred_at FROM maintenance.report_reviews WHERE
			report_id = $1 ORDER BY occurred_at, report_version`,
		x.ID,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.report_reviews")
	}

	x.ReviewHistory = []Review{}

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

		x.ReviewHistory = append(x.ReviewHistory, review)
	}

	rows.Close()

	rowsErr = rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "select maintenance.report_reviews")
	}

	var latest int

	scanErr := call.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, x.ID).
		Scan(&latest)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select maintenance.work_reports")
	}

	reason := any(nil)

	switch {
	case (call.Principal.Role != "contractor" || !call.Principal.Permissions["partner.review"]) &&
		(call.Principal.Role != "admin" || !call.Principal.Permissions["job.write"]):
		reason = "permission_denied"
	case x.AuthorID == call.Principal.UserID:
		reason = "self_authored"
	case x.Version != latest:
		reason = "not_current"
	case x.State != "submitted":
		reason = "not_submitted"
	}

	x.ReviewAvailability = map[string]any{"allowed": reason == nil, "reason": reason}

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

var (
	itemResults = map[string]bool{
		"normal":         true,
		"attention":      true,
		"not_inspected":  true,
		"not_applicable": true,
	}
	partSources  = map[string]bool{"van_stock": true, "hq_warehouse": true, "bought_locally": true}
	disposals    = map[string]bool{"disposed_on_site": true, "returned": true}
	refrigerants = map[string]bool{"R32": true, "R410A": true}
	leakChecks   = map[string]bool{"pass": true, "fail": true, "not_done": true}
	unitSymbols  = map[string]bool{
		"°C":       true,
		"%":        true,
		"ppm":      true,
		"µg/m³":    true,
		"kW":       true,
		"mm/s":     true,
		"kPa":      true,
		"cycles/h": true,
		"min":      true,
	}
)

// Validate implements ops.Validator (IR126 item 2, drafting rules).
func (in *DraftInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs["jobId"] = "error.required"
	}

	if in.Items == nil || in.Measurements == nil || in.Parts == nil || in.Refrigerant == nil ||
		in.AttachmentIDs == nil {
		fieldErrs["draft"] = "error.required"

		return fieldErrs
	}

	seen := map[string]bool{}

	for i := range in.Items {
		itemInput := &in.Items[i]
		if groupOf(itemInput.ComponentKey) == "" ||
			groupOf(itemInput.ComponentKey) != itemInput.ComponentGroup ||
			seen[itemInput.ComponentKey] {
			fieldErrs["items"] = "error.invalid"
		}

		seen[itemInput.ComponentKey] = true
		if itemInput.Result != nil && !itemResults[*itemInput.Result] {
			fieldErrs["items"] = "error.invalid"
		}

		if itemInput.Reason != nil {
			*itemInput.Reason = strings.TrimSpace(*itemInput.Reason)
			if utf8.RuneCountInString(*itemInput.Reason) > 1000 {
				fieldErrs["items"] = "error.length"
			}
		}

		if itemInput.EvidenceIDs == nil {
			itemInput.EvidenceIDs = []uuid.UUID{}
		}
	}

	for _, x := range in.Measurements {
		if !seen[x.ComponentKey] || MetricUnits[x.Metric] == "" || !unitSymbols[x.Unit] ||
			x.ObservedAt.IsZero() {
			fieldErrs["measurements"] = "error.invalid"
		}
	}

	for _, p := range in.Parts {
		n := utf8.RuneCountInString(strings.TrimSpace(p.Name))
		if n < 1 || n > 120 || p.Quantity < 0 || p.Quantity > 999 || !partSources[p.Source] ||
			(p.OldPartDisposal != nil && !disposals[*p.OldPartDisposal]) ||
			(p.ReplacesComponentKey != nil && groupOf(*p.ReplacesComponentKey) == "") {
			fieldErrs["parts"] = "error.invalid"
		}
	}

	for _, r := range in.Refrigerant {
		n := utf8.RuneCountInString(r.CylinderID)
		if !refrigerants[r.Refrigerant] || n < 1 || n > 64 || r.RecoveredKg < 0 ||
			r.ChargedKg < 0 ||
			!leakChecks[r.LeakCheck] {
			fieldErrs["refrigerant"] = "error.invalid"
		}
	}

	if utf8.RuneCountInString(in.WorkText) > 4000 {
		fieldErrs["workText"] = "error.length"
	}

	if nextAction := in.NextAction; nextAction != nil {
		switch nextAction.Kind {
		case "none":
			if nextAction.Date != nil || nextAction.Note != nil {
				fieldErrs["nextAction"] = "error.invalid"
			}
		case "follow_up":
			if nextAction.Date == nil || nextAction.Note == nil ||
				utf8.RuneCountInString(*nextAction.Note) > 1000 {
				fieldErrs["nextAction"] = "error.invalid"
			}
		default:
			fieldErrs["nextAction"] = "error.invalid"
		}
	}

	return fieldErrs
}

// draftOf returns the job's current draft (nil when none).
func (m Reports) draftOf(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	lock bool,
) (*Report, error) {
	q := "SELECT " + reportCols + " FROM maintenance.work_reports WHERE job_id = $1 AND state = 'draft'"
	if lock {
		q += " FOR UPDATE"
	}

	x, err := scanReport(call.Tx.QueryRow(ctx, q, job))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &x, err
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
func (m Reports) saveDraft(ctx context.Context, c *ops.Call, input *DraftInput) (Report, error) {
	var (
		status string
		unit   uuid.UUID
	)

	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, input.JobID).
		Scan(&status, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return Report{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	technicianJobErr := m.Access.TechnicianJob(ctx, c, input.JobID, unit)
	if technicianJobErr != nil {
		return Report{}, technicianJobErr
	}

	if status != "in_progress" {
		return Report{}, apperr.E(apperr.Conflict, "error.invalidState")
	}

	draft, err := m.draftOf(ctx, c, input.JobID, true)
	if err != nil {
		return Report{}, err
	}

	var prev Report

	switch {
	case input.ReportID == nil:
		var n int

		err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.work_reports WHERE job_id = $1`, input.JobID).
			Scan(&n)
		if err != nil {
			return Report{}, apperr.Wrap(err, "select maintenance.work_reports")
		}

		if n > 0 {
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
	case draft == nil || draft.ID != *input.ReportID:
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	case draft.Version != *c.ExpectedVersion:
		return Report{}, apperr.E(apperr.Conflict, "error.versionConflict")
	default:
		prev = *draft
	}
	// attachments of the same report
	allowed := map[uuid.UUID]bool{}

	rows, err := c.Tx.Query(
		ctx,
		`SELECT id FROM maintenance.attachments WHERE report_id = $1`,
		prev.ID,
	)
	if err != nil {
		return Report{}, apperr.Wrap(err, "select maintenance.attachments")
	}

	for rows.Next() {
		var id uuid.UUID

		err := rows.Scan(&id)
		if err != nil {
			rows.Close()

			return Report{}, apperr.Wrap(err, "select maintenance.attachments")
		}

		allowed[id] = true
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return Report{}, apperr.Wrap(rowsErr, "select maintenance.attachments")
	}

	for _, id := range input.AttachmentIDs {
		if !allowed[id] {
			return Report{}, apperr.Fields(
				map[string]string{"attachmentIds": "error.otherReportAttachment"},
			)
		}
	}
	// items keep their id and author unless changed (SR07)
	old := map[string]Item{}
	for _, it := range prev.Items {
		old[it.ComponentKey] = it
	}

	items := []Item{}

	for _, req := range input.Items {
		for _, e := range req.EvidenceIDs {
			if !allowed[e] {
				return Report{}, apperr.Fields(
					map[string]string{"items": "error.otherReportAttachment"},
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
			AuthorID:       c.Principal.UserID,
			ObservedAt:     c.Now,
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

	itemID := map[string]uuid.UUID{}
	for _, it := range items {
		itemID[it.ComponentKey] = it.ID
	}

	oldM := map[uuid.UUID]ReportMeasurement{}
	for _, x := range prev.Measurements {
		oldM[x.ID] = x
	}

	meas := []ReportMeasurement{}

	for idx, x := range input.Measurements {
		reportMeasurement := ReportMeasurement{
			ID:            uuid.New(),
			TenantID:      c.Principal.TenantID,
			Version:       1,
			CreatedAt:     c.Now,
			UpdatedAt:     c.Now,
			UnitID:        unit,
			SensorID:      itemID[x.ComponentKey],
			ComponentKey:  x.ComponentKey,
			Metric:        x.Metric,
			Value:         x.Value,
			Unit:          x.Unit,
			ObservedAt:    x.ObservedAt,
			ReceivedAt:    c.Now,
			Origin:        "inspection",
			Quality:       "valid",
			IsDemo:        true,
			Sequence:      idx + 1,
			BoundaryID:    nil,
			QualityReason: nil,
			RawUnit:       nil,
			EventID:       uuid.UUID{},
		}
		if x.ID != nil {
			o, ok := oldM[*x.ID]
			if !ok {
				return Report{}, apperr.E(apperr.NotFound, "error.notFound")
			}

			reportMeasurement.ID, reportMeasurement.CreatedAt, reportMeasurement.Version = o.ID, o.CreatedAt, o.Version+1
		}

		reportMeasurement.EventID = reportMeasurement.ID

		switch {
		case x.Value == nil || math.IsNaN(*x.Value) || math.IsInf(*x.Value, 0):
			reportMeasurement.Quality, reportMeasurement.Value = "missing", nil
		case MetricUnits[x.Metric] != x.Unit:
			reason := "unit_mismatch"
			reportMeasurement.Quality, reportMeasurement.QualityReason, reportMeasurement.RawUnit = "suspect", &reason, &x.Unit
		}

		meas = append(meas, reportMeasurement)
	}

	for i := range input.Parts {
		input.Parts[i].Name = strings.TrimSpace(input.Parts[i].Name)
	}

	enc := func(v any) []byte {
		b, _ := json.Marshal(v)

		return b
	}

	var next any
	if input.NextAction != nil {
		next = enc(input.NextAction)
	}

	if input.ReportID == nil {
		_, err = c.Tx.Exec(
			ctx,
			`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items, measurements, parts,
				refrigerant, work_text,
			next_action, created_at, updated_at) VALUES ($1, 1, current_setting('app.tenant_id')::uuid, $2, $3, 'draft', $4, $5,
			$6, $7, $8, $9, $10, $10)`,
			prev.ID,
			input.JobID,
			c.Principal.UserID,
			enc(items),
			enc(meas),
			enc(input.Parts),
			enc(input.Refrigerant),
			input.WorkText,
			next,
			c.Now,
		)
	} else {
		_, err = c.Tx.Exec(
			ctx,
			`UPDATE maintenance.work_reports SET version = version + 1, items = $3, measurements = $4, parts = $5, refrigerant =
				$6, work_text = $7,
			next_action = $8, sign_off = NULL, updated_at = $9 WHERE id = $1 AND version = $2`,
			prev.ID,
			prev.Version,
			enc(items),
			enc(meas),
			enc(input.Parts),
			enc(input.Refrigerant),
			input.WorkText,
			next,
			c.Now,
		)
	}

	if err != nil {
		return Report{}, apperr.Wrap(err, "update maintenance.work_reports")
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE maintenance.attachments SET report_id = $2 WHERE id = ANY($1)`,
		input.AttachmentIDs,
		prev.ID,
	)
	if execErr != nil {
		return Report{}, apperr.Wrap(execErr, "update maintenance.attachments")
	}

	_, execErr = c.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET draft_report_version = $2 WHERE id = $1`,
		input.JobID,
		prev.Version+1,
	)
	if execErr != nil {
		return Report{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	report, err := m.load(ctx, c, prev.ID, prev.Version+1)
	if err != nil {
		return report, err
	}

	c.Emit(
		ops.Event{
			AggregateType: "report",
			AggregateID:   report.ID,
			Type:          "ReportDraftSaved",
			Payload:       map[string]any{"jobId": input.JobID, "version": report.Version},
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:          "jobs.saveDraft",
			TargetKind:      "report",
			TargetID:        report.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &report.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return report, nil
}

func eqStr(
	a, b *string,
) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func (m Reports) load(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	version int,
) (Report, error) {
	report, err := scanReport(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+reportCols+" FROM maintenance.work_reports WHERE id = $1 AND version = $2",
			id,
			version,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return report, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return report, err
	}

	return report, m.decorate(ctx, call, &report)
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
		fieldErrs["jobId"] = "error.required"
	}

	if in.ReportVersion < 1 {
		fieldErrs["reportVersion"] = "error.required"
	}

	return fieldErrs
}

func (m Reports) lockJob(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
) (string, uuid.UUID, error) {
	var (
		status string
		unit   uuid.UUID
		v      int
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, id).
		Scan(&status, &unit, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unit, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return "", unit, apperr.Wrap(err, "select maintenance.jobs")
	}

	if call.Principal.Role == "technician" {
		err := m.Access.TechnicianJob(ctx, call, id, unit)
		if err != nil {
			return "", unit, err
		}
	}

	if v != *call.ExpectedVersion {
		return "", unit, apperr.E(apperr.Conflict, "error.versionConflict")
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
func (m Reports) submitJob(ctx context.Context, call *ops.Call, in *SubmitInput) (Job, error) {
	status, unit, err := m.lockJob(ctx, call, in.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != "in_progress" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}

	draft, err := m.draftOf(ctx, call, in.JobID, true)
	if err != nil {
		return Job{}, err
	}

	if draft == nil || draft.Version != in.ReportVersion {
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

	var raw []byte

	scanErr := call.Tx.QueryRow(ctx, `SELECT time_on_site FROM maintenance.jobs WHERE id = $1`, in.JobID).
		Scan(&raw)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.jobs")
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
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &timeOnSite)
	}

	now := call.Now
	timeOnSite.FinishedAt = &now
	// a pause still open ends with the submission, so its time is not counted as on site (IR448)
	if n := len(timeOnSite.Pauses); n > 0 && timeOnSite.Pauses[n-1].To == nil {
		timeOnSite.Pauses[n-1].To = &now
	}

	if from := firstNonNil(timeOnSite.ArrivedAt, timeOnSite.StartedAt); from != nil {
		off, err := offWork(ctx, call, in.JobID, *from, timeOnSite.Pauses)
		if err != nil {
			return Job{}, err
		}

		mins := int(workedFor(*from, now, off) / time.Minute)
		timeOnSite.OnSiteMinutes = &mins
	}

	tos, _ := json.Marshal(timeOnSite)

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'submitted', time_on_site = $2, draft_report_version = NULL, version = version +
			1, updated_at = platform.app_now()
		WHERE id = $1`,
		in.JobID,
		tos,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	reportEventErr := m.reportEvent(
		ctx,
		call,
		in.JobID,
		"job.submitted",
		draft.ID,
		draft.Version,
	)
	if reportEventErr != nil {
		return Job{}, reportEventErr
	}

	return m.Jobs.finishScoped(ctx, call, in.JobID, "jobs.submit", "ReportSubmitted", "")
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

	keys := []string{}
	for _, it := range report.Items {
		keys = append(keys, it.ComponentKey)
		if it.Result == nil {
			fieldErrs["items"] = "errors.result_required"
		} else if *it.Result != "normal" && (it.Reason == nil || utf8.RuneCountInString(*it.Reason) < 1) {
			fieldErrs["items"] = "errors.reason_required"
		}
	}

	a, b := slices.Clone(keys), slices.Clone(components)
	slices.Sort(a)
	slices.Sort(b)

	if !slices.Equal(a, b) {
		fieldErrs["items"] = "errors.components_mismatch"
	}

	if n := utf8.RuneCountInString(report.WorkText); n < 10 || n > 4000 {
		fieldErrs["workText"] = "error.length"
	}

	switch {
	case report.NextAction == nil:
		fieldErrs["nextAction"] = "error.required"
	case report.NextAction.Kind == "follow_up" &&
		(report.NextAction.Date == nil || !report.NextAction.Date.After(now) || report.NextAction.Note == nil ||
			utf8.RuneCountInString(*report.NextAction.Note) < 1):
		fieldErrs["nextAction"] = "error.invalid"
	}

	for _, p := range report.Parts {
		if p.Quantity < 1 || p.Quantity > 999 {
			fieldErrs["parts"] = "error.range"
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

func (m Reports) reportEvent(
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
		fieldErrs["jobId"] = "error.required"
	}

	if in.ReportVersion < 1 {
		fieldErrs["reportVersion"] = "error.required"
	}

	if in.Decision != "accept" && in.Decision != "return" {
		fieldErrs["decision"] = "error.invalid"
	}

	if in.ReviewMode != "normal" && in.ReviewMode != "hq_escalation" {
		fieldErrs["reviewMode"] = "error.invalid"
	}

	if in.Reason != nil {
		trimmedReason(in.Reason, 1000, "reason", fieldErrs)
	}

	if (in.Decision == "return" || in.ReviewMode == "hq_escalation") && in.Reason == nil {
		fieldErrs["reason"] = "error.required"
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
func (m Reports) reviewJob(ctx context.Context, c *ops.Call, input *ReviewInput) (Job, error) {
	var contractor *uuid.UUID

	err := c.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.jobs WHERE id = $1`, input.JobID).
		Scan(&contractor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, apperr.E(apperr.NotFound, "error.notFound")
		}

		return Job{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	switch c.Principal.Role {
	case "contractor":
		if contractor == nil || *contractor != c.Principal.OrgID {
			return Job{}, apperr.E(apperr.NotFound, "error.notFound")
		}

		if input.ReviewMode != "normal" {
			return Job{}, apperr.Fields(map[string]string{"reviewMode": "error.invalid"})
		}
	default: // admin
		if (contractor == nil) != (input.ReviewMode == "normal") {
			return Job{}, apperr.E(apperr.Forbidden, "errors.review_mode")
		}
	}

	status, _, err := m.lockJob(ctx, c, input.JobID)
	if err != nil {
		return Job{}, err
	}

	var (
		id     uuid.UUID
		author uuid.UUID
		state  string
	)

	err = c.Tx.QueryRow(ctx, `SELECT id, author_id, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY version
		DESC LIMIT 1 FOR UPDATE`, input.JobID).
		Scan(&id, &author, &state)
	if errors.Is(err, pgx.ErrNoRows) || status != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}

	if err != nil {
		return Job{}, apperr.Wrap(err, "select maintenance.work_reports")
	}

	var latest int

	scanErr := c.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, id).
		Scan(&latest)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "select maintenance.work_reports")
	}

	if latest != input.ReportVersion || state != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}

	if author == c.Principal.UserID {
		return Job{}, apperr.E(apperr.Forbidden, "errors.self_review")
	}

	reportState, jobStatus := "accepted", "completed"
	if input.Decision == "return" {
		reportState, jobStatus = "returned", "rework_requested"
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE maintenance.work_reports SET state = $3, accepted_at = CASE WHEN $3 = 'accepted' THEN $4::timestamptz END,
			updated_at = $4
		WHERE id = $1 AND version = $2`,
		id,
		latest,
		reportState,
		c.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.work_reports")
	}

	_, execErr = c.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, reason,
			occurred_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6)`,
		id,
		latest,
		c.Principal.UserID,
		input.Decision,
		input.Reason,
		c.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "insert maintenance.report_reviews")
	}

	_, execErr = c.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = $2, completed_at = CASE WHEN $2 = 'completed' THEN $3::timestamptz ELSE
			completed_at END,
		version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		input.JobID,
		jobStatus,
		c.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	if jobStatus == "completed" { // DEC-73 / IR234: completion releases the assignment; a return keeps it for the rework
		err := ReleaseOnCompletion(ctx, c.Tx, input.JobID, c.Now)
		if err != nil {
			return Job{}, err
		}
	}

	reportEventErr := m.reportEvent(ctx, c, input.JobID, "job.reviewed", id, latest)
	if reportEventErr != nil {
		return Job{}, reportEventErr
	}

	return m.Jobs.finishScoped(
		ctx,
		c,
		input.JobID,
		"jobs.review",
		"ReportReviewed",
		deref(input.Reason),
	)
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
func (m Reports) resumeReworkJob(ctx context.Context, c *ops.Call, input *JobIDInput) (Job, error) {
	status, _, err := m.lockJob(ctx, c, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != "rework_requested" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}

	newDraftFromLatestErr := NewDraftFromLatest(ctx, c, input.JobID)
	if newDraftFromLatestErr != nil {
		return Job{}, newDraftFromLatestErr
	}

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = 'in_progress', version = version + 1, updated_at = platform.app_now() WHERE id =
			$1`,
		input.JobID,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.Jobs.event(ctx, c, input.JobID, "job.rework_started", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	return m.Jobs.finishScoped(ctx, c, input.JobID, "jobs.resumeRework", "ReworkStarted", "")
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
		fieldErrs["jobId"] = "error.required"
	}

	if in.ReportID == uuid.Nil {
		fieldErrs["reportId"] = "error.required"
	}

	if in.ReportVersion < 1 {
		fieldErrs["reportVersion"] = "error.required"
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
func (m Reports) get(ctx context.Context, c *ops.Call, input *GetReportInput) (Report, error) {
	var (
		org        uuid.UUID
		unit       uuid.UUID
		contractor *uuid.UUID
	)

	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, unit_id, contractor_org_id FROM maintenance.jobs WHERE id =
		$1`, input.JobID).
		Scan(&org, &unit, &contractor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return Report{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	report, err := m.load(ctx, c, input.ReportID, input.ReportVersion)
	if err != nil {
		return report, err
	}

	nf := apperr.E(apperr.NotFound, "error.notFound")
	if report.JobID != input.JobID {
		return Report{}, nf
	}

	switch c.Principal.Role {
	case "client":
		if org != c.Principal.OrgID || report.State != "accepted" {
			return Report{}, nf
		}
	case "contractor":
		if contractor == nil || *contractor != c.Principal.OrgID || report.State == "draft" {
			return Report{}, nf
		}
	case "technician":
		err := m.Access.TechnicianJob(ctx, c, input.JobID, unit)
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
