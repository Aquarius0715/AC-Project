package ops

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

type restTarget struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

type restIn struct {
	JobID  uuid.UUID    `json:"jobId"`
	Count  *int         `json:"count,omitempty"`
	Ratio  float64      `json:"ratio"`
	On     *bool        `json:"on,omitempty"`
	IDs    []uuid.UUID  `json:"ids"`
	Target restTarget   `json:"target"`
	Query  paging.Query `json:"query"`
	Action string       `json:"action"`
}

type restOpaque struct {
	Lines []struct {
		Qty int `json:"qty"`
	} `json:"lines"`
}

func restOp(mode Mode, in func() any) *Operation {
	return &Operation{Spec: Spec{Name: "test.op", Mode: mode, Filters: []string{"status", "unitIds", "overdueOnly", "jobId"}}, NewInput: in}
}

// bind runs Binding.Input for a request on route with the given path values.
func bind(t *testing.T, b *Binding, method, target, body string, path map[string]string) (map[string]any, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	c := echo.New().NewContext(req, httptest.NewRecorder())
	var pv echo.PathValues
	for _, name := range b.Path {
		pv = append(pv, echo.PathValue{Name: name, Value: path[name]})
	}
	c.SetPathValues(pv)
	raw, err := b.Input(c, []byte(body))
	if err != nil {
		return nil, apperr.From(err).FieldErrors
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj, nil
}

func TestRESTParameters(t *testing.T) {
	b, err := NewBinding(restOp(Read, func() any { return new(restIn) }), Route{Method: "GET", Path: "/v1/jobs/{jobId}/things"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Param{
		"jobId":       {Name: "jobId", At: []string{"jobId"}, UUID: true},
		"count":       {Name: "count", At: []string{"count"}, Kind: KindInteger},
		"ratio":       {Name: "ratio", At: []string{"ratio"}, Kind: KindNumber},
		"on":          {Name: "on", At: []string{"on"}, Kind: KindBool},
		"ids":         {Name: "ids", At: []string{"ids"}, List: true, UUID: true},
		"target.kind": {Name: "target.kind", At: []string{"target", "kind"}},
		"target.id":   {Name: "target.id", At: []string{"target", "id"}, UUID: true},
		"action":      {Name: "action", At: []string{"action"}},
		"cursor":      {Name: "cursor", At: []string{"query", "cursor"}},
		"limit":       {Name: "limit", At: []string{"query", "limit"}, Kind: KindInteger},
		"sort":        {Name: "sort", At: []string{"query", "sort"}, Sort: true},
		"status":      {Name: "status", At: []string{"query", "filters", "status"}},
		"unitIds":     {Name: "unitIds", At: []string{"query", "filters", "unitIds"}, List: true},
		"overdueOnly": {Name: "overdueOnly", At: []string{"query", "filters", "overdueOnly"}, Kind: KindBool},
	}
	got := map[string]Param{}
	for name, p := range b.Params {
		p.Role, p.Order = "", 0
		got[name] = p
	}
	if b.Params["status"].Role != "filter" || b.Params["sort"].Role != "sort" || b.Params["count"].Role != "field" || b.Params["jobId"].Order != 0 || b.Params["overdueOnly"].Order <= b.Params["sort"].Order {
		t.Fatalf("roles and order: %+v", b.Params)
	}
	if !reflect.DeepEqual(got, want) { // the filter jobId is left out: the input field jobId owns the name
		t.Fatalf("params\n got %+v\nwant %+v", got, want)
	}
	if !reflect.DeepEqual(b.Path, []string{"jobId"}) || b.Body {
		t.Fatalf("path %v body %v", b.Path, b.Body)
	}
	// a list read whose input is the paging query itself
	q, err := NewBinding(restOp(Read, func() any { return new(paging.Query) }), Route{Method: "GET", Path: "/v1/things"})
	if err != nil {
		t.Fatal(err)
	}
	if p := q.Params["sort"]; !p.Sort || !reflect.DeepEqual(p.At, []string{"sort"}) || q.Params["status"].At[0] != "filters" || len(q.Params) != 7 {
		t.Fatalf("paging query params %+v", q.Params)
	}
}

func TestRESTQueryInput(t *testing.T) {
	b, err := NewBinding(restOp(Read, func() any { return new(restIn) }), Route{Method: "GET", Path: "/v1/jobs/{jobId}/things"})
	if err != nil {
		t.Fatal(err)
	}
	job, u1, u2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	q := url.Values{"count": {"3"}, "ratio": {"1.50"}, "on": {"true"}, "ids": {u1 + "," + u2, u1}, "target.kind": {"unit"}, "sort": {"dueAt:desc"},
		"limit": {"10"}, "cursor": {""}, "status": {"open"}, "unitIds": {u2}, "overdueOnly": {"false"}}
	got, fe := bind(t, b, "GET", "/v1/jobs/"+job+"/things?"+q.Encode(), "", map[string]string{"jobId": job})
	if fe != nil {
		t.Fatal(fe)
	}
	want := map[string]any{"jobId": job, "count": 3.0, "ratio": 1.5, "on": true, "ids": []any{u1, u2, u1}, "target": map[string]any{"kind": "unit"},
		"query": map[string]any{"sort": map[string]any{"field": "dueAt", "direction": "desc"}, "limit": 10.0, "filters": map[string]any{"status": "open", "unitIds": []any{u2}, "overdueOnly": false}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input\n got %v\nwant %v", got, want)
	}
	// sort without a direction is ascending; an empty value is absent
	got, _ = bind(t, b, "GET", "/v1/jobs/"+job+"/things?sort=name&count=", "", map[string]string{"jobId": job})
	if s := got["query"].(map[string]any)["sort"]; !reflect.DeepEqual(s, map[string]any{"field": "name", "direction": "asc"}) || got["count"] != nil {
		t.Fatalf("defaults: %v", got)
	}
	// an empty list parameter is the empty list, not an absent field
	got, _ = bind(t, b, "GET", "/v1/jobs/"+job+"/things?ids=&unitIds=,", "", map[string]string{"jobId": job})
	if !reflect.DeepEqual(got["ids"], []any{}) || !reflect.DeepEqual(got["query"].(map[string]any)["filters"], map[string]any{"unitIds": []any{}}) {
		t.Fatalf("empty lists: %v", got)
	}
	for target, wantFE := range map[string]map[string]string{
		"?count=x":          {"count": "error.invalid"},
		"?count=1&count=2":  {"count": "error.invalid"},
		"?on=yes":           {"on": "error.invalid"},
		"?ratio=NaN":        {"ratio": "error.invalid"},
		"?sort=:asc":        {"sort": "error.invalid"},
		"?nope=1":           {"nope": "error.notAllowed"},
		"?jobId=" + job:     {"jobId": "error.notAllowed"}, // a path parameter is not repeated in the query
		"?limit=2&target=x": {"target": "error.notAllowed"},
	} {
		if _, fe := bind(t, b, "GET", "/v1/jobs/"+job+"/things"+target, "", map[string]string{"jobId": job}); !reflect.DeepEqual(fe, wantFE) {
			t.Errorf("%s: %v, want %v", target, fe, wantFE)
		}
	}
	if _, fe := bind(t, b, "GET", "/v1/jobs/x/things", "", map[string]string{"jobId": ""}); fe["jobId"] != "error.invalid" {
		t.Errorf("empty path parameter: %v", fe)
	}
	// an ID in the path that is not a UUID names no resource
	c := echo.New().NewContext(httptest.NewRequest("GET", "/v1/jobs/invoice-a/things", nil), httptest.NewRecorder())
	c.SetPathValues(echo.PathValues{{Name: "jobId", Value: "invoice-a"}})
	if _, err := b.Input(c, nil); apperr.From(err).Code != apperr.NotFound {
		t.Errorf("malformed ID: %v", err)
	}
}

func TestRESTBodyInput(t *testing.T) {
	b, err := NewBinding(restOp(Write, func() any { return new(restIn) }), Route{Method: "POST", Path: "/v1/things/{jobId}/{target.kind}/approve", FixedField: "action", FixedValue: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	job, other := uuid.NewString(), uuid.NewString()
	path := map[string]string{"jobId": job, "target.kind": "space%20a"}
	got, fe := bind(t, b, "POST", "/", `{"ratio": 12345678901234567890, "jobId": "`+job+`"}`, path)
	if fe != nil {
		t.Fatal(fe)
	}
	if got["action"] != "approve" || got["jobId"] != job || got["target"].(map[string]any)["kind"] != "space a" {
		t.Fatalf("body input %v", got)
	}
	c := echo.New().NewContext(httptest.NewRequest("POST", "/", nil), httptest.NewRecorder())
	c.SetPathValues(echo.PathValues{{Name: "jobId", Value: job}, {Name: "target.kind", Value: "unit"}})
	if raw, err := b.Input(c, []byte(`{"ratio": 12345678901234567890}`)); err != nil || !strings.Contains(string(raw), "12345678901234567890") { // numbers keep their text
		t.Fatalf("number changed: %s %v", raw, err)
	}
	for body, wantFE := range map[string]map[string]string{
		`{"jobId":"` + other + `"}`:     {"jobId": "error.pathMismatch"},
		`{"action":"mark_paid"}`:        {"action": "error.pathMismatch"},
		`{"target":"unit"}`:             {"target.kind": "error.pathMismatch"},
		`[1]`:                           {"_": "error.malformedInput"},
		`null`:                          {"_": "error.malformedInput"},
		`{"a":1} {"b":2}`:               {"_": "error.malformedInput"},
		`{"action":"approve"}`:          nil,
		`{"target":{"kind":"space a"}}`: nil,
	} {
		if _, fe := bind(t, b, "POST", "/", body, path); !reflect.DeepEqual(fe, wantFE) {
			t.Errorf("%s: %v, want %v", body, fe, wantFE)
		}
	}
	if _, fe := bind(t, b, "POST", "/?ratio=1", `{}`, path); fe["ratio"] != "error.notAllowed" {
		t.Errorf("query on a body route: %v", fe)
	}
}

func TestRESTBindingRules(t *testing.T) {
	in := func() any { return new(restIn) }
	for name, c := range map[string]struct {
		op *Operation
		rt Route
	}{
		"GET for a write":        {restOp(Write, in), Route{Method: "GET", Path: "/v1/x"}},
		"DELETE for a read":      {restOp(Read, in), Route{Method: "DELETE", Path: "/v1/x"}},
		"unknown method":         {restOp(Write, in), Route{Method: "OPTIONS", Path: "/v1/x"}},
		"unknown path parameter": {restOp(Read, in), Route{Method: "GET", Path: "/v1/x/{nope}"}},
		"list path parameter":    {restOp(Read, in), Route{Method: "GET", Path: "/v1/x/{ids}"}},
		"filter path parameter":  {restOp(Read, in), Route{Method: "GET", Path: "/v1/x/{status}"}},
		"unknown fixed field":    {restOp(Write, in), Route{Method: "POST", Path: "/v1/x", FixedField: "nope", FixedValue: "a"}},
		"objects in a query":     {restOp(Read, func() any { return new(restOpaque) }), Route{Method: "GET", Path: "/v1/x"}},
	} {
		if _, err := NewBinding(c.op, c.rt); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := NewBinding(restOp(Write, func() any { return new(restOpaque) }), Route{Method: "POST", Path: "/v1/x"}); err != nil {
		t.Errorf("objects in a body: %v", err)
	}
	if EchoPath("/v1/jobs/{jobId}/offers/{offerId}") != "/v1/jobs/:jobId/offers/:offerId" || !reflect.DeepEqual(PathParams("/v1/l/{target.kind}/{target.id}"), []string{"target.kind", "target.id"}) {
		t.Error("path helpers")
	}
}

// TestRESTRoutes drives catalog routes end to end through the shared pipeline (authorization, idempotency,
// ServiceResult) with the input taken from the path, the query string, the body and the fixed field.
func TestRESTRoutes(t *testing.T) {
	r := NewRegistry()
	r.DB = fakeDB{}
	idem := &fakeIdem{stored: map[string]json.RawMessage{}, hash: map[string]string{}}
	r.Rec, r.Idem = &fakeRec{}, idem
	type saveIn struct {
		ID   *string `json:"id,omitempty"`
		Name string  `json:"name"`
	}
	type getIn struct {
		ID    string  `json:"id"`
		JobID *string `json:"jobId,omitempty"`
	}
	type transitionIn struct {
		StatementID string `json:"statementId"`
		Action      string `json:"action"`
	}
	type deleteIn struct {
		ID     string `json:"id"`
		Reason string `json:"reason"`
	}
	Register(r, "units.save", func(ctx context.Context, c *Call, in *saveIn) (map[string]any, error) {
		return map[string]any{"id": in.ID, "name": in.Name}, nil
	})
	Register(r, "units.get", func(ctx context.Context, c *Call, in *getIn) (map[string]any, error) {
		if err := c.Require(authz.Facts{"self": in.ID == "mine"}); err != nil {
			return nil, err
		}
		return map[string]any{"id": in.ID, "jobId": in.JobID}, nil
	})
	Register(r, "units.delete", func(ctx context.Context, c *Call, in *deleteIn) (map[string]any, error) {
		return map[string]any{"id": in.ID, "reason": in.Reason}, nil
	})
	Register(r, "payouts.transition", func(ctx context.Context, c *Call, in *transitionIn) (map[string]any, error) {
		return map[string]any{"statementId": in.StatementID, "action": in.Action}, nil
	})
	e := echo.New()
	e.HTTPErrorHandler = HTTPErrorHandler
	if err := r.MountREST(e.Group("/v1")); err != nil {
		t.Fatal(err)
	}
	billing := &Principal{Principal: authz.Principal{Role: "admin", Permissions: map[string]bool{"asset.write": true, "asset.read": true, "billing.payment": true}}}
	call := func(p *Principal, method, target, body, key string, version string) (int, map[string]any, string) {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if version != "" {
			req.Header.Set("X-Expected-Version", version)
		}
		req = req.WithContext(WithPrincipal(req.Context(), p))
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		var res struct {
			Data map[string]any `json:"data"`
			Meta map[string]any `json:"meta"`
			Code string         `json:"code"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if res.Meta != nil && res.Meta["operation"] == nil {
			t.Fatalf("meta without operation: %s", w.Body.String())
		}
		return w.Code, res.Data, res.Code + w.Body.String()
	}
	if code, data, _ := call(billing, "POST", "/v1/units", `{"name":"a"}`, "key-create-1", ""); code != 200 || data["name"] != "a" || data["id"] != nil {
		t.Fatalf("create: %d %v", code, data)
	}
	if code, data, _ := call(billing, "PUT", "/v1/units/u1", `{"name":"b"}`, "key-update-1", "3"); code != 200 || data["id"] != "u1" {
		t.Fatalf("update: %d %v", code, data)
	}
	if code, _, body := call(billing, "PUT", "/v1/units/u1", `{"id":"u2","name":"b"}`, "key-update-2", "3"); code != 422 || !strings.Contains(body, "error.pathMismatch") {
		t.Fatalf("update mismatch: %d %s", code, body)
	}
	if code, _, body := call(billing, "PUT", "/v1/units/u1", `{"name":"b"}`, "", "3"); code != 422 || !strings.Contains(body, "Idempotency-Key") {
		t.Fatalf("write without key: %d %s", code, body)
	}
	if code, data, _ := call(client, "GET", "/v1/units/mine?jobId=j1", "", "", ""); code != 200 || data["id"] != "mine" || data["jobId"] != "j1" {
		t.Fatalf("get: %d %v", code, data)
	}
	if code, _, body := call(client, "GET", "/v1/units/other", "", "", ""); code != 404 || !strings.Contains(body, "error.notFound") { // outside the scope reads as absent
		t.Fatalf("get other: %d %s", code, body)
	}
	if code, _, body := call(client, "GET", "/v1/units/mine?x=1", "", "", ""); code != 422 || !strings.Contains(body, "error.notAllowed") {
		t.Fatalf("unknown parameter: %d %s", code, body)
	}
	if code, data, _ := call(billing, "DELETE", "/v1/units/u1?reason=duplicate%20entry", "", "key-delete-1", "4"); code != 200 || data["reason"] != "duplicate entry" {
		t.Fatalf("delete: %d %v", code, data)
	}
	for path, action := range map[string]string{"/v1/payouts/s1/approve": "approve", "/v1/payouts/s1/mark-paid": "mark_paid"} {
		if code, data, _ := call(billing, "POST", path, ``, "key-"+action, "2"); code != 200 || data["action"] != action || data["statementId"] != "s1" {
			t.Fatalf("%s: %d %v", path, code, data)
		}
	}
	// the same key and input replays the stored result; the same key with other input is CONFLICT
	if code, data, _ := call(billing, "POST", "/v1/units", `{"name":"a"}`, "key-create-1", ""); code != 200 || data["name"] != "a" {
		t.Fatalf("replay: %d %v", code, data)
	}
	if code, _, _ := call(billing, "POST", "/v1/units", `{"name":"z"}`, "key-create-1", ""); code != 409 {
		t.Fatalf("reused key: %d", code)
	}
	for _, c := range []struct{ method, path string }{{"GET", "/v1/nope"}, {"PATCH", "/v1/units/u1"}, {"GET", "/v1/units"}} {
		if code, _, body := call(billing, c.method, c.path, "", "", ""); code != 404 || !strings.Contains(body, "NOT_FOUND") {
			t.Errorf("%s %s: %d %s", c.method, c.path, code, body)
		}
	}
}
