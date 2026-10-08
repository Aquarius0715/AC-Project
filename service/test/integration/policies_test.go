package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

// alertPolicyBody builds an alert PolicyInput with overrides (raw JSON values).
func alertPolicyBody(extra map[string]string) string {
	f := map[string]string{"kind": `"alert"`, "name": `"Server room too warm"`, "timezone": `"Asia/Kuala_Lumpur"`, "enabled": "true", "priority": "50",
		"customerId": `"` + seed.ID("cust-a").String() + `"`, "metric": `"temperature"`, "operator": `"gte"`, "threshold": "28", "recoveryThreshold": "26",
		"durationSeconds": "300", "activeWindow": "null", "severity": `"warning"`, "recipientMembershipIds": `["` + seed.ID("customer-a").String() + `"]`,
		"channels": `["inApp","email"]`, "escalateAfterMinutes": "60", "cooldownMinutes": "5"}
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

func TestPolicies(t *testing.T) {
	s := server(t)
	def := seed.DefaultPolicyID("tenant-a").String()
	owner(t, `DELETE FROM monitoring.default_rule_settings WHERE customer_id = $1`, seed.ID("cust-b"))

	// create (client owner) and read back
	code, m := write(s, &customerA, "policies.save", alertPolicyBody(map[string]string{"name": `"  Warm room  "`}), 0)
	if code != 200 || data(m)["kind"] != "alert" || data(m)["name"] != "Warm room" || data(m)["metric"] != "temperature" || len(data(m)["unitIds"].([]any)) != 0 {
		t.Fatalf("create: %d %v", code, m)
	}
	pid := data(m)["id"].(string)
	if code, m := post(s, &customerA, "policies.get", `{"policyId":"`+pid+`"}`); code != 200 || data(m)["customerId"] != seed.ID("cust-a").String() {
		t.Fatalf("get: %d %v", code, m)
	}
	if code, _ := post(s, &customerB, "policies.get", `{"policyId":"`+pid+`"}`); code != 404 {
		t.Error("customer-b must not read customer-a's policy")
	}
	// default policy: clients see it with only their own settings; HQ sees all settings
	_, m = post(s, &customerA, "policies.get", `{"policyId":"`+def+`"}`)
	if len(data(m)["rules"].([]any)) != 6 || data(m)["customerId"] != nil {
		t.Fatalf("default policy: %v", m)
	}
	for _, rs := range data(m)["ruleSettings"].([]any) {
		if rs.(map[string]any)["customerId"] != seed.ID("cust-a").String() {
			t.Fatal("client sees another customer's rule setting")
		}
	}
	_, m = post(s, &customerB, "policies.get", `{"policyId":"`+def+`"}`)
	if len(data(m)["ruleSettings"].([]any)) != 0 {
		t.Fatal("customer-b sees customer-a's setting")
	}

	// validation
	bad := map[string]map[string]string{
		"recovery direction gte":   {"recoveryThreshold": "28"},
		"recovery direction lte":   {"operator": `"lte"`, "recoveryThreshold": "27"},
		"duration 0":               {"durationSeconds": "0"},
		"duration > 86400":         {"durationSeconds": "86401"},
		"no recipients":            {"recipientMembershipIds": "[]"},
		"channels without inApp":   {"channels": `["email"]`},
		"duplicate channel":        {"channels": `["inApp","inApp"]`},
		"bad channel":              {"channels": `["inApp","sms"]`},
		"blank name":               {"name": `"   "`},
		"name too long":            {"name": `"` + strings.Repeat("n", 121) + `"`},
		"priority 101":             {"priority": "101"},
		"bad timezone":             {"timezone": `"Mars/Base"`},
		"cooldown 0":               {"cooldownMinutes": "0"},
		"escalate 1441":            {"escalateAfterMinutes": "1441"},
		"bad severity":             {"severity": `"info"`},
		"unknown metric":           {"metric": `"noise"`},
		"bad operator":             {"operator": `"eq"`},
		"window weekday 0":         {"activeWindow": `{"weekdays":[0],"startLocal":"08:00","endLocal":"18:00"}`},
		"window duplicate weekday": {"activeWindow": `{"weekdays":[1,1],"startLocal":"08:00","endLocal":"18:00"}`},
		"window same times":        {"activeWindow": `{"weekdays":[1],"startLocal":"08:00","endLocal":"08:00"}`},
		"window bad time":          {"activeWindow": `{"weekdays":[1],"startLocal":"24:00","endLocal":"08:00"}`},
		"unitIds are not input":    {"unitIds": `["` + uuid.NewString() + `"]`},
		"missing condition":        {"metric": ""},
		"client metric":            {"metric": `"vibration"`},
		"recipient of other org":   {"recipientMembershipIds": `["` + seed.ID("customer-b").String() + `"]`},
		"technician recipient":     {"recipientMembershipIds": `["` + seed.ID("tech-internal-a").String() + `"]`},
		"unknown field":            {"color": `"red"`},
	}
	for name, e := range bad {
		if code, _ := write(s, &customerA, "policies.save", alertPolicyBody(e), 0); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	// overnight window, HQ-only metric and HQ recipient are valid for HQ
	code, m = write(s, &hq, "policies.save", alertPolicyBody(map[string]string{"metric": `"vibration"`,
		"activeWindow": `{"weekdays":[5,6],"startLocal":"22:00","endLocal":"06:00"}`, "recipientMembershipIds": `["` + seed.ID("hq-operator").String() + `","` + seed.ID("customer-a").String() + `"]`}), 0)
	if code != 200 || data(m)["ownerMembershipId"] != seed.ID("hq-operator").String() {
		t.Fatalf("hq create: %d %v", code, m)
	}
	hqPolicy := data(m)["id"].(string)
	if code, _ := write(s, &customerA, "policies.save", alertPolicyBody(map[string]string{"customerId": `"` + seed.ID("cust-b").String() + `"`}), 0); code != 404 {
		t.Error("client saving for another customer → NOT_FOUND")
	}
	if code, _ := write(s, &hq, "policies.save", alertPolicyBody(map[string]string{"customerId": `"` + uuid.NewString() + `"`}), 0); code != 404 {
		t.Error("unknown customer → NOT_FOUND")
	}
	_, dm := post(s, &hq, "policies.get", `{"policyId":"`+def+`"}`)
	defRules, _ := json.Marshal(data(dm)["rules"])
	if code, _ := write(s, &customerA, "policies.save", `{"kind":"default_alert","id":"`+def+`","rules":`+string(defRules)+`}`, ver(dm)); code != 403 {
		t.Error("client cannot edit default limits")
	}
	if code, _ := write(s, &customerA, "policies.save", autoPolicyBody(nil), 0); code != 403 {
		t.Error("client cannot save automation policies")
	}

	// update: version, owner fixed
	upd := func(a *actor, extra map[string]string, v int) (int, map[string]any) {
		extra["id"] = `"` + pid + `"`
		return write(s, a, "policies.save", alertPolicyBody(extra), v)
	}
	if code, m := upd(&customerA, map[string]string{"threshold": "30", "recoveryThreshold": "27"}, 1); code != 200 || data(m)["threshold"].(float64) != 30 || data(m)["version"].(float64) != 2 {
		t.Fatalf("update: %d %v", code, m)
	}
	if code, _ := upd(&customerA, map[string]string{}, 1); code != 409 {
		t.Error("stale version")
	}
	if code, _ := upd(&hq, map[string]string{"customerId": `"` + seed.ID("cust-b").String() + `"`, "recipientMembershipIds": `["` + seed.ID("hq-operator").String() + `"]`}, 2); code != 422 {
		t.Error("owner customer is fixed")
	}
	if code, _ := upd(&hq, map[string]string{"kind": `"alert"`, "id": `"` + def + `"`}, 1); code != 422 && code != 409 {
		t.Errorf("default policy is not an alert policy: %d", code)
	}

	// attach to a unit, then list filters
	unit := seed.ID("unit-non-rto").String()
	_, u := post(s, &customerA, "units.get", `{"id":"`+unit+`"}`)
	if code, m := write(s, &customerA, "units.setAlertPolicies", `{"unitId":"`+unit+`","alertPolicyIds":["`+pid+`"]}`, ver(u)); code != 200 {
		t.Fatalf("attach: %d %v", code, m)
	}
	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(m) {
			out[it["id"].(string)] = true
		}
		return out
	}
	_, m = post(s, &customerA, "policies.list", `{"limit":100}`)
	got := ids(m)
	if !got[def] || !got[pid] || !got[hqPolicy] || items(m)[0]["id"] != def {
		t.Fatalf("client list: default first + own policies: %v", got)
	}
	for _, it := range items(m) {
		if it["kind"] == "automation" || (it["kind"] == "alert" && it["customerId"] != seed.ID("cust-a").String()) {
			t.Fatal("client list leaks")
		}
		if it["id"] == pid && len(it["unitIds"].([]any)) != 1 {
			t.Fatal("derived unitIds")
		}
	}
	_, m = post(s, &customerB, "policies.list", `{"limit":100}`)
	if ids(m)[pid] || !ids(m)[def] {
		t.Fatal("customer-b list")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"unitId":"`+unit+`"},"limit":100}`)
	if got := ids(m); !got[pid] || got[def] || got[hqPolicy] {
		t.Fatalf("unitId filter: %v", got)
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"propertyId":"`+seed.ID("property-home-a").String()+`","kind":"alert"},"limit":100}`)
	if !ids(m)[pid] {
		t.Fatal("propertyId filter")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"customerId":"`+seed.ID("cust-b").String()+`"},"limit":100}`)
	if got := ids(m); got[pid] || !got[def] {
		t.Fatalf("customerId filter: %v", got)
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"kind":"default_alert"}}`)
	if len(items(m)) != 1 {
		t.Fatal("kind filter")
	}
	_, m = post(s, &hq, "policies.list", `{"filters":{"enabled":false,"kind":"alert"}}`)
	if ids(m)[pid] {
		t.Fatal("enabled filter")
	}
	_, m = post(s, &hq, "policies.list", `{"sort":{"field":"name","direction":"desc"},"limit":100}`)
	names := []string{}
	for _, it := range items(m) {
		names = append(names, strings.ToLower(it["name"].(string)))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] < names[i] {
			t.Fatalf("name desc: %v", names)
		}
	}
	for _, b := range []string{`{"filters":{"kind":"air_quality"}}`, `{"filters":{"x":1}}`, `{"sort":{"field":"metric","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "policies.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	for _, sf := range []string{"priority", "updatedAt"} {
		if code, _ := post(s, &hq, "policies.list", `{"sort":{"field":"`+sf+`","direction":"asc"}}`); code != 200 {
			t.Errorf("sort %s: %d", sf, code)
		}
	}
	if code, _ := post(s, &techB, "policies.list", `{}`); code != 403 {
		t.Error("technicians do not read policies")
	}

	// default limits (HQ): fixed keys and order, validated conditions
	_, m = post(s, &hq, "policies.get", `{"policyId":"`+def+`"}`)
	dv := ver(m)
	rules := data(m)["rules"].([]any)
	raw, _ := json.Marshal(rules)
	rules[0].(map[string]any)["threshold"] = 1200.0
	rules[0].(map[string]any)["recoveryThreshold"] = 1000.0
	changed, _ := json.Marshal(rules)
	if code, m := write(s, &hq, "policies.save", `{"kind":"default_alert","id":"`+def+`","rules":`+string(changed)+`}`, dv); code != 200 ||
		data(m)["rules"].([]any)[0].(map[string]any)["threshold"].(float64) != 1200 {
		t.Fatalf("default limits: %d %v", code, m)
	}
	dv++
	swapped := strings.Replace(string(raw), `"ruleKey":"ventilation_co2"`, `"ruleKey":"dust_pm25"`, 1)
	rules[1].(map[string]any)["metric"] = "temperature"
	wrongMetric, _ := json.Marshal(rules)
	for name, body := range map[string]string{"reordered keys": swapped, "five rules": "[" + strings.Join(strings.SplitN(string(raw)[1:], "},", 6)[:5], "},") + "}]",
		"metric changed": string(wrongMetric)} {
		if code, _ := write(s, &hq, "policies.save", `{"kind":"default_alert","id":"`+def+`","rules":`+body+`}`, dv); code != 422 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if code, _ := write(s, &hq, "policies.save", `{"kind":"default_alert","id":"`+pid+`","rules":`+string(raw)+`}`, 3); code != 422 {
		t.Error("alert policy saved as default")
	}
	if code, _ := write(s, &hq, "policies.save", `{"kind":"default_alert","id":"`+def+`","rules":`+string(raw)+`}`, dv-1); code != 409 {
		t.Error("default stale version")
	}
	write(s, &hq, "policies.save", `{"kind":"default_alert","id":"`+def+`","rules":`+string(raw)+`}`, dv) // restore

	// setDefaultRule: version 0 for a rule without a stored setting, owner only for clients
	sdr := func(a *actor, cust, key string, enabled bool, v int) (int, map[string]any) {
		e := "false"
		if enabled {
			e = "true"
		}
		return write(s, a, "policies.setDefaultRule", `{"policyId":"`+def+`","ruleKey":"`+key+`","customerId":"`+seed.ID(cust).String()+`","enabled":`+e+`,"reason":" test "}`, v)
	}
	owner(t, `DELETE FROM monitoring.default_rule_settings WHERE customer_id = $1`, seed.ID("cust-b"))
	if code, m := sdr(&customerB, "cust-b", "refrigerant_low_pressure", false, versionZero); code != 200 || data(m)["version"].(float64) != 1 || data(m)["reason"] != "test" {
		t.Fatalf("first switch: %d %v", code, m)
	}
	if code, _ := sdr(&customerB, "cust-b", "refrigerant_low_pressure", true, versionZero); code != 409 {
		t.Error("stale setting version")
	}
	if code, m := sdr(&hq, "cust-b", "refrigerant_low_pressure", true, 1); code != 200 || data(m)["enabled"] != true || data(m)["version"].(float64) != 2 {
		t.Fatalf("hq switch: %d", code)
	}
	_, m = post(s, &customerA, "policies.get", `{"policyId":"`+def+`"}`)
	for _, rs := range data(m)["ruleSettings"].([]any) {
		if rs.(map[string]any)["ruleKey"] == "refrigerant_low_pressure" {
			t.Fatal("customer-a unchanged by customer-b's switch")
		}
	}
	if code, _ := sdr(&customerB, "cust-a", "dust_pm25", false, versionZero); code != 404 {
		t.Error("client switches only its own customer")
	}
	if code, _ := write(s, &hq, "policies.setDefaultRule", `{"policyId":"`+def+`","ruleKey":"noise","customerId":"`+seed.ID("cust-b").String()+`","enabled":true}`, versionZero); code != 422 {
		t.Error("unknown rule key")
	}
	if code, _ := write(s, &hq, "policies.setDefaultRule", `{"policyId":"`+pid+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").String()+`","enabled":true}`, versionZero); code != 422 {
		t.Error("not the default policy")
	}
	if code, _ := write(s, &hq, "policies.setDefaultRule", `{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+uuid.NewString()+`","enabled":true}`, versionZero); code != 404 {
		t.Error("unknown customer")
	}
	if code, _ := write(s, &hq, "policies.setDefaultRule", `{"policyId":"`+uuid.NewString()+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").String()+`","enabled":true}`, versionZero); code != 404 {
		t.Error("unknown policy")
	}
	if code, _ := write(s, &hq, "policies.setDefaultRule", `{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").String()+`","enabled":true,"reason":"`+strings.Repeat("r", 501)+`"}`, versionZero); code != 422 {
		t.Error("reason too long")
	}
	owner(t, `UPDATE identity.memberships SET client_role = 'member' WHERE id = $1`, seed.ID("customer-b"))
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`, seed.ID("customer-b"))
	})
	if code, _ := sdr(&customerB, "cust-b", "dust_pm25", false, versionZero); code != 403 {
		t.Errorf("member → FORBIDDEN, got %d", code)
	}
	owner(t, `UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`, seed.ID("customer-b"))

	// delete: default fixed, detach from units, version
	if code, m := write(s, &hq, "policies.delete", `{"policyId":"`+def+`"}`, dv+1); code != 422 {
		t.Errorf("default policy cannot be deleted: %d %v", code, m)
	}
	if code, _ := write(s, &customerA, "policies.delete", `{"policyId":"`+pid+`"}`, 1); code != 409 {
		t.Error("delete stale version")
	}
	if code, _ := write(s, &customerB, "policies.delete", `{"policyId":"`+pid+`"}`, 2); code != 404 {
		t.Error("other customer delete")
	}
	uv := ver(u) + 1
	if code, m := write(s, &customerA, "policies.delete", `{"policyId":"`+pid+`"}`, 2); code != 200 || data(m)["deleted"] != true {
		t.Fatalf("delete: %d %v", code, m)
	}
	_, u = post(s, &customerA, "units.get", `{"id":"`+unit+`"}`)
	if len(data(u)["alertPolicyIds"].([]any)) != 0 || ver(u) != uv+1 {
		t.Fatalf("unit detached with a new version: %v %d", data(u)["alertPolicyIds"], ver(u))
	}
	if code, _ := post(s, &customerA, "policies.get", `{"policyId":"`+pid+`"}`); code != 404 {
		t.Error("deleted policy")
	}
	if code, _ := write(s, &hq, "policies.delete", `{"policyId":"`+hqPolicy+`"}`, 1); code != 200 {
		t.Error("delete unattached policy")
	}
}
