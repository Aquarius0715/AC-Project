package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/blob"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Grants updates qualification grants (Identity).
type Grants interface {
	SetGrant(ctx context.Context, c *ops.Call, membership uuid.UUID, code string, from, until time.Time) error
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

const certCols = `id, tenant_id, version, created_at, updated_at, membership_id, organization_id, code, name, number, issued_at, expires_at, file_name, status,
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
	var x Certificate
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.MembershipID, &x.OrganizationID, &x.Code, &x.Name, &x.Number, &x.IssuedAt,
		&x.ExpiresAt, &x.FileName, &x.Status, &x.RenewalOf, &x.VerifiedByMembershipID, &x.VerifiedAt, &x.TrainingRequestedAt)
	x.Status = readStatus(x.Status, x.ExpiresAt, now)
	return x, err
}

func (m Certificates) lock(ctx context.Context, c *ops.Call, id uuid.UUID) (Certificate, string, error) {
	var stored string
	x, err := scanCert(c.Tx.QueryRow(ctx, "SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1 FOR UPDATE", id), c.Now)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Principal.Role == "contractor" && x.OrganizationID != c.Principal.OrgID) {
		return x, "", apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, "", err
	}
	if err := c.Tx.QueryRow(ctx, `SELECT status FROM maintenance.certificates WHERE id = $1`, id).Scan(&stored); err != nil {
		return x, "", err
	}
	if x.Version != *c.ExpectedVersion {
		return x, "", apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return x, stored, nil
}

func (m Certificates) reload(ctx context.Context, c *ops.Call, id uuid.UUID) (Certificate, error) {
	return scanCert(c.Tx.QueryRow(ctx, "SELECT "+certCols+" FROM maintenance.certificates WHERE id = $1", id), c.Now)
}

// ---- certificates.submit ----

var certCodes = map[string]bool{"demo_indoor": true, "demo_outdoor": true, "demo_electrical": true, "other": true}

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
	fe := map[string]string{}
	if in.MembershipID == uuid.Nil {
		fe["membershipId"] = "error.required"
	}
	if !certCodes[in.Code] {
		fe["code"] = "error.invalid"
	}
	trimmedReason(&in.Name, 120, "name", fe)
	trimmedReason(&in.Number, 64, "number", fe)
	if in.IssuedAt.IsZero() || !in.IssuedAt.Before(in.ExpiresAt) {
		fe["expiresAt"] = "error.range"
	}
	f := in.File
	n := utf8.RuneCountInString(strings.TrimSpace(f.Name))
	if n < 1 || n > 200 || (f.Mime != "application/pdf" && f.Mime != "image/jpeg" && f.Mime != "image/png") || f.Size != len(f.Bytes) || f.Size < 1 || f.Size > 10*1000*1000 {
		fe["file"] = "error.invalidFile"
	}
	return fe
}

func (m Certificates) submit(ctx context.Context, c *ops.Call, in *SubmitCertInput) (Certificate, error) {
	t, found, err := m.Delivery.Directory.Technician(ctx, c, in.MembershipID)
	if err != nil {
		return Certificate{}, err
	}
	if !found || !t.Active || t.OrgID != c.Principal.OrgID {
		return Certificate{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if in.RenewalOf != nil {
		var ok bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM maintenance.certificates WHERE id = $1 AND membership_id = $2 AND code = $3)`, *in.RenewalOf, in.MembershipID, in.Code).Scan(&ok); err != nil {
			return Certificate{}, err
		}
		if !ok {
			return Certificate{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	id := uuid.New()
	key := "certificates/" + in.MembershipID.String() + "/" + id.String()
	if err := m.Blobs.Put(ctx, key, in.File.Mime, in.File.Bytes); err != nil {
		return Certificate{}, err
	}
	name := strings.TrimSpace(in.File.Name)
	if _, err := c.Tx.Exec(ctx, `INSERT INTO maintenance.certificates (id, tenant_id, membership_id, organization_id, code, name, number, issued_at, expires_at, object_key,
		file_name, status, renewal_of, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending_verification', $11, $12, $12)`,
		id, in.MembershipID, t.OrgID, in.Code, in.Name, in.Number, in.IssuedAt, in.ExpiresAt, key, name, in.RenewalOf, c.Now); err != nil {
		return Certificate{}, err
	}
	x, err := m.reload(ctx, c, id)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "certificate", AggregateID: id, Type: "CertificateSubmitted", Payload: map[string]any{"membershipId": in.MembershipID}})
	c.Audit(ops.AuditEntry{Action: "certificates.submit", TargetKind: "certificate", TargetID: id.String(), NextVersion: &x.Version})
	return x, nil
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
	fe := map[string]string{}
	if in.CertificateID == uuid.Nil {
		fe["certificateId"] = "error.required"
	}
	switch in.Decision {
	case "approve":
	case "reject":
		if in.Reason == nil {
			fe["reason"] = "error.required"
		} else {
			trimmedReason(in.Reason, 1000, "reason", fe)
		}
	default:
		fe["decision"] = "error.invalid"
	}
	return fe
}

