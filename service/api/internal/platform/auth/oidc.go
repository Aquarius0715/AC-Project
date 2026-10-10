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
)

// OIDCVerifier verifies RS256 access tokens from Cognito (production) or Keycloak (local): issuer, audience
// (one of the app clients), expiry and signature against the JWKS (container design §4: OIDC_ISSUER, OIDC_JWKS_URL,
// OIDC_APP_CLIENT_ID, OIDC_ADMIN_CLIENT_ID).
type OIDCVerifier struct {
	Issuer    string
	Audiences []string
	Keyfunc   jwt.Keyfunc
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
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return Identity{}, errors.New("no subject")
	}
	return Identity{Subject: sub, SignedIn: authTime(claims)}, nil
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
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwks})
	if err != nil {
		return nil, fmt.Errorf("jwks: %w", err)
	}
	return &OIDCVerifier{Issuer: iss, Audiences: []string{os.Getenv("OIDC_APP_CLIENT_ID"), os.Getenv("OIDC_ADMIN_CLIENT_ID")}, Keyfunc: k.Keyfunc}, nil
}
