package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdk "github.com/stripe/stripe-go/v85"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const (
	webhookSecret  = "whsec_integration_only" //nolint:gosec // G101: a signing secret of a test fixture, not a credential
	statusPaid     = "paid"
	statusExpire   = "expired"
	eventCompleted = "checkout.session.completed"
	sessionUnpaid  = "unpaid" // the payment_status of a completed session that settles later
)

// webhookServer is a server with the Stripe provider and the webhook secret of the tests.
func webhookServer(t *testing.T, provider *fakeCheckout) *apiserver.Server {
	t.Helper()

	return serverCfg(t, func(c *apiserver.Config) {
		c.Checkout = provider
		c.CustomerAppURL = "https://customer.test"
		c.StripeWebhookSecret = webhookSecret
	})
}

// deliver posts one signed Stripe event to the webhook and returns the status it answers.
func deliver(t *testing.T, srv *apiserver.Server, secret, eventID, eventType, object string) int {
	t.Helper()

	body := `{"id":"` + eventID + `","object":"event","api_version":"` + sdk.APIVersion + `","type":"` + eventType +
		`","livemode":false,"data":{"object":` + object + `}}`
	signed := sdk.GenerateTestSignedPayload(&sdk.UnsignedPayload{
		Payload:   []byte(body),
		Secret:    secret,
		Timestamp: time.Now(),
		Scheme:    "",
	})

	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/stripe",
		strings.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", signed.Header)

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	return rec.Code
}

// sessionObject is a Checkout Session as Stripe sends it for a payment of the invoice.
func sessionObject(sessionID, status string, amount int64, inv, pay string) string {
	return `{"id":"` + sessionID + `","object":"checkout.session","payment_status":"` + status + `",` +
		`"payment_intent":"pi_` + eventSalt + `","amount_total":` + strconv.FormatInt(
		amount,
		10,
	) + `,"currency":"myr",` +
		`"metadata":{"tenant_id":"` + seed.ID("tenant-a").
		String() +
		`","invoice_id":"` + inv + `","payment_id":"` +
		pay + `"}}`
}

// invoiceState reads the invoice's status and its latest payment's status.
func invoiceState(t *testing.T, srv *apiserver.Server, inv string) (string, string) {
	t.Helper()

	_, res := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	return asStr(t, data(res)["status"]), asStr(t, data(res)["paymentStatus"])
}

