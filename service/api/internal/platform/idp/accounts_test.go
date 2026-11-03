package idp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestKeycloakAccounts (IR461, IR462, IR468): a created account carries the claim attributes and returns the new user's
// ID; an existing e-mail is found and updated instead (a retried provisioning is one account), keeping the user's other
// attributes; an update of an account the realm lost is ErrNoAccount; the set-up e-mail asks for a password and the
// e-mail check and returns to the app; an account is turned off.
func TestKeycloakAccounts(t *testing.T) {
	type call struct{ method, path, query, body string }

	var calls []call

	exists := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300}`))

			return
		}

		buf := make([]byte, 4096)
		n, _ := req.Body.Read(buf)

		calls = append(
			calls,
			call{
				req.Method,
				strings.TrimPrefix(req.URL.Path, "/admin"),
				req.URL.RawQuery,
				string(buf[:n]),
			},
		)
		switch {
		case req.Method == http.MethodPost && req.URL.Path == "/admin/users":
			if exists {
				w.WriteHeader(http.StatusConflict)

				return
			}

			w.Header().Set("Location", "http://kc/admin/users/new-id")
			w.WriteHeader(http.StatusCreated)
		case req.Method == http.MethodGet && req.URL.Path == "/admin/users":
			_, _ = w.Write([]byte(`[{"id":"old-id"}]`))
		case req.Method == http.MethodGet && req.URL.Path == "/admin/users/old-id":
			_, _ = w.Write(
				[]byte(
					`{"id":"old-id","username":"tech@contractor.example","email":"tech@contractor.example","enabled":false,` +
						`"attributes":{"locale":["ms"],"membership_id":["m0"]}}`,
				),
			)
		case req.URL.Path == "/admin/users/gone-id":
			w.WriteHeader(http.StatusNotFound)
		case req.Method == http.MethodGet && req.URL.Path == "/admin/users/hq-new/credentials":
			_, _ = w.Write([]byte(`[{"type":"password"}]`))
		case req.Method == http.MethodGet && req.URL.Path == "/admin/users/hq-otp/credentials":
			_, _ = w.Write([]byte(`[{"type":"password"},{"type":"otp"}]`))
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()

	keycloak := &Keycloak{
		TokenURL:     srv.URL + "/token",
		AdminURL:     srv.URL + "/admin",
		ClientID:     "svc",
		ClientSecret: "s", Client: nil, Now: nil, mu: sync.Mutex{}, token: "", until: time.Time{},
	}
	ctx := context.Background()
	account := Account{
		Email:       "tech@contractor.example",
		DisplayName: "New Tech",
		TenantID:    "t1",
		Membership:  "m1",
		Role:        "technician",
		ClientID:    "ac-web",
		RedirectURI: "http://localhost:3002/login",
	}

	id, err := keycloak.CreateAccount(ctx, account)
	if err != nil || id != "new-id" {
		t.Fatalf("create: %q %v", id, err)
	}

	var user struct {
		Username   string              `json:"username"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}

	unmarshalErr := json.Unmarshal(
		[]byte(calls[0].body),
		&user,
	)
	if unmarshalErr != nil || user.Username != account.Email || !user.Enabled ||
		user.Attributes["membership_id"][0] != "m1" || user.Attributes["role"][0] != "technician" ||
		user.Attributes["tenant_id"][0] != "t1" {
		t.Errorf("created user: %s", calls[0].body)
	}

	exists, calls = true, nil

	createAccountId, createAccountErr := keycloak.CreateAccount(
		ctx,
		account,
	)
	if createAccountErr != nil || createAccountId != "old-id" || len(calls) != 4 ||
		calls[3].method != http.MethodPut ||
		calls[3].path != "/users/old-id" {
		t.Fatalf("existing e-mail: %q %v %v", createAccountId, createAccountErr, calls)
	}

	var updated struct {
		Email      string              `json:"email"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}
	// the whole user goes back: the e-mail stays (IR463), the other attributes too
	unmarshalErr = json.Unmarshal(
		[]byte(calls[3].body),
		&updated,
	)
	if unmarshalErr != nil || !updated.Enabled || updated.Email != account.Email ||
		updated.Attributes["membership_id"][0] != "m1" ||
		updated.Attributes["locale"][0] != "ms" {
		t.Errorf("updated user: %s", calls[3].body)
	}

	updateAccountErr := keycloak.UpdateAccount(ctx, "gone-id", account)
	if !errors.Is(updateAccountErr, ErrNoAccount) {
		t.Errorf("an account the realm lost: %v", updateAccountErr)
	}

	calls = nil

	sendSetupErr := keycloak.SendSetup(
		ctx,
		"new-id",
		account,
	)
	if sendSetupErr != nil || calls[0].path != "/users/new-id/execute-actions-email" ||
		!strings.Contains(calls[0].query, "redirect_uri=http%3A%2F%2Flocalhost%3A3002%2Flogin") ||
		calls[0].body != `["UPDATE_PASSWORD","VERIFY_EMAIL"]` {
		t.Errorf("set-up e-mail: %v %v", sendSetupErr, calls)
	}
	// HQ (IR468): a new account must set up its one-time-code device, in the e-mail's steps and as a required action;
	// one with a device keeps it
	hq := account
	hq.Role = "admin"

	exists, calls = false, nil

	_, createAccountErr = keycloak.CreateAccount(
		ctx,
		hq,
	)
	if createAccountErr != nil ||
		!strings.Contains(calls[0].body, `"requiredActions":["CONFIGURE_TOTP"]`) {
		t.Errorf("a new HQ account: %v %v", createAccountErr, calls)
	}

	calls = nil

	sendSetupErr = keycloak.SendSetup(
		ctx,
		"hq-new",
		hq,
	)
	if sendSetupErr != nil ||
		calls[1].body != `["UPDATE_PASSWORD","VERIFY_EMAIL","CONFIGURE_TOTP"]` {
		t.Errorf("the set-up e-mail of an HQ account without a device: %v %v", sendSetupErr, calls)
	}

	calls = nil

	sendSetupErr = keycloak.SendSetup(
		ctx,
		"hq-otp",
		hq,
	)
	if sendSetupErr != nil ||
		calls[1].body != `["UPDATE_PASSWORD","VERIFY_EMAIL"]` {
		t.Errorf("the set-up e-mail of an HQ account with a device: %v %v", sendSetupErr, calls)
	}

	calls = nil

	setEnabledErr := keycloak.SetEnabled(
		ctx,
		"new-id",
		false,
	)
	if setEnabledErr != nil ||
		calls[0].body != `{"enabled":false}` {
		t.Errorf("disable: %v %v", setEnabledErr, calls)
	}
}
