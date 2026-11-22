package idp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// createUser is the script key of the admin API's create-user call.
const createUser = "POST /users"

// reply is one scripted answer of the admin API.
type reply struct {
	status   int
	body     string
	location string
}

// scriptedRealm answers the token endpoint with a token and each admin call by its method and path from the script
// (204 when the script has none); it counts the admin calls and keeps the body of the last PUT.
type scriptedRealm struct {
	script  map[string]reply
	mu      sync.Mutex
	calls   int
	lastPut string
}

func (f *scriptedRealm) ServeHTTP(writer http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/token" {
		_, _ = writer.Write([]byte(`{"access_token":"t","expires_in":300}`))

		return
	}

	body, _ := io.ReadAll(req.Body)

	f.mu.Lock()
	f.calls++

	if req.Method == http.MethodPut {
		f.lastPut = string(body)
	}
	f.mu.Unlock()

	answer, ok := f.script[req.Method+" "+strings.TrimPrefix(req.URL.Path, "/admin")]
	if !ok {
		writer.WriteHeader(http.StatusNoContent)

		return
	}

	if answer.location != "" {
		writer.Header().Set("Location", answer.location)
	}

	writer.WriteHeader(answer.status)
	_, _ = writer.Write([]byte(answer.body))
}

// count is the number of admin calls so far.
func (f *scriptedRealm) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls
}

// put is the body of the last PUT, "" before any.
func (f *scriptedRealm) put() string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.lastPut
}

// newScriptedKeycloak is a client of a fake admin API that answers by the script, stopped when the test ends.
func newScriptedKeycloak(t *testing.T, script map[string]reply) (*Keycloak, *scriptedRealm) {
	t.Helper()

	realm := &scriptedRealm{script: script, mu: sync.Mutex{}, calls: 0, lastPut: ""}
	srv := httptest.NewServer(realm)
	t.Cleanup(srv.Close)

	return &Keycloak{
		TokenURL:     srv.URL + "/token",
		AdminURL:     srv.URL + "/admin",
		ClientID:     testClientID,
		ClientSecret: "s",
		PublicURL:    "",
		Client:       testClient(t),
		Now:          nil,
		mu:           sync.Mutex{},
		tokens:       nil,
	}, realm
}

// wantStatusError: the call failed with the provider's unexpected status, naming the call.
func wantStatusError(t *testing.T, err error, what string, code int) {
	t.Helper()

	var got *statusError
	if !errors.As(err, &got) || got.what != what || got.code != code {
		t.Errorf("%s: want the provider's %d, got %v", what, code, err)

		return
	}

	if !strings.Contains(err.Error(), strconv.Itoa(code)) {
		t.Errorf("%s: the message does not name the status: %q", what, err.Error())
	}
}

// TestKeycloakRefusedAnswers: an answer the call does not expect is an error naming the call and the status, never a
// success: a refused create, enable, delete, set-up e-mail or read of the user; a user or credentials answered with no
// JSON; an HQ account's set-up e-mail does not go out when its devices cannot be read; an account without an e-mail is
// refused before any call.
func TestKeycloakRefusedAnswers(t *testing.T) {
	t.Parallel()

	keycloak, realm := newScriptedKeycloak(t, map[string]reply{
		createUser: {
			status:   http.StatusForbidden,
			body:     "",
			location: "",
		},
		"PUT /users/u1": {
			status:   http.StatusForbidden,
			body:     "",
			location: "",
		},
		"DELETE /users/u1": {
			status:   http.StatusInternalServerError,
			body:     "",
			location: "",
		},
		"PUT /users/u1/execute-actions-email": {
			status:   http.StatusBadRequest,
			body:     "",
			location: "",
		},
		"GET /users/u2": {
			status:   http.StatusInternalServerError,
			body:     "",
			location: "",
		},
		"GET /users/u3/credentials": {
			status:   http.StatusForbidden,
			body:     "",
			location: "",
		},
		"GET /users/u4/credentials": { // an object, not the list: the set-up e-mail (204 by default) must not go out
			status:   http.StatusOK,
			body:     `{"type":"otp"}`,
			location: "",
		},
		"GET /users/u5": {
			status:   http.StatusOK,
			body:     `not json`,
			location: "",
		},
	})
	account := testAccount()

	_, err := keycloak.CreateAccount(t.Context(), account)
	wantStatusError(t, err, "create user", http.StatusForbidden)
	wantStatusError(
		t,
		keycloak.SetEnabled(t.Context(), "u1", false),
		"enable",
		http.StatusForbidden,
	)
	wantStatusError(
		t,
		keycloak.DeleteAccount(t.Context(), "u1"),
		"delete",
		http.StatusInternalServerError,
	)
	wantStatusError(
		t,
		keycloak.SendSetup(t.Context(), "u1", account),
		"set-up e-mail",
		http.StatusBadRequest,
	)
	wantStatusError(
		t,
		keycloak.UpdateAccount(t.Context(), "u2", account),
		"read user",
		http.StatusInternalServerError,
	)

	if keycloak.UpdateAccount(t.Context(), "u5", account) == nil {
		t.Error("a user the provider answers with no JSON is updated")
	}

	hqAccount := testAccount()
	hqAccount.Role = "admin"

	before := realm.count()

	wantStatusError(
		t,
		keycloak.SendSetup(t.Context(), "u3", hqAccount),
		"credentials",
		http.StatusForbidden,
	)

	if realm.count() != before+1 {
		t.Errorf(
			"the set-up e-mail went out without knowing the HQ account's devices: %d calls",
			realm.count()-before,
		)
	}

	if keycloak.SendSetup(t.Context(), "u4", hqAccount) == nil {
		t.Error("credentials the provider answers with no list are read as no device")
	}

	before = realm.count()

	noEmail := testAccount()
	noEmail.Email = ""

	_, err = keycloak.CreateAccount(t.Context(), noEmail)
	if !errors.Is(err, errNoEmail) || realm.count() != before {
		t.Errorf("an account without an e-mail: %v after %d calls", err, realm.count()-before)
	}
}

