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
		PublicURL:    "",
		Now:          now,
		Client:       client,
		mu:           sync.Mutex{},
		tokens:       nil,
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

// TestKeycloakNamesItsPublicAddress: with a PublicURL, the token request and the admin request carry the forwarded host
// and scheme the identity provider builds its links from (IR651); without one they carry nothing of the kind.
func TestKeycloakNamesItsPublicAddress(t *testing.T) {
	t.Parallel()

	for public, want := range map[string]string{"http://10.100.1.17:8281": "10.100.1.17:8281 http", "": " "} {
		var (
			lock sync.Mutex
			seen []string
		)

		srv := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
				lock.Lock()

				seen = append(
					seen,
					req.Header.Get("X-Forwarded-Host")+" "+req.Header.Get("X-Forwarded-Proto"),
				)

				lock.Unlock()

				if strings.HasSuffix(req.URL.Path, "/token") {
					_, _ = writer.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))

					return
				}

				writer.WriteHeader(http.StatusNoContent)
			}),
		)

		keycloak := newTestKeycloak(srv.URL, testSecret, nil, nil)
		keycloak.PublicURL = public
		keycloak.ClientID = ""
		keycloak.ClientSecret = ""

		err := keycloak.EndSessions(t.Context(), "09f7f651-9f19-5407-b222-8730fe8fd5c3")

		srv.Close()

		if err != nil {
			t.Fatalf("%q: %v", public, err)
		}

		lock.Lock()
		got := strings.Join(seen, "|")
		lock.Unlock()

		if got != want+"|"+want {
			t.Errorf(
				"%q: the token and the admin request carried %q, want %q twice",
				public,
				got,
				want,
			)
		}
	}
}

// addressRealm records what each call of a realm named: the path with its forwarded host, and the redirect address of
// the set-up e-mail. Each token it issues names the host it was asked for, as Keycloak does.
type addressRealm struct {
	lock     sync.Mutex
	calls    []string
	redirect string
	tokens   atomic.Int64
}

func (realm *addressRealm) serve(writer http.ResponseWriter, req *http.Request) {
	realm.lock.Lock()
	defer realm.lock.Unlock()

	host := req.Header.Get("X-Forwarded-Host")

	switch {
	case strings.HasSuffix(req.URL.Path, "/token"):
		realm.tokens.Add(1)
		realm.calls = append(realm.calls, "token "+host)
		_, _ = writer.Write([]byte(`{"access_token":"token-of-` + host + `","expires_in":300}`))
	case strings.HasSuffix(req.URL.Path, "/execute-actions-email"):
		if req.Header.Get("Authorization") != "Bearer token-of-"+host {
			writer.WriteHeader(http.StatusUnauthorized)

			return
		}

		realm.calls = append(realm.calls, "mail "+host)
		realm.redirect = req.URL.Query().Get("redirect_uri")

		writer.WriteHeader(http.StatusNoContent)
	default:
		realm.calls = append(realm.calls, "admin "+host)
		writer.WriteHeader(http.StatusNoContent)
	}
}

func (realm *addressRealm) seen() (string, string) {
	realm.lock.Lock()
	defer realm.lock.Unlock()

	return strings.Join(realm.calls, "|"), realm.redirect
}

// setupAt sends one set-up e-mail on a fresh realm and answers what the realm saw.
func setupAt(t *testing.T, public, host, redirect string) (string, string) {
	t.Helper()

	realm := &addressRealm{lock: sync.Mutex{}, calls: nil, redirect: "", tokens: atomic.Int64{}}
	srv := httptest.NewServer(http.HandlerFunc(realm.serve))

	defer srv.Close()

	keycloak := newTestKeycloak(srv.URL, testSecret, nil, nil)
	keycloak.PublicURL = public

	err := keycloak.SendSetup(t.Context(), "subject-1", Account{
		Email: "a@example.com", DisplayName: "A", TenantID: "t", Membership: "m", Role: "client_user",
		RedirectURI: redirect, ClientID: "ac-customer", PublicHost: host,
	})
	if err != nil {
		t.Fatal(err)
	}

	return realm.seen()
}

// TestSendSetupUsesTheAddressOfTheInvitation: the e-mail's link has the address the invitation came from, in the host
// and in the return address, on the ports of the deployment; with one address (no PublicURL) nothing changes.
func TestSendSetupUsesTheAddressOfTheInvitation(t *testing.T) {
	t.Parallel()

	const (
		canonical = "http://10.100.1.17:8281"
		tailnet   = "100.99.169.116"
		app       = "http://10.100.1.17:3200/login"
	)

	for name, testCase := range map[string]struct {
		public, host, redirect, calls, wantRedirect string
	}{
		"another address": {
			canonical, tailnet, app,
			"token 100.99.169.116:8281|mail 100.99.169.116:8281", "http://100.99.169.116:3200/login",
		},
		"no address named": {canonical, "", app, "token 10.100.1.17:8281|mail 10.100.1.17:8281", app},
		"the canonical address": {
			canonical, "10.100.1.17", app, "token 10.100.1.17:8281|mail 10.100.1.17:8281", app,
		},
		"one-address deployment": {"", tailnet, app, "token |mail ", app},
		// an app on a host of its own is not one of the provider's addresses (production: a provider of its own)
		"an app elsewhere": {
			canonical, tailnet, "https://app.example.com/login",
			"token 100.99.169.116:8281|mail 100.99.169.116:8281", "https://app.example.com/login",
		},
	} {
		calls, redirect := setupAt(t, testCase.public, testCase.host, testCase.redirect)
		if calls != testCase.calls {
			t.Errorf("%s: the realm saw %q, want %q", name, calls, testCase.calls)
		}

		if redirect != testCase.wantRedirect {
			t.Errorf("%s: redirect %q, want %q", name, redirect, testCase.wantRedirect)
		}
	}
}

// TestAdminTokensAreKeptPerAddress: Keycloak checks a token's issuer against the address of the request, so a token of
// one address is not used on another; each address's token is kept, and a refusal drops only its own.
func TestAdminTokensAreKeptPerAddress(t *testing.T) {
	t.Parallel()

	realm := &addressRealm{lock: sync.Mutex{}, calls: nil, redirect: "", tokens: atomic.Int64{}}
	srv := httptest.NewServer(http.HandlerFunc(realm.serve))

	defer srv.Close()

	keycloak := newTestKeycloak(srv.URL, testSecret, nil, nil)
	keycloak.PublicURL = "http://10.100.1.17:8281"

	send := func(host string) {
		t.Helper()

		err := keycloak.SendSetup(t.Context(), "subject-1", Account{
			Email: "a@example.com", DisplayName: "A", TenantID: "t", Membership: "m", Role: "client_user",
			RedirectURI: "http://10.100.1.17:3200/login", ClientID: "ac-customer", PublicHost: host,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	send("100.99.169.116")
	send("")
	send("100.99.169.116")
	send("")

	if got := realm.tokens.Load(); got != 2 {
		t.Errorf("%d tokens were issued, want one for each of the 2 addresses", got)
	}

	// other calls stay on the canonical address, with its token
	err := keycloak.SetEnabled(t.Context(), "subject-1", true)
	if err != nil {
		t.Fatal(err)
	}

	calls, _ := realm.seen()
	if !strings.HasSuffix(calls, "admin 10.100.1.17:8281") {
		t.Errorf("the other call went out as %q", calls)
	}
}
