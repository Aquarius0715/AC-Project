package app

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// powerSeries stores one power sample per minute from start for the given kW values (nil skips the minute).
func powerSeries(t *testing.T, unit string, start time.Time, origin string, kw ...*float64) {
	t.Helper()
	sensor := uuid.NewString()
	for i, v := range kw {
		if v == nil {
			continue
		}
		at := start.Add(time.Duration(i) * time.Minute)
		owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, boundary_id, received_at, event_id)
			VALUES ($1,$2,$3,'power',$4,1,$5,'kW',$6,'valid','ac_input_electricity',$4,$7)`, seed.ID("tenant-a"), unit, sensor, at, *v, origin, uuid.NewString())
	}
}

func kw(v float64) *float64 { return &v }

func TestFactors(t *testing.T) {
	s := server(t)
	if _, m := post(s, &hq, "factors.list", `{"filters":{"year":2026}}`); len(items(m)) == 0 || items(m)[0]["isDemo"] != true {
		t.Fatalf("seed factor: %v", m)
	}
	region := "Test region " + uuid.NewString()[:6]
	body := func(extra string) string {
		return `{"region":"` + region + `","year":2030,"kgCO2ePerKWh":0.42,"source":"demo test factor","isDemo":true` + extra + `}`
	}
	for _, b := range []string{strings.Replace(body(""), `"year":2030`, `"year":1999`, 1), strings.Replace(body(""), `0.42`, `0`, 1),
		strings.Replace(body(""), `"isDemo":true`, `"isDemo":false`, 1), strings.Replace(body(""), `"demo test factor"`, `" "`, 1)} {
		if code, _ := write(s, &hq, "factors.save", b, 0); code != 422 {
			t.Errorf("%s accepted", b)
		}
	}
	code, m := write(s, &hq, "factors.save", body(""), 0)
	if code != 200 || ver(m) != 1 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, _ := write(s, &hq, "factors.save", body(""), 0); code != 422 {
		t.Error("second current factor for the region and year")
	}
	code, m = write(s, &hq, "factors.save", strings.Replace(body(`,"id":"`+id+`"`), "0.42", "0.40", 1), 1)
	if code != 200 || ver(m) != 2 || data(m)["kgCO2ePerKWh"].(float64) != 0.40 {
		t.Fatalf("new version: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "factors.save", body(`,"id":"`+id+`"`), 1); code != 409 {
		t.Error("stale factor version")
	}
	if code, _ := write(s, &customerA, "factors.save", body(""), 0); code != 403 {
		t.Error("client saves factor")
	}
	if code, _ := post(s, &customerA, "factors.list", `{}`); code != 403 {
		t.Error("client lists factors")
	}
	if code, _ := post(s, &hq, "factors.list", `{"filters":{"x":1}}`); code != 422 {
		t.Error("bad factor filter")
	}
}

func TestBaselines(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Energy AC")
	start := clock.Add(-3 * time.Hour).Truncate(time.Minute)
	powerSeries(t, u, start, "measured", kw(60), kw(60), nil, kw(120))
	powerSeries(t, u, start.Add(4*time.Minute), "estimated", kw(30))
	period := `{"from":"` + start.Format(time.RFC3339) + `","to":"` + start.Add(5*time.Minute).Format(time.RFC3339) + `"}`
	body := func(method, extra string) string {
		return `{"unitIds":["` + u + `"],"period":` + period + `,"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test","method":"` + method + `"` + extra + `}`
	}
	for name, b := range map[string]string{
		"fixed without value":    body("demo_fixed", ""),
		"negative fixed":         body("demo_fixed", `,"baselineKWh":-1`),
		"comparison with value":  body("demo_period_comparison", `,"baselineKWh":5`),
		"comparison on building": strings.Replace(body("demo_period_comparison", ""), "ac_input_electricity", "whole_building_electricity", 1),
		"unaligned period":       strings.Replace(body("demo_fixed", `,"baselineKWh":1`), start.Format(time.RFC3339), start.Add(30*time.Second).Format(time.RFC3339), 1),
		"no units":               strings.Replace(body("demo_fixed", `,"baselineKWh":1`), `["`+u+`"]`, `[]`, 1),
		"unknown unit":           strings.Replace(body("demo_fixed", `,"baselineKWh":1`), u, uuid.NewString(), 1),
		"bad method":             body("guess", ""),
	} {
		if code, m := write(s, &hq, "baselines.save", b, 0); code != 422 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	code, m := write(s, &hq, "baselines.save", body("demo_period_comparison", ""), 0)
	q, _ := data(m)["quality"].(map[string]any)
	if code != 200 || q["kind"] != "measured" || q["expectedSlots"].(float64) != 5 || q["validSlots"].(float64) != 3 || data(m)["baselineKWh"].(float64) != 4 {
		t.Fatalf("measured baseline: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	code, m = write(s, &hq, "baselines.save", body("demo_fixed", `,"id":"`+id+`","baselineKWh":7.5`), 1)
	if code != 200 || ver(m) != 2 || data(m)["quality"].(map[string]any)["kind"] != "modeled" || data(m)["baselineKWh"].(float64) != 7.5 {
		t.Fatalf("modeled version: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "baselines.save", body("demo_fixed", `,"id":"`+id+`","baselineKWh":1`), 1); code != 409 {
		t.Error("stale baseline")
	}
	// empty period: zero valid slots gives baselineKWh=null
	empty := strings.Replace(body("demo_period_comparison", ""), start.Format(time.RFC3339), start.Add(-10*time.Minute).Format(time.RFC3339), 1)
	empty = strings.Replace(empty, start.Add(5*time.Minute).Format(time.RFC3339), start.Add(-5*time.Minute).Format(time.RFC3339), 1)
	if code, m := write(s, &hq, "baselines.save", empty, 0); code != 200 || data(m)["baselineKWh"] != nil || data(m)["quality"].(map[string]any)["coverage"].(float64) != 0 {
		t.Errorf("empty measured: %d %v", code, m)
	}
	// lists: HQ all; the customer only when every unit is theirs
	if _, m := post(s, &hq, "baselines.list", `{"filters":{"unitId":"`+u+`","method":"demo_fixed"}}`); len(items(m)) != 1 {
		t.Errorf("HQ list: %v", m)
	}
	if _, m := post(s, &customerB, "baselines.list", `{"filters":{"unitId":"`+u+`"},"limit":100}`); len(items(m)) != 2 {
		t.Errorf("client list: %v", m)
	}
	if _, m := post(s, &customerA, "baselines.list", `{"limit":100}`); len(items(m)) != 0 {
		t.Error("seed baseline spans two customers: not visible to customer-a")
	}
	if code, _ := post(s, &hq, "baselines.list", `{"filters":{"method":"magic"}}`); code != 422 {
		t.Error("bad baseline filter")
	}
	if code, _ := write(s, &customerB, "baselines.save", body("demo_fixed", `,"baselineKWh":1`), 0); code != 403 {
		t.Error("client saves baseline")
	}
}

// isolatedUnit creates a customer-b unit in its own new property (so property filters select only it).
func isolatedUnit(t *testing.T, s *Server) (string, string) {
	t.Helper()
	prop := uuid.NewString()
	owner(t, `INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, archived) VALUES ($1,$2,$3,'home',$4,NULL,false)`,
		prop, seed.ID("tenant-a"), seed.ID("org-customer-b"), "Energy site "+prop[:6])
	code, m := write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+prop+`","spaceId":null,"displayName":"Iso AC `+prop[:6]+
		`","modelId":"`+seed.ID("ventilation-demo").String()+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, m)
	}
	return data(m)["id"].(string), prop
}

func TestEnergySummary(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Summary AC")
	start := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	powerSeries(t, u, start, "measured", kw(60), kw(60), kw(60), kw(60), kw(60))
	fixed := func(v string, minutes int) string {
		_, m := write(s, &hq, "baselines.save", `{"unitIds":["`+u+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"`+time.Date(2026, 8, 1, 0, minutes, 0, 0, time.UTC).Format(time.RFC3339)+
			`"},"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test","method":"demo_fixed","baselineKWh":`+v+`}`, 0)
		return data(m)["id"].(string)
	}
	body := func(to time.Time, extra string) string {
		return `{"from":"` + start.Format(time.RFC3339) + `","to":"` + to.Format(time.RFC3339) + `","unitIds":["` + u + `"]` + extra + `}`
	}
	end := start.Add(5 * time.Minute)
	code, m := post(s, &customerB, "energy.summary", body(end, `,"baselineId":"`+fixed("10", 5)+`","tariffVersion":"tariff-demo-1"`))
	tot, _ := data(m)["totals"].(map[string]any)
	if code != 200 || tot["kWh"].(float64) != 5 || tot["amountMinor"].(float64) != 250 || tot["savedKWh"].(float64) != 5 || tot["savingPercentage"].(float64) != 50 ||
		tot["savedAmountMinor"].(float64) != 250 || tot["deltaKWh"].(float64) != -5 || data(m)["coverage"].(float64) != 1 || tot["emissionsKg"].(float64) != 2.5 {
		t.Fatalf("comparable: %d %v", code, m)
	}
	if w := data(m)["qualityWarnings"].([]any); len(w) != 1 || w[0] != "modeled_baseline" {
		t.Errorf("warnings: %v", w)
	}
	// an increase is signed negative (IR68)
	_, m = post(s, &hq, "energy.summary", body(end, `,"baselineId":"`+fixed("4", 5)+`"`))
	if tot := data(m)["totals"].(map[string]any); tot["savedKWh"].(float64) != -1 || tot["savingPercentage"].(float64) != -25 {
		t.Errorf("increase: %v", tot)
	}
	// partial coverage or another minute count: actuals only
	_, m = post(s, &hq, "energy.summary", body(start.Add(6*time.Minute), `,"baselineId":"`+fixed("10", 6)+`"`))
	if tot := data(m)["totals"].(map[string]any); tot["kWh"].(float64) != 5 || tot["savedKWh"] != nil || data(m)["coverage"].(float64) >= 1 {
		t.Errorf("partial: %v", m)
	}
	_, m = post(s, &hq, "energy.summary", body(end, `,"baselineId":"`+fixed("10", 7)+`"`))
	if data(m)["totals"].(map[string]any)["savedKWh"] != nil {
		t.Error("minute count mismatch must not compare")
	}
	// nothing measured: null actuals, coverage 0; no units: coverage null
	_, m = post(s, &hq, "energy.summary", `{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z","unitIds":["`+u+`"]}`)
	if tot := data(m)["totals"].(map[string]any); tot["kWh"] != nil || tot["amountMinor"] != nil || data(m)["coverage"].(float64) != 0 {
		t.Errorf("no data: %v", m)
	}
	if _, m := post(s, &hq, "energy.summary", `{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z","unitIds":[]}`); data(m)["coverage"] != nil {
		t.Errorf("no units: %v", m)
	}
	for _, b := range []string{body(end, `,"tariffVersion":"tariff-x"`), body(start.Add(90*time.Second), ""), body(start, ""), `{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z"}`} {
		if code, _ := post(s, &hq, "energy.summary", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	if code, _ := post(s, &customerA, "energy.summary", body(end, "")); code != 404 {
		t.Error("other customer's units")
	}
	if code, _ := post(s, &hq, "energy.summary", body(end, `,"baselineId":"`+uuid.NewString()+`"`)); code != 404 {
		t.Error("unknown baseline")
	}
}

func TestAdminSummary(t *testing.T) {
	s := server(t)
	u, prop := isolatedUnit(t, s)
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	powerSeries(t, u, start, "measured", kw(60), kw(60), kw(60), kw(60), kw(60))
	q := func(a *actor, extra string) (int, map[string]any) {
		return post(s, a, "admin.summary", `{"from":"`+start.Format(time.RFC3339)+`","to":"`+start.Add(5*time.Minute).Format(time.RFC3339)+`"`+extra+`}`)
	}
	code, m := q(&hq, `,"propertyId":"`+prop+`"`)
	fc, _ := data(m)["energyForecast"].(map[string]any)
	if code != 200 || data(m)["total"].(float64) != 1 || data(m)["customerCount"].(float64) != 1 || len(data(m)["jobCounts"].(map[string]any)) != 10 ||
		data(m)["billingVisibility"] != "allowed" || fc["predictedBaselineKWh"] != nil {
		t.Fatalf("admin summary: %d %v", code, m)
	}
	if w := fc["qualityWarnings"].([]any); len(w) != 1 || w[0] != "baseline_unavailable" {
		t.Errorf("forecast warnings: %v", w)
	}
	es := data(m)["energySummary"].(map[string]any)
	if es["totals"].(map[string]any)["kWh"].(float64) != 5 || es["baselineRef"] != nil || es["totals"].(map[string]any)["savedKWh"] != nil {
		t.Errorf("energySummary actuals only: %v", es)
	}
	// a fixed baseline for exactly U is prorated (IR78): 20 kWh over 10 unit-minutes → 10 kWh expected for 5
	write(s, &hq, "baselines.save", `{"unitIds":["`+u+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:10:00Z"},"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test","method":"demo_fixed","baselineKWh":20}`, 0)
	_, m = q(&hq, `,"propertyId":"`+prop+`"`)
	fc = data(m)["energyForecast"].(map[string]any)
	if fc["predictedBaselineKWh"].(float64) != 10 || fc["predictedActualKWh"].(float64) != 5 || fc["forecastSavedKWh"].(float64) != 5 || fc["forecastSavingPercentage"].(float64) != 50 ||
		fc["expectedUnitMinutes"].(float64) != 5 || fc["validUnitMinutes"].(float64) != 5 {
		t.Fatalf("forecast: %v", fc)
	}
	if w := fc["qualityWarnings"].([]any); len(w) != 2 || w[0] != "modeled_baseline" || w[1] != "prorated_forecast" {
		t.Errorf("forecast warnings: %v", w)
	}
	if _, m := q(&hq, `,"propertyId":"`+uuid.NewString()+`"`); data(m)["energyForecast"].(map[string]any)["qualityWarnings"].([]any)[0] != "no_units" || data(m)["operatingRate"] != nil {
		t.Errorf("no units: %v", m)
	}
	if _, m := q(&hq, `,"customerId":"`+seed.ID("cust-b").String()+`"`); data(m)["customerCount"].(float64) != 1 || data(m)["overdueInvoiceCount"] == nil {
		t.Errorf("customer filter: %v", m)
	}
	if code, _ := post(s, &hq, "admin.summary", `{"from":"2026-09-13T12:00:30Z","to":"2026-09-13T12:05:00Z"}`); code != 422 {
		t.Error("unaligned period")
	}
	for _, a := range []*actor{&customerA, &overrider} {
		if code, _ := q(a, ""); code != 403 {
			t.Errorf("%s admin.summary: %d", a.membership, code)
		}
	}
}
