package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
)

// Files is the attachment and sign-off operation set (IR129).
type Files struct {
	Reports Reports
	Blobs   blob.Store
}

// attachmentReady is the status of a stored report file.
const attachmentReady = "ready"

// MaxReportAttachments is the provisional DD-T09 limit per report.
const MaxReportAttachments = 10

// lockDraft checks the technician write table and returns the job's current draft locked for update.
func (m Files) lockDraft(
	ctx context.Context,
	call *ops.Call,
	job, report uuid.UUID,
) (*Report, error) {
	var (
		status string
		unit   uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, job).
		Scan(&status, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return nil, apperr.Wrap(err, "select maintenance.jobs")
	}

	technicianJobErr := m.Reports.Access.TechnicianJob(ctx, call, job, unit)
	if technicianJobErr != nil {
		return nil, technicianJobErr
	}

	draft, found, err := m.Reports.currentDraft(ctx, call, job, true)
	if err != nil {
		return nil, err
	}

	if status != statusInProgress || !found || draft.ID != report {
		return nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	return &draft, nil
}

// dropSignOffFiles removes the files of the sign-off of one version of the report (a draft), which is being replaced or
// cleared: the signature or site photo belongs to that sign-off alone, so it is no part of the report any more. Left in
// place it would show among the technician's photos and count toward the 10 attachments of a report (DD-T09). A frozen
// version keeps its sign-off and its files.
func dropSignOffFiles(ctx context.Context, call *ops.Call, report uuid.UUID, version int) error {
	_, err := call.Tx.Exec(
		ctx,
		`DELETE FROM maintenance.attachments WHERE id IN (
			SELECT (sign_off->>'signatureAttachmentId')::uuid FROM maintenance.work_reports WHERE id = $1 AND version = $2
			UNION
			SELECT (sign_off->>'sitePhotoAttachmentId')::uuid FROM maintenance.work_reports WHERE id = $1 AND version = $2)`,
		report,
		version,
	)

	return apperr.Wrap(err, "delete maintenance.attachments")
}

// store saves one file as a ready Attachment of the report, added in the report version it is made in (a later
// version lists it, an earlier one does not, IR782), and returns its ID.
func (m Files) store(
	ctx context.Context,
	call *ops.Call,
	job, report uuid.UUID,
	version int,
	blobInput BlobInput,
	tag *string,
) (uuid.UUID, error) {
	attachmentID := uuid.New()

	key := "jobs/" + job.String() + "/reports/" + report.String() + "/" + attachmentID.String()

	putErr := m.Blobs.Put(ctx, key, blobInput.Mime, blobInput.Bytes)
	if putErr != nil {
		return attachmentID, apperr.Wrap(putErr, "blob.Store.Put")
	}

	_, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.attachments (id, tenant_id, job_id, report_id, object_key, name, mime, size_bytes, status,
			uploaded_by, created_at, updated_at, added_version, tag)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, 'ready', $8, $9, $9, $10, $11)`,
		attachmentID,
		job,
		report,
		key,
		strings.TrimSpace(blobInput.Name),
		blobInput.Mime,
		blobInput.Size,
		call.Principal.UserID,
		call.Now,
		version,
		tag,
	)

	return attachmentID, apperr.Wrap(err, "insert maintenance.attachments")
}

// ---- attachments.add ----

// AddInput is attachments.add input.
type AddInput struct {
	JobID    uuid.UUID `json:"jobId"`
	ReportID uuid.UUID `json:"reportId"`
	File     BlobInput `json:"file"`
	// Tag says what a photo of an installation report shows (name plate, indoor unit, outdoor unit; DEC-106).
	Tag *string `json:"tag,omitempty"`
}

// Validate implements ops.Validator.
func (in *AddInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil {
		fieldErrs["reportId"] = apperr.KeyRequired
	}

	if !ValidBlob(in.File, photoMaxBytes) {
		fieldErrs["file"] = apperr.KeyInvalidFile
	}

	if in.Tag != nil &&
		!slices.Contains([]string{"name_plate", "indoor_unit", "outdoor_unit"}, *in.Tag) {
		fieldErrs["tag"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// add serves attachments.add.
//
//	@Summary		attachments.add (write)
//	@ID				attachments.add
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T09
//	@Tags			attachments
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target report, read reports.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			reportId			path		string		true	"input field reportId"
//	@Param			request				body		AddInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=AttachmentRef}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/reports/{reportId}/attachments [post]
func (m Files) add(ctx context.Context, call *ops.Call, input *AddInput) (AttachmentRef, error) {
	draft, err := m.lockDraft(ctx, call, input.JobID, input.ReportID)
	if err != nil {
		return AttachmentRef{}, err
	}

	if draft.Version != *call.ExpectedVersion {
		return AttachmentRef{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	dropErr := dropSignOffFiles(
		ctx,
		call,
		input.ReportID,
		draft.Version,
	) // adding a file clears the sign-off below
	if dropErr != nil {
		return AttachmentRef{}, dropErr
	}

	var count int

	scanErr := call.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.attachments WHERE report_id = $1`, input.ReportID).
		Scan(&count)
	if scanErr != nil {
		return AttachmentRef{}, apperr.Wrap(scanErr, "select maintenance.attachments")
	}

	if count >= MaxReportAttachments {
		return AttachmentRef{}, apperr.E(apperr.Conflict, "errors.too_many_attachments")
	}

	attachmentID, err := m.store(
		ctx,
		call,
		input.JobID,
		input.ReportID,
		draft.Version,
		input.File,
		input.Tag,
	)
	if err != nil {
		return AttachmentRef{}, err
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.work_reports SET version = version + 1, sign_off = NULL, updated_at = $3 WHERE id = $1 AND version
			= $2`,
		input.ReportID,
		draft.Version,
		call.Now,
	)
	if execErr != nil {
		return AttachmentRef{}, apperr.Wrap(execErr, "update maintenance.work_reports")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET draft_report_version = $2 WHERE id = $1`,
		input.JobID,
		draft.Version+1,
	)
	if execErr != nil {
		return AttachmentRef{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	next := draft.Version + 1

	call.Emit(
		outboxEvent(
			"report",
			input.ReportID,
			"AttachmentAdded",
			map[string]any{fieldAttachmentID: attachmentID},
		),
	)
	call.Audit(
		auditEntry("attachments.add", "report", input.ReportID, call.ExpectedVersion, &next, ""),
	)

	return AttachmentRef{
		ID:        attachmentID,
		TenantID:  call.Principal.TenantID,
		Version:   1,
		CreatedAt: call.Now,
		UpdatedAt: call.Now,
		JobID:     input.JobID,
		ReportID:  input.ReportID,
		BlobID:    attachmentID,
		Tag:       input.Tag,
		Name: strings.TrimSpace(
			input.File.Name,
		),
		Mime:   input.File.Mime,
		Size:   input.File.Size,
		Status: attachmentReady,
	}, nil
}

// ---- reports.signOff ----

// SignOffInput is reports.signOff input.
type SignOffInput struct {
	JobID         uuid.UUID  `json:"jobId"`
	ReportID      uuid.UUID  `json:"reportId"`
	ReportVersion int        `json:"reportVersion"`
	SignerName    string     `json:"signerName"`
	Signature     *BlobInput `json:"signature"`
	AbsentReason  *string    `json:"absentReason,omitempty"`
	SitePhoto     *BlobInput `json:"sitePhoto,omitempty"`
}

// Validate implements ops.Validator (IR111 / IR129 item 3).
func (in *SignOffInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil || in.ReportVersion < 1 {
		fieldErrs["reportId"] = apperr.KeyRequired
	}

	trimmedReason(&in.SignerName, maxNameLength, "signerName", fieldErrs)

	switch {
	case in.Signature != nil && in.AbsentReason == nil && in.SitePhoto == nil:
		if in.Signature.Mime != mimePNG || !ValidBlob(*in.Signature, photoMaxBytes) {
			fieldErrs["signature"] = apperr.KeyInvalidFile
		}
	case in.Signature == nil && in.AbsentReason != nil && in.SitePhoto != nil:
		trimmedReason(in.AbsentReason, maxReasonLength, "absentReason", fieldErrs)

		if !ValidBlob(*in.SitePhoto, photoMaxBytes) {
			fieldErrs["sitePhoto"] = apperr.KeyInvalidFile
		}
	default:
		fieldErrs["signature"] = "errors.signature_or_absence"
	}

	return fieldErrs
}

// SignOff is SignOff of service-contracts.ts.
type SignOff struct {
	SignerName            string     `json:"signerName"`
	SignedAt              time.Time  `json:"signedAt"`
	SignatureAttachmentID *uuid.UUID `json:"signatureAttachmentId"`
	AbsentReason          *string    `json:"absentReason"`
	SitePhotoAttachmentID *uuid.UUID `json:"sitePhotoAttachmentId"`
	ReportVersion         int        `json:"reportVersion"`
}

// signOff serves reports.signOff.
//
//	@Summary		reports.signOff (write)
//	@ID				reports.signOff
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 signer 1–120; either signature or absentReason with sitePhoto
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T15
//	@Tags			reports
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target report, read reports.get)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			reportId			path		string			true	"input field reportId"
//	@Param			request				body		SignOffInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/jobs/{jobId}/reports/{reportId}/sign-off [post]
func (m Files) signOff(ctx context.Context, call *ops.Call, input *SignOffInput) (Report, error) {
	draft, err := m.lockDraft(ctx, call, input.JobID, input.ReportID)
	if err != nil {
		return Report{}, err
	}

	if draft.Version != *call.ExpectedVersion || draft.Version != input.ReportVersion {
		return Report{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}

	dropErr := dropSignOffFiles(
		ctx,
		call,
		input.ReportID,
		draft.Version,
	) // the new sign-off replaces the earlier one
	if dropErr != nil {
		return Report{}, dropErr
	}

	signOff := SignOff{
		SignerName:    input.SignerName,
		SignedAt:      call.Now,
		AbsentReason:  input.AbsentReason,
		ReportVersion: draft.Version, SignatureAttachmentID: nil, SitePhotoAttachmentID: nil,
	}
	if input.Signature != nil {
		id, err := m.store(
			ctx,
			call,
			input.JobID,
			input.ReportID,
			draft.Version,
			*input.Signature,
			nil,
		)
		if err != nil {
			return Report{}, err
		}

		signOff.SignatureAttachmentID = &id
	} else {
		id, err := m.store(
			ctx,
			call,
			input.JobID,
			input.ReportID,
			draft.Version,
			*input.SitePhoto,
			nil,
		)
		if err != nil {
			return Report{}, err
		}

		signOff.SitePhotoAttachmentID = &id
	}

	raw, marshalErr := json.Marshal(signOff)
	if marshalErr != nil {
		return Report{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.work_reports SET sign_off = $3, updated_at = $4 WHERE id = $1 AND version = $2`,
		input.ReportID,
		draft.Version,
		raw,
		call.Now,
	)
	if execErr != nil {
		return Report{}, apperr.Wrap(execErr, "update maintenance.work_reports")
	}

	report, err := m.Reports.load(ctx, call, input.ReportID, draft.Version)
	if err != nil {
		return report, err
	}

	call.Emit(
		outboxEvent(
			"report",
			input.ReportID,
			"ReportSignedOff",
			map[string]any{fieldReportVersion: draft.Version},
		),
	)
	call.Audit(
		auditEntry(
			"reports.signOff",
			"report",
			input.ReportID,
			call.ExpectedVersion,
			&report.Version,
			"",
		),
	)

	return report, nil
}

// ---- attachments.getContent ----

// ContentInput is attachments.getContent input.
type ContentInput struct {
	JobID         uuid.UUID `json:"jobId"`
	ReportID      uuid.UUID `json:"reportId"`
	ReportVersion int       `json:"reportVersion"`
	AttachmentID  uuid.UUID `json:"attachmentId"`
}

// Validate implements ops.Validator.
func (in *ContentInput) Validate() map[string]string {
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil || in.AttachmentID == uuid.Nil ||
		in.ReportVersion < 1 {
		return map[string]string{fieldAttachmentID: apperr.KeyRequired}
	}

	return nil
}

// BlobOut is the Blob result (IR129 item 1).
type BlobOut struct {
	Name  string `json:"name"`
	Mime  string `json:"mime"`
	Size  int    `json:"size"`
	Bytes []byte `json:"bytes"`
}

// content serves attachments.getContent.
//
//	@Summary		attachments.getContent (read)
//	@ID				attachments.getContent
//	@Description	Authorization: client:accepted-report-only | contractor:partner.review:submitted | \
//	@Description	technician:assigned-valid-job | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06, DD-C09, DD-P05, DD-T09
//	@Tags			attachments
//	@Accept			json
//	@Produce		json
//	@Param			jobId			path		string	true	"input field jobId"
//	@Param			reportId		path		string	true	"input field reportId"
//	@Param			attachmentId	path		string	true	"input field attachmentId"
//	@Param			reportVersion	query		integer	false	"input field reportVersion"
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
//	@Router			/v1/jobs/{jobId}/reports/{reportId}/attachments/{attachmentId}/content [get]
func (m Files) content(ctx context.Context, call *ops.Call, input *ContentInput) (BlobOut, error) {
	report, getErr := m.Reports.get(
		ctx,
		call,
		&GetReportInput{
			JobID:         input.JobID,
			ReportID:      input.ReportID,
			ReportVersion: input.ReportVersion,
		},
	)
	if getErr != nil {
		return BlobOut{}, getErr
	}

	var (
		key string
		out BlobOut
	)

	err := call.Tx.QueryRow(ctx, `SELECT object_key, name, mime, size_bytes FROM maintenance.attachments WHERE id = $1 AND
		report_id = $2 AND job_id = $3 AND status = 'ready' AND added_version <= $4`,
		input.AttachmentID, input.ReportID, input.JobID, report.Version).
		Scan(&key, &out.Name, &out.Mime, &out.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return out, apperr.Wrap(err, "select maintenance.attachments")
	}

	out.Bytes, err = m.Blobs.Get(ctx, key)
	if errors.Is(err, blob.ErrNotFound) {
		return out, apperr.E(
			apperr.NotFound,
			apperr.KeyNotFound,
		) // the row is kept but its content was lost: not retryable (IR229)
	} else if err != nil {
		return out, apperr.Wrap(err, "blob.Store.Get")
	}

	return out, nil
}

// RegisterFiles binds the attachment and sign-off operations.
func RegisterFiles(r *ops.Registry, m Files) {
	ops.Register(r, "attachments.add", m.add)
	ops.Register(r, "attachments.getContent", m.content)
	ops.Register(r, "attachments.list", m.list)
	ops.Register(r, "reports.signOff", m.signOff)
}