// startedPayment starts a Checkout payment for the invoice as the customer; it returns the payment's ID and the
// Checkout Session the provider created for it.
func startedPayment(
	t *testing.T,
	srv *apiserver.Server,
	provider *fakeCheckout,
	inv string,
) (string, string) {
	t.Helper()

	code, res := write(
		srv,
		customerB(),
		"payments.startCheckout",
		`{"invoiceId":"`+inv+`"}`,
		invoiceVersion(t, srv, inv),
	)
	if code != 200 {
		t.Fatalf("startCheckout: %d %v", code, res)
	}

	return asStr(t, data(res)["paymentId"]), provider.lastSession(t)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookConfirmsPaymentOnce(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)

	completed := sessionObject(session, statusPaid, 5000, inv, pay)
	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_confirm_1"),
		"checkout.session.completed",
		completed,
	); code != 200 {
		t.Fatalf("completed: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status != statusPaid || paymentStatus != statusConfirmed {
		t.Fatalf(
			"after completed: invoice %s, payment %s; want paid, confirmed",
			status,
			paymentStatus,
		)
	}

	// the same event again, and the intent's success for the same payment: nothing changes, both answer 2xx
	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_confirm_1"),
		"checkout.session.completed",
		completed,
	); code != 200 {
		t.Fatalf("replay: %d", code)
	}

	intent := `{"id":"pi_` + eventSalt + `","object":"payment_intent","status":"succeeded",` +
		`"amount":5000,"currency":"myr",` +
		`"metadata":{"tenant_id":"` + seed.ID("tenant-a").
		String() +
		`","invoice_id":"` + inv + `","payment_id":"` +
		pay + `"}}`
	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_intent_1"),
		"payment_intent.succeeded",
		intent,
	); code != 200 {
		t.Fatalf("intent succeeded: %d", code)
	}

	status, paymentStatus = invoiceState(t, srv, inv)
	if status != statusPaid || paymentStatus != statusConfirmed {
		t.Fatalf(
			"after replays: invoice %s, payment %s; want paid, confirmed",
			status,
			paymentStatus,
		)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookRefusesWhatDoesNotMatch(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)

	// a forged signature is refused and changes nothing
	forged := sessionObject(session, statusPaid, 5000, inv, pay)
	if code := deliver(
		t,
		srv,
		"whsec_forged",
		eventID("evt_forged"),
		"checkout.session.completed",
		forged,
	); code != 422 {
		t.Fatalf("forged signature: %d, want 422", code)
	}

	// an amount other than the invoice's, and another session, are stored and change nothing
	if code := deliver(
		t, srv, webhookSecret, eventID("evt_amount"), "checkout.session.completed",
		sessionObject(session, statusPaid, 4999, inv, pay),
	); code != 200 {
		t.Fatalf("wrong amount: %d", code)
	}

	if code := deliver(
		t, srv, webhookSecret, eventID("evt_session"), "checkout.session.completed",
		sessionObject("cs_test_other", statusPaid, 5000, inv, pay),
	); code != 200 {
		t.Fatalf("other session: %d", code)
	}

	// a payment the service does not know is stored, not applied
	if code := deliver(
		t, srv, webhookSecret, eventID("evt_unknown"), "checkout.session.completed",
		sessionObject(session, statusPaid, 5000, inv, seed.ID("tenant-a").String()),
	); code != 200 {
		t.Fatalf("unknown payment: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status == statusPaid || paymentStatus == statusConfirmed {
		t.Fatalf("a refused delivery changed the invoice: %s, %s", status, paymentStatus)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookExpiryFreesTheInvoice(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)

	if code := deliver(
		t, srv, webhookSecret, eventID("evt_expired"), "checkout.session.expired",
		sessionObject(session, "unpaid", 5000, inv, pay),
	); code != 200 {
		t.Fatalf("expired: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status != statusUnpaid || paymentStatus != statusExpire {
		t.Fatalf(
			"after expiry: invoice %s, payment %s; want unpaid, expired",
			status,
			paymentStatus,
		)
	}

	// a new attempt starts after an expired one
	startedPayment(t, srv, provider, inv)
}

// TestStripeWebhookDeclinedCardKeepsTheSessionOpen: a declined card leaves the Checkout Session open and the payer
// retries on the same page, so payment_intent.payment_failed changes nothing and the retry's success pays the invoice
// (IR829, DEC-68).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookDeclinedCardKeepsTheSessionOpen(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)
	before, beforePayment := invoiceState(t, srv, inv)

	declined := `{"id":"pi_retry_` + eventSalt + `","object":"payment_intent","status":"requires_payment_method",` +
		`"amount":5000,"currency":"myr","metadata":{"tenant_id":"` + seed.ID("tenant-a").
		String() +
		`","invoice_id":"` + inv + `","payment_id":"` + pay + `"}}`
	if code := deliver(
		t, srv, webhookSecret, eventID("evt_declined"), "payment_intent.payment_failed", declined,
	); code != 200 {
		t.Fatalf("declined: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status != before || paymentStatus != beforePayment {
		t.Fatalf(
			"after decline: invoice %s, payment %s; want %s, %s (the session is still open)",
			status, paymentStatus, before, beforePayment,
		)
	}

	if code := deliver(
		t, srv, webhookSecret, eventID("evt_retry_paid"), "checkout.session.completed",
		strings.Replace(
			sessionObject(session, statusPaid, 5000, inv, pay),
			`"payment_intent":"pi_`, `"payment_intent":"pi_retry_`, 1,
		),
	); code != 200 {
		t.Fatalf("retry completed: %d", code)
	}

	status, paymentStatus = invoiceState(t, srv, inv)
	if status != statusPaid || paymentStatus != statusConfirmed {
		t.Fatalf(
			"after retry: invoice %s, payment %s; want paid, confirmed",
			status,
			paymentStatus,
		)
	}
}

// TestStripeWebhookSettlesABankPaymentLater: a session completed with a method that settles later (unpaid) makes
// the payment and the invoice processing, not paid; the session's async success then pays the invoice (DEC-68).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookSettlesABankPaymentLater(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)

	// this test's own payment intent: the database keeps intent IDs unique across the tests' confirmations
	object := func(status string) string {
		return strings.Replace(
			sessionObject(session, status, 5000, inv, pay),
			`"payment_intent":"pi_`, `"payment_intent":"pi_async_`, 1,
		)
	}

	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_async_completed"),
		"checkout.session.completed",
		object("unpaid"),
	); code != 200 {
		t.Fatalf("completed unpaid: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status != statusProcessing || paymentStatus != statusProcessing {
		t.Fatalf(
			"after completed unpaid: invoice %s, payment %s; want processing, processing",
			status,
			paymentStatus,
		)
	}

	// HQ cannot confirm it by hand: only Stripe's webhook confirms a Stripe payment
	code, res := write(
		srv,
		hq(),
		"payments.confirm",
		`{"paymentId":"`+pay+`","paymentReference":"BANK-HQ-`+eventSalt[:8]+`","confirmedAmountMinor":5000,`+
			`"currency":"MYR","reason":"seen in the bank"}`,
		paymentVersion(t, srv, inv, pay),
	)
	if code != 409 || res["messageKey"] != "error.invalidState" {
		t.Fatalf("HQ confirm of a Stripe payment: %d %v, want 409 error.invalidState", code, res)
	}

	// while Stripe processes it, the payer cannot start another attempt: the invoice is no longer unpaid
	code, res = write(
		srv,
		customerB(),
		"payments.startCheckout",
		`{"invoiceId":"`+inv+`"}`,
		invoiceVersion(t, srv, inv),
	)
	if code != 409 || res["messageKey"] != "error.invalidState" {
		t.Fatalf("start while processing: %d %v, want 409 error.invalidState", code, res)
	}

	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_async_succeeded"),
		"checkout.session.async_payment_succeeded",
		object(statusPaid),
	); code != 200 {
		t.Fatalf("async succeeded: %d", code)
	}

	status, paymentStatus = invoiceState(t, srv, inv)
	if status != statusPaid || paymentStatus != statusConfirmed {
		t.Fatalf(
			"after async success: invoice %s, payment %s; want paid, confirmed",
			status,
			paymentStatus,
		)
	}

	// the confirmed payment carries its PaymentIntent ID as its reference
	_, res = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	refs, ok := data(res)["paymentRefs"].([]any)
	if !ok || len(refs) != 1 {
		t.Fatalf("paymentRefs %v", data(res)["paymentRefs"])
	}

	if ref, isMap := refs[0].(map[string]any); !isMap ||
		ref["paymentReference"] != "pi_async_"+eventSalt ||
		ref["channel"] != "stripe_checkout" {
		t.Fatalf(
			"confirmed payment %v, want reference pi_async_%s on channel stripe_checkout",
			refs[0],
			eventSalt,
		)
	}
}

// TestStripeWebhookIgnoresWhatDoesNotSettleThePayment: a completed session whose amount or currency is not the
// invoice's, a paid session in another currency and an event type billing does not act on are stored and change
// neither the payment nor its version; a second completed-unpaid session on a payment already processing changes
// nothing either (DEC-68).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookIgnoresWhatDoesNotSettleThePayment(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)
	pay, session := startedPayment(t, srv, provider, inv)
	before, beforePayment := invoiceState(t, srv, inv)
	beforeVersion := paymentVersion(t, srv, inv, pay)

	inUSD := func(object string) string {
		return strings.Replace(object, `"currency":"myr"`, `"currency":"usd"`, 1)
	}

	ignored := []struct {
		name, eventType, object string
	}{
		{
			"evt_unpaid_amount",
			eventCompleted,
			sessionObject(session, sessionUnpaid, 4999, inv, pay),
		},
		{
			"evt_unpaid_currency",
			eventCompleted,
			inUSD(sessionObject(session, sessionUnpaid, 5000, inv, pay)),
		},
		{
			"evt_paid_currency",
			eventCompleted,
			inUSD(sessionObject(session, statusPaid, 5000, inv, pay)),
		},
		{"evt_refunded", "charge.refunded", sessionObject(session, statusPaid, 5000, inv, pay)},
	}

	for _, event := range ignored {
		code := deliver(t, srv, webhookSecret, eventID(event.name), event.eventType, event.object)
		if code != 200 {
			t.Fatalf("%s: %d, want 200", event.name, code)
		}

		status, paymentStatus := invoiceState(t, srv, inv)
		if status != before || paymentStatus != beforePayment ||
			paymentVersion(t, srv, inv, pay) != beforeVersion {
			t.Fatalf(
				"after %s: invoice %s, payment %s (version %d); want %s, %s (version %d)",
				event.name, status, paymentStatus, paymentVersion(t, srv, inv, pay),
				before, beforePayment, beforeVersion,
			)
		}
	}

	if code := deliver(
		t, srv, webhookSecret, eventID("evt_unpaid_first"), eventCompleted,
		sessionObject(session, sessionUnpaid, 5000, inv, pay),
	); code != 200 {
		t.Fatalf("completed unpaid: %d", code)
	}

	processing := paymentVersion(t, srv, inv, pay)

	// another delivery of the session's completion (a new event ID) finds the payment processing, not initiated
	if code := deliver(
		t, srv, webhookSecret, eventID("evt_unpaid_again"), eventCompleted,
		sessionObject(session, sessionUnpaid, 5000, inv, pay),
	); code != 200 {
		t.Fatalf("completed unpaid again: %d", code)
	}

	status, paymentStatus := invoiceState(t, srv, inv)
	if status != statusProcessing || paymentStatus != statusProcessing ||
		paymentVersion(t, srv, inv, pay) != processing {
		t.Fatalf(
			"after the second completion: invoice %s, payment %s (version %d); want processing, processing (version %d)",
			status,
			paymentStatus,
			paymentVersion(t, srv, inv, pay),
			processing,
		)
	}
}

