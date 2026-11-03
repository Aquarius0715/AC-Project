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

// TestKeycloakEndSessions: the service account's token is fetched once and reused, the logout goes to the user's own
// admin path (an ID cannot leave it), an unknown user has nothing to end, and a refused token is fetched again (IR386).
func TestKeycloakEndSessions(t *testing.T) {
	var (
		tokens, logouts atomic.Int32
		paths           []string
	)

	status := http.StatusNoContent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.URL.Path == "/realms/ac/protocol/openid-connect/token":
			_ = req.ParseForm()
			if req.Form.Get("grant_type") != "client_credentials" ||
				req.Form.Get("client_id") != "svc" ||
				req.Form.Get("client_secret") != "test-secret" {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			tokens.Add(1)

			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case strings.HasPrefix(req.URL.EscapedPath(), "/admin/realms/ac/users/"):
			if req.Header.Get("Authorization") != "Bearer admin-token" ||
				req.Method != http.MethodPost {
				w.WriteHeader(http.StatusForbidden)

				return
			}

			logouts.Add(1)

			paths = append(paths, req.URL.EscapedPath())

			w.WriteHeader(status)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	keycloak := &Keycloak{
		TokenURL:     srv.URL + "/realms/ac/protocol/openid-connect/token",
		AdminURL:     srv.URL + "/admin/realms/ac",
		ClientID:     "svc",
		ClientSecret: "test-secret",
		Now:          func() time.Time { return now },
		Client:       nil,
		mu:           sync.Mutex{},
		token:        "",
		until:        time.Time{},
	}

	ctx := context.Background()

	err := keycloak.EndSessions(ctx, "09f7f651-9f19-5407-b222-8730fe8fd5c3")
	if err != nil {
		t.Fatal(err)
	}

	err = keycloak.EndSessions(ctx, "../../clients/x")
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(
		paths,
		" ",
	); got != "/admin/realms/ac/users/09f7f651-9f19-5407-b222-8730fe8fd5c3/logout "+
		"/admin/realms/ac/users/..%2F..%2Fclients%2Fx/logout" {
		t.Errorf("paths %s", got)
	}

	if tokens.Load() != 1 {
		t.Errorf("token requests %d, want one reused", tokens.Load())
	}

	status = http.StatusNotFound // a user the realm does not know

	err = keycloak.EndSessions(ctx, "unknown")
	if err != nil {
		t.Errorf("unknown user: %v", err)
	}

	status = http.StatusUnauthorized // the token was ended early: the next call fetches a new one

	err = keycloak.EndSessions(ctx, "u-1")
	if err == nil {
		t.Error("a refused token passes")
	}

	status = http.StatusNoContent

	err = keycloak.EndSessions(ctx, "u-1")
	if err != nil || tokens.Load() != 2 {
		t.Errorf("after a refused token: %v, %d token requests", err, tokens.Load())
	}

	now = now.Add(5 * time.Minute) // expired: fetched again

	err = keycloak.EndSessions(ctx, "u-1")
	if err != nil || tokens.Load() != 3 {
		t.Errorf("after expiry: %v, %d token requests", err, tokens.Load())
	}

	err = keycloak.EndSessions(ctx, "")
	if err == nil {
		t.Error("no subject passes")
	}

	bad := &Keycloak{
		TokenURL:     keycloak.TokenURL,
		AdminURL:     keycloak.AdminURL,
		ClientID:     "svc",
		ClientSecret: "wrong",
		Client:       nil,
		Now:          nil,
		mu:           sync.Mutex{},
		token:        "",
		until:        time.Time{},
	}

	err = bad.EndSessions(ctx, "u-1")
	if err == nil {
		t.Error("a wrong client secret passes")
	}
	// the service's own HTTP client and the wall clock
	own := &Keycloak{
		TokenURL:     keycloak.TokenURL,
		AdminURL:     keycloak.AdminURL,
		ClientID:     "svc",
		ClientSecret: "test-secret",
		Client:       srv.Client(), Now: nil, mu: sync.Mutex{}, token: "", until: time.Time{},
	}

	err = own.EndSessions(ctx, "u-1")
	if err != nil {
		t.Errorf("with its own client: %v", err)
	}
}

// TestKeycloakTokenAnswers: a token endpoint that answers without a token, or with something else than JSON, ends
// nothing.
func TestKeycloakTokenAnswers(t *testing.T) {
	for _, body := range []string{`{}`, `{"access_token":""}`, `not json`} {
		srv := httptest.NewServer(
			http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/token") {
					_, _ = writer.Write([]byte(body))

					return
				}

				writer.WriteHeader(http.StatusNoContent)
			}),
		)

		keycloak := &Keycloak{
			TokenURL:     srv.URL + "/token",
			AdminURL:     srv.URL + "/admin/realms/ac",
			ClientID:     "svc",
			ClientSecret: "test-secret",
			Client:       nil,
			Now:          nil,
			mu:           sync.Mutex{},
			token:        "",
			until:        time.Time{},
		}

		err := keycloak.EndSessions(context.Background(), "u-1")
		if err == nil {
			t.Errorf("token answer %s passes", body)
		}

		srv.Close()
	}
}
