package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

func freeze(t *testing.T, now time.Time) {
	t.Helper()

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	beginFuncErr := pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error {
			_, err := maintenance.FreezeEnded(ctx, tx, now)

			return apperr.Wrap(err, "maintenance.FreezeEnded")
		},
	)
	if beginFuncErr != nil {
		t.Fatal(beginFuncErr)
	}
}

func byJob(t *testing.T, m map[string]any, job string) map[string]any {
	t.Helper()

	for _, it := range items(t, m) {
		if it["id"] == job || it["jobId"] == job {
			return it
		}
	}

	return nil
}

// listJob pages through the caller's jobs.list and returns the job's row (nil when it is not listed): the shared test
// database keeps other tests' jobs, so a job can be beyond the first page.
func listJob(t *testing.T, s *apiserver.Server, a *actor, job string) map[string]any {
	t.Helper()

	body := bodyFirstHundred
	for range 50 {
		_, m := post(s, a, "jobs.list", body)
		if row := byJob(t, m, job); row != nil {
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
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			lower(scheduled) > $2`,
		seed.ID("tech-internal-a"),
		clock().Add(2*time.Hour),
	)
	// earlier runs leave many offers / histories for contractor-a; retire them so the lists stay within one page
	owner(
		t,
		`UPDATE maintenance.offers SET expired_at = $2 WHERE contractor_org_id = $1 AND decision IS NULL AND expired_at IS
			NULL`,
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.offers SET access_valid_until = $2, access_valid_from = LEAST(access_valid_from, $2::timestamptz -
			interval '1 hour') WHERE contractor_org_id = $1 AND decision = 'accept'`,
		seed.ID("org-contractor-a"),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`DELETE FROM maintenance.job_history_snapshots WHERE owner_id = ANY($1)`,
		[]any{seed.ID("org-contractor-a"), seed.ID("tech-internal-a")},
	)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, res := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(res)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	)

	var offer string
	ownerScan(
		t,
		`SELECT id::text FROM maintenance.offers WHERE job_id = $1 AND decision IS NULL`,
		[]any{job},
		&offer,
	)

	// offer projection before acceptance
	o := listJob(t, srv, contrA(), job)
	if o == nil || o["projection"] != projectionOffer || o["status"] != statusOffered ||
		o["severity"] != nil ||
		len(asArr(t, o["requiredQualifications"])) == 0 ||
		o["unitId"] != nil {
		t.Fatalf("offer projection: %v", o)
	}

	if code, resp := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); code != 200 || data(resp)["projection"] != projectionOffer ||
		data(resp)["offerId"] != offer {
		t.Fatalf("get offer: %d %v", code, resp)
	}

	for body, want := range map[string]bool{
		`{"filters":{"unitId":"` + unit + `"},"limit":100}`:                                                       false,
		`{"filters":{"severity":"normal"},"limit":100}`:                                                           false,
		`{"filters":{"organizationId":"` + seed.ID("org-contractor-a").String() + `"},"limit":100}`:               true,
		`{"filters":{"organizationId":"` + seed.ID("org-contractor-b").String() + `"},"limit":100}`:               false,
		`{"filters":{"status":"offered"},"limit":100}`:                                                            true,
		`{"filters":{"status":"accepted"},"limit":100}`:                                                           false,
		`{"filters":{"origin":"client_request","from":"` + stamp(24) + `","to":"` + stamp(26) + `"},"limit":100}`: true,
		`{"filters":{"overdueOnly":true},"limit":100}`:                                                            false,
		`{"filters":{"type":"reactive"},` +
			`"limit":100}`: true, // the offer's public type (IR290)
		`{"filters":{"type":"periodic"},"limit":100}`:                  false,
		`{"sort":{"field":"severity","direction":"desc"},"limit":100}`: true,
		`{"sort":{"field":"dueAt","direction":"asc"},"limit":100}`:     true,
		`{"sort":{"field":"id","direction":"desc"},"limit":100}`:       true,
	} {
		if code, resp := post(
			srv,
			contrA(),
			"jobs.list",
			body,
		); code != 200 ||
			(byJob(t, resp, job) != nil) != want {
			t.Errorf("%s: want %v", body, want)
		}
	}

	if listJob(t, srv, contrB(), job) != nil {
		t.Error("other contractor sees the offer")
	}

	if code, _ := post(srv, contrB(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("other contractor get")
	}
	// accepted before the access window: still the offer projection with public status accepted
	write(
		srv,
		contrA(),
		"jobs.accept",
		`{"jobId":"`+job+`","offerId":"`+offer+`","termsVersion":"t"}`,
		2,
	)

	if _, resp := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); data(resp)["projection"] != projectionOffer ||
		data(resp)["status"] != statusAccepted {
		t.Fatalf("accepted offer: %v", resp)
	}

	if _, resp := post(
		srv,
		contrA(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{}}`,
	); len(items(t, resp)) != 1 ||
		items(t, resp)[0]["action"] != "offer.accepted" {
		t.Fatalf("own decision events only: %v", resp)
	}
	// inside the access window: summary / detail and all events
	owner(
		t,
		`UPDATE maintenance.offers SET access_valid_from = $2 WHERE id = $1`,
		offer,
		clock().Add(-time.Hour),
	)

	if _, resp := post(
		srv,
		contrA(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); data(resp)["projection"] != "detail" {
		t.Fatalf("detail in window: %v", resp)
	}

	if row := listJob(t, srv, contrA(), job); row["projection"] != projectionSummary {
		t.Fatal("summary in window")
	} else if row["accessValidFrom"] != clock().Add(-time.Hour).Format(time.RFC3339) ||
		row["accessValidUntil"] != stamp(100) {
		t.Fatalf("the summary carries the accepted offer's access window (IR227): %v", row)
	}

	if _, resp := post(
		srv,
		contrA(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{}}`,
	); len(
		items(t, resp),
	) < 3 {
		t.Fatalf("all events in window: %v", resp)
	}
	// the list counts the events the caller reads and those the customer reads, with the newest (IR506); HQ's
	// internal note is the contractor's to read, not the customer's
	if code, _ := write(
		srv,
		hq(),
		"jobs.addNote",
		`{"jobId":"`+job+`","message":"Check parts stock","visibility":"internal"}`,
		int(asNum(t, listJob(t, srv, contrA(), job)["version"])),
	); code != 200 {
		t.Fatalf("internal note: %d", code)
	}

	_, all := post(srv, contrA(), "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
	_, seen := post(srv, customerA(), "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
	es := asObj(t, listJob(t, srv, contrA(), job)["eventSummary"])

	newest := items(t, all)[len(items(t, all))-1]
	if es["count"] != float64(len(items(t, all))) ||
		es["customerVisible"] != float64(len(items(t, seen))) ||
		es["customerVisible"] == es["count"] ||
		es["notes"] != float64(1) ||
		asObj(t, es["latest"])["action"] != newest["action"] ||
		asObj(t, es["latest"])["occurredAt"] != newest["occurredAt"] {
		t.Fatalf(
			"event summary: %v (contractor reads %d, customer %d)",
			es,
			len(items(t, all)),
			len(items(t, seen)),
		)
	}

	if cs := asObj(t, listJob(t, srv, customerA(), job)["eventSummary"]); cs["count"] != float64(
		len(items(t, seen)),
	) ||
		cs["count"] != cs["customerVisible"] ||
		cs["notes"] != float64(0) {
		t.Fatalf("the customer's own summary counts what it reads: %v", cs)
	}
	// after the window: nothing until the worker freezes, then history
	owner(
		t,
		`UPDATE maintenance.offers SET access_valid_until = $2 WHERE id = $1`,
		offer,
		clock().Add(-time.Minute),
	)

	if code, _ := post(srv, contrA(), "jobs.get", `{"jobId":"`+job+`"}`); code != 404 {
		t.Error("ended window before freeze")
	}

	freeze(t, clock())
	freeze(t, clock()) // idempotent

	code, res := post(srv, contrA(), "jobs.get", `{"jobId":"`+job+`"}`)

	h := data(res)
	if code != 200 || h["projection"] != projectionHistory || h["dueAt"] != nil ||
		h["severity"] != nil ||
		len(asArr(t, h["ownDecisionEvents"])) != 1 ||
		asObj(t, h["redactedReportSummary"])["hasReport"] != false {
		t.Fatalf("history: %d %v", code, res)
	}

	if code, resp := post(
		srv,
		contrA(),
		"jobs.list",
		`{"filters":{"status":"assigned"},"limit":100}`,
	); code != 200 ||
		byJob(t, resp, job) != nil {
		t.Error("history status filter")
	}
	// without a completion the history counts at its as-of, when the delegation ended (IR614)
	for period, want := range map[string]bool{
		`"from":"` + stamp(-100) + `"`: true, `"from":"` + stamp(1) + `"`: false,
		`"to":"` + stamp(-100) + `"`: false,
	} {
		if code, resp := post(
			srv,
			contrA(),
			"jobs.list",
			`{"filters":{`+period+`},"limit":100}`,
		); code != 200 ||
			(byJob(t, resp, job) != nil) != want {
			t.Errorf("history period filter {%s} on null completedAt: want %v", period, want)
		}
	}

	if _, resp := post(
		srv,
		contrA(),
		"jobs.list",
		`{"filters":{"status":"accepted","organizationId":"`+seed.ID("org-contractor-a").
			String()+
			`"},"limit":100}`,
	); byJob(t,
		resp,
		job,
	) == nil {
		t.Error("history with own organization")
	}

	for typ, want := range map[string]bool{"reactive": true, "preventive": false} { // the history's public type (IR290)
		if code, resp := post(
			srv,
			contrA(),
			"jobs.list",
			`{"filters":{"type":"`+typ+`"},"limit":100}`,
		); code != 200 ||
			(byJob(t, resp, job) != nil) != want {
			t.Errorf("history type filter %s: want %v", typ, want)
		}
	}

	if _, resp := post(
		srv,
		contrA(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{}}`,
	); len(
		items(t, resp),
	) != 1 {
		t.Error("history events are own decisions only")
	}

	// expired unanswered offers disappear
	_, res = write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	j2 := asStr(t, data(res)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+j2+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	)
	owner(t, `UPDATE maintenance.offers SET offer_expires_at = $2 WHERE job_id = $1`, j2, clock())

	if listJob(t, srv, contrA(), j2) != nil {
		t.Error("expired offer listed")
	}

	// technician: detail during the viewing window, history after it
	_, res = write(
		srv,
		hq(),
		"jobs.create",
		jobBody(unit, map[string]string{fieldAlternativeSlots: "[]"}),
		0,
	)
	key := asStr(t, data(res)["id"])
	write(
		srv,
		hq(),
		"jobs.assign",
		`{"jobId":"`+key+`","technicianMembershipId":"`+seed.ID("tech-internal-a").
			String()+
			`","startAt":"`+stamp(
			25,
		)+`","endAt":"`+stamp(
			27,
		)+`"}`,
		1,
	)

	if code, resp := post(
		srv,
		techInt(),
		"jobs.get",
		`{"jobId":"`+key+`"}`,
	); code != 200 ||
		data(resp)["projection"] != "detail" {
		t.Fatalf("technician detail: %d %v", code, resp)
	}

	if _, resp := post(
		srv,
		techInt(),
		"jobs.list",
		`{"filters":{"unitId":"`+unit+`"},"limit":100}`,
	); byJob(t,
		resp,
		key,
	) == nil {
		t.Fatal("technician summary")
	} else if row := byJob(t,
		resp,
		key,
	); row["technicianMembershipId"] != seed.ID("tech-internal-a").String() ||
		row["assignmentAcknowledgement"] != statusPending {
		t.Fatalf("the summary names the active assignment's technician (IR225): %v", row)
	}

	if _, resp := post(
		srv,
		hq(),
		"jobs.list",
		`{"filters":{"unitId":"`+unit+`"},"limit":100}`,
	); byJob(t, resp, job)["technicianMembershipId"] != nil {
		t.Errorf("a job without an active assignment has no technician: %v", byJob(t, resp, job))
	} else if byJob(t, resp, key)["accessValidFrom"] != nil || byJob(t, resp, key)["accessValidUntil"] != nil ||
		byJob(t, resp, job)["accessValidUntil"] == nil {
		t.Errorf(
			"an internal job has no access window, a delegated one keeps its offer's: %v / %v",
			byJob(t, resp, key),
			byJob(t, resp, job),
		)
	}

	if code, _ := post(srv, techA(), "jobs.get", `{"jobId":"`+key+`"}`); code != 404 {
		t.Error("another technician")
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active' AND
			job_id <> $2 AND scheduled && tstzrange($3, $4)`,
		seed.ID("tech-internal-a"),
		key,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.assignments SET scheduled = tstzrange($2, $3), created_at = $2 WHERE job_id = $1 AND status =
			'active'`,
		key,
		clock().Add(-3*time.Hour),
		clock().Add(-time.Hour),
	)

	if code, _ := post(srv, techInt(), "jobs.get", `{"jobId":"`+key+`"}`); code != 404 {
		t.Error("ended viewing window before freeze")
	}

	freeze(t, clock())

	if code, resp := post(
		srv,
		techInt(),
		"jobs.get",
		`{"jobId":"`+key+`"}`,
	); code != 200 || data(resp)["projection"] != projectionHistory ||
		len(asArr(t, data(resp)["ownDecisionEvents"])) != 0 {
		t.Fatalf("technician history: %d %v", code, resp)
	}

	if listJob(t, srv, techInt(), key) == nil {
		t.Error("technician history listed")
	}
}
