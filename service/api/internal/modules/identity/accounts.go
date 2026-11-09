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
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
)

// Provider is what identity asks of the identity provider: ending sessions (IR386) and the accounts of the people HQ,
// a contractor or a customer owner adds (IR461). Locally Keycloak's admin API; production Cognito.
type Provider interface {
	SessionEnder
	CreateAccount(ctx context.Context, a idp.Account) (string, error)
	UpdateAccount(
		ctx context.Context,
		subject string,
		a idp.Account,
	) error // idp.ErrNoAccount: the provider lost it
	SendSetup(ctx context.Context, subject string, a idp.Account) error
	SetEnabled(ctx context.Context, subject string, enabled bool) error
	DeleteAccount(
		ctx context.Context,
		subject string,
	) error // an account HQ deleted; already gone is no error
}

// AccountApps are the apps a new account's set-up link returns to, by role.
type AccountApps struct{ Customer, Partner, Technician, Admin string }

// of is the app a role's set-up link returns to: its URL and its client ID.
func (a AccountApps) of(role string) (string, string) {
	switch role {
	case authz.RoleClient:
		return a.Customer, appClientID
	case authz.RoleContractor:
		return a.Partner, appClientID
	case authz.RoleTechnician:
		return a.Technician, appClientID
	}

	return a.Admin, "ac-admin-web"
}

// Events of IR461, handled by the identity-accounts consumer (IR511).
const (
	EventAccountRequested = "AccountRequested" // create the provider account if missing, then send the set-up e-mail
	EventAccountEnabled   = "AccountEnabled"   // turn the provider account on or off
	EventAccountDeleted   = "AccountDeleted"   // delete the provider account of a person HQ deleted (DEC-103)
)

// TechnicianAccessMax is the longest access a contractor gives a technician it adds (IR461, the user's option 1).
const TechnicianAccessMax = 366 * 24 * time.Hour

