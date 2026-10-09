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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
)

// WarrantyUnits returns a unit's warranty end (Assets).
type WarrantyUnits interface {
	WarrantyEnd(ctx context.Context, c *ops.Call, unit uuid.UUID) (*time.Time, error)
}

// FollowUps is the IR127 operation set.
type FollowUps struct {
	Jobs     Jobs
	Blobs    blob.Store
	Warranty WarrantyUnits
}

// RatingTags are the allowed JobRating tags (DD-C17).
var RatingTags = []string{"On time", "Clean work", "Explained clearly", "Polite", "Fixed the problem"}

// Rating is JobRating.
type Rating struct {
	Stars         int       `json:"stars"`
	Tags          []string  `json:"tags"`
	Comment       *string   `json:"comment"`
	RatedAt       time.Time `json:"ratedAt"`
	EditableUntil time.Time `json:"editableUntil"`
}

type jobState struct {
	status      string
	org, unit   uuid.UUID
	completedAt *time.Time
	rating      []byte
	followUp    *string
	claims      []byte
}

func (m FollowUps) lock(ctx context.Context, c *ops.Call, id uuid.UUID) (jobState, error) {
	var s jobState
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT status, customer_org_id, unit_id, completed_at, rating, follow_up_class, warranty_claims, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, id).
		Scan(&s.status, &s.org, &s.unit, &s.completedAt, &s.rating, &s.followUp, &s.claims, &v)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "client" && s.org != c.Principal.OrgID) {
		return s, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return s, err
	}
	if v != *c.ExpectedVersion {
		return s, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return s, nil
}

