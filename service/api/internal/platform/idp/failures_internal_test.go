package idp

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

// errDown is the failure of an HTTP transport that cannot reach the provider.
var errDown = errors.New("provider down")

// downTransport fails every request without sending it.
type downTransport struct{}

func (downTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, errDown }

// tokenHeld is a Keycloak at base with a valid cached token for the canonical address and the given HTTP client, so a
// call goes straight to the admin API.
func tokenHeld(base string, client *http.Client) *Keycloak {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	keycloak := newTestKeycloak(base, testSecret, func() time.Time { return now }, client)
	keycloak.tokens = map[string]adminToken{"": {value: "t", until: now.Add(time.Hour)}}

	return keycloak
}

// wantWrapped fails the test unless err is there and names where it happened.
func wantWrapped(t *testing.T, err error, where, what string) {
	t.Helper()

	if err == nil || !strings.HasPrefix(err.Error(), where+": ") {
		t.Errorf("%s: error %v, want one from %s", what, err, where)
	}
}

// adminFails sends one admin call on the user u-1 and fails the test unless it ends with an error from where.
func adminFails(t *testing.T, keycloak *Keycloak, method string, body any, where, what string) {
	t.Helper()

	res, err := keycloak.do(t.Context(), method, "/users/u-1", body)
	if res != nil {
		_ = res.Body.Close()
	}

	wantWrapped(t, err, where, what)
}

// TestAdminCallsWithoutAToken: when the token endpoint refuses, every admin call ends with that refusal and nothing is
// sent to the admin API.
func TestAdminCallsWithoutAToken(t *testing.T) {
	t.Parallel()

	keycloak, realm := newScriptedKeycloak(t, nil)
	keycloak.TokenURL = keycloak.AdminURL + "/no-token" // the realm answers 204 there, without a token

	ctx := t.Context()
	account := testAccount() // a technician: the set-up e-mail is the first admin call

	calls := map[string]func() error{
		"create": func() error {
			_, err := keycloak.CreateAccount(ctx, account)

			return err
		},
		"update": func() error { return keycloak.UpdateAccount(ctx, "u-1", account) },
		"set-up": func() error { return keycloak.SendSetup(ctx, "u-1", account) },
		"enable": func() error { return keycloak.SetEnabled(ctx, "u-1", true) },
		"delete": func() error { return keycloak.DeleteAccount(ctx, "u-1") },
		"search": func() error {
			_, err := keycloak.findByEmail(ctx, account.Email)

			return err
		},
		"otp": func() error {
			_, err := keycloak.hasOTP(ctx, "u-1")

			return err
		},
		"write user": func() error { return keycloak.writeUser(ctx, "u-1", map[string]any{"enabled": true}) },
	}

	for name, call := range calls {
		var status *statusError

		err := call()
		if !errors.As(err, &status) || status.what != "token endpoint" ||
			status.code != http.StatusNoContent {
			t.Errorf("%s: error %v, want the token endpoint's 204", name, err)
		}
	}

	if got := realm.count(); got != len(calls) { // only the token requests
		t.Errorf("%d calls, want %d token requests", got, len(calls))
	}
}

// TestRequestsThatCannotBeSent: a provider address that is no URL, a provider that cannot be reached, a body that is
// no JSON and a method that is none end the call with an error that names where it happened, and nothing is cached.
func TestRequestsThatCannotBeSent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	down := &http.Client{
		Timeout:       time.Second,
		Transport:     downTransport{},
		CheckRedirect: nil,
		Jar:           nil,
	}

	bad := tokenHeld("http://\x7f", testClient(t))
	wantWrapped(t, bad.EndSessions(ctx, "u-1"), "http.NewRequestWithContext", "logout at no URL")

	wantWrapped(
		t,
		tokenHeld("http://idp", down).EndSessions(ctx, "u-1"),
		"http.Client.Do",
		"logout at a provider down",
	)

	_, err := newTestKeycloak("http://\x7f", testSecret, nil, testClient(t)).accessToken(ctx, "")
	wantWrapped(t, err, "http.NewRequestWithContext", "token at no URL")

	unreached := newTestKeycloak("http://idp", testSecret, nil, down)
	_, err = unreached.accessToken(ctx, "")
	wantWrapped(t, err, "http.Client.Do", "token at a provider down")

	if len(unreached.tokens) != 0 {
		t.Errorf("tokens %v cached without an answer", unreached.tokens)
	}

	held := tokenHeld("http://idp", down)

	adminFails(
		t,
		held,
		http.MethodPut,
		map[string]any{"enabled": func() {}},
		"json.Marshal",
		"a body that is no JSON",
	)
	adminFails(t, held, "NO METHOD", nil, "http.NewRequestWithContext", "no method")
	adminFails(t, held, http.MethodGet, nil, "http.Client.Do", "an admin call at a provider down")

	if _, ok := held.tokens[""]; !ok {
		t.Error("a provider down cleared the token")
	}
}

// TestAdminRefusalDropsTheToken: an admin call answered 401 (the token was ended early) ends with that status and
// clears the cached token (TestKeycloakEndSessions checks that a cleared token is fetched again).
func TestAdminRefusalDropsTheToken(t *testing.T) {
	t.Parallel()

	keycloak, _ := newScriptedKeycloak(t, map[string]reply{
		"GET /users/u-1": {status: http.StatusUnauthorized, body: "", location: ""},
	})
	_, err := keycloak.readUser(t.Context(), "u-1")

	var status *statusError
	if !errors.As(err, &status) || status.code != http.StatusUnauthorized {
		t.Errorf("read user: error %v, want its 401", err)
	}

	keycloak.mu.Lock()
	_, cached := keycloak.tokens[""]
	keycloak.mu.Unlock()

	if cached {
		t.Error("the refused token is still cached")
	}
}

// TestWithPort: a host without a port stays as it is, and one with a port gets it.
func TestWithPort(t *testing.T) {
	t.Parallel()

	if got := withPort("idp.example", ""); got != "idp.example" {
		t.Errorf("no port: %s", got)
	}

	if got := withPort("idp.example", "8443"); got != "idp.example:8443" {
		t.Errorf("a port: %s", got)
	}
}
