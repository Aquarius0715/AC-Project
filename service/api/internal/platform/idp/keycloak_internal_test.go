package idp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The service account of the fake realm.
const (
	testClientID = "svc"
	testSecret   = "test-secret"
)

// fakeRealm is the realm's token endpoint and its users' logout: it counts the tokens it issues and keeps the logout
// paths it is asked for, and answers a logout with the status the test sets.
type fakeRealm struct {
	tokens atomic.Int32
	mu     sync.Mutex
	paths  []string
	status int
}

func (f *fakeRealm) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	switch {
	case req.URL.Path == "/realms/ac/protocol/openid-connect/token":
		f.issue(writer, req)
	case strings.HasPrefix(req.URL.EscapedPath(), "/admin/realms/ac/users/"):
		f.logout(writer, req)
	default:
		writer.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeRealm) setStatus(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.status = status
}

func (f *fakeRealm) loggedOut() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return strings.Join(f.paths, " ")
}

// issue answers the client-credentials grant of the service account only.
func (f *fakeRealm) issue(writer http.ResponseWriter, req *http.Request) {
	_ = req.ParseForm()
	if req.Form.Get("grant_type") != "client_credentials" ||
		req.Form.Get("client_id") != testClientID ||
		req.Form.Get("client_secret") != testSecret {
		writer.WriteHeader(http.StatusUnauthorized)

		return
	}

	f.tokens.Add(1)

	_, _ = writer.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
}

// logout takes a POST with the issued token and answers with the set status.
func (f *fakeRealm) logout(writer http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Authorization") != "Bearer admin-token" || req.Method != http.MethodPost {
		writer.WriteHeader(http.StatusForbidden)

		return
	}

	f.mu.Lock()
	f.paths = append(f.paths, req.URL.EscapedPath())
	status := f.status
	f.mu.Unlock()

	writer.WriteHeader(status)
}

// newTestKeycloak is a client of the fake realm at base with the given secret, clock and HTTP client.
func newTestKeycloak(base, secret string, now func() time.Time, client *http.Client) *Keycloak {
	return &Keycloak{
		TokenURL:     base + "/realms/ac/protocol/openid-connect/token",
		AdminURL:     base + "/admin/realms/ac",
		ClientID:     testClientID,
		ClientSecret: secret,
		Now:          now,
		Client:       client,
		mu:           sync.Mutex{},
		token:        "",
		until:        time.Time{},
	}
}

// endOK ends the subject's sessions and fails the test when that fails.
func endOK(ctx context.Context, t *testing.T, keycloak *Keycloak, subject, what string) {
	t.Helper()

	err := keycloak.EndSessions(ctx, subject)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
}

// wantTokens fails the test when the realm has not issued want tokens so far.
func wantTokens(t *testing.T, realm *fakeRealm, want int32, what string) {
	t.Helper()

	if got := realm.tokens.Load(); got != want {
		t.Errorf("%s: %d token requests, want %d", what, got, want)
	}
}

// TestKeycloakEndSessions: the service account's token is fetched once and reused, the logout goes to the user's own
// admin path (an ID cannot leave it), an unknown user has nothing to end, and a refused token is fetched again (IR386).
func TestKeycloakEndSessions(t *testing.T) {
	t.Parallel()

	realm := &fakeRealm{
		tokens: atomic.Int32{},
		mu:     sync.Mutex{},
		paths:  nil,
		status: http.StatusNoContent,
	}

	srv := httptest.NewServer(realm)
	defer srv.Close()

	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	keycloak := newTestKeycloak(srv.URL, testSecret, func() time.Time { return now }, nil)
	ctx := t.Context()

	endOK(ctx, t, keycloak, "09f7f651-9f19-5407-b222-8730fe8fd5c3", "a user")
	endOK(ctx, t, keycloak, "../../clients/x", "an escaping ID")

	if got := realm.loggedOut(); got != "/admin/realms/ac/users/09f7f651-9f19-5407-b222-8730fe8fd5c3/logout "+
		"/admin/realms/ac/users/..%2F..%2Fclients%2Fx/logout" {
		t.Errorf("paths %s", got)
	}

	wantTokens(t, realm, 1, "one token reused")

	realm.setStatus(http.StatusNotFound) // a user the realm does not know
	endOK(ctx, t, keycloak, "unknown", "an unknown user")

	realm.setStatus(
		http.StatusUnauthorized,
	) // the token was ended early: the next call fetches a new one

	if keycloak.EndSessions(ctx, "u-1") == nil {
		t.Error("a refused token passes")
	}

	realm.setStatus(http.StatusNoContent)
	endOK(ctx, t, keycloak, "u-1", "after a refused token")

	wantTokens(t, realm, 2, "after a refused token")

	now = now.Add(5 * time.Minute) // expired: fetched again

	endOK(ctx, t, keycloak, "u-1", "after expiry")

	wantTokens(t, realm, 3, "after expiry")

	if keycloak.EndSessions(ctx, "") == nil {
		t.Error("no subject passes")
	}

	if newTestKeycloak(srv.URL, "wrong", nil, nil).EndSessions(ctx, "u-1") == nil {
		t.Error("a wrong client secret passes")
	}
	// the service's own HTTP client and the wall clock
	endOK(
		ctx,
		t,
		newTestKeycloak(srv.URL, testSecret, nil, srv.Client()),
		"u-1",
		"with its own client",
	)
}

// TestKeycloakTokenAnswers: a token endpoint that answers without a token, or with something else than JSON, ends
// nothing.
func TestKeycloakTokenAnswers(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{}`, `{"access_token":""}`, `not json`} {
		srv := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				if strings.HasSuffix(req.URL.Path, "/token") {
					_, _ = writer.Write([]byte(body))

					return
				}

				writer.WriteHeader(http.StatusNoContent)
			}),
		)

		keycloak := newTestKeycloak(srv.URL, testSecret, nil, nil)
		keycloak.TokenURL = srv.URL + "/token"

		if keycloak.EndSessions(t.Context(), "u-1") == nil {
			t.Errorf("token answer %s passes", body)
		}

		srv.Close()
	}
}