func (m FollowUps) bump(ctx context.Context, c *ops.Call, id uuid.UUID, set string, args ...any) error {
	if set != "" {
		set += ", "
	}
	_, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET `+set+`version = version + 1, updated_at = platform.app_now() WHERE id = $1`, append([]any{id}, args...)...)
	return err
}

func (m FollowUps) done(ctx context.Context, c *ops.Call, id uuid.UUID, action, op, event, reason string) (Job, error) {
	if err := m.Jobs.event(ctx, c, id, action, nil); err != nil {
		return Job{}, err
	}
	return m.Jobs.finish(ctx, c, id, op, event, reason)
}

// ---- jobs.rate ----

// RateInput is jobs.rate input.
type RateInput struct {
	JobID   uuid.UUID `json:"jobId"`
	Stars   int       `json:"stars"`
	Tags    []string  `json:"tags"`
	Comment *string   `json:"comment,omitempty"`
}

// Validate implements ops.Validator.
func (in *RateInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.Stars < 1 || in.Stars > 5 {
		fe["stars"] = "error.range"
	}
	seen := map[string]bool{}
	if in.Tags == nil {
		fe["tags"] = "error.required"
	}
	for _, t := range in.Tags {
		if !slices.Contains(RatingTags, t) || seen[t] {
			fe["tags"] = "error.invalid"
		}
		seen[t] = true
	}
	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > 1000 {
			fe["comment"] = "error.length"
		}
		if *in.Comment == "" {
			in.Comment = nil
		}
	}
	return fe
}

// @Summary		jobs.rate (write)
// @ID				jobs.rate
// @Description	Authorization: client:self:completed
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR110 stars 1–5 required; tags from a fixed list; comment 0–1000; editable for 7 days
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C17
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get)"
// @Param			request				body		RateInput	true	"input"
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
// @Router			/v1/ops/jobs.rate [post]
func (m FollowUps) rate(ctx context.Context, c *ops.Call, in *RateInput) (Job, error) {
	s, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if s.status != "completed" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	r := Rating{Stars: in.Stars, Tags: in.Tags, Comment: in.Comment, RatedAt: c.Now, EditableUntil: c.Now.Add(7 * 24 * time.Hour)}
	first := len(s.rating) == 0 || string(s.rating) == "null"
	if !first {
		var prev Rating
		if err := json.Unmarshal(s.rating, &prev); err != nil {
			return Job{}, err
		}
		if !c.Now.Before(prev.EditableUntil) {
			return Job{}, apperr.E(apperr.Conflict, "errors.rating_locked")
		}
		r.RatedAt, r.EditableUntil = prev.RatedAt, prev.EditableUntil
	}
	raw, _ := json.Marshal(r)
	if err := m.bump(ctx, c, in.JobID, "rating = $2, customer_confirmed_at = COALESCE(customer_confirmed_at, $3)", raw, c.Now); err != nil {
		return Job{}, err
	}
	return m.done(ctx, c, in.JobID, "job.rated", "jobs.rate", "JobRated", "")
}

// ---- jobs.reportProblem ----

// BlobInput is BlobInput of service-contracts.ts (bytes as base64 in JSON).
type BlobInput struct {
	Name  string `json:"name"`
	Mime  string `json:"mime"`
	Size  int    `json:"size"`
	Bytes []byte `json:"bytes"`
}

// ProblemInput is jobs.reportProblem input.
type ProblemInput struct {
	JobID         uuid.UUID   `json:"jobId"`
	ReasonCode    string      `json:"reasonCode"`
	Details       string      `json:"details"`
	Photos        []BlobInput `json:"photos"`
	PreferredSlot *Slot       `json:"preferredSlot"`
}

var problemCodes = map[string]bool{"same_problem": true, "new_damage": true, "not_completed": true, "other": true}

// ValidBlob checks one image upload (JPEG/PNG, 1 byte – max bytes, size equal to the payload).
func ValidBlob(b BlobInput, max int) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(b.Name))
	return n >= 1 && n <= 200 && (b.Mime == "image/jpeg" || b.Mime == "image/png") && b.Size == len(b.Bytes) && b.Size >= 1 && b.Size <= max
}

// Validate implements ops.Validator.
func (in *ProblemInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if !problemCodes[in.ReasonCode] {
		fe["reasonCode"] = "error.invalid"
	}
	in.Details = strings.TrimSpace(in.Details)
	if n := utf8.RuneCountInString(in.Details); n < 10 || n > 2000 {
		fe["details"] = "error.length"
	}
	if in.Photos == nil || len(in.Photos) > 5 {
		fe["photos"] = "error.count"
	}
	for _, p := range in.Photos {
		if !ValidBlob(p, 5<<20) {
			fe["photos"] = "error.invalidFile"
		}
	}
	return fe
}

// @Summary		jobs.reportProblem (write)
// @ID				jobs.reportProblem
// @Description	Authorization: client:self:completed
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR110 details 10–2000; up to 5 photos; creates a requested follow-up job
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C17
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get)"
// @Param			request				body		ProblemInput	true	"input"
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
// @Router			/v1/ops/jobs.reportProblem [post]
func (m FollowUps) reportProblem(ctx context.Context, c *ops.Call, in *ProblemInput) (Job, error) {
	s, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if s.status != "completed" || s.completedAt == nil || !c.Now.Before(s.completedAt.Add(7*24*time.Hour)) {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	var requested Slot
	preferred := []Slot{}
	if in.PreferredSlot != nil {
		if !PreferredSlotsOK(c.Now, []Slot{*in.PreferredSlot}) {
			return Job{}, apperr.Fields(map[string]string{"preferredSlot": "error.slotRules"})
		}
		requested, preferred = *in.PreferredSlot, []Slot{*in.PreferredSlot}
	} else {
		y, mo, d := c.Now.In(kualaLumpur).Date()
		start := time.Date(y, mo, d+1, 9, 0, 0, 0, kualaLumpur)
		requested = Slot{start.UTC(), start.Add(3 * time.Hour).UTC()}
	}
	pref, _ := json.Marshal(preferred)
	var id uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status, origin, symptom, requested_slot, preferred_slots,
		preference_round, due_at, follow_up_of_job_id, follow_up_class) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'reactive', 'requested', 'client_request',
		$3, tstzrange($4, $5), $6, 1, $5, $7, 'pending') RETURNING id`, s.unit, s.org, in.Details, requested.StartAt, requested.EndAt, pref, in.JobID).Scan(&id); err != nil {
		return Job{}, err
	}
	for _, p := range in.Photos {
		aid := uuid.New()
		key := "jobs/" + id.String() + "/" + aid.String()
		if err := m.Blobs.Put(ctx, key, p.Mime, p.Bytes); err != nil {
			return Job{}, err
		}
		if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.attachments (id, tenant_id, job_id, object_key, name, mime, size_bytes, status, uploaded_by)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'ready', $7)`, aid, id, key, strings.TrimSpace(p.Name), p.Mime, p.Size, c.Principal.UserID); err != nil {
			return Job{}, err
		}
	}
	if err := m.Jobs.event(ctx, c, id, "job.created", nil); err != nil {
		return Job{}, err
	}
	if err := m.bump(ctx, c, in.JobID, ""); err != nil {
		return Job{}, err
	}
	if err := m.Jobs.event(ctx, c, in.JobID, "job.problem_reported", nil); err != nil {
		return Job{}, err
	}
	j, err := m.Jobs.detail(ctx, c, id)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: id, Type: "JobCreated", Payload: map[string]any{"followUpOfJobId": in.JobID, "reasonCode": in.ReasonCode}})
	c.Audit(ops.AuditEntry{Action: "jobs.reportProblem", TargetKind: "job", TargetID: in.JobID.String(), PreviousVersion: c.ExpectedVersion, Reason: in.ReasonCode})
	return j, nil
}

// ---- jobs.classifyFollowUp ----

// ClassifyInput is jobs.classifyFollowUp input.
type ClassifyInput struct {
	JobID          uuid.UUID `json:"jobId"`
	Classification string    `json:"classification"`
	Reason         string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ClassifyInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.Classification != "rework" && in.Classification != "new_request" {
		fe["classification"] = "error.invalid"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// @Summary		jobs.classifyFollowUp (write)
// @ID				jobs.classifyFollowUp
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR110 only followUpClass=pending; reason 1–1000
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get)"
// @Param			request				body		ClassifyInput	true	"input"
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
// @Router			/v1/ops/jobs.classifyFollowUp [post]
func (m FollowUps) classify(ctx context.Context, c *ops.Call, in *ClassifyInput) (Job, error) {
	s, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if s.followUp == nil || *s.followUp != "pending" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if err := m.bump(ctx, c, in.JobID, "follow_up_class = $2, follow_up_reason = $3", in.Classification, in.Reason); err != nil {
		return Job{}, err
	}
	return m.done(ctx, c, in.JobID, "job.follow_up_classified", "jobs.classifyFollowUp", "FollowUpClassified", in.Reason)
}

// ---- jobs.saveCost ----

// CostLine is CostLine of service-contracts.ts.
type CostLine struct {
	AmountMinor int    `json:"amountMinor"`
	Currency    string `json:"currency"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
}

