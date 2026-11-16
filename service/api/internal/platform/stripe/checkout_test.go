package stripe_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/stripe"
)

const (
	sessionPath = "/v1/checkout/sessions/cs_test_1"
	expirePath  = sessionPath + "/expire"
)

// fakeStripe answers the Checkout Session endpoints the client calls and records each request it got.
type fakeStripe struct {
	mu       sync.Mutex
	paths    []string
	form     map[string]string
	idemKey  string
	status   string
	expireOK bool
	// refuseCreate answers the create with a 400
	refuseCreate bool
}

func (f *fakeStripe) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.paths = append(f.paths, r.Method+" "+r.URL.Path)

	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions" && !f.refuseCreate:
		err := r.ParseForm()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		f.form = map[string]string{}
		for key := range r.PostForm {
			f.form[key] = r.PostForm.Get(key)
		}

		f.idemKey = r.Header.Get("Idempotency-Key")
		_, _ = w.Write(
			[]byte(
				`{"id":"cs_test_1","object":"checkout.session","url":"https://checkout.example/cs_test_1"}`,
			),
		)
	case r.Method == http.MethodGet && r.URL.Path == sessionPath:
		_, _ = w.Write(
			[]byte(`{"id":"cs_test_1","object":"checkout.session","status":"` + f.status + `"}`),
		)
	case r.Method == http.MethodPost && r.URL.Path == expirePath && f.expireOK:
		_, _ = w.Write([]byte(`{"id":"cs_test_1","object":"checkout.session","status":"expired"}`))
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(
			[]byte(`{"error":{"type":"invalid_request_error","message":"refused by the fake"}}`),
		)
	}
}

func (f *fakeStripe) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.paths...)
}

// newCheckout starts fake and returns a client that talks to it.
func newCheckout(t *testing.T, fake *fakeStripe) *stripe.Checkout {
	t.Helper()

	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	client, err := stripe.NewCheckout(stripe.Config{
		SecretKey:     "sk_test_fake",
		WebhookSecret: testSecret,
		APIBase:       srv.URL,
	})
	if err != nil {
		t.Fatalf("NewCheckout: %v", err)
	}

	return client
}

func TestNewCheckoutWithoutKeyIsNotConfigured(t *testing.T) {
	t.Parallel()

	_, err := stripe.NewCheckout(stripe.Config{SecretKey: "", WebhookSecret: "", APIBase: ""})
	if !errors.Is(err, stripe.ErrNotConfigured) {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// TestCreateSessionSendsOneLine: the amount, the lowercase currency, the metadata on both the session and its intent,
// Managed Payments off and the idempotency key all reach Stripe.
func TestCreateSessionSendsOneLine(t *testing.T) {
	t.Parallel()

	fake := new(fakeStripe)
	client := newCheckout(t, fake)

	req := new(stripe.SessionRequest)
	req.IdempotencyKey = "idem-1"
	req.AmountMinor = 12345
	req.Currency = "MYR"
	req.Description = "Invoice INV-1"
	req.Metadata = map[string]string{"invoice_id": invoiceID}
	req.SuccessURL = "https://app.example/ok"
	req.CancelURL = "https://app.example/cancel"

	session, err := client.CreateSession(t.Context(), *req)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if session.ID != "cs_test_1" || session.URL != "https://checkout.example/cs_test_1" {
		t.Fatalf("session = %+v", session)
	}

	want := map[string]string{
		"mode":                                          "payment",
		"line_items[0][quantity]":                       "1",
		"line_items[0][price_data][currency]":           "myr",
		"line_items[0][price_data][unit_amount]":        "12345",
		"line_items[0][price_data][product_data][name]": "Invoice INV-1",
		"metadata[invoice_id]":                          invoiceID,
		"payment_intent_data[metadata][invoice_id]":     invoiceID,
		"managed_payments[enabled]":                     "false",
		"success_url":                                   "https://app.example/ok",
		"cancel_url":                                    "https://app.example/cancel",
	}
	for key, value := range want {
		if fake.form[key] != value {
			t.Errorf("form[%s] = %q, want %q", key, fake.form[key], value)
		}
	}

	if fake.idemKey != "idem-1" {
		t.Errorf("Idempotency-Key = %q, want idem-1", fake.idemKey)
	}
}

func TestCreateSessionRefusedIsAnError(t *testing.T) {
	t.Parallel()

	fake := new(fakeStripe)
	fake.refuseCreate = true
	client := newCheckout(t, fake)

	req := new(stripe.SessionRequest)
	req.Currency = "MYR"

	_, err := client.CreateSession(t.Context(), *req)
	if err == nil || !strings.Contains(err.Error(), "create checkout session") {
		t.Fatalf("err = %v, want a wrapped create error", err)
	}
}

// TestReleaseSessionRetrieveRefused: a session Stripe does not know is an error, not a release.
func TestReleaseSessionRetrieveRefused(t *testing.T) {
	t.Parallel()

	client := newCheckout(t, new(fakeStripe))

	released, err := client.ReleaseSession(t.Context(), "cs_other")
	if err == nil || released || !strings.Contains(err.Error(), "retrieve checkout session") {
		t.Fatalf("released = %v, err = %v, want a wrapped retrieve error", released, err)
	}
}

func TestReleaseSession(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   string
		expireOK bool
		want     bool
		wantErr  bool
		calls    []string
	}{
		{
			name: "expired already", status: "expired", expireOK: false, want: true, wantErr: false,
			calls: []string{"GET " + sessionPath},
		},
		{
			name: "open is expired", status: "open", expireOK: true, want: true, wantErr: false,
			calls: []string{"GET " + sessionPath, "POST " + expirePath},
		},
		{
			name:     "complete stays",
			status:   "complete",
			expireOK: false,
			want:     false,
			wantErr:  false,
			calls:    []string{"GET " + sessionPath},
		},
		{
			name: "expire refused", status: "open", expireOK: false, want: false, wantErr: true,
			calls: []string{"GET " + sessionPath, "POST " + expirePath},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fake := new(fakeStripe)
			fake.status = testCase.status
			fake.expireOK = testCase.expireOK
			client := newCheckout(t, fake)

			released, err := client.ReleaseSession(t.Context(), "cs_test_1")
			if (err != nil) != testCase.wantErr {
				t.Fatalf("err = %v, want error %v", err, testCase.wantErr)
			}

			if released != testCase.want {
				t.Fatalf("released = %v, want %v", released, testCase.want)
			}

			got := fake.calls()
			if strings.Join(got, ",") != strings.Join(testCase.calls, ",") {
				t.Fatalf("calls = %v, want %v", got, testCase.calls)
			}
		})
	}
}
