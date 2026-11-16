package stripe

import (
	"encoding/json"
	"errors"

	sdk "github.com/stripe/stripe-go/v85"
)

// errNoObject refuses an event that carries no resource to read.
var errNoObject = errors.New("stripe: event has no object")

// Delivery is what billing reads from a verified event: the event itself, the Checkout Session or PaymentIntent it
// is about, and the metadata written at checkout (tenant_id, invoice_id, payment_id).
type Delivery struct {
	// Raw is the signed body as received; billing keeps it as evidence (billing.stripe_events.payload).
	Raw      []byte
	ID       string
	Type     string
	Livemode bool
	// SessionID is set for checkout.session.* events; PaymentIntentID for payment_intent.* and for a session's intent.
	SessionID       string
	PaymentIntentID string
	// PaymentStatus is the session's payment_status (paid, unpaid) or the intent's status (succeeded, ...).
	PaymentStatus string
	// AmountTotal and Currency are the amount Stripe charged, in the currency's minor unit, lowercase currency.
	AmountTotal int64
	Currency    string
	Metadata    map[string]string
}

// deliveryObject is the resource shape of the events billing handles: a Checkout Session or a PaymentIntent.
type deliveryObject struct {
	ID            string            `json:"id"`
	Object        string            `json:"object"`
	PaymentIntent *string           `json:"payment_intent"` //nolint:tagliatelle // Stripe's wire name is snake_case
	PaymentStatus string            `json:"payment_status"` //nolint:tagliatelle // Stripe's wire name is snake_case
	Status        string            `json:"status"`
	AmountTotal   int64             `json:"amount_total"` //nolint:tagliatelle // Stripe's wire name is snake_case
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Metadata      map[string]string `json:"metadata"`
}

// ParseDelivery reads the resource of a verified event. raw is the body the event was verified from.
func ParseDelivery(event sdk.Event, raw []byte) (Delivery, error) {
	if event.Data == nil || len(event.Data.Raw) == 0 {
		return Delivery{}, errNoObject
	}

	var obj deliveryObject

	err := json.Unmarshal(event.Data.Raw, &obj)
	if err != nil {
		return Delivery{}, errNoObject
	}

	delivery := Delivery{
		Raw:             raw,
		ID:              event.ID,
		Type:            string(event.Type),
		Livemode:        event.Livemode,
		SessionID:       "",
		PaymentIntentID: "",
		PaymentStatus:   obj.PaymentStatus,
		AmountTotal:     obj.AmountTotal,
		Currency:        obj.Currency,
		Metadata:        obj.Metadata,
	}

	switch obj.Object {
	case "checkout.session":
		delivery.SessionID = obj.ID
		if obj.PaymentIntent != nil {
			delivery.PaymentIntentID = *obj.PaymentIntent
		}
	case "payment_intent":
		delivery.PaymentIntentID = obj.ID
		delivery.PaymentStatus = obj.Status
		delivery.AmountTotal = obj.Amount
	}

	return delivery, nil
}