// TestKeycloakCreatedWithoutLocation: a 201 whose Location names no user is no subject, not an empty one.
func TestKeycloakCreatedWithoutLocation(t *testing.T) {
	t.Parallel()

	for _, location := range []string{"", "http://kc/admin/users/"} {
		keycloak, _ := newScriptedKeycloak(t, map[string]reply{
			createUser: {status: http.StatusCreated, body: "", location: location},
		})

		id, err := keycloak.CreateAccount(t.Context(), testAccount())
		if !errors.Is(err, errNoUserLocation) || id != "" {
			t.Errorf("Location %q: %q %v", location, id, err)
		}
	}
}

// TestKeycloakConflictNeedsOneUser: a create that conflicts updates the e-mail's user only when the exact search finds
// exactly one; none, two or a refused search is an error and nothing is written.
func TestKeycloakConflictNeedsOneUser(t *testing.T) {
	t.Parallel()

	cases := map[string]reply{
		"none":    {status: http.StatusOK, body: `[]`, location: ""},
		"two":     {status: http.StatusOK, body: `[{"id":"a"},{"id":"b"}]`, location: ""},
		"refused": {status: http.StatusForbidden, body: "", location: ""},
	}

	for name, search := range cases {
		keycloak, realm := newScriptedKeycloak(t, map[string]reply{
			createUser:   {status: http.StatusConflict, body: "", location: ""},
			"GET /users": search,
		})

		id, err := keycloak.CreateAccount(t.Context(), testAccount())
		if err == nil || id != "" || realm.put() != "" {
			t.Errorf("%s: %q %v, wrote %q", name, id, err, realm.put())
		}

		if name == "refused" {
			wantStatusError(t, err, "user search", http.StatusForbidden)
		} else if !errors.Is(err, errUserNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestKeycloakUpdateWrite: a user without attributes gets the claims; a user deleted between the read and the write
// is ErrNoAccount; another refused write is an error naming the update.
func TestKeycloakUpdateWrite(t *testing.T) {
	t.Parallel()

	keycloak, realm := newScriptedKeycloak(t, map[string]reply{
		"GET /users/bare": {
			status:   http.StatusOK,
			body:     `{"id":"bare","email":"own@contractor.example"}`,
			location: "",
		},
		"GET /users/lost":   {status: http.StatusOK, body: `{"id":"lost"}`, location: ""},
		"PUT /users/lost":   {status: http.StatusNotFound, body: "", location: ""},
		"GET /users/locked": {status: http.StatusOK, body: `{"id":"locked"}`, location: ""},
		"PUT /users/locked": {status: http.StatusConflict, body: "", location: ""},
	})
	account := testAccount()

	err := keycloak.UpdateAccount(t.Context(), "bare", account)
	if err != nil {
		t.Fatalf("a user without attributes: %v", err)
	}

	var written struct {
		Email      string              `json:"email"`
		Attributes map[string][]string `json:"attributes"`
	}

	err = json.Unmarshal([]byte(realm.put()), &written)
	if err != nil || written.Email != "own@contractor.example" ||
		len(written.Attributes["role"]) != 1 ||
		written.Attributes["role"][0] != account.Role {
		t.Errorf("a user without attributes was written as %s", realm.put())
	}

	err = keycloak.UpdateAccount(t.Context(), "lost", account)
	if !errors.Is(err, ErrNoAccount) {
		t.Errorf("a user deleted before the write: %v", err)
	}

	wantStatusError(
		t,
		keycloak.UpdateAccount(t.Context(), "locked", account),
		"update user",
		http.StatusConflict,
	)
}
