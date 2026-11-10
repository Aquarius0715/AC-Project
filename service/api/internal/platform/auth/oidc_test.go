package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"maps"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/auth"
)

// testIssuer is the local Keycloak realm the test tokens name.
const testIssuer = "http://localhost:8081/realms/ac"

// tailnetIssuer is the same realm on a second address of the deployment.
const tailnetIssuer = "http://100.64.0.10:8281/realms/ac"

// newTestVerifier returns a verifier of the customer and HQ apps that trusts key.
func newTestVerifier(key *rsa.PrivateKey) *auth.OIDCVerifier {
	return &auth.OIDCVerifier{
		Issuers: []string{testIssuer}, Audiences: []string{appClient, adminClient},
		Keyfunc: func(*jwt.Token) (any, error) { return &key.PublicKey, nil }, AdminClient: "",
	}
}

// newKey generates an RSA signing key.
func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	return key
}

// sign signs the claims with the key (RS256).
func sign(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}

	return token
}

// accessClaims are the claims of a valid Keycloak access token of the customer app.
func accessClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":        testIssuer,
		claimSubject: "user-1",
		claimAZP:     appClient,
		"typ":        "Bearer",
		"exp":        time.Now().Add(time.Hour).Unix(),
	}
}

func TestOIDCVerifier(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	oidcVerifier := newTestVerifier(key)

	identity, err := oidcVerifier.Verify(
		t.Context(),
		sign(t, key, accessClaims()),
	)
	if err != nil || identity.Subject != "user-1" || !identity.SignedIn.IsZero() ||
		identity.Client != appClient ||
		identity.AdminApp {
		t.Fatalf("valid token: %+v %v", identity, err)
	}
	// IR510: the token names its app; HQ's app is the admin client (Keycloak azp, Cognito client_id)
	oidcVerifier.AdminClient = adminClient

	for name, mapClaims := range map[string]jwt.MapClaims{
		claimAZP:    {claimAZP: adminClient},
		"client_id": {"client_id": adminClient, "token_use": "access"},
	} {
		claims := accessClaims()
		delete(claims, claimAZP)

		maps.Copy(claims, mapClaims)

		hqIdentity, err := oidcVerifier.Verify(
			t.Context(),
			sign(t, key, claims),
		)
		if err != nil || hqIdentity.Client != adminClient ||
			!hqIdentity.AdminApp {
			t.Errorf("HQ's app by %s: %+v %v", name, hqIdentity, err)
		}
	}
}

func TestOIDCVerifierSignInTime(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	oidcVerifier := newTestVerifier(key)
	signedIn := time.Date(
		2026,
		9,
		14,
		0,
		55,
		0,
		0,
		time.UTC,
	) // IR268: auth_time is the sign-in time
	claims := accessClaims()

	claims["auth_time"] = signedIn.Unix()

	identity, err := oidcVerifier.Verify(
		t.Context(),
		sign(t, key, claims),
	)
	if err != nil ||
		!identity.SignedIn.Equal(signedIn) {
		t.Fatalf("auth_time: %+v %v", identity, err)
	}

	claims["auth_time"] = "yesterday"

	identity, err = oidcVerifier.Verify(
		t.Context(),
		sign(t, key, claims),
	)
	if err != nil ||
		!identity.SignedIn.IsZero() {
		t.Fatalf("a non-numeric auth_time gives no sign-in time: %+v %v", identity, err)
	}
}

func TestOIDCVerifierAccepts(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	oidcVerifier := newTestVerifier(key)

	claims := accessClaims()
	delete(claims, claimAZP)

	claims["aud"] = adminClient

	_, err := oidcVerifier.Verify(t.Context(), sign(t, key, claims))
	if err != nil {
		t.Fatalf("aud claim: %v", err)
	}

	claims = accessClaims() // Cognito: an access token says token_use=access and has no typ
	delete(claims, "typ")

	claims["token_use"] = "access"

	_, err = oidcVerifier.Verify(t.Context(), sign(t, key, claims))
	if err != nil {
		t.Fatalf("Cognito access token: %v", err)
	}
}

