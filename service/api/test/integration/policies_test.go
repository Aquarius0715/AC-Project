package integration_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const fieldChannels = "channels"

// alertPolicyBody builds an alert PolicyInput with overrides (raw JSON values).
func alertPolicyBody(extra map[string]string) string {
	f := map[string]string{
		fieldKind:     `"alert"`,
		fieldName:     `"Server room too warm"`,
		fieldTimezone: jsonKualaLumpur,
		fieldEnabled:  jsonTrue,
		fieldPriority: "50",
		fieldCustomerID: `"` + seed.ID("cust-a").
			String() +
			`"`,
		fieldMetric:                 `"temperature"`,
		fieldOperator:               `"gte"`,
		fieldThreshold:              "28",
		fieldRecoveryThreshold:      "26",
		fieldDurationSeconds:        "300",
		fieldActiveWindow:           jsonNull,
		fieldSeverity:               `"warning"`,
		fieldRecipientMembershipIDs: `["` + seed.ID("customer-a").String() + `"]`,
		fieldChannels:               `["inApp","email"]`,
		"escalateAfterMinutes":      "60",
		"cooldownMinutes":           "5",
	}

	for key, value := range extra {
		if value == "" {
			delete(f, key)

			continue
		}

		f[key] = value
	}

	parts := make([]string, 0, len(f))
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPolicies(t *testing.T) {
	srv := server(t)
	def := seed.DefaultPolicyID("tenant-a").String()

	owner(
		t,
		`DELETE FROM monitoring.default_rule_settings WHERE customer_id = $1`,
		seed.ID("cust-b"),
	)

	// create (client owner) and read back
	code, resp := write(
		srv,
		customerA(),
		"policies.save",
		alertPolicyBody(map[string]string{fieldName: `"  Warm room  "`}),
		0,
	)
	if code != 200 || data(resp)["kind"] != "alert" || data(resp)["name"] != "Warm room" ||
		data(resp)["metric"] != metricTemperature ||
		len(asArr(t, data(resp)["unitIds"])) != 0 {
		t.Fatalf("create: %d %v", code, resp)
	}

	pid := asStr(t, data(resp)["id"])
	if code, res := post(
		srv,
		customerA(),
		"policies.get",
		`{"policyId":"`+pid+`"}`,
	); code != 200 ||
		data(res)["customerId"] != seed.ID("cust-a").String() {
		t.Fatalf("get: %d %v", code, res)
	}

	if code, _ := post(srv, customerB(), "policies.get", `{"policyId":"`+pid+`"}`); code != 404 {
		t.Error("customer-b must not read customer-a's policy")
	}
	// default policy: clients see it with only their own settings; HQ sees all settings
	_, resp = post(srv, customerA(), "policies.get", `{"policyId":"`+def+`"}`)
	if len(asArr(t, data(resp)["rules"])) != 6 || data(resp)["customerId"] != nil {
		t.Fatalf("default policy: %v", resp)
	}

	for _, rs := range asArr(t, data(resp)["ruleSettings"]) {
		if asObj(t, rs)["customerId"] != seed.ID("cust-a").String() {
			t.Fatal("client sees another customer's rule setting")
		}
	}

	_, resp = post(srv, customerB(), "policies.get", `{"policyId":"`+def+`"}`)
	if len(asArr(t, data(resp)["ruleSettings"])) != 0 {
		t.Fatal("customer-b sees customer-a's setting")
	}

	// validation
	bad := map[string]map[string]string{
		"recovery direction gte": {fieldRecoveryThreshold: "28"},
		"recovery direction lte": {fieldOperator: `"lte"`, fieldRecoveryThreshold: "27"},
		caseZeroDuration:         {fieldDurationSeconds: "0"},
		"duration > 86400":       {fieldDurationSeconds: "86401"},
		"no recipients":          {fieldRecipientMembershipIDs: "[]"},
		"channels without inApp": {fieldChannels: `["email"]`},
		"duplicate channel":      {fieldChannels: `["inApp","inApp"]`},
		"bad channel":            {fieldChannels: `["inApp","sms"]`},
		caseBlankName:            {fieldName: `"   "`},
		"name too long":          {fieldName: `"` + strings.Repeat("n", 121) + `"`},
		casePriorityOver100:      {fieldPriority: "101"},
		"bad timezone":           {fieldTimezone: `"Mars/Base"`},
		"cooldown 0":             {"cooldownMinutes": "0"},
		"escalate 1441":          {"escalateAfterMinutes": "1441"},
		"bad severity":           {fieldSeverity: `"info"`},
		caseUnknownMetric:        {fieldMetric: `"noise"`},
		caseBadOperator:          {fieldOperator: `"eq"`},
		"window weekday 0": {
			fieldActiveWindow: `{"weekdays":[0],"startLocal":"08:00","endLocal":"18:00"}`,
		},
		"window duplicate weekday": {
			fieldActiveWindow: `{"weekdays":[1,1],"startLocal":"08:00","endLocal":"18:00"}`,
		},
		"window same times": {
			fieldActiveWindow: `{"weekdays":[1],"startLocal":"08:00","endLocal":"08:00"}`,
		},
		"window bad time": {
			fieldActiveWindow: `{"weekdays":[1],"startLocal":"24:00","endLocal":"08:00"}`,
		},
		"unitIds are not input": {fieldUnitIDs: `["` + uuid.NewString() + `"]`},
		"missing condition":     {fieldMetric: ""},
		"client metric":         {fieldMetric: `"vibration"`},
		"recipient of other org": {
			fieldRecipientMembershipIDs: `["` + seed.ID("customer-b").String() + `"]`,
		},
		"technician recipient": {
			fieldRecipientMembershipIDs: `["` + seed.ID("tech-internal-a").String() + `"]`,
		},
		caseUnknownField: {"color": `"red"`},
	}
	// the field each case is refused for
	want := map[string]string{
		"recovery direction gte":   "recoveryThreshold=error.recoveryDirection",
		"recovery direction lte":   "recoveryThreshold=error.recoveryDirection",
		caseZeroDuration:           "durationSeconds=error.range",
		"duration > 86400":         "durationSeconds=error.range",
		"no recipients":            "recipientMembershipIds=error.invalid",
		"channels without inApp":   refusedChannelsInvalid,
		"duplicate channel":        refusedChannelsInvalid,
		"bad channel":              refusedChannelsInvalid,
		caseBlankName:              refusedNameLength,
		"name too long":            refusedNameLength,
		casePriorityOver100:        refusedPriorityRange,
		"bad timezone":             refusedTimezoneInvalid,
		"cooldown 0":               "cooldownMinutes=error.range",
		"escalate 1441":            "escalateAfterMinutes=error.range",
		"bad severity":             "severity=error.invalid",
		caseUnknownMetric:          refusedMetricInvalid,
		caseBadOperator:            "operator=error.invalid",
		"window weekday 0":         "activeWindow.weekdays=error.invalid",
		"window duplicate weekday": "activeWindow.weekdays=error.invalid",
		"window same times":        "activeWindow=error.window",
		"window bad time":          "activeWindow=error.window",
		"unitIds are not input":    "unitIds=error.unitsAttachOnUnit",
		"missing condition":        "condition=error.required",
		"client metric":            refusedMetricInvalid,
		"recipient of other org":   "recipientMembershipIds=error.recipientCannotRead",
		"technician recipient":     "recipientMembershipIds=error.recipientCannotRead",
		caseUnknownField:           refusedMalformedInput,
	}
	for name, extra := range bad {
		field, key, _ := strings.Cut(want[name], "=")
		if code, res := write(
			srv,
			customerA(),
			"policies.save",
			alertPolicyBody(extra),
			0,
		); code != 422 ||
			asObj(t, res["fieldErrors"])[field] != key {
			t.Errorf("%s: %d %v, want %s", name, code, res["fieldErrors"], want[name])
		}
	}
	// overnight window, HQ-only metric and HQ recipient are valid for HQ
	code, resp = write(srv, hq(), "policies.save", alertPolicyBody(map[string]string{
		fieldMetric:       `"vibration"`,
		fieldActiveWindow: `{"weekdays":[5,6],"startLocal":"22:00","endLocal":"06:00"}`,
		fieldRecipientMembershipIDs: `["` + seed.ID("hq-operator").
			String() +
			`","` + seed.ID("customer-a").
			String() +
			`"]`,
	}), 0)
	if code != 200 || data(resp)["ownerMembershipId"] != seed.ID("hq-operator").String() {
		t.Fatalf("hq create: %d %v", code, resp)
	}

	hqPolicy := asStr(t, data(resp)["id"])

	if code, _ := write(
		srv,
		customerA(),
		"policies.save",
		alertPolicyBody(map[string]string{fieldCustomerID: `"` + seed.ID("cust-b").String() + `"`}),
		0,
	); code != 404 {
		t.Error("client saving for another customer → NOT_FOUND")
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.save",
		alertPolicyBody(map[string]string{fieldCustomerID: `"` + uuid.NewString() + `"`}),
		0,
	); code != 404 {
		t.Error("unknown customer → NOT_FOUND")
	}

	_, defaultPolicy := post(srv, hq(), "policies.get", `{"policyId":"`+def+`"}`)

	defRules := jsonBytes(t, data(defaultPolicy)["rules"])
	if code, _ := write(
		srv,
		customerA(),
		"policies.save",
		`{"kind":"default_alert","id":"`+def+`","rules":`+string(defRules)+`}`,
		ver(t, defaultPolicy),
	); code != 403 {
		t.Error("client cannot edit default limits")
	}

	if code, _ := write(srv, customerA(), "policies.save", autoPolicyBody(nil), 0); code != 403 {
		t.Error("client cannot save automation policies")
	}

	// update: version, owner fixed
	upd := func(a *actor, extra map[string]string, v int) (int, map[string]any) {
		extra["id"] = `"` + pid + `"`

		return write(srv, a, "policies.save", alertPolicyBody(extra), v)
	}
	if code, res := upd(
		customerA(),
		map[string]string{fieldThreshold: "30", fieldRecoveryThreshold: "27"},
		1,
	); code != 200 || asNum(t, data(res)["threshold"]) != 30 ||
		asNum(t, data(res)["version"]) != 2 {
		t.Fatalf("update: %d %v", code, res)
	}

	if code, _ := upd(customerA(), map[string]string{}, 1); code != 409 {
		t.Error("stale version")
	}

	if code, res := upd(
		hq(),
		map[string]string{
			fieldCustomerID:             `"` + seed.ID("cust-b").String() + `"`,
			fieldRecipientMembershipIDs: `["` + seed.ID("hq-operator").String() + `"]`,
		},
		2,
	); code != 422 ||
		fields(res) != "customerId=error.ownerFixed" {
		t.Errorf("owner customer is fixed: %d %s", code, fields(res))
	}
	// the default policy cannot become an alert policy (upd always targets the customer's policy: it tested that one,
	// stale)
	_, policyBefore := post(srv, hq(), "policies.get", `{"policyId":"`+def+`"}`)
	if code, res := write(
		srv,
		hq(),
		"policies.save",
		alertPolicyBody(map[string]string{"id": `"` + def + `"`}),
		ver(t, policyBefore),
	); code != 422 ||
		fields(res) != refusedKindFixed {
		t.Errorf("default policy is not an alert policy: %d %s", code, fields(res))
	}

	// attach to a unit, then list filters
	unit := seed.ID("unit-non-rto").String()

	_, detail := post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)
	if code, res := write(
		srv,
		customerA(),
		"units.setAlertPolicies",
		`{"unitId":"`+unit+`","alertPolicyIds":["`+pid+`"]}`,
		ver(t, detail),
	); code != 200 {
		t.Fatalf("attach: %d %v", code, res)
	}

	ids := func(m map[string]any) map[string]bool {
		out := map[string]bool{}
		for _, it := range items(t, m) {
			out[asStr(t, it["id"])] = true
		}

		return out
	}
	_, resp = post(srv, customerA(), "policies.list", `{"limit":100}`)

	got := ids(resp)
	if !got[def] || !got[pid] || !got[hqPolicy] || items(t, resp)[0]["id"] != def {
		t.Fatalf("client list: default first + own policies: %v", got)
	}

	for _, item := range items(t, resp) {
		if item["kind"] == kindAutomation ||
			(item["kind"] == "alert" && item["customerId"] != seed.ID("cust-a").String()) {
			t.Fatal("client list leaks")
		}

		if item["id"] == pid && len(asArr(t, item["unitIds"])) != 1 {
			t.Fatal("derived unitIds")
		}
	}

	_, resp = post(srv, customerB(), "policies.list", `{"limit":100}`)
	if ids(resp)[pid] || !ids(resp)[def] {
		t.Fatal("customer-b list")
	}

	_, resp = post(srv, hq(), "policies.list", `{"filters":{"unitId":"`+unit+`"},"limit":100}`)
	if got := ids(resp); !got[pid] || got[def] || got[hqPolicy] {
		t.Fatalf("unitId filter: %v", got)
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"propertyId":"`+seed.ID("property-home-a").
			String()+
			`","kind":"alert"},"limit":100}`,
	)
	if !ids(resp)[pid] {
		t.Fatal("propertyId filter")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"filters":{"customerId":"`+seed.ID("cust-b").String()+`"},"limit":100}`,
	)
	if got := ids(resp); got[pid] || !got[def] {
		t.Fatalf("customerId filter: %v", got)
	}

	_, resp = post(srv, hq(), "policies.list", `{"filters":{"kind":"default_alert"}}`)
	if len(items(t, resp)) != 1 {
		t.Fatal("kind filter")
	}

	_, resp = post(srv, hq(), "policies.list", `{"filters":{"enabled":false,"kind":"alert"}}`)
	if ids(resp)[pid] {
		t.Fatal("enabled filter")
	}

	_, resp = post(
		srv,
		hq(),
		"policies.list",
		`{"sort":{"field":"name","direction":"desc"},"limit":100}`,
	)

	rows := items(t, resp)

	names := make([]string, 0, len(rows))
	for _, it := range rows {
		names = append(names, strings.ToLower(asStr(t, it["name"])))
	}

	for i := 1; i < len(names); i++ {
		if names[i-1] < names[i] {
			t.Fatalf("name desc: %v", names)
		}
	}

	want = map[string]string{ // the field each case is refused for
		"{\"filters\":{\"kind\":\"air_quality\"}}":                refusedFilterKindInvalid,
		bodyUnknownFilter:                                         refusedUnknownFieldX,
		"{\"sort\":{\"field\":\"metric\",\"direction\":\"asc\"}}": refusedSortFieldInvalid,
	}

	for _, b := range []string{`{"filters":{"kind":"air_quality"}}`, bodyUnknownFilter, `{"sort":{"field":"metric",` +
		`"direction":"asc"}}`} {
		code, m := post(srv, hq(), "policies.list", b)
		refusal(t, b, code, m, want[b])
	}

	for _, sortField := range []string{fieldPriority, "updatedAt"} {
		if code, _ := post(
			srv,
			hq(),
			"policies.list",
			`{"sort":{"field":"`+sortField+`","direction":"asc"}}`,
		); code != 200 {
			t.Errorf("sort %s: %d", sortField, code)
		}
	}

	if code, _ := post(srv, techB(), "policies.list", `{}`); code != 403 {
		t.Error("technicians do not read policies")
	}

	// default limits (HQ): fixed keys and order, validated conditions
	_, resp = post(srv, hq(), "policies.get", `{"policyId":"`+def+`"}`)
	defaultVersion := ver(t, resp)
	rules := asArr(t, data(resp)["rules"])
	raw := jsonBytes(t, rules)
	asObj(t, rules[0])["threshold"] = 1200.0
	asObj(t, rules[0])["recoveryThreshold"] = 1000.0

	changed := jsonBytes(t, rules)
	if code, res := write(
		srv,
		hq(),
		"policies.save",
		`{"kind":"default_alert","id":"`+def+`","rules":`+string(changed)+`}`,
		defaultVersion,
	); code != 200 ||
		asNum(t, asObj(t, asArr(t, data(res)["rules"])[0])["threshold"]) != 1200 {
		t.Fatalf("default limits: %d %v", code, res)
	}

	defaultVersion++
	swapped := strings.Replace(
		string(raw),
		`"ruleKey":"ventilation_co2"`,
		`"ruleKey":"dust_pm25"`,
		1,
	)
	asObj(t, rules[1])["metric"] = metricTemperature
	wrongMetric := jsonBytes(t, rules)
	want = map[string]string{ // the field each case is refused for
		"five rules":     "rules=error.fixedRules",
		"metric changed": "rules.1.metric=error.invalid",
		"reordered keys": "rules=error.fixedRules",
	}

	for name, body := range map[string]string{
		"reordered keys": swapped, "five rules": "[" + strings.Join(strings.SplitN(string(raw)[1:], "},", 6)[:5], "},") +
			"}]",
		"metric changed": string(wrongMetric),
	} {
		code, res := write(
			srv,
			hq(),
			"policies.save",
			`{"kind":"default_alert","id":"`+def+`","rules":`+body+`}`,
			defaultVersion,
		)
		refusal(t, name, code, res, want[name])
	}

	if code, res := write(
		srv,
		hq(),
		"policies.save",
		`{"kind":"default_alert","id":"`+pid+`","rules":`+string(raw)+`}`,
		3,
	); code != 422 ||
		fields(res) != refusedKindFixed {
		t.Errorf("alert policy saved as default: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.save",
		`{"kind":"default_alert","id":"`+def+`","rules":`+string(raw)+`}`,
		defaultVersion-1,
	); code != 409 {
		t.Error("default stale version")
	}

	write(
		srv,
		hq(),
		"policies.save",
		`{"kind":"default_alert","id":"`+def+`","rules":`+string(raw)+`}`,
		defaultVersion,
	) // restore

	// setDefaultRule: version 0 for a rule without a stored setting, owner only for clients
	sdr := func(who *actor, cust, key string, enabled bool, version int) (int, map[string]any) {
		enabledJSON := jsonFalse
		if enabled {
			enabledJSON = jsonTrue
		}

		return write(
			srv,
			who,
			"policies.setDefaultRule",
			`{"policyId":"`+def+`","ruleKey":"`+key+`","customerId":"`+seed.ID(cust).
				String()+
				`","enabled":`+enabledJSON+`,"reason":" test "}`,
			version,
		)
	}

	owner(
		t,
		`DELETE FROM monitoring.default_rule_settings WHERE customer_id = $1`,
		seed.ID("cust-b"),
	)

	if code, res := sdr(
		customerB(),
		"cust-b",
		"refrigerant_low_pressure",
		false,
		versionZero,
	); code != 200 || asNum(t, data(res)["version"]) != 1 ||
		data(res)["reason"] != "test" {
		t.Fatalf("first switch: %d %v", code, res)
	}

	if code, _ := sdr(
		customerB(),
		"cust-b",
		"refrigerant_low_pressure",
		true,
		versionZero,
	); code != 409 {
		t.Error("stale setting version")
	}

	if code, res := sdr(
		hq(),
		"cust-b",
		"refrigerant_low_pressure",
		true,
		1,
	); code != 200 || data(res)["enabled"] != true ||
		asNum(t, data(res)["version"]) != 2 {
		t.Fatalf("hq switch: %d", code)
	}

	_, resp = post(srv, customerA(), "policies.get", `{"policyId":"`+def+`"}`)
	for _, rs := range asArr(t, data(resp)["ruleSettings"]) {
		if asObj(t, rs)["ruleKey"] == "refrigerant_low_pressure" {
			t.Fatal("customer-a unchanged by customer-b's switch")
		}
	}

	if code, _ := sdr(customerB(), "cust-a", "dust_pm25", false, versionZero); code != 404 {
		t.Error("client switches only its own customer")
	}

	if code, res := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+def+`","ruleKey":"noise","customerId":"`+seed.ID("cust-b").
			String()+
			`","enabled":true}`,
		versionZero,
	); code != 422 ||
		fields(res) != "ruleKey=error.invalid" {
		t.Errorf("unknown rule key: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+pid+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").
			String()+
			`","enabled":true}`,
		versionZero,
	); code != 422 ||
		fields(res) != "policyId=error.notDefaultPolicy" {
		t.Errorf("not the default policy: %d %s", code, fields(res))
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+uuid.NewString()+`","enabled":true}`,
		versionZero,
	); code != 404 {
		t.Error("unknown customer")
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+uuid.NewString()+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").
			String()+
			`","enabled":true}`,
		versionZero,
	); code != 404 {
		t.Error("unknown policy")
	}

	if code, res := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").
			String()+
			`","enabled":true,"reason":"`+strings.Repeat(
			"r",
			501,
		)+`"}`,
		versionZero,
	); code != 422 ||
		fields(res) != refusedReasonLength {
		t.Errorf("reason too long: %d %s", code, fields(res))
	}

	owner(
		t,
		`UPDATE identity.memberships SET client_role = 'member' WHERE id = $1`,
		seed.ID("customer-b"),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`,
			seed.ID("customer-b"),
		)
	})

	if code, _ := sdr(customerB(), "cust-b", "dust_pm25", false, versionZero); code != 403 {
		t.Errorf("member → FORBIDDEN, got %d", code)
	}

	owner(
		t,
		`UPDATE identity.memberships SET client_role = 'owner' WHERE id = $1`,
		seed.ID("customer-b"),
	)
	// a switch without a reason records none
	if code, res := write(
		srv,
		hq(),
		"policies.setDefaultRule",
		`{"policyId":"`+def+`","ruleKey":"dust_pm25","customerId":"`+seed.ID("cust-b").
			String()+
			`","enabled":false}`,
		versionZero,
	); code != 200 ||
		data(res)["reason"] != nil {
		t.Errorf("switch without a reason: %d %v", code, res)
	}

	// delete: default fixed, detach from units, version
	if code, res := write(
		srv,
		hq(),
		"policies.delete",
		`{"policyId":"`+def+`"}`,
		defaultVersion+1,
	); code != 422 ||
		fields(res) != "policyId=error.defaultPolicyFixed" {
		t.Errorf("default policy cannot be deleted: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		customerA(),
		"policies.delete",
		`{"policyId":"`+pid+`"}`,
		1,
	); code != 409 {
		t.Error("delete stale version")
	}

	if code, _ := write(
		srv,
		customerB(),
		"policies.delete",
		`{"policyId":"`+pid+`"}`,
		2,
	); code != 404 {
		t.Error("other customer delete")
	}

	unitVersion := ver(t, detail) + 1

	if code, res := write(
		srv,
		customerA(),
		"policies.delete",
		`{"policyId":"`+pid+`"}`,
		2,
	); code != 200 ||
		data(res)["deleted"] != true {
		t.Fatalf("delete: %d %v", code, res)
	}

	_, detail = post(srv, customerA(), "units.get", `{"id":"`+unit+`"}`)
	if len(asArr(t, data(detail)["alertPolicyIds"])) != 0 || ver(t, detail) != unitVersion+1 {
		t.Fatalf("unit detached with a new version: %v %d", data(detail)["alertPolicyIds"], ver(t, detail))
	}

	if code, _ := post(srv, customerA(), "policies.get", `{"policyId":"`+pid+`"}`); code != 404 {
		t.Error("deleted policy")
	}

	if code, _ := write(
		srv,
		hq(),
		"policies.delete",
		`{"policyId":"`+hqPolicy+`"}`,
		1,
	); code != 200 {
		t.Error("delete unattached policy")
	}
}

// TestPolicySaveRefusals covers two refusals of policies.save: a customer that is no longer active cannot get an alert
// policy, and the kind of a saved policy is fixed (an automation policy cannot be saved as an alert policy).
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPolicySaveRefusals(t *testing.T) {
	srv := server(t)
	owner(t, `UPDATE assets.customers SET status = 'inactive' WHERE id = $1`, seed.ID("cust-a"))

	code, res := write(srv, hq(), "policies.save", alertPolicyBody(nil), 0)

	owner(t, `UPDATE assets.customers SET status = 'active' WHERE id = $1`, seed.ID("cust-a"))

	if code != 422 ||
		asObj(t, res["fieldErrors"])["customerId"] != apperr.KeyInactiveCustomer {
		t.Errorf("inactive customer: %d %v", code, res)
	}

	code, res = write(srv, hq(), "policies.save", autoPolicyBody(nil), 0)
	if code != 200 {
		t.Fatalf("automation policy: %d %v", code, res)
	}

	policyID, version := asStr(t, data(res)["id"]), ver(t, res)
	if code, resp := write(
		srv,
		hq(),
		"policies.save",
		alertPolicyBody(map[string]string{"id": `"` + policyID + `"`}),
		version,
	); code != 422 ||
		asObj(t, resp["fieldErrors"])["kind"] != apperr.KeyKindFixed {
		t.Errorf("alert save over an automation policy: %d %v", code, resp)
	}

	owner(t, `DELETE FROM control.automation_policies WHERE id = $1`, policyID)
}
