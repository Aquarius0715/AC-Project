package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg" // ImageSizeOK reads JPEG headers
	_ "image/png"  // and PNG headers
	"slices"
	"strings"
	"time"
	"unicode"
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

// ratingTags are the allowed JobRating tags (DD-C17).
func ratingTags() []string {
	return []string{
		"On time",
		"Clean work",
		"Explained clearly",
		"Polite",
		"Fixed the problem",
	}
}

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

func (FollowUps) lock(ctx context.Context, call *ops.Call, jobID uuid.UUID) (jobState, error) {
	var (
		state   jobState
		version int
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, customer_org_id, unit_id, completed_at, rating, follow_up_class,
		warranty_claims, version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, jobID).
		Scan(&state.status, &state.org, &state.unit, &state.completedAt, &state.rating, &state.followUp, &state.claims,
			&version)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == roleClient && state.org != call.Principal.OrgID) {
		return state, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return state, apperr.Wrap(err, "select maintenance.jobs")
	}

	if version != *call.ExpectedVersion {
		return state, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return state, nil
}

func (FollowUps) bump(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	set string,
	args ...any,
) error {
	if set != "" {
		set += ", "
	}

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET `+set+`version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		append([]any{jobID}, args...)...)

	return apperr.Wrap(err, "update maintenance.jobs")
}

func (m FollowUps) done(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	action, operation, event, reason string,
) (Job, error) {
	err := m.Jobs.event(ctx, call, jobID, action, nil)
	if err != nil {
		return Job{}, err
	}

	return m.Jobs.finish(ctx, call, jobID, operation, event, reason)
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
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.Stars < 1 || in.Stars > 5 {
		fieldErrs["stars"] = apperr.KeyRange
	}

	seen := map[string]bool{}

	if in.Tags == nil {
		fieldErrs["tags"] = apperr.KeyRequired
	}

	for _, t := range in.Tags {
		if !slices.Contains(ratingTags(), t) || seen[t] {
			fieldErrs["tags"] = apperr.KeyInvalid
		}

		seen[t] = true
	}

	if in.Comment != nil {
		*in.Comment = strings.TrimSpace(*in.Comment)
		if utf8.RuneCountInString(*in.Comment) > maxReasonLength {
			fieldErrs["comment"] = apperr.KeyLength
		}

		if *in.Comment == "" {
			in.Comment = nil
		}
	}

	return fieldErrs
}

// rate serves jobs.rate.
//
//	@Summary		jobs.rate (write)
//	@ID				jobs.rate
//	@Description	Authorization: client:self:completed
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR110 stars 1–5 required; tags from a fixed list; comment 0–1000; editable for 7 days
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C17
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		RateInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/rate [post]
func (m FollowUps) rate(ctx context.Context, c *ops.Call, in *RateInput) (MaintenanceJob, error) {
	return written(m.rateJob(ctx, c, in))
}

