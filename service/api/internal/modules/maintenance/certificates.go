package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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

// The statuses of a certificate: stored as pending_verification until HQ approves it (valid) or rejects it; a valid
// one reads as expiring within 30 days of its expiry and as expired from it on (IR133 item 2).
const (
	certValid               = "valid"
	certExpiring            = "expiring"
	certExpired             = "expired"
	certPendingVerification = "pending_verification"
	certRejected            = "rejected"
)

// readStatus is IR133 item 2.
func readStatus(stored string, expires, now time.Time) string {
	if stored != certValid {
		return stored
	}

	switch {
	case !expires.After(now):
		return certExpired
	case expires.Sub(now) <= 30*24*time.Hour:
		return certExpiring
	}

	return certValid
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

// ---- certificates.submit ----

// codeOther is the catch-all choice of the code lists: a qualification that grants no work, a problem's reason, a kind
// of unavailable day and a client's reason to decline a proposed slot.
const codeOther = "other"

// certCodes are the qualification codes a certificate is for (QualificationCode).
func certCodes() map[string]bool {
	return map[string]bool{
		"demo_indoor":     true,
		"demo_outdoor":    true,
		"demo_electrical": true,
		codeOther:         true,
	}
}

// certStatuses are the statuses a certificate reads as, which certificates.list filters on.
func certStatuses() map[string]bool {
	return map[string]bool{
		certValid:               true,
		certExpiring:            true,
		certExpired:             true,
		certPendingVerification: true,
		certRejected:            true,
	}
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
		fieldErrs["membershipId"] = apperr.KeyRequired
	}

	if !certCodes()[in.Code] {
		fieldErrs["code"] = apperr.KeyInvalid
	}

	trimmedReason(&in.Name, maxNameLength, fieldName, fieldErrs)
	trimmedReason(&in.Number, maxCodeLength, "number", fieldErrs)

	if in.IssuedAt.IsZero() || !in.IssuedAt.Before(in.ExpiresAt) {
		fieldErrs["expiresAt"] = apperr.KeyRange
	}

	blobInput := in.File
	if !ValidFileName(blobInput.Name) ||
		(blobInput.Mime != mimePDF && blobInput.Mime != mimeJPEG && blobInput.Mime != mimePNG) ||
		blobInput.Size != len(blobInput.Bytes) ||
		blobInput.Size < 1 ||
		blobInput.Size > 10*1000*1000 ||
		!ContentMatches(blobInput.Mime, blobInput.Bytes) ||
		(blobInput.Mime != mimePDF && !ImageSizeOK(blobInput.Mime, blobInput.Bytes)) {
		fieldErrs["file"] = apperr.KeyInvalidFile
	}

	return fieldErrs
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
		fieldErrs["certificateId"] = apperr.KeyRequired
	}

	switch in.Decision {
	case "approve":
	case "reject":
		if in.Reason == nil {
			fieldErrs[fieldReason] = apperr.KeyRequired
		} else {
			trimmedReason(in.Reason, maxReasonLength, fieldReason, fieldErrs)
		}
	default:
		fieldErrs["decision"] = apperr.KeyInvalid
	}

	return fieldErrs
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
		fieldErrs["certificateId"] = apperr.KeyRequired
	}

	trimmedReason(&in.Note, maxReasonLength, "note", fieldErrs)

	return fieldErrs
}

// ---- certificates.getContent, certificates.list ----

// CertificateContentInput is certificates.getContent input.
type CertificateContentInput struct {
	CertificateID uuid.UUID `json:"certificateId"`
}

// Validate implements ops.Validator.
func (in *CertificateContentInput) Validate() map[string]string {
	if in.CertificateID == uuid.Nil {
		return map[string]string{"certificateId": apperr.KeyRequired}
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
	for _, m := range []string{mimePDF, mimePNG, mimeJPEG} {
		if ContentMatches(m, b) {
			return m
		}
	}

	return "application/octet-stream"
}

// ---- parts.list ----

// PartItem is PartCatalogItem.
type PartItem struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	VanStockQuantity *int   `json:"vanStockQuantity"`
}

// ---- expiry reminders (scheduler job) ----

// The BR-P09 reminders in days before a certificate expires: the last, the middle and the first one, which is also how
// far ahead the job looks.
const (
	lastCertReminder   = 7
	middleCertReminder = 30
	firstCertReminder  = 60
)

// reminderDays are the BR-P09 reminders in days before expiry, the latest first.
func reminderDays() []int { return []int{lastCertReminder, middleCertReminder, firstCertReminder} }

