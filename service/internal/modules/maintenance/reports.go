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

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// ComponentGroups lists the inspection components per Unit.serviceScope group in DD-T04–T06 order (IR100 / IR126).
var ComponentGroups = []struct {
	Group string
	Keys  []string
}{
	{"indoor", []string{"filter", "evaporator_coil", "blower_motor", "blower_fan", "drain_pipe", "drain_pan", "outlet", "louver"}},
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
var MetricUnits = map[string]string{"temperature": "°C", "humidity": "%", "co2": "ppm", "pm25": "µg/m³", "power": "kW", "vibration": "mm/s",
	"refrigerant_pressure": "kPa", "compressor_cycles": "cycles/h", "airflow_drop": "%", "heartbeat_gap": "min"}

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
	SignOff            json.RawMessage     `json:"signOff"`
	WorkText           string              `json:"workText"`
	NextAction         *NextAction         `json:"nextAction"`
	AttachmentRefs     []AttachmentRef     `json:"attachmentRefs"`
	SubmittedAt        *time.Time          `json:"submittedAt"`
	AcceptedAt         *time.Time          `json:"acceptedAt"`
	ReviewHistory      []Review            `json:"reviewHistory"`
	State              string              `json:"-"`
	attachmentIDs      []uuid.UUID
}

// Reports is the work-report operation set (IR126).
type Reports struct {
	Jobs   Jobs
	Access Access
	Units  ServiceUnits
}

const reportCols = `id, tenant_id, version, created_at, updated_at, job_id, author_id, state, items, measurements, parts, refrigerant, sign_off, work_text,
	next_action, submitted_at, accepted_at`

func scanReport(r pgx.Row) (Report, error) {
	var x Report
	var items, meas, parts, refr, next []byte
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.JobID, &x.AuthorID, &x.State, &items, &meas, &parts, &refr, &x.SignOff,
		&x.WorkText, &next, &x.SubmittedAt, &x.AcceptedAt)
	if err != nil {
		return x, err
	}
	x.Items, x.Measurements, x.Parts, x.Refrigerant = []Item{}, []ReportMeasurement{}, []Part{}, []Refrigerant{}
	for _, p := range []struct {
		raw []byte
		dst any
	}{{items, &x.Items}, {meas, &x.Measurements}, {parts, &x.Parts}, {refr, &x.Refrigerant}} {
		if len(p.raw) > 0 {
			if err := json.Unmarshal(p.raw, p.dst); err != nil {
				return x, err
			}
		}
	}
	if len(next) > 0 && string(next) != "null" {
		x.NextAction = &NextAction{}
		if err := json.Unmarshal(next, x.NextAction); err != nil {
			return x, err
		}
	}
	if x.SignOff == nil {
		x.SignOff = json.RawMessage("null")
	}
	return x, nil
}

