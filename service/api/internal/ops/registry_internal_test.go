package ops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

type fakeDB struct{ readOnly *bool }

func (f fakeDB) Run(ctx context.Context, ro bool, p *Principal, fn func(pgx.Tx) error) error {
	if f.readOnly != nil {
		*f.readOnly = ro
	}

	return fn(nil)
}

type fakeRec struct {
	n   int
	err error
}

func (f *fakeRec) Record(ctx context.Context, tx pgx.Tx, c *Call, op string) error {
	f.n++

	return f.err
}

type fakeIdem struct {
	stored  map[string]json.RawMessage
	hash    map[string]string
	aborted int
}

func (f *fakeIdem) Begin(
	ctx context.Context,
	p *Principal,
	op, key, hash string,
) (json.RawMessage, error) {
	if h, ok := f.hash[key]; ok {
		if h != hash {
			return nil, apperr.E(apperr.Conflict, "error.idempotencyKeyReused")
		}

		return f.stored[key], nil
	}

	f.hash[key] = hash

	return nil, nil
}

func (f *fakeIdem) Complete(
	ctx context.Context,
	p *Principal,
	op, key string,
	resp json.RawMessage,
) error {
	f.stored[key] = resp

	return nil
}

func (f *fakeIdem) Abort(ctx context.Context, p *Principal, op, key string) {
	f.aborted++
	delete(f.hash, key)
}

type unitIn struct {
	ID   *string `json:"id,omitempty"` // the item route PUT /v1/units/{id} fills it
	Name string  `json:"name"`
}

func (u *unitIn) Validate() map[string]string {
	if u.Name == "bad" {
		return map[string]string{"name": apperr.KeyInvalid}
	}

	return nil
}

type out struct {
	OK    bool   `json:"ok"`
	Name  string `json:"name"`
	Calls int    `json:"calls"`
}

