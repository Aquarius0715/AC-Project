package identity

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
)

// Provider is what identity asks of the identity provider: ending sessions (IR386) and the accounts of the people HQ,
// a contractor or a customer owner adds (IR461). Locally Keycloak's admin API; production Cognito.
type Provider interface {
	SessionEnder
	CreateAccount(ctx context.Context, a idp.Account) (string, error)
	UpdateAccount(ctx context.Context, subject string, a idp.Account) error // idp.ErrNoAccount: the provider lost it
	SendSetup(ctx context.Context, subject string, a idp.Account) error
	SetEnabled(ctx context.Context, subject string, enabled bool) error
}

// AccountApps are the apps a new account's set-up link returns to, by role.
type AccountApps struct{ Customer, Partner, Technician, Admin string }

func (a AccountApps) of(role string) (url, client string) {
	switch role {
	case "client":
		return a.Customer, "ac-web"
	case "contractor":
		return a.Partner, "ac-web"
	case "technician":
		return a.Technician, "ac-web"
	}
	return a.Admin, "ac-admin-web"
}

// Events of IR461, handled by the identity consumer.
const (
	EventAccountRequested = "AccountRequested" // create the provider account if missing, then send the set-up e-mail
	EventAccountEnabled   = "AccountEnabled"   // turn the provider account on or off
)

// TechnicianAccessMax is the longest access a contractor gives a technician it adds (IR461, the user's option 1).
const TechnicianAccessMax = 366 * 24 * time.Hour

// defaultPermissions and the scopes of an account a contractor or a customer owner adds: the sets the demo accounts of
// each role hold. An external technician has no unit scope: the contractor assigns them only on its accepted offers.
var defaultPermissions = map[string][]string{
	"client":     {"control.execute"},
	"contractor": {"partner.accept", "partner.assign", "partner.review"},
	"technician": {"alert.read", "alert.resolve", "control.diagnose", "device.maintain"},
}

// newAccount is a person with one membership to create.
type newAccount struct {
	Email, DisplayName, Role string
	OrgID                    uuid.UUID
	Employment, ClientRole   *string
	ValidUntil               *time.Time
	Permissions              []string
	Scopes                   []ScopeRef
}

// AccountsPerOrgPerDay is how many accounts a contractor or a customer owner adds to their organization in 24 hours
// (IR464): each one mails a set-up link to the address typed in, so a member cannot have the sign-in service mail
// strangers in bulk. HQ is not limited. ACCOUNTS_PER_ORG_PER_DAY sets another value at start-up (server Config).
var AccountsPerOrgPerDay = 30

// addCap refuses an account beyond AccountsPerOrgPerDay with the seconds until the oldest of them is a day old. The
// organization row is locked so two adds at once count each other. Memberships dated after now (the seed's, written
// on the wall clock ahead of the demo clock) do not count.
func addCap(ctx context.Context, c *ops.Call, org uuid.UUID) error {
	if c.Principal.Role == "admin" {
		return nil
	}
	if _, err := c.Tx.Exec(ctx, `SELECT 1 FROM identity.organizations WHERE id = $1 FOR UPDATE`, org); err != nil {
		return err
	}
	var n int
	var oldest *time.Time
	if err := c.Tx.QueryRow(ctx, `SELECT count(*), min(created_at) FROM (SELECT created_at FROM identity.memberships
		WHERE organization_id = $1 AND created_at > $2 AND created_at <= $3 ORDER BY created_at DESC LIMIT $4) x`,
		org, c.Now.Add(-24*time.Hour), c.Now, AccountsPerOrgPerDay).Scan(&n, &oldest); err != nil {
		return err
	}
	if n < AccountsPerOrgPerDay || oldest == nil {
		return nil
	}
	wait := oldest.Add(24 * time.Hour).Sub(c.Now)
	return apperr.E(apperr.RateLimited, "errors.too_many_accounts").RetryAfter(int((wait + time.Second - 1) / time.Second))
}

// errEmailTaken: the address is a person's whom this tenant may not add (personFor).
var errEmailTaken = errors.New("identity: e-mail taken")

