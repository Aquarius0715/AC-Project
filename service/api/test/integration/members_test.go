package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func newUser(t *testing.T) string {
	id := uuid.NewString()
	owner(t, `INSERT INTO identity.users (id, email, display_name) VALUES ($1, $2, 'Test user')`, id, id+"@ac.local")
	return id
}

func TestMembersSave(t *testing.T) {
	s := server(t)
	user := newUser(t)
	op, contractor, customer := seed.ID("org-operator-a").String(), seed.ID("org-contractor-a").String(), seed.ID("org-customer-a").String()
	unit := seed.ID("unit-non-rto").String()
	from := clock.Add(-time.Hour).Format(time.RFC3339)
	until := clock.Add(30 * 24 * time.Hour).Format(time.RFC3339)
	body := func(f map[string]string) string {
		base := map[string]string{"userId": `"` + user + `"`, "organizationId": `"` + op + `"`, "role": `"technician"`, "employment": `"internal"`,
			"permissions": `["alert.read"]`, "scopes": `[{"kind":"unit","id":"` + unit + `"}]`, "validFrom": `"` + from + `"`, "validUntil": "null", "reason": `"onboarding"`}
		for k, v := range f {
			if v == "" {
				delete(base, k)
				continue
			}
			base[k] = v
		}
		parts := []string{}
		for k, v := range base {
			parts = append(parts, `"`+k+`":`+v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	for name, tc := range map[string]struct {
		f    map[string]string
		code int
	}{
		"no employment":          {map[string]string{"employment": "null"}, 422},
		"employment for admin":   {map[string]string{"role": `"admin"`}, 422},
		"technician permission":  {map[string]string{"permissions": `["job.write"]`}, 422},
		"write without read":     {map[string]string{"role": `"admin"`, "employment": "null", "permissions": `["job.write"]`, "scopes": `[]`}, 422},
		"unknown permission":     {map[string]string{"role": `"admin"`, "employment": "null", "permissions": `["god.mode"]`, "scopes": `[]`}, 422},
		"admin unit scope":       {map[string]string{"role": `"admin"`, "employment": "null", "permissions": `[]`}, 422},
		"external without end":   {map[string]string{"employment": `"external"`, "organizationId": `"` + contractor + `"`}, 422},
		"until before from":      {map[string]string{"validUntil": `"` + clock.Add(-2*time.Hour).Format(time.RFC3339) + `"`}, 422},
		"blank reason":           {map[string]string{"reason": `" "`}, 422},
		"internal at contractor": {map[string]string{"organizationId": `"` + contractor + `"`}, 422},
		"unknown scope":          {map[string]string{"scopes": `[{"kind":"unit","id":"` + uuid.NewString() + `"}]`}, 422},
		"unknown property":       {map[string]string{"scopes": `[{"kind":"property","id":"` + uuid.NewString() + `"}]`}, 422},
		"unknown user":           {map[string]string{"userId": `"` + uuid.NewString() + `"`}, 404},
		"unknown organization":   {map[string]string{"organizationId": `"` + uuid.NewString() + `"`}, 404},
	} {
		if code, _ := write(s, &hq, "members.save", body(tc.f), 0); code != tc.code {
			t.Errorf("save %s: %d want %d", name, code, tc.code)
		}
	}
	code, m := write(s, &hq, "members.save", body(nil), 0)
	if code != 200 || data(m)["role"] != "technician" || len(data(m)["scopes"].([]any)) != 1 || data(m)["scopeVersion"].(float64) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	// update: permissions change bumps scopeVersion; role is fixed
	if code, m := write(s, &hq, "members.save", body(map[string]string{"id": `"` + id + `"`, "permissions": `["alert.read","alert.resolve"]`, "validUntil": `"` + until + `"`}), 1); code != 200 ||
		data(m)["scopeVersion"].(float64) != 2 || data(m)["version"].(float64) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, m := write(s, &hq, "members.save", body(map[string]string{"id": `"` + id + `"`, "permissions": `["alert.read","alert.resolve"]`, "reason": `"period"`}), 2); code != 200 ||
		data(m)["scopeVersion"].(float64) != 2 {
		t.Fatalf("same permissions keep scopeVersion: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "members.save", body(map[string]string{"id": `"` + id + `"`, "role": `"admin"`, "employment": "null", "permissions": `[]`, "scopes": `[]`}), 3); code != 422 {
		t.Error("role fixed")
	}
	// a technician may also be scoped to a whole property (the identity copy of the property, IR186)
	if code, m := write(s, &hq, "members.save", body(map[string]string{"id": `"` + id + `"`, "permissions": `["alert.read","alert.resolve"]`, "scopes": `[{"kind":"property","id":"` + seed.ID("property-home-a").String() + `"}]`, "reason": `"whole home"`}), 3); code != 200 ||
		data(m)["scopes"].([]any)[0].(map[string]any)["kind"] != "property" || data(m)["scopeVersion"].(float64) != 3 {
		t.Fatalf("property scope: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "members.save", body(map[string]string{"id": `"` + id + `"`}), 1); code != 409 {
		t.Error("stale membership version")
	}
	// client memberships get clientRole=member and the initial consent
	code, m = write(s, &hq, "members.save", body(map[string]string{"organizationId": `"` + customer + `"`, "role": `"client"`, "employment": "null", "permissions": `[]`,
		"scopes": `[{"kind":"organization","id":"` + customer + `"}]`}), 0)
	if code != 200 || data(m)["clientRole"] != "member" {
		t.Fatalf("client: %d %v", code, m)
	}
	var consents int
	ownerScan(t, `SELECT count(*) FROM identity.consents WHERE membership_id = $1 AND granted = false`, []any{data(m)["id"]}, &consents)
	if consents != 1 {
		t.Error("initial consent")
	}
	if code, _ := write(s, &customerA, "members.save", body(nil), 0); code != 403 {
		t.Error("client manages members")
	}

	// self promotion and the last identity.write holder
	_, m = post(s, &hq, "members.list", `{"filters":{"role":"admin"},"limit":100}`)
	var me map[string]any
	for _, it := range items(m) {
		if it["id"] == seed.ID("hq-operator").String() {
			me = it
		}
	}
	perms, _ := json.Marshal(me["permissions"])
	selfBody := func(p string) string {
		return `{"id":"` + me["id"].(string) + `","userId":"` + me["userId"].(string) + `","organizationId":"` + op + `","role":"admin","employment":null,"permissions":` + p +
			`,"scopes":[{"kind":"tenant","id":"` + seed.ID("tenant-a").String() + `"}],"validFrom":"` + me["validFrom"].(string) + `","validUntil":null,"reason":"self"}`
	}
	withOverride := strings.Replace(string(perms), `[`, `["restriction.override",`, 1)
	if code, _ := write(s, &hq, "members.save", selfBody(withOverride), int(me["version"].(float64))); code != 403 {
		t.Error("self promotion")
	}
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id = $1`, seed.ID("hq-restriction-manager"), clock.Add(-time.Minute))
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = NULL WHERE id = $1`, seed.ID("hq-restriction-manager"))
	})
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE id <> $1 AND role = 'admin' AND id IN (SELECT membership_id FROM identity.membership_permissions WHERE permission = 'identity.write')
		AND created_at > $3`, seed.ID("hq-operator"), clock.Add(-time.Minute), clock.Add(-24*time.Hour))
	withoutIdentity := strings.Replace(strings.Replace(string(perms), `"identity.write",`, ``, 1), `,"identity.write"`, ``, 1)
	if code, m := write(s, &hq, "members.save", selfBody(withoutIdentity), int(me["version"].(float64))); code != 409 {
		t.Errorf("last identity.write holder: %d %v", code, m)
	}
}

func TestMembersList(t *testing.T) {
	s := server(t)
	_, m := post(s, &hq, "members.list", `{"limit":100}`)
	for _, it := range items(m) {
		if it["role"] == "client" {
			t.Fatal("clients excluded by default")
		}
	}
	// IR172: every membership carries its user's display name (contractors list their technicians by name)
	_, m = post(s, &contrA, "members.list", `{"limit":100}`)
	if len(items(m)) == 0 {
		t.Fatal("contractor sees its technicians")
	}
	for _, it := range items(m) {
		if name, _ := it["displayName"].(string); name == "" {
			t.Fatalf("displayName missing: %v", it["id"])
		}
	}
	if _, m := post(s, &hq, "members.list", `{"filters":{"role":"client"},"limit":100}`); len(items(m)) < 2 {
		t.Error("role=client")
	}
	if _, m := post(s, &hq, "members.list", `{"filters":{"activeOnly":false,"organizationId":"`+uuid.NewString()+`"}}`); len(items(m)) != 0 {
		t.Error("filters")
	}
	_, m = post(s, &contrA, "members.list", `{"limit":100}`)
	if len(items(m)) == 0 {
		t.Fatal("contractor sees its technicians")
	}
	for _, it := range items(m) {
		if it["role"] != "technician" || it["organizationId"] != seed.ID("org-contractor-a").String() || len(it["permissions"].([]any)) != 0 || len(it["scopes"].([]any)) != 0 {
			t.Fatalf("contractor projection: %v", it)
		}
	}
	for _, b := range []string{`{"filters":{"role":"guest"}}`, `{"filters":{"x":1}}`, `{"sort":{"field":"name","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "members.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "members.list", `{}`); code != 403 {
		t.Error("client lists members")
	}
}
