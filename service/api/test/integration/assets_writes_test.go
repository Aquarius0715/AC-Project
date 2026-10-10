package integration

import (
	"encoding/json"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// write posts with a fresh idempotency key and an optional expected version.
// versionZero sends X-Expected-Version: 0 (an absent row); 0 itself omits the header.
const versionZero = -100

func write(s *apiserver.Server, a *actor, op, body string, version int) (int, map[string]any) {
	req := httptest.NewRequest(ops.ClientRequest(op, body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
	req.Header.Set("X-Membership-Id", seed.ID(a.membership).String())
	req.Header.Set("Idempotency-Key", "test-"+uuid.NewString())
	if version > 0 {
		req.Header.Set("X-Expected-Version", strconv.Itoa(version))
	}
	if version == versionZero {
		req.Header.Set("X-Expected-Version", "0")
	}
	w := httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w.Code, m
}

func data(m map[string]any) map[string]any { d, _ := m["data"].(map[string]any); return d }
func ver(m map[string]any) int             { return int(data(m)["version"].(float64)) }

func TestAssetWritesLifecycle(t *testing.T) {
	s := server(t)
	org := seed.ID("org-customer-b").String()
	model := seed.ID("ventilation-demo").String()

	// property create / validation / update / conflict
	propName := "Test HQ " + uuid.NewString()[:6]
	code, m := write(s, &hq, "properties.save", `{"customerOrgId":"`+org+`","kind":"office","name":"`+propName+`","address":null,"accessInstructions":null}`, 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("property create: %d %v", code, m)
	}
	prop := data(m)["id"].(string)
	// names are unique among non-archived siblings, trimmed and case-insensitive (IR208); the duplicate is rolled back
	if code, m := write(s, &hq, "properties.save", `{"customerOrgId":"`+org+`","kind":"home","name":" `+strings.ToLower(propName)+`","address":null,"accessInstructions":null}`, 0); code != 409 || m["messageKey"] != "error.duplicateSiblingName" {
		t.Errorf("duplicate property name: %d %v", code, m)
	}
	for _, bad := range []string{
		`{"customerOrgId":"` + org + `","kind":"castle","name":"x","address":null,"accessInstructions":null}`,
		`{"customerOrgId":"` + org + `","kind":"home","name":"","address":null,"accessInstructions":null}`,
		`{"customerOrgId":"` + org + `","kind":"home","name":"` + strings.Repeat("x", 121) + `","address":null,"accessInstructions":null}`,
		`{"kind":"home","name":"x","address":"` + strings.Repeat("a", 501) + `","accessInstructions":null}`,
	} {
		if code, _ := write(s, &hq, "properties.save", bad, 0); code != 422 {
			t.Errorf("property validation %s: %d", bad[:40], code)
		}
	}
	if code, _ := write(s, &hq, "properties.save", `{"customerOrgId":"`+uuid.NewString()+`","kind":"home","name":"x","address":null,"accessInstructions":null}`, 0); code != 404 {
		t.Errorf("unknown customer: %d", code)
	}
	renamed := "Renamed " + uuid.NewString()[:6]
	upd := `{"id":"` + prop + `","customerOrgId":"` + org + `","kind":"office","name":"` + renamed + `","address":"1 Test Rd (fictional)","accessInstructions":null}`
	if code, m := write(s, &hq, "properties.save", upd, 1); code != 200 || ver(m) != 2 || data(m)["name"] != renamed {
		t.Fatalf("property update: %d %v", code, m)
	}
	if code, m := write(s, &hq, "properties.save", upd, 1); code != 409 || m["code"] != "CONFLICT" {
		t.Fatalf("stale version: %d", code)
	}
	if code, _ := write(s, &hq, "properties.save", upd, 0); code != 422 {
		t.Fatal("update without version")
	}
	if code, _ := write(s, &customerB, "properties.save", upd, 2); code != 403 {
		t.Fatalf("client cannot save properties: %d", code)
	}

	// spaces: floor → room, cycle, other property
	code, m = write(s, &hq, "spaces.save", `{"propertyId":"`+prop+`","parentSpaceId":null,"kind":"floor","name":"1F"}`, 0)
	if code != 200 {
		t.Fatalf("floor: %d %v", code, m)
	}
	floor := data(m)["id"].(string)
	code, m = write(s, &hq, "spaces.save", `{"propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"Server room"}`, 0)
	if code != 200 {
		t.Fatalf("room: %d", code)
	}
	room := data(m)["id"].(string)
	if code, _ := write(s, &hq, "spaces.save", `{"id":"`+floor+`","propertyId":"`+prop+`","parentSpaceId":"`+room+`","kind":"floor","name":"1F"}`, 1); code != 422 {
		t.Errorf("cycle: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.save", `{"id":"`+floor+`","propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"floor","name":"1F"}`, 1); code != 422 {
		t.Errorf("self parent: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.save", `{"propertyId":"`+prop+`","parentSpaceId":"`+seed.ID("floor-1").String()+`","kind":"room","name":"x"}`, 0); code != 422 {
		t.Errorf("parent in another property: %d", code)
	}
	if code, m := write(s, &hq, "spaces.save", `{"id":"`+room+`","propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"Server room A"}`, 1); code != 200 || ver(m) != 2 {
		t.Fatalf("space update: %d", code)
	}
	for name, b := range map[string]string{
		"floor at the root": `{"propertyId":"` + prop + `","parentSpaceId":null,"kind":"floor","name":"1f"}`,
		"room on the floor": `{"propertyId":"` + prop + `","parentSpaceId":"` + floor + `","kind":"room","name":" server room a "}`,
	} {
		if code, m := write(s, &hq, "spaces.save", b, 0); code != 409 || m["messageKey"] != "error.duplicateSiblingName" {
			t.Errorf("duplicate space (%s): %d %v", name, code, m)
		}
	}
	code, m = write(s, &hq, "spaces.save", `{"propertyId":"`+prop+`","parentSpaceId":"`+floor+`","kind":"room","name":"1F"}`, 0)
	if code != 200 {
		t.Fatalf("the same name under another parent is allowed: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.archive", `{"id":"`+data(m)["id"].(string)+`","reason":"test cleanup"}`, 1); code != 200 {
		t.Errorf("room 1F archive: %d", code)
	}

	// units: create, wrong customer, unknown model, future install, relocation reason
	unitBody := func(id, space string, extra string) string {
		sp := "null"
		if space != "" {
			sp = `"` + space + `"`
		}
		idf := ""
		if id != "" {
			idf = `"id":"` + id + `",`
		}
		return `{` + idf + `"customerOrgId":"` + org + `","propertyId":"` + prop + `","spaceId":` + sp + `,"displayName":"Server AC","modelId":"` + model + `","type":"split","installedAt":null,"serviceScope":["indoor","outdoor"]` + extra + `}`
	}
	code, m = write(s, &hq, "units.save", unitBody("", room, ""), 0)
	if code != 200 || data(m)["displayName"] != "Server AC" || int(data(m)["capabilityVersion"].(float64)) != 3 {
		t.Fatalf("unit create: %d %v", code, m)
	}
	unit := data(m)["id"].(string)
	if code, m := write(s, &hq, "units.save", strings.Replace(unitBody("", room, ""), "Server AC", "server ac", 1), 0); code != 409 || m["messageKey"] != "error.duplicateSiblingName" {
		t.Errorf("duplicate unit name in the room: %d %v", code, m)
	}
	badUnits := map[string]string{
		"other customer":  strings.Replace(unitBody("", "", ""), org, seed.ID("org-customer-a").String(), 1),
		"unknown model":   strings.Replace(unitBody("", "", ""), model, uuid.NewString(), 1),
		"future install":  strings.Replace(unitBody("", "", ""), `"installedAt":null`, `"installedAt":"2030-01-01T00:00:00Z"`, 1),
		"bad scope":       strings.Replace(unitBody("", "", ""), `["indoor","outdoor"]`, `["roof"]`, 1),
		"dup scope":       strings.Replace(unitBody("", "", ""), `["indoor","outdoor"]`, `["indoor","indoor"]`, 1),
		"empty scope":     strings.Replace(unitBody("", "", ""), `["indoor","outdoor"]`, `[]`, 1),
		"space elsewhere": unitBody("", seed.ID("room-1").String(), ""),
	}
	for name, b := range badUnits {
		if code, _ := write(s, &hq, "units.save", b, 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "units.save", unitBody(unit, floor, ""), 1); code != 422 {
		t.Error("relocation without reason")
	}
	if code, m := write(s, &hq, "units.save", unitBody(unit, floor, `,"changeReason":"moved to floor"`), 1); code != 200 || ver(m) != 2 {
		t.Fatalf("relocate: %d %v", code, m)
	}
	// renaming another unit on the floor to the moved unit's name is a duplicate too (update path)
	code, m = write(s, &hq, "units.save", strings.Replace(unitBody("", floor, ""), "Server AC", "Server AC 2", 1), 0)
	if code != 200 {
		t.Fatalf("second unit: %d %v", code, m)
	}
	second := data(m)["id"].(string)
	if code, m := write(s, &hq, "units.save", unitBody(second, floor, ""), 1); code != 409 || m["messageKey"] != "error.duplicateSiblingName" {
		t.Errorf("rename to a sibling's name: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+second+`","reason":"test cleanup"}`, 1); code != 200 {
		t.Errorf("second unit archive: %d", code)
	}
	// warranty end (IR209): set, kept when omitted, cleared with null, never before the installation
	if code, m := write(s, &hq, "units.save", unitBody(unit, floor, `,"warrantyEndsAt":"2027-01-01T00:00:00+08:00"`), 2); code != 200 || data(m)["warrantyEndsAt"] != "2026-12-31T16:00:00Z" {
		t.Fatalf("warranty set: %d %v", code, m)
	}
	if code, m := write(s, &hq, "units.save", unitBody(unit, floor, ""), 3); code != 200 || data(m)["warrantyEndsAt"] != "2026-12-31T16:00:00Z" {
		t.Fatalf("warranty kept when omitted: %d %v", code, data(m)["warrantyEndsAt"])
	}
	early := strings.Replace(unitBody(unit, floor, `,"warrantyEndsAt":"2026-08-01T00:00:00+08:00"`), `"installedAt":null`, `"installedAt":"2026-09-01T00:00:00+08:00"`, 1)
	if code, m := write(s, &hq, "units.save", early, 4); code != 422 || m["fieldErrors"].(map[string]any)["warrantyEndsAt"] != "error.range" {
		t.Errorf("warranty before installation: %d %v", code, m)
	}
	// the stored warranty end counts too (an installation in 2027 would be refused as future, so the end moves back)
	owner(t, `UPDATE assets.units SET warranty_ends_at = '2026-08-01T00:00:00+08:00' WHERE id = $1`, unit)
	if code, m := write(s, &hq, "units.save", strings.Replace(unitBody(unit, floor, ""), `"installedAt":null`, `"installedAt":"2026-09-01T00:00:00+08:00"`, 1), 4); code != 422 ||
		m["fieldErrors"].(map[string]any)["warrantyEndsAt"] != "error.range" {
		t.Errorf("an installation after the stored warranty end: %d %v", code, m)
	}
	if code, m := write(s, &hq, "units.save", unitBody(unit, floor, `,"warrantyEndsAt":null`), 4); code != 200 || data(m)["warrantyEndsAt"] != nil {
		t.Fatalf("warranty cleared: %d %v", code, data(m)["warrantyEndsAt"])
	}
	// the history keeps the original location: only the changed field, with its old and new value (DD-A02 step 3)
	day := `"from":"` + clock.Add(-time.Hour).Format(time.RFC3339) + `","to":"` + clock.Add(time.Hour).Format(time.RFC3339) + `"`
	_, am := post(s, &hq, "audit.list", `{"filters":{`+day+`,"targetKind":"unit","targetId":"`+unit+`"},"limit":10}`)
	var moved map[string]any
	for _, a := range items(am) {
		if a["nextVersion"] == float64(2) {
			moved = a
		}
	}
	if moved == nil || moved["reason"] != "moved to floor" || moved["maskedBefore"].(map[string]any)["spaceId"] != room ||
		moved["maskedAfter"].(map[string]any)["spaceId"] != floor || len(moved["maskedAfter"].(map[string]any)) != 1 {
		t.Fatalf("relocation audit: %v", am)
	}
	if code, _ := write(s, &hq, "units.save", unitBody(uuid.NewString(), "", ""), 1); code != 404 {
		t.Errorf("unknown unit: %d", code)
	}

	// archive rules
	if code, _ := write(s, &hq, "spaces.archive", `{"id":"`+floor+`","reason":"x"}`, 2); code != 409 {
		t.Errorf("space with units/children must not archive: %d", code)
	}
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":""}`, 5); code != 422 {
		t.Error("archive reason required")
	}
	if code, m := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":"decommissioned"}`, 5); code != 200 || data(m)["archived"] != true {
		t.Fatalf("unit archive: %d", code)
	}
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":"again"}`, 6); code != 409 {
		t.Errorf("archive twice: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.archive", `{"id":"`+room+`","reason":"empty"}`, 2); code != 200 {
		t.Errorf("room archive: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.archive", `{"id":"`+floor+`","reason":"empty"}`, 1); code != 200 {
		t.Errorf("floor archive: %d", code)
	}
	if code, _ := write(s, &hq, "properties.archive", `{"id":"`+prop+`","reason":"closed"}`, 2); code != 200 {
		t.Errorf("property archive: %d", code)
	}
	if code, _ := write(s, &hq, "spaces.save", `{"propertyId":"`+prop+`","parentSpaceId":null,"kind":"floor","name":"2F"}`, 0); code != 404 {
		t.Errorf("archived property takes no spaces: %d", code)
	}
	// every successful write left one audit row and one outbox event (same transaction)
	var audits, events int
	p := &ops.Principal{TenantID: seed.ID("tenant-a"), MembershipID: seed.ID("hq-operator")}
	if err := s.DB.Run(t.Context(), true, p, func(tx pgx.Tx) error {
		if err := tx.QueryRow(t.Context(), `SELECT count(*) FROM audit.audit_log WHERE target_id = $1`, prop).Scan(&audits); err != nil {
			return err
		}
		return tx.QueryRow(t.Context(), `SELECT count(*) FROM platform.outbox WHERE aggregate_id = $1 AND event_type NOT LIKE 'RowChanged:%'`, prop).Scan(&events)
	}); err != nil {
		t.Fatal(err)
	}
	if audits != 3 || events != 3 { // create, update, archive (the stale update and failed writes left nothing)
		t.Fatalf("property audit rows %d, outbox events %d", audits, events)
	}
}
