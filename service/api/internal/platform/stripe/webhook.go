// Package stripe verifies Stripe webhook deliveries (DEC-68). Card data never reaches the platform: payments are taken
// on Stripe Checkout, and the result arrives as a signed webhook that is the only thing that confirms a payment.
package stripe

import (
	"errors"
	"os"
	"time"

	sdk "github.com/stripe/stripe-go/v85"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// WebhookTolerance is how old a signed delivery may be before it is refused (backend-go-design: 5 minutes).
const WebhookTolerance = 5 * time.Minute

// errWebhookSecret refuses a key without its webhook secret.
var errWebhookSecret = errors.New(
	"stripe: STRIPE_WEBHOOK_SECRET is required with STRIPE_SECRET_KEY",
)

// Config is the Stripe configuration of a process, read from the environment at start-up.
type Config struct {
	// SecretKey is the restricted API key; empty means the demo payment provider is used instead.
	SecretKey string
	// WebhookSecret is the signing secret of the webhook endpoint (whsec_...).
	WebhookSecret string
	// APIBase overrides the API host (stripe-mock in tests); empty means api.stripe.com.
	APIBase string
}

// ConfigFromEnv reads STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET and STRIPE_API_BASE.
func ConfigFromEnv() Config {
	return Config{
		SecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		APIBase:       os.Getenv("STRIPE_API_BASE"),
	}
}

// Enabled reports whether Stripe Checkout is configured for this process.
func (c Config) Enabled() bool {
	return c.SecretKey != ""
}

// Validate refuses a configuration that is partly set: a key without its webhook secret would take payments that
// can never be confirmed.
func (c Config) Validate() error {
	if c.SecretKey != "" && c.WebhookSecret == "" {
		return errWebhookSecret
	}

	return nil
}

// VerifyEvent checks the Stripe-Signature header against the raw body and returns the event. A body whose signature
// does not match, or whose signed timestamp is older than WebhookTolerance, is refused as a validation error.
func VerifyEvent(payload []byte, signature, secret string) (sdk.Event, error) {
	event, err := sdk.ConstructEvent(
		payload,
		signature,
		secret,
		sdk.WithTolerance(WebhookTolerance),
		// the signature is still checked. An event comes in the API version of its endpoint, or of the account for the
		// Stripe CLI, which moves on after this SDK's; ParseDelivery reads only fields every version keeps (id, type,
		// object, status, amount, currency, metadata), so another version is no reason to refuse a signed delivery
		sdk.WithIgnoreAPIVersionMismatch(),
	)
	if err != nil {
		return sdk.Event{}, apperr.E(apperr.Validation, apperr.KeyInvalid)
	}

	return event, nil
}
