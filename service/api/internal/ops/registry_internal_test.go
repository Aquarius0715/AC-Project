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

// The operations, fields and values the registry and REST tests share.
const (
	opUnitsSave   = "units.save"
	opUnitsGet    = "units.get"
	fieldName     = "name"
	fieldJobID    = "jobId"
	fieldUnitIDs  = "unitIds"
	ownUnit       = "mine" // the unit id units.get treats as the caller's own
	roleAdmin     = "admin"
	permAssetRead = "asset.read"
	kindUnit      = "unit"
	bodyNameA     = `{"name":"a"}`
	bodyIDX       = `{"id":"x"}`
)

var errBoom = errors.New("boom")

type fakeDB struct{ readOnly *bool }

func (f fakeDB) Run(_ context.Context, readOnly bool, _ *Principal, fn func(pgx.Tx) error) error {
	if f.readOnly != nil {
		*f.readOnly = readOnly
	}

	return fn(nil)
}

type fakeRec struct {
	n   int
	err error
}

func (f *fakeRec) Record(context.Context, pgx.Tx, *Call, string) error {
	f.n++

	return f.err
}

type fakeIdem struct {
	stored  map[string]json.RawMessage
	hash    map[string]string
	aborted int
}

func (f *fakeIdem) Begin(
	_ context.Context,
	_ *Principal,
	_, key, hash string,
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
	_ context.Context,
	_ *Principal,
	_, key string,
	resp json.RawMessage,
) error {
	f.stored[key] = resp

	return nil
}

func (f *fakeIdem) Abort(_ context.Context, _ *Principal, _, key string) {
	f.aborted++
	delete(f.hash, key)
}

type unitIn struct {
	ID   *string `json:"id,omitempty"` // the item route PUT /v1/units/{id} fills it
	Name string  `json:"name"`
}

func (u *unitIn) Validate() map[string]string {
	if u.Name == "bad" {
		return map[string]string{fieldName: apperr.KeyInvalid}
	}

	return nil
}

type out struct {
	OK    bool   `json:"ok"`
	Name  string `json:"name"`
	Calls int    `json:"calls"`
}

// rig is the registry of the pipeline tests with its fakes and its Echo app.
type rig struct {
	reg      *Registry
	app      *echo.Echo
	rec      *fakeRec
	idem     *fakeIdem
	readOnly *bool
}

func setup(t *testing.T) rig {
	t.Helper()

	readOnly := new(bool)
	reg := NewRegistry()
	reg.DB = fakeDB{readOnly: readOnly}
	rec := &fakeRec{n: 0, err: nil}
	idem := &fakeIdem{stored: map[string]json.RawMessage{}, hash: map[string]string{}, aborted: 0}
	reg.Rec, reg.Idem = rec, idem
	reg.Clock = func() time.Time { return time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC) }
	registerTestOperations(reg)

	app := echo.New()

	app.HTTPErrorHandler = HTTPErrorHandler

	err := reg.MountREST(app.Group("/v1"))
	if err != nil {
		t.Fatal(err)
	}

	return rig{reg: reg, app: app, rec: rec, idem: idem, readOnly: readOnly}
}

