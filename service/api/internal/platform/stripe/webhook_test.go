package stripe_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/stripe/stripe-go/v85"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

const (
	eventID    = "evt_1"
	invoiceID  = "inv-1"
	testSecret = "whsec_test_only" //nolint:gosec // G101: a signing secret of a test fixture, not a credential
	testBody   = `{"id":"evt_1","object":"event","api_version":"` + sdk.APIVersion +
		`","type":"checkout.session.completed","data":{"object":{}},"livemode":false}`
)

// sign returns the Stripe-Signature header of testBody signed with secret at the given time.
func sign(t *testing.T, secret string, signedAt time.Time) string {
	t.Helper()

	signed := sdk.GenerateTestSignedPayload(&sdk.UnsignedPayload{
		Payload:   []byte(testBody),
		Secret:    secret,
		Timestamp: signedAt,
		Scheme:    "",
	})

	return signed.Header
}

func TestVerifyEventAcceptsValidSignature(t *testing.T) {
	t.Parallel()

	header := sign(t, testSecret, time.Now())

	event, err := stripe.VerifyEvent([]byte(testBody), header, testSecret)
	if err != nil {
		t.Fatalf("VerifyEvent: %v", err)
	}

	if event.ID != eventID || event.Type != "checkout.session.completed" {
		t.Fatalf("event = %s %s, want evt_1 checkout.session.completed", event.ID, event.Type)
	}
}

// TestVerifyEventAcceptsAnotherAPIVersion: the Stripe CLI sends events in the account's API version, which can be
// newer than this SDK's; a signed delivery is not refused for that.
func TestVerifyEventAcceptsAnotherAPIVersion(t *testing.T) {
	t.Parallel()

	body := strings.Replace(testBody, sdk.APIVersion, "2026-10-28.endive", 1)
	signed := sdk.GenerateTestSignedPayload(&sdk.UnsignedPayload{
		Payload:   []byte(body),
		Secret:    testSecret,
		Timestamp: time.Now(),
		Scheme:    "",
	})

	event, err := stripe.VerifyEvent([]byte(body), signed.Header, testSecret)
	if err != nil {
		t.Fatalf("VerifyEvent: %v", err)
	}

	if event.ID != eventID {
		t.Fatalf("event = %s, want evt_1", event.ID)
	}
}

func TestVerifyEventRefusesWrongSecret(t *testing.T) {
	t.Parallel()

	header := sign(t, "whsec_other", time.Now())

	assertInvalid(t, testBody, header)
}

func TestVerifyEventRefusesTamperedBody(t *testing.T) {
	t.Parallel()

	header := sign(t, testSecret, time.Now())
	tampered := testBody + " "

	assertInvalid(t, tampered, header)
}

func TestVerifyEventRefusesOldTimestamp(t *testing.T) {
	t.Parallel()

	header := sign(t, testSecret, time.Now().Add(-stripe.WebhookTolerance-time.Minute))

	assertInvalid(t, testBody, header)
}

func TestVerifyEventRefusesMissingHeader(t *testing.T) {
	t.Parallel()

	assertInvalid(t, testBody, "")
}

func TestConfigValidateRequiresWebhookSecretWithKey(t *testing.T) {
	t.Parallel()

	keyOnly := stripe.Config{SecretKey: "sk_test_x", WebhookSecret: "", APIBase: ""}
	demo := stripe.Config{SecretKey: "", WebhookSecret: "", APIBase: ""}

	err := keyOnly.Validate()
	if err == nil {
		t.Fatal("key without webhook secret: want an error")
	}

	err = demo.Validate()
	if err != nil {
		t.Fatalf("demo mode: %v", err)
	}

	if demo.Enabled() {
		t.Fatal("empty config must not enable Stripe")
	}
}

// assertInvalid checks that a delivery is refused as a validation error, with the test's signing secret.
func assertInvalid(t *testing.T, body, header string) {
	t.Helper()

	_, err := stripe.VerifyEvent([]byte(body), header, testSecret)
	if err == nil {
		t.Fatal("VerifyEvent accepted a delivery it must refuse")
	}

	var domainErr *apperr.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != apperr.Validation {
		t.Fatalf("err = %v, want a VALIDATION domain error", err)
	}
}
