package integration_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// write posts with a fresh idempotency key and an optional expected version.
// versionZero sends X-Expected-Version: 0 (an absent row); 0 itself omits the header.
const versionZero = -100

func write(srv *apiserver.Server, who *actor, op, body string, version int) (int, map[string]any) {
	method, target, reqBody := ops.ClientRequest(op, body)
	req := httptest.NewRequestWithContext(context.Background(), method, target, reqBody)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+who.token)
	req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
	req.Header.Set("X-Membership-Id", seed.ID(who.membership).String())
	req.Header.Set("Idempotency-Key", "test-"+uuid.NewString())

	if version > 0 {
		req.Header.Set("X-Expected-Version", strconv.Itoa(version))
	}

	if version == versionZero {
		req.Header.Set("X-Expected-Version", "0")
	}

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	var m map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &m)

	return rec.Code, m
}

func data(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)

	return d
}

func ver(t *testing.T, m map[string]any) int {
	t.Helper()

	return int(asNum(t, data(m)["version"]))
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAssetWritesLifecycle(t *testing.T) {
	srv := server(t)
	org := seed.ID("org-customer-b").String()
	model := seed.ID("ventilation-demo").String()

	// property create / validation / update / conflict
	propName := "Test HQ " + uuid.NewString()[:6]

	code, resp := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"office","name":"`+propName+`","address":null,"accessInstructions":null}`,
		0,
	)
	if code != 200 || ver(t, resp) != 1 {
		t.Fatalf("property create: %d %v", code, resp)
	}

	prop := asStr(t, data(resp)["id"])
	// names are unique among non-archived siblings, trimmed and case-insensitive (IR208); the duplicate is rolled back
	if code, res := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+org+`","kind":"home","name":" `+strings.ToLower(
			propName,
		)+`","address":null,"accessInstructions":null}`,
		0,
	); code != 409 ||
		res["messageKey"] != apperr.KeyDuplicateSiblingName {
		t.Errorf("duplicate property name: %d %v", code, res)
	}

	for bad, field := range map[string]string{
		`{"customerOrgId":"` + org + `","kind":"castle","name":"x","address":null,` +
			`"accessInstructions":null}`: fieldKind,
		`{"customerOrgId":"` + org + `","kind":"home","name":"","address":null,` +
			`"accessInstructions":null}`: fieldName,
		`{"customerOrgId":"` + org + `","kind":"home",` +
			`"name":"` + strings.Repeat("x", 121) + `","address":null,` +
			`"accessInstructions":null}`: fieldName,
		`{"customerOrgId":"` + org + `","kind":"home","name":"x",` +
			`"address":"` + strings.Repeat("a", 501) + `","accessInstructions":null}`: "address",
		`{"kind":"home","name":"x","address":null,` +
			`"accessInstructions":null}`: "customerOrgId",
	} {
		if code, res := write(
			srv,
			hq(),
			"properties.save",
			bad,
			0,
		); code != 422 || len(asObj(t, res["fieldErrors"])) != 1 ||
			asObj(t, res["fieldErrors"])[field] == nil {
			t.Errorf(
				"property validation %s: %d %v, want %s",
				bad[:40],
				code,
				res["fieldErrors"],
				field,
			)
		}
	}

	if code, _ := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+uuid.NewString()+`","kind":"home","name":"x","address":null,"accessInstructions":null}`,
		0,
	); code != 404 {
		t.Errorf("unknown customer: %d", code)
	}

	renamed := "Renamed " + uuid.NewString()[:6]

	upd := `{"id":"` + prop + `","customerOrgId":"` + org + `","kind":"office",` +
		`"name":"` + renamed + `","address":"1 ` +
		`Test Rd (fictional)","accessInstructions":null}`
	if code, res := write(
		srv,
		hq(),
		"properties.save",
		upd,
		1,
	); code != 200 || ver(t, res) != 2 ||
		data(res)["name"] != renamed {
		t.Fatalf("property update: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"properties.save",
		upd,
		1,
	); code != 409 ||
		res["code"] != codeConflict {
		t.Fatalf("stale version: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"properties.save",
		upd,
		0,
	); code != 422 ||
		fields(res) != "expectedVersion=error.required" {
		t.Fatalf("update without version: %d %s", code, fields(res))
	}

	if code, _ := write(srv, customerB(), "properties.save", upd, 2); code != 403 {
		t.Fatalf("client cannot save properties: %d", code)
	}

	// spaces: floor → room, cycle, other property
	code, resp = write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+prop+`","parentSpaceId":null,"kind":"floor","name":"1F"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("floor: %d %v", code, resp)
	}

	floor := asStr(t, data(resp)["id"])

	code, resp = write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"Server room"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("room: %d", code)
	}

	room := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"spaces.save",
		`{"id":"`+floor+`","propertyId":"`+prop+`","parentSpaceId":"`+room+`","kind":"floor","name":"1F"}`,
		1,
	); code != 422 ||
		fields(res) != "parentSpaceId=error.cycle" {
		t.Errorf("cycle: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"spaces.save",
		`{"id":"`+floor+`","propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"floor","name":"1F"}`,
		1,
	); code != 422 ||
		fields(res) != "parentSpaceId=error.cycle" {
		t.Errorf("self parent: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+prop+`","parentSpaceId":"`+seed.ID("floor-1").
			String()+
			`","kind":"room","name":"x"}`,
		0,
	); code != 422 ||
		fields(res) != "parentSpaceId=error.otherProperty" {
		t.Errorf("parent in another property: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"spaces.save",
		`{"id":"`+room+`","propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"Server room A"}`,
		1,
	); code != 200 ||
		ver(t, res) != 2 {
		t.Fatalf("space update: %d", code)
	}

	for name, body := range map[string]string{
		"floor at the root": `{"propertyId":"` + prop + `","parentSpaceId":null,"kind":"floor","name":"1f"}`,
		"room on the floor": `{"propertyId":"` + prop + `","parentSpaceId":"` + floor + `","kind":"room","name":" server ` +
			`room a "}`,
	} {
		if code, res := write(
			srv,
			hq(),
			"spaces.save",
			body,
			0,
		); code != 409 ||
			res["messageKey"] != apperr.KeyDuplicateSiblingName {
			t.Errorf("duplicate space (%s): %d %v", name, code, res)
		}
	}

	code, resp = write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"1F"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("the same name under another parent is allowed: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"spaces.archive",
		`{"id":"`+asStr(t, data(resp)["id"])+`","reason":"test cleanup"}`,
		1,
	); code != 200 {
		t.Errorf("room 1F archive: %d", code)
	}

	// units: create, wrong customer, unknown model, future install, relocation reason
	unitBody := func(unitID, space string, extra string) string {
		spaceJSON := jsonNull
		if space != "" {
			spaceJSON = `"` + space + `"`
		}

		idf := ""
		if unitID != "" {
			idf = `"id":"` + unitID + `",`
		}

		return `{` + idf + `"customerOrgId":"` + org + `","propertyId":"` + prop + `","spaceId":` + spaceJSON +
			`,"displayName":"Server AC","modelId":"` + model +
			`","type":"split","installedAt":null,"serviceScope":["indoor","outdoor"]` + extra + `}`
	}

	code, resp = write(srv, hq(), "units.save", unitBody("", room, ""), 0)
	if code != 200 || data(resp)["displayName"] != "Server AC" ||
		int(asNum(t, data(resp)["capabilityVersion"])) != 3 {
		t.Fatalf("unit create: %d %v", code, resp)
	}

	unit := asStr(t, data(resp)["id"])

	if code, res := write(
		srv,
		hq(),
		"units.save",
		strings.Replace(unitBody("", room, ""), "Server AC", "server ac", 1),
		0,
	); code != 409 ||
		res["messageKey"] != apperr.KeyDuplicateSiblingName {
		t.Errorf("duplicate unit name in the room: %d %v", code, res)
	}

	badUnits := map[string]string{
		caseOtherCustomer: strings.Replace(
			unitBody("", "", ""),
			org,
			seed.ID("org-customer-a").String(),
			1,
		),
		caseUnknownModel: strings.Replace(unitBody("", "", ""), model, uuid.NewString(), 1),
		"future install": strings.Replace(
			unitBody("", "", ""),
			`"installedAt":null`,
			`"installedAt":"2030-01-01T00:00:00Z"`,
			1,
		),
		"bad scope": strings.Replace(
			unitBody("", "", ""),
			`["indoor","outdoor"]`,
			`["roof"]`,
			1,
		),
		"dup scope": strings.Replace(
			unitBody("", "", ""),
			`["indoor","outdoor"]`,
			`["indoor","indoor"]`,
			1,
		),
		"empty scope":     strings.Replace(unitBody("", "", ""), `["indoor","outdoor"]`, `[]`, 1),
		"space elsewhere": unitBody("", seed.ID("room-1").String(), ""),
	}
	want := map[string]string{ // the field each case is refused for
		"bad scope":       "serviceScope=error.invalid",
		"dup scope":       "serviceScope=error.invalid",
		"empty scope":     "serviceScope=error.required",
		"future install":  "installedAt=error.future",
		caseOtherCustomer: "propertyId=error.otherCustomer",
		"space elsewhere": "spaceId=error.otherProperty",
		caseUnknownModel:  "modelId=error.unknownModel",
	}

	for name, b := range badUnits {
		code, m := write(srv, hq(), "units.save", b, 0)
		refusal(t, name, code, m, want[name])
	}

	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(unit, floor, ""),
		1,
	); code != 422 ||
		fields(res) != "changeReason=error.required" {
		t.Errorf("relocation without reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(unit, floor, `,"changeReason":"moved to floor"`),
		1,
	); code != 200 ||
		ver(t, res) != 2 {
		t.Fatalf("relocate: %d %v", code, res)
	}
	// renaming another unit on the floor to the moved unit's name is a duplicate too (update path)
	code, resp = write(
		srv,
		hq(),
		"units.save",
		strings.Replace(unitBody("", floor, ""), "Server AC", "Server AC 2", 1),
		0,
	)
	if code != 200 {
		t.Fatalf("second unit: %d %v", code, resp)
	}

	second := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(second, floor, ""),
		1,
	); code != 409 ||
		res["messageKey"] != apperr.KeyDuplicateSiblingName {
		t.Errorf("rename to a sibling's name: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"units.archive",
		`{"id":"`+second+`","reason":"test cleanup"}`,
		1,
	); code != 200 {
		t.Errorf("second unit archive: %d", code)
	}
	// warranty end (IR209): set, kept when omitted, cleared with null, never before the installation
	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(unit, floor, `,"warrantyEndsAt":"2027-01-01T00:00:00+08:00"`),
		2,
	); code != 200 ||
		data(res)["warrantyEndsAt"] != "2026-12-31T16:00:00Z" {
		t.Fatalf("warranty set: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(unit, floor, ""),
		3,
	); code != 200 ||
		data(res)["warrantyEndsAt"] != "2026-12-31T16:00:00Z" {
		t.Fatalf("warranty kept when omitted: %d %v", code, data(res)["warrantyEndsAt"])
	}

	early := strings.Replace(
		unitBody(unit, floor, `,"warrantyEndsAt":"2026-08-01T00:00:00+08:00"`),
		`"installedAt":null`,
		`"installedAt":"2026-09-01T00:00:00+08:00"`,
		1,
	)
	if code, res := write(
		srv,
		hq(),
		"units.save",
		early,
		4,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["warrantyEndsAt"] != apperr.KeyRange {
		t.Errorf("warranty before installation: %d %v", code, res)
	}
	// the stored warranty end counts too (an installation in 2027 would be refused as future, so the end moves back)
	owner(
		t,
		`UPDATE assets.units SET warranty_ends_at = '2026-08-01T00:00:00+08:00' WHERE id = $1`,
		unit,
	)

	if code, res := write(
		srv,
		hq(),
		"units.save",
		strings.Replace(
			unitBody(unit, floor, ""),
			`"installedAt":null`,
			`"installedAt":"2026-09-01T00:00:00+08:00"`,
			1,
		),
		4,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["warrantyEndsAt"] != apperr.KeyRange {
		t.Errorf("an installation after the stored warranty end: %d %v", code, res)
	}

	if code, res := write(
		srv,
		hq(),
		"units.save",
		unitBody(unit, floor, `,"warrantyEndsAt":null`),
		4,
	); code != 200 ||
		data(res)["warrantyEndsAt"] != nil {
		t.Fatalf("warranty cleared: %d %v", code, data(res)["warrantyEndsAt"])
	}
	// the history keeps the original location: only the changed field, with its old and new value (DD-A02 step 3)
	day := `"from":"` + clock().Add(-time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Add(time.Hour).
		Format(time.RFC3339) +
		`"`
	_, audit := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"targetKind":"unit","targetId":"`+unit+`"},"limit":10}`,
	)

	var moved map[string]any

	for _, a := range items(t, audit) {
		if a["nextVersion"] == float64(2) {
			moved = a
		}
	}

	if moved == nil || moved["reason"] != "moved to floor" ||
		asObj(t, moved["maskedBefore"])["spaceId"] != room ||
		asObj(t, moved["maskedAfter"])["spaceId"] != floor ||
		len(asObj(t, moved["maskedAfter"])) != 1 {
		t.Fatalf("relocation audit: %v", audit)
	}

	if code, _ := write(
		srv,
		hq(),
		"units.save",
		unitBody(uuid.NewString(), "", ""),
		1,
	); code != 404 {
		t.Errorf("unknown unit: %d", code)
	}

	// archive rules
	if code, _ := write(
		srv,
		hq(),
		"spaces.archive",
		`{"id":"`+floor+`","reason":"x"}`,
		2,
	); code != 409 {
		t.Errorf("space with units/children must not archive: %d", code)
	}

	if code, res := write(
		srv,
		hq(),
		"units.archive",
		`{"id":"`+unit+`","reason":""}`,
		5,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("archive reason required: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"units.archive",
		`{"id":"`+unit+`","reason":"decommissioned"}`,
		5,
	); code != 200 ||
		data(res)["archived"] != true {
		t.Fatalf("unit archive: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"units.archive",
		`{"id":"`+unit+`","reason":"again"}`,
		6,
	); code != 409 {
		t.Errorf("archive twice: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"spaces.archive",
		`{"id":"`+room+`","reason":"empty"}`,
		2,
	); code != 200 {
		t.Errorf("room archive: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"spaces.archive",
		`{"id":"`+floor+`","reason":"empty"}`,
		1,
	); code != 200 {
		t.Errorf("floor archive: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"properties.archive",
		`{"id":"`+prop+`","reason":"closed"}`,
		2,
	); code != 200 {
		t.Errorf("property archive: %d", code)
	}

	if code, _ := write(
		srv,
		hq(),
		"spaces.save",
		`{"propertyId":"`+prop+`","parentSpaceId":null,"kind":"floor","name":"2F"}`,
		0,
	); code != 404 {
		t.Errorf("archived property takes no spaces: %d", code)
	}
	// every successful write left one audit row and one outbox event (same transaction); a refused one may leave a
	// denied entry (IR603), counted apart
	var audits, events int

	principal := &ops.Principal{
		TenantID:          seed.ID("tenant-a"),
		MembershipID:      seed.ID("hq-operator"),
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}

	err := srv.DB.Run(t.Context(), true, principal, func(txn pgx.Tx) error {
		err := txn.QueryRow(t.Context(), `SELECT count(*) FROM audit.audit_log WHERE target_id = $1 AND result =
			'success'`, prop).
			Scan(&audits)
		if err != nil {
			return apperr.Wrap(err, "select audit.audit_log")
		}

		return txn.QueryRow(t.Context(), `SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type NOT LIKE
			'RowChanged:%'`, prop).
			Scan(&events)
	})
	if err != nil {
		t.Fatal(err)
	}

	if audits != 3 ||
		events != 3 { // create, update, archive (the stale update and failed writes left nothing)
		t.Fatalf("property audit rows %d, outbox events %d", audits, events)
	}
}