func setup(t *testing.T) (*Registry, *echo.Echo, *fakeRec, *fakeIdem, *bool) {
	t.Helper()

	readOnly := new(bool)
	reg := NewRegistry()
	reg.DB = fakeDB{readOnly: readOnly}
	rec := &fakeRec{n: 0, err: nil}
	idem := &fakeIdem{stored: map[string]json.RawMessage{}, hash: map[string]string{}, aborted: 0}
	reg.Rec, reg.Idem = rec, idem
	reg.Clock = func() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }
	calls := 0

	Register(reg, "units.save", func(ctx context.Context, call *Call, input *unitIn) (out, error) {
		if input.Name == "boom" {
			return out{}, errors.New("boom")
		}

		if input.Name == "conflict" {
			return out{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		calls++

		call.Emit(
			Event{Type: "UnitChanged", AggregateType: "", AggregateID: uuid.UUID{}, Payload: nil},
		)
		call.Audit(
			AuditEntry{
				Action:          "units.save",
				TargetKind:      "",
				TargetID:        "",
				PreviousVersion: nil,
				NextVersion:     nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return out{OK: true, Name: input.Name, Calls: calls}, nil
	})
	Register(reg, "units.get", func(ctx context.Context, c *Call, in *struct {
		ID string `json:"id"`
	},
	) (out, error) {
		err := c.Require(authz.Facts{"self": in.ID == "mine"})
		if err != nil {
			return out{}, err
		}

		return out{OK: true, Name: "", Calls: 0}, nil
	})
	Register(reg, "attachments.add", func(ctx context.Context, c *Call, input *struct {
		JobID    string `json:"jobId"`
		ReportID string `json:"reportId"`
		File     struct {
			Bytes []byte `json:"bytes"`
		} `json:"file"`
	},
	) (out, error) {
		return out{OK: true, Calls: len(input.File.Bytes), Name: ""}, nil
	})

	app := echo.New()

	app.HTTPErrorHandler = HTTPErrorHandler

	err := reg.MountREST(app.Group("/v1"))
	if err != nil {
		t.Fatal(err)
	}

	return reg, app, rec, idem, readOnly
}

// do sends one operation call at its REST route (ClientRequest).
func do(app *echo.Echo, principal *Principal, op, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(ClientRequest(op, body))
	req.Header.Set("Content-Type", "application/json")

	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	if principal != nil {
		req = req.WithContext(WithPrincipal(req.Context(), principal))
	}

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	return w
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var domainError apperr.DomainError

	err := json.Unmarshal(rec.Body.Bytes(), &domainError)
	if err != nil {
		t.Fatalf("not a DomainError: %s", rec.Body.String())
	}

	if domainError.CorrelationID == "" {
		t.Fatal("correlationId missing")
	}

	return string(domainError.Code)
}

var (
	admin = &Principal{
		Principal: authz.Principal{
			Role:        "admin",
			Permissions: map[string]bool{"asset.write": true, "asset.read": true}, ClientRole: "",
		}, TenantID: uuid.UUID{}, MembershipID: uuid.UUID{}, UserID: uuid.UUID{}, OrgID: uuid.UUID{}, ScopeVersion: 0,
		Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
	}
	client = &Principal{
		Principal: authz.Principal{
			Role:        "client",
			Permissions: map[string]bool{},
			ClientRole:  "",
		},
		TenantID:          uuid.UUID{},
		MembershipID:      uuid.UUID{},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}
)

func TestPipeline(t *testing.T) {
	_, app, rec, idem, ro := setup(t)

	cases := []struct {
		name, op, body, key string
		p                   *Principal
		status              int
		code                string
	}{
		{"unknown op", "nope.nope", `{}`, "", admin, 404, "NOT_FOUND"},
		{"malformed", "units.save", `{`, "k", admin, 422, "VALIDATION"},
		{"unknown field", "units.save", `{"x":1}`, "k", admin, 422, "VALIDATION"},
		{"field rule", "units.save", `{"name":"bad"}`, "k", admin, 422, "VALIDATION"},
		{"anonymous", "units.save", `{"name":"a"}`, "k", nil, 401, "UNAUTHENTICATED"},
		{
			"no permission",
			"units.save",
			`{"name":"a"}`,
			"k",
			&Principal{
				Principal: authz.Principal{
					Role:        "admin",
					Permissions: map[string]bool{"asset.read": true}, ClientRole: "",
				}, TenantID: uuid.UUID{}, MembershipID: uuid.UUID{}, UserID: uuid.UUID{}, OrgID: uuid.UUID{}, ScopeVersion: 0,
				Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
			},
			403,
			"FORBIDDEN",
		},
		{"missing key", "units.save", `{"name":"a"}`, "", admin, 422, "VALIDATION"},
		{
			"key too long",
			"units.save",
			`{"name":"a"}`,
			strings.Repeat("k", 129),
			admin,
			422,
			"VALIDATION",
		},
		{"handler error", "units.save", `{"name":"boom"}`, "key-boom", admin, 503, "UNAVAILABLE"},
		{
			"domain conflict",
			"units.save",
			`{"name":"conflict"}`,
			"key-conf",
			admin,
			409,
			"CONFLICT",
		},
		{"out of scope", "units.get", `{"id":"other"}`, "", client, 404, "NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := do(app, c.p, c.op, c.body, c.key)
			if w.Code != c.status || errCode(t, w) != c.code {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}

	if idem.aborted != 2 {
		t.Fatalf("failed writes must abort their key, got %d", idem.aborted)
	}

	// success + idempotent replay + key reuse with another body
	saved := do(app, admin, "units.save", `{"name":"a"}`, "key-0001")
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"calls":1`) {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}

	if *ro {
		t.Fatal("writes must run read-write")
	}

	w2 := do(app, admin, "units.save", `{"name":"a"}`, "key-0001")
	if w2.Body.String() != saved.Body.String() {
		t.Fatal("replay must return the stored response")
	}

	if w3 := do(
		app,
		admin,
		"units.save",
		`{"name":"b"}`,
		"key-0001",
	); errCode(
		t,
		w3,
	) != "CONFLICT" {
		t.Fatal("reused key with another body must conflict")
	}

	if rec.n != 1 {
		t.Fatalf("recorder runs once per successful write, got %d", rec.n)
	}
	// reads run read-only and do not need a key
	saved = do(app, client, "units.get", `{"id":"mine"}`, "")
	if saved.Code != 200 || !*ro {
		t.Fatalf("read: %d %s ro=%v", saved.Code, saved.Body.String(), *ro)
	}

	var res struct {
		Data out            `json:"data"`
		Meta map[string]any `json:"meta"`
	}

	_ = json.Unmarshal(saved.Body.Bytes(), &res)
	if res.Meta["operation"] != "units.get" || res.Meta["correlationId"] == "" {
		t.Fatalf("meta: %v", res.Meta)
	}
	// an empty body counts as {}
	if recorder := do(
		app,
		admin,
		"units.save",
		``,
		"key-empty1",
	); recorder.Code != 200 ||
		!strings.Contains(recorder.Body.String(), `"name":""`) {
		t.Fatalf("empty body: %d %s", recorder.Code, recorder.Body.String())
	}
	// oversize body
	if recorder := do(
		app,
		admin,
		"units.save",
		`{"name":"`+strings.Repeat("x", 1<<20)+`"}`,
		"key-big1",
	); recorder.Code != 422 {
		t.Fatalf("oversize: %d", recorder.Code)
	}
}

func TestRecorderFailureRollsBack(t *testing.T) {
	r, e, rec, idem, _ := setup(t)
	rec.err = apperr.E(apperr.Unavailable, apperr.KeyUnavailable)
	_ = r

	if recorder := do(
		e,
		admin,
		"units.save",
		`{"name":"a"}`,
		"key-0009",
	); recorder.Code != 503 ||
		idem.aborted != 1 {
		t.Fatalf("%d aborted=%d", recorder.Code, idem.aborted)
	}
}

func TestRegisterGuards(t *testing.T) {
	reg := NewRegistry()

	mustPanic(
		t,
		func() {
			Register(
				reg,
				"no.such",
				func(context.Context, *Call, *struct{}) (int, error) { return 0, nil },
			)
		},
	)
	Register(
		reg,
		"units.list",
		func(context.Context, *Call, *struct{}) (int, error) { return 0, nil },
	)
	mustPanic(t, func() {
		Register(
			reg,
			"units.list",
			func(context.Context, *Call, *struct{}) (int, error) { return 0, nil },
		)
	})

	if !reg.Registered()["units.list"] || len(reg.Registered()) != 1 {
		t.Fatal("Registered")
	}

	if Read.String() != "read" || Write.String() != "write" {
		t.Fatal("Mode.String")
	}

	if PrincipalFrom(context.Background()) != nil {
		t.Fatal("no principal")
	}
}

func TestCatalogShape(t *testing.T) {
	if len(
		Catalog,
	) != 212 {
		// IR612 members.unavailability + members.clearUnavailability, IR437 jobs.getAttachment, IR436
		// certificates.getContent, IR214 automations.delete, IR216 commands.list, IR238 filterCare.getSettings, IR327
		// alerts.evidence, IR349 attachments.list, IR379 notifications.markAllRead, IR384 members.revokeSessions, IR416
		// partnerUsers.*
		t.Fatalf("catalog size %d", len(Catalog))
	}

	for _, spec := range Catalog {
		if spec.Mode == Write && len(spec.Versions) == 0 {
			t.Errorf("%s: write without version rule", spec.Name)
		}

		if spec.Module == "" {
			t.Errorf("%s: no module", spec.Name)
		}
	}
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	fn()
}

func TestExpectedVersionRules(t *testing.T) {
	spec := Spec{
		Versions:      []VersionRule{{"id omitted", "omit"}, {"id present", "required"}},
		Name:          "",
		Mode:          0,
		Module:        "",
		Authorization: "",
		DesignIDs:     "",
		Routes:        nil,
		Filters:       nil,
	}
	eventSpec := Spec{
		Versions: []VersionRule{
			{"event=request", "omit"},
			{"event=retry|cancel", "required"},
		},
		Name:          "",
		Mode:          0,
		Module:        "",
		Authorization: "",
		DesignIDs:     "",
		Routes:        nil,
		Filters:       nil,
	}

	cases := []struct {
		s      Spec
		body   string
		header string
		want   apperr.Code
		v      int
	}{
		{spec, `{}`, "", "", 0},
		{spec, `{"id":null}`, "", "", 0},
		{spec, `{}`, "3", apperr.Validation, 0},
		{spec, `{"id":"x"}`, "", apperr.Validation, 0},
		{spec, `{"id":"x"}`, "3", "", 3},
		{spec, `{"id":"x"}`, "0", "", 0},
		{spec, `{"id":"x"}`, "-1", apperr.Validation, 0},
		{spec, `{"id":"x"}`, "abc", apperr.Validation, 0},
		{eventSpec, `{"event":"retry"}`, "2", "", 2},
		{eventSpec, `{"event":"request"}`, "2", apperr.Validation, 0},
		{eventSpec, `{"event":"other"}`, "", "", 0},
		{
			Spec{
				Versions:      []VersionRule{{"all", "required"}},
				Name:          "",
				Mode:          0,
				Module:        "",
				Authorization: "",
				DesignIDs:     "",
				Routes:        nil,
				Filters:       nil,
			},
			`{}`,
			"1",
			"",
			1,
		},
		{
			Spec{
				Versions:      []VersionRule{{"weird", "required"}},
				Name:          "",
				Mode:          0,
				Module:        "",
				Authorization: "",
				DesignIDs:     "",
				Routes:        nil,
				Filters:       nil,
			},
			`{}`,
			"",
			"",
			0,
		},
	}
	for i, testCase := range cases {
		got, present, err := expectedVersion(testCase.s, []byte(testCase.body), testCase.header)

		var code apperr.Code
		if err != nil {
			code = apperr.From(err).Code
		}

		if code != testCase.want || (testCase.v != 0 && (!present || got != testCase.v)) {
			t.Errorf("case %d: %v %v %v", i, got, present, err)
		}
	}
}

func TestVersionHeader(t *testing.T) {
	_, app, _, _, _ := setup(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/units", strings.NewReader(`{"name":"a"}`))
	req.Header.Set("Idempotency-Key", "key-ver-0001")
	req.Header.Set("X-Expected-Version", "4")
	req = req.WithContext(WithPrincipal(req.Context(), admin))
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != 422 || !strings.Contains(w.Body.String(), "expectedVersion") {
		t.Fatalf("version on a create must be rejected: %d %s", w.Code, w.Body.String())
	}
}

// IR221: ordinary operations keep the 1 MiB body limit; operations with BlobInput files accept their larger bodies.
func TestBodyLimits(t *testing.T) {
	_, app, _, _, _ := setup(t)
	tech := &Principal{
		Principal: authz.Principal{
			Role:        "technician",
			Permissions: map[string]bool{},
			ClientRole:  "",
		},
		TenantID:          uuid.UUID{},
		MembershipID:      uuid.UUID{},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}

	big := strings.Repeat("a", 2<<20)
	if recorder := do(
		app,
		admin,
		"units.save",
		`{"name":"`+big+`"}`,
		"key-12345678",
	); recorder.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(recorder.Body.String(), apperr.KeyBodyTooLarge) {
		t.Fatalf(
			"2 MiB to an ordinary operation: %d %s",
			recorder.Code,
			recorder.Body.String()[:120],
		)
	}

	photo := base64.StdEncoding.EncodeToString(
		make([]byte, 4<<20),
	) // a 4 MiB photo is about 5.3 MiB in JSON

	if bodyLimit("attachments.add") <= 1<<20 ||
		bodyLimit("reports.signOff") < bodyLimit("attachments.add") ||
		bodyLimit("units.save") != 1<<20 {
		t.Fatal("limits by operation")
	}

	if cert := int64(
		len(base64.StdEncoding.EncodeToString(make([]byte, 10*1000*1000))),
	) + 4<<10; bodyLimit(
		"certificates.submit",
	) < cert { // IR277: a 10 MB certificate file
		t.Fatalf(
			"certificates.submit limit %d below a 10 MB file in JSON (%d)",
			bodyLimit("certificates.submit"),
			cert,
		)
	}

	rec := do(
		app,
		tech,
		"attachments.add",
		`{"jobId":"j1","reportId":"r1","file":{"bytes":"`+photo+`"}}`,
		"key-23456789",
	)
	if rec.Code == http.StatusUnprocessableEntity &&
		strings.Contains(rec.Body.String(), apperr.KeyBodyTooLarge) {
		t.Fatalf("a 4 MiB photo must pass the body limit: %d", rec.Code)
	}

	huge := base64.StdEncoding.EncodeToString(make([]byte, 7<<20))
	if recorder := do(
		app,
		tech,
		"attachments.add",
		`{"jobId":"j1","reportId":"r1","file":{"bytes":"`+huge+`"}}`,
		"key-34567890",
	); recorder.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(recorder.Body.String(), apperr.KeyBodyTooLarge) {
		t.Fatalf("a body above the file limit: %d", recorder.Code)
	}
}

// TestDeniedTarget: a refused write records the target it asked for — the input's id under the kind its resource's
// successful writes use, else its <kind>Id, else another …Id holding a UUID named by that field; nothing else of the
// input (IR603).
func TestDeniedTarget(t *testing.T) {
	const a, b = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"

	type review struct {
		JobID         string `json:"jobId"`
		ReportVersion int    `json:"reportVersion"`
	}
	for _, testCase := range []struct {
		op       string
		in       any
		kind, id string
	}{
		{"jobs.review", review{JobID: a, ReportVersion: 2}, "job", a},
		{"units.save", map[string]any{"id": a, "customerOrgId": b}, "unit", a},
		{"members.revokeSessions", map[string]any{"membershipId": b}, "membership", b},
		{"members.clearUnavailability", map[string]any{"id": a, "from": "2026-09-20"}, "unavailability", a},
		{"jobs.offer", map[string]any{"contractorOrgId": b, "jobId": a}, "job", a},
		{"policies.delete", map[string]any{"policyId": "not-a-uuid"}, "policy", "—"},
		{"offsets.request", map[string]any{"quoteId": a, "unitIds": []string{b}}, "quote", a},
		{"spaces.save", map[string]any{"propertyId": a, "parentSpaceId": nil, "name": "2F"}, "property", a},
		{"restrictions.override", map[string]any{}, "restriction", "—"},
	} {
		if kind, id := deniedTarget(
			testCase.op,
			testCase.in,
		); kind != testCase.kind ||
			id != testCase.id {
			t.Errorf("%s: %s %s, want %s %s", testCase.op, kind, id, testCase.kind, testCase.id)
		}
	}
}