// CostInput is jobs.saveCost input.
type CostInput struct {
	JobID     uuid.UUID  `json:"jobId"`
	CostLines []CostLine `json:"costLines"`
}

// Validate implements ops.Validator.
func (in *CostInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.CostLines == nil || len(in.CostLines) > 50 {
		fe["costLines"] = "error.count"
	}
	for i := range in.CostLines {
		l := &in.CostLines[i]
		l.Description = strings.TrimSpace(l.Description)
		n := utf8.RuneCountInString(l.Description)
		if l.AmountMinor < 0 || (l.Currency != "MYR" && l.Currency != "USD") || (l.Kind != "estimate" && l.Kind != "actual") || n < 1 || n > 200 ||
			(l.Visibility != "internal" && l.Visibility != "customer") {
			fe["costLines"] = "error.invalid"
		}
	}
	return fe
}

// @Summary		jobs.saveCost (write)
// @ID				jobs.saveCost
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			request				body		CostInput	true	"input"
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
// @Router			/v1/ops/jobs.saveCost [post]
func (m FollowUps) saveCost(ctx context.Context, c *ops.Call, in *CostInput) (Job, error) {
	s, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if s.status == "cancelled" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	raw, _ := json.Marshal(in.CostLines)
	if err := m.bump(ctx, c, in.JobID, "costs = $2", raw); err != nil {
		return Job{}, err
	}
	return m.done(ctx, c, in.JobID, "job.costs_saved", "jobs.saveCost", "JobCostsSaved", "")
}

// ---- jobs.extendAccess ----

