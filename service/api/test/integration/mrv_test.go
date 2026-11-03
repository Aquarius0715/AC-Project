package integration_test

import (
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMRV(t *testing.T) {
	srv := server(t)
	unit, _ := isolatedUnit(t, srv)
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	powerSeries(
		t,
		unit,
		start,
		"measured",
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
	)
	_, resp := write(
		srv,
		hq(),
		"baselines.save",
		`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:05:00Z"},`+
			`"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test",`+
			`"method":"demo_fixed","baselineKWh":10}`,
		0,
	)
	base := asStr(t, data(resp)["id"])
	org := seed.ID("org-customer-b").String()
	cond := func(extra map[string]string) string {
		bodyFields := map[string]string{
			fieldFrom: `"` + start.Format(
				time.RFC3339,
			) + `"`,
			"to":                `"` + start.Add(5*time.Minute).Format(time.RFC3339) + `"`,
			fieldUnitIDs:        `["` + unit + `"]`,
			"baselineId":        `"` + base + `"`,
			"baselineVersion":   "1",
			"factorId":          `"` + seed.ID("factor-demo-2026").String() + `"`,
			"factorVersion":     "1",
			fieldBoundaryID:     `"ac_input_electricity"`,
			fieldBoundary:       `"AC input electricity"`,
			fieldOrganizationID: `"` + org + `"`,
		}
		maps.Copy(bodyFields, extra)

		parts := make([]string, 0, len(bodyFields))
		for k, v := range bodyFields {
			parts = append(parts, `"`+k+`":`+v)
		}

		return "{" + strings.Join(parts, ",") + "}"
	}

	code, resp := post(srv, hq(), "mrv.preview", cond(nil))
	if code != 200 || data(resp)["incomplete"] != false || data(resp)["scope"] != "scope_2" ||
		asNum(t, asObj(t, asObj(t, data(resp)["summary"])["totals"])["savedKWh"]) != 5 ||
		asNum(t, asObj(t, asObj(t, data(resp)["summary"])["totals"])["emissionsKg"]) != 2.5 {
		t.Fatalf("preview: %d %v", code, resp)
	}

	if _, m := post(
		srv,
		hq(),
		"mrv.preview",
		cond(map[string]string{fieldBoundaryID: `"whole_building_electricity"`}),
	); data(m)["incomplete"] != true {
		t.Error("boundary mismatch is incomplete")
	}

	refused := map[string]string{ // the field each VALIDATION case is refused for
		"blank boundary":      "boundary=error.length",
		"non-customer org":    "organizationId=error.invalid",
		"unaligned":           refusedEnergyRange,
		"unit of another org": "unitIds=errors.units_outside_organization",
	}

	for name, testCase := range map[string]struct {
		e    map[string]string
		code int
	}{
		"unit of another org":  {map[string]string{fieldUnitIDs: `["` + seed.ID("unit-online-rto").String() + `"]`}, 422},
		"unknown baseline ver": {map[string]string{"baselineVersion": "9"}, 404},
		"unknown factor":       {map[string]string{"factorId": `"` + uuid.NewString() + `"`}, 404},
		"non-customer org":     {map[string]string{fieldOrganizationID: `"` + seed.ID("org-operator-a").String() + `"`}, 422},
		"blank boundary":       {map[string]string{fieldBoundary: jsonBlank}, 422},
		"unaligned":            {map[string]string{fieldFrom: `"` + start.Add(time.Second).Format(time.RFC3339) + `"`}, 422},
	} {
		code, m := post(srv, hq(), "mrv.preview", cond(testCase.e))
		if w, ok := refused[name]; ok {
			refusal(t, name, code, m, w)
		} else if code != testCase.code {
			t.Errorf("%s: %d want %d", name, code, testCase.code)
		}
	}
	// drafts, review, versions
	if code, res := write(
		srv,
		hq(),
		"mrv.saveDraft",
		`{"conditions":`+cond(nil)+`,"evidenceIds":["`+uuid.NewString()+`"]}`,
		0,
	); code != 422 ||
		fields(res) != "evidenceIds=errors.evidence_unknown" {
		t.Errorf("unknown evidence: %d %s", code, fields(res))
	}

	code, resp = write(
		srv,
		hq(),
		"mrv.saveDraft",
		`{"conditions":`+cond(nil)+`,"evidenceIds":[]}`,
		0,
	)
	if code != 200 || data(resp)["status"] != statusDraft || ver(t, resp) != 1 ||
		data(resp)["isDemo"] != true {
		t.Fatalf("draft: %d %v", code, resp)
	}

	reportID := asStr(t, data(resp)["id"])

	review := func(reportVersion, version int) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"mrv.recordReview",
			`{"reportId":"`+reportID+`","reportVersion":`+itoa(
				reportVersion,
			)+`,"reviewComment":"Checked demo inputs"}`,
			version,
		)
	}
	if code, _ := review(2, 1); code != 409 {
		t.Error("review of a non-latest version")
	}

	code, resp = review(1, 1)
	if code != 200 || data(resp)["status"] != "demo_reviewed" || ver(t, resp) != 2 ||
		len(asArr(t, data(resp)["reviewHistory"])) != 1 ||
		asNum(t, asObj(t, asArr(t, data(resp)["reviewHistory"])[0])["reportVersion"]) != 1 {
		t.Fatalf("review: %d %v", code, resp)
	}

	if code, _ := review(2, 2); code != 409 {
		t.Error("review twice")
	}

	if code, _ := write(
		srv,
		hq(),
		"mrv.saveDraft",
		`{"id":"`+reportID+`","conditions":`+cond(nil)+`,"evidenceIds":[]}`,
		1,
	); code != 409 {
		t.Error("stale draft")
	}

	if code, res := write(
		srv,
		hq(),
		"mrv.saveDraft",
		`{"id":"`+reportID+`","conditions":`+cond(
			map[string]string{fieldBoundaryID: `"whole_building_electricity"`},
		)+`,"evidenceIds":[]}`,
		2,
	); code != 200 || ver(t, res) != 3 ||
		data(res)["incomplete"] != true {
		t.Fatalf("new draft version: %d %v", code, res)
	}

	if code, _ := review(3, 3); code != 409 {
		t.Error("review of an incomplete version")
	}

	if _, res := post(
		srv,
		hq(),
		"mrv.versions",
		`{"id":"`+reportID+`","query":{}}`,
	); len(items(t, res)) != 3 || asNum(t, items(t, res)[0]["version"]) != 1 ||
		items(t, res)[1]["status"] != "demo_reviewed" {
		t.Errorf("versions: %v", res)
	}

	if _, res := post(
		srv,
		hq(),
		"mrv.get",
		`{"id":"`+reportID+`","reportVersion":1}`,
	); data(res)["status"] != statusDraft ||
		len(asArr(t, data(res)["reviewHistory"])) != 0 {
		t.Errorf("old version: %v", res)
	}

	if _, m := post(srv, hq(), "mrv.get", `{"id":"`+reportID+`"}`); ver(t, m) != 3 {
		t.Error("latest by default")
	}

	if code, _ := post(srv, hq(), "mrv.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown report")
	}

	if _, res := post(
		srv,
		hq(),
		"mrv.list",
		`{"filters":{"organizationId":"`+org+`","unitId":"`+unit+`","status":"draft"}}`,
	); len(items(t, res)) != 1 ||
		ver(t, map[string]any{"data": items(t, res)[0]}) != 3 {
		t.Errorf("list: %v", res)
	}

	if code, res := post(
		srv,
		hq(),
		"mrv.list",
		`{"filters":{"status":"reviewed"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad status filter: %d %s", code, fields(res))
	}

	if code, _ := post(srv, customerB(), "mrv.list", `{}`); code != 403 {
		t.Error("client MRV")
	}
}
