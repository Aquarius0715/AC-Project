package ops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

type echoIn struct {
	N int `json:"n"`
}
type echoOut struct {
	Twice int `json:"twice"`
}

// TestAskRoutesQueries covers how an internal query runs (IR192): in the caller's process when it serves the owner's
// domain (a result of another type goes through JSON), UNAVAILABLE without a transport for another domain, local for
// a call without a registry, and the errors of a missing registry, an unknown name and an undecodable input.
func TestAskRoutesQueries(t *testing.T) {
	ctx := context.Background()
	r := NewRegistry()
	RegisterQuery(r, DomainEquipment, "test.twice", func(_ context.Context, _ *Call, in *echoIn) (map[string]int, error) {
		return map[string]int{"twice": in.N * 2}, nil
	})
	c := &Call{Queries: r}
	if out, err := Ask[echoOut](ctx, r, c, "test.twice", echoIn{N: 4}); err != nil || out.Twice != 8 {
		t.Fatalf("served domain: %v %v", out, err)
	}
	if _, err := Ask[echoOut](ctx, r, c, "test.twice", "not an object"); apperr.From(err).FieldErrors["_"] != "error.malformedInput" {
		t.Errorf("undecodable input: %v", err)
	}
	if _, err := Ask[echoOut](ctx, nil, c, "test.twice", echoIn{}); err == nil {
		t.Error("a query outside a request")
	}
	if _, err := Ask[echoOut](ctx, r, c, "test.nope", echoIn{}); err == nil {
		t.Error("an unknown query")
	}
	r.ServeDomains(DomainMaintenance) // equipment is now another service's domain, and there is no transport
	if _, err := Ask[echoOut](ctx, r, c, "test.twice", echoIn{N: 1}); apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("another domain without a transport: %v", err)
	}
	local := func(_ context.Context, _ *Call, in *echoIn) (echoOut, error) { return echoOut{Twice: in.N * 2}, nil }
	if out, err := Delegate(ctx, &Call{}, "test.twice", echoIn{N: 3}, local); err != nil || out.Twice != 6 {
		t.Errorf("a call without a registry runs locally: %v %v", out, err)
	}
}

// TestHTTPQueries covers the transport between domain services: the caller's tenant, membership and token, the
// internal token, correlation ID and business time on the request; the system path for a worker without a user;
// a DomainError passed through; anything else UNAVAILABLE.
func TestHTTPQueries(t *testing.T) {
	ctx := context.Background()
	var got *http.Request
	status, body := http.StatusOK, `{"data":{"twice":8}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL + "/")
	h := &HTTPQueries{Targets: map[string]*url.URL{DomainEquipment: target}, Token: "internal-token"}
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	p := &Principal{TenantID: uuid.New(), MembershipID: uuid.New()}
	user := &Call{Principal: p, Bearer: "user-token", CorrelationID: "corr-1", Now: now}

	out, err := h.Query(ctx, user, DomainEquipment, "assets.unitState", json.RawMessage(`{"unitId":"u1"}`))
	if err != nil || string(out) != `{"twice":8}` {
		t.Fatalf("query: %s %v", out, err)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/internal/v1/queries/assets.unitState" ||
		got.Header.Get("X-Tenant-Id") != p.TenantID.String() || got.Header.Get("X-Membership-Id") != p.MembershipID.String() ||
		got.Header.Get("Authorization") != "Bearer user-token" || got.Header.Get(InternalTokenHeader) != "internal-token" ||
		got.Header.Get("X-Request-Id") != "corr-1" || got.Header.Get(BusinessNowHeader) != now.Format(time.RFC3339Nano) {
		t.Errorf("request: %s %s %v", got.Method, got.URL.Path, got.Header)
	}
	worker := &Call{Principal: p, Now: now} // a scheduler job: no user, the tenant only (IR195)
	if _, err := h.Query(ctx, worker, DomainEquipment, "assets.unitState", nil); err != nil || got.URL.Path != "/internal/v1/system/queries/assets.unitState" ||
		got.Header.Get("Authorization") != "" || got.Header.Get("X-Membership-Id") != "" {
		t.Errorf("system query: %v %s %v", err, got.URL.Path, got.Header)
	}

	status, body = http.StatusNotFound, `{"code":"NOT_FOUND","messageKey":"error.notFound","fieldErrors":{}}`
	if _, err := h.Query(ctx, user, DomainEquipment, "q", nil); apperr.From(err).Code != apperr.NotFound {
		t.Errorf("a DomainError from the owner: %v", err)
	}
	status, body = http.StatusBadGateway, `<html>proxy error</html>`
	if _, err := h.Query(ctx, user, DomainEquipment, "q", nil); apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("an answer that is no DomainError: %v", err)
	}
	status, body = http.StatusOK, `not json`
	if _, err := h.Query(ctx, user, DomainEquipment, "q", nil); err == nil {
		t.Error("an undecodable envelope")
	}
	for name, c := range map[string]*Call{"no principal": {Bearer: "x"}} {
		if _, err := h.Query(ctx, c, DomainEquipment, "q", nil); apperr.From(err).Code != apperr.Unavailable {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := h.Query(ctx, user, DomainBilling, "q", nil); apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("a domain without a target: %v", err)
	}
	srv.Close()
	if _, err := h.Query(ctx, user, DomainEquipment, "q", nil); apperr.From(err).Code != apperr.Unavailable || errors.Is(err, context.Canceled) {
		t.Errorf("the owner is down: %v", err)
	}
}
