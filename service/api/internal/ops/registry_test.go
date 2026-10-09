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

func (f *fakeIdem) Begin(ctx context.Context, p *Principal, op, key, hash string) (json.RawMessage, error) {
	if h, ok := f.hash[key]; ok {
		if h != hash {
			return nil, apperr.E(apperr.Conflict, "error.idempotencyKeyReused")
		}
		return f.stored[key], nil
	}
	f.hash[key] = hash
	return nil, nil
}
func (f *fakeIdem) Complete(ctx context.Context, p *Principal, op, key string, resp json.RawMessage) error {
	f.stored[key] = resp
	return nil
}
func (f *fakeIdem) Abort(ctx context.Context, p *Principal, op, key string) {
	f.aborted++
	delete(f.hash, key)
}

type unitIn struct {
	Name string `json:"name"`
}

func (u *unitIn) Validate() map[string]string {
	if u.Name == "bad" {
		return map[string]string{"name": "error.invalid"}
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
	ro := new(bool)
	r := NewRegistry()
	r.DB = fakeDB{readOnly: ro}
	rec := &fakeRec{}
	idem := &fakeIdem{stored: map[string]json.RawMessage{}, hash: map[string]string{}}
	r.Rec, r.Idem = rec, idem
	r.Clock = func() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }
	calls := 0
	Register(r, "units.save", func(ctx context.Context, c *Call, in *unitIn) (out, error) {
		if in.Name == "boom" {
			return out{}, errors.New("boom")
		}
		if in.Name == "conflict" {
			return out{}, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		calls++
		c.Emit(Event{Type: "UnitChanged"})
		c.Audit(AuditEntry{Action: "units.save"})
		return out{OK: true, Name: in.Name, Calls: calls}, nil
	})
	Register(r, "units.get", func(ctx context.Context, c *Call, in *struct {
		ID string `json:"id"`
	}) (out, error) {
		if err := c.Require(authz.Facts{"self": in.ID == "mine"}); err != nil {
			return out{}, err
		}
		return out{OK: true}, nil
	})
	e := echo.New()
	e.HTTPErrorHandler = HTTPErrorHandler
	e.POST("/v1/ops/:operation", r.Dispatch)
	return r, e, rec, idem, ro
}

func do(e *echo.Echo, p *Principal, op, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/ops/"+op, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if p != nil {
		req = req.WithContext(WithPrincipal(req.Context(), p))
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func errCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var de apperr.DomainError
	if err := json.Unmarshal(w.Body.Bytes(), &de); err != nil {
		t.Fatalf("not a DomainError: %s", w.Body.String())
	}
	if de.CorrelationID == "" {
		t.Fatal("correlationId missing")
	}
	return string(de.Code)
}

var admin = &Principal{Principal: authz.Principal{Role: "admin", Permissions: map[string]bool{"asset.write": true, "asset.read": true}}}
var client = &Principal{Principal: authz.Principal{Role: "client", Permissions: map[string]bool{}}}

func TestDispatch(t *testing.T) {
	_, e, rec, idem, ro := setup(t)
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
		{"no permission", "units.save", `{"name":"a"}`, "k", &Principal{Principal: authz.Principal{Role: "admin", Permissions: map[string]bool{"asset.read": true}}}, 403, "FORBIDDEN"},
		{"missing key", "units.save", `{"name":"a"}`, "", admin, 422, "VALIDATION"},
		{"key too long", "units.save", `{"name":"a"}`, strings.Repeat("k", 129), admin, 422, "VALIDATION"},
		{"handler error", "units.save", `{"name":"boom"}`, "key-boom", admin, 503, "UNAVAILABLE"},
		{"domain conflict", "units.save", `{"name":"conflict"}`, "key-conf", admin, 409, "CONFLICT"},
		{"out of scope", "units.get", `{"id":"other"}`, "", client, 404, "NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := do(e, c.p, c.op, c.body, c.key)
			if w.Code != c.status || errCode(t, w) != c.code {
				t.Fatalf("got %d %s", w.Code, w.Body.String())
			}
		})
	}
	if idem.aborted != 2 {
		t.Fatalf("failed writes must abort their key, got %d", idem.aborted)
	}

	// success + idempotent replay + key reuse with another body
	w := do(e, admin, "units.save", `{"name":"a"}`, "key-0001")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"calls":1`) {
		t.Fatalf("save: %d %s", w.Code, w.Body.String())
	}
	if *ro {
		t.Fatal("writes must run read-write")
	}
	w2 := do(e, admin, "units.save", `{"name":"a"}`, "key-0001")
	if w2.Body.String() != w.Body.String() {
		t.Fatal("replay must return the stored response")
	}
	if w3 := do(e, admin, "units.save", `{"name":"b"}`, "key-0001"); errCode(t, w3) != "CONFLICT" {
		t.Fatal("reused key with another body must conflict")
	}
	if rec.n != 1 {
		t.Fatalf("recorder runs once per successful write, got %d", rec.n)
	}
	// reads run read-only and do not need a key
	w = do(e, client, "units.get", `{"id":"mine"}`, "")
	if w.Code != 200 || !*ro {
		t.Fatalf("read: %d %s ro=%v", w.Code, w.Body.String(), *ro)
	}
	var res struct {
		Data out            `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Meta["operation"] != "units.get" || res.Meta["correlationId"] == "" {
		t.Fatalf("meta: %v", res.Meta)
	}
	// empty body counts as {}
	if w := do(e, client, "units.get", ``, ""); w.Code != 404 {
		t.Fatalf("empty body: %d", w.Code)
	}
	// oversize body
	if w := do(e, admin, "units.save", `{"name":"`+strings.Repeat("x", 1<<20)+`"}`, "key-big1"); w.Code != 422 {
		t.Fatalf("oversize: %d", w.Code)
	}
}

