package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Grants updates qualification grants (Identity).
type Grants interface {
	SetGrant(
		ctx context.Context,
		c *ops.Call,
		membership uuid.UUID,
		code string,
		from, until time.Time,
	) error
}

// Certificates is the certificate and parts catalog operation set (IR133).
type Certificates struct {
	Delivery Delivery
	Grants   Grants
	Blobs    blob.Store
}

// Certificate is Certificate of service-contracts.ts.
type Certificate struct {
	ID                     uuid.UUID  `json:"id"`
	TenantID               uuid.UUID  `json:"tenantId"`
	Version                int        `json:"version"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
	MembershipID           uuid.UUID  `json:"membershipId"`
	OrganizationID         uuid.UUID  `json:"organizationId"`
	Code                   string     `json:"code"`
	Name                   string     `json:"name"`
	Number                 string     `json:"number"`
	IssuedAt               time.Time  `json:"issuedAt"`
	ExpiresAt              time.Time  `json:"expiresAt"`
	FileName               *string    `json:"fileName"`
	Status                 string     `json:"status"`
	RenewalOf              *uuid.UUID `json:"renewalOf"`
	VerifiedByMembershipID *uuid.UUID `json:"verifiedByMembershipId"`
	VerifiedAt             *time.Time `json:"verifiedAt"`
	TrainingRequestedAt    *time.Time `json:"trainingRequestedAt"`
}

const certCols = `id, tenant_id, version, created_at, updated_at, membership_id, organization_id, code, name, number,` +
	` issued_at, expires_at, file_name, status,
	renewal_of, verified_by_membership_id, verified_at, training_requested_at`

// readStatus is IR133 item 2.
func readStatus(stored string, expires, now time.Time) string {
	if stored != "valid" {
		return stored
	}

	switch {
	case !expires.After(now):
		return "expired"
	case expires.Sub(now) <= 30*24*time.Hour:
		return "expiring"
	}

	return "valid"
}

func scanCert(r pgx.Row, now time.Time) (Certificate, error) {
	var certificate Certificate

	err := r.Scan(
		&certificate.ID,
		&certificate.TenantID,
		&certificate.Version,
		&certificate.CreatedAt,
		&certificate.UpdatedAt,
		&certificate.MembershipID,
		&certificate.OrganizationID,
		&certificate.Code,
		&certificate.Name,
		&certificate.Number,
		&certificate.IssuedAt,
		&certificate.ExpiresAt,
		&certificate.FileName,
		&certificate.Status,
		&certificate.RenewalOf,
		&certificate.VerifiedByMembershipID,
		&certificate.VerifiedAt,
		&certificate.TrainingRequestedAt,
	)
	certificate.Status = readStatus(certificate.Status, certificate.ExpiresAt, now)

	return certificate, apperr.Wrap(err, "scan (scanCert)")
}

func (m Certificates) lock(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
) (Certificate, string, error) {
	var stored string

	certificate, err := scanCert(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1 FOR UPDATE",
			id,
		),
		call.Now,
	)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "contractor" && certificate.OrganizationID != call.Principal.OrgID) {
		return certificate, "", apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return certificate, "", err
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT status FROM maintenance.certificates WHERE id = $1`, id).
		Scan(&stored)
	if scanErr != nil {
		return certificate, "", apperr.Wrap(scanErr, "select maintenance.certificates")
	}

	if certificate.Version != *call.ExpectedVersion {
		return certificate, "", apperr.E(apperr.Conflict, "error.versionConflict")
	}

	return certificate, stored, nil
}

func (m Certificates) reload(ctx context.Context, c *ops.Call, id uuid.UUID) (Certificate, error) {
	return scanCert(
		c.Tx.QueryRow(ctx, "SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1", id),
		c.Now,
	)
}

// ---- certificates.submit ----

var certCodes = map[string]bool{
	"demo_indoor":     true,
	"demo_outdoor":    true,
	"demo_electrical": true,
	"other":           true,
}

var certStatuses = map[string]bool{
	"valid":                true,
	"expiring":             true,
	"expired":              true,
	"pending_verification": true,
	"rejected":             true,
}

