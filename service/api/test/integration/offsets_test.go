package integration

import (
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestOffsets(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Offset AC")
	quote := func(a *actor, extra string) (int, map[string]any) {
		return write(s, a, "offsets.preview", `{"purpose":"Offset September usage","amountKg":12.5,"period":{"from":"2026-09-01T00:00:00Z","to":"2026-09-14T00:00:00Z"},"unitIds":["`+u+`"]`+extra+`}`, 0)
	}
	for name, b := range map[string]string{"zero kg": `,"amountKg":0`, "4 decimals": `,"amountKg":1.2345`} {
		if code, _ := write(s, &customerB, "offsets.preview", `{"purpose":"x","period":{"from":"2026-09-01T00:00:00Z","to":"2026-09-14T00:00:00Z"},"unitIds":["`+u+`"]`+b+`}`, 0); code != 422 {
			t.Errorf("%s accepted", name)
		}
	}
	if code, _ := quote(&customerA, ""); code != 404 {
		t.Error("client quotes another customer's unit")
	}
	if code, _ := quote(&hq, ""); code != 422 {
		t.Error("HQ quote without customerId")
	}
	code, m := quote(&customerB, "")
	if code != 200 || data(m)["provider"] != "unselected" || data(m)["estimatedAmountMinor"] != nil || data(m)["marketConcept"].(map[string]any)["stage"] != "future_concept" {
		t.Fatalf("quote: %d %v", code, m)
	}
	qid := data(m)["id"].(string)
	sim := func(a *actor, body string, v int) (int, map[string]any) {
		return write(s, a, "offsets.simulate", body, v)
	}
	if code, _ := sim(&customerB, `{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":false}`, 0); code != 422 {
		t.Error("not demo confirmed")
	}
	if code, _ := sim(&customerB, `{"event":"request","quoteId":"`+qid+`","quoteVersion":2,"demoConfirmed":true}`, 0); code != 409 {
		t.Error("stale quote version")
	}
	if code, _ := sim(&customerA, `{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`, 0); code != 404 {
		t.Error("another customer's quote")
	}
	code, m = sim(&customerB, `{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`, 0)
	if code != 200 || data(m)["state"] != "demo_requested" || len(data(m)["attempts"].([]any)) != 1 {
		t.Fatalf("request: %d %v", code, m)
	}
	rid := data(m)["id"].(string)
	att := data(m)["currentAttemptId"].(string)
	if code, _ := sim(&customerB, `{"event":"request","quoteId":"`+qid+`","quoteVersion":1,"demoConfirmed":true}`, 0); code != 409 {
		t.Error("second request for one quote")
	}
	ev := func(event, attempt, eventID string) string {
		return `{"event":"` + event + `","recordId":"` + rid + `","attemptId":"` + attempt + `","eventId":"` + eventID + `","demoConfirmed":true}`
	}
	if code, _ := sim(&customerB, ev("purchase_confirm", att, uuid.NewString()), 1); code != 403 {
		t.Error("client confirms purchase")
	}
	// purchase fails → retry → purchase → retire
	failID := uuid.NewString()
	code, m = sim(&hq, ev("fail", att, failID), 1)
	if code != 200 || data(m)["state"] != "failed" || data(m)["previousState"] != "demo_requested" || data(m)["currentAttemptId"] != att {
		t.Fatalf("fail: %d %v", code, m)
	}
	if code, m := sim(&hq, ev("fail", att, failID), 2); code != 200 || ver(m) != 2 {
		t.Error("same eventId replays")
	}
	if code, _ := sim(&hq, ev("purchase_confirm", att, failID), 2); code != 409 {
		t.Error("same eventId with other content")
	}
	if code, _ := sim(&hq, ev("purchase_confirm", att, uuid.NewString()), 2); code != 409 {
		t.Error("late success while failed")
	}
	retry := func(attempt string, v int) (int, map[string]any) {
		return sim(&customerB, `{"event":"retry","recordId":"`+rid+`","attemptId":"`+attempt+`","demoConfirmed":true}`, v)
	}
	if code, _ := retry(att, 1); code != 409 {
		t.Error("retry with a stale version")
	}
	code, m = retry(att, 2)
	if code != 200 || data(m)["state"] != "demo_requested" || data(m)["currentAttemptId"] == att || len(data(m)["attempts"].([]any)) != 2 {
		t.Fatalf("retry: %d %v", code, m)
	}
	att2 := data(m)["currentAttemptId"].(string)
	if code, _ := sim(&hq, ev("purchase_confirm", att, uuid.NewString()), 3); code != 409 {
		t.Error("old attempt")
	}
	code, m = sim(&hq, ev("purchase_confirm", att2, uuid.NewString()), 3)
	if code != 200 || data(m)["state"] != "demo_purchased" || data(m)["purchaseRef"] == nil {
		t.Fatalf("purchase: %d %v", code, m)
	}
	att3 := data(m)["currentAttemptId"].(string)
	if code, _ := sim(&hq, ev("purchase_confirm", att3, uuid.NewString()), 4); code != 409 {
		t.Error("wrong stage")
	}
	code, m = sim(&hq, ev("retire", att3, uuid.NewString()), 4)
	if code != 200 || data(m)["state"] != "demo_retired" || data(m)["retirementRef"] == nil || data(m)["demoCertificateRef"] == nil || data(m)["currentAttemptId"] != nil {
		t.Fatalf("retire: %d %v", code, m)
	}
	if h := data(m)["eventHistory"].([]any); len(h) != 4 || h[0].(map[string]any)["action"] != "offsets.request" || h[3].(map[string]any)["action"] != "offsets.purchase_confirm" {
		t.Errorf("event history (the retire audit is written with the response): %v", h)
	}
	if code, _ := retry(att3, 5); code != 409 {
		t.Error("retry a retired record")
	}
	// lists
	if _, m := post(s, &customerB, "offsets.list", `{"filters":{"status":"demo_retired"},"limit":100}`); len(items(m)) == 0 {
		t.Error("client list")
	}
	if _, m := post(s, &customerA, "offsets.list", `{"limit":100}`); func() bool {
		for _, it := range items(m) {
			if it["id"] == rid {
				return true
			}
		}
		return false
	}() {
		t.Error("other customer sees the record")
	}
	if _, m := post(s, &hq, "offsets.list", `{"filters":{"customerId":"`+seed.ID("cust-b").String()+`"},"limit":100}`); len(items(m)) == 0 {
		t.Error("HQ list")
	}
	if code, _ := post(s, &hq, "offsets.list", `{"filters":{"status":"sold"}}`); code != 422 {
		t.Error("bad state filter")
	}
	// expired quotes cannot be requested; HQ quotes need the customer's units
	code, m = quote(&hq, `,"customerId":"`+seed.ID("cust-b").String()+`"`)
	if code != 200 {
		t.Fatalf("HQ quote: %d %v", code, m)
	}
	owner(t, `UPDATE energy.offset_quotes SET expires_at = $2 WHERE id = $1`, data(m)["id"], clock.Add(-time.Minute))
	if code, _ := sim(&hq, `{"event":"request","quoteId":"`+data(m)["id"].(string)+`","quoteVersion":1,"demoConfirmed":true}`, 0); code != 409 {
		t.Error("expired quote")
	}
	if code, _ := quote(&hq, `,"customerId":"`+seed.ID("cust-a").String()+`"`); code != 422 {
		t.Error("HQ quote with another customer's unit")
	}
	_ = strconv.Itoa
}