func TestRecorderFailureRollsBack(t *testing.T) {
	r, e, rec, idem, _ := setup(t)
	rec.err = apperr.E(apperr.Unavailable, "error.unavailable")
	_ = r
	if w := do(e, admin, "units.save", `{"name":"a"}`, "key-0009"); w.Code != 503 || idem.aborted != 1 {
		t.Fatalf("%d aborted=%d", w.Code, idem.aborted)
	}
}

func TestRegisterGuards(t *testing.T) {
	r := NewRegistry()
	mustPanic(t, func() { Register(r, "no.such", func(context.Context, *Call, *struct{}) (int, error) { return 0, nil }) })
	Register(r, "units.list", func(context.Context, *Call, *struct{}) (int, error) { return 0, nil })
	mustPanic(t, func() {
		Register(r, "units.list", func(context.Context, *Call, *struct{}) (int, error) { return 0, nil })
	})
	if !r.Registered()["units.list"] || len(r.Registered()) != 1 {
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
	if len(Catalog) != 199 { // IR214 automations.delete, IR216 commands.list
		t.Fatalf("catalog size %d", len(Catalog))
	}
	for _, s := range Catalog {
		if s.Mode == Write && len(s.Versions) == 0 {
			t.Errorf("%s: write without version rule", s.Name)
		}
		if s.Module == "" {
			t.Errorf("%s: no module", s.Name)
		}
	}
}

func mustPanic(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	f()
}

func TestExpectedVersionRules(t *testing.T) {
	spec := Spec{Versions: []VersionRule{{"id omitted", "omit"}, {"id present", "required"}}}
	ev := Spec{Versions: []VersionRule{{"event=request", "omit"}, {"event=retry|cancel", "required"}}}
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
		{ev, `{"event":"retry"}`, "2", "", 2},
		{ev, `{"event":"request"}`, "2", apperr.Validation, 0},
		{ev, `{"event":"other"}`, "", "", 0},
		{Spec{Versions: []VersionRule{{"all", "required"}}}, `{}`, "1", "", 1},
		{Spec{Versions: []VersionRule{{"weird", "required"}}}, `{}`, "", "", 0},
	}
	for i, c := range cases {
		got, err := expectedVersion(c.s, []byte(c.body), c.header)
		var code apperr.Code
		if err != nil {
			code = apperr.From(err).Code
		}
		if code != c.want || (c.v != 0 && (got == nil || *got != c.v)) {
			t.Errorf("case %d: %v %v", i, got, err)
		}
	}
}

func TestDispatchVersionHeader(t *testing.T) {
	_, e, _, _, _ := setup(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/ops/units.save", strings.NewReader(`{"name":"a"}`))
	req.Header.Set("Idempotency-Key", "key-ver-0001")
	req.Header.Set("X-Expected-Version", "4")
	req = req.WithContext(WithPrincipal(req.Context(), admin))
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "expectedVersion") {
		t.Fatalf("version on a create must be rejected: %d %s", w.Code, w.Body.String())
	}
}

// IR221: ordinary operations keep the 1 MiB body limit; operations with BlobInput files accept their larger bodies.
func TestDispatchBodyLimits(t *testing.T) {
	r, e, _, _, _ := setup(t)
	tech := &Principal{Principal: authz.Principal{Role: "technician", Permissions: map[string]bool{}}}
	Register(r, "attachments.add", func(ctx context.Context, c *Call, in *struct {
		File struct {
			Bytes []byte `json:"bytes"`
		} `json:"file"`
	}) (out, error) {
		return out{OK: true, Calls: len(in.File.Bytes)}, nil
	})
	big := strings.Repeat("a", 2<<20)
	if w := do(e, admin, "units.save", `{"name":"`+big+`"}`, "key-12345678"); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "error.bodyTooLarge") {
		t.Fatalf("2 MiB to an ordinary operation: %d %s", w.Code, w.Body.String()[:120])
	}
	photo := base64.StdEncoding.EncodeToString(make([]byte, 4<<20)) // a 4 MiB photo is about 5.3 MiB in JSON
	if bodyLimit("attachments.add") <= 1<<20 || bodyLimit("reports.signOff") < bodyLimit("attachments.add") || bodyLimit("units.save") != 1<<20 {
		t.Fatal("limits by operation")
	}
	w := do(e, tech, "attachments.add", `{"file":{"bytes":"`+photo+`"}}`, "key-23456789")
	if w.Code == http.StatusUnprocessableEntity && strings.Contains(w.Body.String(), "error.bodyTooLarge") {
		t.Fatalf("a 4 MiB photo must pass the body limit: %d", w.Code)
	}
	huge := base64.StdEncoding.EncodeToString(make([]byte, 7<<20))
	if w := do(e, tech, "attachments.add", `{"file":{"bytes":"`+huge+`"}}`, "key-34567890"); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "error.bodyTooLarge") {
		t.Fatalf("a body above the file limit: %d", w.Code)
	}
}
