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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// Identity is what a verified token says about its holder: the subject (Cognito/Keycloak `sub`, or the user ID
// locally) and, when the identity provider sends it, when the holder signed in (OIDC `auth_time`).
type Identity struct {
	Subject  string
	SignedIn time.Time // zero when the token does not say
	// Client is the app the token was issued to (azp / client_id), empty when the verifier does not say. AdminApp says
	// it is HQ's app, whose sign-in asks for the one-time code every time (DEC-85, IR510).
	Client   string
	AdminApp bool
	// Issuer is the token's verified issuer, empty when the verifier does not say. It names the address the holder
	// signed in on (IR651, IR695).
	Issuer string
}

// Verifier checks a bearer token and returns who holds it.
type Verifier interface {
	Verify(ctx context.Context, token string) (Identity, error)
}

// StaticVerifier maps fixed tokens to subjects (tests and local tooling only); its tokens carry no sign-in time.
type StaticVerifier map[string]string

// errUnknownToken is a StaticVerifier's answer for a token it does not know.
var errUnknownToken = errors.New("unknown token")

// Verify implements Verifier.
func (s StaticVerifier) Verify(_ context.Context, token string) (Identity, error) {
	if sub, ok := s[token]; ok {
		return Identity{
			Subject:  sub,
			SignedIn: time.Time{},
			Client:   "",
			AdminApp: false,
			Issuer:   "",
		}, nil
	}

	return Identity{}, errUnknownToken
}

// Revoked reports whether a sign-in at signedIn ended with the principal's sessions (members.revokeSessions, IR384):
// it began before the revocation or within its second, or the token does not say when it began. auth_time has whole
// seconds, so a sign-in in the revocation's own second cannot be told from one just before it and is refused too: a
// sign-in a second later works (IR385; the E2E run found such a sign-in surviving).
func Revoked(p *ops.Principal, signedIn time.Time) bool {
	if p == nil || p.SessionsRevokedAt == nil {
		return false
	}

	return signedIn.IsZero() || !signedIn.After(p.SessionsRevokedAt.Truncate(time.Second))
}

// PrincipalSource resolves the selected Membership of a token subject into an ops.Principal (IR181 step 1):
// identity-api reads its own tables (DB), every other service asks identity-api (RemoteSource). signedIn is the
// token's sign-in time (zero: none), which identity records as the user's last sign-in (IR268).
type PrincipalSource interface {
	Principal(
		ctx context.Context,
		subject string,
		tenant, membership uuid.UUID,
		signedIn time.Time,
	) (*ops.Principal, error)
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

// unauth returns the 401 DomainError; Echo's HTTPErrorHandler writes it.
func unauth(c *echo.Context, key string) error {
	de := apperr.E(apperr.Unauthenticated, key)
	de.CorrelationID = c.Response().Header().Get(echo.HeaderXRequestID)

	return de
}

// Middleware returns the Echo middleware.
func (a *Authenticator) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ectx *echo.Context) error {
			header := ectx.Request().Header.Get(echo.HeaderAuthorization)
			if header == "" {
				return next(ectx)
			}

			principal, err := a.authenticate(ectx, header)
			if err != nil {
				return err
			}

			ectx.SetRequest(
				ectx.Request().WithContext(ops.WithPrincipal(ectx.Request().Context(), principal)),
			)

			return next(ectx)
		}
	}
}