// DefaultPermissions are the permissions an account of the role gets when a contractor or a customer owner adds it: the
// sets the demo accounts of each role hold. The development seed gives the live site's owner a client's
// (seed.ApplyLive). An external technician also has no unit scope: the contractor assigns them only on its accepted
// offers.
func DefaultPermissions(role string) []string {
	switch role {
	case authz.RoleClient:
		return []string{"control.execute"}
	case authz.RoleContractor:
		return []string{"partner.accept", "partner.assign", "partner.review"}
	case authz.RoleTechnician:
		return []string{"alert.read", "alert.resolve", "control.diagnose", "device.maintain"}
	}

	return nil
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

// DefaultAccountsPerOrgPerDay is how many accounts a contractor or a customer owner adds to their organization in 24
// hours (IR464): each one mails a set-up link to the address typed in, so a member cannot have the sign-in service
// mail strangers in bulk. HQ is not limited. ACCOUNTS_PER_ORG_PER_DAY sets another number (Accounts.PerOrgPerDay).
const DefaultAccountsPerOrgPerDay = 30

// capWindow is the period the accounts an organization adds are counted over.
const capWindow = 24 * time.Hour

// Accounts are the account rules a server runs with.
type Accounts struct {
	// PerOrgPerDay is how many accounts a contractor or a customer owner adds to their organization in 24 hours; 0
	// keeps DefaultAccountsPerOrgPerDay
	PerOrgPerDay int
}

// perOrgPerDay is the number of accounts an organization adds in 24 hours.
func (a Accounts) perOrgPerDay() int {
	if a.PerOrgPerDay > 0 {
		return a.PerOrgPerDay
	}

	return DefaultAccountsPerOrgPerDay
}

// addCap refuses an account beyond limit with the seconds until the oldest of them is a day old. The
// organization row is locked so two adds at once count each other. Memberships dated after now (the seed's, written
// on the wall clock ahead of the demo clock) do not count.
func addCap(ctx context.Context, call *ops.Call, org uuid.UUID, limit int) error {
	if call.Principal.Role == authz.RoleAdmin {
		return nil
	}

	_, err := call.Tx.Exec(
		ctx,
		`SELECT 1 FROM identity.organizations WHERE id = $1 FOR UPDATE`,
		org,
	)
	if err != nil {
		return apperr.Wrap(err, "select identity.organizations")
	}

	var (
		count  int
		oldest *time.Time
	)

	err = call.Tx.QueryRow(ctx, `SELECT count(*), min(created_at) FROM (SELECT created_at FROM identity.memberships
		WHERE organization_id = $1 AND created_at > $2 AND created_at <= $3 ORDER BY created_at DESC LIMIT $4) x`,
		org, call.Now.Add(-capWindow), call.Now, limit).
		Scan(&count, &oldest)
	if err != nil {
		return apperr.Wrap(err, "select identity.memberships")
	}

	if count < limit || oldest == nil {
		return nil
	}

	wait := oldest.Add(capWindow).Sub(call.Now)

	return apperr.E(apperr.RateLimited, "errors.too_many_accounts").
		RetryAfter(int((wait + time.Second - 1) / time.Second))
}

// errEmailTaken: the address is a person's whom this tenant may not add (personFor).
var errEmailTaken = errors.New("identity: e-mail taken")

// personFor is the person to add for the e-mail (IR461, IR462): a new one, or one this tenant took away before — their
// sign-in account names one of its memberships and none of their memberships here is open any more. That person keeps
// their ID and sign-in account, takes the name given and has not signed in since. Anyone else holding the address is
// errEmailTaken: an active user here, or someone whose sign-in belongs to another tenant (memberships are
// tenant-isolated, so the claims' membership is visible only when it is this tenant's).
func personFor(ctx context.Context, call *ops.Call, email, name string) (uuid.UUID, error) {
	email, name = strings.TrimSpace(email), strings.TrimSpace(name)

	var user uuid.UUID

	err := call.Tx.QueryRow(ctx, `INSERT INTO identity.users (email, display_name, status, created_at, updated_at) VALUES
		($1,
		$2, 'active', $3, $3)
		ON CONFLICT (email) DO NOTHING RETURNING id`, email, name, call.Now).
		Scan(&user)
	if !errors.Is(err, pgx.ErrNoRows) {
		return user, apperr.Wrap(err, "insert identity.users")
	}

	err = call.Tx.QueryRow(ctx, `SELECT u.id FROM identity.users u
		WHERE u.email = $1 AND u.status = 'active'
			AND EXISTS (SELECT 1 FROM identity.memberships m WHERE m.id = u.sign_in_membership_id)
			AND NOT EXISTS (SELECT 1 FROM identity.memberships m WHERE m.user_id = u.id AND (m.valid_until IS NULL OR
			m.valid_until > $2))
		FOR UPDATE OF u`, email, call.Now).Scan(&user)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, errEmailTaken
	}

	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "select identity.users")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE identity.users SET display_name = $2, last_sign_in_at = NULL, version = version + 1, updated_at = $3 WHERE id
			= $1`,
		user,
		name,
		call.Now,
	)

	return user, apperr.Wrap(err, "update identity.users")
}

// createAccount creates the person (or takes back one removed before, personFor), the membership with its permissions
// and scopes, and asks the identity-accounts consumer to open or re-point the provider account and e-mail the set-up
// link (IR461, IR462, IR511). An e-mail another person holds is a field error. It returns the membership; limit is how
// many accounts the organization adds in 24 hours.
func createAccount(
	ctx context.Context,
	call *ops.Call,
	account newAccount,
	limit int,
) (uuid.UUID, error) {
	var membership uuid.UUID

	err := addCap(ctx, call, account.OrgID, limit)
	if err != nil {
		return membership, err
	}

	user, err := personFor(ctx, call, account.Email, account.DisplayName)
	if errors.Is(err, errEmailTaken) {
		return membership, apperr.Fields(map[string]string{keyEmail: keyEmailTaken})
	}

	if err != nil {
		return membership, err
	}

	if call.Principal.Role != authz.RoleAdmin {
		// a person whose access HQ ended comes back only through HQ (IR508), not as a new account
		var revoked bool

		err = call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE user_id = $1 AND revoked_at IS
			NOT
			NULL)`, user).
			Scan(&revoked)
		if err != nil {
			return membership, apperr.Wrap(err, "select identity.memberships")
		}

		if revoked {
			return membership, apperr.Fields(
				map[string]string{keyEmail: "errors.revoked_by_hq"},
			)
		}
	}

	err = call.Tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, employment,
		client_role, valid_from, valid_until, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $6, $6) RETURNING id`,
		user, account.OrgID, account.Role, account.Employment, account.ClientRole, call.Now, account.ValidUntil).
		Scan(&membership)
	if err != nil {
		return membership, apperr.Wrap(err, "insert identity.memberships")
	}

	for _, permission := range account.Permissions {
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			membership,
			permission,
		)
		if err != nil {
			return membership, apperr.Wrap(err, "insert identity.membership_permissions")
		}
	}

	for _, scopeRef := range account.Scopes {
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2, $3) ON CONFLICT DO NOTHING`,
			membership,
			scopeRef.Kind,
			scopeRef.ID,
		)
		if err != nil {
			return membership, apperr.Wrap(err, "insert identity.membership_scopes")
		}
	}

	if account.Role == authz.RoleClient { // IR84 initial Consent
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted) VALUES
				(current_setting('app.tenant_id')::uuid, $1, 'location_automation', false)`,
			membership,
		)
		if err != nil {
			return membership, apperr.Wrap(err, "insert identity.consents")
		}
	}

	call.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   membership,
			Type:          "MembershipSaved",
			Payload:       map[string]any{keyRole: account.Role},
		},
	)
	requestAccount(call, user, membership, account.Role)

	return membership, nil
}

// requestAccount asks the identity-accounts consumer to open the provider account (if missing) or point it at this
// membership, and send the set-up e-mail.
func requestAccount(c *ops.Call, user, membership uuid.UUID, role string) {
	payload := map[string]any{
		"userId":        user,
		keyMembershipID: membership,
		keyRole:         role,
	}
	// the set-up e-mail's link opens where the sender is (IR697); without it the provider's own address (a bootstrap)
	if c.Principal != nil && c.Principal.IssuerHost != "" {
		payload["publicHost"] = c.Principal.IssuerHost
	}

	c.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   membership,
			Type:          EventAccountRequested,
			Payload:       payload,
		},
	)
}

// setAccountEnabled asks the identity-accounts consumer to turn the provider account of the membership's person on or
// off; it acts only when the account signs in with this membership (IR462).
func setAccountEnabled(c *ops.Call, membership uuid.UUID, enabled bool) {
	c.Emit(
		ops.Event{
			AggregateType: kindMembership,
			AggregateID:   membership,
			Type:          EventAccountEnabled,
			Payload:       map[string]any{keyMembershipID: membership, "enabled": enabled},
		},
	)
}

// AccountHandlers open, e-mail and turn off the provider accounts (IR461). Like the session ending (IR386) they are
// best effort: a provider that does not answer is logged and the event is done, so the identity-accounts consumer's
// later events are not held up (IR511); the person's set-up e-mail is sent again with resendInvite.
func AccountHandlers(provider Provider, apps AccountApps) map[string]events.Handler {
	if provider == nil {
		return nil
	}

	consumer := accountConsumer{provider: provider, apps: apps}

	return map[string]events.Handler{
		EventAccountRequested: consumer.requested,
		EventAccountEnabled:   consumer.enabled,
		EventAccountDeleted:   consumer.deleted,
	}
}

// providerTimeout bounds one call to the identity provider.
const providerTimeout = 10 * time.Second

// accountConsumer applies the account events at the identity provider, returning to the apps of each role.
type accountConsumer struct {
	provider Provider
	apps     AccountApps
}

// requested opens the person's provider account, or updates it for a person added again (IR462) or a set-up e-mail
// sent once more, points the sign-in at the membership, and sends the set-up e-mail (IR461). A provider that fails is
// logged and the event is done: HQ can send the set-up e-mail again.
func (consumer accountConsumer) requested(
	ctx context.Context,
	txn pgx.Tx,
	event events.Event,
) error {
	var input struct {
		UserID       uuid.UUID `json:"userId"`
		MembershipID uuid.UUID `json:"membershipId"`
		Role         string    `json:"role"`
		PublicHost   string    `json:"publicHost"`
	}

	decodeErr := event.Decode(&input)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	var (
		email, name string
		sub         *string
	)

	err := txn.QueryRow(ctx, `SELECT email, display_name, cognito_sub FROM identity.users WHERE id = $1`, input.UserID).
		Scan(&email, &name, &sub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return apperr.Wrap(err, "select identity.users")
	}

	url, client := consumer.apps.of(input.Role)
	acct := idp.Account{
		Email:       email,
		DisplayName: name,
		TenantID:    event.TenantID.String(),
		Membership:  input.MembershipID.String(),
		Role:        input.Role,
		ClientID:    client,
		RedirectURI: url + "/login",
		PublicHost:  input.PublicHost,
	}

	cctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	subject, err := consumer.openAccount(cctx, sub, acct)
	if err != nil {
		slog.WarnContext(
			ctx,
			"identity: the provider account was not opened",
			"user",
			input.UserID,
			"err",
			err,
		)

		return nil
	}

	_, execErr := txn.Exec(
		ctx,
		`UPDATE identity.users SET cognito_sub = $2, sign_in_membership_id = $3 WHERE id = $1`,
		input.UserID,
		subject,
		input.MembershipID,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update identity.users")
	}

	sendSetupErr := consumer.provider.SendSetup(cctx, subject, acct)
	if sendSetupErr != nil {
		slog.WarnContext(
			ctx,
			"identity: the set-up e-mail was not sent",
			"user",
			input.UserID,
			"err",
			sendSetupErr,
		)
	}

	return nil
}

// openAccount returns the subject of the person's provider account: the account they have (sub), updated for a person
// added again (IR462) or a set-up e-mail sent once more, so the claims name this membership; or a new account when
// they have none.
func (consumer accountConsumer) openAccount(
	ctx context.Context,
	sub *string,
	acct idp.Account,
) (string, error) {
	if sub != nil {
		err := consumer.provider.UpdateAccount(ctx, *sub, acct)
		if !errors.Is(err, idp.ErrNoAccount) {
			return *sub, apperr.Wrap(err, "update the provider account")
		}
	}

	subject, err := consumer.provider.CreateAccount(ctx, acct)

	return subject, apperr.Wrap(err, "create the provider account")
}

// enabled turns the provider account on or off with the membership (IR461, IR508).
func (consumer accountConsumer) enabled(ctx context.Context, txn pgx.Tx, event events.Event) error {
	var input struct {
		MembershipID uuid.UUID `json:"membershipId"`
		Enabled      bool      `json:"enabled"`
	}

	decodeErr := event.Decode(&input)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}
	// the subject is the provider's, or the person's ID for the realm's demo accounts (auth accepts either); a
	// membership the account does not sign in with leaves it alone (IR462)
	var sub string

	err := txn.QueryRow(ctx, `SELECT coalesce(u.cognito_sub, u.id::text) FROM identity.memberships m JOIN identity.users
		u
		ON u.id = m.user_id
		WHERE m.id = $1 AND (u.sign_in_membership_id IS NULL OR u.sign_in_membership_id = m.id)`, input.MembershipID).
		Scan(&sub)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return apperr.Wrap(err, "select identity.memberships")
	}

	cctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	setEnabledErr := consumer.provider.SetEnabled(cctx, sub, input.Enabled)
	if setEnabledErr != nil {
		slog.WarnContext(
			ctx,
			"identity: the provider account was not turned on or off",
			kindMembership,
			input.MembershipID,
			"err",
			setEnabledErr,
		)
	}

	return nil
}

// deleted deletes the provider account of a person HQ deleted. Like the other account events it is best effort: a
// provider that does not answer is logged and the event is done (the account has no membership, so it cannot sign in).
func (consumer accountConsumer) deleted(ctx context.Context, _ pgx.Tx, event events.Event) error {
	var input struct {
		Subject string `json:"subject"`
	}

	decodeErr := event.Decode(&input)
	if decodeErr != nil {
		return apperr.Wrap(decodeErr, "events.Event.Decode")
	}

	if input.Subject == "" {
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()

	deleteErr := consumer.provider.DeleteAccount(cctx, input.Subject)
	if deleteErr != nil {
		slog.WarnContext(
			ctx,
			"identity: the provider account was not deleted",
			"subject",
			input.Subject,
			"err",
			deleteErr,
		)
	}

	return nil
}
