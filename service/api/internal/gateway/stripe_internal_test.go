package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TestStripeReachesBillingUnchanged: a Stripe delivery reaches the billing service with its raw body and its
// Stripe-Signature header as sent, since the signature covers both (DEC-68); no other service sees it.
func TestStripeReachesBillingUnchanged(t *testing.T) {
	t.Parallel()

	const (
		payload   = "{\"id\":\"evt_1\",\n  \"type\":\"checkout.session.completed\"}"
		signature = "t=1700000000,v1=abc123"
	)

	got := map[string]string{}
	app := New(upstreams(t, func(domain string) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			raw, _ := io.ReadAll(request.Body)
			got[domain] = request.Method + " " + request.URL.Path + " " + request.Header.Get(
				"Stripe-Signature",
			) + " " + string(raw)

			writer.WriteHeader(http.StatusOK)
		}
	}), nil)

	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/stripe",
		strings.NewReader(payload),
	)
	req.Header.Set("Stripe-Signature", signature)

	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	want := "POST /stripe " + signature + " " + payload
	if rec.Code != http.StatusOK || got[ops.DomainBilling] != want || len(got) != 1 {
		t.Fatalf("stripe → %d, upstreams saw %q, want only billing %q", rec.Code, got, want)
	}
}

// TestRouteOfServiceWithoutTarget: a catalog route whose domain has no service URL answers 404 UNKNOWN_OPERATION from
// the gateway instead of panicking on the missing proxy.
func TestRouteOfServiceWithoutTarget(t *testing.T) {
	t.Parallel()

	targets := upstreams(t, recordingUpstream(map[string]string{}))
	delete(targets, ops.DomainBilling)
	app := New(targets, nil)

	for _, path := range []string{"/stripe", ops.SpecByName()["invoices.create"].Routes[0].Path} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(
			rec,
			httptest.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				path,
				strings.NewReader(`{}`),
			),
		)

		if rec.Code != http.StatusNotFound ||
			!strings.Contains(rec.Body.String(), apperr.KeyUnknownOperation) {
			t.Fatalf("%s without billing → %d %s", path, rec.Code, rec.Body)
		}
	}
}