// registerTestOperations registers units.save (a handler error for the name boom, a conflict for conflict),
// units.get (only the caller's own unit) and attachments.add (a file in the body).
func registerTestOperations(reg *Registry) {
	calls := 0

	Register(reg, opUnitsSave, func(_ context.Context, call *Call, input *unitIn) (out, error) {
		if input.Name == "boom" {
			return out{}, errBoom
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
				Action:          opUnitsSave,
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
	Register(reg, opUnitsGet, func(_ context.Context, call *Call, input *struct {
		ID string `json:"id"`
	},
	) (out, error) {
		err := call.Require(authz.Facts{"self": input.ID == ownUnit})
		if err != nil {
			return out{}, err
		}

		return out{OK: true, Name: "", Calls: 0}, nil
	})
	Register(reg, "attachments.add", func(_ context.Context, _ *Call, input *struct {
		JobID    string `json:"jobId"`
		ReportID string `json:"reportId"`
		File     struct {
			Bytes []byte `json:"bytes"`
		} `json:"file"`
	},
	) (out, error) {
		return out{OK: true, Calls: len(input.File.Bytes), Name: ""}, nil
	})
}

// do sends one operation call at its REST route (ClientRequest).
func do(
	t *testing.T,
	app *echo.Echo,
	principal *Principal,
	op, body, key string,
) *httptest.ResponseRecorder {
	t.Helper()

	method, target, reqBody := ClientRequest(op, body)
	req := httptest.NewRequestWithContext(t.Context(), method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")

	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	if principal != nil {
		req = req.WithContext(WithPrincipal(req.Context(), principal))
	}

	recorder := httptest.NewRecorder()
	app.ServeHTTP(recorder, req)

	return recorder
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) apperr.Code {
	t.Helper()

	var domainError apperr.DomainError

	err := json.Unmarshal(rec.Body.Bytes(), &domainError)
	if err != nil {
		t.Fatalf("not a DomainError: %s", rec.Body.String())
	}

	if domainError.CorrelationID == "" {
		t.Fatal("correlationId missing")
	}

	return domainError.Code
}

// adminPrincipal is an HQ admin who reads and writes assets.
func adminPrincipal() *Principal {
	return &Principal{
		Principal: authz.Principal{
			Role:        roleAdmin,
			Permissions: map[string]bool{"asset.write": true, permAssetRead: true}, ClientRole: "",
		}, TenantID: uuid.UUID{}, MembershipID: uuid.UUID{}, UserID: uuid.UUID{}, OrgID: uuid.UUID{}, ScopeVersion: 0,
		Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
	}
}

// clientPrincipal is a client without permissions.
func clientPrincipal() *Principal {
	return &Principal{
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
}

// technicianPrincipal is a technician without permissions.
func technicianPrincipal() *Principal {
	return &Principal{
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
}

// refusal is a call the pipeline refuses, with the status and the error code it answers.
type refusal struct {
	name, op, body, key string
	principal           *Principal
	status              int
	code                apperr.Code
}

func refusals() []refusal {
	return []refusal{
		{"unknown op", "nope.nope", `{}`, "", adminPrincipal(), 404, apperr.NotFound},
		{"malformed", opUnitsSave, `{`, "k", adminPrincipal(), 422, apperr.Validation},
		{"unknown field", opUnitsSave, `{"x":1}`, "k", adminPrincipal(), 422, apperr.Validation},
		{
			"field rule",
			opUnitsSave,
			`{"name":"bad"}`,
			"k",
			adminPrincipal(),
			422,
			apperr.Validation,
		},
		{"anonymous", opUnitsSave, bodyNameA, "k", nil, 401, apperr.Unauthenticated},
		{
			"no permission",
			opUnitsSave,
			bodyNameA,
			"k",
			&Principal{
				Principal: authz.Principal{
					Role:        roleAdmin,
					Permissions: map[string]bool{permAssetRead: true}, ClientRole: "",
				}, TenantID: uuid.UUID{}, MembershipID: uuid.UUID{}, UserID: uuid.UUID{}, OrgID: uuid.UUID{}, ScopeVersion: 0,
				Scopes: nil, Employment: "", Timezone: "", SessionsRevokedAt: nil,
			},
			403,
			apperr.Forbidden,
		},
		{"missing key", opUnitsSave, bodyNameA, "", adminPrincipal(), 422, apperr.Validation},
		{
			"key too long",
			opUnitsSave,
			bodyNameA,
			strings.Repeat("k", 129),
			adminPrincipal(),
			422,
			apperr.Validation,
		},
		{
			"handler error",
			opUnitsSave,
			`{"name":"boom"}`,
			"key-boom",
			adminPrincipal(),
			503,
			apperr.Unavailable,
		},
		{
			"domain conflict",
			opUnitsSave,
			`{"name":"conflict"}`,
			"key-conf",
			adminPrincipal(),
			409,
			apperr.Conflict,
		},
		{"out of scope", opUnitsGet, `{"id":"other"}`, "", clientPrincipal(), 404, apperr.NotFound},
	}
}

func TestPipeline(t *testing.T) {
	t.Parallel()

	testRig := setup(t)

	// one after the other: the cases share the registry and its fake key store, whose aborts are counted below
	for _, testCase := range refusals() {
		recorder := do(
			t,
			testRig.app,
			testCase.principal,
			testCase.op,
			testCase.body,
			testCase.key,
		)
		if recorder.Code != testCase.status || errCode(t, recorder) != testCase.code {
			t.Errorf("%s: got %d %s", testCase.name, recorder.Code, recorder.Body.String())
		}
	}

	if testRig.idem.aborted != 2 {
		t.Fatalf("failed writes must abort their key, got %d", testRig.idem.aborted)
	}

	checkWriteReplay(t, testRig)
	checkRead(t, testRig)
	checkBodies(t, testRig)
}

// checkWriteReplay: a write succeeds read-write, its idempotent replay returns the stored response, and the same key
// with another body conflicts.
func checkWriteReplay(t *testing.T, testRig rig) {
	t.Helper()

	saved := do(t, testRig.app, adminPrincipal(), opUnitsSave, bodyNameA, "key-0001")
	if saved.Code != 200 || !strings.Contains(saved.Body.String(), `"calls":1`) {
		t.Fatalf("save: %d %s", saved.Code, saved.Body.String())
	}

	if *testRig.readOnly {
		t.Fatal("writes must run read-write")
	}

	replayed := do(t, testRig.app, adminPrincipal(), opUnitsSave, bodyNameA, "key-0001")
	if replayed.Body.String() != saved.Body.String() {
		t.Fatal("replay must return the stored response")
	}

	reused := do(t, testRig.app, adminPrincipal(), opUnitsSave, `{"name":"b"}`, "key-0001")
	if errCode(t, reused) != apperr.Conflict {
		t.Fatal("reused key with another body must conflict")
	}

	if testRig.rec.n != 1 {
		t.Fatalf("recorder runs once per successful write, got %d", testRig.rec.n)
	}
}

// checkRead: reads run read-only, need no key and carry the operation and the correlation ID in meta.
func checkRead(t *testing.T, testRig rig) {
	t.Helper()

	recorder := do(t, testRig.app, clientPrincipal(), opUnitsGet, `{"id":"mine"}`, "")
	if recorder.Code != 200 || !*testRig.readOnly {
		t.Fatalf("read: %d %s ro=%v", recorder.Code, recorder.Body.String(), *testRig.readOnly)
	}

	var res struct {
		Data out            `json:"data"`
		Meta map[string]any `json:"meta"`
	}

	_ = json.Unmarshal(recorder.Body.Bytes(), &res)
	if res.Meta["operation"] != opUnitsGet || res.Meta[metaCorrelationID] == "" {
		t.Fatalf("meta: %v", res.Meta)
	}
}

// checkBodies: an empty body counts as {}, and a body above the limit is refused.
func checkBodies(t *testing.T, testRig rig) {
	t.Helper()

	if recorder := do(
		t,
		testRig.app,
		adminPrincipal(),
		opUnitsSave,
		``,
		"key-empty1",
	); recorder.Code != 200 ||
		!strings.Contains(recorder.Body.String(), `"name":""`) {
		t.Fatalf("empty body: %d %s", recorder.Code, recorder.Body.String())
	}

	if recorder := do(
		t,
		testRig.app,
		adminPrincipal(),
		opUnitsSave,
		`{"name":"`+strings.Repeat("x", 1<<20)+`"}`,
		"key-big1",
	); recorder.Code != 422 {
		t.Fatalf("oversize: %d", recorder.Code)
	}
}

func TestRecorderFailureRollsBack(t *testing.T) {
	t.Parallel()

	testRig := setup(t)
	testRig.rec.err = apperr.E(apperr.Unavailable, apperr.KeyUnavailable)

	if recorder := do(
		t,
		testRig.app,
		adminPrincipal(),
		opUnitsSave,
		bodyNameA,
		"key-0009",
	); recorder.Code != 503 ||
		testRig.idem.aborted != 1 {
		t.Fatalf("%d aborted=%d", recorder.Code, testRig.idem.aborted)
	}
}

func TestRegisterGuards(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func mustPanic(t *testing.T, register func()) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()

	register()
}

// versionSpec is a catalog entry with only the given write version rules.
func versionSpec(rules ...VersionRule) Spec {
	return Spec{
		Versions:      rules,
		Name:          "",
		Mode:          0,
		Module:        "",
		Authorization: "",
		DesignIDs:     "",
		Routes:        nil,
		Filters:       nil,
	}
}

func TestExpectedVersionRules(t *testing.T) {
	t.Parallel()

	idSpec := versionSpec(
		VersionRule{"id omitted", "omit"},
		VersionRule{"id present", ruleRequired},
	)
	eventSpec := versionSpec(
		VersionRule{"event=request", "omit"},
		VersionRule{"event=retry|cancel", ruleRequired},
	)

	for index, testCase := range []struct {
		spec    Spec
		body    string
		header  string
		want    apperr.Code
		version int
	}{
		{idSpec, `{}`, "", "", 0},
		{idSpec, `{"id":null}`, "", "", 0},
		{idSpec, `{}`, "3", apperr.Validation, 0},
		{idSpec, bodyIDX, "", apperr.Validation, 0},
		{idSpec, bodyIDX, "3", "", 3},
		{idSpec, bodyIDX, "0", "", 0},
		{idSpec, bodyIDX, "-1", apperr.Validation, 0},
		{idSpec, bodyIDX, "abc", apperr.Validation, 0},
		{eventSpec, `{"event":"retry"}`, "2", "", 2},
		{eventSpec, `{"event":"request"}`, "2", apperr.Validation, 0},
		{eventSpec, `{"event":"other"}`, "", "", 0},
		{versionSpec(VersionRule{branchAll, ruleRequired}), `{}`, "1", "", 1},
		{versionSpec(VersionRule{"weird", ruleRequired}), `{}`, "", "", 0},
	} {
		got, present, err := expectedVersion(testCase.spec, []byte(testCase.body), testCase.header)

		var code apperr.Code
		if err != nil {
			code = apperr.From(err).Code
		}

		if code != testCase.want ||
			(testCase.version != 0 && (!present || got != testCase.version)) {
			t.Errorf("case %d: %v %v %v", index, got, present, err)
		}
	}
}

func TestVersionHeader(t *testing.T) {
	t.Parallel()

	testRig := setup(t)
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/v1/units",
		strings.NewReader(bodyNameA),
	)
	req.Header.Set("Idempotency-Key", "key-ver-0001")
	req.Header.Set("X-Expected-Version", "4")
	req = req.WithContext(WithPrincipal(req.Context(), adminPrincipal()))
	recorder := httptest.NewRecorder()
	testRig.app.ServeHTTP(recorder, req)

	if recorder.Code != 422 || !strings.Contains(recorder.Body.String(), fieldExpectedVersion) {
		t.Fatalf(
			"version on a create must be rejected: %d %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}
}

// IR221: ordinary operations keep the 1 MiB body limit; operations with BlobInput files accept their larger bodies.
func TestBodyLimits(t *testing.T) {
	t.Parallel()

	testRig := setup(t)
	tech := technicianPrincipal()

	big := strings.Repeat("a", 2<<20)
	if recorder := do(
		t,
		testRig.app,
		adminPrincipal(),
		opUnitsSave,
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

	rec := do(
		t,
		testRig.app,
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
		t,
		testRig.app,
		tech,
		"attachments.add",
		`{"jobId":"j1","reportId":"r1","file":{"bytes":"`+huge+`"}}`,
		"key-34567890",
	); recorder.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(recorder.Body.String(), apperr.KeyBodyTooLarge) {
		t.Fatalf("a body above the file limit: %d", recorder.Code)
	}
}

// TestBodyLimitByOperation: the file operations take more than 1 MiB, a sign-off at least as much as an attachment,
// and certificates.submit a 10 MB file in JSON (IR277).
func TestBodyLimitByOperation(t *testing.T) {
	t.Parallel()

	if bodyLimit("attachments.add") <= 1<<20 ||
		bodyLimit("reports.signOff") < bodyLimit("attachments.add") ||
		bodyLimit(opUnitsSave) != 1<<20 {
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
}

// TestDeniedTarget: a refused write records the target it asked for — the input's id under the kind its resource's
// successful writes use, else its <kind>Id, else another …Id holding a UUID named by that field; nothing else of the
// input (IR603).
func TestDeniedTarget(t *testing.T) {
	t.Parallel()

	const firstID, secondID = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"

	type review struct {
		JobID         string `json:"jobId"`
		ReportVersion int    `json:"reportVersion"`
	}
	for _, testCase := range []struct {
		op       string
		in       any
		kind, id string
	}{
		{"jobs.review", review{JobID: firstID, ReportVersion: 2}, "job", firstID},
		{opUnitsSave, map[string]any{"id": firstID, "customerOrgId": secondID}, kindUnit, firstID},
		{"members.revokeSessions", map[string]any{"membershipId": secondID}, "membership", secondID},
		{
			"members.clearUnavailability",
			map[string]any{"id": firstID, "from": "2026-09-20"},
			"unavailability",
			firstID,
		},
		{"jobs.offer", map[string]any{"contractorOrgId": secondID, fieldJobID: firstID}, "job", firstID},
		{"policies.delete", map[string]any{"policyId": "not-a-uuid"}, "policy", noTargetID},
		{"offsets.request", map[string]any{"quoteId": firstID, fieldUnitIDs: []string{secondID}}, "quote", firstID},
		{
			"spaces.save",
			map[string]any{"propertyId": firstID, "parentSpaceId": nil, fieldName: "2F"},
			"property",
			firstID,
		},
		{"restrictions.override", map[string]any{}, "restriction", noTargetID},
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
