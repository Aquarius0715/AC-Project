package integration

import (
	"encoding/json"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// write posts with a fresh idempotency key and an optional expected version.
// versionZero sends X-Expected-Version: 0 (an absent row); 0 itself omits the header.
const versionZero = -100

func write(s *apiserver.Server, a *actor, op, body string, version int) (int, map[string]any) {
	req := httptest.NewRequest(http.MethodPost, "/v1/ops/"+op, strings.NewReader(body))
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
	code, m := write(s, &hq, "properties.save", `{"customerOrgId":"`+org+`","kind":"office","name":"Test HQ `+uuid.NewString()[:6]+`","address":null,"accessInstructions":null}`, 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("property create: %d %v", code, m)
	}
	prop := data(m)["id"].(string)
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
	upd := `{"id":"` + prop + `","customerOrgId":"` + org + `","kind":"office","name":"Renamed","address":"1 Test Rd (fictional)","accessInstructions":null}`
	if code, m := write(s, &hq, "properties.save", upd, 1); code != 200 || ver(m) != 2 || data(m)["name"] != "Renamed" {
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
	if code, _ := write(s, &hq, "units.save", unitBody(uuid.NewString(), "", ""), 1); code != 404 {
		t.Errorf("unknown unit: %d", code)
	}

	// archive rules
	if code, _ := write(s, &hq, "spaces.archive", `{"id":"`+floor+`","reason":"x"}`, 2); code != 409 {
		t.Errorf("space with units/children must not archive: %d", code)
	}
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":""}`, 2); code != 422 {
		t.Error("archive reason required")
	}
	if code, m := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":"decommissioned"}`, 2); code != 200 || data(m)["archived"] != true {
		t.Fatalf("unit archive: %d", code)
	}
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":"again"}`, 3); code != 409 {
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