// SubmitCertInput is certificates.submit input.
type SubmitCertInput struct {
	MembershipID uuid.UUID  `json:"membershipId"`
	Code         string     `json:"code"`
	Name         string     `json:"name"`
	Number       string     `json:"number"`
	IssuedAt     time.Time  `json:"issuedAt"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	File         BlobInput  `json:"file"`
	RenewalOf    *uuid.UUID `json:"renewalOf,omitempty"`
}

// Validate implements ops.Validator.
func (in *SubmitCertInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.MembershipID == uuid.Nil {
		fieldErrs["membershipId"] = "error.required"
	}

	if !certCodes[in.Code] {
		fieldErrs["code"] = "error.invalid"
	}

	trimmedReason(&in.Name, 120, "name", fieldErrs)
	trimmedReason(&in.Number, 64, "number", fieldErrs)

	if in.IssuedAt.IsZero() || !in.IssuedAt.Before(in.ExpiresAt) {
		fieldErrs["expiresAt"] = "error.range"
	}

	blobInput := in.File
	if !ValidFileName(blobInput.Name) ||
		(blobInput.Mime != "application/pdf" && blobInput.Mime != "image/jpeg" && blobInput.Mime != "image/png") ||
		blobInput.Size != len(blobInput.Bytes) ||
		blobInput.Size < 1 ||
		blobInput.Size > 10*1000*1000 ||
		!ContentMatches(blobInput.Mime, blobInput.Bytes) ||
		(blobInput.Mime != "application/pdf" && !ImageSizeOK(blobInput.Mime, blobInput.Bytes)) {
		fieldErrs["file"] = "error.invalidFile"
	}

	return fieldErrs
}

// submit serves certificates.submit.
//
//	@Summary		certificates.submit (write)
//	@ID				certificates.submit
//	@Description	Authorization: contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 PDF/JPEG/PNG up to 10 MB; issuedAt < expiresAt
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P09
//	@Tags			certificates
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		SubmitCertInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Certificate}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/certificates [post]
func (m Certificates) submit(
	ctx context.Context,
	call *ops.Call,
	input *SubmitCertInput,
) (Certificate, error) {
	technician, found, err := m.Delivery.Directory.Technician(ctx, call, input.MembershipID)
	if err != nil {
		return Certificate{}, apperr.Wrap(err, "maintenance.Directory.Technician")
	}

	if !found || !technician.Active || technician.OrgID != call.Principal.OrgID {
		return Certificate{}, apperr.E(apperr.NotFound, "error.notFound")
	}

	if input.RenewalOf != nil {
		var ok bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.certificates WHERE id = $1 AND membership_id =
			$2
			AND code = $3)`, *input.RenewalOf, input.MembershipID, input.Code).
			Scan(&ok)
		if err != nil {
			return Certificate{}, apperr.Wrap(err, "select maintenance.certificates")
		}

		if !ok {
			return Certificate{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	}

	id := uuid.New()

	key := "certificates/" + input.MembershipID.String() + "/" + id.String()

	putErr := m.Blobs.Put(ctx, key, input.File.Mime, input.File.Bytes)
	if putErr != nil {
		return Certificate{}, apperr.Wrap(putErr, "blob.Store.Put")
	}

	name := strings.TrimSpace(input.File.Name)

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.certificates (id, tenant_id, membership_id, organization_id, code, name, number, issued_at,
			expires_at, object_key,
		file_name, status, renewal_of, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4,
		$5, $6, $7, $8, $9, $10, 'pending_verification', $11, $12, $12)`,
		id,
		input.MembershipID,
		technician.OrgID,
		input.Code,
		input.Name,
		input.Number,
		input.IssuedAt,
		input.ExpiresAt,
		key,
		name,
		input.RenewalOf,
		call.Now,
	)
	if execErr != nil {
		return Certificate{}, apperr.Wrap(execErr, "insert maintenance.certificates")
	}

	certificate, err := m.reload(ctx, call, id)
	if err != nil {
		return certificate, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "certificate",
			AggregateID:   id,
			Type:          "CertificateSubmitted",
			Payload:       map[string]any{"membershipId": input.MembershipID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "certificates.submit",
			TargetKind:      "certificate",
			TargetID:        id.String(),
			NextVersion:     &certificate.Version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return certificate, nil
}

// ---- certificates.verify ----

// VerifyInput is certificates.verify input.
type VerifyInput struct {
	CertificateID uuid.UUID `json:"certificateId"`
	Decision      string    `json:"decision"`
	Reason        *string   `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *VerifyInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.CertificateID == uuid.Nil {
		fieldErrs["certificateId"] = "error.required"
	}

	switch in.Decision {
	case "approve":
	case "reject":
		if in.Reason == nil {
			fieldErrs["reason"] = "error.required"
		} else {
			trimmedReason(in.Reason, 1000, "reason", fieldErrs)
		}
	default:
		fieldErrs["decision"] = "error.invalid"
	}

	return fieldErrs
}