// Principal implements PrincipalSource with the identity tables (identity-api) and records a new sign-in.
func (a *Authenticator) Principal(
	ctx context.Context,
	subject string,
	tenant, membership uuid.UUID,
	signedIn time.Time,
) (*ops.Principal, error) {
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
func (a *Authenticator) RecordSignIn(
	ctx context.Context,
	principal *ops.Principal,
	signedIn time.Time,
) {
	if principal == nil || signedIn.IsZero() {
		return
	}

	signedIn = signedIn.UTC().Truncate(time.Second)

	last, found := a.signIns.Load(principal.UserID)
	if lastSignIn, isTime := last.(time.Time); found && isTime && !signedIn.After(lastSignIn) {
		return
	}

	err := a.DB.Run(ctx, false, principal, func(txn pgx.Tx) error {
		_, err := txn.Exec(
			ctx,
			`UPDATE identity.users SET last_sign_in_at = $3, sign_in_auth_time = $2
			WHERE id = $1 AND (sign_in_auth_time IS NULL OR sign_in_auth_time < $2)`,
			principal.UserID,
			signedIn,
			a.now(),
		)
		if err != nil {
			return apperr.Wrap(err, "update identity.users")
		}
		// an account HQ, a contractor or a customer owner added is listed as invited until its first sign-in (IR461)
		for _, stmt := range []string{
			`UPDATE identity.partner_users SET status = 'active', version = version + 1, updated_at = $2 WHERE membership_id = $1
				AND status = 'invited'`,
			`UPDATE identity.client_users SET status = 'active', version = version + 1, updated_at = $2 WHERE membership_id = $1
				AND status = 'invited'`,
		} {
			_, err := txn.Exec(ctx, stmt, principal.MembershipID, a.now())
			if err != nil {
				return apperr.Wrap(err, "exec (RecordSignIn)")
			}
		}

		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "auth: sign-in not recorded", "user", principal.UserID, "err", err)

		return
	}

	a.signIns.Store(principal.UserID, signedIn)
}

// Load reads the Membership for subject in tenant; it fails when the membership does not belong to the subject,
// is outside its validity window, or the user is not active.
func (a *Authenticator) Load(
	ctx context.Context,
	subject string,
	tenant, membership uuid.UUID,
) (*ops.Principal, error) {
	principal := &ops.Principal{
		TenantID:          tenant,
		MembershipID:      membership,
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
		IssuerHost:        "",
	}
	now := a.now()

	err := a.DB.Run(ctx, true, principal, func(txn pgx.Tx) error {
		err := loadMembership(ctx, txn, principal, subject, now)
		if err != nil {
			return err
		}

		principal.Permissions, err = loadPermissions(ctx, txn, membership)
		if err != nil {
			return err
		}

		principal.Scopes, err = loadScopes(ctx, txn, membership)

		return err
	})
	if err != nil {
		return nil, apperr.Wrap(err, "ops.Runner.Run")
	}

	return principal, nil
}

// authenticate verifies the bearer token of the Authorization header and resolves the selected Membership; its
// error is the DomainError to answer with.
func (a *Authenticator) authenticate(ectx *echo.Context, header string) (*ops.Principal, error) {
	token, found := strings.CutPrefix(header, "Bearer ")
	if !found || token == "" {
		return nil, unauth(ectx, apperr.KeyUnauthenticated)
	}

	identity, err := a.Verifier.Verify(ectx.Request().Context(), token)
	if err != nil {
		// why, never the token: an expired one, an unknown signing key or another issuer or client
		slog.InfoContext(ectx.Request().Context(), "auth: token refused", "err", err)

		return nil, unauth(ectx, apperr.KeyUnauthenticated)
	}

	tenant, err1 := uuid.Parse(ectx.Request().Header.Get("X-Tenant-Id"))

	membership, err2 := uuid.Parse(ectx.Request().Header.Get("X-Membership-Id"))
	if err1 != nil || err2 != nil {
		return nil, unauth(ectx, "error.membershipRequired")
	}

	principal, err := a.principal(
		ectx.Request().Context(),
		identity.Subject,
		tenant,
		membership,
		identity.SignedIn,
	)
	if errors.Is(err, ErrUnavailable) {
		return nil, apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	}

	if err != nil {
		return nil, unauth(ectx, apperr.KeyMembershipInvalid)
	}

	refusal := refused(ectx, principal, identity)
	if refusal != nil {
		return nil, refusal
	}

	return onAddress(principal, identity.Issuer), nil
}

// onAddress is the principal naming the host of the token's issuer. It is a copy: a principal source may share one
// principal among requests (RemoteSource's cache), and those can have signed in on different addresses.
func onAddress(principal *ops.Principal, issuer string) *ops.Principal {
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Hostname() == "" {
		return principal
	}

	named := *principal
	named.IssuerHost = parsed.Hostname()

	return &named
}

// refused answers the DomainError for a principal this token may not act as, nil when it may.
func refused(ectx *echo.Context, principal *ops.Principal, identity Identity) error {
	if Revoked(
		principal,
		identity.SignedIn,
	) { // HQ signed this user out everywhere after this sign-in (IR384)
		return unauth(ectx, "errors.session_revoked")
	}

	if principal.Role == "admin" && identity.Client != "" &&
		!identity.AdminApp { // HQ works through its own app, signed in with the code (DEC-85, IR510)
		return apperr.E(apperr.Forbidden, "errors.hq_app_required")
	}

	return nil
}

func (a *Authenticator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}

	return time.Now().UTC()
}