// refusedClaims are the claims a verifier must refuse, by what is wrong with them.
func refusedClaims() map[string]jwt.MapClaims {
	bad := map[string]jwt.MapClaims{}
	claims := accessClaims()
	claims["iss"] = "evil"
	bad["issuer"] = claims
	claims = accessClaims()
	claims["exp"] = time.Now().Add(-time.Minute).Unix()
	bad["expired"] = claims
	claims = accessClaims()
	delete(claims, "exp")
	bad["no exp"] = claims
	claims = accessClaims()
	claims[claimAZP] = "other"
	bad["audience"] = claims
	claims = accessClaims()
	delete(claims, claimSubject)
	bad["no sub"] = claims
	// IR367: only access tokens; the ID token of the same client has the same issuer, audience and key
	claims = accessClaims()
	claims["typ"] = "ID"
	bad["Keycloak ID token"] = claims
	claims = accessClaims()
	delete(claims, "typ")
	claims["token_use"] = "id"
	bad["Cognito ID token"] = claims
	claims = accessClaims()
	claims["token_use"] = "id" // token_use decides when both are present
	bad["token_use id with typ Bearer"] = claims
	claims = accessClaims()
	claims["typ"] = "Refresh"
	bad["Keycloak refresh token"] = claims
	claims = accessClaims()
	delete(claims, "typ")

	bad["no token type"] = claims

	return bad
}

func TestOIDCVerifierRefuses(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	oidcVerifier := newTestVerifier(key)

	for name, claims := range refusedClaims() {
		_, err := oidcVerifier.Verify(t.Context(), sign(t, key, claims))
		if err == nil {
			t.Errorf("%s must fail", name)
		}
	}

	_, err := oidcVerifier.Verify(t.Context(), sign(t, newKey(t), accessClaims()))
	if err == nil {
		t.Error("wrong key must fail")
	}

	hs, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims()).
		SignedString([]byte("secret"))

	_, err = oidcVerifier.Verify(t.Context(), hs)
	if err == nil {
		t.Error("HS256 must be rejected")
	}
}

func TestNewVerifierFromEnv(t *testing.T) {
	t.Setenv("OIDC_ISSUER", "")

	_, err := auth.NewVerifierFromEnv(context.Background())
	if err == nil {
		t.Fatal("issuer required")
	}

	t.Setenv("OIDC_ISSUER", "http://127.0.0.1:1/realms/ac")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	v, err := auth.NewVerifierFromEnv(
		ctx,
	) // keyfunc starts with an unreachable JWKS and refreshes in the background
	if err == nil && v == nil {
		t.Fatal("verifier")
	}
}

// TestOIDCVerifierSeveralIssuers: when the identity provider answers on several addresses, the tokens of a sign-in
// name the address it started on, so the verifier takes each listed issuer and no other (IR651).
func TestOIDCVerifierSeveralIssuers(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	verifier := newTestVerifier(key)
	verifier.Issuers = []string{testIssuer, tailnetIssuer}

	for issuer, want := range map[string]bool{
		testIssuer:                      true,
		tailnetIssuer:                   true,
		"http://evil.example/realms/ac": false,
		"":                              false,
	} {
		claims := accessClaims()
		claims["iss"] = issuer

		_, err := verifier.Verify(t.Context(), sign(t, key, claims))
		if (err == nil) != want {
			t.Errorf("issuer %q: accepted %v, want %v (%v)", issuer, err == nil, want, err)
		}
	}
}

// TestNewVerifierFromEnvListsIssuers: OIDC_ISSUER may name several issuers, comma separated, with blanks around them.
func TestNewVerifierFromEnvListsIssuers(t *testing.T) {
	t.Setenv("OIDC_ISSUER", " http://a.example/realms/ac , http://b.example/realms/ac ,")
	t.Setenv("OIDC_JWKS_URL", "http://127.0.0.1:1/certs")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	v, err := auth.NewVerifierFromEnv(ctx)
	if err != nil {
		t.Skip(err)
	}

	if len(v.Issuers) != 2 || v.Issuers[0] != "http://a.example/realms/ac" ||
		v.Issuers[1] != "http://b.example/realms/ac" {
		t.Errorf("issuers %q", v.Issuers)
	}
}

// TestOIDCVerifierNamesTheIssuer: the verified issuer is part of the identity: it says which address the holder signed
// in on, and being checked against the list it cannot be made up.
func TestOIDCVerifierNamesTheIssuer(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	verifier := newTestVerifier(key)
	verifier.Issuers = []string{testIssuer, tailnetIssuer}

	claims := accessClaims()
	claims["iss"] = tailnetIssuer

	identity, err := verifier.Verify(t.Context(), sign(t, key, claims))
	if err != nil {
		t.Fatal(err)
	}

	if identity.Issuer != tailnetIssuer {
		t.Errorf("issuer %q, want %q", identity.Issuer, tailnetIssuer)
	}
}
