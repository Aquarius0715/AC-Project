package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// OIDCVerifier verifies RS256 access tokens from Cognito (production) or Keycloak (local): issuer, audience
// (one of the app clients), expiry, the token type and the signature against the JWKS (container design §4:
// OIDC_ISSUER, OIDC_JWKS_URL, OIDC_APP_CLIENT_ID, OIDC_ADMIN_CLIENT_ID).
type OIDCVerifier struct {
	Issuer      string
	Audiences   []string
	AdminClient string // HQ's app client (OIDC_ADMIN_CLIENT_ID): its tokens alone carry HQ's memberships (IR510)
	Keyfunc     jwt.Keyfunc
}

// Verify implements Verifier and returns the `sub` claim with the `auth_time` claim, which Cognito and Keycloak put
// in access tokens (IR268).
func (v *OIDCVerifier) Verify(_ context.Context, token string) (Identity, error) {
	t, err := jwt.Parse(token, v.Keyfunc,
		jwt.WithIssuer(v.Issuer), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return Identity{}, err
	}
	claims := t.Claims.(jwt.MapClaims)
	if !audienceOK(claims, v.Audiences) {
		return Identity{}, errors.New("audience")
	}
	if !accessToken(claims) {
		return Identity{}, errors.New("not an access token")
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Identity{}, errors.New("no subject")
	}
	client, _ := claims["azp"].(string) // Keycloak; Cognito's access tokens name it client_id
	if client == "" {
		client, _ = claims["client_id"].(string)
	}
	return Identity{Subject: sub, SignedIn: authTime(claims), Client: client, AdminApp: client != "" && client == v.AdminClient}, nil
}

// authTime reads the `auth_time` claim (seconds since the epoch); zero when it is missing or not a number.
func authTime(c jwt.MapClaims) time.Time {
	var sec float64
	switch v := c["auth_time"].(type) {
	case float64:
		sec = v
	case json.Number:
		sec, _ = v.Float64()
	}
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(sec), 0).UTC()
}

// accessToken reports whether the claims are an access token's: Cognito's say token_use=access, Keycloak's typ=Bearer.
// The ID token of the same client has the same issuer, audience and signature (token_use=id, typ=ID), so without this
// check it would pass as an access token (backend Go design §4 step 6: token_use=access, IR367).
func accessToken(c jwt.MapClaims) bool {
	if use, ok := c["token_use"]; ok {
		return use == "access"
	}
	typ, _ := c["typ"].(string)
	return strings.EqualFold(typ, "Bearer")
}

// audienceOK accepts `aud` or Keycloak/Cognito `azp` / `client_id` matching an allowed client.
func audienceOK(c jwt.MapClaims, allowed []string) bool {
	var got []string
	if aud, err := c.GetAudience(); err == nil {
		got = append(got, aud...)
	}
	for _, k := range []string{"azp", "client_id"} {
		if s, ok := c[k].(string); ok {
			got = append(got, s)
		}
	}
	for _, a := range allowed {
		for _, g := range got {
			if a != "" && a == g {
				return true
			}
		}
	}
	return false
}

// NewVerifierFromEnv builds the OIDC verifier from the environment.
func NewVerifierFromEnv(ctx context.Context) (Verifier, error) {
	iss := os.Getenv("OIDC_ISSUER")
	jwks := os.Getenv("OIDC_JWKS_URL")
	if iss == "" {
		return nil, errors.New("OIDC_ISSUER is required")
	}
	if jwks == "" {
		jwks = strings.TrimRight(iss, "/") + "/protocol/openid-connect/certs"
	}
	// keys of a token with an unknown key ID are fetched again at most every 10 s (the library default is 5 min):
	// a service that started before the identity provider, or one whose provider was recreated with new keys,
	// accepts tokens again within seconds instead of refusing every request for minutes (CI e2e, IR419)
	k, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{jwks}, keyfunc.Override{
		RefreshUnknownKID: rate.NewLimiter(rate.Every(10*time.Second), 1),
		RefreshInterval:   15 * time.Minute,
		RateLimitWaitMax:  15 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	admin := os.Getenv("OIDC_ADMIN_CLIENT_ID")
	return &OIDCVerifier{Issuer: iss, Audiences: []string{os.Getenv("OIDC_APP_CLIENT_ID"), admin}, AdminClient: admin, Keyfunc: k.Keyfunc}, nil
}