// verify serves certificates.verify.
//
//	@Summary		certificates.verify (write)
//	@ID				certificates.verify
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 only pending_verification; approve updates the Membership qualification validUntil; a \
//	@Description	not-started assignment on a unit that requires the code and that the approved window no longer \
//	@Description	covers is released (IR440)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A21
//	@Tags			certificates
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target certificate, read certificates.list)"
//	@Param			certificateId		path		string		true	"input field certificateId"
//	@Param			request				body		VerifyInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Certificate}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/certificates/{certificateId}/verify [post]
func (m Certificates) verify(
	ctx context.Context,
	call *ops.Call,
	input *VerifyInput,
) (Certificate, error) {
	certificate, stored, err := m.lock(ctx, call, input.CertificateID)
	if err != nil {
		return certificate, err
	}

	if stored != "pending_verification" {
		return certificate, apperr.E(apperr.Conflict, "error.invalidState")
	}

	if input.Decision == "approve" {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.certificates SET status = 'valid', verified_by_membership_id = $2, verified_at = $3, version =
				version + 1, updated_at = $3 WHERE id = $1`,
			certificate.ID,
			call.Principal.MembershipID,
			call.Now,
		)
		if err != nil {
			return certificate, apperr.Wrap(err, "update maintenance.certificates")
		}

		if certificate.Code != "other" {
			err := m.Grants.SetGrant(
				ctx,
				call,
				certificate.MembershipID,
				certificate.Code,
				certificate.IssuedAt,
				certificate.ExpiresAt,
			)
			if err != nil {
				return certificate, apperr.Wrap(err, "maintenance.Grants.SetGrant")
			}

			err = releaseQualificationEnded(
				ctx,
				call,
				m.Delivery.Units,
				certificate.MembershipID,
				certificate.Code,
				certificate.IssuedAt,
				certificate.ExpiresAt,
			)
			if err != nil { // IR440
				return certificate, err
			}
		}
	} else {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.certificates SET status = 'rejected', rejection_reason = $2, verified_by_membership_id = $3,
				verified_at = $4,
		version = version + 1, updated_at = $4 WHERE id = $1`,
			certificate.ID,
			*input.Reason,
			call.Principal.MembershipID,
			call.Now,
		)
		if err != nil {
			return certificate, apperr.Wrap(err, "update maintenance.certificates")
		}
	}

	certificate, err = m.reload(ctx, call, certificate.ID)
	if err != nil {
		return certificate, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "certificate",
			AggregateID:   certificate.ID,
			Type:          "CertificateVerified",
			Payload:       map[string]any{"decision": input.Decision},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "certificates.verify",
			TargetKind:      "certificate",
			TargetID:        certificate.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &certificate.Version,
			Reason:          deref(input.Reason), Result: "", Before: nil, After: nil,
		},
	)

	return certificate, nil
}

// ---- certificates.requestTraining ----

// TrainingInput is certificates.requestTraining input.
type TrainingInput struct {
	CertificateID uuid.UUID `json:"certificateId"`
	Note          string    `json:"note"`
}