// ExtendInput is jobs.extendAccess input.
type ExtendInput struct {
	JobID            uuid.UUID `json:"jobId"`
	AccessValidUntil time.Time `json:"accessValidUntil"`
	Reason           string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ExtendInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.AccessValidUntil.IsZero() {
		fe["accessValidUntil"] = "error.required"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// @Summary		jobs.extendAccess (write)
// @ID				jobs.extendAccess
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A06
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
// @Param			request				body		ExtendInput	true	"input"
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
// @Router			/v1/ops/jobs.extendAccess [post]
func (m FollowUps) extendAccess(ctx context.Context, c *ops.Call, in *ExtendInput) (Job, error) {
	if _, err := m.lock(ctx, c, in.JobID); err != nil {
		return Job{}, err
	}
	var offer uuid.UUID
	var until time.Time
	err := c.Tx.QueryRow(ctx, `SELECT id, access_valid_until FROM maintenance.offers WHERE job_id = $1 AND decision = 'accept' ORDER BY decided_at DESC LIMIT 1 FOR UPDATE`, in.JobID).
		Scan(&offer, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.Conflict, "errors.no_accepted_offer")
	}
	if err != nil {
		return Job{}, err
	}
	if !in.AccessValidUntil.After(until) || !in.AccessValidUntil.After(c.Now) {
		return Job{}, apperr.Fields(map[string]string{"accessValidUntil": "error.mustExtend"})
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.offers SET access_valid_until = $2, version = version + 1, updated_at = platform.app_now() WHERE id = $1`, offer, in.AccessValidUntil); err != nil {
		return Job{}, err
	}
	if err := m.bump(ctx, c, in.JobID, ""); err != nil {
		return Job{}, err
	}
	return m.done(ctx, c, in.JobID, "offer.access_extended", "jobs.extendAccess", "AccessExtended", in.Reason)
}

// ---- jobs.recordWarrantyClaim ----

// ClaimInput is jobs.recordWarrantyClaim input.
type ClaimInput struct {
	JobID       uuid.UUID `json:"jobId"`
	PartLabel   string    `json:"partLabel"`
	AmountMinor int       `json:"amountMinor"`
	Currency    string    `json:"currency"`
	Reason      string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ClaimInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	trimmedReason(&in.PartLabel, 120, "partLabel", fe)
	if in.AmountMinor < 1 {
		fe["amountMinor"] = "error.range"
	}
	if in.Currency != "MYR" && in.Currency != "USD" {
		fe["currency"] = "error.invalid"
	}
	trimmedReason(&in.Reason, 1000, "reason", fe)
	return fe
}

// WarrantyClaim is WarrantyClaim of service-contracts.ts.
type WarrantyClaim struct {
	ID          uuid.UUID  `json:"id"`
	PartLabel   string     `json:"partLabel"`
	AmountMinor int        `json:"amountMinor"`
	Currency    string     `json:"currency"`
	State       string     `json:"state"`
	FiledAt     *time.Time `json:"filedAt"`
	Reason      *string    `json:"reason"`
}

// @Summary		jobs.recordWarrantyClaim (write)
// @ID				jobs.recordWarrantyClaim
// @Description	Authorization: admin:job.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR111 only for completed jobs whose replaced part was under warranty; amount > 0 in contract currency
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A19
// @Tags			jobs
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get)"
// @Param			request				body		ClaimInput	true	"input"
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
// @Router			/v1/ops/jobs.recordWarrantyClaim [post]
func (m FollowUps) recordWarrantyClaim(ctx context.Context, c *ops.Call, in *ClaimInput) (Job, error) {
	s, err := m.lock(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if s.status != "completed" || s.completedAt == nil {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	end, err := m.Warranty.WarrantyEnd(ctx, c, s.unit)
	if err != nil {
		return Job{}, err
	}
	if end == nil || end.Before(*s.completedAt) {
		return Job{}, apperr.Fields(map[string]string{"jobId": "errors.outside_warranty"})
	}
	claims := []WarrantyClaim{}
	if len(s.claims) > 0 {
		if err := json.Unmarshal(s.claims, &claims); err != nil {
			return Job{}, err
		}
	}
	now := c.Now
	claims = append(claims, WarrantyClaim{ID: uuid.New(), PartLabel: in.PartLabel, AmountMinor: in.AmountMinor, Currency: in.Currency, State: "filed", FiledAt: &now, Reason: &in.Reason})
	raw, _ := json.Marshal(claims)
	if err := m.bump(ctx, c, in.JobID, "warranty_claims = $2", raw); err != nil {
		return Job{}, err
	}
	return m.done(ctx, c, in.JobID, "job.warranty_claim_filed", "jobs.recordWarrantyClaim", "WarrantyClaimFiled", in.Reason)
}

// ConfirmUnrated applies IR110: completed jobs without a rating are confirmed 7 days after completedAt.
func ConfirmUnrated(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	tag, err := tx.Exec(ctx, `UPDATE maintenance.jobs SET customer_confirmed_at = completed_at + interval '7 days', version = version + 1, updated_at = platform.app_now()
		WHERE status = 'completed' AND customer_confirmed_at IS NULL AND rating IS NULL AND completed_at + interval '7 days' <= $1`, now)
	return int(tag.RowsAffected()), err
}

// RegisterFollowUps binds the IR127 operations.
func RegisterFollowUps(r *ops.Registry, m FollowUps) {
	ops.Register(r, "jobs.rate", m.rate)
	ops.Register(r, "jobs.reportProblem", m.reportProblem)
	ops.Register(r, "jobs.classifyFollowUp", m.classify)
	ops.Register(r, "jobs.saveCost", m.saveCost)
	ops.Register(r, "jobs.extendAccess", m.extendAccess)
	ops.Register(r, "jobs.recordWarrantyClaim", m.recordWarrantyClaim)
}
