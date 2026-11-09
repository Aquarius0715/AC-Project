package idp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// adminCall is one call the fake admin API took.
type adminCall struct{ method, path, query, body string }

// accountsRealm is the realm's token endpoint and the admin API's users: it keeps every admin call, answers a create
// with a conflict once the e-mail exists, and knows the users old-id, gone-id (lost), hq-new (no one-time-code device)
// and hq-otp (with one).
type accountsRealm struct {
	calls  []adminCall
	exists bool
}

func (f *accountsRealm) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/token" {
		_, _ = writer.Write([]byte(`{"access_token":"t","expires_in":300}`))

		return
	}

	buf := make([]byte, 4096)
	read, _ := req.Body.Read(buf)
	f.calls = append(
		f.calls,
		adminCall{
			req.Method,
			strings.TrimPrefix(req.URL.Path, "/admin"),
			req.URL.RawQuery,
			string(buf[:read]),
		},
	)

	f.answer(writer, req)
}

func (f *accountsRealm) answer(writer http.ResponseWriter, req *http.Request) {
	switch {
	case req.Method == http.MethodPost && req.URL.Path == "/admin/users":
		if f.exists {
			writer.WriteHeader(http.StatusConflict)

			return
		}

		writer.Header().Set("Location", "http://kc/admin/users/new-id")
		writer.WriteHeader(http.StatusCreated)
	case req.URL.Path == "/admin/users/gone-id":
		writer.WriteHeader(http.StatusNotFound)
	default:
		body, ok := cannedAdminAnswers()[req.Method+" "+req.URL.Path]
		if !ok {
			writer.WriteHeader(http.StatusNoContent)

			return
		}

		_, _ = writer.Write([]byte(body))
	}
}

// cannedAdminAnswers are the admin API's answers to the reads the test makes, by method and path.
func cannedAdminAnswers() map[string]string {
	return map[string]string{ //nolint:gosec // the keys are Keycloak admin API paths (…/credentials), not credentials
		"GET /admin/users": `[{"id":"old-id"}]`,
		"GET /admin/users/old-id": `{"id":"old-id","username":"tech@contractor.example","email":"tech@contractor.example",` +
			`"enabled":false,"attributes":{"locale":["ms"],"membership_id":["m0"]}}`,
		"GET /admin/users/hq-new/credentials": `[{"type":"password"}]`,
		"GET /admin/users/hq-otp/credentials": `[{"type":"password"},{"type":"otp"}]`,
	}
}

// newAccountsKeycloak is a client of a fresh fake admin API, stopped when the test ends.
func newAccountsKeycloak(t *testing.T) (*Keycloak, *accountsRealm) {
	t.Helper()

	realm := &accountsRealm{calls: nil, exists: false}
	srv := httptest.NewServer(realm)
	t.Cleanup(srv.Close)

	return &Keycloak{
		TokenURL:     srv.URL + "/token",
		AdminURL:     srv.URL + "/admin",
		ClientID:     testClientID,
		ClientSecret: "s",
		PublicURL:    "",
		Client:       nil,
		Now:          nil,
		mu:           sync.Mutex{},
		tokens: nil,
	}, realm
}

// testAccount is the technician account the tests provision.
func testAccount() Account {
	return Account{
		Email:       "tech@contractor.example",
		DisplayName: "New Tech",
		TenantID:    "t1",
		Membership:  "m1",
		Role:        "technician",
		ClientID:    "ac-web",
		RedirectURI: "http://localhost:3002/login",
	}
}

// setupActions sends the set-up e-mail and answers the actions it asks for (the body of the call that sends it, at the
// given position among the calls).
func setupActions(
	t *testing.T,
	keycloak *Keycloak,
	realm *accountsRealm,
	subject string,
	account Account,
	sentAt int,
) string {
	t.Helper()

	realm.calls = nil

	err := keycloak.SendSetup(t.Context(), subject, account)
	if err != nil || len(realm.calls) <= sentAt {
		t.Fatalf("set-up e-mail of %s: %v %v", subject, err, realm.calls)
	}

	return realm.calls[sentAt].body
}