// TestStripeWebhookAcceptsEventsOfNoCheckout: an event of the Stripe account that no checkout here started (no
// tenant in its metadata) is answered 2xx, so Stripe does not resend it, and changes nothing.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStripeWebhookAcceptsEventsOfNoCheckout(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	srv := webhookServer(t, newFakeCheckout())

	charge := `{"id":"ch_` + eventSalt + `","object":"charge","amount":100,"currency":"myr","metadata":{}}`
	if code := deliver(
		t,
		srv,
		webhookSecret,
		eventID("evt_foreign"),
		"charge.succeeded",
		charge,
	); code != 200 {
		t.Fatalf("event of no checkout: %d, want 200", code)
	}
}

// TestDemoPaymentEventsAreRefusedWithStripe: once Stripe is configured a customer cannot mark an invoice paid with a
// demo event; the invoice stays unpaid and only the webhook confirms (DEC-68).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestDemoPaymentEventsAreRefusedWithStripe(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := webhookServer(t, provider)
	_, inv := newInvoice(t, srv)

	code, res := write(
		srv,
		customerB(),
		"payments.simulate",
		`{"event":"initiate","invoiceId":"`+inv+`","method":"demo_credit_card","demoConfirmed":true}`,
		invoiceVersion(t, srv, inv),
	)
	if code != 403 || res["messageKey"] != "error.forbidden" {
		t.Fatalf("demo initiate with Stripe: %d %v, want 403 error.forbidden", code, res)
	}

	_, after := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
	if data(after)["status"] == statusPaid {
		t.Fatal("a refused demo event marked the invoice paid")
	}
}

// eventSalt makes the event and payment-intent IDs of this test process unique: the database keeps them unique, so a
// rerun on the same database must not replay an earlier run's events.
var eventSalt = uuid.NewString() //nolint:gochecknoglobals // one salt for the whole test binary

// eventID is the Stripe event ID the test process uses for name.
func eventID(name string) string {
	return name + "_" + eventSalt
}

// paymentVersion reads the version of one of the invoice's payments (the expected version of payments.confirm).
func paymentVersion(t *testing.T, srv *apiserver.Server, inv, pay string) int {
	t.Helper()

	_, res := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	refs, ok := data(res)["paymentRefs"].([]any)
	if !ok {
		t.Fatalf("paymentRefs %v", res)
	}

	for _, ref := range refs {
		if p, isMap := ref.(map[string]any); isMap && p["id"] == pay {
			return ver(t, map[string]any{"data": p})
		}
	}

	t.Fatalf("payment %s not on invoice %s", pay, inv)

	return 0
}
