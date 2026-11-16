package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// clusterSkip explains why the two provider tests do not run against the split services: the cluster is cached for
// the whole binary, so a test's own fake provider cannot reach it. The monolith run covers them.
const clusterSkip = "the cluster caches its servers for the binary; the monolith run covers this test"

// errStripeUnreachable is what the fake answers while it is made to fail.
var errStripeUnreachable = errors.New("stripe: unreachable")

// fakeCheckout is the Stripe Checkout provider of the tests: it records every session request and can be made to fail.
// Each session gets its own ID, as Stripe's do: the database keeps them unique.
type fakeCheckout struct {
	mu       sync.Mutex
	requests []stripe.SessionRequest
	sessions []string
	// completed holds the sessions the payer finished on Stripe's page; released the ones ended by ReleaseSession
	completed map[string]bool
	released  []string
	fail      bool
}

func newFakeCheckout() *fakeCheckout {
	return &fakeCheckout{
		mu:        sync.Mutex{},
		requests:  nil,
		sessions:  nil,
		completed: map[string]bool{},
		released:  nil,
		fail:      false,
	}
}

func (f *fakeCheckout) CreateSession(
	_ context.Context,
	req stripe.SessionRequest,
) (stripe.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests = append(f.requests, req)
	if f.fail {
		return stripe.Session{}, errStripeUnreachable
	}

	id := "cs_test_" + uuid.NewString()
	f.sessions = append(f.sessions, id)

	return stripe.Session{ID: id, URL: "https://checkout.stripe.test/pay/" + id}, nil
}

// ReleaseSession ends an open session, as Stripe's expire does; a completed one is not released.
func (f *fakeCheckout) ReleaseSession(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fail {
		return false, errStripeUnreachable
	}

	if f.completed[id] {
		return false, nil
	}

	f.released = append(f.released, id)

	return true, nil
}

// complete marks a session as finished by the payer on Stripe's page.
func (f *fakeCheckout) complete(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.completed[id] = true
}

// releasedSessions is the sessions ReleaseSession ended, oldest first.
func (f *fakeCheckout) releasedSessions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.released...)
}

// lastSession is the ID of the newest session the provider created.
func (f *fakeCheckout) lastSession(t *testing.T) string {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.sessions) == 0 {
		t.Fatal("no Checkout Session was created")
	}

	return f.sessions[len(f.sessions)-1]
}

func (f *fakeCheckout) last(t *testing.T) stripe.SessionRequest {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.requests) == 0 {
		t.Fatal("no Checkout Session was created")
	}

	return f.requests[len(f.requests)-1]
}

func (f *fakeCheckout) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.requests)
}

func (f *fakeCheckout) setFail(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.fail = v
}

func checkoutServer(t *testing.T, provider *fakeCheckout) *apiserver.Server {
	t.Helper()

	return serverCfg(t, func(c *apiserver.Config) {
		c.Checkout = provider
		c.CustomerAppURL = "https://customer.test"
	})
}

