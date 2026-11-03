package ops_test

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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// queryTwice is the internal query of the tests: twice its input.
const queryTwice = "test.twice"

type echoIn struct {
	N int `json:"n"`
}
type echoOut struct {
	Twice int `json:"twice"`
}

// twiceRegistry registers test.twice in the equipment domain, with a result of another type than echoOut.
func twiceRegistry() *ops.Registry {
	reg := ops.NewRegistry()
	ops.RegisterQuery(
		reg,
		ops.DomainEquipment,
		queryTwice,
		func(_ context.Context, _ *ops.Call, in *echoIn) (map[string]int, error) {
			return map[string]int{"twice": in.N * 2}, nil
		},
	)

	return reg
}

// callOf is a call whose internal queries go through reg (none for nil: the scheduler worker).
func callOf(reg *ops.Registry) *ops.Call {
	return &ops.Call{
		Queries:         reg,
		Tx:              nil,
		Principal:       nil,
		Now:             time.Time{},
		CorrelationID:   "",
		Bearer:          "",
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	}
}

// TestAskRoutesQueries covers how an internal query runs (IR192): in the caller's process when it serves the owner's
// domain (a result of another type goes through JSON), and the errors of an undecodable input, a missing registry and
// an unknown name.
func TestAskRoutesQueries(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	reg := twiceRegistry()
	call := callOf(reg)

	out, err := ops.Ask[echoOut](
		ctx,
		reg,
		call,
		queryTwice,
		echoIn{N: 4},
	)
	if err != nil ||
		out.Twice != 8 {
		t.Fatalf("served domain: %v %v", out, err)
	}

	_, err = ops.Ask[echoOut](
		ctx,
		reg,
		call,
		queryTwice,
		"not an object",
	)
	if apperr.From(err).FieldErrors["_"] != apperr.KeyMalformedInput {
		t.Errorf("undecodable input: %v", err)
	}

	_, err = ops.Ask[echoOut](ctx, nil, call, queryTwice, echoIn{N: 0})
	if err == nil {
		t.Error("a query outside a request")
	}

	_, err = ops.Ask[echoOut](ctx, reg, call, "test.nope", echoIn{N: 0})
	if err == nil {
		t.Error("an unknown query")
	}
}

// TestAskAnotherDomain: a query of a domain another service serves is UNAVAILABLE without a transport.
func TestAskAnotherDomain(t *testing.T) {
	t.Parallel()

	reg := twiceRegistry()
	reg.ServeDomains(
		ops.DomainMaintenance,
	) // equipment is now another service's domain, and there is no transport

	_, err := ops.Ask[echoOut](
		t.Context(),
		reg,
		callOf(reg),
		queryTwice,
		echoIn{N: 1},
	)
	if apperr.From(
		err,
	).Code != apperr.Unavailable {
		t.Errorf("another domain without a transport: %v", err)
	}
}

// TestDelegateRunsLocally: a call without a registry (the scheduler worker) runs the local handler.
func TestDelegateRunsLocally(t *testing.T) {
	t.Parallel()

	local := func(_ context.Context, _ *ops.Call, in *echoIn) (echoOut, error) {
		return echoOut{Twice: in.N * 2}, nil
	}

	out, err := ops.Delegate(
		t.Context(),
		callOf(nil),
		queryTwice,
		echoIn{N: 3},
		local,
	)
	if err != nil ||
		out.Twice != 6 {
		t.Errorf("a call without a registry runs locally: %v %v", out, err)
	}
}

// owner is the owning service of the transport tests: it answers every query with status and body and keeps the
// request it got.
type owner struct {
	srv    *httptest.Server
	got    *http.Request
	status int
	body   string
}

func newOwner(t *testing.T) *owner {
	t.Helper()

	fake := &owner{srv: nil, got: nil, status: http.StatusOK, body: `{"data":{"twice":8}}`}
	fake.srv = httptest.NewServer(
		http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			fake.got = request

			writer.WriteHeader(fake.status)
			_, _ = writer.Write([]byte(fake.body))
		}),
	)
	t.Cleanup(fake.srv.Close)

	return fake
}

// answer sets the owner's next answer.
func (o *owner) answer(status int, body string) { o.status, o.body = status, body }

// transport is the HTTP transport to the owner as the equipment domain's service.
func (o *owner) transport() *ops.HTTPQueries {
	target, _ := url.Parse(o.srv.URL + "/")

	return &ops.HTTPQueries{
		Targets: map[string]*url.URL{ops.DomainEquipment: target},
		Token:   "internal-token", Client: nil,
	}
}

