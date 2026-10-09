package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// Preferences is Preferences of service-contracts.ts (per user; defaults when no row exists).
type Preferences struct {
	Locale             string `json:"locale"`
	Timezone           string `json:"timezone"`
	Currency           string `json:"currency"`
	MonthlyReportEmail bool   `json:"monthlyReportEmail"`
	TwoFactorEnabled   bool   `json:"twoFactorEnabled"`
}

func loadPreferences(ctx context.Context, c *ops.Call) (Preferences, error) {
	p := Preferences{Locale: "en", Timezone: "Asia/Kuala_Lumpur", Currency: "MYR"}
	err := c.Tx.QueryRow(ctx, `SELECT locale, timezone, currency, monthly_report_email FROM identity.preferences WHERE user_id = $1`, c.Principal.UserID).
		Scan(&p.Locale, &p.Timezone, &p.Currency, &p.MonthlyReportEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return p, err
	}
	err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.two_factor WHERE user_id = $1)`, c.Principal.UserID).Scan(&p.TwoFactorEnabled)
	return p, err
}

// PreferencesInput is preferences.update input.
type PreferencesInput struct {
	Locale             string `json:"locale"`
	Timezone           string `json:"timezone"`
	MonthlyReportEmail *bool  `json:"monthlyReportEmail,omitempty"`
}

// ValidTimezone reports whether tz is a supported IANA zone name (not "Local").
func ValidTimezone(tz string) bool {
	if tz != "UTC" && !strings.Contains(tz, "/") {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// Validate implements ops.Validator.
func (in *PreferencesInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.Locale != "en" && in.Locale != "ms" {
		fe["locale"] = "error.invalid"
	}
	if !ValidTimezone(in.Timezone) {
		fe["timezone"] = "error.invalid"
	}
	return fe
}

// updatePreferences saves the caller's preferences; monthlyReportEmail is accepted only from client sessions (IR112).
//
//	@Summary		preferences.update (write)
//	@ID				preferences.update
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR112 monthlyReportEmail only for client sessions
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			preferences
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		PreferencesInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Preferences}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/preferences [put]
func updatePreferences(ctx context.Context, c *ops.Call, in *PreferencesInput) (Preferences, error) {
	if in.MonthlyReportEmail != nil && c.Principal.Role != "client" {
		return Preferences{}, apperr.Fields(map[string]string{"monthlyReportEmail": "error.notAllowed"})
	}
	cur, err := loadPreferences(ctx, c)
	if err != nil {
		return cur, err
	}
	monthly := cur.MonthlyReportEmail
	if in.MonthlyReportEmail != nil {
		monthly = *in.MonthlyReportEmail
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.preferences (user_id, locale, timezone, monthly_report_email, updated_at) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET locale = $2, timezone = $3, monthly_report_email = $4, version = identity.preferences.version + 1, updated_at = $5`,
		c.Principal.UserID, in.Locale, in.Timezone, monthly, c.Now); err != nil {
		return cur, err
	}
	c.Audit(ops.AuditEntry{Action: "preferences.update", TargetKind: "preferences", TargetID: c.Principal.UserID.String()})
	cur.Locale, cur.Timezone, cur.MonthlyReportEmail = in.Locale, in.Timezone, monthly
	return cur, nil
}

// Consent is Consent of service-contracts.ts.
type Consent struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenantId"`
	Version      int        `json:"version"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	MembershipID string     `json:"membershipId"`
	Purpose      string     `json:"purpose"`
	Granted      bool       `json:"granted"`
	GrantedAt    *time.Time `json:"grantedAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
}

// ConsentGetInput is consents.get input.
type ConsentGetInput struct {
	Purpose string `json:"purpose"`
}

// Validate implements ops.Validator.
func (in *ConsentGetInput) Validate() map[string]string {
	if in.Purpose != "location_automation" {
		return map[string]string{"purpose": "error.invalid"}
	}
	return nil
}

