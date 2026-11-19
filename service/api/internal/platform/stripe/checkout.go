package stripe

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/stripe/stripe-go/v85"
)

// ErrNotConfigured is returned by NewCheckout when no secret key is set.
var ErrNotConfigured = errors.New("stripe: STRIPE_SECRET_KEY is not set")

// SessionRequest is one payment taken on a hosted Checkout page: a single line in the currency's minor unit.
type SessionRequest struct {
	// IdempotencyKey is the write's Idempotency-Key; Stripe answers a repeated request with the same session.
	IdempotencyKey string
	AmountMinor    int64
	// Currency is ISO 4217 in any case; Stripe takes it lowercase.
	Currency    string
	Description string
	// Metadata is set on the session and on its payment intent, so a webhook can find the invoice and the payment.
	Metadata   map[string]string
	SuccessURL string
	CancelURL  string
}

// Session is the part of a Checkout Session the platform keeps: its ID and the page the payer is sent to.
type Session struct {
	ID  string
	URL string
}

// Checkout creates Checkout Sessions with the restricted API key (DEC-68). Card data is entered on Stripe's page only.
type Checkout struct {
	client *sdk.Client
}

// NewCheckout builds the client; APIBase, when set, replaces api.stripe.com (stripe-mock in tests).
func NewCheckout(cfg Config) (*Checkout, error) {
	if cfg.SecretKey == "" {
		return nil, ErrNotConfigured
	}

	var opts []sdk.ClientOption

	if cfg.APIBase != "" {
		// the SDK's configuration struct is set field by field: the fields not named keep the SDK's defaults
		backendCfg := new(sdk.BackendConfig)
		backendCfg.URL = sdk.String(cfg.APIBase)
		opts = append(opts, sdk.WithBackends(sdk.NewBackendsWithConfig(backendCfg)))
	}

	return &Checkout{client: sdk.NewClient(cfg.SecretKey, opts...)}, nil
}

// CreateSession creates a payment-mode Checkout Session for one invoice amount.
func (c *Checkout) CreateSession(ctx context.Context, req SessionRequest) (Session, error) {
	// the SDK's parameter structs are large: each is set field by field, so the request names only what it sends
	params := new(sdk.CheckoutSessionCreateParams)
	params.Mode = sdk.String(string(sdk.CheckoutSessionModePayment))
	params.SuccessURL = sdk.String(req.SuccessURL)
	params.CancelURL = sdk.String(req.CancelURL)
	params.Metadata = req.Metadata

	product := new(sdk.CheckoutSessionCreateLineItemPriceDataProductDataParams)
	product.Name = sdk.String(req.Description)

	price := new(sdk.CheckoutSessionCreateLineItemPriceDataParams)
	price.Currency = sdk.String(strings.ToLower(req.Currency))
	price.UnitAmount = new(req.AmountMinor)
	price.ProductData = product

	item := new(sdk.CheckoutSessionCreateLineItemParams)
	item.Quantity = sdk.Int64(1)
	item.PriceData = price
	params.LineItems = []*sdk.CheckoutSessionCreateLineItemParams{item}

	// the platform is the seller of its own invoices (DEC-68): Stripe's Managed Payments (Stripe as merchant of record,
	// on by default for new accounts) would refuse a line without a product tax code, so it is turned off here
	params.ManagedPayments = new(sdk.CheckoutSessionCreateManagedPaymentsParams)
	params.ManagedPayments.Enabled = new(false)

	params.PaymentIntentData = new(sdk.CheckoutSessionCreatePaymentIntentDataParams)
	params.PaymentIntentData.Metadata = req.Metadata
	params.SetIdempotencyKey(req.IdempotencyKey)

	session, err := c.client.V1CheckoutSessions.Create(ctx, params)
	if err != nil {
		return Session{}, fmt.Errorf("stripe: create checkout session: %w", err)
	}

	return Session{ID: session.ID, URL: session.URL}, nil
}

// ReleaseSession ends a Checkout Session the payer left, so that a new one can take its place. It reports true when
// the session can no longer take a payment: it was open and is now expired, or it had expired already. A completed
// session is not released (false): its payment is made, and only its webhook may settle it.
func (c *Checkout) ReleaseSession(ctx context.Context, id string) (bool, error) {
	session, err := c.client.V1CheckoutSessions.Retrieve(ctx, id, nil)
	if err != nil {
		return false, fmt.Errorf("stripe: retrieve checkout session: %w", err)
	}

	switch session.Status {
	case sdk.CheckoutSessionStatusExpired:
		return true, nil
	case sdk.CheckoutSessionStatusOpen:
		_, err = c.client.V1CheckoutSessions.Expire(ctx, id, nil)
		if err != nil {
			return false, fmt.Errorf("stripe: expire checkout session: %w", err)
		}

		return true, nil
	case sdk.CheckoutSessionStatusComplete:
		// its payment is made: the session stays, and only its webhook settles the payment
	}

	return false, nil
}