// TestHTTPQueries covers the transport between domain services: the caller's tenant, membership and token, the
// internal token, correlation ID and business time on the request; the system path for a worker without a user;
// a DomainError passed through; anything else UNAVAILABLE.
func TestHTTPQueries(t *testing.T) {
	t.Parallel()

	fake := newOwner(t)
	httpQueries := fake.transport()
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	principal := &ops.Principal{
		TenantID:          uuid.New(),
		MembershipID:      uuid.New(),
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}
	user := &ops.Call{
		Principal:       principal,
		Bearer:          "user-token",
		CorrelationID:   correlation,
		Now:             now,
		Tx:              nil,
		Queries:         nil,
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	}

	out, err := httpQueries.Query(
		t.Context(),
		user,
		ops.DomainEquipment,
		"assets.unitState",
		json.RawMessage(`{"unitId":"u1"}`),
	)
	if err != nil || string(out) != `{"twice":8}` {
		t.Fatalf("query: %s %v", out, err)
	}

	checkUserRequest(t, fake.got, principal, now)

	checkSystemQuery(t, fake, httpQueries, principal, now)
	checkQueryFailures(t, fake, httpQueries, user)
}

// checkSystemQuery: a scheduler job's query has no user, the tenant only (IR195), and goes to the system path.
func checkSystemQuery(
	t *testing.T,
	fake *owner,
	httpQueries *ops.HTTPQueries,
	principal *ops.Principal,
	now time.Time,
) {
	t.Helper()

	worker := &ops.Call{
		Principal:       principal,
		Now:             now,
		Tx:              nil,
		CorrelationID:   "",
		Bearer:          "",
		Queries:         nil,
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	} // a scheduler job: no user, the tenant only (IR195)

	_, queryErr := httpQueries.Query(
		t.Context(),
		worker,
		ops.DomainEquipment,
		"assets.unitState",
		nil,
	)
	if queryErr != nil || fake.got.URL.Path != "/internal/v1/system/queries/assets.unitState" ||
		fake.got.Header.Get("Authorization") != "" ||
		fake.got.Header.Get("X-Membership-Id") != "" {
		t.Errorf("system query: %v %s %v", queryErr, fake.got.URL.Path, fake.got.Header)
	}
}

// checkUserRequest: the caller's tenant, membership and token, the internal token, the correlation ID and the
// business time reach the owner.
func checkUserRequest(t *testing.T, got *http.Request, principal *ops.Principal, now time.Time) {
	t.Helper()

	if got.Method != http.MethodPost || got.URL.Path != "/internal/v1/queries/assets.unitState" ||
		got.Header.Get(
			"X-Tenant-Id",
		) != principal.TenantID.String() || got.Header.Get("X-Membership-Id") != principal.MembershipID.String() ||
		got.Header.Get(
			"Authorization",
		) != "Bearer user-token" || got.Header.Get(ops.InternalTokenHeader) != "internal-token" ||
		got.Header.Get(
			"X-Request-ID",
		) != correlation || got.Header.Get(ops.BusinessNowHeader) != now.Format(time.RFC3339Nano) {
		t.Errorf("request: %s %s %v", got.Method, got.URL.Path, got.Header)
	}
}

// checkQueryFailures: a DomainError from the owner passes through; an answer that is no DomainError, a call without a
// principal, a domain without a target and an owner that is down are UNAVAILABLE; an undecodable envelope fails.
func checkQueryFailures(t *testing.T, fake *owner, httpQueries *ops.HTTPQueries, user *ops.Call) {
	t.Helper()

	ctx := t.Context()

	fake.answer(
		http.StatusNotFound,
		`{"code":"NOT_FOUND","messageKey":"error.notFound","fieldErrors":{}}`,
	)

	_, err := httpQueries.Query(ctx, user, ops.DomainEquipment, "q", nil)
	if apperr.From(err).Code != apperr.NotFound {
		t.Errorf("a DomainError from the owner: %v", err)
	}

	fake.answer(http.StatusBadGateway, `<html>proxy error</html>`)

	_, err = httpQueries.Query(ctx, user, ops.DomainEquipment, "q", nil)
	if apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("an answer that is no DomainError: %v", err)
	}

	fake.answer(http.StatusOK, `not json`)

	_, err = httpQueries.Query(ctx, user, ops.DomainEquipment, "q", nil)
	if err == nil {
		t.Error("an undecodable envelope")
	}

	noPrincipal := &ops.Call{
		Bearer: "x", Tx: nil, Principal: nil, Now: time.Time{}, CorrelationID: "", Queries: nil,
		ExpectedVersion: nil, Candidates: nil, Events: nil, Audits: nil,
	}

	_, err = httpQueries.Query(ctx, noPrincipal, ops.DomainEquipment, "q", nil)
	if apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("no principal: %v", err)
	}

	_, err = httpQueries.Query(ctx, user, ops.DomainBilling, "q", nil)
	if apperr.From(err).Code != apperr.Unavailable {
		t.Errorf("a domain without a target: %v", err)
	}

	fake.srv.Close()

	_, err = httpQueries.Query(ctx, user, ops.DomainEquipment, "q", nil)
	if apperr.From(err).Code != apperr.Unavailable ||
		errors.Is(err, context.Canceled) {
		t.Errorf("the owner is down: %v", err)
	}
}
