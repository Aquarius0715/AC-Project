// Package auth authenticates requests to the Core API: it verifies the bearer token, reads the selected
// Membership (X-Tenant-Id + X-Membership-Id from the BFF session context, backend architecture §5) inside the
// tenant's RLS context, and stores the resulting ops.Principal in the request context. Requests without a token
// stay anonymous so public demo operations can run; the dispatcher rejects anonymous calls to other operations.
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Verifier checks a bearer token and returns the subject (Cognito/Keycloak `sub`, or the user ID locally).
type Verifier interface {
	Verify(ctx context.Context, token string) (string, error)
}

// StaticVerifier maps fixed tokens to subjects (tests and local tooling only).
type StaticVerifier map[string]string

// Verify implements Verifier.
func (s StaticVerifier) Verify(_ context.Context, token string) (string, error) {
	if sub, ok := s[token]; ok {
		return sub, nil
	}
	return "", errors.New("unknown token")
}

// PrincipalSource resolves the selected Membership of a token subject into an ops.Principal (IR181 step 1):
// identity-api reads its own tables (DB), every other service asks identity-api (RemoteSource).
type PrincipalSource interface {
	Principal(ctx context.Context, subject string, tenant, membership uuid.UUID) (*ops.Principal, error)
}

// ErrUnavailable means the principal could not be resolved because identity-api did not answer.
var ErrUnavailable = errors.New("auth: principal source unavailable")

// Authenticator builds principals. Source wins when set; otherwise the identity tables are read through DB.
type Authenticator struct {
	Verifier Verifier
	DB       ops.Runner
	Now      func() time.Time
	Source   PrincipalSource
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
			sub, err := a.Verifier.Verify(c.Request().Context(), token)
			if err != nil {
				return unauth(c, "error.unauthenticated")
			}
			tenant, err1 := uuid.Parse(c.Request().Header.Get("X-Tenant-Id"))
			membership, err2 := uuid.Parse(c.Request().Header.Get("X-Membership-Id"))
			if err1 != nil || err2 != nil {
				return unauth(c, "error.membershipRequired")
			}
			p, err := a.principal(c.Request().Context(), sub, tenant, membership)
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

func (a *Authenticator) principal(ctx context.Context, subject string, tenant, membership uuid.UUID) (*ops.Principal, error) {
	if a.Source != nil {
		return a.Source.Principal(ctx, subject, tenant, membership)
	}
	return a.Load(ctx, subject, tenant, membership)
}

// Principal implements PrincipalSource with the identity tables (identity-api).
func (a *Authenticator) Principal(ctx context.Context, subject string, tenant, membership uuid.UUID) (*ops.Principal, error) {
	return a.Load(ctx, subject, tenant, membership)
}

// Load reads the Membership for subject in tenant; it fails when the membership does not belong to the subject,
// is outside its validity window, or the user is not active.
func (a *Authenticator) Load(ctx context.Context, subject string, tenant, membership uuid.UUID) (*ops.Principal, error) {
	p := &ops.Principal{TenantID: tenant, MembershipID: membership}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now()
	}
	err := a.DB.Run(ctx, true, p, func(tx pgx.Tx) error {
		var clientRole, employment *string
		var validUntil *time.Time
		var validFrom time.Time
		var status string
		if err := tx.QueryRow(ctx, `SELECT m.user_id, m.organization_id, m.role, m.client_role, m.employment, m.scope_version, m.valid_from, m.valid_until, u.status
			FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
			WHERE m.id = $1 AND (u.cognito_sub = $2 OR u.id::text = $2)`, membership, subject).
			Scan(&p.UserID, &p.OrgID, &p.Role, &clientRole, &employment, &p.ScopeVersion, &validFrom, &validUntil, &status); err != nil {
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
