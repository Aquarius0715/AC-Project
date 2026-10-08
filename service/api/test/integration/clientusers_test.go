package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestClientUsers(t *testing.T) {
	s := server(t)
	cust := seed.ID("cust-b").String()
	email := "member-" + uuid.NewString()[:8] + "@example.com"
	save := func(a *actor, body string, v int) (int, map[string]any) {
		return write(s, a, "clientUsers.save", body, v)
	}
	invite := `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member"}`
	for name, b := range map[string]string{
		"bad email":    `{"customerId":"` + cust + `","email":"not-an-email","clientRole":"member"}`,
		"bad role":     `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"admin"}`,
		"bad status":   `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","status":"invited"}`,
		"blank reason": `{"customerId":"` + cust + `","email":"` + email + `","clientRole":"member","reason":" "}`,
	} {
		if code, _ := save(&hq, b, 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := save(&hq, `{"customerId":"`+cust+`","email":"`+email+`","clientRole":"member","status":"active"}`, 0); code != 422 {
		t.Error("status on invite")
	}
	if code, _ := save(&customerB, `{"customerId":"`+cust+`","email":"`+email+`","clientRole":"owner"}`, 0); code != 403 {
		t.Error("client invites an owner")
	}
	if code, _ := save(&customerA, invite, 0); code != 404 {
		t.Error("client invites into another customer")
	}
	code, m := save(&customerB, invite, 0)
	if code != 200 || data(m)["status"] != "invited" || data(m)["membershipId"] != nil || data(m)["invitedByMembershipId"] != seed.ID("customer-b").String() {
		t.Fatalf("invite: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, _ := save(&hq, strings.Replace(invite, email, strings.ToUpper(email[:1])+email[1:], 1), 0); code != 422 {
		t.Error("duplicate email (case-insensitive)")
	}
	_, m = post(s, &customerB, "clientUsers.list", `{"limit":100}`)
	var owner map[string]any
	for _, it := range items(m) {
		if it["membershipId"] == seed.ID("customer-b").String() {
			owner = it
		}
	}
	if owner == nil || owner["clientRole"] != "owner" || owner["status"] != "active" {
		t.Fatalf("seed owner listed: %v", m)
	}
	if _, m := post(s, &hq, "clientUsers.list", `{"filters":{"customerId":"`+cust+`","status":"invited","clientRole":"member"},"limit":100}`); len(items(m)) == 0 {
		t.Error("HQ filtered list")
	}
	if code, _ := post(s, &hq, "clientUsers.list", `{"filters":{"status":"gone"}}`); code != 422 {
		t.Error("bad status filter")
	}
	if _, m := post(s, &customerA, "clientUsers.list", `{"limit":100}`); len(items(m)) == 0 || items(m)[0]["customerId"] != seed.ID("cust-a").String() {
		t.Error("client list scoped to own customer")
	}
	// resend: preview only
	if code, m := post(s, &customerB, "clientUsers.resendInvite", `{"id":"`+id+`"}`); code != 200 || data(m)["deliveryState"] != "preview" || data(m)["templateKey"] != "invite" {
		t.Fatalf("resend: %d %v", code, m)
	}
	if code, _ := post(s, &customerB, "clientUsers.resendInvite", `{"id":"`+owner["id"].(string)+`"}`); code != 409 {
		t.Error("resend to an active user")
	}
	if code, _ := post(s, &customerA, "clientUsers.resendInvite", `{"id":"`+id+`"}`); code != 404 {
		t.Error("resend another customer's invite")
	}
	// updates are HQ only; the last active owner is protected
	upd := func(target map[string]any, role, status string, v int) (int, map[string]any) {
		return save(&hq, `{"id":"`+target["id"].(string)+`","customerId":"`+cust+`","email":"`+target["email"].(string)+`","clientRole":"`+role+`","status":"`+status+`","reason":"admin change"}`, v)
	}
	if code, _ := save(&customerB, `{"id":"`+id+`","customerId":"`+cust+`","email":"`+email+`","clientRole":"member","reason":"x"}`, 1); code != 403 {
		t.Error("client updates a user")
	}
	ov := int(owner["version"].(float64))
	if code, _ := upd(owner, "member", "active", ov); code != 409 {
		t.Error("demote the last owner")
	}
	if code, _ := upd(owner, "owner", "disabled", ov); code != 409 {
		t.Error("disable the last owner")
	}
	if code, _ := write(s, &hq, "clientUsers.remove", `{"id":"`+owner["id"].(string)+`","reason":"leaving"}`, ov); code != 409 {
		t.Error("remove the last owner")
	}
	if code, _ := save(&hq, `{"id":"`+id+`","customerId":"`+cust+`","email":"`+email+`","clientRole":"member"}`, 1); code != 422 {
		t.Error("update without reason")
	}
	if code, _ := upd(map[string]any{"id": id, "email": email}, "member", "active", 1); code != 409 {
		t.Error("activate a pending invite")
	}
	if code, _ := upd(map[string]any{"id": id, "email": email}, "member", "disabled", 5); code != 409 {
		t.Error("stale client user version")
	}
	code, m = upd(map[string]any{"id": id, "email": email}, "owner", "disabled", 1)
	if code != 200 || data(m)["status"] != "disabled" || data(m)["clientRole"] != "owner" || ver(m) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, _ := write(s, &customerB, "clientUsers.remove", `{"id":"`+id+`","reason":"x"}`, 2); code != 403 {
		t.Error("client removes")
	}
	if code, m := write(s, &hq, "clientUsers.remove", `{"id":"`+id+`","reason":"no longer with the company"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("remove: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "clientUsers.remove", `{"id":"`+id+`","reason":"again"}`, 2); code != 404 {
		t.Error("remove twice")
	}
}

func TestTwoFactor(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM identity.two_factor WHERE user_id = (SELECT user_id FROM identity.memberships WHERE id = $1)`, seed.ID("tech-external-b"))
	code, m := post(s, &techB, "twoFactor.get", `{}`)
	if code != 200 || data(m)["enabled"] != false || data(m)["setupKey"] == nil || data(m)["recoveryCodesLeft"] != nil {
		t.Fatalf("get: %d %v", code, m)
	}
	for _, b := range []string{`{"code":"12345"}`, `{"code":"abcdef"}`, `{}`} {
		if code, _ := write(s, &techB, "twoFactor.enable", b, 0); code != 422 {
			t.Errorf("%s accepted", b)
		}
	}
	if code, _ := write(s, &techB, "twoFactor.disable", `{"code":"123456"}`, 0); code != 409 {
		t.Error("disable when off")
	}
	code, m = write(s, &techB, "twoFactor.enable", `{"code":"123456"}`, 0)
	if code != 200 || len(data(m)["recoveryCodes"].([]any)) != 8 || data(m)["recoveryCodesLeft"].(float64) != 8 {
		t.Fatalf("enable: %d %v", code, m)
	}
	if code, _ := write(s, &techB, "twoFactor.enable", `{"code":"654321"}`, 0); code != 409 {
		t.Error("enable twice")
	}
	if _, m := post(s, &techB, "twoFactor.get", `{}`); data(m)["enabled"] != true || data(m)["setupKey"] != nil {
		t.Errorf("enabled status: %v", m)
	}
	if _, m := post(s, &techB, "preferences.get", `{}`); data(m)["twoFactorEnabled"] != true {
		t.Error("preferences.twoFactorEnabled")
	}
	if code, m := write(s, &techB, "twoFactor.disable", `{"code":"000000"}`, 0); code != 200 || data(m)["enabled"] != false {
		t.Fatalf("disable: %d %v", code, m)
	}
}