// decorate adds attachments, review history and the caller's review availability.
func (m Reports) decorate(ctx context.Context, c *ops.Call, x *Report) error {
	rows, err := c.Tx.Query(ctx, `SELECT id, tenant_id, created_at, updated_at, job_id, name, mime, size_bytes, status FROM maintenance.attachments
		WHERE report_id = $1 ORDER BY created_at, id`, x.ID)
	if err != nil {
		return err
	}
	x.AttachmentRefs = []AttachmentRef{}
	for rows.Next() {
		var a AttachmentRef
		if err := rows.Scan(&a.ID, &a.TenantID, &a.CreatedAt, &a.UpdatedAt, &a.JobID, &a.Name, &a.Mime, &a.Size, &a.Status); err != nil {
			rows.Close()
			return err
		}
		a.Version, a.ReportID, a.BlobID = 1, x.ID, a.ID
		x.AttachmentRefs = append(x.AttachmentRefs, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = c.Tx.Query(ctx, `SELECT reviewer_user_id, report_version, decision, reason, occurred_at FROM maintenance.report_reviews WHERE report_id = $1 ORDER BY occurred_at, report_version`, x.ID)
	if err != nil {
		return err
	}
	x.ReviewHistory = []Review{}
	for rows.Next() {
		var r Review
		if err := rows.Scan(&r.ReviewerUserID, &r.ReportVersion, &r.Decision, &r.Reason, &r.OccurredAt); err != nil {
			rows.Close()
			return err
		}
		x.ReviewHistory = append(x.ReviewHistory, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var latest int
	if err := c.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, x.ID).Scan(&latest); err != nil {
		return err
	}
	reason := any(nil)
	switch {
	case !(c.Principal.Role == "contractor" && c.Principal.Permissions["partner.review"]) && !(c.Principal.Role == "admin" && c.Principal.Permissions["job.write"]):
		reason = "permission_denied"
	case x.AuthorID == c.Principal.UserID:
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
	itemResults  = map[string]bool{"normal": true, "attention": true, "not_inspected": true, "not_applicable": true}
	partSources  = map[string]bool{"van_stock": true, "hq_warehouse": true, "bought_locally": true}
	disposals    = map[string]bool{"disposed_on_site": true, "returned": true}
	refrigerants = map[string]bool{"R32": true, "R410A": true}
	leakChecks   = map[string]bool{"pass": true, "fail": true, "not_done": true}
	unitSymbols  = map[string]bool{"°C": true, "%": true, "ppm": true, "µg/m³": true, "kW": true, "mm/s": true, "kPa": true, "cycles/h": true, "min": true}
)

// Validate implements ops.Validator (IR126 item 2, drafting rules).
func (in *DraftInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.Items == nil || in.Measurements == nil || in.Parts == nil || in.Refrigerant == nil || in.AttachmentIDs == nil {
		fe["draft"] = "error.required"
		return fe
	}
	seen := map[string]bool{}
	for i := range in.Items {
		it := &in.Items[i]
		if groupOf(it.ComponentKey) == "" || groupOf(it.ComponentKey) != it.ComponentGroup || seen[it.ComponentKey] {
			fe["items"] = "error.invalid"
		}
		seen[it.ComponentKey] = true
		if it.Result != nil && !itemResults[*it.Result] {
			fe["items"] = "error.invalid"
		}
		if it.Reason != nil {
			*it.Reason = strings.TrimSpace(*it.Reason)
			if utf8.RuneCountInString(*it.Reason) > 1000 {
				fe["items"] = "error.length"
			}
		}
		if it.EvidenceIDs == nil {
			it.EvidenceIDs = []uuid.UUID{}
		}
	}
	for _, x := range in.Measurements {
		if !seen[x.ComponentKey] || MetricUnits[x.Metric] == "" || !unitSymbols[x.Unit] || x.ObservedAt.IsZero() {
			fe["measurements"] = "error.invalid"
		}
	}
	for _, p := range in.Parts {
		n := utf8.RuneCountInString(strings.TrimSpace(p.Name))
		if n < 1 || n > 120 || p.Quantity < 0 || p.Quantity > 999 || !partSources[p.Source] || (p.OldPartDisposal != nil && !disposals[*p.OldPartDisposal]) ||
			(p.ReplacesComponentKey != nil && groupOf(*p.ReplacesComponentKey) == "") {
			fe["parts"] = "error.invalid"
		}
	}
	for _, r := range in.Refrigerant {
		n := utf8.RuneCountInString(r.CylinderID)
		if !refrigerants[r.Refrigerant] || n < 1 || n > 64 || r.RecoveredKg < 0 || r.ChargedKg < 0 || !leakChecks[r.LeakCheck] {
			fe["refrigerant"] = "error.invalid"
		}
	}
	if utf8.RuneCountInString(in.WorkText) > 4000 {
		fe["workText"] = "error.length"
	}
	if a := in.NextAction; a != nil {
		switch a.Kind {
		case "none":
			if a.Date != nil || a.Note != nil {
				fe["nextAction"] = "error.invalid"
			}
		case "follow_up":
			if a.Date == nil || a.Note == nil || utf8.RuneCountInString(*a.Note) > 1000 {
				fe["nextAction"] = "error.invalid"
			}
		default:
			fe["nextAction"] = "error.invalid"
		}
	}
	return fe
}

// draftOf returns the job's current draft (nil when none).
func (m Reports) draftOf(ctx context.Context, c *ops.Call, job uuid.UUID, lock bool) (*Report, error) {
	q := "SELECT " + reportCols + " FROM maintenance.work_reports WHERE job_id = $1 AND state = 'draft'"
	if lock {
		q += " FOR UPDATE"
	}
	x, err := scanReport(c.Tx.QueryRow(ctx, q, job))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}

func (m Reports) saveDraft(ctx context.Context, c *ops.Call, in *DraftInput) (Report, error) {
	var status string
	var unit uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, in.JobID).Scan(&status, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Report{}, err
	}
	if err := m.Access.TechnicianJob(ctx, c, in.JobID, unit); err != nil {
		return Report{}, err
	}
	if status != "in_progress" {
		return Report{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	draft, err := m.draftOf(ctx, c, in.JobID, true)
	if err != nil {
		return Report{}, err
	}
	var prev Report
	switch {
	case in.ReportID == nil:
		var n int
		if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.work_reports WHERE job_id = $1`, in.JobID).Scan(&n); err != nil {
			return Report{}, err
		}
		if n > 0 {
			return Report{}, apperr.E(apperr.Conflict, "errors.draft_exists")
		}
		prev = Report{ID: uuid.New(), Version: 0}
	case draft == nil || draft.ID != *in.ReportID:
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	case draft.Version != *c.ExpectedVersion:
		return Report{}, apperr.E(apperr.Conflict, "error.versionConflict")
	default:
		prev = *draft
	}
	// attachments of the same report
	allowed := map[uuid.UUID]bool{}
	rows, err := c.Tx.Query(ctx, `SELECT id FROM maintenance.attachments WHERE report_id = $1`, prev.ID)
	if err != nil {
		return Report{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Report{}, err
		}
		allowed[id] = true
	}
	rows.Close()
	for _, id := range in.AttachmentIDs {
		if !allowed[id] {
			return Report{}, apperr.Fields(map[string]string{"attachmentIds": "error.otherReportAttachment"})
		}
	}
	// items keep their id and author unless changed (SR07)
	old := map[string]Item{}
	for _, it := range prev.Items {
		old[it.ComponentKey] = it
	}
	items := []Item{}
	for _, in := range in.Items {
		for _, e := range in.EvidenceIDs {
			if !allowed[e] {
				return Report{}, apperr.Fields(map[string]string{"items": "error.otherReportAttachment"})
			}
		}
		it := Item{ID: uuid.New(), ComponentGroup: in.ComponentGroup, ComponentKey: in.ComponentKey, Result: in.Result, Reason: in.Reason, EvidenceIDs: in.EvidenceIDs,
			AuthorID: c.Principal.UserID, ObservedAt: c.Now}
		if it.Reason != nil && *it.Reason == "" {
			it.Reason = nil
		}
		if o, ok := old[in.ComponentKey]; ok {
			it.ID = o.ID
			if eqStr(o.Result, it.Result) && eqStr(o.Reason, it.Reason) && slices.Equal(o.EvidenceIDs, it.EvidenceIDs) {
				it.AuthorID, it.ObservedAt = o.AuthorID, o.ObservedAt
			}
		}
		items = append(items, it)
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
	for i, x := range in.Measurements {
		r := ReportMeasurement{ID: uuid.New(), TenantID: c.Principal.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, UnitID: unit, SensorID: itemID[x.ComponentKey],
			ComponentKey: x.ComponentKey, Metric: x.Metric, Value: x.Value, Unit: x.Unit, ObservedAt: x.ObservedAt, ReceivedAt: c.Now, Origin: "inspection",
			Quality: "valid", IsDemo: true, Sequence: i + 1}
		if x.ID != nil {
			o, ok := oldM[*x.ID]
			if !ok {
				return Report{}, apperr.E(apperr.NotFound, "error.notFound")
			}
			r.ID, r.CreatedAt, r.Version = o.ID, o.CreatedAt, o.Version+1
		}
		r.EventID = r.ID
		switch {
		case x.Value == nil || math.IsNaN(*x.Value) || math.IsInf(*x.Value, 0):
			r.Quality, r.Value = "missing", nil
		case MetricUnits[x.Metric] != x.Unit:
			reason := "unit_mismatch"
			r.Quality, r.QualityReason, r.RawUnit = "suspect", &reason, &x.Unit
		}
		meas = append(meas, r)
	}
	for i := range in.Parts {
		in.Parts[i].Name = strings.TrimSpace(in.Parts[i].Name)
	}
	enc := func(v any) []byte { b, _ := json.Marshal(v); return b }
	var next any
	if in.NextAction != nil {
		next = enc(in.NextAction)
	}
	if in.ReportID == nil {
		_, err = c.Tx.Exec(ctx, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items, measurements, parts, refrigerant, work_text,
			next_action, created_at, updated_at) VALUES ($1, 1, current_setting('app.tenant_id')::uuid, $2, $3, 'draft', $4, $5, $6, $7, $8, $9, $10, $10)`,
			prev.ID, in.JobID, c.Principal.UserID, enc(items), enc(meas), enc(in.Parts), enc(in.Refrigerant), in.WorkText, next, c.Now)
	} else {
		_, err = c.Tx.Exec(ctx, `UPDATE maintenance.work_reports SET version = version + 1, items = $3, measurements = $4, parts = $5, refrigerant = $6, work_text = $7,
			next_action = $8, sign_off = NULL, updated_at = $9 WHERE id = $1 AND version = $2`,
			prev.ID, prev.Version, enc(items), enc(meas), enc(in.Parts), enc(in.Refrigerant), in.WorkText, next, c.Now)
	}
	if err != nil {
		return Report{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.attachments SET report_id = $2 WHERE id = ANY($1)`, in.AttachmentIDs, prev.ID); err != nil {
		return Report{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET draft_report_version = $2 WHERE id = $1`, in.JobID, prev.Version+1); err != nil {
		return Report{}, err
	}
	x, err := m.load(ctx, c, prev.ID, prev.Version+1)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "report", AggregateID: x.ID, Type: "ReportDraftSaved", Payload: map[string]any{"jobId": in.JobID, "version": x.Version}})
	c.Audit(ops.AuditEntry{Action: "jobs.saveDraft", TargetKind: "report", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version})
	return x, nil
}

func eqStr(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func (m Reports) load(ctx context.Context, c *ops.Call, id uuid.UUID, version int) (Report, error) {
	x, err := scanReport(c.Tx.QueryRow(ctx, "SELECT "+reportCols+" FROM maintenance.work_reports WHERE id = $1 AND version = $2", id, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	return x, m.decorate(ctx, c, &x)
}

// ---- jobs.submit ----

// SubmitInput is jobs.submit input.
type SubmitInput struct {
	JobID         uuid.UUID `json:"jobId"`
	ReportVersion int       `json:"reportVersion"`
}

// Validate implements ops.Validator.
func (in *SubmitInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ReportVersion < 1 {
		fe["reportVersion"] = "error.required"
	}
	return fe
}

func (m Reports) lockJob(ctx context.Context, c *ops.Call, id uuid.UUID) (string, uuid.UUID, error) {
	var status string
	var unit uuid.UUID
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, id).Scan(&status, &unit, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unit, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return "", unit, err
	}
	if c.Principal.Role == "technician" {
		if err := m.Access.TechnicianJob(ctx, c, id, unit); err != nil {
			return "", unit, err
		}
	}
	if v != *c.ExpectedVersion {
		return "", unit, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return status, unit, nil
}

func (m Reports) submit(ctx context.Context, c *ops.Call, in *SubmitInput) (Job, error) {
	status, unit, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "in_progress" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	draft, err := m.draftOf(ctx, c, in.JobID, true)
	if err != nil {
		return Job{}, err
	}
	if draft == nil || draft.Version != in.ReportVersion {
		return Job{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}
	if err := m.decorate(ctx, c, draft); err != nil {
		return Job{}, err
	}
	_, _, scope, err := m.Units.UnitService(ctx, c, unit)
	if err != nil {
		return Job{}, err
	}
	if fe := submitErrors(draft, Components(scope), c.Now); len(fe) > 0 {
		return Job{}, apperr.Fields(fe)
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.work_reports SET state = 'submitted', submitted_at = $3, updated_at = $3 WHERE id = $1 AND version = $2`,
		draft.ID, draft.Version, c.Now); err != nil {
		return Job{}, err
	}
	var raw []byte
	if err := c.Tx.QueryRow(ctx, `SELECT time_on_site FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&raw); err != nil {
		return Job{}, err
	}
	t := TimeOnSite{Pauses: []Pause{}}
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &t)
	}
	now := c.Now
	t.FinishedAt = &now
	if from := firstNonNil(t.ArrivedAt, t.StartedAt); from != nil {
		d := now.Sub(*from)
		for _, p := range t.Pauses {
			if p.To != nil {
				d -= p.To.Sub(p.From)
			}
		}
		mins := int(d / time.Minute)
		t.OnSiteMinutes = &mins
	}
	tos, _ := json.Marshal(t)
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'submitted', time_on_site = $2, draft_report_version = NULL, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1`, in.JobID, tos); err != nil {
		return Job{}, err
	}
	if err := m.reportEvent(ctx, c, in.JobID, "job.submitted", draft.ID, draft.Version); err != nil {
		return Job{}, err
	}
	return m.Jobs.finishScoped(ctx, c, in.JobID, "jobs.submit", "ReportSubmitted", "")
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
func submitErrors(x *Report, components []string, now time.Time) map[string]string {
	fe := map[string]string{}
	keys := []string{}
	for _, it := range x.Items {
		keys = append(keys, it.ComponentKey)
		if it.Result == nil {
			fe["items"] = "errors.result_required"
		} else if *it.Result != "normal" && (it.Reason == nil || utf8.RuneCountInString(*it.Reason) < 1) {
			fe["items"] = "errors.reason_required"
		}
	}
	a, b := slices.Clone(keys), slices.Clone(components)
	slices.Sort(a)
	slices.Sort(b)
	if !slices.Equal(a, b) {
		fe["items"] = "errors.components_mismatch"
	}
	if n := utf8.RuneCountInString(x.WorkText); n < 10 || n > 4000 {
		fe["workText"] = "error.length"
	}
	switch {
	case x.NextAction == nil:
		fe["nextAction"] = "error.required"
	case x.NextAction.Kind == "follow_up" && (x.NextAction.Date == nil || !x.NextAction.Date.After(now) || x.NextAction.Note == nil ||
		utf8.RuneCountInString(*x.NextAction.Note) < 1):
		fe["nextAction"] = "error.invalid"
	}
	for _, p := range x.Parts {
		if p.Quantity < 1 || p.Quantity > 999 {
			fe["parts"] = "error.range"
		}
	}
	for _, a := range x.AttachmentRefs {
		if a.Status != "ready" {
			fe["attachmentIds"] = "errors.attachment_not_ready"
		}
	}
	for _, mm := range x.Measurements {
		if mm.QualityReason != nil && *mm.QualityReason == "unit_mismatch" {
			fe["measurements"] = "errors.unit_mismatch"
		}
	}
	return fe
}

func (m Reports) reportEvent(ctx context.Context, c *ops.Call, job uuid.UUID, action string, report uuid.UUID, version int) error {
	_, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.job_events (id, tenant_id, job_id, actor_user_id, action, report_id, report_version, occurred_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7)`, uuid.Must(uuid.NewV7()), job, c.Principal.UserID, action, report, version, c.Now)
	return err
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ReportVersion < 1 {
		fe["reportVersion"] = "error.required"
	}
	if in.Decision != "accept" && in.Decision != "return" {
		fe["decision"] = "error.invalid"
	}
	if in.ReviewMode != "normal" && in.ReviewMode != "hq_escalation" {
		fe["reviewMode"] = "error.invalid"
	}
	if in.Reason != nil {
		trimmedReason(in.Reason, 1000, "reason", fe)
	}
	if (in.Decision == "return" || in.ReviewMode == "hq_escalation") && in.Reason == nil {
		fe["reason"] = "error.required"
	}
	return fe
}

func (m Reports) review(ctx context.Context, c *ops.Call, in *ReviewInput) (Job, error) {
	var contractor *uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT contractor_org_id FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&contractor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Job{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		return Job{}, err
	}
	switch c.Principal.Role {
	case "contractor":
		if contractor == nil || *contractor != c.Principal.OrgID {
			return Job{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if in.ReviewMode != "normal" {
			return Job{}, apperr.Fields(map[string]string{"reviewMode": "error.invalid"})
		}
	default: // admin
		if (contractor == nil) != (in.ReviewMode == "normal") {
			return Job{}, apperr.E(apperr.Forbidden, "errors.review_mode")
		}
	}
	status, _, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	var id uuid.UUID
	var author uuid.UUID
	var state string
	err = c.Tx.QueryRow(ctx, `SELECT id, author_id, state FROM maintenance.work_reports WHERE job_id = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`, in.JobID).Scan(&id, &author, &state)
	if errors.Is(err, pgx.ErrNoRows) || status != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if err != nil {
		return Job{}, err
	}
	var latest int
	if err := c.Tx.QueryRow(ctx, `SELECT max(version) FROM maintenance.work_reports WHERE id = $1`, id).Scan(&latest); err != nil {
		return Job{}, err
	}
	if latest != in.ReportVersion || state != "submitted" {
		return Job{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}
	if author == c.Principal.UserID {
		return Job{}, apperr.E(apperr.Forbidden, "errors.self_review")
	}
	reportState, jobStatus := "accepted", "completed"
	if in.Decision == "return" {
		reportState, jobStatus = "returned", "rework_requested"
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.work_reports SET state = $3, accepted_at = CASE WHEN $3 = 'accepted' THEN $4::timestamptz END, updated_at = $4
		WHERE id = $1 AND version = $2`, id, latest, reportState, c.Now); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.report_reviews (tenant_id, report_id, report_version, reviewer_user_id, decision, reason, occurred_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6)`, id, latest, c.Principal.UserID, in.Decision, in.Reason, c.Now); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = $2, completed_at = CASE WHEN $2 = 'completed' THEN $3::timestamptz ELSE completed_at END,
		version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID, jobStatus, c.Now); err != nil {
		return Job{}, err
	}
	if err := m.reportEvent(ctx, c, in.JobID, "job.reviewed", id, latest); err != nil {
		return Job{}, err
	}
	return m.Jobs.finishScoped(ctx, c, in.JobID, "jobs.review", "ReportReviewed", deref(in.Reason))
}

// ---- jobs.resumeRework ----

// NewDraftFromLatest copies the latest frozen version into a new draft (version + 1) when the job has none.
func NewDraftFromLatest(ctx context.Context, c *ops.Call, job uuid.UUID) error {
	_, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, items, measurements, parts, refrigerant, work_text,
			next_action, created_at, updated_at)
		SELECT id, version + 1, tenant_id, job_id, author_id, 'draft', items, measurements, parts, refrigerant, work_text, next_action, $2, $2
		FROM maintenance.work_reports r WHERE r.job_id = $1 AND NOT EXISTS (SELECT 1 FROM maintenance.work_reports d WHERE d.job_id = $1 AND d.state = 'draft')
		ORDER BY version DESC LIMIT 1`, job, c.Now)
	if err != nil {
		return err
	}
	_, err = c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET draft_report_version = (SELECT version FROM maintenance.work_reports WHERE job_id = $1 AND state = 'draft') WHERE id = $1`, job)
	return err
}

func (m Reports) resumeRework(ctx context.Context, c *ops.Call, in *JobIDInput) (Job, error) {
	status, _, err := m.lockJob(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "rework_requested" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if err := NewDraftFromLatest(ctx, c, in.JobID); err != nil {
		return Job{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = 'in_progress', version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID); err != nil {
		return Job{}, err
	}
	if err := m.Jobs.event(ctx, c, in.JobID, "job.rework_started", nil); err != nil {
		return Job{}, err
	}
	return m.Jobs.finishScoped(ctx, c, in.JobID, "jobs.resumeRework", "ReworkStarted", "")
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.ReportID == uuid.Nil {
		fe["reportId"] = "error.required"
	}
	if in.ReportVersion < 1 {
		fe["reportVersion"] = "error.required"
	}
	return fe
}

func (m Reports) get(ctx context.Context, c *ops.Call, in *GetReportInput) (Report, error) {
	var org uuid.UUID
	var unit uuid.UUID
	var contractor *uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, unit_id, contractor_org_id FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&org, &unit, &contractor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Report{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Report{}, err
	}
	x, err := m.load(ctx, c, in.ReportID, in.ReportVersion)
	if err != nil {
		return x, err
	}
	nf := apperr.E(apperr.NotFound, "error.notFound")
	if x.JobID != in.JobID {
		return Report{}, nf
	}
	switch c.Principal.Role {
	case "client":
		if org != c.Principal.OrgID || x.State != "accepted" {
			return Report{}, nf
		}
	case "contractor":
		if contractor == nil || *contractor != c.Principal.OrgID || x.State == "draft" {
			return Report{}, nf
		}
	case "technician":
		if err := m.Access.TechnicianJob(ctx, c, in.JobID, unit); err != nil {
			return Report{}, err
		}
	}
	return x, nil
}

// RegisterReports binds the work-report operations.
func RegisterReports(r *ops.Registry, m Reports) {
	ops.Register(r, "jobs.saveDraft", m.saveDraft)
	ops.Register(r, "jobs.submit", m.submit)
	ops.Register(r, "jobs.review", m.review)
	ops.Register(r, "jobs.resumeRework", m.resumeRework)
	ops.Register(r, "reports.get", m.get)
}
