package integration

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestDeniedWritesAudited (IR603, DEC-90): a write refused for permission (FORBIDDEN, before the handler) or for scope
// (NOT_FOUND, a record outside the caller's scope) leaves an audit entry with the result denied — the actor, the
// operation and the target it asked for, the refusal as the reason — at most one per actor and operation a minute.
// A refusal of the input (VALIDATION) and a refused read leave none.
func TestDeniedWritesAudited(t *testing.T) {
	s := server(t)
	day := `"from":"` + clock.Add(-24*time.Hour).Format(time.RFC3339) + `","to":"` + clock.Add(time.Hour).Format(time.RFC3339) + `"`
	denied := func(action, actor string) []map[string]any {
		t.Helper()
		drainEvents(t)
		_, m := post(s, &hq, "audit.list", `{"filters":{`+day+`,"result":"denied","actorId":"`+actor+`"},"limit":100}`)
		out := []map[string]any{}
		for _, a := range items(m) {
			if a["action"] == action {
				out = append(out, a)
			}
		}
		return out
	}
	contractor, operator := seed.ID("user-contractor-a").String(), seed.ID("user-hq-operator").String()
	unit := seed.ID("unit-online-rto").String()
	before := denied("units.archive", contractor)

	// permission: a contractor may not archive a unit (no candidate role); twice in a row
	minute := time.Now().Truncate(time.Minute)
	for range 2 {
		if code, m := write(s, &contrA, "units.archive", `{"id":"`+unit+`","reason":"probe"}`, 1); code != 403 {
			t.Fatalf("a contractor archives a unit: %d %v", code, m)
		}
	}
	got := denied("units.archive", contractor)
	want := 1
	if !time.Now().Truncate(time.Minute).Equal(minute) { // the attempts straddled a minute: one entry each minute at most
		want = 2
	}
	if n := len(got) - len(before); n < 1 || n > want {
		t.Fatalf("denied entries of the two attempts: %d, want 1 a minute", n)
	}
	e := got[0]
	if e["actorRoleAtTime"] != "contractor" || e["result"] != "denied" || e["reason"] != "error.forbidden" ||
		e["targetRef"].(map[string]any)["kind"] != "unit" || e["targetRef"].(map[string]any)["id"] != unit || e["correlationId"] == "" {
		t.Errorf("the denied entry names the actor, the operation, the target and the refusal: %v", e)
	}

	// scope: a contractor reviewing a job HQ keeps in-house (NOT_FOUND) — the job it asked for is the target
	_, m := write(s, &customerA, "jobs.create", jobBody(seed.ID("unit-non-rto").String(), nil), 0)
	job := data(m)["id"].(string)
	if code, m := write(s, &contrA, "jobs.review", `{"jobId":"`+job+`","reportVersion":1,"decision":"accept","reviewMode":"normal"}`, 1); code != 404 {
		t.Fatalf("a contractor reviews an in-house job: %d %v", code, m)
	}
	var target any
	for _, a := range denied("jobs.review", contractor) {
		if a["targetRef"].(map[string]any)["id"] == job {
			target = a["targetRef"]
		}
	}
	if target == nil || target.(map[string]any)["kind"] != "job" {
		t.Errorf("the scope refusal records the job asked for: %v", target)
	}
	// a refused read is not audited
	if code, _ := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Errorf("a contractor reads an in-house job: %d", code)
	}
	if n := len(denied("jobs.get", contractor)); n != 0 {
		t.Errorf("a refused read left %d denied entries", n)
	}
	// the input's own refusal is not a denial
	if code, _ := write(s, &hq, "units.archive", `{"id":"`+unit+`","reason":""}`, 1); code != 422 {
		t.Errorf("an archive without a reason: %d", code)
	}
	if n := len(denied("units.archive", operator)); n != 0 {
		t.Errorf("a VALIDATION refusal left %d denied entries", n)
	}
}