// reminderStage is the latest reminder the certificate has reached at now (0 when it expires later than 60 days).
func reminderStage(expires, now time.Time) int {
	for _, d := range reminderDays() {
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
func (Certificates) Remind(ctx context.Context, call *ops.Call) (int, error) {
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
		call.Now.Add(firstCertReminder*24*time.Hour),
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

	count := 0

	for _, cert := range list {
		sent, err := remindCertificate(
			ctx,
			call,
			cert.id,
			cert.org,
			cert.expires,
			reminderStage(cert.expires, call.Now),
		)
		if err != nil {
			return count, err
		}

		if sent {
			count++
		}
	}

	return count, nil
}

// remindCertificate sends the certificate's reminder of the stage to its company's contractors who assign work, unless
// it (or a later one) went already, and reports whether it went.
func remindCertificate(
	ctx context.Context,
	call *ops.Call,
	certificate, org uuid.UUID,
	expires time.Time,
	days int,
) (bool, error) {
	tag, err := call.Tx.Exec(
		ctx,
		`INSERT INTO maintenance.certificate_reminders (certificate_id, days_before, tenant_id, sent_at)
		SELECT $1, $2, current_setting('app.tenant_id')::uuid, $3
		WHERE NOT EXISTS (SELECT 1 FROM maintenance.certificate_reminders WHERE certificate_id = $1 AND days_before <= $2) ON
		CONFLICT DO NOTHING`,
		certificate,
		days,
		call.Now,
	)
	if err != nil {
		return false, apperr.Wrap(err, "insert maintenance.certificate_reminders")
	}

	if tag.RowsAffected() == 0 { // this reminder (or a later one) went already
		return false, nil
	}

	recipients, err := identity.Members(
		ctx,
		call,
		identity.MembersInput{
			Role:           roleContractor,
			OrganizationID: &org,
			Permission:     "partner.assign", IDs: nil, ClientRole: "",
		},
	)
	if err != nil {
		return false, apperr.Wrap(err, "identity.Members")
	}

	severity := severityNormal
	if days == lastCertReminder {
		severity = severityWarning
	}

	for _, recipient := range recipients {
		_, err := (notify.Store{}).Create(ctx, call, notify.New{
			RecipientMembershipID: recipient,
			Type:                  "certificate_expiry",
			TemplateKey:           "certificate_expiry",
			TargetKind:            "certificate",
			TargetID:              certificate,
			Severity:              severity,
			Params: map[string]any{
				"at":        expires,
				fieldStatus: certExpiring,
			},
			Channel:       "",
			SourceAlertID: nil,
		})
		if err != nil {
			return false, apperr.Wrap(err, "notify.Store.Create")
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.certificate_reminders SET notifications = $3 WHERE certificate_id = $1 AND days_before = $2`,
		certificate,
		days,
		len(recipients),
	)
	if execErr != nil {
		return false, apperr.Wrap(execErr, "update maintenance.certificate_reminders")
	}

	call.Audit(auditEntry("certificates.remind", "certificate", certificate, nil, nil, fmt.Sprintf(
		"%d days before expiry",
		days,
	)))

	return true, nil
}

func (Certificates) lock(
	ctx context.Context,
	call *ops.Call,
	certificateID uuid.UUID,
) (Certificate, string, error) {
	var stored string

	certificate, err := scanCert(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1 FOR UPDATE",
			certificateID,
		),
		call.Now,
	)
	if errors.Is(err, pgx.ErrNoRows) ||
		(err == nil && call.Principal.Role == roleContractor && certificate.OrganizationID != call.Principal.OrgID) {
		return certificate, "", apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return certificate, "", err
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT status FROM maintenance.certificates WHERE id = $1`, certificateID).
		Scan(&stored)
	if scanErr != nil {
		return certificate, "", apperr.Wrap(scanErr, "select maintenance.certificates")
	}

	if certificate.Version != *call.ExpectedVersion {
		return certificate, "", apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return certificate, stored, nil
}

func (Certificates) reload(ctx context.Context, c *ops.Call, id uuid.UUID) (Certificate, error) {
	return scanCert(
		c.Tx.QueryRow(ctx, "SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1", id),
		c.Now,
	)
}

// submit serves certificates.submit.
//
//	@Summary		certificates.submit (write)
//	@ID				certificates.submit
//	@Description	Authorization: contractor:partner.assign:own-company
//	@Description	Validation: D01; input constraints in the corresponding DD; IR111 PDF/JPEG/PNG up to 10 MB; issuedAt \
//	@Description	< expiresAt
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
		return Certificate{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if input.RenewalOf != nil {
		var exists bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.certificates WHERE id = $1 AND membership_id =
			$2
			AND code = $3)`, *input.RenewalOf, input.MembershipID, input.Code).
			Scan(&exists)
		if err != nil {
			return Certificate{}, apperr.Wrap(err, "select maintenance.certificates")
		}

		if !exists {
			return Certificate{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	certificateID := uuid.New()

	key := "certificates/" + input.MembershipID.String() + "/" + certificateID.String()

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
		certificateID,
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

	certificate, err := m.reload(ctx, call, certificateID)
	if err != nil {
		return certificate, err
	}

	call.Emit(
		outboxEvent(
			"certificate",
			certificateID,
			"CertificateSubmitted",
			map[string]any{"membershipId": input.MembershipID},
		),
	)
	call.Audit(
		auditEntry(
			"certificates.submit",
			"certificate",
			certificateID,
			nil,
			&certificate.Version,
			"",
		),
	)

	return certificate, nil
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
		return certificate, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	if input.Decision == "approve" {
		err = m.approve(ctx, call, certificate)
	} else {
		err = reject(ctx, call, certificate, *input.Reason)
	}

	if err != nil {
		return certificate, err
	}

	certificate, err = m.reload(ctx, call, certificate.ID)
	if err != nil {
		return certificate, err
	}

	call.Emit(
		outboxEvent(
			"certificate",
			certificate.ID,
			"CertificateVerified",
			map[string]any{"decision": input.Decision},
		),
	)
	call.Audit(
		auditEntry(
			"certificates.verify",
			"certificate",
			certificate.ID,
			call.ExpectedVersion,
			&certificate.Version,
			deref(input.Reason),
		),
	)

	return certificate, nil
}

// approve makes the certificate valid; one of a qualification code sets the membership's grant to what the member's
// valid certificates of that code cover together, and an assignment that grant no longer covers is released (IR440,
// IR781).
func (m Certificates) approve(ctx context.Context, call *ops.Call, certificate Certificate) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.certificates SET status = 'valid', verified_by_membership_id = $2, verified_at = $3, version =
			version + 1, updated_at = $3 WHERE id = $1`,
		certificate.ID,
		call.Principal.MembershipID,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.certificates")
	}

	if certificate.Code == codeOther {
		return nil
	}

	from, until, err := coveredSpan(ctx, call, certificate)
	if err != nil {
		return err
	}

	err = m.Grants.SetGrant(ctx, call, certificate.MembershipID, certificate.Code, from, until)
	if err != nil {
		return apperr.Wrap(err, "maintenance.Grants.SetGrant")
	}

	err = releaseQualificationEnded(
		ctx,
		call,
		m.Delivery.Units,
		certificate.MembershipID,
		certificate.Code,
		from,
		until,
	)
	if err != nil { // IR440
		return err
	}

	return nil
}

// coveredSpan is the time the approved certificate and the member's other valid certificates of its code cover
// together: its own span, widened by every valid certificate that overlaps or touches it, again and again (IR781). A
// shorter certificate approved beside a longer one never cuts what the longer one still proves, and a gap between two
// certificates is not covered.
func coveredSpan(
	ctx context.Context,
	call *ops.Call,
	certificate Certificate,
) (time.Time, time.Time, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT issued_at, expires_at FROM maintenance.certificates WHERE membership_id = $1 AND code = $2 AND id <> $3
			AND status IN ('valid', 'expiring') AND expires_at > $4`,
		certificate.MembershipID,
		certificate.Code,
		certificate.ID,
		call.Now,
	)
	if err != nil {
		return time.Time{}, time.Time{}, apperr.Wrap(err, "select maintenance.certificates")
	}

	type span struct{ from, until time.Time }

	others, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (span, error) {
		var other span

		return other, row.Scan(&other.from, &other.until)
	})
	if err != nil {
		return time.Time{}, time.Time{}, apperr.Wrap(err, "select maintenance.certificates")
	}

	from, until := certificate.IssuedAt, certificate.ExpiresAt

	for widened := true; widened; {
		widened = false

		for i, other := range others {
			if other.from.After(until) || other.until.Before(from) {
				continue
			}

			from, until = minTime(from, other.from), maxTime(until, other.until)
			others = slices.Delete(others, i, i+1)
			widened = true

			break
		}
	}

	return from, until, nil
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}

	return a
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}

	return a
}

// reject refuses the certificate with the reason.
func reject(ctx context.Context, call *ops.Call, certificate Certificate, reason string) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.certificates SET status = 'rejected', rejection_reason = $2, verified_by_membership_id = $3,
			verified_at = $4,
	version = version + 1, updated_at = $4 WHERE id = $1`,
		certificate.ID,
		reason,
		call.Principal.MembershipID,
		call.Now,
	)
	if err != nil {
		return apperr.Wrap(err, "update maintenance.certificates")
	}

	return nil
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

	call.Emit(outboxEvent("certificate", certificate.ID, "TrainingRequested", nil))
	call.Audit(
		auditEntry(
			"certificates.requestTraining",
			"certificate",
			certificate.ID,
			call.ExpectedVersion,
			&certificate.Version,
			input.Note,
		),
	)

	return certificate, nil
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
		(err == nil && call.Principal.Role == roleContractor && org != call.Principal.OrgID) {
		return DocumentOut{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DocumentOut{}, apperr.Wrap(err, "select maintenance.certificates")
	}

	if key == nil || *key == "" {
		return DocumentOut{}, apperr.E(
			apperr.NotFound,
			apperr.KeyNotFound,
		) // a certificate without an uploaded document
	}

	out := DocumentOut{Name: "certificate", Mime: "", Size: 0, Bytes: nil}
	if name != nil && *name != "" {
		out.Name = *name
	}

	out.Bytes, err = m.Blobs.Get(ctx, *key)
	if errors.Is(err, blob.ErrNotFound) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
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
func (Certificates) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Certificate], error) {
	var filters certFilters

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil || (filters.Code != nil && !certCodes()[*filters.Code]) ||
			(filters.Status != nil && !certStatuses()[*filters.Status]) ||
			(filters.ExpiringWithinDays != nil && (*filters.ExpiringWithinDays < 1 || *filters.ExpiringWithinDays > 3650)) {
			return paging.Page[Certificate]{}, apperr.Fields(
				map[string]string{fieldFilters: apperr.KeyInvalid},
			)
		}
	}

	var args []any

	where := filters.where(call, &args)

	rows, err := call.Tx.Query(
		ctx,
		"SELECT "+certCols+" FROM maintenance.certificates WHERE "+where+" ORDER BY expires_at, id",
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

		if filters.Status == nil || certificate.Status == *filters.Status {
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
		fieldStatus: func(a, b Certificate) int { return strings.Compare(a.Status, b.Status) },
	})
	if sortSliceErr != nil {
		return paging.Page[Certificate]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(all, *query, filters, call.Principal.ScopeVersion)
}

// certFilters are the filters of certificates.list.
type certFilters struct {
	OrganizationID     *uuid.UUID `json:"organizationId,omitempty"`
	MembershipID       *uuid.UUID `json:"membershipId,omitempty"`
	Code               *string    `json:"code,omitempty"`
	Status             *string    `json:"status,omitempty"`
	ExpiringWithinDays *int       `json:"expiringWithinDays,omitempty"` // now <= expiresAt < now + N days
}

// where is the SQL condition of the filters over the certificates the caller may list (a contractor its company's);
// the arguments it uses are added to args.
func (filters certFilters) where(call *ops.Call, args *[]any) string {
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	conds := []string{sqlTrue}
	if call.Principal.Role == roleContractor {
		conds = append(conds, "organization_id = "+add(call.Principal.OrgID))
	}

	if filters.OrganizationID != nil {
		conds = append(conds, "organization_id = "+add(*filters.OrganizationID))
	}

	if filters.MembershipID != nil {
		conds = append(conds, "membership_id = "+add(*filters.MembershipID))
	}

	if filters.Code != nil {
		conds = append(conds, "code = "+add(*filters.Code))
	}

	if filters.ExpiringWithinDays != nil {
		now := add(call.Now)
		conds = append(
			conds,
			"expires_at >= "+now+"::timestamptz AND expires_at < "+now+"::timestamptz + make_interval(days => "+add(
				*filters.ExpiringWithinDays,
			)+"::int)",
		)
	}

	return strings.Join(conds, " AND ")
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
func (Certificates) parts(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[PartItem], error) {
	var filters struct {
		Search *string `json:"search,omitempty"` // code or name contains, case-insensitive
		// that technician's van stock first (van stock is not tracked yet)
		MembershipID *uuid.UUID `json:"membershipId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[PartItem]{}, apperr.Fields(
				map[string]string{fieldFilters: apperr.KeyInvalid},
			)
		}
	}

	stmt, args := `SELECT code, name FROM maintenance.parts_catalog WHERE active`, []any{}
	if filters.Search != nil {
		stmt, args = stmt+` AND (code ILIKE $1 OR name ILIKE $1)`, append(
			args,
			paging.Contains(*filters.Search),
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		stmt+" ORDER BY lower(name), code",
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
		"id":      func(a, b PartItem) int { return strings.Compare(a.Code, b.Code) },
		fieldName: func(a, b PartItem) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) },
	})
	if sortSliceErr != nil {
		return paging.Page[PartItem]{}, apperr.Wrap(sortSliceErr, "paging.SortSlice")
	}

	return pageOf(all, *query, filters, call.Principal.ScopeVersion)
}

// RegisterCertificates binds the IR133 operations.
func RegisterCertificates(reg *ops.Registry, certificates Certificates) {
	ops.Register(reg, "certificates.submit", certificates.submit)
	ops.Register(reg, "certificates.verify", certificates.verify)
	ops.Register(reg, "certificates.requestTraining", certificates.requestTraining)
	ops.Register(reg, "certificates.list", certificates.list)
	ops.Register(reg, "certificates.getContent", certificates.content)
	ops.Register(reg, "parts.list", certificates.parts)
}
