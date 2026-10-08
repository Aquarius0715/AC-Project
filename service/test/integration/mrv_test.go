package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func TestMRV(t *testing.T) {
	s := server(t)
	u, _ := isolatedUnit(t, s)
	start := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	powerSeries(t, u, start, "measured", kw(60), kw(60), kw(60), kw(60), kw(60))
	_, m := write(s, &hq, "baselines.save", `{"unitIds":["`+u+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:05:00Z"},"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test","method":"demo_fixed","baselineKWh":10}`, 0)
	base := data(m)["id"].(string)
	org := seed.ID("org-customer-b").String()
	cond := func(extra map[string]string) string {
		f := map[string]string{"from": `"` + start.Format(time.RFC3339) + `"`, "to": `"` + start.Add(5*time.Minute).Format(time.RFC3339) + `"`, "unitIds": `["` + u + `"]`,
			"baselineId": `"` + base + `"`, "baselineVersion": "1", "factorId": `"` + seed.ID("factor-demo-2026").String() + `"`, "factorVersion": "1",
			"boundaryId": `"ac_input_electricity"`, "boundary": `"AC input electricity"`, "organizationId": `"` + org + `"`}
		for k, v := range extra {
			f[k] = v
		}
		parts := []string{}
		for k, v := range f {
			parts = append(parts, `"`+k+`":`+v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	}
	code, m := post(s, &hq, "mrv.preview", cond(nil))
	if code != 200 || data(m)["incomplete"] != false || data(m)["scope"] != "scope_2" || data(m)["summary"].(map[string]any)["totals"].(map[string]any)["savedKWh"].(float64) != 5 ||
		data(m)["summary"].(map[string]any)["totals"].(map[string]any)["emissionsKg"].(float64) != 2.5 {
		t.Fatalf("preview: %d %v", code, m)
	}
	if _, m := post(s, &hq, "mrv.preview", cond(map[string]string{"boundaryId": `"whole_building_electricity"`})); data(m)["incomplete"] != true {
		t.Error("boundary mismatch is incomplete")
	}
	for name, tc := range map[string]struct {
		e    map[string]string
		code int
	}{
		"unit of another org":  {map[string]string{"unitIds": `["` + seed.ID("unit-online-rto").String() + `"]`}, 422},
		"unknown baseline ver": {map[string]string{"baselineVersion": "9"}, 404},
		"unknown factor":       {map[string]string{"factorId": `"` + uuid.NewString() + `"`}, 404},
		"non-customer org":     {map[string]string{"organizationId": `"` + seed.ID("org-operator-a").String() + `"`}, 422},
		"blank boundary":       {map[string]string{"boundary": `" "`}, 422},
		"unaligned":            {map[string]string{"from": `"` + start.Add(time.Second).Format(time.RFC3339) + `"`}, 422},
	} {
		if code, _ := post(s, &hq, "mrv.preview", cond(tc.e)); code != tc.code {
			t.Errorf("%s: %d want %d", name, code, tc.code)
		}
	}
	// drafts, review, versions
	if code, _ := write(s, &hq, "mrv.saveDraft", `{"conditions":`+cond(nil)+`,"evidenceIds":["`+uuid.NewString()+`"]}`, 0); code != 422 {
		t.Error("unknown evidence")
	}
	code, m = write(s, &hq, "mrv.saveDraft", `{"conditions":`+cond(nil)+`,"evidenceIds":[]}`, 0)
	if code != 200 || data(m)["status"] != "draft" || ver(m) != 1 || data(m)["isDemo"] != true {
		t.Fatalf("draft: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	review := func(rv, v int) (int, map[string]any) {
		return write(s, &hq, "mrv.recordReview", `{"reportId":"`+id+`","reportVersion":`+itoa(rv)+`,"reviewComment":"Checked demo inputs"}`, v)
	}
	if code, _ := review(2, 1); code != 409 {
		t.Error("review of a non-latest version")
	}
	code, m = review(1, 1)
	if code != 200 || data(m)["status"] != "demo_reviewed" || ver(m) != 2 || len(data(m)["reviewHistory"].([]any)) != 1 ||
		data(m)["reviewHistory"].([]any)[0].(map[string]any)["reportVersion"].(float64) != 1 {
		t.Fatalf("review: %d %v", code, m)
	}
	if code, _ := review(2, 2); code != 409 {
		t.Error("review twice")
	}
	if code, _ := write(s, &hq, "mrv.saveDraft", `{"id":"`+id+`","conditions":`+cond(nil)+`,"evidenceIds":[]}`, 1); code != 409 {
		t.Error("stale draft")
	}
	if code, m := write(s, &hq, "mrv.saveDraft", `{"id":"`+id+`","conditions":`+cond(map[string]string{"boundaryId": `"whole_building_electricity"`})+`,"evidenceIds":[]}`, 2); code != 200 || ver(m) != 3 || data(m)["incomplete"] != true {
		t.Fatalf("new draft version: %d %v", code, m)
	}
	if code, _ := review(3, 3); code != 409 {
		t.Error("review of an incomplete version")
	}
	if _, m := post(s, &hq, "mrv.versions", `{"id":"`+id+`","query":{}}`); len(items(m)) != 3 || items(m)[0]["version"].(float64) != 1 || items(m)[1]["status"] != "demo_reviewed" {
		t.Errorf("versions: %v", m)
	}
	if _, m := post(s, &hq, "mrv.get", `{"id":"`+id+`","reportVersion":1}`); data(m)["status"] != "draft" || len(data(m)["reviewHistory"].([]any)) != 0 {
		t.Errorf("old version: %v", m)
	}
	if _, m := post(s, &hq, "mrv.get", `{"id":"`+id+`"}`); ver(m) != 3 {
		t.Error("latest by default")
	}
	if code, _ := post(s, &hq, "mrv.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown report")
	}
	if _, m := post(s, &hq, "mrv.list", `{"filters":{"organizationId":"`+org+`","unitId":"`+u+`","status":"draft"}}`); len(items(m)) != 1 || ver(map[string]any{"data": items(m)[0]}) != 3 {
		t.Errorf("list: %v", m)
	}
	if code, _ := post(s, &hq, "mrv.list", `{"filters":{"status":"reviewed"}}`); code != 422 {
		t.Error("bad status filter")
	}
	if code, _ := post(s, &customerB, "mrv.list", `{}`); code != 403 {
		t.Error("client MRV")
	}
}
