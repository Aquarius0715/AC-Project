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

func (k *Keycloak) client() *http.Client {
	if k.Client != nil {
		return k.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
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
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {k.ClientID}, "client_secret": {k.ClientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := k.client().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("idp: token endpoint answered %d", res.StatusCode)
	}
	var t struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&t); err != nil || t.AccessToken == "" {
		return "", errors.New("idp: no access token")
	}
	k.token, k.until = t.AccessToken, k.now().Add(time.Duration(t.ExpiresIn-30)*time.Second)
	return k.token, nil
}

// EndSessions logs the user out of every session in the realm: their refresh tokens and single sign-on end. A user
// the realm does not know has nothing to end. The subject is the user's ID in the realm (the token's sub).
func (k *Keycloak) EndSessions(ctx context.Context, subject string) error {
	if subject == "" {
		return errors.New("idp: no subject")
	}
	tok, err := k.accessToken(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.AdminURL+"/users/"+url.PathEscape(subject)+"/logout", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := k.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	case http.StatusUnauthorized: // the cached token was ended early: fetch a new one next time
		k.mu.Lock()
		k.token = ""
		k.mu.Unlock()
	}
	return fmt.Errorf("idp: logout answered %d", res.StatusCode)
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

// do sends one admin API request with the service account's token; a 401 clears the cached token.
func (k *Keycloak) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	tok, err := k.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var rd *strings.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = strings.NewReader(string(raw))
	} else {
		rd = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, k.AdminURL+path, rd)
	if err != nil {
		return nil, err
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
	return res, err
}

// ErrNoAccount is UpdateAccount's answer for a subject the provider does not know (an account deleted there).
var ErrNoAccount = errors.New("idp: no such account")

// claims are the attributes the apps read at sign-in (IR461).
func (a Account) claims() map[string][]string {
	return map[string][]string{"tenant_id": {a.TenantID}, "membership_id": {a.Membership}, "role": {a.Role}}
}

// CreateAccount creates the person's user (enabled, e-mail not yet verified) with the claim attributes and returns
// its subject. An account that exists for the e-mail already is updated with the attributes and returned, so a
// retried provisioning is one account.
func (k *Keycloak) CreateAccount(ctx context.Context, a Account) (string, error) {
	if a.Email == "" {
		return "", errors.New("idp: no e-mail")
	}
	user := map[string]any{"username": a.Email, "email": a.Email, "firstName": a.DisplayName, "enabled": true, "emailVerified": false, "attributes": a.claims()}
	if a.Role == "admin" { // HQ signs in with two steps (IR468)
		user["requiredActions"] = []string{"CONFIGURE_TOTP"}
	}
	res, err := k.do(ctx, http.MethodPost, "/users", user)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusCreated:
		loc := res.Header.Get("Location")
		if i := strings.LastIndex(loc, "/"); i >= 0 && i < len(loc)-1 {
			return loc[i+1:], nil
		}
		return "", errors.New("idp: no user location")
	case http.StatusConflict:
		id, err := k.findByEmail(ctx, a.Email)
		if err != nil {
			return "", err
		}
		return id, k.UpdateAccount(ctx, id, a)
	}
	return "", fmt.Errorf("idp: create user answered %d", res.StatusCode)
}

// UpdateAccount points the person's account at another membership (a person added again, IR462): the claim
// attributes change, the name is the one given, and the account is on. The whole user is read and written back: with
// the realm's user profile an update replaces the profile — a field left out, the e-mail too, would be cleared — and
// the attribute map (IR463).
func (k *Keycloak) UpdateAccount(ctx context.Context, subject string, a Account) error {
	res, err := k.do(ctx, http.MethodGet, "/users/"+url.PathEscape(subject), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ErrNoAccount
	default:
		return fmt.Errorf("idp: read user answered %d", res.StatusCode)
	}
	var user map[string]any
	if err := json.NewDecoder(res.Body).Decode(&user); err != nil {
		return err
	}
	attrs, _ := user["attributes"].(map[string]any)
	if attrs == nil {
		attrs = map[string]any{}
	}
	for key, v := range a.claims() {
		attrs[key] = v
	}
	user["attributes"], user["firstName"], user["enabled"] = attrs, a.DisplayName, true
	upd, err := k.do(ctx, http.MethodPut, "/users/"+url.PathEscape(subject), user)
	if err != nil {
		return err
	}
	defer upd.Body.Close()
	switch upd.StatusCode {
	case http.StatusNoContent, http.StatusOK:
		return nil
	case http.StatusNotFound:
		return ErrNoAccount
	}
	return fmt.Errorf("idp: update user answered %d", upd.StatusCode)
}

func (k *Keycloak) findByEmail(ctx context.Context, email string) (string, error) {
	res, err := k.do(ctx, http.MethodGet, "/users?exact=true&email="+url.QueryEscape(email), nil)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("idp: user search answered %d", res.StatusCode)
	}
	var users []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&users); err != nil || len(users) != 1 {
		return "", errors.New("idp: the e-mail's user was not found")
	}
	return users[0].ID, nil
}

// SendSetup e-mails the person a link to set their password (and verify the e-mail) that returns to their app. An HQ
// account (role admin) without a one-time-code device also sets one up there: HQ signs in with two steps, and the
// realm asks for the code only once a device is configured (IR468).
func (k *Keycloak) SendSetup(ctx context.Context, subject string, a Account) error {
	actions := []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}
	if a.Role == "admin" {
		has, err := k.hasOTP(ctx, subject)
		if err != nil {
			return err
		}
		if !has {
			actions = append(actions, "CONFIGURE_TOTP")
		}
	}
	q := url.Values{"client_id": {a.ClientID}, "redirect_uri": {a.RedirectURI}, "lifespan": {"259200"}} // 3 days
	res, err := k.do(ctx, http.MethodPut, "/users/"+url.PathEscape(subject)+"/execute-actions-email?"+q.Encode(), actions)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		return fmt.Errorf("idp: set-up e-mail answered %d", res.StatusCode)
	}
	return nil
}

// hasOTP reports whether the user has a one-time-code device.
func (k *Keycloak) hasOTP(ctx context.Context, subject string) (bool, error) {
	res, err := k.do(ctx, http.MethodGet, "/users/"+url.PathEscape(subject)+"/credentials", nil)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("idp: credentials answered %d", res.StatusCode)
	}
	var creds []struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(res.Body).Decode(&creds); err != nil {
		return false, err
	}
	for _, c := range creds {
		if c.Type == "otp" {
			return true, nil
		}
	}
	return false, nil
}

// SetEnabled turns the person's sign-in on or off (a removed or disabled user); a user the realm does not know has
// nothing to turn off.
func (k *Keycloak) SetEnabled(ctx context.Context, subject string, enabled bool) error {
	res, err := k.do(ctx, http.MethodPut, "/users/"+url.PathEscape(subject), map[string]any{"enabled": enabled})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNoContent, http.StatusOK, http.StatusNotFound:
		return nil
	}
	return fmt.Errorf("idp: enable answered %d", res.StatusCode)
}
