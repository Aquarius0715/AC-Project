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
