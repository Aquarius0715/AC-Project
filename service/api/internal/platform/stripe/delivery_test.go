package stripe_test

import (
	"encoding/json"
	"reflect"
	"testing"

	sdk "github.com/stripe/stripe-go/v85"

	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

// eventWith returns an event whose data object is raw.
func eventWith(eventType, raw string) sdk.Event {
	event := new(sdk.Event)
	event.ID = eventID
	event.Type = sdk.EventType(eventType)
	event.Livemode = true

	if raw != "" {
		event.Data = new(sdk.EventData)
		event.Data.Raw = json.RawMessage(raw)
	}

	return *event
}

func TestParseDeliveryCheckoutSession(t *testing.T) {
	t.Parallel()

	raw := `{"id":"cs_1","object":"checkout.session","payment_intent":"pi_1","payment_status":"paid",` +
		`"status":"complete","amount_total":500,"currency":"myr","metadata":{"invoice_id":"` + invoiceID + `"}}`

	delivery, err := stripe.ParseDelivery(
		eventWith("checkout.session.completed", raw),
		[]byte("body"),
	)
	if err != nil {
		t.Fatalf("ParseDelivery: %v", err)
	}

	want := stripe.Delivery{
		Raw:             []byte("body"),
		ID:              eventID,
		Type:            "checkout.session.completed",
		Livemode:        true,
		SessionID:       "cs_1",
		PaymentIntentID: "pi_1",
		PaymentStatus:   "paid",
		AmountTotal:     500,
		Currency:        "myr",
		Metadata:        map[string]string{"invoice_id": invoiceID},
	}
	if !reflect.DeepEqual(delivery, want) {
		t.Fatalf("delivery = %+v, want %+v", delivery, want)
	}
}

// TestParseDeliveryPaymentIntent: an intent's status and amount stand in for the session's payment_status and total.
func TestParseDeliveryPaymentIntent(t *testing.T) {
	t.Parallel()

	raw := `{"id":"pi_1","object":"payment_intent","status":"requires_payment_method","amount":700,` +
		`"amount_total":0,"currency":"myr","metadata":{"payment_id":"pay-1"}}`

	delivery, err := stripe.ParseDelivery(eventWith("payment_intent.payment_failed", raw), nil)
	if err != nil {
		t.Fatalf("ParseDelivery: %v", err)
	}

	if delivery.SessionID != "" || delivery.PaymentIntentID != "pi_1" ||
		delivery.PaymentStatus != "requires_payment_method" || delivery.AmountTotal != 700 ||
		delivery.Metadata["payment_id"] != "pay-1" {
		t.Fatalf("delivery = %+v", delivery)
	}
}

func TestParseDeliveryRefusesMissingOrBadObject(t *testing.T) {
	t.Parallel()

	for name, raw := range map[string]string{"no data": "", "bad JSON": `[1,2]`} {
		_, err := stripe.ParseDelivery(eventWith("checkout.session.completed", raw), nil)
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
