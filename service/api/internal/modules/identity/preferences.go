package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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

func loadPreferences(ctx context.Context, call *ops.Call) (Preferences, error) {
	preferences := Preferences{
		Locale:             "en",
		Timezone:           "Asia/Kuala_Lumpur",
		Currency:           "MYR",
		MonthlyReportEmail: false,
		TwoFactorEnabled:   false,
	}

	err := call.Tx.QueryRow(ctx, `SELECT locale, timezone, currency, monthly_report_email FROM identity.preferences WHERE
		user_id = $1`, call.Principal.UserID).
		Scan(&preferences.Locale, &preferences.Timezone, &preferences.Currency, &preferences.MonthlyReportEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}

	if err != nil {
		return preferences, err
	}

	err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.two_factor WHERE user_id =
		$1)`, call.Principal.UserID).
		Scan(&preferences.TwoFactorEnabled)

	return preferences, apperr.Wrap(err, "select identity.two_factor")
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
	fieldErrs := map[string]string{}
	if !slices.Contains([]string{"en", "ms", "ja", "id"}, in.Locale) {
		fieldErrs["locale"] = apperr.KeyInvalid
	}

	if !ValidTimezone(in.Timezone) {
		fieldErrs["timezone"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// updatePreferences saves the caller's preferences; monthlyReportEmail is accepted only from client sessions (IR112).
//
//	@Summary		preferences.update (write)
//	@ID				preferences.update
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: D01; input constraints in the corresponding DD; IR112 monthlyReportEmail only for client \
//	@Description	sessions
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
func updatePreferences(
	ctx context.Context,
	call *ops.Call,
	input *PreferencesInput,
) (Preferences, error) {
	if input.MonthlyReportEmail != nil && call.Principal.Role != authz.RoleClient {
		return Preferences{}, apperr.Fields(
			map[string]string{"monthlyReportEmail": apperr.KeyNotAllowed},
		)
	}

	cur, err := loadPreferences(ctx, call)
	if err != nil {
		return cur, err
	}

	monthly := cur.MonthlyReportEmail
	if input.MonthlyReportEmail != nil {
		monthly = *input.MonthlyReportEmail
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO identity.preferences (user_id, locale, timezone, monthly_report_email, updated_at) VALUES ($1, $2, $3,
			$4, $5)
		ON CONFLICT (user_id) DO UPDATE SET locale = $2, timezone = $3, monthly_report_email = $4, version =
		identity.preferences.version + 1, updated_at = $5`,
		call.Principal.UserID,
		input.Locale,
		input.Timezone,
		monthly,
		call.Now,
	)
	if execErr != nil {
		return cur, apperr.Wrap(execErr, "insert identity.preferences")
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "preferences.update",
			TargetKind:      "preferences",
			TargetID:        call.Principal.UserID.String(),
			PreviousVersion: nil,
			NextVersion:     nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	cur.Locale, cur.Timezone, cur.MonthlyReportEmail = input.Locale, input.Timezone, monthly

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
		return map[string]string{"purpose": apperr.KeyInvalid}
	}

	return nil
}

func loadConsent(ctx context.Context, call *ops.Call, purpose string, lock bool) (Consent, error) {
	stmt := `SELECT id::text, tenant_id::text, version, created_at, updated_at, membership_id::text, purpose, granted,
		granted_at, revoked_at
		FROM identity.consents WHERE membership_id = $1 AND purpose = $2`
	if lock {
		stmt += " FOR UPDATE"
	}

	var record Consent

	err := call.Tx.QueryRow(ctx, stmt, call.Principal.MembershipID, purpose).
		Scan(&record.ID, &record.TenantID, &record.Version, &record.CreatedAt, &record.UpdatedAt, &record.MembershipID,
			&record.Purpose, &record.Granted,
			&record.GrantedAt, &record.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) { // IR84: a missing record is a fixture defect
		return record, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return record, apperr.Wrap(err, "update (loadConsent)")
}

// getConsent serves consents.get.
//
//	@Summary		consents.get (read)
//	@ID				consents.get
//	@Description	Authorization: client:own-membership
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR84 seed/initial \
//	@Description	record; absent record NOT_FOUND
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C05, DD-C01
//	@Tags			consents
//	@Accept			json
//	@Produce		json
//	@Param			purpose	path		string	true	"input field purpose"
//	@Success		200		{object}	ops.Envelope{data=Consent}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/consents/{purpose} [get]
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
		return map[string]string{"purpose": apperr.KeyInvalid}
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
func updateConsent(
	ctx context.Context,
	call *ops.Call,
	input *ConsentUpdateInput,
) (Consent, error) {
	record, err := loadConsent(ctx, call, input.Purpose, true)
	if err != nil {
		return record, err
	}

	if record.Version != *call.ExpectedVersion {
		return record, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if record.Granted == input.Granted {
		return record, nil
	}

	if input.Granted {
		record.GrantedAt, record.RevokedAt = &call.Now, nil
	} else {
		record.RevokedAt = &call.Now
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE identity.consents SET granted = $2, granted_at = $3, revoked_at = $4, version = version + 1, updated_at = $5
			WHERE id = $1::uuid`,
		record.ID,
		input.Granted,
		record.GrantedAt,
		record.RevokedAt,
		call.Now,
	)
	if execErr != nil {
		return record, apperr.Wrap(execErr, "update identity.consents")
	}

	prev := record.Version

	record.Granted, record.Version, record.UpdatedAt = input.Granted, record.Version+1, call.Now
	if !input.Granted { // equipment disables the membership's location automations (IR53; another domain, so an event)
		id, _ := uuid.Parse(record.ID)
		call.Emit(ops.Event{
			AggregateType: "consent",
			AggregateID:   id,
			Type:          events.ConsentRevoked,
			Payload: events.ConsentRevocation{
				MembershipID: call.Principal.MembershipID,
				Purpose:      input.Purpose,
				At:           call.Now,
			},
		})
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "consents.update",
			TargetKind:      "consent",
			TargetID:        record.ID,
			PreviousVersion: &prev,
			NextVersion:     &record.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return record, nil
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
//	@Description	Design: DDC-07, DD-C04, DD-C15
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