// personFor is the person to add for the e-mail (IR461, IR462): a new one, or one this tenant took away before — their
// sign-in account names one of its memberships and none of their memberships here is open any more. That person keeps
// their ID and sign-in account, takes the name given and has not signed in since. Anyone else holding the address is
// errEmailTaken: an active user here, or someone whose sign-in belongs to another tenant (memberships are
// tenant-isolated, so the claims' membership is visible only when it is this tenant's).
func personFor(ctx context.Context, c *ops.Call, email, name string) (uuid.UUID, error) {
	email, name = strings.TrimSpace(email), strings.TrimSpace(name)
	var user uuid.UUID
	err := c.Tx.QueryRow(ctx, `INSERT INTO identity.users (email, display_name, status, created_at, updated_at) VALUES ($1, $2, 'active', $3, $3)
		ON CONFLICT (email) DO NOTHING RETURNING id`, email, name, c.Now).Scan(&user)
	if !errors.Is(err, pgx.ErrNoRows) {
		return user, err
	}
	err = c.Tx.QueryRow(ctx, `SELECT u.id FROM identity.users u
		WHERE u.email = $1 AND u.status = 'active'
			AND EXISTS (SELECT 1 FROM identity.memberships m WHERE m.id = u.sign_in_membership_id)
			AND NOT EXISTS (SELECT 1 FROM identity.memberships m WHERE m.user_id = u.id AND (m.valid_until IS NULL OR m.valid_until > $2))
		FOR UPDATE OF u`, email, c.Now).Scan(&user)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errEmailTaken
	}
	if err != nil {
		return uuid.Nil, err
	}
	_, err = c.Tx.Exec(ctx, `UPDATE identity.users SET display_name = $2, last_sign_in_at = NULL, version = version + 1, updated_at = $3 WHERE id = $1`, user, name, c.Now)
	return user, err
}

// createAccount creates the person (or takes back one removed before, personFor), the membership with its permissions
// and scopes, and asks the identity consumer to open or re-point the provider account and e-mail the set-up link
// (IR461, IR462). An e-mail another person holds is a field error.
func createAccount(ctx context.Context, c *ops.Call, a newAccount) (user, membership uuid.UUID, err error) {
	if err = addCap(ctx, c, a.OrgID); err != nil {
		return user, membership, err
	}
	user, err = personFor(ctx, c, a.Email, a.DisplayName)
	if errors.Is(err, errEmailTaken) {
		return user, membership, apperr.Fields(map[string]string{"email": "errors.email_taken"})
	}
	if err != nil {
		return user, membership, err
	}
	if c.Principal.Role != "admin" { // a person whose access HQ ended comes back only through HQ (IR508), not as a new account
		var revoked bool
		if err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE user_id = $1 AND revoked_at IS NOT NULL)`, user).Scan(&revoked); err != nil {
			return user, membership, err
		}
		if revoked {
			return user, membership, apperr.Fields(map[string]string{"email": "errors.revoked_by_hq"})
		}
	}
	if err = c.Tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, employment, client_role, valid_from, valid_until, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $6, $6) RETURNING id`,
		user, a.OrgID, a.Role, a.Employment, a.ClientRole, c.Now, a.ValidUntil).Scan(&membership); err != nil {
		return user, membership, err
	}
	for _, p := range a.Permissions {
		if _, err = c.Tx.Exec(ctx, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)`, membership, p); err != nil {
			return user, membership, err
		}
	}
	for _, sc := range a.Scopes {
		if _, err = c.Tx.Exec(ctx, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3) ON CONFLICT DO NOTHING`,
			membership, sc.Kind, sc.ID); err != nil {
			return user, membership, err
		}
	}
	if a.Role == "client" { // IR84 initial Consent
		if _, err = c.Tx.Exec(ctx, `INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted) VALUES (current_setting('app.tenant_id')::uuid, $1, 'location_automation', false)`, membership); err != nil {
			return user, membership, err
		}
	}
	c.Emit(ops.Event{AggregateType: "membership", AggregateID: membership, Type: "MembershipSaved", Payload: map[string]any{"role": a.Role}})
	requestAccount(c, user, membership, a.Role)
	return user, membership, nil
}

