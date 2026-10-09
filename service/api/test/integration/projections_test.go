package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

func freeze(t *testing.T, now time.Time) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { _, err := maintenance.FreezeEnded(ctx, tx, now); return err }); err != nil {
		t.Fatal(err)
	}
}

func byJob(m map[string]any, job string) map[string]any {
	for _, it := range items(m) {
		if it["id"] == job || it["jobId"] == job {
			return it
		}
	}
	return nil
}

// listJob pages through the caller's jobs.list and returns the job's row (nil when it is not listed): the shared test
// database keeps other tests' jobs, so a job can be beyond the first page.
func listJob(s *apiserver.Server, a *actor, job string) map[string]any {
	body := `{"limit":100}`
	for page := 0; page < 50; page++ {
		_, m := post(s, a, "jobs.list", body)
		if row := byJob(m, job); row != nil {
			return row
		}
		next, _ := data(m)["nextCursor"].(string)
		if next == "" {
			return nil
		}
		body = `{"limit":100,"cursor":"` + next + `"}`
	}
	return nil
}

func TestContractorAndTechnicianProjections(t *testing.T) {
	s := server(t)
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND lower(scheduled) > $2`,
		seed.ID("tech-internal-a"), clock.Add(2*time.Hour))
	// earlier runs leave many offers / histories for contractor-a; retire them so the lists stay within one page
	owner(t, `UPDATE maintenance.offers SET expired_at = $2 WHERE contractor_org_id = $1 AND decision IS NULL AND expired_at IS NULL`, seed.ID("org-contractor-a"), clock.Add(-time.Hour))
	owner(t, `UPDATE maintenance.offers SET access_valid_until = $2, access_valid_from = LEAST(access_valid_from, $2::timestamptz - interval '1 hour') WHERE contractor_org_id = $1 AND decision = 'accept'`,
		seed.ID("org-contractor-a"), clock.Add(-time.Hour))
	owner(t, `DELETE FROM maintenance.job_history_snapshots WHERE owner_id = ANY($1)`, []any{seed.ID("org-contractor-a"), seed.ID("tech-internal-a")})
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	var offer string
	ownerScan(t, `SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`, []any{job}, &offer)

	// offer projection before acceptance
	o := listJob(s, &contrA, job)
	if o == nil || o["projection"] != "offer" || o["status"] != "offered" || o["severity"] != nil || len(o["requiredQualifications"].([]any)) == 0 || o["unitId"] != nil {
		t.Fatalf("offer projection: %v", o)
	}
	if code, m := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); code != 200 || data(m)["projection"] != "offer" || data(m)["offerId"] != offer {
		t.Fatalf("get offer: %d %v", code, m)
	}
	for body, want := range map[string]bool{
		`{"filters":{"unitId":"` + unit + `"},"limit":100}`:                                                 false,
		`{"filters":{"severity":"normal"},"limit":100}`:                                                     false,
		`{"filters":{"organizationId":"` + seed.ID("org-contractor-a").String() + `"},"limit":100}`:         true,
		`{"filters":{"organizationId":"` + seed.ID("org-contractor-b").String() + `"},"limit":100}`:         false,
		`{"filters":{"status":"offered"},"limit":100}`:                                                      true,
		`{"filters":{"status":"accepted"},"limit":100}`:                                                     false,
		`{"filters":{"origin":"client_request","from":"` + ts(24) + `","to":"` + ts(26) + `"},"limit":100}`: true,
		`{"filters":{"overdueOnly":true},"limit":100}`:                                                      false,
		`{"sort":{"field":"severity","direction":"desc"},"limit":100}`:                                      true,
		`{"sort":{"field":"dueAt","direction":"asc"},"limit":100}`:                                          true,
		`{"sort":{"field":"id","direction":"desc"},"limit":100}`:                                            true,
	} {
		if _, m := post(s, &contrA, "jobs.list", body); (byJob(m, job) != nil) != want {
			t.Errorf("%s: want %v", body, want)
		}
	}
	if listJob(s, &contrB, job) != nil {
		t.Error("other contractor sees the offer")
	}
	if code, _ := post(s, &contrB, "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("other contractor get")
	}
	// accepted before the access window: still the offer projection with public status accepted
	write(s, &contrA, "jobs.accept", `{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`, 2)
	if _, m := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["projection"] != "offer" || data(m)["status"] != "accepted" {
		t.Fatalf("accepted offer: %v", m)
	}
	if _, m := post(s, &contrA, "jobs.events", `{"jobId":"`+job+`","query":{}}`); len(items(m)) != 1 || items(m)[0]["action"] != "offer.accepted" {
		t.Fatalf("own decision events only: %v", m)
	}
	// inside the access window: summary / detail and all events
	owner(t, `UPDATE maintenance.offers SET access_valid_from = $2 WHERE id = $1`, offer, clock.Add(-time.Hour))
	if _, m := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["projection"] != "detail" {
		t.Fatalf("detail in window: %v", m)
	}
	if listJob(s, &contrA, job)["projection"] != "summary" {
		t.Fatal("summary in window")
	}
	if _, m := post(s, &contrA, "jobs.events", `{"jobId":"`+job+`","query":{}}`); len(items(m)) < 3 {
		t.Fatalf("all events in window: %v", m)
	}
	// after the window: nothing until the worker freezes, then history
	owner(t, `UPDATE maintenance.offers SET access_valid_until = $2 WHERE id = $1`, offer, clock.Add(-time.Minute))
	if code, _ := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("ended window before freeze")
	}
	freeze(t, clock)
	freeze(t, clock) // idempotent
	code, m := post(s, &contrA, "jobs.get", `{"jobId":"`+job+`"}`)
	h := data(m)
	if code != 200 || h["projection"] != "history" || h["dueAt"] != nil || h["severity"] != nil || len(h["ownDecisionEvents"].([]any)) != 1 ||
		h["redactedReportSummary"].(map[string]any)["hasReport"] != false {
		t.Fatalf("history: %d %v", code, m)
	}
	if _, m := post(s, &contrA, "jobs.list", `{"filters":{"status":"assigned"},"limit":100}`); byJob(m, job) != nil {
		t.Error("history status filter")
	}
	if _, m := post(s, &contrA, "jobs.list", `{"filters":{"from":"`+ts(-100)+`"},"limit":100}`); byJob(m, job) != nil {
		t.Error("history period filter on null completedAt")
	}
	if _, m := post(s, &contrA, "jobs.list", `{"filters":{"status":"accepted","organizationId":"`+seed.ID("org-contractor-a").String()+`"},"limit":100}`); byJob(m, job) == nil {
		t.Error("history with own organization")
	}
	if _, m := post(s, &contrA, "jobs.events", `{"jobId":"`+job+`","query":{}}`); len(items(m)) != 1 {
		t.Error("history events are own decisions only")
	}

	// expired unanswered offers disappear
	_, m = write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	j2 := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+j2+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE job_id = $1`, j2, clock)
	if listJob(s, &contrA, j2) != nil {
		t.Error("expired offer listed")
	}

	// technician: detail during the viewing window, history after it
	_, m = write(s, &hq, "jobs.create", jobBody(unit, map[string]string{"alternativeSlots": "[]"}), 0)
	k := data(m)["id"].(string)
	write(s, &hq, "jobs.assign", `{"jobId":"`+k+`","technicianMembershipId":"`+seed.ID("tech-internal-a").String()+`","startAt":"`+ts(25)+`","endAt":"`+ts(27)+`"}`, 1)
	if code, m := post(s, &techInt, "jobs.get", `{"jobId":"`+k+`"}`); code != 200 || data(m)["projection"] != "detail" {
		t.Fatalf("technician detail: %d %v", code, m)
	}
	if _, m := post(s, &techInt, "jobs.list", `{"filters":{"unitId":"`+unit+`"},"limit":100}`); byJob(m, k) == nil {
		t.Fatal("technician summary")
	}
	if code, _ := post(s, &techA, "jobs.get", `{"jobId":"`+k+`"}`); code != 404 {
		t.Error("another technician")
	}
	owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND job_id <> $2 AND scheduled && tstzrange($3, $4)`,
		seed.ID("tech-internal-a"), k, clock.Add(-3*time.Hour), clock.Add(-time.Hour))
	owner(t, `UPDATE maintenance.assignments SET scheduled = tstzrange($2, $3), created_at = $2 WHERE job_id = $1 AND status = 'active'`, k, clock.Add(-3*time.Hour), clock.Add(-time.Hour))
	if code, _ := post(s, &techInt, "jobs.get", `{"jobId":"`+k+`"}`); code != 404 {
		t.Error("ended viewing window before freeze")
	}
	freeze(t, clock)
	if code, m := post(s, &techInt, "jobs.get", `{"jobId":"`+k+`"}`); code != 200 || data(m)["projection"] != "history" || len(data(m)["ownDecisionEvents"].([]any)) != 0 {
		t.Fatalf("technician history: %d %v", code, m)
	}
	if listJob(s, &techInt, k) == nil {
		t.Error("technician history listed")
	}
}
