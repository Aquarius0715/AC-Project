package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCVerifier(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	v := &OIDCVerifier{Issuer: "http://localhost:8081/realms/ac", Audiences: []string{"ac-web", "ac-admin-web"},
		Keyfunc: func(*jwt.Token) (any, error) { return &key.PublicKey, nil }}
	sign := func(k *rsa.PrivateKey, c jwt.MapClaims) string {
		s, _ := jwt.NewWithClaims(jwt.SigningMethodRS256, c).SignedString(k)
		return s
	}
	base := func() jwt.MapClaims {
		return jwt.MapClaims{"iss": v.Issuer, "sub": "user-1", "azp": "ac-web", "typ": "Bearer", "exp": time.Now().Add(time.Hour).Unix()}
	}
	if id, err := v.Verify(context.Background(), sign(key, base())); err != nil || id.Subject != "user-1" || !id.SignedIn.IsZero() {
		t.Fatalf("valid token: %+v %v", id, err)
	}
	signedIn := time.Date(2026, 9, 14, 0, 55, 0, 0, time.UTC) // IR268: auth_time is the sign-in time
	c := base()
	c["auth_time"] = signedIn.Unix()
	if id, err := v.Verify(context.Background(), sign(key, c)); err != nil || !id.SignedIn.Equal(signedIn) {
		t.Fatalf("auth_time: %+v %v", id, err)
	}
	c["auth_time"] = "yesterday"
	if id, err := v.Verify(context.Background(), sign(key, c)); err != nil || !id.SignedIn.IsZero() {
		t.Fatalf("a non-numeric auth_time gives no sign-in time: %+v %v", id, err)
	}
	c = base()
	delete(c, "azp")
	c["aud"] = "ac-admin-web"
	if _, err := v.Verify(context.Background(), sign(key, c)); err != nil {
		t.Fatalf("aud claim: %v", err)
	}
	c = base() // Cognito: an access token says token_use=access and has no typ
	delete(c, "typ")
	c["token_use"] = "access"
	if _, err := v.Verify(context.Background(), sign(key, c)); err != nil {
		t.Fatalf("Cognito access token: %v", err)
	}
	bad := map[string]jwt.MapClaims{}
	c = base()
	c["iss"] = "evil"
	bad["issuer"] = c
	c = base()
	c["exp"] = time.Now().Add(-time.Minute).Unix()
	bad["expired"] = c
	c = base()
	delete(c, "exp")
	bad["no exp"] = c
	c = base()
	c["azp"] = "other"
	bad["audience"] = c
	c = base()
	delete(c, "sub")
	bad["no sub"] = c
	// IR367: only access tokens; the ID token of the same client has the same issuer, audience and key
	c = base()
	c["typ"] = "ID"
	bad["Keycloak ID token"] = c
	c = base()
	delete(c, "typ")
	c["token_use"] = "id"
	bad["Cognito ID token"] = c
	c = base()
	c["token_use"] = "id" // token_use decides when both are present
	bad["token_use id with typ Bearer"] = c
	c = base()
	c["typ"] = "Refresh"
	bad["Keycloak refresh token"] = c
	c = base()
	delete(c, "typ")
	bad["no token type"] = c
	for name, cl := range bad {
		if _, err := v.Verify(context.Background(), sign(key, cl)); err == nil {
			t.Errorf("%s must fail", name)
		}
	}
	if _, err := v.Verify(context.Background(), sign(other, base())); err == nil {
		t.Error("wrong key must fail")
	}
	hs, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, base()).SignedString([]byte("secret"))
	if _, err := v.Verify(context.Background(), hs); err == nil {
		t.Error("HS256 must be rejected")
	}
}

func TestNewVerifierFromEnv(t *testing.T) {
	t.Setenv("OIDC_ISSUER", "")
	if _, err := NewVerifierFromEnv(context.Background()); err == nil {
		t.Fatal("issuer required")
	}
	t.Setenv("OIDC_ISSUER", "http://127.0.0.1:1/realms/ac")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	v, err := NewVerifierFromEnv(ctx) // keyfunc starts with an unreachable JWKS and refreshes in the background
	if err == nil && v == nil {
		t.Fatal("verifier")
	}
}
