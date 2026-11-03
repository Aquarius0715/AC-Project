package ops

import (
	"context"
	"encoding/json"
	"net/http"
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

// The input fields, values and paths of the REST binding tests.
const (
	fieldStatus      = "status"
	fieldOverdueOnly = "overdueOnly"
	fieldCount       = "count"
	fieldRatio       = "ratio"
	fieldIDs         = "ids"
	fieldTarget      = "target"
	fieldTargetKind  = "target.kind"
	fieldTargetID    = "target.id"
	fieldAction      = "action"
	statusOpen       = "open"
	actionApprove    = "approve"
	pathX            = "/v1/x"
	pathJobThings    = "/v1/jobs/{jobId}/things"
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

// restTwice names one parameter twice: the embedded struct's field and its own.
type restTwice struct {
	RestInner

	Name string `json:"name"`
}

// RestInner is exported, so the binding walks it as an embedded struct.
type RestInner struct {
	Other string `json:"name"`
}

// restEmbedded embeds the paging query and has a field without a JSON name.
type restEmbedded struct {
	paging.Query

	Note string
}

func restOp(mode Mode, input func() any) *Operation {
	return &Operation{
		Spec: Spec{
			Name: "test.op",
			Mode: mode,
			Filters: []string{
				fieldStatus,
				fieldUnitIDs,
				fieldOverdueOnly,
				fieldJobID,
			},
			Module:        "",
			Authorization: "",
			Versions:      nil,
			DesignIDs:     "",
			Routes:        nil,
		},
		NewInput: input,
		Expr:     authz.Expr{Alternatives: nil, Notes: nil, Remark: ""},
		Result:   nil,
		Handle:   nil,
	}
}

// bind runs Binding.Input for a request on route with the given path values.
func bind(
	t *testing.T,
	binding *Binding,
	method, target, body string,
	path map[string]string,
) (map[string]any, map[string]string) {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
	ectx := echo.New().NewContext(req, httptest.NewRecorder())

	pathValues := make(echo.PathValues, 0, len(binding.Path))
	for _, name := range binding.Path {
		pathValues = append(pathValues, echo.PathValue{Name: name, Value: path[name]})
	}

	if len(pathValues) > 0 {
		ectx.SetPathValues(pathValues)
	}

	raw, err := binding.Input(ectx, []byte(body))
	if err != nil {
		return nil, apperr.From(err).FieldErrors
	}

	var obj map[string]any

	unmarshalErr := json.Unmarshal(raw, &obj)
	if unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}

	return obj, nil
}

// object is value as a JSON object; anything else fails the test.
func object(t *testing.T, value any) map[string]any {
	t.Helper()

	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not an object: %v", value)
	}

	return obj
}

// jobThingsBinding is the binding of GET /v1/jobs/{jobId}/things for a restIn read.
func jobThingsBinding(t *testing.T) *Binding {
	t.Helper()

	binding, err := NewBinding(
		restOp(Read, func() any { return new(restIn) }),
		Route{Method: http.MethodGet, Path: pathJobThings, FixedField: "", FixedValue: ""},
	)
	if err != nil {
		t.Fatal(err)
	}

	return binding
}

// wantParam is an expected parameter, without its role and order.
func wantParam(name string, kind ParamKind, list, isUUID bool, at ...string) Param {
	return Param{
		Name:  name,
		At:    at,
		Kind:  kind,
		List:  list,
		UUID:  isUUID,
		Sort:  false,
		Role:  "",
		Order: 0,
	}
}

