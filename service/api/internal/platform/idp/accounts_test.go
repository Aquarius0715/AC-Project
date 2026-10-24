package idp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestKeycloakAccounts (IR461, IR462): a created account carries the claim attributes and returns the new user's ID;
// an existing e-mail is found and updated instead (a retried provisioning is one account), keeping the user's other
// attributes; an update of an account the realm lost is ErrNoAccount; the set-up e-mail asks for a password and the
// e-mail check and returns to the app; an account is turned off.
func TestKeycloakAccounts(t *testing.T) {
	type call struct{ method, path, query, body string }
	var calls []call
	exists := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300}`))
			return
		}
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		calls = append(calls, call{r.Method, strings.TrimPrefix(r.URL.Path, "/admin"), r.URL.RawQuery, string(buf[:n])})
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/admin/users":
			if exists {
				w.WriteHeader(http.StatusConflict)
				return
			}
			w.Header().Set("Location", "http://kc/admin/users/new-id")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/admin/users":
			_, _ = w.Write([]byte(`[{"id":"old-id"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/admin/users/old-id":
			_, _ = w.Write([]byte(`{"id":"old-id","username":"tech@contractor.example","email":"tech@contractor.example","enabled":false,"attributes":{"locale":["ms"],"membership_id":["m0"]}}`))
		case r.URL.Path == "/admin/users/gone-id":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	k := &Keycloak{TokenURL: srv.URL + "/token", AdminURL: srv.URL + "/admin", ClientID: "svc", ClientSecret: "s"}
	ctx := context.Background()
	a := Account{Email: "tech@contractor.example", DisplayName: "New Tech", TenantID: "t1", Membership: "m1", Role: "technician", ClientID: "ac-web", RedirectURI: "http://localhost:3002/login"}
	id, err := k.CreateAccount(ctx, a)
	if err != nil || id != "new-id" {
		t.Fatalf("create: %q %v", id, err)
	}
	var user struct {
		Username   string              `json:"username"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(calls[0].body), &user); err != nil || user.Username != a.Email || !user.Enabled ||
		user.Attributes["membership_id"][0] != "m1" || user.Attributes["role"][0] != "technician" || user.Attributes["tenant_id"][0] != "t1" {
		t.Errorf("created user: %s", calls[0].body)
	}
	exists, calls = true, nil
	if id, err := k.CreateAccount(ctx, a); err != nil || id != "old-id" || len(calls) != 4 || calls[3].method != http.MethodPut || calls[3].path != "/users/old-id" {
		t.Fatalf("existing e-mail: %q %v %v", id, err, calls)
	}
	var updated struct {
		Email      string              `json:"email"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}
	// the whole user goes back: the e-mail stays (IR463), the other attributes too
	if err := json.Unmarshal([]byte(calls[3].body), &updated); err != nil || !updated.Enabled || updated.Email != a.Email || updated.Attributes["membership_id"][0] != "m1" || updated.Attributes["locale"][0] != "ms" {
		t.Errorf("updated user: %s", calls[3].body)
	}
	if err := k.UpdateAccount(ctx, "gone-id", a); !errors.Is(err, ErrNoAccount) {
		t.Errorf("an account the realm lost: %v", err)
	}
	calls = nil
	if err := k.SendSetup(ctx, "new-id", a); err != nil || calls[0].path != "/users/new-id/execute-actions-email" ||
		!strings.Contains(calls[0].query, "redirect_uri=http%3A%2F%2Flocalhost%3A3002%2Flogin") || calls[0].body != `["UPDATE_PASSWORD","VERIFY_EMAIL"]` {
		t.Errorf("set-up e-mail: %v %v", err, calls)
	}
	calls = nil
	if err := k.SetEnabled(ctx, "new-id", false); err != nil || calls[0].body != `{"enabled":false}` {
		t.Errorf("disable: %v %v", err, calls)
	}
}