// TestKeycloakAccounts (IR461, IR462): a created account carries the claim attributes and returns the new user's ID;
// an existing e-mail is found and updated instead (a retried provisioning is one account), keeping the user's other
// attributes; an update of an account the realm lost is ErrNoAccount; the set-up e-mail asks for a password and the
// e-mail check and returns to the app; an account is turned off.
func TestKeycloakAccounts(t *testing.T) {
	t.Parallel()

	keycloak, realm := newAccountsKeycloak(t)
	account := testAccount()

	id, err := keycloak.CreateAccount(t.Context(), account)
	if err != nil || id != "new-id" {
		t.Fatalf("create: %q %v", id, err)
	}

	checkCreatedUser(t, realm.calls[0].body, account)
	checkExistingEmail(t, keycloak, realm, account)

	err = keycloak.UpdateAccount(t.Context(), "gone-id", account)
	if !errors.Is(err, ErrNoAccount) {
		t.Errorf("an account the realm lost: %v", err)
	}

	checkSetupEmail(t, keycloak, realm, account)

	realm.calls = nil

	err = keycloak.SetEnabled(t.Context(), "new-id", false)
	if err != nil || realm.calls[0].body != `{"enabled":false}` {
		t.Errorf("disable: %v %v", err, realm.calls)
	}

	realm.calls = nil

	err = keycloak.DeleteAccount(t.Context(), "new-id")
	if err != nil || realm.calls[0].method != http.MethodDelete ||
		realm.calls[0].path != "/users/new-id" {
		t.Errorf("delete: %v %v", err, realm.calls)
	}

	if keycloak.DeleteAccount(t.Context(), "") == nil {
		t.Error("delete without a subject passes")
	}
}

// checkExistingEmail: creating an account whose e-mail exists updates that user instead, and the whole user goes
// back: the e-mail stays (IR463), the other attributes too.
func checkExistingEmail(t *testing.T, keycloak *Keycloak, realm *accountsRealm, account Account) {
	t.Helper()

	realm.exists, realm.calls = true, nil

	id, err := keycloak.CreateAccount(t.Context(), account)
	if err != nil || id != "old-id" || len(realm.calls) != 4 {
		t.Fatalf("existing e-mail: %q %v %v", id, err, realm.calls)
	}

	if put := realm.calls[3]; put.method != http.MethodPut || put.path != "/users/old-id" {
		t.Fatalf("existing e-mail: the update went %s %s", put.method, put.path)
	}

	checkUpdatedUser(t, realm.calls[3].body, account)
}

// checkSetupEmail: the set-up e-mail asks for a password and the e-mail check and returns to the app.
func checkSetupEmail(t *testing.T, keycloak *Keycloak, realm *accountsRealm, account Account) {
	t.Helper()

	body := setupActions(t, keycloak, realm, "new-id", account, 0)
	if realm.calls[0].path != "/users/new-id/execute-actions-email" ||
		!strings.Contains(
			realm.calls[0].query,
			"redirect_uri=http%3A%2F%2Flocalhost%3A3002%2Flogin",
		) {
		t.Errorf("set-up e-mail: %v", realm.calls)
	}

	if body != `["UPDATE_PASSWORD","VERIFY_EMAIL"]` {
		t.Errorf("set-up e-mail actions: %s", body)
	}
}

// TestKeycloakHQAccounts (IR468): a new HQ account must set up its one-time-code device, in the e-mail's steps and as a
// required action; one with a device keeps it.
func TestKeycloakHQAccounts(t *testing.T) {
	t.Parallel()

	keycloak, realm := newAccountsKeycloak(t)
	hqAccount := testAccount()
	hqAccount.Role = "admin"

	_, err := keycloak.CreateAccount(t.Context(), hqAccount)
	if err != nil ||
		!strings.Contains(realm.calls[0].body, `"requiredActions":["CONFIGURE_TOTP"]`) {
		t.Errorf("a new HQ account: %v %v", err, realm.calls)
	}

	if body := setupActions(t, keycloak, realm, "hq-new", hqAccount, 1); body !=
		`["UPDATE_PASSWORD","VERIFY_EMAIL","CONFIGURE_TOTP"]` {
		t.Errorf("the set-up e-mail of an HQ account without a device: %s", body)
	}

	if body := setupActions(
		t,
		keycloak,
		realm,
		"hq-otp",
		hqAccount,
		1,
	); body != `["UPDATE_PASSWORD","VERIFY_EMAIL"]` {
		t.Errorf("the set-up e-mail of an HQ account with a device: %s", body)
	}
}

// checkCreatedUser: the created user signs in with the e-mail, is enabled and carries the claim attributes.
func checkCreatedUser(t *testing.T, body string, account Account) {
	t.Helper()

	var user struct {
		Username   string              `json:"username"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}

	err := json.Unmarshal([]byte(body), &user)
	if err != nil || user.Username != account.Email || !user.Enabled ||
		user.Attributes["membership_id"][0] != "m1" || user.Attributes["role"][0] != "technician" ||
		user.Attributes["tenant_id"][0] != "t1" {
		t.Errorf("created user: %s", body)
	}
}

// checkUpdatedUser: the updated user keeps the e-mail and the attributes the update did not set.
func checkUpdatedUser(t *testing.T, body string, account Account) {
	t.Helper()

	var updated struct {
		Email      string              `json:"email"`
		Enabled    bool                `json:"enabled"`
		Attributes map[string][]string `json:"attributes"`
	}

	err := json.Unmarshal([]byte(body), &updated)
	if err != nil || !updated.Enabled || updated.Email != account.Email ||
		updated.Attributes["membership_id"][0] != "m1" || updated.Attributes["locale"][0] != "ms" {
		t.Errorf("updated user: %s", body)
	}
}
