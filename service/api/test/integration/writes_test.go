package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestWritesGetResult(t *testing.T) {
	s := server(t)
	key := "wr-" + uuid.NewString()
	req := httptest.NewRequest(http.MethodPut, "/v1/preferences", strings.NewReader(`{"locale":"en","timezone":"Asia/Kuala_Lumpur"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+techB.token)
	req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
	req.Header.Set("X-Membership-Id", seed.ID(techB.membership).String())
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}
	get := func(a *actor, op, k string) (int, map[string]any) {
		return post(s, a, "writes.getResult", `{"operation":"`+op+`","idempotencyKey":"`+k+`"}`)
	}
	code, m := get(&techB, "preferences.update", key)
	if code != 200 || data(m)["state"] != "succeeded" || data(m)["result"].(map[string]any)["locale"] != "en" {
		t.Fatalf("succeeded: %d %v", code, m)
	}
	for name, tc := range map[string]struct {
		a   *actor
		op  string
		key string
	}{"other member": {&techA, "preferences.update", key}, "other operation": {&techB, "consents.update", key}, "unknown key": {&techB, "preferences.update", "nokey-" + uuid.NewString()}} {
		if _, m := get(tc.a, tc.op, tc.key); data(m)["state"] != "not_received" {
			t.Errorf("%s: %v", name, m)
		}
	}
	if code, _ := get(&techB, "no.such", key); code != 422 {
		t.Error("unknown operation")
	}
	if code, _ := get(&techB, "preferences.update", "short"); code != 422 {
		t.Error("short key")
	}
	owner(t, `UPDATE platform.idempotency_keys SET status = 'in_progress' WHERE key = $1`, key)
	if _, m := get(&techB, "preferences.update", key); data(m)["state"] != "pending" {
		t.Errorf("pending: %v", m)
	}
	// password reset preview is public and generic
	req = httptest.NewRequest(http.MethodPost, "/v1/auth/password-reset-preview", strings.NewReader(`{"demoEmail":"nobody@example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	s.Echo.ServeHTTP(w, req)
	var r map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if w.Code != 200 || data(r)["messageKey"] != "auth.reset_generic" {
		t.Fatalf("reset preview: %d %s", w.Code, w.Body.String())
	}
	if code, _ := post(s, nil, "auth.previewPasswordReset", `{"demoEmail":"bad"}`); code != 422 {
		t.Error("bad reset email")
	}
}

func TestResolveQr(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	var serial string
	ownerScan(t, `SELECT serial FROM devices.devices WHERE unit_id = $1`, []any{u}, &serial)
	qr := func(code string) (int, map[string]any) {
		return post(s, &techB, "units.resolveQr", `{"code":"`+code+`"}`)
	}
	if code, _ := qr(u); code != 404 {
		t.Error("unit outside assignments")
	}
	job := assign(t, u, "tech-external-b", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	for _, c := range []string{u, "AC-UNIT:" + u, strings.ToLower(serial)} {
		code, m := qr(c)
		if code != 200 || data(m)["unitId"] != u || data(m)["jobId"] != job {
			t.Errorf("%s: %d %v", c, code, m)
		}
	}
	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, job)
	if code, m := qr(u); code != 200 || data(m)["jobId"] != nil {
		t.Errorf("no open job: %d %v", code, m)
	}
	if code, _ := qr("SN-UNKNOWN"); code != 404 {
		t.Error("unknown label")
	}
	if code, _ := qr(" "); code != 422 {
		t.Error("blank code")
	}
	if code, _ := post(s, &customerB, "units.resolveQr", `{"code":"`+u+`"}`); code != 403 {
		t.Error("client resolves QR")
	}
}
