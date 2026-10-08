package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// TwoFactorStatus is TwoFactorStatus of service-contracts.ts (FR-X08 demo, IR112).
type TwoFactorStatus struct {
	Enabled           bool       `json:"enabled"`
	EnabledAt         *time.Time `json:"enabledAt"`
	RecoveryCodesLeft *int       `json:"recoveryCodesLeft"`
	SetupKey          *string    `json:"setupKey"`
}

// TwoFactorEnabled is the enable result: the status plus the recovery codes shown once.
type TwoFactorEnabled struct {
	TwoFactorStatus
	RecoveryCodes []string `json:"recoveryCodes"`
}

var sixDigits = regexp.MustCompile(`^[0-9]{6}$`)

// setupKey is a stable demo base32 key derived from the user ID (no real TOTP secret is stored).
func setupKey(c *ops.Call) string {
	sum := sha256.Sum256([]byte("ac-demo-2fa:" + c.Principal.UserID.String()))
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:10])
}

func twoFactorStatus(ctx context.Context, c *ops.Call) (TwoFactorStatus, error) {
	var at time.Time
	var left int
	err := c.Tx.QueryRow(ctx, `SELECT enabled_at, cardinality(recovery_code_hashes) FROM identity.two_factor WHERE user_id = $1`, c.Principal.UserID).Scan(&at, &left)
	if errors.Is(err, pgx.ErrNoRows) {
		k := setupKey(c)
		return TwoFactorStatus{SetupKey: &k}, nil
	}
	if err != nil {
		return TwoFactorStatus{}, err
	}
	return TwoFactorStatus{Enabled: true, EnabledAt: &at, RecoveryCodesLeft: &left}, nil
}

// CodeInput is twoFactor.enable / disable input.
type CodeInput struct {
	Code string `json:"code"`
}

// Validate implements ops.Validator.
func (in *CodeInput) Validate() map[string]string {
	if !sixDigits.MatchString(in.Code) {
		return map[string]string{"code": "error.invalid"}
	}
	return nil
}

func enableTwoFactor(ctx context.Context, c *ops.Call, in *CodeInput) (TwoFactorEnabled, error) {
	st, err := twoFactorStatus(ctx, c)
	if err != nil {
		return TwoFactorEnabled{}, err
	}
	if st.Enabled {
		return TwoFactorEnabled{}, apperr.E(apperr.Conflict, "errors.two_factor_enabled")
	}
	codes, hashes := make([]string, 8), make([]string, 8)
	for i := range codes {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return TwoFactorEnabled{}, err
		}
		s := hex.EncodeToString(b)
		codes[i] = s[:5] + "-" + s[5:]
		h := sha256.Sum256([]byte(codes[i]))
		hashes[i] = hex.EncodeToString(h[:])
	}
	if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.two_factor (user_id, enabled_at, recovery_code_hashes, updated_at) VALUES ($1, $2, $3, $2)`, c.Principal.UserID, c.Now, hashes); err != nil {
		return TwoFactorEnabled{}, err
	}
	c.Audit(ops.AuditEntry{Action: "twoFactor.enable", TargetKind: "user", TargetID: c.Principal.UserID.String()})
	left := len(codes)
	return TwoFactorEnabled{TwoFactorStatus: TwoFactorStatus{Enabled: true, EnabledAt: &c.Now, RecoveryCodesLeft: &left}, RecoveryCodes: codes}, nil
}

func disableTwoFactor(ctx context.Context, c *ops.Call, in *CodeInput) (TwoFactorStatus, error) {
	tag, err := c.Tx.Exec(ctx, `DELETE FROM identity.two_factor WHERE user_id = $1`, c.Principal.UserID)
	if err != nil {
		return TwoFactorStatus{}, err
	}
	if tag.RowsAffected() == 0 {
		return TwoFactorStatus{}, apperr.E(apperr.Conflict, "errors.two_factor_disabled")
	}
	c.Audit(ops.AuditEntry{Action: "twoFactor.disable", TargetKind: "user", TargetID: c.Principal.UserID.String()})
	return twoFactorStatus(ctx, c)
}

// RegisterTwoFactor binds twoFactor.*.
func RegisterTwoFactor(r *ops.Registry) {
	ops.Register(r, "twoFactor.get", func(ctx context.Context, c *ops.Call, _ *struct{}) (TwoFactorStatus, error) {
		return twoFactorStatus(ctx, c)
	})
	ops.Register(r, "twoFactor.enable", enableTwoFactor)
	ops.Register(r, "twoFactor.disable", disableTwoFactor)
}
