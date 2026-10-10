package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestInquiries(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	_, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0)
	rid := data(m)["id"].(string)
	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"payment without invoice":  {`{"subjectType":"payment","message":"why"}`, 422},
		"payment with restriction": {`{"subjectType":"payment","invoiceId":"` + inv + `","restrictionId":"` + rid + `","message":"why"}`, 422},
		"restriction without id":   {`{"subjectType":"restriction","message":"why"}`, 422},
		"bad subject":              {`{"subjectType":"other","message":"why"}`, 422},
		"blank message":            {`{"subjectType":"payment","invoiceId":"` + inv + `","message":"  "}`, 422},
		"unknown invoice":          {`{"subjectType":"payment","invoiceId":"` + uuid.NewString() + `","message":"why"}`, 404},
		"unknown restriction":      {`{"subjectType":"restriction","restrictionId":"` + uuid.NewString() + `","message":"why"}`, 404},
		"unknown cited invoice":    {`{"subjectType":"restriction","restrictionId":"` + rid + `","invoiceId":"` + uuid.NewString() + `","message":"why"}`, 404},
	} {
		if code, m := write(s, &customerB, "inquiries.create", tc.body, 0); code != tc.code {
			t.Errorf("%s: %d want %d %v", name, code, tc.code, m)
		}
	}
	if code, _ := write(s, &customerA, "inquiries.create", `{"subjectType":"payment","invoiceId":"`+inv+`","message":"why"}`, 0); code != 404 {
		t.Error("other customer's invoice")
	}
	if code, _ := write(s, &hq, "inquiries.create", `{"subjectType":"payment","invoiceId":"`+inv+`","message":"why"}`, 0); code != 403 {
		t.Error("HQ creates inquiry")
	}
	code, m := write(s, &customerB, "inquiries.create", `{"subjectType":"restriction","restrictionId":"`+rid+`","invoiceId":"`+inv+`","message":"I paid yesterday"}`, 0)
	if code != 200 || data(m)["state"] != "received" || data(m)["reply"] != nil || data(m)["customerId"] != seed.ID("cust-b").String() {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	var received int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2`, []any{id, seed.ID("hq-operator")}, &received)
	if received != 1 {
		t.Error("HQ notified of the inquiry")
	}
	if _, m := post(s, &customerB, "inquiries.list", `{"filters":{"restrictionId":"`+rid+`"}}`); len(items(m)) != 1 {
		t.Error("client list")
	}
	if _, m := post(s, &customerA, "inquiries.list", `{"filters":{"restrictionId":"`+rid+`"}}`); len(items(m)) != 0 {
		t.Error("other customer's inquiries")
	}
	if _, m := post(s, &hq, "inquiries.list", `{"filters":{"status":"received","subjectType":"restriction","invoiceId":"`+inv+`"},"limit":100}`); len(items(m)) != 1 {
		t.Errorf("HQ list: %v", m)
	}
	if code, _ := post(s, &hq, "inquiries.list", `{"filters":{"status":"open"}}`); code != 422 {
		t.Error("bad state filter")
	}
	if code, _ := post(s, &techB, "inquiries.list", `{}`); code != 403 {
		t.Error("technician inquiries")
	}
	answer := func(reply string, v int) (int, map[string]any) {
		return write(s, &hq, "inquiries.answer", `{"inquiryId":"`+id+`","reply":"`+reply+`"}`, v)
	}
	if code, _ := answer(" ", 1); code != 422 {
		t.Error("blank reply")
	}
	if code, _ := answer("ok", 2); code != 409 {
		t.Error("stale inquiry")
	}
	if code, _ := write(s, &customerB, "inquiries.answer", `{"inquiryId":"`+id+`","reply":"x"}`, 1); code != 403 {
		t.Error("client answers")
	}
	code, m = answer("Payment received, restriction will be lifted", 1)
	if code != 200 || data(m)["state"] != "answered" || ver(m) != 2 {
		t.Fatalf("answer: %d %v", code, m)
	}
	if code, _ := answer("again", 2); code != 409 {
		t.Error("answer twice")
	}
	if _, m := post(s, &customerB, "notifications.list", `{"filters":{"type":"inquiry"},"limit":100}`); len(items(m)) == 0 {
		t.Error("client notified of the answer")
	}
	if code, _ := write(s, &hq, "inquiries.answer", `{"inquiryId":"`+uuid.NewString()+`","reply":"x"}`, 1); code != 404 {
		t.Error("unknown inquiry")
	}
}

func TestAuditList(t *testing.T) {
	s := server(t)
	_, inv := newInvoice(t, s)
	period := func(from, to time.Time) string {
		return `"from":"` + from.Format(time.RFC3339) + `","to":"` + to.Format(time.RFC3339) + `"`
	}
	day := period(clock.Add(-time.Hour), clock.Add(time.Hour))
	_, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"targetKind":"invoice","targetId":"`+inv+`"}}`)
	if len(items(m)) != 1 || items(m)[0]["action"] != "invoices.create" || items(m)[0]["result"] != "success" {
		t.Fatalf("invoice audit: %v", m)
	}
	if items(m)[0]["actorName"] != "hq-operator" { // the actor user's display name (IR305)
		t.Errorf("actor name: %v", items(m)[0]["actorName"])
	}
	sys := uuid.NewString() // a system actor matches no user, so it has no name
	owner(t, `INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, action, target_kind, target_id, occurred_at, correlation_id, result)
		VALUES ($1, 'system-demo', 'system', 'demo.tick', 'test', $2, $3, $4, 'success')`, seed.ID("tenant-a"), sys, clock, uuid.NewString())
	if _, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"targetKind":"test","targetId":"`+sys+`"}}`); len(items(m)) != 1 || items(m)[0]["actorName"] != nil || items(m)[0]["actorId"] != "system-demo" {
		t.Errorf("system actor: %v", m)
	}
	corr := items(m)[0]["correlationId"].(string)
	if _, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"correlationId":"`+corr+`"},"limit":100}`); len(items(m)) < 1 {
		t.Error("by correlation")
	}
	if _, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"correlationId":"`+uuid.NewString()+`"}}`); len(items(m)) != 0 {
		t.Error("unknown correlation is empty")
	}
	if _, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"result":"denied"},"sort":{"field":"occurredAt","direction":"asc"},"limit":2}`); m["data"] == nil {
		t.Error("result filter")
	}
	for _, b := range []string{`{}`, `{"filters":{"from":"` + clock.Format(time.RFC3339) + `"}}`, `{"filters":{` + period(clock, clock.Add(-time.Hour)) + `}}`,
		`{"filters":{` + period(clock.Add(-400*24*time.Hour), clock) + `}}`, `{"filters":{` + day + `,"result":"ok"}}`, `{"filters":{` + day + `,"x":1}}`} {
		if code, _ := post(s, &hq, "audit.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerB, "audit.list", `{"filters":{`+day+`}}`); code != 403 {
		t.Error("client audit")
	}
}
