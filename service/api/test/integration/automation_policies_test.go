package integration_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func autoPolicyBody(extra map[string]string) string {
	f := map[string]string{
		fieldKind:     `"automation"`,
		fieldName:     `"Peak shave"`,
		fieldTimezone: jsonKualaLumpur,
		fieldEnabled:  jsonTrue,
		fieldPriority: "60",
		fieldUnitIDs: `["` + seed.ID("unit-non-rto").
			String() +
			`","` + seed.ID("unit-online-rto").
			String() +
			`"]`,
		fieldCondition: `{"type":"tariff","operator":"gte","value":0.5,"unit":"MYR_per_kWh"}`,
		fieldAction:    `{"kind":"set_temperature","celsius":26}`,
	}

	for k, v := range extra {
		if v == "" {
			delete(f, k)

			continue
		}

		f[k] = v
	}

	parts := make([]string, 0, len(f))
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAutomationPolicies(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`DELETE FROM control.automation_policies`,
	) // test-only rows (none are seeded); keeps the lists within one page

	code, resp := write(srv, hq(), "policies.save", autoPolicyBody(nil), 0)
	if code != 200 || data(resp)["kind"] != kindAutomation ||
		len(asArr(t, data(resp)["unitIds"])) != 2 ||
		asNum(t, asObj(t, data(resp)["action"])["celsius"]) != 26 {
		t.Fatalf("create: %d %v", code, resp)
	}

	id := asStr(t, data(resp)["id"])

	valid := map[string]map[string]string{
		"occupancy": {
			fieldCondition: `{"type":"occupancy","occupied":false}`,
			fieldAction:    actionPowerOff,
		},
		"peak": {
			fieldCondition: `{"type":"peak","active":true}`,
			fieldAction:    `{"kind":"set_mode","mode":"dry"}`,
		},
		"solar": {
			fieldCondition: `{"type":"solar","operator":"gt","value":2.5,"unit":"kW"}`,
			fieldAction:    `{"kind":"set_fan","fanLevel":"high"}`,
		},
		"battery": {
			fieldCondition: `{"type":"battery","operator":"lt","value":1,"unit":"kW"}`,
			fieldAction:    `{"kind":"set_temperature","celsius":16}`,
		},
		"ventilate": {
			fieldUnitIDs: `["` + seed.ID("unit-online-rto").String() + `"]`,
			fieldAction:  `{"kind":"ventilate","level":"mid"}`,
		},
	}
	for name, e := range valid {
		if code, m := write(srv, hq(), "policies.save", autoPolicyBody(e), 0); code != 200 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}

	bad := map[string]map[string]string{
		caseNoUnits: {fieldUnitIDs: `[]`},
		caseDuplicateUnit: {
			fieldUnitIDs: `["` + seed.ID("unit-non-rto").
				String() +
				`","` + seed.ID("unit-non-rto").
				String() +
				`"]`,
		},
		"weather not in DD-A11": {
			fieldCondition: `{"type":"weather","metric":"temperature","operator":"gt","value":30}`,
		},
		"tariff wrong unit": {
			fieldCondition: `{"type":"tariff","operator":"gte","value":0.5,"unit":"kW"}`,
		},
		"tariff negative": {
			fieldCondition: `{"type":"tariff","operator":"gte","value":-1,"unit":"MYR_per_kWh"}`,
		},
		"occupancy extra field": {fieldCondition: `{"type":"occupancy","occupied":true,"value":1}`},
		"peak missing active":   {fieldCondition: `{"type":"peak"}`},
		"solar bad operator": {
			fieldCondition: `{"type":"solar","operator":"eq","value":1,"unit":"kW"}`,
		},
		"no condition":            {fieldCondition: ""},
		"bad action kind":         {fieldAction: `{"kind":"explode"}`},
		"action two fields":       {fieldAction: `{"kind":"set_power","power":true,"celsius":20}`},
		caseBadMode:               {fieldAction: `{"kind":"set_mode","mode":"heat"}`},
		"temperature above max":   {fieldAction: `{"kind":"set_temperature","celsius":31}`},
		"temperature off step":    {fieldAction: `{"kind":"set_temperature","celsius":24.5}`},
		"ventilate unsupported":   {fieldAction: actionVentilateLow},
		"alert fields":            {fieldCustomerID: `"` + seed.ID("cust-a").String() + `"`},
		"name blank":              {fieldName: jsonBlank},
		"priority -1":             {fieldPriority: "-1"},
		"no action":               {fieldAction: ""},
		"temperature not finite?": {fieldAction: `{"kind":"set_temperature"}`},
	}
	// the field each case is refused for
	want := map[string]string{
		"action two fields":       refusedActionInvalid,
		"alert fields":            "kind=error.fieldsOfOtherKind",
		"bad action kind":         refusedActionInvalid,
		caseBadMode:               refusedActionInvalid,
		caseDuplicateUnit:         refusedUnitIDsInvalid,
		"name blank":              refusedNameLength,
		"no action":               refusedActionInvalid,
		"no condition":            refusedConditionInvalid,
		caseNoUnits:               refusedUnitIDsInvalid,
		"occupancy extra field":   refusedConditionInvalid,
		"peak missing active":     refusedConditionInvalid,
		"priority -1":             refusedPriorityRange,
		"solar bad operator":      refusedConditionInvalid,
		"tariff negative":         refusedConditionInvalid,
		"tariff wrong unit":       refusedConditionInvalid,
		"temperature above max":   refusedActionUnsupported,
		"temperature not finite?": refusedActionInvalid,
		"temperature off step":    refusedActionUnsupported,
		"ventilate unsupported":   refusedActionUnsupported,
		"weather not in DD-A11":   refusedConditionInvalid,
	}

	for name, e := range bad {
		code, m := write(srv, hq(), "policies.save", autoPolicyBody(e), 0)
		refusal(t, name, code, m, want[name])
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.save",
		autoPolicyBody(map[string]string{fieldUnitIDs: `["` + uuid.NewString() + `"]`}),
		0,
	); code != 404 {
		t.Error("unknown unit → NOT_FOUND")
	}

	if code, res := write(
		srv,
		hq(),
		"policies.save",
		alertPolicyBody(map[string]string{fieldCondition: `{"type":"peak","active":true}`}),
		0,
	); code != 422 ||
		fields(res) != "kind=error.fieldsOfOtherKind" {
		t.Errorf("alert policy with automation fields: %d %s", code, fields(res))
	}

	// update replaces units and clears disabledReason; kind and version rules
	owner(
		t,
		`UPDATE control.automation_policies SET disabled_reason = 'capability_changed' WHERE id = $1`,
		id,
	)

	code, resp = write(
		srv,
		hq(),
		"policies.save",
		autoPolicyBody(
			map[string]string{
				"id":         `"` + id + `"`,
				fieldUnitIDs: `["` + seed.ID("unit-limited").String() + `"]`,
			},
		),
		1,
	)
	if code != 200 || len(asArr(t, data(resp)["unitIds"])) != 1 ||
		data(resp)["disabledReason"] != nil ||
		asNum(t, data(resp)["version"]) != 2 {
		t.Fatalf("update: %d %v", code, resp)
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.save",
		autoPolicyBody(map[string]string{"id": `"` + id + `"`}),
		1,
	); code != 409 {
		t.Error("stale version")
	}

	def := seed.DefaultPolicyID("tenant-a").String()

	_, dm := post(srv, hq(), "policies.get", `{"policyId":"`+def+`"}`)
	if code, res := write(
		srv,
		hq(),
		"policies.save",
		autoPolicyBody(map[string]string{"id": `"` + def + `"`}),
		ver(t, dm),
	); code != 422 ||
		fields(res) != refusedKindFixed {
		t.Errorf("default policy saved as automation: %d %s", code, fields(res))
	}

	// reads: kind filter, unit/customer filters, client and permission scope
	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(t, m) {
			out[asStr(t, it["id"])] = true
		}

		return out
	}

	_, resp = post(srv, hq(), "policies.list", `{"filters":{"kind":"automation"},"limit":100}`)
	if !ids(resp)[id] {
		t.Fatal("kind=automation")
	}

	for _, it := range items(t, resp) {
		if it["kind"] != kindAutomation {
			t.Fatal("kind filter leaks")
		}
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").
			String()+
			`"},"limit":100}`,
	)
	if !ids(resp)[id] {
		t.Fatal("automation unitId filter")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-other-customer").
			String()+
			`"},"limit":100}`,
	)
	if ids(resp)[id] {
		t.Fatal("automation unitId filter excludes")
	}
	// the unit and the property together: the unit counts only when it is in the property
	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").
			String()+
			`","propertyId":"`+seed.ID("property-office-a").
			String()+
			`"},"limit":100}`,
	)
	if !ids(resp)[id] {
		t.Error("unit in the property")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").
			String()+
			`","propertyId":"`+seed.ID("property-home-a").
			String()+
			`"},"limit":100}`,
	)
	if ids(resp)[id] {
		t.Error("unit outside the property")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","customerId":"`+seed.ID("cust-a").
			String()+
			`","enabled":true},"limit":100}`,
	)
	if !ids(resp)[id] {
		t.Fatal("automation customerId filter")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"kind":"automation","customerId":"`+seed.ID("cust-b").
			String()+
			`"},"limit":100}`,
	)
	if ids(resp)[id] {
		t.Fatal("automation customerId filter excludes")
	}

	if code, _ := post(srv, customerA(), "policies.get", `{"policyId":"`+id+`"}`); code != 404 {
		t.Error("clients do not see automation policies")
	}

	_, resp = post(srv, customerA(), "policies.list", `{"filters":{"kind":"automation"}}`)
	if len(items(t, resp)) != 0 {
		t.Error("client automation list")
	}

	// delete removes unit targets
	if code, _ := write(
		srv,
		customerA(),
		"policies.delete",
		`{"policyId":"`+id+`"}`,
		2,
	); code != 404 {
		t.Error("client delete automation")
	}

	if code, res := write(
		srv,
		hq(),
		"policies.delete",
		`{"policyId":"`+id+`"}`,
		2,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("delete: %d", code)
	}

	if code, _ := post(srv, hq(), "policies.get", `{"policyId":"`+id+`"}`); code != 404 {
		t.Error("deleted automation policy")
	}
}
