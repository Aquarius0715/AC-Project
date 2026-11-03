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
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
	reg := NewRegistry()
	RegisterQuery(
		reg,
		DomainEquipment,
		"test.twice",
		func(_ context.Context, _ *Call, in *echoIn) (map[string]int, error) {
			return map[string]int{"twice": in.N * 2}, nil
		},
	)

	call := &Call{
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

	out, err := Ask[echoOut](
		ctx,
		reg,
		call,
		"test.twice",
		echoIn{N: 4},
	)
	if err != nil ||
		out.Twice != 8 {
		t.Fatalf("served domain: %v %v", out, err)
	}

	_, err = Ask[echoOut](
		ctx,
		reg,
		call,
		"test.twice",
		"not an object",
	)
	if apperr.From(err).FieldErrors["_"] != "error.malformedInput" {
		t.Errorf("undecodable input: %v", err)
	}

	_, err = Ask[echoOut](ctx, nil, call, "test.twice", echoIn{N: 0})
	if err == nil {
		t.Error("a query outside a request")
	}

	_, err = Ask[echoOut](ctx, reg, call, "test.nope", echoIn{N: 0})
	if err == nil {
		t.Error("an unknown query")
	}

	reg.ServeDomains(
		DomainMaintenance,
	) // equipment is now another service's domain, and there is no transport

	_, err = Ask[echoOut](
		ctx,
		reg,
		call,
		"test.twice",
		echoIn{N: 1},
	)
	if apperr.From(
		err,
	).Code != apperr.Unavailable {
		t.Errorf("another domain without a transport: %v", err)
	}

	local := func(_ context.Context, _ *Call, in *echoIn) (echoOut, error) { return echoOut{Twice: in.N * 2}, nil }

	out, err = Delegate(
		ctx,
		&Call{
			Tx:              nil,
			Principal:       nil,
			Now:             time.Time{},
			CorrelationID:   "",
			Bearer:          "",
			Queries:         nil,
			ExpectedVersion: nil,
			Candidates:      nil,
			Events:          nil,
			Audits:          nil,
		},
		"test.twice",
		echoIn{N: 3},
		local,
	)
	if err != nil ||
		out.Twice != 6 {
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
	httpQueries := &HTTPQueries{
		Targets: map[string]*url.URL{DomainEquipment: target},
		Token:   "internal-token", Client: nil,
	}
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	principal := &Principal{
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
	user := &Call{
		Principal:       principal,
		Bearer:          "user-token",
		CorrelationID:   "corr-1",
		Now:             now,
		Tx:              nil,
		Queries:         nil,
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	}

	out, err := httpQueries.Query(
		ctx,
		user,
		DomainEquipment,
		"assets.unitState",
		json.RawMessage(`{"unitId":"u1"}`),
	)
	if err != nil || string(out) != `{"twice":8}` {
		t.Fatalf("query: %s %v", out, err)
	}

	if got.Method != http.MethodPost || got.URL.Path != "/internal/v1/queries/assets.unitState" ||
		got.Header.Get(
			"X-Tenant-Id",
		) != principal.TenantID.String() || got.Header.Get("X-Membership-Id") != principal.MembershipID.String() ||
		got.Header.Get(
			"Authorization",
		) != "Bearer user-token" || got.Header.Get(InternalTokenHeader) != "internal-token" ||
		got.Header.Get(
			"X-Request-ID",
		) != "corr-1" || got.Header.Get(BusinessNowHeader) != now.Format(time.RFC3339Nano) {
		t.Errorf("request: %s %s %v", got.Method, got.URL.Path, got.Header)
	}

	worker := &Call{
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
		ctx,
		worker,
		DomainEquipment,
		"assets.unitState",
		nil,
	)
	if queryErr != nil || got.URL.Path != "/internal/v1/system/queries/assets.unitState" ||
		got.Header.Get("Authorization") != "" ||
		got.Header.Get("X-Membership-Id") != "" {
		t.Errorf("system query: %v %s %v", queryErr, got.URL.Path, got.Header)
	}

	status, body = http.StatusNotFound, `{"code":"NOT_FOUND","messageKey":"error.notFound","fieldErrors":{}}`

	_, queryErr = httpQueries.Query(
		ctx,
		user,
		DomainEquipment,
		"q",
		nil,
	)
	if apperr.From(
		queryErr,
	).Code != apperr.NotFound {
		t.Errorf("a DomainError from the owner: %v", queryErr)
	}

	status, body = http.StatusBadGateway, `<html>proxy error</html>`

	_, queryErr = httpQueries.Query(
		ctx,
		user,
		DomainEquipment,
		"q",
		nil,
	)
	if apperr.From(
		queryErr,
	).Code != apperr.Unavailable {
		t.Errorf("an answer that is no DomainError: %v", queryErr)
	}

	status, body = http.StatusOK, `not json`

	_, queryErr = httpQueries.Query(ctx, user, DomainEquipment, "q", nil)
	if queryErr == nil {
		t.Error("an undecodable envelope")
	}

	for name, call := range map[string]*Call{
		"no principal": {
			Bearer: "x", Tx: nil, Principal: nil, Now: time.Time{}, CorrelationID: "", Queries: nil,
			ExpectedVersion: nil, Candidates: nil, Events: nil, Audits: nil,
		},
	} {
		_, err := httpQueries.Query(
			ctx,
			call,
			DomainEquipment,
			"q",
			nil,
		)
		if apperr.From(
			err,
		).Code != apperr.Unavailable {
			t.Errorf("%s: %v", name, err)
		}
	}

	_, queryErr = httpQueries.Query(
		ctx,
		user,
		DomainBilling,
		"q",
		nil,
	)
	if apperr.From(
		queryErr,
	).Code != apperr.Unavailable {
		t.Errorf("a domain without a target: %v", queryErr)
	}

	srv.Close()

	_, queryErr = httpQueries.Query(
		ctx,
		user,
		DomainEquipment,
		"q",
		nil,
	)
	if apperr.From(queryErr).Code != apperr.Unavailable ||
		errors.Is(queryErr, context.Canceled) {
		t.Errorf("the owner is down: %v", queryErr)
	}
}
