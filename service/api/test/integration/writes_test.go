package integration_test

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
	srv := server(t)
	key := "wr-" + uuid.NewString()
	req := httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPut,
		"/v1/preferences",
		strings.NewReader(`{"locale":"en","timezone":"Asia/Kuala_Lumpur"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+techB().token)
	req.Header.Set("X-Tenant-Id", seed.ID("tenant-a").String())
	req.Header.Set("X-Membership-Id", seed.ID(techB().membership).String())
	req.Header.Set("Idempotency-Key", key)

	rec := httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("write: %d %s", rec.Code, rec.Body.String())
	}

	get := func(a *actor, op, k string) (int, map[string]any) {
		return post(srv, a, "writes.getResult", `{"operation":"`+op+`","idempotencyKey":"`+k+`"}`)
	}

	code, m := get(techB(), "preferences.update", key)
	if code != 200 || data(m)["state"] != "succeeded" ||
		asObj(t, data(m)["result"])["locale"] != "en" {
		t.Fatalf("succeeded: %d %v", code, m)
	}

	for name, tc := range map[string]struct {
		a   *actor
		op  string
		key string
	}{
		"other member": {techA(), opPreferencesUpdate, key}, "other operation": {techB(), "consents.update", key},
		"unknown key": {techB(), opPreferencesUpdate, "nokey-" + uuid.NewString()},
	} {
		if _, m := get(tc.a, tc.op, tc.key); data(m)["state"] != "not_received" {
			t.Errorf("%s: %v", name, m)
		}
	}

	if code, res := get(
		techB(),
		"no.such",
		key,
	); code != 422 ||
		fields(res) != "operation=error.invalid" {
		t.Errorf("unknown operation: %d %s", code, fields(res))
	}

	if code, res := get(
		techB(),
		"preferences.update",
		"short",
	); code != 422 ||
		fields(res) != "idempotencyKey=error.invalid" {
		t.Errorf("short key: %d %s", code, fields(res))
	}

	owner(t, `UPDATE platform.idempotency_keys SET status = 'in_progress' WHERE key = $1`, key)

	if _, m := get(techB(), "preferences.update", key); data(m)["state"] != statusPending {
		t.Errorf("pending: %v", m)
	}
	// password reset preview is public and generic
	req = httptest.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"/v1/auth/password-reset-preview",
		strings.NewReader(`{"demoEmail":"nobody@example.com"}`),
	)
	req.Header.Set("Content-Type", "application/json")

	rec = httptest.NewRecorder()
	srv.Echo.ServeHTTP(rec, req)

	var r map[string]any

	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || data(r)["messageKey"] != "auth.reset_generic" {
		t.Fatalf("reset preview: %d %s", rec.Code, rec.Body.String())
	}

	if code, res := post(
		srv,
		nil,
		"auth.previewPasswordReset",
		`{"demoEmail":"bad"}`,
	); code != 422 ||
		fields(res) != "demoEmail=error.invalid" {
		t.Errorf("bad reset email: %d %s", code, fields(res))
	}
}

func TestResolveQr(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")

	var serial string
	ownerScan(t, `SELECT serial FROM devices.devices WHERE unit_id = $1`, []any{unit}, &serial)

	qr := func(code string) (int, map[string]any) {
		return post(srv, techB(), "units.resolveQr", `{"code":"`+code+`"}`)
	}
	if code, _ := qr(unit); code != 404 {
		t.Error("unit outside assignments")
	}

	job := assign(
		t,
		unit,
		"tech-external-b",
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
		"active",
	)
	for _, c := range []string{unit, "AC-UNIT:" + unit, strings.ToLower(serial)} {
		code, m := qr(c)
		if code != 200 || data(m)["unitId"] != unit || data(m)["jobId"] != job {
			t.Errorf("%s: %d %v", c, code, m)
		}
	}

	owner(t, `UPDATE maintenance.jobs SET status = 'completed' WHERE id = $1`, job)

	if code, m := qr(unit); code != 200 || data(m)["jobId"] != nil {
		t.Errorf("no open job: %d %v", code, m)
	}

	if code, _ := qr("SN-UNKNOWN"); code != 404 {
		t.Error("unknown label")
	}

	if code, m := qr(" "); code != 422 || fields(m) != refusedCodeInvalid {
		t.Errorf("blank code: %d %s", code, fields(m))
	}

	if code, _ := post(srv, customerB(), "units.resolveQr", `{"code":"`+unit+`"}`); code != 403 {
		t.Error("client resolves QR")
	}
}
