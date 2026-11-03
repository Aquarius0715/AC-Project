// Package idp ends a user's sessions at the identity provider when HQ signs the user out everywhere (IR386). The
// Core API refuses every earlier sign-in by itself (IR384); ending the provider's sessions also revokes the refresh
// tokens and the single sign-on, so the next sign-in asks for the password without the BFF's prompt=login (IR385).
// Locally that is Keycloak's admin API with a service-account client; production calls Cognito's
// AdminUserGlobalSignOut in the same place.
package idp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

const (
	// clientTimeout bounds each call to the identity provider.
	clientTimeout = 5 * time.Second
	// renewEarlySeconds is how long before its expiry a cached admin token is renewed.
	renewEarlySeconds = 30
)

// statusError is an answer of the identity provider with a status the call does not expect.
type statusError struct {
	what string
	code int
}

func (e *statusError) Error() string { return fmt.Sprintf("idp: %s answered %d", e.what, e.code) }

var (
	errNoAccessToken  = errors.New("idp: no access token")
	errNoSubject      = errors.New("idp: no subject")
	errNoEmail        = errors.New("idp: no e-mail")
	errNoUserLocation = errors.New("idp: no user location")
	errUserNotFound   = errors.New("idp: the e-mail's user was not found")
)

// Keycloak ends sessions through the admin REST API of one realm.
type Keycloak struct {
	TokenURL     string // the realm's token endpoint, e.g. http://keycloak:8080/realms/ac/protocol/openid-connect/token
	AdminURL     string // the realm's admin API, e.g. http://keycloak:8080/admin/realms/ac
	ClientID     string // a confidential client whose service account holds realm-management manage-users
	ClientSecret string
	Client       *http.Client
	Now          func() time.Time

	mu    sync.Mutex
	token string
	until time.Time
}

// EndSessions logs the user out of every session in the realm: their refresh tokens and single sign-on end. A user
// the realm does not know has nothing to end. The subject is the user's ID in the realm (the token's sub).
func (k *Keycloak) EndSessions(ctx context.Context, subject string) error {
	if subject == "" {
		return errNoSubject
	}

	tok, err := k.accessToken(ctx)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		k.AdminURL+"/users/"+url.PathEscape(subject)+"/logout",
		nil,
	)
	if err != nil {
		return apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set("Authorization", "Bearer "+tok)

	res, err := k.client().Do(req)
	if err != nil {
		return apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized: // the cached token was ended early: fetch a new one next time
		k.mu.Lock()
		k.token = ""
		k.mu.Unlock()
	}

	return &statusError{what: "logout", code: res.StatusCode}
}

// Account is a person's sign-in account to provision (IR461): the provider's user carries the tenant, membership and
// role claims the apps read at sign-in.
type Account struct {
	Email       string
	DisplayName string
	TenantID    string
	Membership  string
	Role        string
	// RedirectURI and ClientID are where the password set-up e-mail's link returns: the app the person signs in to
	RedirectURI string
	ClientID    string
}

// ErrNoAccount is UpdateAccount's answer for a subject the provider does not know (an account deleted there).
var ErrNoAccount = errors.New("idp: no such account")

// claims are the attributes the apps read at sign-in (IR461).
func (a Account) claims() map[string][]string {
	return map[string][]string{
		"tenant_id":     {a.TenantID},
		"membership_id": {a.Membership},
		"role":          {a.Role},
	}
}

// CreateAccount creates the person's user (enabled, e-mail not yet verified) with the claim attributes and returns
// its subject. An account that exists for the e-mail already is updated with the attributes and returned, so a
// retried provisioning is one account.
func (k *Keycloak) CreateAccount(ctx context.Context, account Account) (string, error) {
	if account.Email == "" {
		return "", errNoEmail
	}

	user := map[string]any{
		"username":      account.Email,
		"email":         account.Email,
		"firstName":     account.DisplayName,
		"enabled":       true,
		"emailVerified": false,
		"attributes":    account.claims(),
	}
	if account.Role == "admin" { // HQ signs in with two steps (IR468)
		user["requiredActions"] = []string{"CONFIGURE_TOTP"}
	}

	res, err := k.do(ctx, http.MethodPost, "/users", user)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusCreated:
		loc := res.Header.Get("Location")
		if i := strings.LastIndex(loc, "/"); i >= 0 && i < len(loc)-1 {
			return loc[i+1:], nil
		}

		return "", errNoUserLocation
	case http.StatusConflict:
		id, err := k.findByEmail(ctx, account.Email)
		if err != nil {
			return "", err
		}

		return id, k.UpdateAccount(ctx, id, account)
	}

	return "", &statusError{what: "create user", code: res.StatusCode}
}

// UpdateAccount points the person's account at another membership (a person added again, IR462): the claim
// attributes change, the name is the one given, and the account is on. The whole user is read and written back: with
// the realm's user profile an update replaces the profile — a field left out, the e-mail too, would be cleared — and
// the attribute map (IR463).
func (k *Keycloak) UpdateAccount(ctx context.Context, subject string, account Account) error {
	user, err := k.readUser(ctx, subject)
	if err != nil {
		return err
	}

	attrs, _ := user["attributes"].(map[string]any)
	if attrs == nil {
		attrs = map[string]any{}
	}

	for key, v := range account.claims() {
		attrs[key] = v
	}

	user["attributes"], user["firstName"], user["enabled"] = attrs, account.DisplayName, true

	return k.writeUser(ctx, subject, user)
}

