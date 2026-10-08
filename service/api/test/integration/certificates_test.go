package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestCertificatesAndParts(t *testing.T) {
	s := server(t)
	tech := seed.ID("tech-external-a").String()
	pdf := blobJSON("cert.pdf", "application/pdf", []byte("%PDF-1.4 demo"), 13)
	issued, expires := clock.Add(-10*24*time.Hour).Format(time.RFC3339), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	body := func(extra string) string {
		return `{"membershipId":"` + tech + `","code":"demo_indoor","name":" Indoor AC ","number":"N-1","issuedAt":"` + issued + `","expiresAt":"` + expires + `","file":` + pdf + extra + `}`
	}
	for name, b := range map[string]string{
		"bad code":       strings.Replace(body(""), `"demo_indoor"`, `"demo_roof"`, 1),
		"blank name":     strings.Replace(body(""), `" Indoor AC "`, `" "`, 1),
		"reversed dates": `{"membershipId":"` + tech + `","code":"demo_indoor","name":"x","number":"N","issuedAt":"` + expires + `","expiresAt":"` + issued + `","file":` + pdf + `}`,
		"gif file":       `{"membershipId":"` + tech + `","code":"demo_indoor","name":"x","number":"N","issuedAt":"` + issued + `","expiresAt":"` + expires + `","file":` + blobJSON("a.gif", "image/gif", []byte("x"), 1) + `}`,
	} {
		if code, _ := write(s, &contrA, "certificates.submit", b, 0); code != 422 {
			t.Errorf("submit %s: %d", name, code)
		}
	}
	if code, _ := write(s, &contrB, "certificates.submit", body(""), 0); code != 404 {
		t.Error("other contractor's technician")
	}
	if code, _ := write(s, &contrA, "certificates.submit", body(`,"renewalOf":"`+uuid.NewString()+`"`), 0); code != 404 {
		t.Error("unknown renewal")
	}
	code, m := write(s, &contrA, "certificates.submit", body(""), 0)
	if code != 200 || data(m)["status"] != "pending_verification" || data(m)["fileName"] != "cert.pdf" || data(m)["name"] != "Indoor AC" {
		t.Fatalf("submit: %d %v", code, m)
	}
	cert := data(m)["id"].(string)
	// verify
	if code, _ := write(s, &hq, "certificates.verify", `{"certificateId":"`+cert+`","decision":"reject"}`, 1); code != 422 {
		t.Error("reject without reason")
	}
	if code, _ := write(s, &contrA, "certificates.verify", `{"certificateId":"`+cert+`","decision":"approve"}`, 1); code != 403 {
		t.Error("contractor verifies")
	}
	if code, m := write(s, &hq, "certificates.verify", `{"certificateId":"`+cert+`","decision":"approve"}`, 1); code != 200 || data(m)["status"] != "valid" || data(m)["verifiedAt"] == nil {
		t.Fatalf("approve: %d %v", code, m)
	}
	var until time.Time
	ownerScan(t, `SELECT valid_until FROM identity.qualification_grants WHERE membership_id = $1 AND code = 'demo_indoor'`, []any{tech}, &until)
	if !until.Equal(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("grant extended to %v", until)
	}
	if code, _ := write(s, &hq, "certificates.verify", `{"certificateId":"`+cert+`","decision":"approve"}`, 2); code != 409 {
		t.Error("verify twice")
	}
	// a renewal can be rejected; training request
	code, m = write(s, &contrA, "certificates.submit", body(`,"renewalOf":"`+cert+`"`), 0)
	if code != 200 || data(m)["renewalOf"] != cert {
		t.Fatalf("renewal: %d %v", code, m)
	}
	renewal := data(m)["id"].(string)
	if code, m := write(s, &hq, "certificates.verify", `{"certificateId":"`+renewal+`","decision":"reject","reason":"blurry scan"}`, 1); code != 200 || data(m)["status"] != "rejected" {
		t.Fatalf("reject: %d %v", code, m)
	}
	if code, _ := write(s, &contrA, "certificates.requestTraining", `{"certificateId":"`+cert+`","note":" "}`, 2); code != 422 {
		t.Error("blank note")
	}
	if code, m := write(s, &contrA, "certificates.requestTraining", `{"certificateId":"`+cert+`","note":"refresher course"}`, 2); code != 200 || data(m)["trainingRequestedAt"] == nil {
		t.Fatalf("training: %d %v", code, m)
	}
	if code, _ := write(s, &contrB, "certificates.requestTraining", `{"certificateId":"`+cert+`","note":"x"}`, 3); code != 404 {
		t.Error("other contractor training")
	}
	// expiring / expired read status
	owner(t, `UPDATE maintenance.certificates SET expires_at = $2 WHERE id = $1`, cert, clock.Add(10*24*time.Hour))
	ids := func(body string, a *actor) map[string]string {
		_, m := post(s, a, "certificates.list", body)
		out := map[string]string{}
		for _, it := range items(m) {
			out[it["id"].(string)] = it["status"].(string)
		}
		return out
	}
	if got := ids(`{"filters":{"membershipId":"`+tech+`"},"limit":100}`, &hq); got[cert] != "expiring" || got[renewal] != "rejected" {
		t.Fatalf("statuses: %v", got)
	}
	if got := ids(`{"filters":{"status":"expiring","code":"demo_indoor"},"limit":100}`, &contrA); got[renewal] != "" || got[cert] != "expiring" {
		t.Fatalf("status filter: %v", got)
	}
	if got := ids(`{"limit":100}`, &contrB); got[cert] != "" {
		t.Error("contractor-b sees contractor-a's certificates")
	}
	if code, _ := post(s, &hq, "certificates.list", `{"filters":{"code":"x"}}`); code != 422 {
		t.Error("bad code filter")
	}
	owner(t, `UPDATE maintenance.certificates SET expires_at = $2 WHERE id = $1`, cert, time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))

	// parts catalog
	code1 := "TST-" + uuid.NewString()[:6]
	owner(t, `INSERT INTO maintenance.parts_catalog (code, tenant_id, name) VALUES ($1, $2, 'Test filter mesh')`, code1, seed.ID("tenant-a"))
	_, m = post(s, &hq, "parts.list", `{"filters":{"search":"filter MESH"},"limit":100}`)
	found := false
	for _, it := range items(m) {
		found = found || (it["code"] == code1 && it["vanStockQuantity"] == nil)
	}
	if !found {
		t.Fatalf("parts: %v", m)
	}
	if code, _ := post(s, &techInt, "parts.list", `{}`); code != 200 {
		t.Error("technician parts")
	}
	if code, _ := post(s, &hq, "parts.list", `{"filters":{"x":1}}`); code != 422 {
		t.Error("bad parts filter")
	}
}