func (m Certificates) verify(ctx context.Context, c *ops.Call, in *VerifyInput) (Certificate, error) {
	x, stored, err := m.lock(ctx, c, in.CertificateID)
	if err != nil {
		return x, err
	}
	if stored != "pending_verification" {
		return x, apperr.E(apperr.Conflict, "error.invalidState")
	}
	if in.Decision == "approve" {
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.certificates SET status = 'valid', verified_by_membership_id = $2, verified_at = $3, version = version + 1, updated_at = $3 WHERE id = $1`,
			x.ID, c.Principal.MembershipID, c.Now); err != nil {
			return x, err
		}
		if x.Code != "other" {
			if err := m.Grants.SetGrant(ctx, c, x.MembershipID, x.Code, x.IssuedAt, x.ExpiresAt); err != nil {
				return x, err
			}
		}
	} else if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.certificates SET status = 'rejected', rejection_reason = $2, verified_by_membership_id = $3, verified_at = $4,
		version = version + 1, updated_at = $4 WHERE id = $1`, x.ID, *in.Reason, c.Principal.MembershipID, c.Now); err != nil {
		return x, err
	}
	x, err = m.reload(ctx, c, x.ID)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "certificate", AggregateID: x.ID, Type: "CertificateVerified", Payload: map[string]any{"decision": in.Decision}})
	c.Audit(ops.AuditEntry{Action: "certificates.verify", TargetKind: "certificate", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version, Reason: deref(in.Reason)})
	return x, nil
}

// ---- certificates.requestTraining ----

// TrainingInput is certificates.requestTraining input.
type TrainingInput struct {
	CertificateID uuid.UUID `json:"certificateId"`
	Note          string    `json:"note"`
}

// Validate implements ops.Validator.
func (in *TrainingInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.CertificateID == uuid.Nil {
		fe["certificateId"] = "error.required"
	}
	trimmedReason(&in.Note, 1000, "note", fe)
	return fe
}

func (m Certificates) requestTraining(ctx context.Context, c *ops.Call, in *TrainingInput) (Certificate, error) {
	x, _, err := m.lock(ctx, c, in.CertificateID)
	if err != nil {
		return x, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.certificates SET training_requested_at = $2, training_request_note = $3, version = version + 1, updated_at = $2 WHERE id = $1`,
		x.ID, c.Now, in.Note); err != nil {
		return x, err
	}
	x, err = m.reload(ctx, c, x.ID)
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "certificate", AggregateID: x.ID, Type: "TrainingRequested"})
	c.Audit(ops.AuditEntry{Action: "certificates.requestTraining", TargetKind: "certificate", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version, Reason: in.Note})
	return x, nil
}

// ---- certificates.list ----

func (m Certificates) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Certificate], error) {
	var f struct {
		MembershipID *uuid.UUID `json:"membershipId,omitempty"`
		Code         *string    `json:"code,omitempty"`
		Status       *string    `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Code != nil && !certCodes[*f.Code]) {
			return paging.Page[Certificate]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "contractor" {
		conds = append(conds, "organization_id = "+add(c.Principal.OrgID))
	}
	if f.MembershipID != nil {
		conds = append(conds, "membership_id = "+add(*f.MembershipID))
	}
	if f.Code != nil {
		conds = append(conds, "code = "+add(*f.Code))
	}
	rows, err := c.Tx.Query(ctx, "SELECT "+certCols+" FROM maintenance.certificates WHERE "+strings.Join(conds, " AND ")+" ORDER BY expires_at, id", args...)
	if err != nil {
		return paging.Page[Certificate]{}, err
	}
	all := []Certificate{}
	for rows.Next() {
		x, err := scanCert(rows, c.Now)
		if err != nil {
			rows.Close()
			return paging.Page[Certificate]{}, err
		}
		if f.Status == nil || x.Status == *f.Status {
			all = append(all, x)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Certificate]{}, err
	}
	return pageOf(all, *in, f, c.Principal.ScopeVersion)
}

// ---- parts.list ----

// PartItem is PartCatalogItem.
type PartItem struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	VanStockQuantity *int   `json:"vanStockQuantity"`
}

func (m Certificates) parts(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[PartItem], error) {
	var f struct {
		Q *string `json:"q,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[PartItem]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	q, args := `SELECT code, name FROM maintenance.parts_catalog WHERE active`, []any{}
	if f.Q != nil {
		q, args = q+` AND (code ILIKE $1 OR name ILIKE $1)`, append(args, "%"+strings.TrimSpace(*f.Q)+"%")
	}
	rows, err := c.Tx.Query(ctx, q+" ORDER BY code", args...)
	if err != nil {
		return paging.Page[PartItem]{}, err
	}
	all := []PartItem{}
	for rows.Next() {
		var p PartItem
		if err := rows.Scan(&p.Code, &p.Name); err != nil {
			rows.Close()
			return paging.Page[PartItem]{}, err
		}
		all = append(all, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[PartItem]{}, err
	}
	return pageOf(all, *in, f, c.Principal.ScopeVersion)
}

// RegisterCertificates binds the IR133 operations.
func RegisterCertificates(r *ops.Registry, m Certificates) {
	ops.Register(r, "certificates.submit", m.submit)
	ops.Register(r, "certificates.verify", m.verify)
	ops.Register(r, "certificates.requestTraining", m.requestTraining)
	ops.Register(r, "certificates.list", m.list)
	ops.Register(r, "parts.list", m.parts)
}