func (a *Authenticator) principal(
	ctx context.Context,
	subject string,
	tenant, membership uuid.UUID,
	signedIn time.Time,
) (*ops.Principal, error) {
	if a.Source != nil {
		out, err := a.Source.Principal(ctx, subject, tenant, membership, signedIn)

		return out, apperr.Wrap(err, "auth.PrincipalSource.Principal")
	}

	return a.Principal(ctx, subject, tenant, membership, signedIn)
}

// loadMembership reads the Membership of the subject into the principal; it fails when the user is not active or
// now is outside the membership's validity window.
func loadMembership(
	ctx context.Context,
	txn pgx.Tx,
	principal *ops.Principal,
	subject string,
	now time.Time,
) error {
	var (
		clientRole, employment, timezone *string
		validUntil                       *time.Time
		validFrom                        time.Time
		status                           string
	)

	err := txn.QueryRow(ctx, `SELECT m.user_id, m.organization_id, m.role, m.client_role, m.employment, m.scope_version,
			m.valid_from, m.valid_until, m.sessions_revoked_at, u.status, pf.timezone
			FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id LEFT JOIN identity.preferences pf ON pf.user_id
			= u.id
			WHERE m.id = $1 AND (u.cognito_sub = $2 OR u.id::text = $2)`, principal.MembershipID, subject).
		Scan(&principal.UserID, &principal.OrgID, &principal.Role, &clientRole, &employment, &principal.ScopeVersion,
			&validFrom, &validUntil,
			&principal.SessionsRevokedAt, &status, &timezone)
	if err != nil {
		return apperr.Wrap(err, "select identity.memberships")
	}

	if status != "active" || now.Before(validFrom) ||
		(validUntil != nil && !now.Before(*validUntil)) {
		return apperr.E(
			apperr.NotFound,
			apperr.KeyMembershipInvalid,
		) // inactive user or outside the validity window
	}

	if clientRole != nil {
		principal.ClientRole = *clientRole
	}

	if employment != nil {
		principal.Employment = *employment
	}

	if timezone != nil {
		principal.Timezone = *timezone
	}

	return nil
}

// loadPermissions reads the permissions of the membership.
func loadPermissions(
	ctx context.Context,
	txn pgx.Tx,
	membership uuid.UUID,
) (map[string]bool, error) {
	rows, err := txn.Query(
		ctx,
		`SELECT permission FROM identity.membership_permissions WHERE membership_id = $1`,
		membership,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select identity.membership_permissions")
	}

	permissions := map[string]bool{}

	for rows.Next() {
		var perm string

		err := rows.Scan(&perm)
		if err != nil {
			return nil, apperr.Wrap(err, "select identity.membership_permissions")
		}

		permissions[perm] = true
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select identity.membership_permissions")
	}

	return permissions, nil
}

// loadScopes reads the scopes of the membership by kind.
func loadScopes(
	ctx context.Context,
	txn pgx.Tx,
	membership uuid.UUID,
) (map[string][]uuid.UUID, error) {
	scopes, err := txn.Query(
		ctx,
		`SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1`,
		membership,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select identity.membership_scopes")
	}

	byKind := map[string][]uuid.UUID{}

	for scopes.Next() {
		var (
			kind string
			ref  uuid.UUID
		)

		err := scopes.Scan(&kind, &ref)
		if err != nil {
			return nil, apperr.Wrap(err, "select identity.membership_scopes")
		}

		byKind[kind] = append(byKind[kind], ref)
	}

	return byKind, apperr.Wrap(scopes.Err(), "select identity.membership_scopes")
}
