// Package auth authenticates requests to the Core API: it verifies the bearer token, reads the selected
// Membership (X-Tenant-Id + X-Membership-Id from the BFF session context, backend architecture §5) inside the
// tenant's RLS context, and stores the resulting ops.Principal in the request context. Requests without a token
// stay anonymous so public demo operations can run; the dispatcher rejects anonymous calls to other operations.
// The token's sign-in time becomes the user's last sign-in (IR268).
package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Identity is what a verified token says about its holder: the subject (Cognito/Keycloak `sub`, or the user ID
// locally) and, when the identity provider sends it, when the holder signed in (OIDC `auth_time`).
type Identity struct {
	Subject  string
	SignedIn time.Time // zero when the token does not say
}

// Verifier checks a bearer token and returns who holds it.
type Verifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}

// StaticVerifier maps fixed tokens to subjects (tests and local tooling only); its tokens carry no sign-in time.
type StaticVerifier map[string]string

// Verify implements Verifier.
func (s StaticVerifier) Verify(_ context.Context, token string) (Identity, error) {
	if sub, ok := s[token]; ok {
		return Identity{Subject: sub}, nil
	}
	return Identity{}, errors.New("unknown token")
}

// PrincipalSource resolves the selected Membership of a token subject into an ops.Principal (IR181 step 1):
// identity-api reads its own tables (DB), every other service asks identity-api (RemoteSource). signedIn is the
// token's sign-in time (zero: none), which identity records as the user's last sign-in (IR268).
type PrincipalSource interface {
	Principal(ctx context.Context, subject string, tenant, membership uuid.UUID, signedIn time.Time) (*ops.Principal, error)
}

// ErrUnavailable means the principal could not be resolved because identity-api did not answer.
var ErrUnavailable = errors.New("auth: principal source unavailable")

// Authenticator builds principals. Source wins when set; otherwise the identity tables are read through DB.
type Authenticator struct {
	Verifier Verifier
	DB       ops.Runner
	Now      func() time.Time
	Source   PrincipalSource

	signIns sync.Map // user ID → the latest sign-in time this process has recorded
}

func (a *Authenticator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now().UTC()
}

// unauth returns the 401 DomainError; Echo's HTTPErrorHandler writes it.
func unauth(c *echo.Context, key string) error {
	de := apperr.E(apperr.Unauthenticated, key)
	de.CorrelationID = c.Response().Header().Get(echo.HeaderXRequestID)
	return de
}

// Middleware returns the Echo middleware.
func (a *Authenticator) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Request().Header.Get(echo.HeaderAuthorization)
			if h == "" {
				return next(c)
			}
			token, ok := strings.CutPrefix(h, "Bearer ")
			if !ok || token == "" {
				return unauth(c, "error.unauthenticated")
			}
			id, err := a.Verifier.Verify(c.Request().Context(), token)
			if err != nil {
				return unauth(c, "error.unauthenticated")
			}
			tenant, err1 := uuid.Parse(c.Request().Header.Get("X-Tenant-Id"))
			membership, err2 := uuid.Parse(c.Request().Header.Get("X-Membership-Id"))
			if err1 != nil || err2 != nil {
				return unauth(c, "error.membershipRequired")
			}
			p, err := a.principal(c.Request().Context(), id.Subject, tenant, membership, id.SignedIn)
			if errors.Is(err, ErrUnavailable) {
				return apperr.E(apperr.Unavailable, "error.unavailable")
			}
			if err != nil {
				return unauth(c, "error.membershipInvalid")
			}
			c.SetRequest(c.Request().WithContext(ops.WithPrincipal(c.Request().Context(), p)))
			return next(c)
		}
	}
}

func (a *Authenticator) principal(ctx context.Context, subject string, tenant, membership uuid.UUID, signedIn time.Time) (*ops.Principal, error) {
	if a.Source != nil {
		return a.Source.Principal(ctx, subject, tenant, membership, signedIn)
	}
	return a.Principal(ctx, subject, tenant, membership, signedIn)
}