func loadConsent(ctx context.Context, c *ops.Call, purpose string, lock bool) (Consent, error) {
	q := `SELECT id::text, tenant_id::text, version, created_at, updated_at, membership_id::text, purpose, granted, granted_at, revoked_at
		FROM identity.consents WHERE membership_id = $1 AND purpose = $2`
	if lock {
		q += " FOR UPDATE"
	}
	var x Consent
	err := c.Tx.QueryRow(ctx, q, c.Principal.MembershipID, purpose).Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.MembershipID, &x.Purpose, &x.Granted, &x.GrantedAt, &x.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) { // IR84: a missing record is a fixture defect
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// @Summary		consents.get (read)
// @ID				consents.get
// @Description	Authorization: client:own-membership
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR84 seed/initial record; absent record NOT_FOUND
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C05
// @Tags			consents
// @Accept			json
// @Produce		json
// @Param			purpose	path		string	true	"input field purpose"
// @Success		200		{object}	ops.Envelope{data=Consent}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/consents/{purpose} [get]
func getConsent(ctx context.Context, c *ops.Call, in *ConsentGetInput) (Consent, error) {
	return loadConsent(ctx, c, in.Purpose, false)
}

// ConsentUpdateInput is consents.update input.
type ConsentUpdateInput struct {
	Purpose string `json:"purpose"`
	Granted bool   `json:"granted"`
}

// Validate implements ops.Validator.
func (in *ConsentUpdateInput) Validate() map[string]string {
	if in.Purpose != "location_automation" {
		return map[string]string{"purpose": "error.invalid"}
	}
	return nil
}

// updateConsent grants or revokes location consent (SR02): granting sets grantedAt=now and clears revokedAt;
// revoking keeps grantedAt and sets revokedAt=now. The same value returns the record unchanged.
//
//	@Summary		consents.update (write)
//	@ID				consents.update
//	@Description	Authorization: client:own-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C05
//	@Tags			consents
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target consent, read consents.get)"
//	@Param			purpose				path		string				true	"input field purpose"
//	@Param			request				body		ConsentUpdateInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Consent}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/consents/{purpose} [put]
func updateConsent(ctx context.Context, c *ops.Call, in *ConsentUpdateInput) (Consent, error) {
	x, err := loadConsent(ctx, c, in.Purpose, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if x.Granted == in.Granted {
		return x, nil
	}
	if in.Granted {
		x.GrantedAt, x.RevokedAt = &c.Now, nil
	} else {
		x.RevokedAt = &c.Now
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE identity.consents SET granted = $2, granted_at = $3, revoked_at = $4, version = version + 1, updated_at = $5 WHERE id = $1::uuid`,
		x.ID, in.Granted, x.GrantedAt, x.RevokedAt, c.Now); err != nil {
		return x, err
	}
	prev := x.Version
	x.Granted, x.Version, x.UpdatedAt = in.Granted, x.Version+1, c.Now
	if !in.Granted { // equipment disables the membership's location automations (IR53; another domain, so an event)
		id, _ := uuid.Parse(x.ID)
		c.Emit(ops.Event{AggregateType: "consent", AggregateID: id, Type: events.ConsentRevoked,
			Payload: events.ConsentRevocation{MembershipID: c.Principal.MembershipID, Purpose: in.Purpose, At: c.Now}})
	}
	c.Audit(ops.AuditEntry{Action: "consents.update", TargetKind: "consent", TargetID: x.ID, PreviousVersion: &prev, NextVersion: &x.Version})
	return x, nil
}

// RegisterPreferences binds preferences.* and consents.*.
func RegisterPreferences(r *ops.Registry) {
	ops.Register(r, "preferences.get", preferencesGet)
	ops.Register(r, "preferences.update", updatePreferences)
	ops.Register(r, "consents.get", getConsent)
	ops.Register(r, "consents.update", updateConsent)
}

// preferencesGet answers preferences.get: the caller's preferences.
//
//	@Summary		preferences.get (read)
//	@ID				preferences.get
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DDC-07, DD-C04
//	@Tags			preferences
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ops.Envelope{data=Preferences}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/preferences [get]
func preferencesGet(ctx context.Context, c *ops.Call, _ *struct{}) (Preferences, error) {
	return loadPreferences(ctx, c)
}
