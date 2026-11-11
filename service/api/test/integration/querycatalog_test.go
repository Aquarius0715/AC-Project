package integration_test

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const jsonStatusActive = `"active"`

// TestQueryCatalogFilters checks every row of docs/02-design/query-catalog.csv against the API: each catalogued filter
// key is accepted on its own (no VALIDATION on filters) and an unknown key is VALIDATION (SR06); each allowed sort
// field sorts both ways and an unknown one is VALIDATION. It keeps the allowlists of the design and the handlers the
// same.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestQueryCatalogFilters(t *testing.T) {
	srv := server(t)

	file, err := os.Open("../../../../docs/02-design/query-catalog.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}

	device, job, invoice := seed.ID("device-online-rto").
		String(),
		seed.ID("job-contractor-a").
			String(),
		seed.ID("invoice-overdue-a").
			String()
	alert := newAlert(
		t,
		newUnit(t, srv, "Query catalog AC"),
		seed.ID("org-customer-b").String(),
		"warning",
		clock().Add(-time.Hour),
	) // the seed has no alerts
	// input wraps the Query of operations whose input is not a bare Query
	input := map[string]func(q string) string{
		"alerts.evidence": func(q string) string { return `{"alertId":"` + alert + `","query":` + q + `}` },
		"attachments.list": func(q string) string { // IR349: the MRV evidence candidates of a unit set and period
			return `{"unitIds":["` + seed.ID("unit-online-rto").
				String() +
				`"],"from":"2026-09-01T00:00:00Z","to":"2026-09-30T00:00:00Z","query":` + q + `}`
		},
		opDevicesEvents:       func(q string) string { return `{"id":"` + device + `","query":` + q + `}` },
		opDevicesCalibrations: func(q string) string { return `{"deviceId":"` + device + `","query":` + q + `}` },
		opDevicesOperations:   func(q string) string { return `{"deviceId":"` + device + `","query":` + q + `}` },
		opJobsEvents:          func(q string) string { return `{"jobId":"` + job + `","query":` + q + `}` },
		"members.capacity":    func(q string) string { return `{"date":"2026-09-15","query":` + q + `}` },
		opMembersEligible: func(q string) string {
			return `{"jobId":"` + job + `","startAt":"2026-09-20T01:00:00Z","endAt":"2026-09-20T03:00:00Z","query":` + q + `}`
		},
		opRestrictionsForInvoice: func(q string) string { return `{"invoiceId":"` + invoice + `","query":` + q + `}` },
		"diagnosticRuns.list": func(q string) string {
			return `{"unitId":"` + seed.ID("unit-online-rto").String() + `","query":` + q + `}`
		},
		opCommandsList: func(q string) string { // IR216: the unit's history (HQ or the owning customer)
			return `{"unitId":"` + seed.ID("unit-online-rto").String() + `","query":` + q + `}`
		},
		"mrv.versions": func(q string) string { return `{"id":"` + uuid.NewString() + `","query":` + q + `}` },
		"notifications.recipients": func(q string) string {
			return `{"target":{"kind":"invoice",` +
				`"id":"` + invoice + `"},"templateKey":"payment_reminder","channel":"email",` +
				`"query":` + q + `}`
		},
		opTelemetrySeries: func(q string) string {
			return `{"from":"2026-09-13T00:00:00Z","to":"2026-09-14T00:00:00Z","unitIds":["` + seed.ID("unit-online-rto").
				String() +
				`"],"metric":"temperature","query":` + q + `}`
		},
		opSummariesGet: func(q string) string { // summaries take the filters directly
			var input struct {
				Filters json.RawMessage `json:"filters"`
			}

			_ = json.Unmarshal([]byte(q), &input)
			if len(input.Filters) == 0 {
				input.Filters = json.RawMessage(`{}`)
			}

			return `{"kind":"partner","filters":` + string(input.Filters) + `}`
		},
	}
	// sample filter values: by key, then per operation where the enum differs
	sample := map[string]string{
		fieldFrom:            `"2026-01-01T00:00:00Z"`,
		"to":                 `"2026-12-31T00:00:00Z"`,
		fieldUnitIDs:         `["` + uuid.NewString() + `"]`,
		"statuses":           `["requested"]`,
		"connections":        `["online"]`,
		fieldSeverity:        `"warning"`,
		"powerState":         `"on"`,
		"search":             `"a"`,
		"qualification":      `"demo_indoor"`,
		"expiringWithinDays": `30`,
		fieldPeriod:          `"2026-08"`,
		"origin":             `"client_request"`,
		"coverage":           `"contract"`,
		"method":             `"demo_fixed"`,
		fieldBoundaryID:      `"ac_input_electricity"`,
		"region":             `"MY"`,
		"year":               `2026`,
		"clientRole":         `"owner"`,
		fieldCode:            `"demo_indoor"`,
		"subjectType":        `"payment"`,
		"targetKind":         `"invoice"`,
		fieldAction:          `"payments.confirm"`,
		fieldRole:            jsonRoleTechnician,
		"result":             `"success"`,
		"correlationId":      `"corr-1"`,
		"actorId":            `"` + uuid.NewString() + `"`,
		"targetId":           `"` + uuid.NewString() + `"`,
		fieldMetric:          `"temperature"`,
		"includeDescendants": jsonTrue,
		"overdueOnly":        jsonTrue,
		"unreadOnly":         jsonTrue,
		"activeOnly":         jsonTrue,
		"unassignedOnly":     jsonTrue,
		"bound":              jsonTrue,
		"withoutDevice":      jsonTrue,
		"proposalPending":    jsonTrue,
		fieldEnabled:         jsonTrue,
	}
	perOp := map[string]map[string]string{
		opAlertsList:             {fieldStatus: `"open"`},
		"automations.list":       {fieldKind: `"schedule"`},
		"customers.list":         {fieldKind: jsonKindRTO, fieldStatus: jsonStatusActive},
		"organizations.list":     {fieldKind: `"customer"`, fieldStatus: jsonStatusActive},
		"spaces.list":            {fieldKind: `"floor"`},
		"notifications.list":     {fieldType: `"payment"`},
		"contracts.list":         {fieldKind: jsonKindRTO},
		opDevicesList:            {fieldStatus: `"online"`},
		opInquiriesList:          {fieldStatus: `"received"`},
		"invoices.list":          {fieldStatus: `"unpaid"`},
		opJobsList:               {fieldStatus: `"requested"`, fieldType: `"reactive"`},
		"mrv.list":               {fieldStatus: `"draft"`},
		"offsets.list":           {fieldStatus: `"demo_requested"`},
		opPoliciesList:           {fieldKind: `"alert"`},
		"properties.list":        {fieldKind: `"home"`},
		"restrictions.list":      {fieldStatus: `"applied"`},
		opRestrictionsForInvoice: {fieldStatus: `"applied"`},
		opUnitsList: {
			fieldStatus:          `"online"`,
			"includeDescendants": `true,"spaceId":"` + seed.ID("floor-1").String() + `"`,
		},
		opClientUsersList: {fieldStatus: jsonStatusActive},
		"partnerUsers.list": {
			fieldStatus:   jsonStatusActive,
			"partnerRole": jsonRoleTechnician,
		},
		opContractorsList:        {fieldStatus: jsonStatusActive},
		"certificates.list":      {fieldStatus: `"valid"`},
		"payouts.list":           {fieldStatus: `"draft"`},
		opFilterCareList:         {fieldStatus: `"overdue"`},
		"firmwareCampaigns.list": {fieldStatus: `"running"`},
		opSummariesGet:           {fieldStatus: `"requested"`},
	}
	// base filters every call of the operation carries (audit.list needs its period)
	base := map[string]string{
		"audit.list": `"from":"2026-01-01T00:00:00Z","to":"2026-12-31T00:00:00Z"`,
	}
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
	callers := []*actor{hq(), restrMgr(), contrA(), techA(), customerA()}

	for _, row := range rows[1:] {
		operation := row[0]

		wrap := input[operation]
		if wrap == nil {
			wrap = func(q string) string { return q }
		}

		t.Run(operation, func(t *testing.T) {
			var who *actor

			for _, caller := range callers {
				if code, _ := post(
					srv,
					caller,
					operation,
					wrap(query(operation, "")),
				); code == 200 ||
					(missingParent[operation] && code == 404) {
					who = caller

					break
				}
			}

			if who == nil {
				t.Fatalf("no actor can read %s", operation)
			}

			if code, res := post(
				srv,
				who,
				operation,
				wrap(query(operation, `"zzUnknown":1`)),
			); code != 422 ||
				!filterError(res) {
				t.Errorf("unknown filter key: %d %v", code, res)
			}

			for key := range strings.SplitSeq(row[1], ",") {
				if key = strings.TrimSpace(key); key == "" {
					continue
				}

				value, ok := perOp[operation][key]
				if !ok {
					value, ok = sample[key]
				}

				if !ok {
					switch {
					case strings.HasSuffix(key, "Id"):
						value = `"` + uuid.NewString() + `"`
					default:
						t.Errorf("no sample value for %s", key)

						continue
					}
				}

				if code, res := post(
					srv,
					who,
					operation,
					wrap(query(operation, `"`+key+`":`+value)),
				); code != 200 &&
					(!missingParent[operation] || code != 404) {
					t.Errorf("filter %s=%s: %d %v", key, value, code, res)
				}
			}

			if operation == opSummariesGet { // takes filters only, no Query
				return
			}
			// allowed_sort: each field sorts both ways; an unknown field is VALIDATION
			sorted := func(field, dir string) (int, map[string]any) {
				q := strings.TrimSuffix(
					query(operation, ""),
					"}",
				) + `,"sort":{"field":"` + field + `","direction":"` + dir + `"}}`

				return post(srv, who, operation, wrap(q))
			}
			for field := range strings.SplitSeq(row[2], ",") {
				if field = strings.TrimSpace(field); field == "" {
					continue
				}

				for _, dir := range []string{"asc", "desc"} {
					if code, m := sorted(
						field,
						dir,
					); code != 200 &&
						(!missingParent[operation] || code != 404) {
						t.Errorf("sort %s %s: %d %v", field, dir, code, m)
					}
				}
				// a direction other than asc / desc never reaches the ORDER BY: an injection attempt is refused on
				// the direction (IR365)
				if code, m := sorted(field, `desc, (SELECT 1)`); code != 422 || !directionError(m) {
					t.Errorf("injected direction on %s: %d %v", field, code, m)
				}
			}
			// an unknown sort field is refused on the sort (sort.field, query.sort.field; query where nothing sorts)
			if code, m := sorted("zzUnknown", "asc"); code != 422 || !sortError(m) {
				t.Errorf("unknown sort field: %d %v", code, m)
			}
		})
	}
}

// directionError reports whether a VALIDATION result names the sort direction.
func directionError(m map[string]any) bool {
	fe, _ := m["fieldErrors"].(map[string]any)

	return fe["sort.direction"] != nil || fe["query.sort.direction"] != nil
}

// sortError reports whether a VALIDATION result names the sort.
func sortError(m map[string]any) bool {
	fe, _ := m["fieldErrors"].(map[string]any)
	for k := range fe {
		if k == "sort.field" || k == "query.sort.field" || k == "query" {
			return true
		}
	}

	return false
}

// filterError reports whether a VALIDATION result names the filters.
func filterError(m map[string]any) bool {
	fieldErrs, _ := m["fieldErrors"].(map[string]any)
	if fieldErrs["zzUnknown"] == apperr.KeyNotAllowed {
		// REST: filters are query parameters, an unknown one is not allowed (IR222)
		return true
	}

	for k := range fieldErrs {
		// filters of a bare Query; query.filters (or the whole query) of a nested one
		if k == fieldFilters || strings.HasPrefix(k, "filters.") || k == "query" ||
			strings.HasPrefix(k, "query.filters") {
			return true
		}
	}

	return false
}