// Principal implements PrincipalSource with the identity tables (identity-api) and records a new sign-in.
func (a *Authenticator) Principal(ctx context.Context, subject string, tenant, membership uuid.UUID, signedIn time.Time) (*ops.Principal, error) {
	p, err := a.Load(ctx, subject, tenant, membership)
	if err == nil {
		a.RecordSignIn(ctx, p, signedIn)
	}
	return p, err
}

// RecordSignIn notes a sign-in of the principal's user (IR268). When signedIn (the token's auth_time) is later than
// the sign-in recorded last, last_sign_in_at becomes the business time now. In the demo that is the demo clock, so
// the time reads like the rest of the scenario. The token of an older session does not move it back. A sign-in this
// process has already recorded costs no query, and a failure is logged without failing the request.
func (a *Authenticator) RecordSignIn(ctx context.Context, p *ops.Principal, signedIn time.Time) {
	if p == nil || signedIn.IsZero() {
		return
	}
	signedIn = signedIn.UTC().Truncate(time.Second)
	if last, ok := a.signIns.Load(p.UserID); ok && !signedIn.After(last.(time.Time)) {
		return
	}
	err := a.DB.Run(ctx, false, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE identity.users SET last_sign_in_at = $3, sign_in_auth_time = $2
			WHERE id = $1 AND (sign_in_auth_time IS NULL OR sign_in_auth_time < $2)`, p.UserID, signedIn, a.now())
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "auth: sign-in not recorded", "user", p.UserID, "err", err)
		return
	}
	a.signIns.Store(p.UserID, signedIn)
}

// Load reads the Membership for subject in tenant; it fails when the membership does not belong to the subject,
// is outside its validity window, or the user is not active.
func (a *Authenticator) Load(ctx context.Context, subject string, tenant, membership uuid.UUID) (*ops.Principal, error) {
	p := &ops.Principal{TenantID: tenant, MembershipID: membership}
	now := a.now()
	err := a.DB.Run(ctx, true, p, func(tx pgx.Tx) error {
		var clientRole, employment, timezone *string
		var validUntil *time.Time
		var validFrom time.Time
		var status string
		if err := tx.QueryRow(ctx, `SELECT m.user_id, m.organization_id, m.role, m.client_role, m.employment, m.scope_version, m.valid_from, m.valid_until, u.status, pf.timezone
			FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id LEFT JOIN identity.preferences pf ON pf.user_id = u.id
			WHERE m.id = $1 AND (u.cognito_sub = $2 OR u.id::text = $2)`, membership, subject).
			Scan(&p.UserID, &p.OrgID, &p.Role, &clientRole, &employment, &p.ScopeVersion, &validFrom, &validUntil, &status, &timezone); err != nil {
			return err
		}
		if status != "active" || now.Before(validFrom) || (validUntil != nil && !now.Before(*validUntil)) {
			return apperr.E(apperr.NotFound, "error.membershipInvalid") // inactive user or outside the validity window
		}
		if clientRole != nil {
			p.ClientRole = *clientRole
		}
		if employment != nil {
			p.Employment = *employment
		}
		if timezone != nil {
			p.Timezone = *timezone
		}
		rows, err := tx.Query(ctx, `SELECT permission FROM identity.membership_permissions WHERE membership_id = $1`, membership)
		if err != nil {
			return err
		}
		p.Permissions = map[string]bool{}
		for rows.Next() {
			var perm string
			if err := rows.Scan(&perm); err != nil {
				return err
			}
			p.Permissions[perm] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
		scopes, err := tx.Query(ctx, `SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1`, membership)
		if err != nil {
			return err
		}
		p.Scopes = map[string][]uuid.UUID{}
		for scopes.Next() {
			var kind string
			var ref uuid.UUID
			if err := scopes.Scan(&kind, &ref); err != nil {
				return err
			}
			p.Scopes[kind] = append(p.Scopes[kind], ref)
		}
		return scopes.Err()
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}
