package integration

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func autoPolicyBody(extra map[string]string) string {
	f := map[string]string{"kind": `"automation"`, "name": `"Peak shave"`, "timezone": `"Asia/Kuala_Lumpur"`, "enabled": "true", "priority": "60",
		"unitIds":   `["` + seed.ID("unit-non-rto").String() + `","` + seed.ID("unit-online-rto").String() + `"]`,
		"condition": `{"type":"tariff","operator":"gte","value":0.5,"unit":"MYR_per_kWh"}`, "action": `{"kind":"set_temperature","celsius":26}`}
	for k, v := range extra {
		if v == "" {
			delete(f, k)
			continue
		}
		f[k] = v
	}
	parts := []string{}
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestAutomationPolicies(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.automation_policies`) // test-only rows (none are seeded); keeps the lists within one page
	code, m := write(s, &hq, "policies.save", autoPolicyBody(nil), 0)
	if code != 200 || data(m)["kind"] != "automation" || len(data(m)["unitIds"].([]any)) != 2 || data(m)["action"].(map[string]any)["celsius"].(float64) != 26 {
		t.Fatalf("create: %d %v", code, m)
	}
	id := data(m)["id"].(string)

	valid := map[string]map[string]string{
		"occupancy": {"condition": `{"type":"occupancy","occupied":false}`, "action": `{"kind":"set_power","power":false}`},
		"peak":      {"condition": `{"type":"peak","active":true}`, "action": `{"kind":"set_mode","mode":"dry"}`},
		"solar":     {"condition": `{"type":"solar","operator":"gt","value":2.5,"unit":"kW"}`, "action": `{"kind":"set_fan","fanLevel":"high"}`},
		"battery":   {"condition": `{"type":"battery","operator":"lt","value":1,"unit":"kW"}`, "action": `{"kind":"set_temperature","celsius":16}`},
		"ventilate": {"unitIds": `["` + seed.ID("unit-online-rto").String() + `"]`, "action": `{"kind":"ventilate","level":"mid"}`},
	}
	for name, e := range valid {
		if code, m := write(s, &hq, "policies.save", autoPolicyBody(e), 0); code != 200 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	bad := map[string]map[string]string{
		"no units":                {"unitIds": `[]`},
		"duplicate unit":          {"unitIds": `["` + seed.ID("unit-non-rto").String() + `","` + seed.ID("unit-non-rto").String() + `"]`},
		"weather not in DD-A11":   {"condition": `{"type":"weather","metric":"temperature","operator":"gt","value":30}`},
		"tariff wrong unit":       {"condition": `{"type":"tariff","operator":"gte","value":0.5,"unit":"kW"}`},
		"tariff negative":         {"condition": `{"type":"tariff","operator":"gte","value":-1,"unit":"MYR_per_kWh"}`},
		"occupancy extra field":   {"condition": `{"type":"occupancy","occupied":true,"value":1}`},
		"peak missing active":     {"condition": `{"type":"peak"}`},
		"solar bad operator":      {"condition": `{"type":"solar","operator":"eq","value":1,"unit":"kW"}`},
		"no condition":            {"condition": ""},
		"bad action kind":         {"action": `{"kind":"explode"}`},
		"action two fields":       {"action": `{"kind":"set_power","power":true,"celsius":20}`},
		"bad mode":                {"action": `{"kind":"set_mode","mode":"heat"}`},
		"temperature above max":   {"action": `{"kind":"set_temperature","celsius":31}`},
		"temperature off step":    {"action": `{"kind":"set_temperature","celsius":24.5}`},
		"ventilate unsupported":   {"action": `{"kind":"ventilate","level":"low"}`},
		"alert fields":            {"customerId": `"` + seed.ID("cust-a").String() + `"`},
		"name blank":              {"name": `" "`},
		"priority -1":             {"priority": "-1"},
		"no action":               {"action": ""},
		"temperature not finite?": {"action": `{"kind":"set_temperature"}`},
	}
	for name, e := range bad {
		if code, _ := write(s, &hq, "policies.save", autoPolicyBody(e), 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "policies.save", autoPolicyBody(map[string]string{"unitIds": `["` + uuid.NewString() + `"]`}), 0); code != 404 {
		t.Error("unknown unit → NOT_FOUND")
	}
	if code, _ := write(s, &hq, "policies.save", alertPolicyBody(map[string]string{"condition": `{"type":"peak","active":true}`}), 0); code != 422 {
		t.Error("alert policy with automation fields")
	}

	// update replaces units and clears disabledReason; kind and version rules
	owner(t, `UPDATE control.automation_policies SET disabled_reason = 'capability_changed' WHERE id = $1`, id)
	code, m = write(s, &hq, "policies.save", autoPolicyBody(map[string]string{"id": `"` + id + `"`, "unitIds": `["` + seed.ID("unit-limited").String() + `"]`}), 1)
	if code != 200 || len(data(m)["unitIds"].([]any)) != 1 || data(m)["disabledReason"] != nil || data(m)["version"].(float64) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "policies.save", autoPolicyBody(map[string]string{"id": `"` + id + `"`}), 1); code != 409 {
		t.Error("stale version")
	}
	def := seed.DefaultPolicyID("tenant-a").String()
	_, dm := post(s, &hq, "policies.get", `{"policyId":"`+def+`"}`)
	if code, _ := write(s, &hq, "policies.save", autoPolicyBody(map[string]string{"id": `"` + def + `"`}), ver(dm)); code != 422 {
		t.Error("default policy saved as automation")
	}

	// reads: kind filter, unit/customer filters, client and permission scope
	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(m) {
			out[it["id"].(string)] = true
		}
		return out
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation"},"limit":100}`)
	if !ids(m)[id] {
		t.Fatal("kind=automation")
	}
	for _, it := range items(m) {
		if it["kind"] != "automation" {
			t.Fatal("kind filter leaks")
		}
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").String()+`"},"limit":100}`)
	if !ids(m)[id] {
		t.Fatal("automation unitId filter")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-other-customer").String()+`"},"limit":100}`)
	if ids(m)[id] {
		t.Fatal("automation unitId filter excludes")
	}
	// the unit and the property together: the unit counts only when it is in the property
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").String()+`","propertyId":"`+seed.ID("property-office-a").String()+`"},"limit":100}`)
	if !ids(m)[id] {
		t.Error("unit in the property")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","unitId":"`+seed.ID("unit-limited").String()+`","propertyId":"`+seed.ID("property-home-a").String()+`"},"limit":100}`)
	if ids(m)[id] {
		t.Error("unit outside the property")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","customerId":"`+seed.ID("cust-a").String()+`","enabled":true},"limit":100}`)
	if !ids(m)[id] {
		t.Fatal("automation customerId filter")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"automation","customerId":"`+seed.ID("cust-b").String()+`"},"limit":100}`)
	if ids(m)[id] {
		t.Fatal("automation customerId filter excludes")
	}
	if code, _ := post(s, &customerA, "policies.get", `{"policyId":"`+id+`"}`); code != 404 {
		t.Error("clients do not see automation policies")
	}
	_, m = post(s, &customerA, "policies.list", `{"filters":{"kind":"automation"}}`)
	if len(items(m)) != 0 {
		t.Error("client automation list")
	}

	// delete removes unit targets
	if code, _ := write(s, &customerA, "policies.delete", `{"policyId":"`+id+`"}`, 2); code != 404 {
		t.Error("client delete automation")
	}
	if code, m := write(s, &hq, "policies.delete", `{"policyId":"`+id+`"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := post(s, &hq, "policies.get", `{"policyId":"`+id+`"}`); code != 404 {
		t.Error("deleted automation policy")
	}
}
