package integration

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestQueryCatalogFilters checks every row of docs/02-design/query-catalog.csv against the API: each catalogued filter
// key is accepted on its own (no VALIDATION on filters) and an unknown key is VALIDATION (SR06); each allowed sort field
// sorts both ways and an unknown one is VALIDATION. It keeps the allowlists of the design and the handlers the same.
func TestQueryCatalogFilters(t *testing.T) {
	s := server(t)
	f, err := os.Open("../../../../docs/02-design/query-catalog.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	device, job, invoice := seed.ID("device-online-rto").String(), seed.ID("job-contractor-a").String(), seed.ID("invoice-overdue-a").String()
	alert := newAlert(t, newUnit(t, s, "Query catalog AC"), seed.ID("org-customer-b").String(), "warning", clock.Add(-time.Hour)) // the seed has no alerts
	// input wraps the Query of operations whose input is not a bare Query
	input := map[string]func(q string) string{
		"alerts.evidence":      func(q string) string { return `{"alertId":"` + alert + `","query":` + q + `}` },
		"devices.events":       func(q string) string { return `{"id":"` + device + `","query":` + q + `}` },
		"devices.calibrations": func(q string) string { return `{"deviceId":"` + device + `","query":` + q + `}` },
		"devices.operations":   func(q string) string { return `{"deviceId":"` + device + `","query":` + q + `}` },
		"jobs.events":          func(q string) string { return `{"jobId":"` + job + `","query":` + q + `}` },
		"members.capacity":     func(q string) string { return `{"date":"2026-09-15","query":` + q + `}` },
		"members.eligible": func(q string) string {
			return `{"jobId":"` + job + `","startAt":"2026-09-20T01:00:00Z","endAt":"2026-09-20T03:00:00Z","query":` + q + `}`
		},
		"restrictions.forInvoice": func(q string) string { return `{"invoiceId":"` + invoice + `","query":` + q + `}` },
		"diagnosticRuns.list": func(q string) string {
			return `{"unitId":"` + seed.ID("unit-online-rto").String() + `","query":` + q + `}`
		},
		"commands.list": func(q string) string { // IR216: the unit's history (HQ or the owning customer)
			return `{"unitId":"` + seed.ID("unit-online-rto").String() + `","query":` + q + `}`
		},
		"mrv.versions": func(q string) string { return `{"id":"` + uuid.NewString() + `","query":` + q + `}` },
		"notifications.recipients": func(q string) string {
			return `{"target":{"kind":"invoice","id":"` + invoice + `"},"templateKey":"payment_reminder","channel":"email","query":` + q + `}`
		},
		"telemetry.series": func(q string) string {
			return `{"from":"2026-09-13T00:00:00Z","to":"2026-09-14T00:00:00Z","unitIds":["` + seed.ID("unit-online-rto").String() + `"],"metric":"temperature","query":` + q + `}`
		},
		"summaries.get": func(q string) string { // summaries take the filters directly
			var x struct {
				Filters json.RawMessage `json:"filters"`
			}
			_ = json.Unmarshal([]byte(q), &x)
			if len(x.Filters) == 0 {
				x.Filters = json.RawMessage(`{}`)
			}
			return `{"kind":"partner","filters":` + string(x.Filters) + `}`
		},
	}
	// sample filter values: by key, then per operation where the enum differs
	sample := map[string]string{
		"from": `"2026-01-01T00:00:00Z"`, "to": `"2026-12-31T00:00:00Z"`, "unitIds": `["` + uuid.NewString() + `"]`, "statuses": `["requested"]`,
		"connections": `["online"]`, "severity": `"warning"`, "powerState": `"on"`, "search": `"a"`, "qualification": `"demo_indoor"`,
		"expiringWithinDays": `30`, "period": `"2026-08"`, "origin": `"client_request"`, "coverage": `"contract"`, "method": `"demo_fixed"`,
		"boundaryId": `"ac_input_electricity"`, "region": `"MY"`, "year": `2026`, "clientRole": `"owner"`, "code": `"demo_indoor"`,
		"subjectType": `"payment"`, "targetKind": `"invoice"`, "action": `"payments.confirm"`, "role": `"technician"`, "result": `"success"`,
		"correlationId": `"corr-1"`, "actorId": `"` + uuid.NewString() + `"`, "targetId": `"` + uuid.NewString() + `"`, "metric": `"temperature"`,
		"includeDescendants": `true`, "overdueOnly": `true`, "unreadOnly": `true`, "activeOnly": `true`, "unassignedOnly": `true`, "proposalPending": `true`, "enabled": `true`,
	}
	perOp := map[string]map[string]string{
		"alerts.list":             {"status": `"open"`},
		"automations.list":        {"kind": `"schedule"`},
		"customers.list":          {"kind": `"rto"`, "status": `"active"`},
		"organizations.list":      {"kind": `"customer"`, "status": `"active"`},
		"spaces.list":             {"kind": `"floor"`},
		"notifications.list":      {"type": `"payment"`},
		"contracts.list":          {"kind": `"rto"`},
		"devices.list":            {"status": `"online"`},
		"inquiries.list":          {"status": `"received"`},
		"invoices.list":           {"status": `"unpaid"`},
		"jobs.list":               {"status": `"requested"`, "type": `"reactive"`},
		"mrv.list":                {"status": `"draft"`},
		"offsets.list":            {"status": `"demo_requested"`},
		"policies.list":           {"kind": `"alert"`},
		"properties.list":         {"kind": `"home"`},
		"restrictions.list":       {"status": `"applied"`},
		"restrictions.forInvoice": {"status": `"applied"`},
		"units.list":              {"status": `"online"`, "includeDescendants": `true,"spaceId":"` + seed.ID("floor-1").String() + `"`},
		"clientUsers.list":        {"status": `"active"`},
		"contractors.list":        {"status": `"active"`},
		"certificates.list":       {"status": `"valid"`},
		"payouts.list":            {"status": `"draft"`},
		"filterCare.list":         {"status": `"overdue"`},
		"firmwareCampaigns.list":  {"status": `"running"`},
		"summaries.get":           {"status": `"requested"`},
	}
	// base filters every call of the operation carries (audit.list needs its period)
	base := map[string]string{"audit.list": `"from":"2026-01-01T00:00:00Z","to":"2026-12-31T00:00:00Z"`}
	query := func(op, extra string) string {
		parts := []string{}
		for _, p := range []string{base[op], extra} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		return `{"filters":{` + strings.Join(parts, ",") + `},"limit":5}`
	}
	// operations whose parent record does not exist here: the filter checks still run first
	missingParent := map[string]bool{"mrv.versions": true}
	callers := []*actor{&hq, &restrMgr, &contrA, &techA, &customerA}
	for _, row := range rows[1:] {
		op := row[0]
		wrap := input[op]
		if wrap == nil {
			wrap = func(q string) string { return q }
		}
		t.Run(op, func(t *testing.T) {
			var who *actor
			for _, a := range callers {
				if code, _ := post(s, a, op, wrap(query(op, ""))); code == 200 || (missingParent[op] && code == 404) {
					who = a
					break
				}
			}
			if who == nil {
				t.Fatalf("no actor can read %s", op)
			}
			if code, m := post(s, who, op, wrap(query(op, `"zzUnknown":1`))); code != 422 || !filterError(m) {
				t.Errorf("unknown filter key: %d %v", code, m)
			}
			for _, key := range strings.Split(row[1], ",") {
				if key = strings.TrimSpace(key); key == "" {
					continue
				}
				v, ok := perOp[op][key]
				if !ok {
					v, ok = sample[key]
				}
				if !ok {
					switch {
					case strings.HasSuffix(key, "Id"):
						v = `"` + uuid.NewString() + `"`
					default:
						t.Errorf("no sample value for %s", key)
						continue
					}
				}
				if code, m := post(s, who, op, wrap(query(op, `"`+key+`":`+v))); code != 200 && !(missingParent[op] && code == 404) {
					t.Errorf("filter %s=%s: %d %v", key, v, code, m)
				}
			}
			if op == "summaries.get" { // takes filters only, no Query
				return
			}
			// allowed_sort: each field sorts both ways; an unknown field is VALIDATION
			sorted := func(field, dir string) (int, map[string]any) {
				q := strings.TrimSuffix(query(op, ""), "}") + `,"sort":{"field":"` + field + `","direction":"` + dir + `"}}`
				return post(s, who, op, wrap(q))
			}
			for _, field := range strings.Split(row[2], ",") {
				if field = strings.TrimSpace(field); field == "" {
					continue
				}
				for _, dir := range []string{"asc", "desc"} {
					if code, m := sorted(field, dir); code != 200 && !(missingParent[op] && code == 404) {
						t.Errorf("sort %s %s: %d %v", field, dir, code, m)
					}
				}
			}
			if code, m := sorted("zzUnknown", "asc"); code != 422 {
				t.Errorf("unknown sort field: %d %v", code, m)
			}
		})
	}
}

// filterError reports whether a VALIDATION result names the filters.
func filterError(m map[string]any) bool {
	fe, _ := m["fieldErrors"].(map[string]any)
	if fe["zzUnknown"] == "error.notAllowed" { // REST: filters are query parameters, an unknown one is not allowed (IR222)
		return true
	}
	for k := range fe {
		// filters of a bare Query; query.filters (or the whole query) of a nested one
		if k == "filters" || strings.HasPrefix(k, "filters.") || k == "query" || strings.HasPrefix(k, "query.filters") {
			return true
		}
	}
	return false
}
