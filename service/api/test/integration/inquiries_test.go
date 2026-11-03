package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const fieldFiltersTo = "filters.to"

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestInquiries(t *testing.T) {
	srv := server(t)
	u := boundUnit(t, srv, "online")
	k, inv := overdueContract(t, srv, u)
	_, resp := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(k, inv, []string{u}, nil),
		0,
	)
	rid := asStr(t, data(resp)["id"])
	refused := map[string]string{ // the field each VALIDATION case is refused for
		"bad subject":              "subjectType=error.invalid",
		"blank message":            refusedMessageLength,
		"payment with restriction": "restrictionId=error.notAllowed",
		"payment without invoice":  refusedInvoiceIDRequired,
		"restriction without id":   "restrictionId=error.required",
	}

	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"payment without invoice": {`{"subjectType":"payment","message":"why"}`, 422},
		"payment with restriction": {`{"subjectType":"payment",` +
			`"invoiceId":"` + inv + `","restrictionId":"` + rid + `","message":"why"}`, 422},
		"restriction without id": {`{"subjectType":"restriction","message":"why"}`, 422},
		"bad subject":            {`{"subjectType":"other","message":"why"}`, 422},
		"blank message":          {`{"subjectType":"payment","invoiceId":"` + inv + `","message":"  "}`, 422},
		"unknown invoice":        {`{"subjectType":"payment","invoiceId":"` + uuid.NewString() + `","message":"why"}`, 404},
		"unknown restriction": {`{"subjectType":"restriction",` +
			`"restrictionId":"` + uuid.NewString() + `","message":"why"}`, 404},
		"unknown cited invoice": {`{"subjectType":"restriction",` +
			`"restrictionId":"` + rid + `","invoiceId":"` + uuid.NewString() + `","message":"why"}`, 404},
	} {
		code, m := write(srv, customerB(), "inquiries.create", tc.body, 0)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != tc.code {
			t.Errorf("%s: %d want %d %v", name, code, tc.code, m)
		}
	}

	if code, _ := write(
		srv,
		customerA(),
		"inquiries.create",
		`{"subjectType":"payment","invoiceId":"`+inv+`","message":"why"}`,
		0,
	); code != 404 {
		t.Error("other customer's invoice")
	}

	if code, _ := write(
		srv,
		hq(),
		"inquiries.create",
		`{"subjectType":"payment","invoiceId":"`+inv+`","message":"why"}`,
		0,
	); code != 403 {
		t.Error("HQ creates inquiry")
	}

	code, resp := write(
		srv,
		customerB(),
		"inquiries.create",
		`{"subjectType":"restriction","restrictionId":"`+rid+`","invoiceId":"`+inv+`","message":"I paid yesterday"}`,
		0,
	)
	if code != 200 || data(resp)["state"] != "received" || data(resp)["reply"] != nil ||
		data(resp)["customerId"] != seed.ID("cust-b").String() {
		t.Fatalf("create: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	var received int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1 AND recipient_membership_id = $2`,
		[]any{id, seed.ID("hq-operator")},
		&received,
	)

	if received != 1 {
		t.Error("HQ notified of the inquiry")
	}

	if _, res := post(
		srv,
		customerB(),
		"inquiries.list",
		`{"filters":{"restrictionId":"`+rid+`"}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Error("client list")
	}

	if code, res := post(
		srv,
		customerA(),
		"inquiries.list",
		`{"filters":{"restrictionId":"`+rid+`"}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("other customer's inquiries")
	}

	if _, res := post(
		srv,
		hq(),
		"inquiries.list",
		`{"filters":{"status":"received","subjectType":"restriction","invoiceId":"`+inv+`"},"limit":100}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("HQ list: %v", res)
	}

	if code, res := post(
		srv,
		hq(),
		"inquiries.list",
		`{"filters":{"status":"open"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad state filter: %d %s", code, fields(res))
	}

	if code, _ := post(srv, techB(), "inquiries.list", `{}`); code != 403 {
		t.Error("technician inquiries")
	}

	answer := func(reply string, v int) (int, map[string]any) {
		return write(srv, hq(), "inquiries.answer", `{"inquiryId":"`+id+`","reply":"`+reply+`"}`, v)
	}
	if code, m := answer(" ", 1); code != 422 || fields(m) != "reply=error.length" {
		t.Errorf("blank reply: %d %s", code, fields(m))
	}

	if code, _ := answer("ok", 2); code != 409 {
		t.Error("stale inquiry")
	}

	if code, _ := write(
		srv,
		customerB(),
		"inquiries.answer",
		`{"inquiryId":"`+id+`","reply":"x"}`,
		1,
	); code != 403 {
		t.Error("client answers")
	}

	code, resp = answer("Payment received, restriction will be lifted", 1)
	if code != 200 || data(resp)["state"] != "answered" || ver(t, resp) != 2 {
		t.Fatalf("answer: %d %v", code, resp)
	}

	if code, _ := answer("again", 2); code != 409 {
		t.Error("answer twice")
	}

	if _, res := post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"type":"inquiry"},"limit":100}`,
	); len(
		items(t, res),
	) == 0 {
		t.Error("client notified of the answer")
	}

	if code, _ := write(
		srv,
		hq(),
		"inquiries.answer",
		`{"inquiryId":"`+uuid.NewString()+`","reply":"x"}`,
		1,
	); code != 404 {
		t.Error("unknown inquiry")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAuditList(t *testing.T) {
	srv := server(t)
	_, inv := newInvoice(t, srv)
	period := func(from, to time.Time) string {
		return `"from":"` + from.Format(time.RFC3339) + `","to":"` + to.Format(time.RFC3339) + `"`
	}
	day := period(clock().Add(-time.Hour), clock().Add(time.Hour))

	_, resp := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"targetKind":"invoice","targetId":"`+inv+`"}}`,
	)
	if len(items(t, resp)) != 1 || items(t, resp)[0]["action"] != "invoices.create" ||
		items(t, resp)[0]["result"] != "success" {
		t.Fatalf("invoice audit: %v", resp)
	}

	if items(t, resp)[0]["actorName"] != membershipHQOperator { // the actor user's display name (IR305)
		t.Errorf("actor name: %v", items(t, resp)[0]["actorName"])
	}

	sys := uuid.NewString() // a system actor matches no user, so it has no name
	owner(
		t,
		`INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, action, target_kind, target_id, occurred_at,
			correlation_id, result)
		VALUES ($1, 'system-demo', 'system', 'demo.tick', 'test', $2, $3, $4, 'success')`,
		seed.ID("tenant-a"),
		sys,
		clock(),
		uuid.NewString(),
	)

	if _, res := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"targetKind":"test","targetId":"`+sys+`"}}`,
	); len(items(t, res)) != 1 || items(t, res)[0]["actorName"] != nil ||
		items(t, res)[0]["actorId"] != "system-demo" {
		t.Errorf("system actor: %v", res)
	}

	corr := asStr(t, items(t, resp)[0]["correlationId"])
	if _, res := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"correlationId":"`+corr+`"},"limit":100}`,
	); len(
		items(t, res),
	) < 1 {
		t.Error("by correlation")
	}

	if code, res := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"correlationId":"`+uuid.NewString()+`"}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("unknown correlation is empty")
	}

	if _, m := post(
		srv,
		hq(),
		"audit.list",
		`{"filters":{`+day+`,"result":"denied"},"sort":{"field":"occurredAt","direction":"asc"},"limit":2}`,
	); m["data"] == nil {
		t.Error("result filter")
	}
	// the period is required (both ends named when missing), from < to and at most 366 days
	for b, want := range map[string]map[string]string{
		`{}`: {"filters.from": apperr.KeyRequired, fieldFiltersTo: apperr.KeyRequired},
		`{"filters":{"from":"` + clock().Format(time.RFC3339) + `"}}`:           {fieldFiltersTo: apperr.KeyRequired},
		`{"filters":{"to":"` + clock().Format(time.RFC3339) + `"}}`:             {"filters.from": apperr.KeyRequired},
		`{"filters":{` + period(clock(), clock().Add(-time.Hour)) + `}}`:        {fieldFiltersTo: "errors.period_range"},
		`{"filters":{` + period(clock().Add(-400*24*time.Hour), clock()) + `}}`: {fieldFiltersTo: "errors.period_range"},
		`{"filters":{` + day + `,"result":"ok"}}`:                               {fieldFilters: apperr.KeyInvalid},
		`{"filters":{` + day + `,"x":1}}`:                                       {"x": apperr.KeyNotAllowed},
	} {
		code, m := post(srv, hq(), "audit.list", b)

		fe, _ := m["fieldErrors"].(map[string]any)
		if code != 422 || len(fe) != len(want) {
			t.Errorf("%s: %d %v, want %v", b, code, fe, want)

			continue
		}

		for k, v := range want {
			if fe[k] != v {
				t.Errorf("%s: %v, want %v", b, fe, want)
			}
		}
	}

	if code, _ := post(srv, customerB(), "audit.list", `{"filters":{`+day+`}}`); code != 403 {
		t.Error("client audit")
	}
}
