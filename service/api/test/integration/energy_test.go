package integration_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/energy"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

// powerSeries stores one power sample per minute from start for the given kW values (nil skips the minute).
func powerSeries(t *testing.T, unit string, start time.Time, origin string, kw ...*float64) {
	t.Helper()

	sensor := uuid.NewString()

	for i, v := range kw {
		if v == nil {
			continue
		}

		when := start.Add(time.Duration(i) * time.Minute)
		owner(
			t,
			`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
				origin, quality, boundary_id, received_at, event_id)
			VALUES ($1,$2,$3,'power',$4,1,$5,'kW',$6,'valid','ac_input_electricity',$4,$7)`,
			seed.ID("tenant-a"),
			unit,
			sensor,
			when,
			*v,
			origin,
			uuid.NewString(),
		)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestFactors(t *testing.T) {
	srv := server(t)
	if _, resp := post(
		srv,
		hq(),
		"factors.list",
		`{"filters":{"year":2026}}`,
	); len(items(t, resp)) == 0 ||
		items(t, resp)[0]["isDemo"] != true {
		t.Fatalf("seed factor: %v", resp)
	}

	region := "Test region " + uuid.NewString()[:6]

	body := func(extra string) string {
		return `{"region":"` + region + `","year":2030,"kgCO2ePerKWh":0.42,"source":"demo test factor",` +
			`"isDemo":true` + extra + `}`
	}
	for b, want := range map[string]string{
		strings.Replace(body(""), `"year":2030`, `"year":1999`, 1):      "year=error.range",
		strings.Replace(body(""), `0.42`, `0`, 1):                       "kgCO2ePerKWh=error.range",
		strings.Replace(body(""), `"isDemo":true`, `"isDemo":false`, 1): "isDemo=error.invalid",
		strings.Replace(body(""), `"demo test factor"`, `" "`, 1):       "source=error.length",
	} {
		code, m := write(srv, hq(), "factors.save", b, 0)
		refusal(t, b, code, m, want)
	}

	code, res := write(srv, hq(), "factors.save", body(""), 0)
	if code != 200 || ver(t, res) != 1 {
		t.Fatalf("create: %d %v", code, res)
	}

	id := asStr(t, data(res)["id"])

	if code, resp := write(
		srv,
		hq(),
		"factors.save",
		body(""),
		0,
	); code != 422 ||
		fields(resp) != "region=errors.factor_exists" {
		t.Errorf("second current factor for the region and year: %d %s", code, fields(resp))
	}

	code, res = write(
		srv,
		hq(),
		"factors.save",
		strings.Replace(body(`,"id":"`+id+`"`), "0.42", "0.40", 1),
		1,
	)
	if code != 200 || ver(t, res) != 2 || asNum(t, data(res)["kgCO2ePerKWh"]) != 0.40 {
		t.Fatalf("new version: %d %v", code, res)
	}

	if code, _ := write(srv, hq(), "factors.save", body(`,"id":"`+id+`"`), 1); code != 409 {
		t.Error("stale factor version")
	}

	if code, _ := write(srv, customerA(), "factors.save", body(""), 0); code != 403 {
		t.Error("client saves factor")
	}

	if code, _ := post(srv, customerA(), "factors.list", `{}`); code != 403 {
		t.Error("client lists factors")
	}

	if code, resp := post(
		srv,
		hq(),
		"factors.list",
		`{"filters":{"x":1}}`,
	); code != 422 ||
		fields(resp) != refusedUnknownFieldX {
		t.Errorf("bad factor filter: %d %s", code, fields(resp))
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestBaselines(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Energy AC")
	start := clock().Add(-3 * time.Hour).Truncate(time.Minute)
	powerSeries(
		t,
		unit,
		start,
		"measured",
		new(float64(60)),
		new(float64(60)),
		nil,
		new(float64(120)),
	)
	powerSeries(t, unit, start.Add(4*time.Minute), "estimated", new(float64(30)))
	period := `{"from":"` + start.Format(
		time.RFC3339,
	) + `","to":"` + start.Add(5*time.Minute).
		Format(time.RFC3339) +
		`"}`
	body := func(method, extra string) string {
		return `{"unitIds":["` + unit + `"],"period":` + period + `,"boundaryId":"ac_input_electricity","boundary":"AC ` +
			`input",` +
			`"assumptions":"demo","source":"test","method":"` + method + `"` + extra + `}`
	}
	// the field each case is refused for
	want := map[string]string{
		caseBadMethod:            refusedMethodInvalid,
		"comparison on building": "boundaryId=errors.boundary_not_measurable",
		"comparison with value":  "baselineKWh=error.notAllowed",
		"fixed without value":    "baselineKWh=error.range",
		"negative fixed":         "baselineKWh=error.range",
		caseNoUnits:              refusedUnitIDsInvalid,
		"unaligned period":       "period=errors.energy_range",
		caseUnknownUnit:          "unitIds=errors.unit_unknown",
	}

	for name, b := range map[string]string{
		"fixed without value":   body("demo_fixed", ""),
		"negative fixed":        body("demo_fixed", `,"baselineKWh":-1`),
		"comparison with value": body("demo_period_comparison", `,"baselineKWh":5`),
		"comparison on building": strings.Replace(body("demo_period_comparison", ""), "ac_input_electricity",
			"whole_building_electricity", 1),
		"unaligned period": strings.Replace(body("demo_fixed", `,"baselineKWh":1`), start.Format(time.RFC3339),
			start.Add(30*time.Second).Format(time.RFC3339), 1),
		caseNoUnits:     strings.Replace(body("demo_fixed", `,"baselineKWh":1`), `["`+unit+`"]`, `[]`, 1),
		caseUnknownUnit: strings.Replace(body("demo_fixed", `,"baselineKWh":1`), unit, uuid.NewString(), 1),
		caseBadMethod:   body("guess", ""),
	} {
		code, m := write(srv, hq(), "baselines.save", b, 0)
		refusal(t, name, code, m, want[name])
	}

	code, resp := write(srv, hq(), "baselines.save", body("demo_period_comparison", ""), 0)

	q, _ := data(resp)["quality"].(map[string]any)
	if code != 200 || q["kind"] != "measured" || asNum(t, q["expectedSlots"]) != 5 ||
		asNum(t, q["validSlots"]) != 3 ||
		asNum(t, data(resp)["baselineKWh"]) != 4 {
		t.Fatalf("measured baseline: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	code, resp = write(
		srv,
		hq(),
		"baselines.save",
		body("demo_fixed", `,"id":"`+id+`","baselineKWh":7.5`),
		1,
	)
	if code != 200 || ver(t, resp) != 2 ||
		asObj(t, data(resp)["quality"])["kind"] != "modeled" ||
		asNum(t, data(resp)["baselineKWh"]) != 7.5 {
		t.Fatalf("modeled version: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		hq(),
		"baselines.save",
		body("demo_fixed", `,"id":"`+id+`","baselineKWh":1`),
		1,
	); code != 409 {
		t.Error("stale baseline")
	}
	// empty period: zero valid slots gives baselineKWh=null
	empty := strings.Replace(
		body("demo_period_comparison", ""),
		start.Format(time.RFC3339),
		start.Add(-10*time.Minute).Format(time.RFC3339),
		1,
	)

	empty = strings.Replace(
		empty,
		start.Add(5*time.Minute).Format(time.RFC3339),
		start.Add(-5*time.Minute).Format(time.RFC3339),
		1,
	)
	if code, res := write(
		srv,
		hq(),
		"baselines.save",
		empty,
		0,
	); code != 200 || data(res)["baselineKWh"] != nil ||
		asNum(t, asObj(t, data(res)["quality"])["coverage"]) != 0 {
		t.Errorf("empty measured: %d %v", code, res)
	}
	// lists: HQ all; the customer only when every unit is theirs
	if _, res := post(
		srv,
		hq(),
		"baselines.list",
		`{"filters":{"unitId":"`+unit+`","method":"demo_fixed"}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("HQ list: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"baselines.list",
		`{"filters":{"unitId":"`+unit+`"},"limit":100}`,
	); len(
		items(t, res),
	) != 2 {
		t.Errorf("client list: %v", res)
	}

	if code, res := post(
		srv,
		customerA(),
		"baselines.list",
		`{"limit":100}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Error("seed baseline spans two customers: not visible to customer-a")
	}

	if code, res := post(
		srv,
		hq(),
		"baselines.list",
		`{"filters":{"method":"magic"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad baseline filter: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		customerB(),
		"baselines.save",
		body("demo_fixed", `,"baselineKWh":1`),
		0,
	); code != 403 {
		t.Error("client saves baseline")
	}
}

// isolatedUnit creates a customer-b unit in its own new property (so property filters select only it).
func isolatedUnit(t *testing.T, srv *apiserver.Server) (string, string) {
	t.Helper()

	prop := uuid.NewString()
	owner(
		t,
		`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, archived) VALUES ($1,$2,$3,
			'home',$4,NULL,false)`,
		prop,
		seed.ID("tenant-a"),
		seed.ID("org-customer-b"),
		"Energy site "+prop[:6],
	)

	code, res := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","propertyId":"`+prop+`","spaceId":null,"displayName":"Iso AC `+prop[:6]+
			`","modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, res)
	}

	return asStr(t, data(res)["id"]), prop
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEnergySummary(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Summary AC")
	start := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
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

	fixed := func(v string, minutes int) string {
		_, res := write(
			srv,
			hq(),
			"baselines.save",
			`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z",`+
				`"to":"`+time.Date(2026, 8, 1, 0, minutes, 0, 0, time.UTC).
				Format(time.RFC3339)+
				`"},"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test",`+
				`"method":"demo_fixed","baselineKWh":`+v+`}`,
			0,
		)

		return asStr(t, data(res)["id"])
	}
	body := func(to time.Time, extra string) string {
		return `{"from":"` + start.Format(
			time.RFC3339,
		) + `","to":"` + to.Format(
			time.RFC3339,
		) + `","unitIds":["` + unit + `"]` + extra + `}`
	}
	end := start.Add(5 * time.Minute)
	code, resp := post(
		srv,
		customerB(),
		"energy.summary",
		body(end, `,"baselineId":"`+fixed("10", 5)+`","tariffVersion":"tariff-demo-1"`),
	)

	tot, _ := data(resp)["totals"].(map[string]any)
	if code != 200 || asNum(t, tot["kWh"]) != 5 || asNum(t, tot["amountMinor"]) != 250 ||
		asNum(t, tot["savedKWh"]) != 5 ||
		asNum(t, tot["savingPercentage"]) != 50 ||
		asNum(t, tot["savedAmountMinor"]) != 250 ||
		asNum(t, tot["deltaKWh"]) != -5 ||
		asNum(t, data(resp)["coverage"]) != 1 ||
		asNum(t, tot["emissionsKg"]) != 2.5 {
		t.Fatalf("comparable: %d %v", code, resp)
	}

	if w := asArr(t, data(resp)["qualityWarnings"]); len(w) != 1 || w[0] != "modeled_baseline" {
		t.Errorf("warnings: %v", w)
	}
	// without a current default emission factor the emissions are unknown, never zero (SR09): factor_missing
	owner(
		t,
		`UPDATE energy.emission_factors SET is_current = false WHERE id = $1 AND is_current`,
		energy.DefaultFactorID,
	)

	_, resp = post(srv, hq(), "energy.summary", body(end, ""))

	owner(
		t,
		`UPDATE energy.emission_factors SET is_current = true WHERE id = $1 AND version = (SELECT max(version) FROM
			energy.emission_factors WHERE id = $1)`,
		energy.DefaultFactorID,
	)

	if tot := asObj(t, data(resp)["totals"]); tot["emissionsKg"] != nil ||
		asNum(t, tot["kWh"]) != 5 ||
		data(resp)["factorRef"] != nil ||
		!slices.Contains(asArr(t, data(resp)["qualityWarnings"]), any("factor_missing")) {
		t.Errorf("factor missing: %v", resp)
	}
	// an increase is signed negative (IR68)
	_, resp = post(srv, hq(), "energy.summary", body(end, `,"baselineId":"`+fixed("4", 5)+`"`))
	if tot := asObj(t, data(resp)["totals"]); asNum(t, tot["savedKWh"]) != -1 ||
		asNum(t, tot["savingPercentage"]) != -25 {
		t.Errorf("increase: %v", tot)
	}
	// partial coverage or another minute count: actuals only
	_, resp = post(
		srv,
		hq(),
		"energy.summary",
		body(start.Add(6*time.Minute), `,"baselineId":"`+fixed("10", 6)+`"`),
	)
	if tot := asObj(t, data(resp)["totals"]); asNum(t, tot["kWh"]) != 5 ||
		tot["savedKWh"] != nil ||
		asNum(t, data(resp)["coverage"]) >= 1 {
		t.Errorf("partial: %v", resp)
	}

	_, resp = post(srv, hq(), "energy.summary", body(end, `,"baselineId":"`+fixed("10", 7)+`"`))
	if asObj(t, data(resp)["totals"])["savedKWh"] != nil {
		t.Error("minute count mismatch must not compare")
	}
	// nothing measured: null actuals, coverage 0; no units: coverage null
	_, resp = post(
		srv,
		hq(),
		"energy.summary",
		`{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z","unitIds":["`+unit+`"]}`,
	)
	if tot := asObj(t, data(resp)["totals"]); tot["kWh"] != nil ||
		tot["amountMinor"] != nil ||
		asNum(t, data(resp)["coverage"]) != 0 {
		t.Errorf("no data: %v", resp)
	}

	if code, res := post(
		srv,
		hq(),
		"energy.summary",
		`{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z","unitIds":[]}`,
	); code != 200 ||
		data(res)["coverage"] != nil {
		t.Errorf("no units: %v", res)
	}

	for b, want := range map[string]string{
		body(end, `,"tariffVersion":"tariff-x"`):                      "tariffVersion=error.invalid",
		body(start.Add(90*time.Second), ""):                           refusedEnergyRange, // not whole minutes
		body(start, ""):                                               refusedEnergyRange, // empty
		`{"from":"2026-09-01T00:00:00Z","to":"2026-09-01T00:10:00Z"}`: refusedUnitIDsInvalid,
	} {
		code, m := post(srv, hq(), "energy.summary", b)
		refusal(t, b, code, m, want)
	}

	if code, _ := post(srv, customerA(), "energy.summary", body(end, "")); code != 404 {
		t.Error("other customer's units")
	}

	if code, _ := post(
		srv,
		hq(),
		"energy.summary",
		body(end, `,"baselineId":"`+uuid.NewString()+`"`),
	); code != 404 {
		t.Error("unknown baseline")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAdminSummary(t *testing.T) {
	srv := server(t)
	unit, prop := isolatedUnit(t, srv)
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
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

	q := func(a *actor, extra string) (int, map[string]any) {
		return post(
			srv,
			a,
			"admin.summary",
			`{"from":"`+start.Format(
				time.RFC3339,
			)+`","to":"`+start.Add(5*time.Minute).
				Format(time.RFC3339)+
				`"`+extra+`}`,
		)
	}
	code, res := q(hq(), `,"propertyId":"`+prop+`"`)

	fc, _ := data(res)["energyForecast"].(map[string]any)
	if code != 200 || asNum(t, data(res)["total"]) != 1 ||
		asNum(t, data(res)["customerCount"]) != 1 ||
		len(asObj(t, data(res)["jobCounts"])) != 10 ||
		data(res)["billingVisibility"] != "allowed" ||
		fc["predictedBaselineKWh"] != nil {
		t.Fatalf("admin summary: %d %v", code, res)
	}

	if w := asArr(t, fc["qualityWarnings"]); len(w) != 1 || w[0] != "baseline_unavailable" {
		t.Errorf("forecast warnings: %v", w)
	}

	es := asObj(t, data(res)["energySummary"])
	if asNum(t, asObj(t, es["totals"])["kWh"]) != 5 || es["baselineRef"] != nil ||
		asObj(t, es["totals"])["savedKWh"] != nil {
		t.Errorf("energySummary actuals only: %v", es)
	}
	// a fixed baseline for exactly U is prorated (IR78): 20 kWh over 10 unit-minutes → 10 kWh expected for 5
	write(
		srv,
		hq(),
		"baselines.save",
		`{"unitIds":["`+unit+`"],"period":{"from":"2026-08-01T00:00:00Z","to":"2026-08-01T00:10:00Z"},`+
			`"boundaryId":"ac_input_electricity","boundary":"AC input","assumptions":"demo","source":"test",`+
			`"method":"demo_fixed","baselineKWh":20}`,
		0,
	)
	_, res = q(hq(), `,"propertyId":"`+prop+`"`)

	fc = asObj(t, data(res)["energyForecast"])
	if asNum(t, fc["predictedBaselineKWh"]) != 10 || asNum(t, fc["predictedActualKWh"]) != 5 ||
		asNum(t, fc["forecastSavedKWh"]) != 5 ||
		asNum(t, fc["forecastSavingPercentage"]) != 50 ||
		asNum(t, fc["expectedUnitMinutes"]) != 5 ||
		asNum(t, fc["validUnitMinutes"]) != 5 {
		t.Fatalf("forecast: %v", fc)
	}

	if w := asArr(t, fc["qualityWarnings"]); len(w) != 2 || w[0] != "modeled_baseline" ||
		w[1] != "prorated_forecast" {
		t.Errorf("forecast warnings: %v", w)
	}

	if _, m := q(
		hq(),
		`,"propertyId":"`+uuid.NewString()+`"`,
	); asArr(t, asObj(t, data(m)["energyForecast"])["qualityWarnings"])[0] != "no_units" ||
		data(m)["operatingRate"] != nil {
		t.Errorf("no units: %v", m)
	}

	if _, m := q(
		hq(),
		`,"customerId":"`+seed.ID("cust-b").String()+`"`,
	); asNum(t, data(m)["customerCount"]) != 1 ||
		data(m)["overdueInvoiceCount"] == nil {
		t.Errorf("customer filter: %v", m)
	}

	if code, resp := post(
		srv,
		hq(),
		"admin.summary",
		`{"from":"2026-09-13T12:00:30Z","to":"2026-09-13T12:05:00Z"}`,
	); code != 422 ||
		fields(resp) != refusedEnergyRange {
		t.Errorf("unaligned period: %d %s", code, fields(resp))
	}

	for _, a := range []*actor{customerA(), overrider()} {
		if code, _ := q(a, ""); code != 403 {
			t.Errorf("%s admin.summary: %d", a.membership, code)
		}
	}
}

// TestForecastWithoutReadings: a unit set without any valid reading has no actual to forecast from (IR78), so the
// forecast says so instead of predicting zero.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestForecastWithoutReadings(t *testing.T) {
	s := server(t)
	_, prop := isolatedUnit(t, s)
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	_, res := post(
		s,
		hq(),
		"admin.summary",
		`{"from":"`+start.Format(
			time.RFC3339,
		)+`","to":"`+start.Add(5*time.Minute).
			Format(time.RFC3339)+
			`","propertyId":"`+prop+`"}`,
	)
	fc, _ := data(res)["energyForecast"].(map[string]any)

	raw := asArr(t, fc["qualityWarnings"])

	warnings := make([]string, 0, len(raw))
	for _, w := range raw {
		warnings = append(warnings, asStr(t, w))
	}

	if !slices.Contains(warnings, "actual_unavailable") || fc["predictedActualKWh"] != nil ||
		asNum(t, fc["validUnitMinutes"]) != 0 {
		t.Errorf("forecast without readings: %v", fc)
	}
}

// IR110 / IR153: the monthly report states the month's figures — 5 kWh at the demo tariff is "5.0" and "2.50 MYR" —
// and "n/a" for a month without readings; only the file's metadata is returned, so its size tells the two apart.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestEnergyReportFigures(t *testing.T) {
	srv := server(t)
	_, resp := write(
		srv,
		hq(),
		"properties.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","kind":"home","name":"Report home `+uuid.NewString()[:6]+`","address":null,"accessInstructions":null}`,
		0,
	)
	prop := asStr(t, data(resp)["id"])

	code, resp := write(
		srv,
		hq(),
		"units.save",
		`{"customerOrgId":"`+seed.ID("org-customer-b").
			String()+
			`","propertyId":"`+prop+`","spaceId":null,"displayName":"Report AC","modelId":"`+seed.ID("ventilation-demo").
			String()+
			`","type":"split","installedAt":null,"serviceScope":["indoor"]}`,
		0,
	)
	if code != 200 {
		t.Fatalf("unit: %d %v", code, resp)
	}

	powerSeries(
		t,
		asStr(t, data(resp)["id"]),
		time.Date(2026, 8, 10, 2, 0, 0, 0, time.UTC),
		"measured",
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
		new(float64(60)),
	)

	size := func(month string) int {
		code, res := post(
			srv,
			customerB(),
			"energy.exportReport",
			`{"month":"`+month+`","propertyIds":["`+prop+`"],"sections":["energy_cost"],"format":"csv"}`,
		)
		if code != 200 {
			t.Fatalf("export %s: %d %v", month, code, res)
		}

		return int(asNum(t, data(res)["size"]))
	}

	header := len("section,metric,value\n")
	if got, want := size(
		"2026-08",
	), header+len(
		"energy_cost,kWh,5.0\n",
	)+len(
		"energy_cost,cost,2.50 MYR\n",
	); got != want {
		t.Errorf("August with readings: %d bytes, want %d", got, want)
	}

	if got, want := size(
		"2026-07",
	), header+len(
		"energy_cost,kWh,n/a\n",
	)+len(
		"energy_cost,cost,n/a\n",
	); got != want {
		t.Errorf("July without readings: %d bytes, want %d", got, want)
	}
}