// Validate implements ops.Validator.
func (in *TrainingInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.CertificateID == uuid.Nil {
		fieldErrs["certificateId"] = "error.required"
	}

	trimmedReason(&in.Note, 1000, "note", fieldErrs)

	return fieldErrs
}

// requestTraining serves certificates.requestTraining.
//
//	@Summary		certificates.requestTraining (write)
//	@ID				certificates.requestTraining
//	@Description	Authorization: contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 note 1–1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-P09
//	@Tags			certificates
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target certificate, read certificates.list)"
//	@Param			certificateId		path		string			true	"input field certificateId"
//	@Param			request				body		TrainingInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Certificate}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/certificates/{certificateId}/request-training [post]
func (m Certificates) requestTraining(
	ctx context.Context,
	call *ops.Call,
	input *TrainingInput,
) (Certificate, error) {
	certificate, _, err := m.lock(ctx, call, input.CertificateID)
	if err != nil {
		return certificate, err
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.certificates SET training_requested_at = $2, training_request_note = $3, version = version + 1,
			updated_at = $2 WHERE id = $1`,
		certificate.ID,
		call.Now,
		input.Note,
	)
	if execErr != nil {
		return certificate, apperr.Wrap(execErr, "update maintenance.certificates")
	}

	certificate, err = m.reload(ctx, call, certificate.ID)
	if err != nil {
		return certificate, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "certificate",
			AggregateID:   certificate.ID,
			Type:          "TrainingRequested",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "certificates.requestTraining",
			TargetKind:      "certificate",
			TargetID:        certificate.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &certificate.Version,
			Reason:          input.Note, Result: "", Before: nil, After: nil,
		},
	)

	return certificate, nil
}

// ---- certificates.getContent, certificates.list ----

// CertificateContentInput is certificates.getContent input.
type CertificateContentInput struct {
	CertificateID uuid.UUID `json:"certificateId"`
}

// Validate implements ops.Validator.
func (in *CertificateContentInput) Validate() map[string]string {
	if in.CertificateID == uuid.Nil {
		return map[string]string{"certificateId": "error.required"}
	}

	return nil
}

// DocumentOut is DocumentContent of service-contracts.ts: the uploaded certificate document (IR436).
type DocumentOut struct {
	Name  string `json:"name"`
	Mime  string `json:"mime"`
	Size  int    `json:"size"`
	Bytes []byte `json:"bytes"`
}

// documentMime names an uploaded document's type from its leading bytes (certificates.submit admitted only these).
func documentMime(b []byte) string {
	for _, m := range []string{"application/pdf", "image/png", "image/jpeg"} {
		if ContentMatches(m, b) {
			return m
		}
	}

	return "application/octet-stream"
}

// content answers a certificate's uploaded document, so HQ sees what it verifies (IR436). Scoped as list: a
// contractor reads only its own company's certificates; another company's is NOT_FOUND.
//
//	@Summary		certificates.getContent (read)
//	@ID				certificates.getContent
//	@Description	Authorization: admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; the uploaded \
//	@Description	document of a certificate so HQ sees what it verifies (IR436); another company's certificate is \
//	@Description	NOT_FOUND
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A21, DD-P09
//	@Tags			certificates
//	@Accept			json
//	@Produce		json
//	@Param			certificateId	path		string	true	"input field certificateId"
//	@Success		200				{object}	ops.Envelope{data=DocumentOut}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/certificates/{certificateId}/content [get]
func (m Certificates) content(
	ctx context.Context,
	call *ops.Call,
	input *CertificateContentInput,
) (DocumentOut, error) {
	var (
		key, name *string
		org       uuid.UUID
	)

	err := call.Tx.QueryRow(ctx, `SELECT object_key, file_name, organization_id FROM maintenance.certificates WHERE id =
		$1`, input.CertificateID).
		Scan(&key, &name, &org)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == "contractor" && org != call.Principal.OrgID) {
		return DocumentOut{}, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return DocumentOut{}, apperr.Wrap(err, "select maintenance.certificates")
	}

	if key == nil || *key == "" {
		return DocumentOut{}, apperr.E(
			apperr.NotFound,
			"error.notFound",
		) // a certificate without an uploaded document
	}

	out := DocumentOut{Name: "certificate", Mime: "", Size: 0, Bytes: nil}
	if name != nil && *name != "" {
		out.Name = *name
	}

	out.Bytes, err = m.Blobs.Get(ctx, *key)
	if errors.Is(err, blob.ErrNotFound) {
		return out, apperr.E(apperr.NotFound, "error.notFound")
	} else if err != nil {
		return out, apperr.Wrap(err, "blob.Store.Get")
	}

	out.Mime, out.Size = documentMime(out.Bytes), len(out.Bytes)

	return out, nil
}

// list serves certificates.list.
//
//	@Summary		certificates.list (read)
//	@ID				certificates.list
//	@Description	Authorization: admin:job.read | contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A21, DD-P09 · Query: filters organizationId,membershipId,code,status,expiringWithinDays · \
//	@Description	sort id,expiresAt,status (default expiresAt asc;id asc)
//	@Tags			certificates
//	@Accept			json
//	@Produce		json
//	@Param			cursor				query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit				query		integer	false	"page size 1–100, default 25"
//	@Param			sort				query		string	false	"field:direction — fields id,expiresAt,status; default expiresAt asc;id asc"
//	@Param			organizationId		query		string	false	"filter → organizationId"
//	@Param			membershipId		query		string	false	"filter → membershipId"
//	@Param			code				query		string	false	"filter → code (QualificationCode or other)"
//	@Param			status				query		string	false	"filter → status"
//	@Param			expiringWithinDays	query		number	false	"filter → expiresAt within N days of now"
//	@Success		200					{object}	ops.Envelope{data=CertificatePage}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/certificates [get]
func (m Certificates) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Certificate], error) {
	var f struct {
		OrganizationID     *uuid.UUID `json:"organizationId,omitempty"`
		MembershipID       *uuid.UUID `json:"membershipId,omitempty"`
		Code               *string    `json:"code,omitempty"`
		Status             *string    `json:"status,omitempty"`
		ExpiringWithinDays *int       `json:"expiringWithinDays,omitempty"` // now <= expiresAt < now + N days
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil || (f.Code != nil && !certCodes[*f.Code]) ||
			(f.Status != nil && !certStatuses[*f.Status]) ||
			(f.ExpiringWithinDays != nil && (*f.ExpiringWithinDays < 1 || *f.ExpiringWithinDays > 3650)) {
			return paging.Page[Certificate]{}, apperr.Fields(
				map[string]string{"filters": "error.invalid"},
			)
		}
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"TRUE"}
	if call.Principal.Role == "contractor" {
		conds = append(conds, "organization_id = "+add(call.Principal.OrgID))
	}

	if f.OrganizationID != nil {
		conds = append(conds, "organization_id = "+add(*f.OrganizationID))
	}

	if f.MembershipID != nil {
		conds = append(conds, "membership_id = "+add(*f.MembershipID))
	}

	if f.Code != nil {
		conds = append(conds, "code = "+add(*f.Code))
	}

	if f.ExpiringWithinDays != nil {
		now := add(call.Now)
		conds = append(
			conds,
			"expires_at >= "+now+"::timestamptz AND expires_at < "+now+"::timestamptz + make_interval(days => "+add(
				*f.ExpiringWithinDays,
			)+"::int)",
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		"SELECT "+certCols+" FROM maintenance.certificates WHERE "+strings.Join(
			conds,
			" AND ",
		)+" ORDER BY expires_at, id",
		args...)
	if err != nil {
		return paging.Page[Certificate]{}, apperr.Wrap(err, "select maintenance.certificates")
	}

	all := []Certificate{}

	for rows.Next() {
		certificate, err := scanCert(rows, call.Now)
		if err != nil {
			rows.Close()

			return paging.Page[Certificate]{}, err
		}

		if f.Status == nil || certificate.Status == *f.Status {
			all = append(all, certificate)
		}
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Certificate]{}, apperr.Wrap(rowsErr, "select maintenance.certificates")
	}

	sortSliceErr := paging.SortSlice(all, query.Sort, map[string]func(a, b Certificate) int{
		"id":        func(a, b Certificate) int { return strings.Compare(a.ID.String(), b.ID.String()) },
		"expiresAt": func(a, b Certificate) int { return a.ExpiresAt.Compare(b.ExpiresAt) },
		"status":    func(a, b Certificate) int { return strings.Compare(a.Status, b.Status) },
	})
	if sortSliceErr != nil {
		return paging.Page[Certificate]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(all, *query, f, call.Principal.ScopeVersion)
}

// ---- parts.list ----

// PartItem is PartCatalogItem.
type PartItem struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	VanStockQuantity *int   `json:"vanStockQuantity"`
}

// parts serves parts.list.
//
//	@Summary		parts.list (read)
//	@ID				parts.list
//	@Description	Authorization: technician:assigned | admin:job.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T14 · Query: filters search,membershipId · sort id,name (default name asc;id asc)
//	@Tags			parts
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,name; default name asc;id asc"
//	@Param			search			query		string	false	"filter → code or name contains"
//	@Param			membershipId	query		string	false	"filter → van stock of that technician listed first"
//	@Success		200				{object}	ops.Envelope{data=PartItemPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/parts [get]
func (m Certificates) parts(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[PartItem], error) {
	var f struct {
		Search *string `json:"search,omitempty"` // code or name contains, case-insensitive
		// that technician's van stock first (van stock is not tracked yet)
		MembershipID *uuid.UUID `json:"membershipId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil {
			return paging.Page[PartItem]{}, apperr.Fields(
				map[string]string{"filters": "error.invalid"},
			)
		}
	}

	q, args := `SELECT code, name FROM maintenance.parts_catalog WHERE active`, []any{}
	if f.Search != nil {
		q, args = q+` AND (code ILIKE $1 OR name ILIKE $1)`, append(
			args,
			paging.Contains(*f.Search),
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		q+" ORDER BY lower(name), code",
		args...) // name asc, id asc (query catalog)
	if err != nil {
		return paging.Page[PartItem]{}, apperr.Wrap(err, "select maintenance.parts_catalog")
	}

	all := []PartItem{}

	for rows.Next() {
		var partItem PartItem

		err := rows.Scan(&partItem.Code, &partItem.Name)
		if err != nil {
			rows.Close()

			return paging.Page[PartItem]{}, apperr.Wrap(err, "select maintenance.parts_catalog")
		}

		all = append(all, partItem)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[PartItem]{}, apperr.Wrap(rowsErr, "select maintenance.parts_catalog")
	}

	sortSliceErr := paging.SortSlice(all, query.Sort, map[string]func(a, b PartItem) int{
		"id":   func(a, b PartItem) int { return strings.Compare(a.Code, b.Code) },
		"name": func(a, b PartItem) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) },
	})
	if sortSliceErr != nil {
		return paging.Page[PartItem]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(all, *query, f, call.Principal.ScopeVersion)
}

