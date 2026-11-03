package integration_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/idp"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// TestRevokeSessions: HQ signs a user out everywhere (members.revokeSessions, IR384, Figma Admin 03-8). Every
// membership of the user in the tenant records the revocation. A token signed in before it, or one that does not say
// when, is UNAUTHENTICATED errors.session_revoked, in identity and in the other services; a sign-in after it works. The
// access stays. A client membership needs asset.write, any other identity.write; a reason is required; another tenant's
// membership is NOT_FOUND and keeps its sessions.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRevokeSessions(t *testing.T) {
	srv := server(t)
	tenantA, tenantB := seed.ID("tenant-a"), seed.ID("tenant-b")
	user := seed.ID("user-test-revoke")
	tech, client, foreign := seed.ID(
		"test-revoke-tech",
	), seed.ID(
		"test-revoke-client",
	), seed.ID(
		"test-revoke-tenant-b",
	)
	assets := seed.ID(
		"test-revoke-assets",
	) // an HQ membership with asset.write but not identity.write
	from := clock().Add(-24 * time.Hour)

	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name) VALUES ($1, 'revoke.test@example.com', 'Revoke Test') ON
			CONFLICT DO NOTHING`,
		user,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from) VALUES ($1,
			$2,$3,$4,'technician','internal',$5) ON CONFLICT DO NOTHING`,
		tech,
		tenantA,
		user,
		seed.ID("org-operator-a"),
		from,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,
			$2,$3,$4,'client','member',$5) ON CONFLICT DO NOTHING`,
		client,
		tenantA,
		user,
		seed.ID("org-customer-a"),
		from,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from) VALUES ($1,
			$2,$3,$4,'client','member',$5) ON CONFLICT DO NOTHING`,
		foreign,
		tenantB,
		user,
		seed.ID("org-customer-tb"),
		from,
	)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, valid_from) VALUES ($1,$2,$3,$4,
			'admin',$5) ON CONFLICT DO NOTHING`,
		assets,
		tenantA,
		seed.ID("user-hq-override-only"),
		seed.ID("org-operator-a"),
		from,
	)

	for membership, copyOf := range map[string]string{
		"test-revoke-tech":   membershipTechInternalA,
		"test-revoke-client": membershipCustomerA,
	} {
		owner(t, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
			SELECT tenant_id, $1, permission FROM identity.membership_permissions WHERE membership_id = $2 ON CONFLICT DO
			NOTHING`, seed.ID(membership), seed.ID(copyOf))
	}

	owner(
		t,
		`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,'asset.read'), ($1,
			$2,'asset.write') ON CONFLICT DO NOTHING`,
		tenantA,
		assets,
	)
	owner(
		t,
		`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT
			DO NOTHING`,
		tenantA,
		tech,
		seed.ID("unit-online-rto"),
	)
	t.Cleanup(func() {
		for _, m := range []any{tech, client, foreign, assets} {
			owner(t, `DELETE FROM identity.membership_scopes WHERE membership_id = $1`, m)
			owner(t, `DELETE FROM identity.membership_permissions WHERE membership_id = $1`, m)
			owner(t, `DELETE FROM identity.memberships WHERE id = $1`, m)
		}
	})

	signedIn := func(membership string, at time.Time) actor {
		return actor{fmt.Sprintf("tok-rv@%d", at.Unix()), membership}
	}
	earlier := time.Now().Add(-time.Hour)

	old, oldClient := signedIn("test-revoke-tech", earlier), signedIn("test-revoke-client", earlier)
	if code, m := post(srv, &old, "session.get", `{}`); code != 200 {
		t.Fatalf("before the revocation: %d %v", code, m)
	}

	body := func(id any, reason string) string {
		return fmt.Sprintf(`{"membershipId":"%s","reason":%q}`, id, reason)
	}

	// the input and who may
	code, resp := write(srv, hq(), "members.revokeSessions", body(tech, "   "), 0)
	refusal(t, "an empty reason", code, resp, "reason=error.length")
	code, resp = write(
		srv,
		hq(),
		"members.revokeSessions",
		body("00000000-0000-0000-0000-000000000000", "Phone lost"),
		0,
	) // the path parameter, as the REST binding sends it
	refusal(t, "no membership", code, resp, "membershipId=error.required")

	if code, _ := write(
		srv,
		overrider(),
		"members.revokeSessions",
		body(tech, "Phone lost"),
		0,
	); code != 403 {
		t.Errorf("without identity.write or asset.write: %d", code)
	}

	assetsOnly := actor{"tok-oo", "test-revoke-assets"}
	if code, _ := write(
		srv,
		&assetsOnly,
		"members.revokeSessions",
		body(tech, "Phone lost"),
		0,
	); code != 404 {
		t.Errorf("asset.write on a technician membership: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"members.revokeSessions",
		body(foreign, "Phone lost"),
		0,
	); code != 404 {
		t.Errorf("another tenant's membership: %d", code)
	}

	if code, m := post(srv, &old, "session.get", `{}`); code != 200 {
		t.Fatalf("the refused calls ended nothing: %d %v", code, m)
	}

	// asset.write signs out a client membership's user: all of the user's memberships in the tenant
	// one per run: the test database keeps earlier runs' audit rows
	reason := "Phone reported lost " + uuid.NewString()[:8]

	code, resp = write(srv, &assetsOnly, "members.revokeSessions", body(client, reason), 0)
	if code != 200 {
		t.Fatalf("revoke: %d %v", code, resp)
	}

	revokedIDs := asArr(t, data(resp)["membershipIds"])

	ids := make([]string, 0, len(revokedIDs))
	for _, v := range revokedIDs {
		ids = append(ids, asStr(t, v))
	}

	if got := strings.Join(
		ids,
		",",
	); !strings.Contains(got, tech.String()) || !strings.Contains(got, client.String()) ||
		strings.Contains(got, foreign.String()) {
		t.Errorf("revoked memberships %s", got)
	}

	for _, testCase := range []struct {
		who actor
		op  string
	}{
		{old, opSessionGet},
		{oldClient, opSessionGet},
		{actor{"tok-rv", "test-revoke-tech"}, opSessionGet},
		{oldClient, opUnitsList},
	} {
		if code, res := post(
			srv,
			&testCase.who,
			testCase.op,
			`{}`,
		); code != 401 ||
			res["messageKey"] != "errors.session_revoked" {
			t.Errorf("%s as %s signed in before: %d %v", testCase.op, testCase.who.token, code, res)
		}
	}
	// a sign-in in the revocation's own second: auth_time has whole seconds, so it cannot be told from one just before
	revokedAt, err := time.Parse(time.RFC3339Nano, asStr(t, data(resp)["revokedAt"]))
	if err != nil {
		t.Fatal(err)
	}

	sameSecond := signedIn("test-revoke-tech", revokedAt.Truncate(time.Second))
	if code, res := post(
		srv,
		&sameSecond,
		"session.get",
		`{}`,
	); code != 401 ||
		res["messageKey"] != "errors.session_revoked" {
		t.Errorf("a sign-in in the revocation's second: %d %v", code, res)
	}

	later := signedIn("test-revoke-tech", time.Now().Add(time.Minute))
	if code, m := post(srv, &later, "session.get", `{}`); code != 200 {
		t.Errorf("a sign-in after the revocation: %d %v", code, m)
	}

	var revoked *time.Time
	ownerScan(
		t,
		`SELECT sessions_revoked_at FROM identity.memberships WHERE id = $1`,
		[]any{foreign},
		&revoked,
	)

	if revoked != nil {
		t.Error("another tenant's membership was revoked too")
	}

	var audits int
	ownerScan(
		t,
		`SELECT count(*) FROM audit.audit_log WHERE action = 'members.revokeSessions' AND target_id = $1 AND reason = $2`,
		[]any{client.String(), reason},
		&audits,
	)

	if audits != 1 {
		t.Errorf("audit entries: %d", audits)
	}
}