// SendSetup e-mails the person a link to set their password (and verify the e-mail) that returns to their app. An HQ
// account (role admin) without a one-time-code device also sets one up there: HQ signs in with two steps, and the
// realm asks for the code only once a device is configured (IR468).
func (k *Keycloak) SendSetup(ctx context.Context, subject string, account Account) error {
	actions := []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}

	if account.Role == "admin" {
		has, err := k.hasOTP(ctx, subject)
		if err != nil {
			return err
		}

		if !has {
			actions = append(actions, "CONFIGURE_TOTP")
		}
	}

	params := url.Values{
		"client_id":    {account.ClientID},
		"redirect_uri": {account.RedirectURI},
		"lifespan":     {"259200"},
	} // 3 days

	res, err := k.do(
		ctx,
		http.MethodPut,
		"/users/"+url.PathEscape(subject)+"/execute-actions-email?"+params.Encode(),
		actions,
	)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return &statusError{what: "set-up e-mail", code: res.StatusCode}
	}

	return nil
}

// SetEnabled turns the person's sign-in on or off (a removed or disabled user); a user the realm does not know has
// nothing to turn off.
func (k *Keycloak) SetEnabled(ctx context.Context, subject string, enabled bool) error {
	res, err := k.do(
		ctx,
		http.MethodPut,
		"/users/"+url.PathEscape(subject),
		map[string]any{"enabled": enabled},
	)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	}

	return &statusError{what: "enable", code: res.StatusCode}
}

func (k *Keycloak) client() *http.Client {
	if k.Client != nil {
		return k.Client
	}

	return &http.Client{Timeout: clientTimeout, Transport: nil, CheckRedirect: nil, Jar: nil}
}

func (k *Keycloak) now() time.Time {
	if k.Now != nil {
		return k.Now()
	}

	return time.Now()
}

// accessToken is the service account's token (client credentials), kept until 30 seconds before it expires.
func (k *Keycloak) accessToken(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.token != "" && k.now().Before(k.until) {
		return k.token, nil
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {k.ClientID},
		"client_secret": {k.ClientSecret},
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		k.TokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := k.client().Do(req)
	if err != nil {
		return "", apperr.Wrap(err, "http.Client.Do")
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", &statusError{what: "token endpoint", code: res.StatusCode}
	}

	var granted struct {
		AccessToken string `json:"access_token"` //nolint:tagliatelle // the OAuth 2.0 token response's name (RFC 6749 §5.1)
		ExpiresIn   int    `json:"expires_in"`   //nolint:tagliatelle // the OAuth 2.0 token response's name (RFC 6749 §5.1)
	}

	decodeErr := json.NewDecoder(res.Body).Decode(&granted)
	if decodeErr != nil || granted.AccessToken == "" {
		return "", errNoAccessToken
	}

	k.token, k.until = granted.AccessToken, k.now().
		Add(time.Duration(granted.ExpiresIn-renewEarlySeconds)*time.Second)

	return k.token, nil
}

// do sends one admin API request with the service account's token; a 401 clears the cached token.
func (k *Keycloak) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	tok, err := k.accessToken(ctx)
	if err != nil {
		return nil, err
	}

	var reader *strings.Reader

	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, apperr.Wrap(err, "json.Marshal")
		}

		reader = strings.NewReader(string(raw))
	} else {
		reader = strings.NewReader("")
	}

	req, err := http.NewRequestWithContext(ctx, method, k.AdminURL+path, reader)
	if err != nil {
		return nil, apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set("Authorization", "Bearer "+tok)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := k.client().Do(req)
	if err == nil && res.StatusCode == http.StatusUnauthorized {
		k.mu.Lock()
		k.token = ""
		k.mu.Unlock()
	}

	return res, apperr.Wrap(err, "http.Client.Do")
}

func (k *Keycloak) findByEmail(ctx context.Context, email string) (string, error) {
	res, err := k.do(ctx, http.MethodGet, "/users?exact=true&email="+url.QueryEscape(email), nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", &statusError{what: "user search", code: res.StatusCode}
	}

	var users []struct {
		ID string `json:"id"`
	}

	decodeErr := json.NewDecoder(res.Body).Decode(&users)
	if decodeErr != nil || len(users) != 1 {
		return "", errUserNotFound
	}

	return users[0].ID, nil
}

// hasOTP reports whether the user has a one-time-code device.
func (k *Keycloak) hasOTP(ctx context.Context, subject string) (bool, error) {
	res, err := k.do(ctx, http.MethodGet, "/users/"+url.PathEscape(subject)+"/credentials", nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return false, &statusError{what: "credentials", code: res.StatusCode}
	}

	var creds []struct {
		Type string `json:"type"`
	}

	decodeErr := json.NewDecoder(res.Body).Decode(&creds)
	if decodeErr != nil {
		return false, apperr.Wrap(decodeErr, "json.Decoder.Decode")
	}

	for _, c := range creds {
		if c.Type == "otp" {
			return true, nil
		}
	}

	return false, nil
}

// readUser reads the realm's user as a whole (the admin API drops what a write leaves out, IR463).
func (k *Keycloak) readUser(ctx context.Context, subject string) (map[string]any, error) {
	res, err := k.do(ctx, http.MethodGet, "/users/"+url.PathEscape(subject), nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, ErrNoAccount
	default:
		return nil, &statusError{what: "read user", code: res.StatusCode}
	}

	var user map[string]any

	decodeErr := json.NewDecoder(res.Body).Decode(&user)
	if decodeErr != nil {
		return nil, apperr.Wrap(decodeErr, "json.Decoder.Decode")
	}

	return user, nil
}

// writeUser puts the realm's user back as a whole.
func (k *Keycloak) writeUser(ctx context.Context, subject string, user map[string]any) error {
	upd, err := k.do(ctx, http.MethodPut, "/users/"+url.PathEscape(subject), user)
	if err != nil {
		return err
	}
	defer func() { _ = upd.Body.Close() }()

	switch upd.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrNoAccount
	}

	return &statusError{what: "update user", code: upd.StatusCode}
}