// rateJob is rate answering the job with its detail extras.
func (m FollowUps) rateJob(ctx context.Context, call *ops.Call, input *RateInput) (Job, error) {
	state, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if state.status != statusCompleted {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	rating := Rating{
		Stars:         input.Stars,
		Tags:          input.Tags,
		Comment:       input.Comment,
		RatedAt:       call.Now,
		EditableUntil: call.Now.Add(7 * 24 * time.Hour),
	}

	first := len(state.rating) == 0 || string(state.rating) == jsonNull
	if !first {
		var prev Rating

		err := json.Unmarshal(state.rating, &prev)
		if err != nil {
			return Job{}, apperr.Wrap(err, "json.Unmarshal")
		}

		if !call.Now.Before(prev.EditableUntil) {
			return Job{}, apperr.E(apperr.Conflict, "errors.rating_locked")
		}

		rating.RatedAt, rating.EditableUntil = prev.RatedAt, prev.EditableUntil
	}

	raw, marshalErr := json.Marshal(rating)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	bumpErr := m.bump(
		ctx,
		call,
		input.JobID,
		"rating = $2, customer_confirmed_at = COALESCE(customer_confirmed_at, $3)",
		raw,
		call.Now,
	)
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	return m.done(ctx, call, input.JobID, "job.rated", "jobs.rate", "JobRated", "")
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

// problemCodes are the reasons a customer reports a problem with a completed job for (ProblemInput.reasonCode).
func problemCodes() map[string]bool {
	return map[string]bool{
		"same_problem":  true,
		"new_damage":    true,
		"not_completed": true,
		codeOther:       true,
	}
}

// The file types an upload may have: JPEG and PNG photos, and PDF certificate documents.
const (
	mimeJPEG = "image/jpeg"
	mimePNG  = "image/png"
	mimePDF  = "application/pdf"
)

// ValidBlob checks one image upload (a valid name, JPEG/PNG whose bytes are that type, 1 byte – maxBytes, size equal
// to the payload, and a header within the pixel limits).
func ValidBlob(blobInput BlobInput, maxBytes int) bool {
	return ValidFileName(blobInput.Name) &&
		(blobInput.Mime == mimeJPEG || blobInput.Mime == mimePNG) &&
		blobInput.Size == len(blobInput.Bytes) &&
		blobInput.Size >= 1 &&
		blobInput.Size <= maxBytes &&
		ContentMatches(blobInput.Mime, blobInput.Bytes) &&
		ImageSizeOK(blobInput.Mime, blobInput.Bytes)
}

// The pixel limits of an uploaded image (IR389): at most 16,384 pixels a side and 50 megapixels, more than a phone
// camera's full resolution in 5 MiB. A few megabytes of PNG or JPEG can declare billions of pixels, and decoding them
// would stall the browser of whoever opens the photo.
const (
	maxImageSide   = 16384
	maxImagePixels = 50_000_000
)

// ImageSizeOK reads an image's header, without decoding the image, and checks that it is the declared type and within
// the pixel limits.
func ImageSizeOK(mime string, b []byte) bool {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || "image/"+format != mime {
		return false
	}

	return cfg.Width >= 1 && cfg.Height >= 1 && cfg.Width <= maxImageSide &&
		cfg.Height <= maxImageSide &&
		cfg.Width*cfg.Height <= maxImagePixels
}

// ValidFileName checks the name of an upload. The apps show it, and offer it as the file name when a photo is opened.
// It has 1–200 characters after trimming, and no control characters, path separators or bidirectional formatting
// characters. Those could break the header that names the file, or make "photo<U+202E>gpj.exe" read as
// "photoexe.jpg" (IR368).
func ValidFileName(name string) bool {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > 200 {
		return false
	}

	for _, r := range name {
		if unicode.IsControl(r) || r == '/' || r == '\\' || bidiFormat(r) {
			return false
		}
	}

	return true
}

// bidiFormat reports the Unicode bidirectional formatting characters: marks, embeddings, overrides and isolates.
func bidiFormat(r rune) bool {
	return r == '\u200e' || r == '\u200f' || r == '\u061c' || (r >= '\u202a' && r <= '\u202e') ||
		(r >= '\u2066' && r <= '\u2069')
}

// ContentMatches reports whether a file's leading bytes are of its declared type — the PNG signature, the JPEG start
// of image marker or the PDF header — so a file is stored and served only as what it is (IR308).
func ContentMatches(mime string, content []byte) bool {
	switch mime {
	case mimePNG:
		return bytes.HasPrefix(content, []byte("\x89PNG\r\n\x1a\n"))
	case mimeJPEG:
		return bytes.HasPrefix(content, []byte{0xFF, 0xD8, 0xFF})
	case mimePDF:
		return bytes.HasPrefix(content, []byte("%PDF-"))
	}

	return false
}

// Validate implements ops.Validator.
func (in *ProblemInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if !problemCodes()[in.ReasonCode] {
		fieldErrs["reasonCode"] = apperr.KeyInvalid
	}

	in.Details = strings.TrimSpace(in.Details)
	if n := utf8.RuneCountInString(in.Details); n < 10 || n > 2000 {
		fieldErrs["details"] = apperr.KeyLength
	}

	if in.Photos == nil || len(in.Photos) > 5 {
		fieldErrs["photos"] = apperr.KeyCount
	}

	for _, p := range in.Photos {
		if !ValidBlob(p, photoMaxBytes) {
			fieldErrs["photos"] = apperr.KeyInvalidFile
		}
	}

	return fieldErrs
}

// reportProblem serves jobs.reportProblem.
//
//	@Summary		jobs.reportProblem (write)
//	@ID				jobs.reportProblem
//	@Description	Authorization: client:self:completed
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR110 details 10–2000; up to 5 photos; creates a requested follow-up job
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C17
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			request				body		ProblemInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/report-problem [post]
func (m FollowUps) reportProblem(
	ctx context.Context,
	c *ops.Call,
	in *ProblemInput,
) (MaintenanceJob, error) {
	return written(m.reportProblemJob(ctx, c, in))
}

// A follow-up visit asked for without a preferred time is the next Kuala Lumpur day from 09:00, for three hours.
const (
	followUpHour = 9
	followUpSlot = 3 * time.Hour
)

// reportProblemJob is reportProblem answering the job with its detail extras.
func (m FollowUps) reportProblemJob(
	ctx context.Context,
	call *ops.Call,
	input *ProblemInput,
) (Job, error) {
	state, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if state.status != statusCompleted || state.completedAt == nil ||
		!call.Now.Before(state.completedAt.Add(7*24*time.Hour)) {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	requested, preferred, err := followUpSlots(call.Now, input.PreferredSlot)
	if err != nil {
		return Job{}, err
	}

	pref, marshalErr := json.Marshal(preferred)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	var followUpID uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, type, status,
		origin,
		symptom, requested_slot, preferred_slots,
		preference_round, due_at, follow_up_of_job_id, follow_up_class) VALUES (current_setting('app.tenant_id')::uuid, $1,
		$2, 'reactive', 'requested', 'client_request',
		$3, tstzrange($4, $5), $6, 1, $5, $7, 'pending') RETURNING
		id`, state.unit, state.org, input.Details, requested.StartAt, requested.EndAt, pref, input.JobID).
		Scan(&followUpID)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "insert maintenance.jobs")
	}

	err = m.attachPhotos(ctx, call, followUpID, input.Photos)
	if err != nil {
		return Job{}, err
	}

	eventErr := m.Jobs.event(ctx, call, followUpID, "job.created", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	bumpErr := m.bump(ctx, call, input.JobID, "")
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	eventErr = m.Jobs.event(ctx, call, input.JobID, "job.problem_reported", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	job, err := m.Jobs.detail(ctx, call, followUpID)
	if err != nil {
		return job, err
	}

	call.Emit(outboxEvent("job", followUpID, "JobCreated", map[string]any{
		"followUpOfJobId": input.JobID,
		"reasonCode":      input.ReasonCode,
	}))
	call.Audit(
		auditEntry(
			"jobs.reportProblem",
			"job",
			input.JobID,
			call.ExpectedVersion,
			nil,
			input.ReasonCode,
		),
	)

	return job, nil
}

// followUpSlots are the requested visit of a follow-up and its preferred times: the customer's preferred time when it
// follows the slot rules (IR113), else the next Kuala Lumpur day from 09:00 for three hours with no preference.
func followUpSlots(now time.Time, preferredSlot *Slot) (Slot, []Slot, error) {
	var requested Slot

	preferred := []Slot{}

	if preferredSlot != nil {
		if !PreferredSlotsOK(now, []Slot{*preferredSlot}) {
			return requested, preferred, apperr.Fields(
				map[string]string{"preferredSlot": apperr.KeySlotRules},
			)
		}

		requested, preferred = *preferredSlot, []Slot{*preferredSlot}
	} else {
		y, mo, d := now.In(kualaLumpur).Date()
		start := time.Date(y, mo, d+1, followUpHour, 0, 0, 0, kualaLumpur)
		requested = Slot{start.UTC(), start.Add(followUpSlot).UTC()}
	}

	return requested, preferred, nil
}

// attachPhotos stores the problem's photos as ready attachments of the follow-up job.
func (m FollowUps) attachPhotos(
	ctx context.Context,
	call *ops.Call,
	job uuid.UUID,
	photos []BlobInput,
) error {
	for _, blobInput := range photos {
		aid := uuid.New()

		key := "jobs/" + job.String() + "/" + aid.String()

		err := m.Blobs.Put(ctx, key, blobInput.Mime, blobInput.Bytes)
		if err != nil {
			return apperr.Wrap(err, "blob.Store.Put")
		}

		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO maintenance.attachments (id, tenant_id, job_id, object_key, name, mime, size_bytes, status, uploaded_by)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, 'ready', $7)`,
			aid,
			job,
			key,
			strings.TrimSpace(blobInput.Name),
			blobInput.Mime,
			blobInput.Size,
			call.Principal.UserID,
		)
		if err != nil {
			return apperr.Wrap(err, "insert maintenance.attachments")
		}
	}

	return nil
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
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.Classification != "rework" && in.Classification != "new_request" {
		fieldErrs["classification"] = apperr.KeyInvalid
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

	return fieldErrs
}

// classify serves jobs.classifyFollowUp.
//
//	@Summary		jobs.classifyFollowUp (write)
//	@ID				jobs.classifyFollowUp
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR110 only followUpClass=pending; reason 1–1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			request				body		ClassifyInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/classify-follow-up [post]
func (m FollowUps) classify(
	ctx context.Context,
	c *ops.Call,
	in *ClassifyInput,
) (MaintenanceJob, error) {
	return written(m.classifyJob(ctx, c, in))
}

// classifyJob is classify answering the job with its detail extras.
func (m FollowUps) classifyJob(
	ctx context.Context,
	call *ops.Call,
	input *ClassifyInput,
) (Job, error) {
	s, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if s.followUp == nil || *s.followUp != "pending" {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	bumpErr := m.bump(
		ctx,
		call,
		input.JobID,
		"follow_up_class = $2, follow_up_reason = $3",
		input.Classification,
		input.Reason,
	)
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	return m.done(
		ctx,
		call,
		input.JobID,
		"job.follow_up_classified",
		"jobs.classifyFollowUp",
		"FollowUpClassified",
		input.Reason,
	)
}

// ---- jobs.saveCost ----

// The currencies of the amounts of cost lines, warranty claims and rate cards.
const (
	currencyMYR = "MYR"
	currencyUSD = "USD"
)

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
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.CostLines == nil || len(in.CostLines) > 50 {
		fieldErrs["costLines"] = apperr.KeyCount
	}

	for i := range in.CostLines {
		costLine := &in.CostLines[i]
		costLine.Description = strings.TrimSpace(costLine.Description)

		n := utf8.RuneCountInString(costLine.Description)
		if costLine.AmountMinor < 0 ||
			(costLine.Currency != currencyMYR && costLine.Currency != currencyUSD) ||
			(costLine.Kind != "estimate" && costLine.Kind != "actual") ||
			n < 1 ||
			n > 200 ||
			(costLine.Visibility != visibilityInternal && costLine.Visibility != visibilityCustomer) {
			fieldErrs["costLines"] = apperr.KeyInvalid
		}
	}

	return fieldErrs
}

// saveCost serves jobs.saveCost.
//
//	@Summary		jobs.saveCost (write)
//	@ID				jobs.saveCost
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
//	@Param			request				body		CostInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/cost-lines [put]
func (m FollowUps) saveCost(
	ctx context.Context,
	c *ops.Call,
	in *CostInput,
) (MaintenanceJob, error) {
	return written(m.saveCostJob(ctx, c, in))
}

// saveCostJob is saveCost answering the job with its detail extras.
func (m FollowUps) saveCostJob(ctx context.Context, call *ops.Call, input *CostInput) (Job, error) {
	s, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if s.status == statusCancelled {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	raw, marshalErr := json.Marshal(input.CostLines)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	bumpErr := m.bump(ctx, call, input.JobID, "costs = $2", raw)
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	return m.done(ctx, call, input.JobID, "job.costs_saved", "jobs.saveCost", "JobCostsSaved", "")
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
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.AccessValidUntil.IsZero() {
		fieldErrs["accessValidUntil"] = apperr.KeyRequired
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

	return fieldErrs
}

// extendAccess serves jobs.extendAccess.
//
//	@Summary		jobs.extendAccess (write)
//	@ID				jobs.extendAccess
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
//	@Param			request				body		ExtendInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/extend-access [post]
func (m FollowUps) extendAccess(
	ctx context.Context,
	c *ops.Call,
	in *ExtendInput,
) (MaintenanceJob, error) {
	return written(m.extendAccessJob(ctx, c, in))
}

// extendAccessJob is extendAccess answering the job with its detail extras.
func (m FollowUps) extendAccessJob(
	ctx context.Context,
	call *ops.Call,
	input *ExtendInput,
) (Job, error) {
	_, lockErr := m.lock(ctx, call, input.JobID)
	if lockErr != nil {
		return Job{}, lockErr
	}

	var (
		offer, contractor uuid.UUID
		until             time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, access_valid_until, contractor_org_id FROM maintenance.offers WHERE job_id =
		$1
		AND decision = 'accept' ORDER BY decided_at DESC LIMIT 1 FOR UPDATE`, input.JobID).
		Scan(&offer, &until, &contractor)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, apperr.E(apperr.Conflict, "errors.no_accepted_offer")
	}

	if err != nil {
		return Job{}, apperr.Wrap(err, "select maintenance.offers")
	}

	if !input.AccessValidUntil.After(until) || !input.AccessValidUntil.After(call.Now) {
		return Job{}, apperr.Fields(map[string]string{"accessValidUntil": "error.mustExtend"})
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.offers SET access_valid_until = $2, version = version + 1, updated_at = platform.app_now() WHERE
			id = $1`,
		offer,
		input.AccessValidUntil,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.offers")
	}
	// a window that had already ended was frozen into the contractor's history; it is frozen again at the new end (IR458)
	_, execErr = call.Tx.Exec(
		ctx,
		`DELETE FROM maintenance.job_history_snapshots WHERE job_id = $1 AND owner_kind = 'organization' AND owner_id = $2`,
		input.JobID,
		contractor,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "delete maintenance.job_history_snapshots")
	}

	bumpErr := m.bump(ctx, call, input.JobID, "")
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	return m.done(
		ctx,
		call,
		input.JobID,
		"offer.access_extended",
		"jobs.extendAccess",
		"AccessExtended",
		input.Reason,
	)
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
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	trimmedReason(&in.PartLabel, maxNameLength, "partLabel", fieldErrs)

	if in.AmountMinor < 1 {
		fieldErrs["amountMinor"] = apperr.KeyRange
	}

	if in.Currency != currencyMYR && in.Currency != currencyUSD {
		fieldErrs["currency"] = apperr.KeyInvalid
	}

	trimmedReason(&in.Reason, maxReasonLength, fieldReason, fieldErrs)

	return fieldErrs
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

// recordWarrantyClaim serves jobs.recordWarrantyClaim.
//
//	@Summary		jobs.recordWarrantyClaim (write)
//	@ID				jobs.recordWarrantyClaim
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 only for completed jobs whose replaced part was under warranty; amount > 0 in contract \
//	@Description	currency
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A19
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		ClaimInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/warranty-claims [post]
func (m FollowUps) recordWarrantyClaim(
	ctx context.Context,
	c *ops.Call,
	in *ClaimInput,
) (MaintenanceJob, error) {
	return written(m.recordWarrantyClaimJob(ctx, c, in))
}

// recordWarrantyClaimJob is recordWarrantyClaim answering the job with its detail extras.
func (m FollowUps) recordWarrantyClaimJob(
	ctx context.Context,
	call *ops.Call,
	input *ClaimInput,
) (Job, error) {
	state, err := m.lock(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if state.status != statusCompleted || state.completedAt == nil {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	end, err := m.Warranty.WarrantyEnd(ctx, call, state.unit)
	if err != nil {
		return Job{}, apperr.Wrap(err, "maintenance.WarrantyUnits.WarrantyEnd")
	}

	if end == nil || end.Before(*state.completedAt) {
		return Job{}, apperr.Fields(map[string]string{fieldJobID: "errors.outside_warranty"})
	}

	claims := make([]WarrantyClaim, 0, 1)
	if len(state.claims) > 0 {
		err := json.Unmarshal(state.claims, &claims)
		if err != nil {
			return Job{}, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	now := call.Now
	claims = append(
		claims,
		WarrantyClaim{
			ID:          uuid.New(),
			PartLabel:   input.PartLabel,
			AmountMinor: input.AmountMinor,
			Currency:    input.Currency,
			State:       "filed",
			FiledAt:     &now,
			Reason:      &input.Reason,
		},
	)

	raw, marshalErr := json.Marshal(claims)
	if marshalErr != nil {
		return Job{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	bumpErr := m.bump(ctx, call, input.JobID, "warranty_claims = $2", raw)
	if bumpErr != nil {
		return Job{}, bumpErr
	}

	return m.done(
		ctx,
		call,
		input.JobID,
		"job.warranty_claim_filed",
		"jobs.recordWarrantyClaim",
		"WarrantyClaimFiled",
		input.Reason,
	)
}

// ConfirmUnrated applies IR110: completed jobs without a rating are confirmed 7 days after completedAt.
func ConfirmUnrated(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	tag, err := tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET customer_confirmed_at = completed_at + interval '7 days', version = version + 1,
			updated_at = platform.app_now()
		WHERE status = 'completed' AND customer_confirmed_at IS NULL AND rating IS NULL AND completed_at + interval '7 days'
		<= $1`,
		now,
	)

	return int(tag.RowsAffected()), err
}

// RegisterFollowUps binds the IR127 operations.
func RegisterFollowUps(reg *ops.Registry, followUps FollowUps) {
	ops.Register(reg, "jobs.rate", followUps.rate)
	ops.Register(reg, "jobs.reportProblem", followUps.reportProblem)
	ops.Register(reg, "jobs.classifyFollowUp", followUps.classify)
	ops.Register(reg, "jobs.saveCost", followUps.saveCost)
	ops.Register(reg, "jobs.extendAccess", followUps.extendAccess)
	ops.Register(reg, "jobs.recordWarrantyClaim", followUps.recordWarrantyClaim)
}