// requestAccount asks the identity consumer to open the provider account (if missing) or point it at this membership,
// and send the set-up e-mail.
func requestAccount(c *ops.Call, user, membership uuid.UUID, role string) {
	c.Emit(ops.Event{AggregateType: "membership", AggregateID: membership, Type: EventAccountRequested, Payload: map[string]any{"userId": user, "membershipId": membership, "role": role}})
}

// setAccountEnabled asks the identity consumer to turn the provider account of the membership's person on or off; it
// acts only when the account signs in with this membership (IR462).
func setAccountEnabled(c *ops.Call, membership uuid.UUID, enabled bool) {
	c.Emit(ops.Event{AggregateType: "membership", AggregateID: membership, Type: EventAccountEnabled, Payload: map[string]any{"membershipId": membership, "enabled": enabled}})
}

// AccountHandlers open, e-mail and turn off the provider accounts (IR461). Like the session ending (IR386) they are
// best effort: a provider that does not answer is logged and the event is done, so the identity consumer's other
// events are not held up; the person's set-up e-mail is sent again with resendInvite.
func AccountHandlers(p Provider, apps AccountApps) map[string]events.Handler {
	if p == nil {
		return nil
	}
	return map[string]events.Handler{
		EventAccountRequested: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var in struct {
				UserID       uuid.UUID `json:"userId"`
				MembershipID uuid.UUID `json:"membershipId"`
				Role         string    `json:"role"`
			}
			if err := e.Decode(&in); err != nil {
				return err
			}
			var email, name string
			var sub *string
			err := tx.QueryRow(ctx, `SELECT email, display_name, cognito_sub FROM identity.users WHERE id = $1`, in.UserID).Scan(&email, &name, &sub)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			url, client := apps.of(in.Role)
			acct := idp.Account{Email: email, DisplayName: name, TenantID: e.TenantID.String(), Membership: in.MembershipID.String(), Role: in.Role, ClientID: client, RedirectURI: url + "/login"}
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			subject := ""
			if sub != nil { // a person added again (IR462), or the set-up e-mail once more: the claims name this membership
				subject = *sub
				err = p.UpdateAccount(cctx, subject, acct)
				if errors.Is(err, idp.ErrNoAccount) {
					subject, err = "", nil
				}
				if err != nil {
					slog.WarnContext(ctx, "identity: the provider account was not updated", "user", in.UserID, "err", err)
					return nil
				}
			}
			if subject == "" {
				if subject, err = p.CreateAccount(cctx, acct); err != nil {
					slog.WarnContext(ctx, "identity: the provider account was not created", "user", in.UserID, "err", err)
					return nil
				}
			}
			if _, err := tx.Exec(ctx, `UPDATE identity.users SET cognito_sub = $2, sign_in_membership_id = $3 WHERE id = $1`, in.UserID, subject, in.MembershipID); err != nil {
				return err
			}
			if err := p.SendSetup(cctx, subject, acct); err != nil {
				slog.WarnContext(ctx, "identity: the set-up e-mail was not sent", "user", in.UserID, "err", err)
			}
			return nil
		},
		EventAccountEnabled: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var in struct {
				MembershipID uuid.UUID `json:"membershipId"`
				Enabled      bool      `json:"enabled"`
			}
			if err := e.Decode(&in); err != nil {
				return err
			}
			// the subject is the provider's, or the person's ID for the realm's demo accounts (auth accepts either); a
			// membership the account does not sign in with leaves it alone (IR462)
			var sub string
			err := tx.QueryRow(ctx, `SELECT coalesce(u.cognito_sub, u.id::text) FROM identity.memberships m JOIN identity.users u ON u.id = m.user_id
				WHERE m.id = $1 AND (u.sign_in_membership_id IS NULL OR u.sign_in_membership_id = m.id)`, in.MembershipID).Scan(&sub)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err != nil {
				return err
			}
			cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			if err := p.SetEnabled(cctx, sub, in.Enabled); err != nil {
				slog.WarnContext(ctx, "identity: the provider account was not turned on or off", "membership", in.MembershipID, "err", err)
			}
			return nil
		},
	}
}
