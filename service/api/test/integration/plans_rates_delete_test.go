package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A maintenance plan nothing has come from is deleted; one that has made a job stays with its history (DEC-103).
// HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPlansDelete(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Plan Del AC")
	next := clock().Add(10 * 24 * time.Hour)
	save := func() (string, int) {
		code, resp := write(
			srv,
			hq(),
			"plans.save",
			`{"unitId":"`+unit+`","recurrence":{"kind":"monthly","intervalMonths":3},"nextDueAt":"`+
				next.Format(time.RFC3339)+`"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("plan: %d %v", code, resp)
		}

		return asStr(t, data(resp)["id"]), ver(t, resp)
	}
	drop := func(a *actor, id string, version int) (int, map[string]any) {
		return write(srv, a, "plans.delete", `{"id":"`+id+`","reason":"saved by mistake"}`, version)
	}

	plan, version := save()

	for name, who := range map[string]*actor{roleClient: customerA(), roleTechnician: techInt()} {
		if code, _ := drop(who, plan, version); code == 200 {
			t.Errorf("%s deletes a plan", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"plans.delete",
		`{"id":"`+plan+`","reason":""}`,
		version,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("plan delete needs a reason: %d %s", code, fields(res))
	}

	if code, _ := drop(hq(), plan, version+5); code != 409 {
		t.Error("plan delete at a stale version")
	}

	code, resp := drop(hq(), plan, version)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != plan {
		t.Fatalf("plan delete: %d %v", code, resp)
	}

	if code, _ := post(srv, hq(), "plans.get", `{"id":"`+plan+`"}`); code != 404 {
		t.Error("a deleted plan is gone")
	}

	if code, _ := drop(hq(), plan, version); code != 404 {
		t.Error("a plan is deleted once")
	}

	// a plan that has made a job stays
	made, madeVersion := save()
	if code, res := write(
		srv,
		hq(),
		"plans.generateNext",
		`{"id":"`+made+`","occurrenceDate":"`+next.Format(time.RFC3339)+`"}`,
		madeVersion,
	); code != 200 {
		t.Fatalf("generate: %d %v", code, res)
	}

	_, current := post(srv, hq(), "plans.get", `{"id":"`+made+`"}`)
	if code, res := drop(
		hq(),
		made,
		ver(t, current),
	); code != 409 ||
		res["messageKey"] != "error.planHasJobs" {
		t.Errorf("plan with a job: %d %v", code, res["messageKey"])
	}
}

// A rate card that has not started is deleted; one that is in effect or has been stays, because payouts are
// priced from it (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRateCardsDelete(t *testing.T) {
	srv := server(t)
	org := uuid.NewString()

	code, resp := write(
		srv,
		hq(),
		"organizations.save",
		`{"name":"Rate Del `+org[:8]+`","kind":"contractor","status":"active"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("organization: %d %v", code, resp)
	}

	org = asStr(t, data(resp)["id"])

	if code, res := write(
		srv,
		hq(),
		"contractors.save",
		`{"organizationId":"`+org+`","registrationNo":"REG-R","serviceAreas":["KL"],"contactEmail":"r@example.com",`+
			`"insuranceValidUntil":null}`,
		0,
	); code != 200 {
		t.Fatalf("contractor: %d %v", code, res)
	}

	line := `[{"workType":"repair_base","amountMinor":45000,"note":null}]`
	card := func(from time.Time) string {
		code, res := write(
			srv,
			hq(),
			"rateCards.save",
			`{"contractorOrgId":"`+org+`","effectiveFrom":"`+from.Format(
				time.RFC3339,
			)+`","currency":"MYR","lines":`+line+`}`,
			0,
		)
		if code != 200 {
			t.Fatalf("rate card: %d %v", code, res)
		}

		return asStr(t, data(res)["id"])
	}
	drop := func(a *actor, id string) (int, map[string]any) {
		return write(srv, a, "rateCards.delete", `{"id":"`+id+`","reason":"typed wrongly"}`, 0)
	}

	future := card(clock().Add(30 * 24 * time.Hour))

	for name, who := range map[string]*actor{roleClient: customerA(), "contractor": contrA(), roleTechnician: techInt()} {
		if code, _ := drop(who, future); code == 200 {
			t.Errorf("%s deletes a rate card", name)
		}
	}

	if code, res := write(
		srv,
		hq(),
		"rateCards.delete",
		`{"id":"`+future+`","reason":""}`,
		0,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("rate card delete needs a reason: %d %s", code, fields(res))
	}

	code, resp = drop(hq(), future)
	if code != 200 || data(resp)["deleted"] != true || data(resp)["id"] != future {
		t.Fatalf("rate card delete: %d %v", code, resp)
	}

	if code, _ := drop(hq(), future); code != 404 {
		t.Error("a rate card is deleted once")
	}

	_, list := post(
		srv,
		hq(),
		"rateCards.list",
		`{"filters":{"contractorOrgId":"`+org+`"},"limit":100}`,
	)
	if n := len(items(t, list)); n != 0 {
		t.Errorf("the deleted rate card is still listed: %d", n)
	}

	// a card that has started stays: the seeded contractor's card is in effect
	var started string

	ownerScan(
		t,
		`SELECT id::text FROM maintenance.rate_cards WHERE contractor_org_id = $1 AND effective_from <= $2 LIMIT 1`,
		[]any{seed.ID("org-contractor-a"), clock()},
		&started,
	)

	if code, res := drop(
		hq(),
		started,
	); code != 409 ||
		res["messageKey"] != "error.rateCardStarted" {
		t.Errorf("rate card in effect: %d %v", code, res["messageKey"])
	}
}