// errProviderDown and errIdentityProviderDown are the failures of an identity provider that does not answer.
var (
	errProviderDown         = errors.New("provider unavailable")
	errIdentityProviderDown = errors.New("identity provider unavailable")
)

// fakeIDP records the subjects whose identity-provider sessions were ended.
type fakeIDP struct {
	mu       sync.Mutex
	subjects []string
	err      error
	// IR461: the accounts created (by e-mail), the set-up e-mails sent and the accounts turned on / off (subject=bool);
	// IR462: the accounts pointed at another membership
	created  []idp.Account
	updated  []idp.Account
	setups   []string
	switched []string
	// failing: creating and updating accounts fail (the provider does not answer); lost: subjects the provider no longer
	// knows (UpdateAccount is idp.ErrNoAccount); generation: a new account's subject is "sub-<email>" plus "#n" from 1 on
	failing    bool
	lost       map[string]bool
	generation int
}

func (f *fakeIDP) CreateAccount(_ context.Context, account idp.Account) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failing {
		return "", errProviderDown
	}

	f.created = append(f.created, account)
	if f.generation > 0 {
		return fmt.Sprintf("sub-%s#%d", account.Email, f.generation), nil
	}

	return "sub-" + account.Email, nil
}

func (f *fakeIDP) UpdateAccount(_ context.Context, subject string, account idp.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.failing {
		return errProviderDown
	}

	if f.lost[subject] {
		return idp.ErrNoAccount
	}

	f.updated = append(f.updated, account)

	return nil
}

