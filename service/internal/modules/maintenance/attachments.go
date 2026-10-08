package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/blob"
)

// Files is the attachment and sign-off operation set (IR129).
type Files struct {
	Reports Reports
	Blobs   blob.Store
}

// MaxReportAttachments is the provisional DD-T09 limit per report.
const MaxReportAttachments = 10

// lockDraft checks the technician write table and returns the job's current draft locked for update.
func (m Files) lockDraft(ctx context.Context, c *ops.Call, job, report uuid.UUID) (*Report, error) {
	var status string
	var unit uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, job).Scan(&status, &unit)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return nil, err
	}
	if err := m.Reports.Access.TechnicianJob(ctx, c, job, unit); err != nil {
		return nil, err
	}
	draft, err := m.Reports.draftOf(ctx, c, job, true)
	if err != nil {
		return nil, err
	}
	if status != "in_progress" || draft == nil || draft.ID != report {
		return nil, apperr.E(apperr.Conflict, "error.invalidState")
	}
	return draft, nil
}

// store saves one file as a ready Attachment of the report and returns its ID.
func (m Files) store(ctx context.Context, c *ops.Call, job, report uuid.UUID, f BlobInput) (uuid.UUID, error) {
	id := uuid.New()
	key := "jobs/" + job.String() + "/reports/" + report.String() + "/" + id.String()
	if err := m.Blobs.Put(ctx, key, f.Mime, f.Bytes); err != nil {
		return id, err
	}
	_, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.attachments (id, tenant_id, job_id, report_id, object_key, name, mime, size_bytes, status, uploaded_by, created_at, updated_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, 'ready', $8, $9, $9)`, id, job, report, key, strings.TrimSpace(f.Name), f.Mime, f.Size, c.Principal.UserID, c.Now)
	return id, err
}

// ---- attachments.add ----

// AddInput is attachments.add input.
type AddInput struct {
	JobID    uuid.UUID `json:"jobId"`
	ReportID uuid.UUID `json:"reportId"`
	File     BlobInput `json:"file"`
}

// Validate implements ops.Validator.
func (in *AddInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil {
		fe["reportId"] = "error.required"
	}
	if !ValidBlob(in.File, 5<<20) {
		fe["file"] = "error.invalidFile"
	}
	return fe
}

func (m Files) add(ctx context.Context, c *ops.Call, in *AddInput) (AttachmentRef, error) {
	draft, err := m.lockDraft(ctx, c, in.JobID, in.ReportID)
	if err != nil {
		return AttachmentRef{}, err
	}
	if draft.Version != *c.ExpectedVersion {
		return AttachmentRef{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	var n int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM maintenance.attachments WHERE report_id = $1`, in.ReportID).Scan(&n); err != nil {
		return AttachmentRef{}, err
	}
	if n >= MaxReportAttachments {
		return AttachmentRef{}, apperr.E(apperr.Conflict, "errors.too_many_attachments")
	}
	id, err := m.store(ctx, c, in.JobID, in.ReportID, in.File)
	if err != nil {
		return AttachmentRef{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.work_reports SET version = version + 1, sign_off = NULL, updated_at = $3 WHERE id = $1 AND version = $2`,
		in.ReportID, draft.Version, c.Now); err != nil {
		return AttachmentRef{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET draft_report_version = $2 WHERE id = $1`, in.JobID, draft.Version+1); err != nil {
		return AttachmentRef{}, err
	}
	next := draft.Version + 1
	c.Emit(ops.Event{AggregateType: "report", AggregateID: in.ReportID, Type: "AttachmentAdded", Payload: map[string]any{"attachmentId": id}})
	c.Audit(ops.AuditEntry{Action: "attachments.add", TargetKind: "report", TargetID: in.ReportID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	return AttachmentRef{ID: id, TenantID: c.Principal.TenantID, Version: 1, CreatedAt: c.Now, UpdatedAt: c.Now, JobID: in.JobID, ReportID: in.ReportID, BlobID: id,
		Name: strings.TrimSpace(in.File.Name), Mime: in.File.Mime, Size: in.File.Size, Status: "ready"}, nil
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil || in.ReportVersion < 1 {
		fe["reportId"] = "error.required"
	}
	trimmedReason(&in.SignerName, 120, "signerName", fe)
	switch {
	case in.Signature != nil && in.AbsentReason == nil && in.SitePhoto == nil:
		if in.Signature.Mime != "image/png" || !ValidBlob(*in.Signature, 5<<20) {
			fe["signature"] = "error.invalidFile"
		}
	case in.Signature == nil && in.AbsentReason != nil && in.SitePhoto != nil:
		trimmedReason(in.AbsentReason, 1000, "absentReason", fe)
		if !ValidBlob(*in.SitePhoto, 5<<20) {
			fe["sitePhoto"] = "error.invalidFile"
		}
	default:
		fe["signature"] = "errors.signature_or_absence"
	}
	return fe
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

func (m Files) signOff(ctx context.Context, c *ops.Call, in *SignOffInput) (Report, error) {
	draft, err := m.lockDraft(ctx, c, in.JobID, in.ReportID)
	if err != nil {
		return Report{}, err
	}
	if draft.Version != *c.ExpectedVersion || draft.Version != in.ReportVersion {
		return Report{}, apperr.E(apperr.Conflict, "errors.report_version_changed")
	}
	so := SignOff{SignerName: in.SignerName, SignedAt: c.Now, AbsentReason: in.AbsentReason, ReportVersion: draft.Version}
	if in.Signature != nil {
		id, err := m.store(ctx, c, in.JobID, in.ReportID, *in.Signature)
		if err != nil {
			return Report{}, err
		}
		so.SignatureAttachmentID = &id
	} else {
		id, err := m.store(ctx, c, in.JobID, in.ReportID, *in.SitePhoto)
		if err != nil {
			return Report{}, err
		}
		so.SitePhotoAttachmentID = &id
	}
	raw, _ := json.Marshal(so)
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.work_reports SET sign_off = $3, updated_at = $4 WHERE id = $1 AND version = $2`, in.ReportID, draft.Version, raw, c.Now); err != nil {
		return Report{}, err
	}
	x, err := m.Reports.load(ctx, c, in.ReportID, draft.Version)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "report", AggregateID: in.ReportID, Type: "ReportSignedOff", Payload: map[string]any{"reportVersion": draft.Version}})
	c.Audit(ops.AuditEntry{Action: "reports.signOff", TargetKind: "report", TargetID: in.ReportID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version})
	return x, nil
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
	if in.JobID == uuid.Nil || in.ReportID == uuid.Nil || in.AttachmentID == uuid.Nil || in.ReportVersion < 1 {
		return map[string]string{"attachmentId": "error.required"}
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

func (m Files) content(ctx context.Context, c *ops.Call, in *ContentInput) (BlobOut, error) {
	if _, err := m.Reports.get(ctx, c, &GetReportInput{JobID: in.JobID, ReportID: in.ReportID, ReportVersion: in.ReportVersion}); err != nil {
		return BlobOut{}, err
	}
	var key string
	var out BlobOut
	err := c.Tx.QueryRow(ctx, `SELECT object_key, name, mime, size_bytes FROM maintenance.attachments WHERE id = $1 AND report_id = $2 AND job_id = $3 AND status = 'ready'`,
		in.AttachmentID, in.ReportID, in.JobID).Scan(&key, &out.Name, &out.Mime, &out.Size)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return out, err
	}
	if out.Bytes, err = m.Blobs.Get(ctx, key); err != nil {
		return out, err
	}
	return out, nil
}

// RegisterFiles binds the attachment and sign-off operations.
func RegisterFiles(r *ops.Registry, m Files) {
	ops.Register(r, "attachments.add", m.add)
	ops.Register(r, "attachments.getContent", m.content)
	ops.Register(r, "reports.signOff", m.signOff)
}