// wantRESTParams are the parameters of a restIn read, without their roles and order.
func wantRESTParams() map[string]Param {
	sort := wantParam(paramSort, KindString, false, false, fieldQuery, paramSort)
	sort.Sort = true

	return map[string]Param{
		fieldJobID:      wantParam(fieldJobID, KindString, false, true, fieldJobID),
		fieldCount:      wantParam(fieldCount, KindInteger, false, false, fieldCount),
		fieldRatio:      wantParam(fieldRatio, KindNumber, false, false, fieldRatio),
		"on":            wantParam("on", KindBool, false, false, "on"),
		fieldIDs:        wantParam(fieldIDs, KindString, true, true, fieldIDs),
		fieldTargetKind: wantParam(fieldTargetKind, KindString, false, false, fieldTarget, "kind"),
		fieldTargetID:   wantParam(fieldTargetID, KindString, false, true, fieldTarget, "id"),
		fieldAction:     wantParam(fieldAction, KindString, false, false, fieldAction),
		paramCursor:     wantParam(paramCursor, KindString, false, false, fieldQuery, paramCursor),
		paramLimit:      wantParam(paramLimit, KindInteger, false, false, fieldQuery, paramLimit),
		paramSort:       sort,
		fieldStatus: wantParam(
			fieldStatus, KindString, false, false, fieldQuery, fieldFilters, fieldStatus,
		),
		fieldUnitIDs: wantParam(
			fieldUnitIDs, KindString, true, false, fieldQuery, fieldFilters, fieldUnitIDs,
		),
		fieldOverdueOnly: wantParam(
			fieldOverdueOnly, KindBool, false, false, fieldQuery, fieldFilters, fieldOverdueOnly,
		),
	}
}

func TestRESTParameters(t *testing.T) {
	t.Parallel()

	jobBinding := jobThingsBinding(t)

	got := map[string]Param{}

	for name, param := range jobBinding.Params {
		param.Role, param.Order = "", 0
		got[name] = param
	}

	if jobBinding.Params[fieldStatus].Role != roleFilter ||
		jobBinding.Params[paramSort].Role != paramSort ||
		jobBinding.Params[fieldCount].Role != roleField ||
		jobBinding.Params[fieldJobID].Order != 0 ||
		jobBinding.Params[fieldOverdueOnly].Order <= jobBinding.Params[paramSort].Order {
		t.Fatalf("roles and order: %+v", jobBinding.Params)
	}

	if !reflect.DeepEqual(
		got,
		wantRESTParams(),
	) { // the filter jobId is left out: the input field jobId owns the name
		t.Fatalf("params\n got %+v\nwant %+v", got, wantRESTParams())
	}

	if !reflect.DeepEqual(jobBinding.Path, []string{fieldJobID}) || jobBinding.Body {
		t.Fatalf("path %v body %v", jobBinding.Path, jobBinding.Body)
	}
}

// TestRESTPagingQueryParameters: a list read whose input is the paging query itself.
func TestRESTPagingQueryParameters(t *testing.T) {
	t.Parallel()

	binding, err := NewBinding(
		restOp(Read, func() any { return new(paging.Query) }),
		Route{Method: http.MethodGet, Path: "/v1/things", FixedField: "", FixedValue: ""},
	)
	if err != nil {
		t.Fatal(err)
	}

	if param := binding.Params[paramSort]; !param.Sort ||
		!reflect.DeepEqual(param.At, []string{paramSort}) ||
		binding.Params[fieldStatus].At[0] != fieldFilters ||
		len(binding.Params) != 7 {
		t.Fatalf("paging query params %+v", binding.Params)
	}
}

