package auth_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
)

// fakeIdentity is identity-api's principal endpoint knowing one membership of subject sub-1: it counts the calls,
// answers status (the principal for 200) and keeps the signedIn parameter of the last call.
type fakeIdentity struct {
	tenant, member, user uuid.UUID
	calls                int
	status               int
	signedIn             string
}

func (f *fakeIdentity) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	f.calls++

	f.signedIn = req.URL.Query().Get("signedIn")
	if req.URL.Path != auth.PrincipalPath || req.Header.Get("Authorization") != "Bearer internal" ||
		req.URL.Query().Get("subject") != "sub-1" {
		http.Error(writer, "bad", http.StatusBadRequest)

		return
	}

	if req.URL.Query().Get("membership") != f.member.String() {
		writer.WriteHeader(http.StatusNotFound)

		return
	}

	if f.status != http.StatusOK {
		writer.WriteHeader(f.status)

		return
	}

	err := json.NewEncoder(writer).Encode(auth.PrincipalDTO{
		UserID:       f.user,
		TenantID:     f.tenant,
		MembershipID: f.member,
		Role:         roleClient,
		ClientRole:   clientOwner,
		Permissions: []string{
			"control.execute",
		},
		Scopes:            map[string][]uuid.UUID{"unit": {f.user}},
		ScopeVersion:      3,
		OrgID:             uuid.UUID{},
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	})
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
	}
}

// newFakeIdentity starts the fake identity-api, answering 200.
func newFakeIdentity(t *testing.T) (*fakeIdentity, *httptest.Server) {
	t.Helper()

	identity := &fakeIdentity{
		tenant:   uuid.New(),
		member:   uuid.New(),
		user:     uuid.New(),
		calls:    0,
		status:   http.StatusOK,
		signedIn: "",
	}
	srv := httptest.NewServer(identity)
	t.Cleanup(srv.Close)

	return identity, srv
}

// checkRemotePrincipal fails the test unless the principal is the one the fake identity-api answers.
func checkRemotePrincipal(t *testing.T, principal *ops.Principal, err error) {
	t.Helper()

	if err != nil || principal.Role != roleClient || !principal.Permissions["control.execute"] ||
		principal.ScopeVersion != 3 ||
		len(principal.Scopes["unit"]) != 1 ||
		principal.ClientRole != clientOwner {
		t.Fatalf("principal %+v %v", principal, err)
	}
}

func TestRemoteSource(t *testing.T) {
	t.Parallel()

	identity, srv := newFakeIdentity(t)
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	remoteSource := &auth.RemoteSource{
		BaseURL: srv.URL,
		Token:   "internal",
		TTL:     30 * time.Second,
		Now:     func() time.Time { return now }, Client: nil,
	}
	ctx := t.Context()
	none := time.Time{}

	principal, err := remoteSource.Principal(ctx, "sub-1", identity.tenant, identity.member, none)
	checkRemotePrincipal(t, principal, err)

	if identity.signedIn != "" {
		t.Fatalf("no sign-in time is sent without one: %q", identity.signedIn)
	}

	_, principalErr := remoteSource.Principal(ctx, "sub-1", identity.tenant, identity.member, none)
	if principalErr != nil || identity.calls != 1 {
		t.Fatalf("cached call: calls=%d err=%v", identity.calls, principalErr)
	}

	now = now.Add(31 * time.Second)

	when := time.Date(2026, 9, 14, 0, 55, 0, 0, time.UTC)

	_, principalErr = remoteSource.Principal(
		ctx,
		"sub-1",
		identity.tenant,
		identity.member,
		when,
	)
	if principalErr != nil || identity.calls != 2 ||
		identity.signedIn != "2026-09-14T00:55:00Z" {
		t.Fatalf(
			"expired cache: calls=%d signedIn=%q",
			identity.calls,
			identity.signedIn,
		) // the fetch passes the token's sign-in time on (IR268)
	}
}

func TestRemoteSourceFailures(t *testing.T) {
	t.Parallel()

	identity, srv := newFakeIdentity(t)
	now := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)
	remoteSource := &auth.RemoteSource{
		BaseURL: srv.URL,
		Token:   "internal",
		TTL:     30 * time.Second,
		Now:     func() time.Time { return now }, Client: nil,
	}
	ctx := t.Context()
	none := time.Time{}

	_, principalErr := remoteSource.Principal(
		ctx,
		"sub-1",
		identity.tenant,
		uuid.New(),
		none,
	)
	if !errors.Is(
		principalErr,
		auth.ErrNoMembership,
	) {
		t.Fatalf("unknown membership: %v", principalErr)
	}

	identity.status = http.StatusServiceUnavailable
	now = now.Add(time.Minute)

	_, principalErr = remoteSource.Principal(ctx, "sub-1", identity.tenant, identity.member, none)
	if !errors.Is(principalErr, auth.ErrUnavailable) {
		t.Fatalf("outage: %v", principalErr)
	}

	identity.status = http.StatusOK

	_, principalErr = remoteSource.Principal(ctx, "sub-1", identity.tenant, identity.member, none)
	if principalErr != nil {
		t.Fatalf("outage must not be cached: %v", principalErr)
	}

	srv.Close()

	now = now.Add(time.Minute)

	_, principalErr = remoteSource.Principal(ctx, "sub-1", identity.tenant, identity.member, none)
	if !errors.Is(principalErr, auth.ErrUnavailable) {
		t.Fatalf("down: %v", principalErr)
	}
}
