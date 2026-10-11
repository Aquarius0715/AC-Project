package idp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestKeycloakEndSessions: the service account's token is fetched once and reused, the logout goes to the user's own
// admin path (an ID cannot leave it), an unknown user has nothing to end, and a refused token is fetched again (IR386).
func TestKeycloakEndSessions(t *testing.T) {
	var tokens, logouts atomic.Int32
	var paths []string
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/ac/protocol/openid-connect/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "svc" || r.Form.Get("client_secret") != "test-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			tokens.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"admin-token","expires_in":300}`))
		case strings.HasPrefix(r.URL.EscapedPath(), "/admin/realms/ac/users/"):
			if r.Header.Get("Authorization") != "Bearer admin-token" || r.Method != http.MethodPost {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			logouts.Add(1)
			paths = append(paths, r.URL.EscapedPath())
			w.WriteHeader(status)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	k := &Keycloak{TokenURL: srv.URL + "/realms/ac/protocol/openid-connect/token", AdminURL: srv.URL + "/admin/realms/ac", ClientID: "svc", ClientSecret: "test-secret", Now: func() time.Time { return now }}
	ctx := context.Background()
	if err := k.EndSessions(ctx, "09f7f651-9f19-5407-b222-8730fe8fd5c3"); err != nil {
		t.Fatal(err)
	}
	if err := k.EndSessions(ctx, "../../clients/x"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(paths, " "); got != "/admin/realms/ac/users/09f7f651-9f19-5407-b222-8730fe8fd5c3/logout /admin/realms/ac/users/..%2F..%2Fclients%2Fx/logout" {
		t.Errorf("paths %s", got)
	}
	if tokens.Load() != 1 {
		t.Errorf("token requests %d, want one reused", tokens.Load())
	}
	status = http.StatusNotFound // a user the realm does not know
	if err := k.EndSessions(ctx, "unknown"); err != nil {
		t.Errorf("unknown user: %v", err)
	}
	status = http.StatusUnauthorized // the token was ended early: the next call fetches a new one
	if err := k.EndSessions(ctx, "u-1"); err == nil {
		t.Error("a refused token passes")
	}
	status = http.StatusNoContent
	if err := k.EndSessions(ctx, "u-1"); err != nil || tokens.Load() != 2 {
		t.Errorf("after a refused token: %v, %d token requests", err, tokens.Load())
	}
	now = now.Add(5 * time.Minute) // expired: fetched again
	if err := k.EndSessions(ctx, "u-1"); err != nil || tokens.Load() != 3 {
		t.Errorf("after expiry: %v, %d token requests", err, tokens.Load())
	}
	if err := k.EndSessions(ctx, ""); err == nil {
		t.Error("no subject passes")
	}
	bad := &Keycloak{TokenURL: k.TokenURL, AdminURL: k.AdminURL, ClientID: "svc", ClientSecret: "wrong"}
	if err := bad.EndSessions(ctx, "u-1"); err == nil {
		t.Error("a wrong client secret passes")
	}
}
