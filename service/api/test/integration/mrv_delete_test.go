package integration_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// A baseline, an emission factor and an MRV draft that nothing cites or reviewed are deleted; one a report cites, the
// factor the energy summary uses, and a report that was reviewed stay (DEC-103). HQ only.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMRVDeletes(t *testing.T) {
	srv := server(t)
	unit, _ := isolatedUnit(t, srv)
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	baseline := func() string {
		code, resp := write(
			srv,
			hq(),
			"baselines.save",
			`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:05:00Z"},`+
				`"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test",`+
				`"method":"demo_fixed","baselineKWh":10}`,
			0,
		)
		if code != 200 {
			t.Fatalf("baseline: %d %v", code, resp)
		}

		return asStr(t, data(resp)["id"])
	}
	factor := func() string {
		code, resp := write(
			srv,
			hq(),
			"factors.save",
			`{"region":"Del region `+uuid.NewString()[:6]+`","year":2031,"kgCO2ePerKWh":0.41,"source":"demo test factor",`+
				`"isDemo":true}`,
			0,
		)
		if code != 200 {
			t.Fatalf("factor: %d %v", code, resp)
		}

		return asStr(t, data(resp)["id"])
	}
	draft := func(baselineID, factorID string) (string, int) {
		conditions := `{"from":"` + start.Format(
			time.RFC3339,
		) + `","to":"` + start.Add(5*time.Minute).
			Format(time.RFC3339) +
			`","unitIds":["` + unit + `"],"baselineId":"` + baselineID + `","baselineVersion":1,"factorId":"` + factorID +
			`","factorVersion":1,"boundaryId":"ac_input_electricity","boundary":"AC input electricity",` +
			`"organizationId":"` + seed.ID("org-customer-b").
			String() +
			`"}`

		code, resp := write(
			srv,
			hq(),
			"mrv.saveDraft",
			`{"conditions":`+conditions+`,"evidenceIds":[]}`,
			0,
		)
		if code != 200 {
			t.Fatalf("draft: %d %v", code, resp)
		}

		return asStr(t, data(resp)["id"]), ver(t, resp)
	}
	sharedFactor := seed.ID("factor-demo-2026").String()
	remove := func(a *actor, operation, id string, version int) (int, map[string]any) {
		return write(srv, a, operation, `{"id":"`+id+`","reason":"made by mistake"}`, version)
	}

	// baselines
	cited, free := baseline(), baseline()
	report, reportVersion := draft(cited, sharedFactor)

	if code, res := remove(
		hq(),
		"baselines.delete",
		cited,
		1,
	); code != 409 ||
		res["messageKey"] != "error.baselineCited" {
		t.Errorf("baseline a report cites: %d %v", code, res["messageKey"])
	}

	if code, _ := remove(customerA(), "baselines.delete", free, 1); code == 200 {
		t.Error("a client deletes a baseline")
	}

	if code, _ := remove(hq(), "baselines.delete", free, 4); code != 409 {
		t.Error("baseline delete at a stale version")
	}

	if code, res := remove(
		hq(),
		"baselines.delete",
		free,
		1,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("baseline delete: %d %v", code, res)
	}

	if code, _ := remove(hq(), "baselines.delete", free, 1); code != 404 {
		t.Error("a baseline is deleted once")
	}

	// factors
	usedFactor, freeFactor := factor(), factor()
	_, _ = draft(cited, usedFactor) // cites the factor

	if code, res := remove(
		hq(),
		"factors.delete",
		usedFactor,
		1,
	); code != 409 ||
		res["messageKey"] != "error.factorCited" {
		t.Errorf("factor a report cites: %d %v", code, res["messageKey"])
	}

	if code, res := remove(
		hq(),
		"factors.delete",
		sharedFactor,
		1,
	); code != 409 ||
		res["messageKey"] != "error.factorDefault" {
		t.Errorf("factor the summary uses: %d %v", code, res["messageKey"])
	}

	if code, _ := remove(customerA(), "factors.delete", freeFactor, 1); code == 200 {
		t.Error("a client deletes a factor")
	}

	if code, res := remove(
		hq(),
		"factors.delete",
		freeFactor,
		1,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("factor delete: %d %v", code, res)
	}

	if code, _ := remove(hq(), "factors.delete", freeFactor, 1); code != 404 {
		t.Error("a factor is deleted once")
	}

	// drafts
	extra, extraVersion := draft(cited, sharedFactor)

	if code, _ := remove(customerA(), "mrv.deleteDraft", extra, extraVersion); code == 200 {
		t.Error("a client deletes a draft")
	}

	if code, _ := remove(hq(), "mrv.deleteDraft", extra, extraVersion+3); code != 409 {
		t.Error("draft delete at a stale version")
	}

	if code, res := remove(
		hq(),
		"mrv.deleteDraft",
		extra,
		extraVersion,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("draft delete: %d %v", code, res)
	}

	if code, _ := post(srv, hq(), "mrv.get", `{"id":"`+extra+`"}`); code != 404 {
		t.Error("a deleted draft is gone")
	}

	// a reviewed report stays (marked so directly: a review needs measured data, which this test does not make)
	owner(t, `UPDATE energy.mrv_reports SET status = 'demo_reviewed' WHERE id = $1`, report)

	if code, res := remove(hq(), "mrv.deleteDraft", report, reportVersion); code != 409 ||
		res["messageKey"] != "error.reportReviewed" {
		t.Errorf("reviewed report: %d %v", code, res["messageKey"])
	}
}
