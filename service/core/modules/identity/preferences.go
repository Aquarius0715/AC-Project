package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
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
	c.Audit(ops.AuditEntry{Action: "consents.update", TargetKind: "consent", TargetID: x.ID, PreviousVersion: &prev, NextVersion: &x.Version})
	return x, nil
}

// RegisterPreferences binds preferences.* and consents.*.
func RegisterPreferences(r *ops.Registry) {
	ops.Register(r, "preferences.get", func(ctx context.Context, c *ops.Call, _ *struct{}) (Preferences, error) {
		return loadPreferences(ctx, c)
	})
	ops.Register(r, "preferences.update", updatePreferences)
	ops.Register(r, "consents.get", getConsent)
	ops.Register(r, "consents.update", updateConsent)
}