// invoiceVersion reads the invoice's version (the expected version of payments.startCheckout).
func invoiceVersion(t *testing.T, srv *apiserver.Server, inv string) int {
	t.Helper()

	_, res := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	return ver(t, res)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartCheckoutCreatesSessionForInvoice(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := checkoutServer(t, provider)
	_, inv := newInvoice(t, srv)
	body := `{"invoiceId":"` + inv + `"}`

	code, res := write(
		srv,
		customerB(),
		"payments.startCheckout",
		body,
		invoiceVersion(t, srv, inv),
	)
	if code != 200 {
		t.Fatalf("startCheckout: %d %v", code, res)
	}

	pay := asStr(t, data(res)["paymentId"])
	url := asStr(t, data(res)["checkoutUrl"])

	if !strings.HasPrefix(url, "https://checkout.stripe.test/pay/cs_test_") {
		t.Fatalf("checkoutUrl = %q", url)
	}

	req := provider.last(t)
	if req.AmountMinor != 5000 || req.Currency != "MYR" {
		t.Fatalf("session amount %d %s, want 5000 MYR", req.AmountMinor, req.Currency)
	}

	if req.Metadata["payment_id"] != pay || req.Metadata["invoice_id"] != inv {
		t.Fatalf(
			"session metadata %v does not name payment %s and invoice %s",
			req.Metadata,
			pay,
			inv,
		)
	}

	if req.IdempotencyKey == "" {
		t.Fatal("session request carries no Idempotency-Key")
	}

	// the payer comes back to the invoice's own page, which shows the state the webhook reached
	if req.SuccessURL != "https://customer.test/customer/payments/"+inv+"?checkout=return" ||
		req.CancelURL != "https://customer.test/customer/payments/"+inv+"?checkout=cancelled" {
		t.Fatalf(
			"return pages %q / %q do not lead back to the invoice",
			req.SuccessURL,
			req.CancelURL,
		)
	}

	// the payment is in flight: the invoice shows it, and nothing is paid yet
	_, res = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
	if data(res)["paymentStatus"] != statusInitiated || data(res)["status"] != statusUnpaid {
		t.Fatalf("invoice after start: %v", res)
	}
}

// TestStartCheckoutAgainReplacesALeftSession: a payer who left Stripe's page (cancelled or closed it) starts again
// at once. The earlier session is ended at Stripe and its payment expires, so only one attempt can take money.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartCheckoutAgainReplacesALeftSession(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := checkoutServer(t, provider)
	_, inv := newInvoice(t, srv)
	body := `{"invoiceId":"` + inv + `"}`

	code, res := write(
		srv,
		customerB(),
		"payments.startCheckout",
		body,
		invoiceVersion(t, srv, inv),
	)
	if code != 200 {
		t.Fatalf("first start: %d %v", code, res)
	}

	first := provider.lastSession(t)

	code, res = write(srv, customerB(), "payments.startCheckout", body, invoiceVersion(t, srv, inv))
	if code != 200 {
		t.Fatalf("start again: %d %v", code, res)
	}

	if got := provider.releasedSessions(); len(got) != 1 || got[0] != first {
		t.Fatalf("released sessions %v, want the first one %s", got, first)
	}

	if provider.count() != 2 {
		t.Fatalf("%d sessions created, want 2", provider.count())
	}

	_, res = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)

	statuses := map[string]int{}

	refs, ok := data(res)["paymentRefs"].([]any)
	if !ok {
		t.Fatalf("invoice has no paymentRefs: %v", res)
	}

	for _, ref := range refs {
		payment, isMap := ref.(map[string]any)
		if !isMap {
			t.Fatalf("payment ref %v", ref)
		}

		statuses[asStr(t, payment["status"])]++
	}

	if statuses["expired"] != 1 || statuses["initiated"] != 1 {
		t.Fatalf("payments %v, want one expired and one initiated", statuses)
	}

	// a session the payer completed is not replaced: its webhook settles it, and a new start is refused
	provider.complete(provider.lastSession(t))

	code, res = write(srv, customerB(), "payments.startCheckout", body, invoiceVersion(t, srv, inv))
	if code != 409 || res["messageKey"] != keyInvoiceHasPayment {
		t.Fatalf("start after completion: %d %v, want 409 errors.invoice_has_payment", code, res)
	}

	if provider.count() != 2 {
		t.Fatalf("%d sessions created, want still 2", provider.count())
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartCheckoutRefusals(t *testing.T) {
	if clusterMode() {
		t.Skip(clusterSkip)
	}

	provider := newFakeCheckout()
	srv := checkoutServer(t, provider)
	_, inv := newInvoice(t, srv)
	body := `{"invoiceId":"` + inv + `"}`
	version := invoiceVersion(t, srv, inv)

	// the invoice's expected version is required
	if code, res := write(srv, customerB(), "payments.startCheckout", body, 0); code != 422 {
		t.Fatalf("no version: %d %v, want 422", code, res)
	}

	// a stale version is a conflict, and creates nothing
	if code, res := write(
		srv,
		customerB(),
		"payments.startCheckout",
		body,
		version+5,
	); code != 409 {
		t.Fatalf("stale version: %d %v, want 409", code, res)
	}

	// another customer's invoice is not theirs to pay (D01: not found, not forbidden)
	if code, res := write(srv, customerA(), "payments.startCheckout", body, version); code != 404 {
		t.Fatalf("foreign invoice: %d %v, want 404", code, res)
	}

	// Stripe unreachable: the attempt is refused and leaves no payment in flight, so the same intent can be retried
	provider.setFail(true)

	code, res := write(srv, customerB(), "payments.startCheckout", body, version)
	if code != 503 || res["messageKey"] != keyUnavailable {
		t.Fatalf("provider down: %d %v, want 503 error.unavailable", code, res)
	}

	_, res = post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
	if data(res)["paymentStatus"] != nil {
		t.Fatalf("a refused start left a payment: %v", res)
	}

	provider.setFail(false)

	if code, res = write(srv, customerB(), "payments.startCheckout", body, version); code != 200 {
		t.Fatalf("retry after recovery: %d %v", code, res)
	}

	// the invoice is named
	code, res = write(srv, customerB(), "payments.startCheckout", `{}`, version)
	refusal(t, "no invoiceId", code, res, "invoiceId=error.required")

	// Stripe unreachable while the payer's left session is open: it cannot be ended, so the start is refused and the
	// open attempt stays as it was
	provider.setFail(true)

	code, res = write(srv, customerB(), "payments.startCheckout", body, invoiceVersion(t, srv, inv))
	if code != 503 || res["messageKey"] != keyUnavailable {
		t.Fatalf("release while provider down: %d %v, want 503 error.unavailable", code, res)
	}

	provider.setFail(false)

	if _, paymentStatus := invoiceState(t, srv, inv); paymentStatus != statusInitiated {
		t.Fatalf("after a refused release: payment %s, want initiated", paymentStatus)
	}

	if got := provider.releasedSessions(); len(got) != 0 {
		t.Fatalf("released sessions %v, want none", got)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartCheckoutNeedsStripeConfigured(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)
	body := `{"invoiceId":"` + inv + `"}`

	code, res := write(
		srv,
		customerB(),
		"payments.startCheckout",
		body,
		invoiceVersion(t, srv, inv),
	)
	if code != 503 || res["messageKey"] != keyUnavailable {
		t.Fatalf("no provider: %d %v, want 503 error.unavailable", code, res)
	}
}