// ---- expiry reminders (scheduler job) ----

// reminderDays are the BR-P09 reminders in days before expiry, the latest first.
var reminderDays = []int{7, 30, 60}

// reminderStage is the latest reminder the certificate has reached at now (0 when it expires later than 60 days).
func reminderStage(expires, now time.Time) int {
	for _, d := range reminderDays {
		if !now.Before(expires.Add(-time.Duration(d) * 24 * time.Hour)) {
			return d
		}
	}

	return 0
}

// Remind is the maintenance scheduler job of BR-P09 (IR336). A valid certificate that expires within 60 days gets
// the latest reminder it has reached — 60, 30 or 7 days before expiresAt — once, and never an earlier one after a later
// one: one certificate_expiry notification to each contractor admin of its organization (partner.assign). No reminder
// goes for a certificate that waits for HQ, was rejected or has expired, that a valid renewal or a later valid
// certificate of the same qualification replaced, or whose technician is no longer an active member. A pending renewal
// does not stop the reminders: the current expiry applies until HQ verifies it.
func (m Certificates) Remind(ctx context.Context, call *ops.Call) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT c.id, c.organization_id, c.expires_at FROM maintenance.certificates c
		JOIN maintenance.ref_memberships rm ON rm.id = c.membership_id AND rm.valid_from <= $1 AND (rm.valid_until IS NULL OR
		rm.valid_until > $1)
		WHERE c.status = 'valid' AND c.expires_at > $1 AND c.expires_at <= $2
		  AND NOT EXISTS (SELECT 1 FROM maintenance.certificates n WHERE n.membership_id = c.membership_id AND n.status =
		  'valid' AND n.expires_at > c.expires_at
		    AND (n.renewal_of = c.id OR (n.code = c.code AND c.code <> 'other')))
		ORDER BY c.expires_at, c.id`,
		call.Now,
		call.Now.Add(time.Duration(reminderDays[len(reminderDays)-1])*24*time.Hour),
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select maintenance.certificates")
	}

	type due struct {
		id, org uuid.UUID
		expires time.Time
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (due, error) {
		var d due

		return d, r.Scan(&d.id, &d.org, &d.expires)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select maintenance.certificates")
	}

	n := 0

	for _, d := range list {
		days := reminderStage(d.expires, call.Now)

		tag, err := call.Tx.Exec(
			ctx,
			`INSERT INTO maintenance.certificate_reminders (certificate_id, days_before, tenant_id, sent_at)
			SELECT $1, $2, current_setting('app.tenant_id')::uuid, $3
			WHERE NOT EXISTS (SELECT 1 FROM maintenance.certificate_reminders WHERE certificate_id = $1 AND days_before <= $2) ON
			CONFLICT DO NOTHING`,
			d.id,
			days,
			call.Now,
		)
		if err != nil {
			return n, apperr.Wrap(err, "insert maintenance.certificate_reminders")
		}

		if tag.RowsAffected() == 0 { // this reminder (or a later one) went already
			continue
		}

		recipients, err := identity.Members(
			ctx,
			call,
			identity.MembersInput{
				Role:           "contractor",
				OrganizationID: &d.org,
				Permission:     "partner.assign", IDs: nil, ClientRole: "",
			},
		)
		if err != nil {
			return n, apperr.Wrap(err, "identity.Members")
		}

		severity := "normal"
		if days == reminderDays[0] {
			severity = "warning"
		}

		for _, r := range recipients {
			_, err := (notify.Store{}).Create(ctx, call, notify.New{
				RecipientMembershipID: r,
				Type:                  "certificate_expiry",
				TemplateKey:           "certificate_expiry",
				TargetKind:            "certificate",
				TargetID:              d.id,
				Severity:              severity,
				Params: map[string]any{
					"at":     d.expires,
					"status": "expiring",
				},
				Channel:       "",
				SourceAlertID: nil,
			})
			if err != nil {
				return n, apperr.Wrap(err, "notify.Store.Create")
			}
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.certificate_reminders SET notifications = $3 WHERE certificate_id = $1 AND days_before = $2`,
			d.id,
			days,
			len(recipients),
		)
		if execErr != nil {
			return n, apperr.Wrap(execErr, "update maintenance.certificate_reminders")
		}

		call.Audit(
			ops.AuditEntry{
				Action:     "certificates.remind",
				TargetKind: "certificate",
				TargetID:   d.id.String(),
				Reason: fmt.Sprintf(
					"%d days before expiry",
					days,
				),
				PreviousVersion: nil,
				NextVersion:     nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		n++
	}

	return n, nil
}

// RegisterCertificates binds the IR133 operations.
func RegisterCertificates(reg *ops.Registry, m Certificates) {
	ops.Register(reg, "certificates.submit", m.submit)
	ops.Register(reg, "certificates.verify", m.verify)
	ops.Register(reg, "certificates.requestTraining", m.requestTraining)
	ops.Register(reg, "certificates.list", m.list)
	ops.Register(reg, "certificates.getContent", m.content)
	ops.Register(reg, "parts.list", m.parts)
}
