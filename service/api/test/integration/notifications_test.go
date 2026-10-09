package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestNotificationsInbox(t *testing.T) {
	s := server(t)
	// earlier runs leave many notifications: mark customer-b's existing ones read so unreadOnly shows this run's
	owner(t, `UPDATE notify.notifications SET read_at = $2 WHERE recipient_membership_id = $1 AND read_at IS NULL`, seed.ID("customer-b"), clock)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	code, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, m)
	}
	rid := data(m)["id"].(string)
	// a notification whose target customer-b cannot read is hidden (IR58)
	hidden := uuid.NewString()
	owner(t, `INSERT INTO notify.notifications (id, tenant_id, recipient_membership_id, scope_version_at_creation, type, template_key, target, params, severity, occurred_at)
		VALUES ($1,$2,$3,1,'fault','alert',$4,'{}','critical',$5)`, hidden, seed.ID("tenant-a"), seed.ID("customer-b"),
		`{"kind":"unit","id":"`+seed.ID("unit-online-rto").String()+`"}`, clock)
	_, m = post(s, &customerB, "notifications.list", `{"filters":{"type":"restriction","unreadOnly":true},"limit":100}`)
	var n map[string]any
	for _, it := range items(m) {
		if it["target"].(map[string]any)["id"] == rid {
			n = it
		}
	}
	if n == nil || n["deliveryState"] != "simulated" || n["params"].(map[string]any)["targetName"] == "" || n["severity"] != "warning" {
		t.Fatalf("restriction notice: %v", m)
	}
	_, m = post(s, &customerB, "notifications.list", `{"filters":{"severity":"critical"},"limit":100}`)
	for _, it := range items(m) {
		if it["id"] == hidden {
			t.Fatal("unreadable target listed")
		}
	}
	for _, b := range []string{`{"filters":{"severity":"high"}}`, `{"filters":{"kind":"x"}}`, `{"filters":{"type":"spam"}}`} {
		if code, _ := post(s, &customerB, "notifications.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if _, m := post(s, &customerB, "notifications.list", `{"sort":{"field":"severity","direction":"desc"},"limit":5}`); len(items(m)) == 0 {
		t.Error("severity sort")
	}
	id := n["id"].(string)
	if code, _ := write(s, &customerB, "notifications.markRead", `{"id":"`+id+`"}`, 2); code != 409 {
		t.Error("stale notification version")
	}
	if code, _ := write(s, &customerA, "notifications.markRead", `{"id":"`+id+`"}`, 1); code != 404 {
		t.Error("mark another member's notification")
	}
	if code, _ := write(s, &customerB, "notifications.markRead", `{"id":"`+hidden+`"}`, 1); code != 404 {
		t.Error("mark an unreadable target")
	}
	code, m = write(s, &customerB, "notifications.markRead", `{"id":"`+id+`"}`, 1)
	if code != 200 || data(m)["readAt"] == nil || ver(m) != 2 {
		t.Fatalf("markRead: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "notifications.markRead", `{"id":"`+id+`"}`, 2); code != 200 || ver(m) != 2 {
		t.Error("markRead again keeps the record")
	}
	// HQ sees the restriction notice target; technicians see nothing for it
	if _, m := post(s, &techB, "notifications.list", `{"filters":{"type":"restriction"}}`); len(items(m)) != 0 {
		t.Error("technician restriction notices")
	}
}

func TestNotificationRecipientsPreview(t *testing.T) {
	s := server(t)
	_, inv := newInvoice(t, s)
	body := func(channel string) string {
		return `{"target":{"kind":"invoice","id":"` + inv + `"},"templateKey":"payment_reminder","channel":"` + channel + `","query":{}}`
	}
	if code, _ := post(s, &hq, "notifications.recipients", body("inApp")); code != 409 {
		t.Error("reminder recipients before due")
	}
	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, inv, clock.Add(-time.Hour))
	code, m := post(s, &hq, "notifications.recipients", body("inApp"))
	if code != 200 || len(items(m)) == 0 {
		t.Fatalf("recipients: %d %v", code, m)
	}
	for _, r := range items(m) {
		if r["role"] != "client" {
			t.Errorf("payment_reminder recipient %v", r)
		}
	}
	recipient := items(m)[0]["id"].(string)
	if code, _ := post(s, &customerB, "notifications.recipients", body("inApp")); code != 403 {
		t.Error("client lists reminder recipients")
	}
	if code, _ := post(s, &customerA, "notifications.recipients", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment","channel":"inApp","query":{}}`); code != 404 {
		t.Error("other customer's invoice")
	}
	for _, b := range []string{`{"target":{"kind":"planet","id":"` + inv + `"},"templateKey":"payment","channel":"inApp","query":{}}`,
		`{"target":{"kind":"invoice","id":"` + inv + `"},"templateKey":"spam","channel":"sms","query":{}}`,
		`{"target":{"kind":"invoice","id":"` + inv + `"},"templateKey":"payment","channel":"inApp","role":"guest","query":{}}`,
		`{"target":{"kind":"invoice","id":"` + inv + `"},"templateKey":"payment","channel":"inApp","query":{"filters":{"x":1}}}`} {
		if code, _ := post(s, &hq, "notifications.recipients", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if _, m := post(s, &hq, "notifications.recipients", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment","channel":"email","role":"admin","query":{"limit":1}}`); len(items(m)) != 1 {
		t.Errorf("admin recipients for payment: %v", m)
	}
	preview := func(recipient string) (int, map[string]any) {
		return post(s, &hq, "notifications.preview", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email","recipientMembershipId":"`+recipient+`","message":"Please pay"}`)
	}
	code, m = preview(recipient)
	if code != 200 || data(m)["deliveryState"] != "preview" || data(m)["params"].(map[string]any)["message"] != "Please pay" || data(m)["type"] != "payment_reminder" {
		t.Fatalf("preview: %d %v", code, m)
	}
	var saved int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE id = $1`, []any{data(m)["id"]}, &saved)
	if saved != 0 {
		t.Error("preview saved a notification")
	}
	if code, _ := preview(seed.ID("customer-a").String()); code != 422 {
		t.Error("ineligible preview recipient")
	}
	if code, _ := post(s, &hq, "notifications.preview", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email","recipientMembershipId":"`+recipient+`","message":" "}`); code != 422 {
		t.Error("blank message")
	}
	// the message may be as long as a job note (2000, DD-P07); the reason stays at 1000 (IR228)
	long := strings.Repeat("m", 2000)
	if code, m := post(s, &hq, "notifications.preview", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email","recipientMembershipId":"`+recipient+`","message":"`+long+`"}`); code != 200 {
		t.Errorf("2000-character message: %d %v", code, m)
	}
	for body, field := range map[string]string{
		`"message":"` + long + `m"`:                    "message",
		`"reason":"` + strings.Repeat("r", 1001) + `"`: "reason",
	} {
		if code, m := post(s, &hq, "notifications.preview", `{"target":{"kind":"invoice","id":"`+inv+`"},"templateKey":"payment_reminder","channel":"email","recipientMembershipId":"`+recipient+`",`+body+`}`); code != 422 || m["fieldErrors"].(map[string]any)[field] != "error.length" {
			t.Errorf("%s too long: %d %v", field, code, m)
		}
	}
	// unit target: the client of the unit's organization reads it, another client does not
	unit := seed.ID("unit-online-rto").String()
	if code, m := post(s, &customerA, "notifications.recipients", `{"target":{"kind":"unit","id":"`+unit+`"},"templateKey":"alert","channel":"inApp","query":{}}`); code != 200 || len(items(m)) == 0 {
		t.Errorf("unit recipients: %d %v", code, m)
	}
	if code, _ := post(s, &customerB, "notifications.recipients", `{"target":{"kind":"unit","id":"`+unit+`"},"templateKey":"alert","channel":"inApp","query":{}}`); code != 404 {
		t.Error("unit of another customer")
	}
}

func TestPreferencesAndConsents(t *testing.T) {
	s := server(t)
	if code, m := post(s, &techB, "preferences.get", `{}`); code != 200 || data(m)["currency"] != "MYR" || data(m)["timezone"] == "" {
		t.Fatalf("preferences default: %d %v", code, m)
	}
	for _, b := range []string{`{"locale":"fr","timezone":"Asia/Kuala_Lumpur"}`, `{"locale":"en","timezone":"Mars/Base"}`, `{"locale":"en","timezone":"Local"}`} {
		if code, _ := write(s, &customerB, "preferences.update", b, 0); code != 422 {
			t.Errorf("%s accepted", b)
		}
	}
	if code, _ := write(s, &hq, "preferences.update", `{"locale":"en","timezone":"UTC","monthlyReportEmail":true}`, 0); code != 422 {
		t.Error("monthlyReportEmail for HQ")
	}
	if code, m := write(s, &customerB, "preferences.update", `{"locale":"ms","timezone":"Asia/Singapore","monthlyReportEmail":true}`, 0); code != 200 || data(m)["locale"] != "ms" {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "preferences.update", `{"locale":"en","timezone":"Asia/Kuala_Lumpur"}`, 0); code != 200 || data(m)["monthlyReportEmail"] != true {
		t.Fatalf("keep monthly flag: %d %v", code, m)
	}
	if _, m := post(s, &customerB, "preferences.get", `{}`); data(m)["locale"] != "en" {
		t.Error("saved preferences")
	}

	owner(t, `UPDATE identity.consents SET granted = false, granted_at = NULL, revoked_at = NULL, version = 1 WHERE membership_id = $1`, seed.ID("customer-b"))
	if code, m := post(s, &customerB, "consents.get", `{"purpose":"location_automation"}`); code != 200 || data(m)["granted"] != false || ver(m) != 1 {
		t.Fatalf("consent get: %d %v", code, m)
	}
	if code, _ := post(s, &customerB, "consents.get", `{"purpose":"marketing"}`); code != 422 {
		t.Error("bad purpose")
	}
	if code, _ := post(s, &hq, "consents.get", `{"purpose":"location_automation"}`); code != 403 {
		t.Error("HQ consent")
	}
	if code, _ := write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":true}`, 3); code != 409 {
		t.Error("stale consent version")
	}
	code, m := write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":true}`, 1)
	if code != 200 || data(m)["grantedAt"] == nil || ver(m) != 2 {
		t.Fatalf("grant: %d %v", code, m)
	}
	if code, m := write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":true}`, 2); code != 200 || ver(m) != 2 {
		t.Error("same value unchanged")
	}
	code, m = write(s, &customerB, "consents.update", `{"purpose":"location_automation","granted":false}`, 2)
	if code != 200 || data(m)["revokedAt"] == nil || data(m)["grantedAt"] == nil {
		t.Fatalf("revoke: %d %v", code, m)
	}
}
