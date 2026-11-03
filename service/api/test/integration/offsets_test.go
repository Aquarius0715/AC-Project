package integration_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestOffsets(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Offset AC")
	quote := func(a *actor, extra string) (int, map[string]any) {
		return write(
			srv,
			a,
			"offsets.preview",
			`{"purpose":"Offset September usage","amountKg":12.5,"period":{"from":"2026-09-01T00:00:00Z",`+
				`"to":"2026-09-14T00:00:00Z"},"unitIds":["`+unit+`"]`+extra+`}`,
			0,
		)
	}
	want := map[string]string{ // the field each case is refused for
		"4 decimals": "amountKg=error.range",
		"zero kg":    "amountKg=error.range",
	}

	for name, b := range map[string]string{"zero kg": `,"amountKg":0`, "4 decimals": `,"amountKg":1.2345`} {
		code, res := write(
			srv,
			customerB(),
			"offsets.preview",
			`{"purpose":"x","period":{"from":"2026-09-01T00:00:00Z","to":"2026-09-14T00:00:00Z"},"unitIds":["`+unit+`"]`+b+`}`,
			0,
		)
		refusal(t, name, code, res, want[name])
	}

	if code, _ := quote(customerA(), ""); code != 404 {
		t.Error("client quotes another customer's unit")
	}

	if code, m := quote(hq(), ""); code != 422 || fields(m) != "customerId=error.required" {
		t.Errorf("HQ quote without customerId: %d %s", code, fields(m))
	}

	code, resp := quote(customerB(), "")
	if code != 200 || data(resp)["provider"] != "unselected" ||
		data(resp)["estimatedAmountMinor"] != nil ||
		asObj(t, data(resp)["marketConcept"])["stage"] != "future_concept" {
		t.Fatalf("quote: %d %v", code, resp)
	}

	qid := asStr(t, data(resp)["id"])

	sim := func(a *actor, body string, v int) (int, map[string]any) {
		return write(srv, a, "offsets.simulate", body, v)
	}
	if code, res := sim(
		customerB(),
		`{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":false}`,
		0,
	); code != 422 ||
		fields(res) != "demoConfirmed=error.required" {
		t.Errorf("not demo confirmed: %d %s", code, fields(res))
	}

	if code, _ := sim(
		customerB(),
		`{"event":"request","quoteId":"`+qid+`","quoteVersion":2,"demoConfirmed":true}`,
		0,
	); code != 409 {
		t.Error("stale quote version")
	}

	if code, _ := sim(
		customerA(),
		`{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	); code != 404 {
		t.Error("another customer's quote")
	}

	code, resp = sim(
		customerB(),
		`{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	)
	if code != 200 || data(resp)["state"] != "demo_requested" ||
		len(asArr(t, data(resp)["attempts"])) != 1 {
		t.Fatalf("request: %d %v", code, resp)
	}

	rid := asStr(t, data(resp)["id"])
	att := asStr(t, data(resp)["currentAttemptId"])

	if code, _ := sim(
		customerB(),
		`{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	); code != 409 {
		t.Error("second request for one quote")
	}

	ev := func(event, attempt, eventID string) string {
		return `{"event":"` + event + `","recordId":"` + rid + `","attemptId":"` + attempt + `","eventId":"` + eventID +
			`","demoConfirmed":true}`
	}
	if code, _ := sim(customerB(), ev("purchase_confirm", att, uuid.NewString()), 1); code != 403 {
		t.Error("client confirms purchase")
	}
	// purchase fails → retry → purchase → retire
	failID := uuid.NewString()

	code, resp = sim(hq(), ev("fail", att, failID), 1)
	if code != 200 || data(resp)["state"] != "failed" ||
		data(resp)["previousState"] != "demo_requested" ||
		data(resp)["currentAttemptId"] != att {
		t.Fatalf("fail: %d %v", code, resp)
	}

	if code, m := sim(hq(), ev("fail", att, failID), 2); code != 200 || ver(t, m) != 2 {
		t.Error("same eventId replays")
	}

	if code, _ := sim(hq(), ev("purchase_confirm", att, failID), 2); code != 409 {
		t.Error("same eventId with other content")
	}

	if code, _ := sim(hq(), ev("purchase_confirm", att, uuid.NewString()), 2); code != 409 {
		t.Error("late success while failed")
	}

	retry := func(attempt string, v int) (int, map[string]any) {
		return sim(
			customerB(),
			`{"event":"retry","recordId":"`+rid+`","attemptId":"`+attempt+`","demoConfirmed":true}`,
			v,
		)
	}
	if code, _ := retry(att, 1); code != 409 {
		t.Error("retry with a stale version")
	}

	code, resp = retry(att, 2)
	if code != 200 || data(resp)["state"] != "demo_requested" ||
		data(resp)["currentAttemptId"] == att ||
		len(asArr(t, data(resp)["attempts"])) != 2 {
		t.Fatalf("retry: %d %v", code, resp)
	}

	att2 := asStr(t, data(resp)["currentAttemptId"])

	if code, _ := sim(hq(), ev("purchase_confirm", att, uuid.NewString()), 3); code != 409 {
		t.Error("old attempt")
	}

	code, resp = sim(hq(), ev("purchase_confirm", att2, uuid.NewString()), 3)
	if code != 200 || data(resp)["state"] != "demo_purchased" || data(resp)["purchaseRef"] == nil {
		t.Fatalf("purchase: %d %v", code, resp)
	}

	att3 := asStr(t, data(resp)["currentAttemptId"])
	if code, _ := sim(hq(), ev("purchase_confirm", att3, uuid.NewString()), 4); code != 409 {
		t.Error("wrong stage")
	}

	code, resp = sim(hq(), ev("retire", att3, uuid.NewString()), 4)
	if code != 200 || data(resp)["state"] != "demo_retired" || data(resp)["retirementRef"] == nil ||
		data(resp)["demoCertificateRef"] == nil ||
		data(resp)["currentAttemptId"] != nil {
		t.Fatalf("retire: %d %v", code, resp)
	}

	if h := asArr(t, data(resp)["eventHistory"]); len(h) != 4 ||
		asObj(t, h[0])["action"] != "offsets.request" ||
		asObj(t, h[3])["action"] != "offsets.purchase_confirm" {
		t.Errorf("event history (the retire audit is written with the response): %v", h)
	}

	if code, _ := retry(att3, 5); code != 409 {
		t.Error("retry a retired record")
	}
	// a failed retirement retries the retirement: back to demo_purchased with a new retirement attempt, then retired
	_, resp = quote(customerB(), "")
	q2 := asStr(t, data(resp)["id"])
	_, resp = sim(
		customerB(),
		`{"event":"request","quoteId":"`+q2+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	)
	rid2 := asStr(t, data(resp)["id"])
	ev2 := func(event, attempt string) string {
		return `{"event":"` + event + `","recordId":"` + rid2 + `","attemptId":"` + attempt + `","eventId":"` +
			uuid.NewString() + `","demoConfirmed":true}`
	}
	_, resp = sim(hq(), ev2("purchase_confirm", asStr(t, data(resp)["currentAttemptId"])), 1)

	retirement := asStr(t, data(resp)["currentAttemptId"])
	if code, res := sim(
		hq(),
		ev2("fail", retirement),
		2,
	); code != 200 || data(res)["state"] != "failed" ||
		data(res)["previousState"] != "demo_purchased" {
		t.Fatalf("the retirement fails: %d %v", code, res)
	}

	code, resp = sim(
		customerB(),
		`{"event":"retry","recordId":"`+rid2+`","attemptId":"`+retirement+`","demoConfirmed":true}`,
		3,
	)
	if code != 200 || data(resp)["state"] != "demo_purchased" ||
		data(resp)["currentAttemptId"] == retirement ||
		len(asArr(t, data(resp)["attempts"])) != 3 {
		t.Fatalf("retry the retirement: %d %v", code, resp)
	}

	if code, res := sim(
		hq(),
		ev2("retire", asStr(t, data(resp)["currentAttemptId"])),
		4,
	); code != 200 || data(res)["state"] != "demo_retired" ||
		data(res)["purchaseRef"] == nil {
		t.Errorf("retired after the retry: %d %v", code, res)
	}
	// what is not there, or not the customer's, is NOT_FOUND
	if code, _ := sim(
		customerB(),
		`{"event":"request","quoteId":"`+uuid.NewString()+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	); code != 404 {
		t.Error("request of an unknown quote")
	}

	if code, _ := sim(
		hq(),
		`{"event":"fail",`+
			`"recordId":"`+uuid.NewString()+`","attemptId":"`+uuid.NewString()+`","eventId":"`+uuid.NewString()+
			`","demoConfirmed":true}`,
		1,
	); code != 404 {
		t.Error("event of an unknown record")
	}

	if code, _ := quote(
		customerB(),
		`,"customerId":"`+seed.ID("cust-a").String()+`"`,
	); code != 404 {
		t.Error("client quotes for another customer")
	}
	// lists
	_, resp = post(
		srv,
		customerB(),
		"offsets.list",
		`{"filters":{"status":"demo_retired"},"limit":100}`,
	)
	if len(items(t, resp)) == 0 {
		t.Error("client list")
	}

	for _, it := range items(t, resp) { // a customer never sees an actor's name (IR305)
		for _, e := range asArr(t, it["eventHistory"]) {
			if asObj(t, e)["actorName"] != nil {
				t.Errorf("client event history shows a name: %v", e)
			}
		}
	}

	if _, m := post(srv, customerA(), "offsets.list", `{"limit":100}`); func() bool {
		for _, it := range items(t, m) {
			if it["id"] == rid {
				return true
			}
		}

		return false
	}() {
		t.Error("other customer sees the record")
	}

	if code, res := post(
		srv,
		hq(),
		"offsets.list",
		`{"filters":{"from":"2026-09-14T00:00:00Z","to":"2026-09-01T00:00:00Z"}}`,
	); code != 422 ||
		asObj(t, res["fieldErrors"])["filters.to"] != apperr.KeyRange {
		t.Errorf("a reversed created period: %d %v", code, res)
	}

	_, resp = post(
		srv,
		hq(),
		"offsets.list",
		`{"filters":{"customerId":"`+seed.ID("cust-b").String()+`"},"limit":100}`,
	)
	if len(items(t, resp)) == 0 {
		t.Error("HQ list")
	}

	named := false // HQ sees who acted

	for _, it := range items(t, resp) {
		for _, e := range asArr(t, it["eventHistory"]) {
			named = named || asObj(t, e)["actorName"] != nil
		}
	}

	if !named {
		t.Error("HQ event history has no actor names")
	}

	if code, res := post(
		srv,
		hq(),
		"offsets.list",
		`{"filters":{"status":"sold"}}`,
	); code != 422 ||
		fields(res) != "filters=error.invalid" {
		t.Errorf("bad state filter: %d %s", code, fields(res))
	}
	// expired quotes cannot be requested; HQ quotes need the customer's units
	code, resp = quote(hq(), `,"customerId":"`+seed.ID("cust-b").String()+`"`)
	if code != 200 {
		t.Fatalf("HQ quote: %d %v", code, resp)
	}

	owner(
		t,
		`UPDATE energy.offset_quotes SET expires_at = $2 WHERE id = $1`,
		data(resp)["id"],
		clock().Add(-time.Minute),
	)

	if code, _ := sim(
		hq(),
		`{"event":"request","quoteId":"`+asStr(
			t,
			data(resp)["id"],
		)+`","quoteVersion":1,"demoConfirmed":true}`,
		0,
	); code != 409 {
		t.Error("expired quote")
	}

	if code, m := quote(
		hq(),
		`,"customerId":"`+seed.ID("cust-a").String()+`"`,
	); code != 422 ||
		fields(m) != "unitIds=errors.units_outside_customer" {
		t.Errorf("HQ quote with another customer's unit: %d %s", code, fields(m))
	}

	_ = strconv.Itoa
}