func (f *fakeIDP) SendSetup(_ context.Context, subject string, _ idp.Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.setups = append(f.setups, subject)

	return nil
}

func (f *fakeIDP) SetEnabled(_ context.Context, subject string, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.switched = append(f.switched, fmt.Sprintf("%s=%v", subject, enabled))

	return nil
}

func (f *fakeIDP) EndSessions(_ context.Context, subject string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.subjects = append(f.subjects, subject)

	return f.err
}

func (f *fakeIDP) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := f.subjects
	f.subjects = nil

	return out
}

// TestRevokeSessionsEndsIdentityProviderSessions: after members.revokeSessions the identity service ends the user's
// sessions at the identity provider (IR386): the user's ID in the realm locally, the Cognito sub once the user has one.
// The event is applied once. A provider that fails is logged and the event is still done, so it does not hold up
// the identity consumer's other events; the Core API refuses the old sign-ins either way (IR384).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRevokeSessionsEndsIdentityProviderSessions(t *testing.T) {
	idp := &fakeIDP{
		mu:         sync.Mutex{},
		subjects:   nil,
		err:        nil,
		created:    nil,
		updated:    nil,
		setups:     nil,
		switched:   nil,
		failing:    false,
		lost:       nil,
		generation: 0,
	}
	srv := serverCfg(
		t,
		func(c *apiserver.Config) { c.IdentityProvider = idp; c.Domains = ops.Domains },
	)
	user, member := seed.ID("user-test-revoke-idp"), seed.ID("test-revoke-idp-tech")
	owner(
		t,
		`INSERT INTO identity.users (id, email, display_name) VALUES ($1, 'revoke.idp.test@example.com', 'Revoke IdP Test') ON
			CONFLICT DO NOTHING`,
		user,
	)
	owner(t, `UPDATE identity.users SET cognito_sub = NULL WHERE id = $1`, user)
	owner(
		t,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, employment, valid_from) VALUES ($1,
			$2,$3,$4,'technician','internal',$5) ON CONFLICT DO NOTHING`,
		member,
		seed.ID("tenant-a"),
		user,
		seed.ID("org-operator-a"),
		clock().Add(-24*time.Hour),
	)
	t.Cleanup(func() { owner(t, `DELETE FROM identity.memberships WHERE id = $1`, member) })

	revoke := func() {
		t.Helper()

		if code, res := write(
			srv,
			hq(),
			"members.revokeSessions",
			fmt.Sprintf(`{"membershipId":"%s","reason":"Laptop stolen"}`, member),
			0,
		); code != 200 {
			t.Fatalf("revoke: %d %v", code, res)
		}

		err := srv.DrainEvents(context.Background())
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
	}

	revoke()

	if got := idp.take(); !slices.Contains(got, user.String()) {
		t.Errorf("ended at the identity provider: %v, want %s", got, user)
	}

	err := srv.DrainEvents(context.Background())
	if err != nil || len(idp.take()) != 0 {
		t.Errorf("the event was applied again: %v", err)
	}

	// a user with a Cognito sub is ended by it; a provider that does not answer leaves the event done
	sub := "cognito-" + uuid.NewString()
	owner(t, `UPDATE identity.users SET cognito_sub = $2 WHERE id = $1`, user, sub)
	t.Cleanup(
		func() { owner(t, `UPDATE identity.users SET cognito_sub = NULL WHERE id = $1`, user) },
	)

	idp.err = errIdentityProviderDown

	revoke()

	if got := idp.take(); !slices.Contains(got, sub) || slices.Contains(got, user.String()) {
		t.Errorf("ended with the Cognito sub: %v", got)
	}

	idp.err = nil

	err = srv.DrainEvents(context.Background())
	if err != nil || len(idp.take()) != 0 {
		t.Errorf("a failed provider call is retried and holds up the consumer: %v", err)
	}

	// a user who is gone by the time the event is applied has no sessions to end
	owner(
		t,
		`INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id)
		VALUES ($1, 'membership', $2, 'SessionsRevoked', jsonb_build_object('userId', $3::text), 'test-gone-user')`,
		seed.ID("tenant-a"),
		member,
		uuid.New(),
	)

	err = srv.DrainEvents(context.Background())
	if err != nil || len(idp.take()) != 0 {
		t.Errorf("a user who is gone: %v", err)
	}
}