func TestRESTQueryInput(t *testing.T) {
	t.Parallel()

	binding := jobThingsBinding(t)
	job, firstID, secondID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	params := url.Values{
		fieldCount: {
			"3",
		},
		fieldRatio:      {"1.50"},
		"on":            {"true"},
		fieldIDs:        {firstID + "," + secondID, firstID},
		fieldTargetKind: {kindUnit},
		paramSort:       {"dueAt:desc"},
		paramLimit: {
			"10",
		},
		paramCursor:      {""},
		fieldStatus:      {statusOpen},
		fieldUnitIDs:     {secondID},
		fieldOverdueOnly: {"false"},
	}

	got, problems := bind(
		t,
		binding,
		http.MethodGet,
		"/v1/jobs/"+job+"/things?"+params.Encode(),
		"",
		map[string]string{fieldJobID: job},
	)
	if problems != nil {
		t.Fatal(problems)
	}

	want := map[string]any{
		fieldJobID:  job,
		fieldCount:  3.0,
		fieldRatio:  1.5,
		"on":        true,
		fieldIDs:    []any{firstID, secondID, firstID},
		fieldTarget: map[string]any{"kind": kindUnit},
		fieldQuery: map[string]any{
			paramSort:  map[string]any{sortKeyField: "dueAt", sortKeyDirection: "desc"},
			paramLimit: 10.0,
			fieldFilters: map[string]any{
				fieldStatus:      statusOpen,
				fieldUnitIDs:     []any{secondID},
				fieldOverdueOnly: false,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input\n got %v\nwant %v", got, want)
	}
}

// TestRESTQueryDefaults: sort without a direction is ascending, an empty value is absent, an empty sort is no sort,
// and an empty list parameter is the empty list, not an absent field.
func TestRESTQueryDefaults(t *testing.T) {
	t.Parallel()

	binding := jobThingsBinding(t)
	job := uuid.NewString()
	path := map[string]string{fieldJobID: job}

	got, _ := bind(t, binding, http.MethodGet, "/v1/jobs/"+job+"/things?sort=name&count=", "", path)
	if sort := object(t, got[fieldQuery])[paramSort]; !reflect.DeepEqual(
		sort,
		map[string]any{sortKeyField: fieldName, sortKeyDirection: "asc"},
	) ||
		got[fieldCount] != nil {
		t.Fatalf("defaults: %v", got)
	}

	if got, fieldErrs := bind(
		t,
		binding,
		http.MethodGet,
		"/v1/jobs/"+job+"/things?sort=",
		"",
		path,
	); fieldErrs != nil ||
		got[fieldQuery] != nil {
		t.Fatalf("empty sort: %v %v", got, fieldErrs)
	}

	got, _ = bind(t, binding, http.MethodGet, "/v1/jobs/"+job+"/things?ids=&unitIds=,", "", path)
	if !reflect.DeepEqual(got[fieldIDs], []any{}) ||
		!reflect.DeepEqual(
			object(t, got[fieldQuery])[fieldFilters],
			map[string]any{fieldUnitIDs: []any{}},
		) {
		t.Fatalf("empty lists: %v", got)
	}
}

// TestRESTQueryRefusals: a bad or repeated value, an unknown parameter, a path parameter or an object in the query,
// and an empty path parameter are VALIDATION; an ID in the path that is not a UUID names no resource.
func TestRESTQueryRefusals(t *testing.T) {
	t.Parallel()

	binding := jobThingsBinding(t)
	job := uuid.NewString()

	for target, wantFE := range map[string]map[string]string{
		"?count=x":          {fieldCount: apperr.KeyInvalid},
		"?count=1&count=2":  {fieldCount: apperr.KeyInvalid},
		"?on=yes":           {"on": apperr.KeyInvalid},
		"?ratio=NaN":        {fieldRatio: apperr.KeyInvalid},
		"?sort=:asc":        {paramSort: apperr.KeyInvalid},
		"?nope=1":           {"nope": apperr.KeyNotAllowed},
		"?jobId=" + job:     {fieldJobID: apperr.KeyNotAllowed}, // a path parameter is not repeated in the query
		"?limit=2&target=x": {fieldTarget: apperr.KeyNotAllowed},
	} {
		if _, fieldErrs := bind(
			t,
			binding,
			http.MethodGet,
			"/v1/jobs/"+job+"/things"+target,
			"",
			map[string]string{fieldJobID: job},
		); !reflect.DeepEqual(
			fieldErrs,
			wantFE,
		) {
			t.Errorf("%s: %v, want %v", target, fieldErrs, wantFE)
		}
	}

	if _, fieldErrs := bind(
		t,
		binding,
		http.MethodGet,
		"/v1/jobs/x/things",
		"",
		map[string]string{fieldJobID: ""},
	); fieldErrs[fieldJobID] != apperr.KeyInvalid {
		t.Errorf("empty path parameter: %v", fieldErrs)
	}

	ectx := echo.New().NewContext(
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/jobs/invoice-a/things", nil),
		httptest.NewRecorder(),
	)
	ectx.SetPathValues(echo.PathValues{{Name: fieldJobID, Value: "invoice-a"}})

	_, inputErr := binding.Input(ectx, nil)
	if apperr.From(inputErr).Code != apperr.NotFound {
		t.Errorf("malformed ID: %v", inputErr)
	}
}

// approveBinding is the binding of POST /v1/things/{jobId}/{target.kind}/approve, which fixes action=approve.
func approveBinding(t *testing.T) *Binding {
	t.Helper()

	binding, err := NewBinding(
		restOp(Write, func() any { return new(restIn) }),
		Route{
			Method:     http.MethodPost,
			Path:       "/v1/things/{jobId}/{target.kind}/approve",
			FixedField: fieldAction,
			FixedValue: actionApprove,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return binding
}

func TestRESTBodyInput(t *testing.T) {
	t.Parallel()

	binding := approveBinding(t)
	job := uuid.NewString()
	path := map[string]string{fieldJobID: job, fieldTargetKind: "space%20a"}

	got, fieldErrs := bind(
		t,
		binding,
		http.MethodPost,
		"/",
		`{"ratio": 12345678901234567890, "jobId": "`+job+`"}`,
		path,
	)
	if fieldErrs != nil {
		t.Fatal(fieldErrs)
	}

	if got[fieldAction] != actionApprove || got[fieldJobID] != job ||
		object(t, got[fieldTarget])["kind"] != "space a" {
		t.Fatalf("body input %v", got)
	}

	ectx := echo.New().NewContext(
		httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", nil),
		httptest.NewRecorder(),
	)
	ectx.SetPathValues(
		echo.PathValues{{Name: fieldJobID, Value: job}, {Name: fieldTargetKind, Value: kindUnit}},
	)

	raw, inputErr := binding.Input(
		ectx,
		[]byte(`{"ratio": 12345678901234567890}`),
	)
	if inputErr != nil ||
		!strings.Contains(string(raw), "12345678901234567890") { // numbers keep their text
		t.Fatalf("number changed: %s %v", raw, inputErr)
	}
}

// TestRESTBodyRefusals: a body that names a path parameter or the fixed field with another value, a body that is not
// one JSON object, and query parameters on a body route are VALIDATION.
func TestRESTBodyRefusals(t *testing.T) {
	t.Parallel()

	binding := approveBinding(t)
	job, other := uuid.NewString(), uuid.NewString()
	path := map[string]string{fieldJobID: job, fieldTargetKind: "space%20a"}

	for body, wantFE := range map[string]map[string]string{
		`{"jobId":"` + other + `"}`:     {fieldJobID: apperr.KeyPathMismatch},
		`{"action":"mark_paid"}`:        {fieldAction: apperr.KeyPathMismatch},
		`{"target":"unit"}`:             {fieldTargetKind: apperr.KeyPathMismatch},
		`[1]`:                           {"_": apperr.KeyMalformedInput},
		`null`:                          {"_": apperr.KeyMalformedInput},
		`{"a":1} {"b":2}`:               {"_": apperr.KeyMalformedInput},
		`{"action":"approve"}`:          nil,
		`{"target":{"kind":"space a"}}`: nil,
	} {
		if _, fieldErrs := bind(
			t,
			binding,
			http.MethodPost,
			"/",
			body,
			path,
		); !reflect.DeepEqual(
			fieldErrs,
			wantFE,
		) {
			t.Errorf("%s: %v, want %v", body, fieldErrs, wantFE)
		}
	}

	if _, fieldErrs := bind(
		t,
		binding,
		http.MethodPost,
		"/?ratio=1",
		`{}`,
		path,
	); fieldErrs[fieldRatio] != apperr.KeyNotAllowed {
		t.Errorf("query on a body route: %v", fieldErrs)
	}
}

// refusedRoute is a route NewBinding refuses for its operation.
type refusedRoute struct {
	op    *Operation
	route Route
}

// refusedRoutes are routes that do not fit their operation: a method against the mode, an unknown method, a path
// parameter or fixed field that is no scalar input field, objects in a query string and a parameter named twice.
func refusedRoutes() map[string]refusedRoute {
	input := func() any { return new(restIn) }

	return map[string]refusedRoute{
		"GET for a write": {
			restOp(Write, input),
			Route{Method: http.MethodGet, Path: pathX, FixedField: "", FixedValue: ""},
		},
		"DELETE for a read": {
			restOp(Read, input),
			Route{Method: http.MethodDelete, Path: pathX, FixedField: "", FixedValue: ""},
		},
		"unknown method": {
			restOp(Write, input),
			Route{Method: "OPTIONS", Path: pathX, FixedField: "", FixedValue: ""},
		},
		"unknown path parameter": {
			restOp(Read, input),
			Route{Method: http.MethodGet, Path: "/v1/x/{nope}", FixedField: "", FixedValue: ""},
		},
		"list path parameter": {
			restOp(Read, input),
			Route{Method: http.MethodGet, Path: "/v1/x/{ids}", FixedField: "", FixedValue: ""},
		},
		"filter path parameter": {
			restOp(Read, input),
			Route{Method: http.MethodGet, Path: "/v1/x/{status}", FixedField: "", FixedValue: ""},
		},
		"unknown fixed field": {
			restOp(Write, input),
			Route{Method: http.MethodPost, Path: pathX, FixedField: "nope", FixedValue: "a"},
		},
		"objects in a query": {
			restOp(Read, func() any { return new(restOpaque) }),
			Route{Method: http.MethodGet, Path: pathX, FixedField: "", FixedValue: ""},
		},
		"a parameter twice": {
			restOp(Read, func() any { return new(restTwice) }),
			Route{Method: http.MethodGet, Path: pathX, FixedField: "", FixedValue: ""},
		},
	}
}

func TestRESTBindingRules(t *testing.T) {
	t.Parallel()

	for name, testCase := range refusedRoutes() {
		_, err := NewBinding(testCase.op, testCase.route)
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	_, err := NewBinding(
		restOp(Write, func() any { return new(restOpaque) }),
		Route{Method: http.MethodPost, Path: pathX, FixedField: "", FixedValue: ""},
	)
	if err != nil {
		t.Errorf("objects in a body: %v", err)
	}

	if EchoPath("/v1/jobs/{jobId}/offers/{offerId}") != "/v1/jobs/:jobId/offers/:offerId" ||
		!reflect.DeepEqual(
			PathParams("/v1/l/{target.kind}/{target.id}"),
			[]string{fieldTargetKind, fieldTargetID},
		) {
		t.Error("path helpers")
	}
}

// TestRESTEmbeddedQuery: an embedded paging query binds its cursor, limit, sort and filters at the top of the input,
// and a field without a JSON name is named as Go names it.
func TestRESTEmbeddedQuery(t *testing.T) {
	t.Parallel()

	binding, err := NewBinding(
		restOp(Read, func() any { return new(restEmbedded) }),
		Route{Method: http.MethodGet, Path: pathX, FixedField: "", FixedValue: ""},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, fieldErrs := bind(
		t,
		binding,
		http.MethodGet,
		"/v1/x?limit=5&sort=name:desc&Note=hi&status=open",
		"",
		nil,
	)
	if fieldErrs != nil {
		t.Fatal(fieldErrs)
	}

	want := map[string]any{
		paramLimit:   5.0,
		paramSort:    map[string]any{sortKeyField: fieldName, sortKeyDirection: "desc"},
		"Note":       "hi",
		fieldFilters: map[string]any{fieldStatus: statusOpen},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("input\n got %v\nwant %v", got, want)
	}
}

// restRoutesApp serves units.save, units.get, units.delete and payouts.transition through the shared pipeline over
// the fakes.
func restRoutesApp(t *testing.T) *echo.Echo {
	t.Helper()

	reg := NewRegistry()
	reg.DB = fakeDB{readOnly: nil}
	idem := &fakeIdem{stored: map[string]json.RawMessage{}, hash: map[string]string{}, aborted: 0}
	reg.Rec, reg.Idem = &fakeRec{n: 0, err: nil}, idem
	registerRouteOperations(reg)

	app := echo.New()

	app.HTTPErrorHandler = HTTPErrorHandler

	err := reg.MountREST(app.Group("/v1"))
	if err != nil {
		t.Fatal(err)
	}

	return app
}

// registerRouteOperations registers the handlers of TestRESTRoutes: each answers with its input.
func registerRouteOperations(reg *Registry) {
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

	Register(
		reg,
		opUnitsSave,
		func(_ context.Context, _ *Call, in *saveIn) (map[string]any, error) {
			return map[string]any{"id": in.ID, fieldName: in.Name}, nil
		},
	)
	Register(
		reg,
		opUnitsGet,
		func(_ context.Context, call *Call, input *getIn) (map[string]any, error) {
			err := call.Require(authz.Facts{"self": input.ID == ownUnit})
			if err != nil {
				return nil, err
			}

			return map[string]any{"id": input.ID, fieldJobID: input.JobID}, nil
		},
	)
	Register(
		reg,
		"units.delete",
		func(_ context.Context, _ *Call, in *deleteIn) (map[string]any, error) {
			return map[string]any{"id": in.ID, "reason": in.Reason}, nil
		},
	)
	Register(
		reg,
		"payouts.transition",
		func(_ context.Context, _ *Call, in *transitionIn) (map[string]any, error) {
			return map[string]any{"statementId": in.StatementID, fieldAction: in.Action}, nil
		},
	)
}

// routeSender sends one request of a principal with an Idempotency-Key and an expected version (none when empty) and
// returns the status, the result data and the error code followed by the body.
type routeSender func(principal *Principal, method, target, body, key, version string) (int, map[string]any, string)

func newRouteSender(t *testing.T, app *echo.Echo) routeSender {
	t.Helper()

	return func(principal *Principal, method, target, body, key, version string) (int, map[string]any, string) {
		req := httptest.NewRequestWithContext(t.Context(), method, target, strings.NewReader(body))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}

		if version != "" {
			req.Header.Set("X-Expected-Version", version)
		}

		req = req.WithContext(WithPrincipal(req.Context(), principal))
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)

		var res struct {
			Data map[string]any `json:"data"`
			Meta map[string]any `json:"meta"`
			Code string         `json:"code"`
		}

		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if res.Meta != nil && res.Meta["operation"] == nil {
			t.Fatalf("meta without operation: %s", rec.Body.String())
		}

		return rec.Code, res.Data, res.Code + rec.Body.String()
	}
}

// billingPrincipal is an HQ admin who writes assets and payouts.
func billingPrincipal() *Principal {
	return &Principal{
		Principal: authz.Principal{
			Role: roleAdmin,
			Permissions: map[string]bool{
				"asset.write":     true,
				permAssetRead:     true,
				"billing.payment": true,
			}, ClientRole: "",
		}, TenantID: uuid.UUID{}, MembershipID: uuid.UUID{}, UserID: uuid.UUID{}, OrgID: uuid.UUID{}, ScopeVersion: 0,
		Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
	}
}

// TestRESTRoutes drives catalog routes end to end through the shared pipeline (authorization, idempotency,
// ServiceResult) with the input taken from the path, the query string, the body and the fixed field.
func TestRESTRoutes(t *testing.T) {
	t.Parallel()

	send := newRouteSender(t, restRoutesApp(t))
	billing := billingPrincipal()

	checkRouteWrites(t, send, billing)
	checkRouteReads(t, send)
	checkRouteDeleteAndVerbs(t, send, billing)
	checkRouteReplay(t, send, billing)

	for _, testCase := range []struct{ method, path string }{
		{http.MethodGet, "/v1/nope"},
		{http.MethodPatch, "/v1/units/u1"},
		{http.MethodGet, "/v1/units"},
	} {
		if code, _, body := send(
			billing,
			testCase.method,
			testCase.path,
			"",
			"",
			"",
		); code != 404 ||
			!strings.Contains(body, string(apperr.NotFound)) {
			t.Errorf("%s %s: %d %s", testCase.method, testCase.path, code, body)
		}
	}
}

// checkRouteWrites: a create on the collection, an update on the item, an item ID the body contradicts, and a write
// without a key.
func checkRouteWrites(t *testing.T, send routeSender, billing *Principal) {
	t.Helper()

	if code, data, _ := send(
		billing,
		http.MethodPost,
		"/v1/units",
		bodyNameA,
		"key-create-1",
		"",
	); code != 200 || data[fieldName] != "a" ||
		data["id"] != nil {
		t.Fatalf("create: %d %v", code, data)
	}

	if code, data, _ := send(
		billing,
		http.MethodPut,
		"/v1/units/u1",
		`{"name":"b"}`,
		"key-update-1",
		"3",
	); code != 200 ||
		data["id"] != "u1" {
		t.Fatalf("update: %d %v", code, data)
	}

	if code, _, body := send(
		billing,
		http.MethodPut,
		"/v1/units/u1",
		`{"id":"u2","name":"b"}`,
		"key-update-2",
		"3",
	); code != 422 ||
		!strings.Contains(body, apperr.KeyPathMismatch) {
		t.Fatalf("update mismatch: %d %s", code, body)
	}

	if code, _, body := send(
		billing,
		http.MethodPut,
		"/v1/units/u1",
		`{"name":"b"}`,
		"",
		"3",
	); code != 422 ||
		!strings.Contains(body, "Idempotency-Key") {
		t.Fatalf("write without key: %d %s", code, body)
	}
}

// checkRouteReads: a read with a query parameter, a unit outside the scope reads as absent, and an unknown parameter
// is VALIDATION.
func checkRouteReads(t *testing.T, send routeSender) {
	t.Helper()

	if code, data, _ := send(
		clientPrincipal(),
		http.MethodGet,
		"/v1/units/mine?jobId=j1",
		"",
		"",
		"",
	); code != 200 || data["id"] != ownUnit ||
		data[fieldJobID] != "j1" {
		t.Fatalf("get: %d %v", code, data)
	}

	if code, _, body := send(
		clientPrincipal(),
		http.MethodGet,
		"/v1/units/other",
		"",
		"",
		"",
	); code != 404 ||
		!strings.Contains(body, apperr.KeyNotFound) { // outside the scope reads as absent
		t.Fatalf("get other: %d %s", code, body)
	}

	if code, _, body := send(
		clientPrincipal(),
		http.MethodGet,
		"/v1/units/mine?x=1",
		"",
		"",
		"",
	); code != 422 ||
		!strings.Contains(body, apperr.KeyNotAllowed) {
		t.Fatalf("unknown parameter: %d %s", code, body)
	}
}

// checkRouteDeleteAndVerbs: a DELETE takes its input from the query string, and the verb routes of
// payouts.transition fix the action.
func checkRouteDeleteAndVerbs(t *testing.T, send routeSender, billing *Principal) {
	t.Helper()

	if code, data, _ := send(
		billing,
		http.MethodDelete,
		"/v1/units/u1?reason=duplicate%20entry",
		"",
		"key-delete-1",
		"4",
	); code != 200 ||
		data["reason"] != "duplicate entry" {
		t.Fatalf("delete: %d %v", code, data)
	}

	for path, action := range map[string]string{
		"/v1/payouts/s1/approve":   actionApprove,
		"/v1/payouts/s1/mark-paid": "mark_paid",
	} {
		if code, data, _ := send(
			billing,
			http.MethodPost,
			path,
			``,
			"key-"+action,
			"2",
		); code != 200 || data[fieldAction] != action ||
			data["statementId"] != "s1" {
			t.Fatalf("%s: %d %v", path, code, data)
		}
	}
}

// checkRouteReplay: the same key and input replays the stored result; the same key with other input is CONFLICT.
func checkRouteReplay(t *testing.T, send routeSender, billing *Principal) {
	t.Helper()

	if code, data, _ := send(
		billing,
		http.MethodPost,
		"/v1/units",
		bodyNameA,
		"key-create-1",
		"",
	); code != 200 ||
		data[fieldName] != "a" {
		t.Fatalf("replay: %d %v", code, data)
	}

	if code, _, _ := send(
		billing,
		http.MethodPost,
		"/v1/units",
		`{"name":"z"}`,
		"key-create-1",
		"",
	); code != 409 {
		t.Fatalf("reused key: %d", code)
	}
}
