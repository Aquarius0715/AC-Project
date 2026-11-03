package integration_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func newAlert(t *testing.T, unit, org, severity string, when time.Time) string {
	t.Helper()

	id := uuid.NewString()
	// one open alert per unit / policy / rule / type (alerts_one_open_per_rule): use the severity as the rule key
	owner(
		t,
		`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, rule_key, type, severity, status, cause_code,
			evidence_kind, evidence_text, observed_at, detected_at)
		VALUES ($1,$2,$3,$4,$5,'sensor',$5,'open','unknown','demo_observation','demo',$6,$6)`,
		id,
		seed.ID("tenant-a"),
		unit,
		org,
		severity,
		when,
	)

	return id
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAlerts(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Alert AC")
	org := seed.ID("org-customer-b").String()
	crit := newAlert(t, unit, org, "critical", clock().Add(-10*time.Minute))
	warn := newAlert(t, unit, org, "warning", clock().Add(-5*time.Minute))
	norm := newAlert(t, unit, org, "normal", clock().Add(-time.Minute))

	ids := func(m map[string]any) []string {
		rows := items(t, m)

		out := make([]string, 0, len(rows))
		for _, it := range rows {
			out = append(out, asStr(t, it["id"]))
		}

		return out
	}

	_, resp := post(srv, hq(), "alerts.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if got := ids(resp); strings.Join(got, ",") != strings.Join([]string{crit, warn, norm}, ",") {
		t.Fatalf("default severity desc: %v", got)
	}

	_, resp = post(
		srv,
		hq(),
		"alerts.list",
		`{"filters":{"unitId":"`+unit+`"},"sort":{"field":"severity","direction":"asc"}}`,
	)
	if got := ids(resp); got[0] != norm || got[2] != crit {
		t.Fatalf("severity asc: %v", got)
	}

	for body, n := range map[string]int{
		`{"filters":{"unitIds":["` + unit + `"],"severity":"warning"}}`: 1,
		`{"filters":{"unitId":"` + unit + `","status":"open"}}`:         3,
		`{"filters":{"unitId":"` + unit + `","from":"` + clock().Add(-6*time.Minute).Format(time.RFC3339) + `","to":"` +
			clock().Format(time.RFC3339) + `"}}`: 2,
		`{"filters":{"unitId":"` + unit + `","customerId":"` + seed.ID("cust-b").String() +
			`"}}`: 3,
		`{"filters":{"propertyId":"` + seed.ID("property-home-b").String() + `","unitId":"` + unit +
			`"}}`: 3,
		`{"filters":{"unitId":"` + unit + `","customerId":"` + uuid.NewString() +
			`"}}`: 0,
	} {
		if code, m := post(srv, hq(), "alerts.list", body); code != 200 || len(items(t, m)) != n {
			t.Errorf("%s: %d want %d", body, len(items(t, m)), n)
		}
	}

	want := map[string]string{ // the field each case is refused for
		bodyReversedPeriod:        refusedFilterToRange,
		bodyInfoSeverity:          "filters.severity=error.invalid",
		bodyUnknownStatus:         refusedFilterStatusInvalid,
		"{\"filters\":{\"z\":1}}": "z=error.notAllowed",
		"{\"sort\":{\"field\":\"title\",\"direction\":\"asc\"}}": refusedSortFieldInvalid,
	}

	for _, b := range []string{
		bodyInfoSeverity, bodyUnknownStatus,
		bodyReversedPeriod, `{"filters":{"z":1}}`,
		`{"sort":{"field":"title","direction":"asc"}}`,
	} {
		code, m := post(srv, hq(), "alerts.list", b)
		refusal(t, b, code, m, want[b])
	}
	// scope: customer-b sees, customer-a does not; units.list shows the active count
	if code, _ := post(srv, customerB(), "alerts.get", `{"id":"`+crit+`"}`); code != 200 {
		t.Error("customer-b reads its alert")
	}

	if code, _ := post(srv, customerA(), "alerts.get", `{"id":"`+crit+`"}`); code != 404 {
		t.Error("customer-a must not read customer-b's alert")
	}

	_, g := post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	if asNum(t, data(g)["activeAlertCount"]) != 2 { // IR51: normal alerts are not counted
		t.Fatalf("activeAlertCount: %v", data(g)["activeAlertCount"])
	}
	// acknowledge → resolve, with state and version rules
	if code, res := write(
		srv,
		hq(),
		"alerts.acknowledge",
		`{"alertId":"`+crit+`"}`,
		1,
	); code != 200 || data(res)["status"] != statusAcknowledged ||
		data(res)["acknowledgedAt"] == nil {
		t.Fatalf("ack: %d %v", code, res)
	}

	if code, _ := write(srv, hq(), "alerts.acknowledge", `{"alertId":"`+crit+`"}`, 2); code != 409 {
		t.Error("ack twice")
	}

	if code, res := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+crit+`","resolutionReason":"   ","resolutionEvidenceIds":[]}`,
		2,
	); code != 422 ||
		fields(res) != "resolutionReason=error.length" {
		t.Errorf("blank reason: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+crit+`","resolutionReason":"`+strings.Repeat(
			"r",
			1001,
		)+`","resolutionEvidenceIds":[]}`,
		2,
	); code != 422 ||
		fields(res) != "resolutionReason=error.length" {
		t.Errorf("reason too long: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+crit+`","resolutionReason":"fixed"}`,
		2,
	); code != 422 ||
		fields(res) != "resolutionEvidenceIds=error.required" {
		t.Errorf("evidence ids required: %d %s", code, fields(res))
	}
	// an alert without a policy cites a candidate (IR66, IR327): here a remeasurement after detection
	back := reading(t, unit, "temperature", 25, "°C", clock().Add(-2*time.Minute), "valid")
	if code, res := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+crit+`","resolutionReason":" window closed ","resolutionEvidenceIds":["`+back+`"]}`,
		2,
	); code != 200 ||
		data(res)["resolutionReason"] != "window closed" {
		t.Fatalf("resolve: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+crit+`","resolutionReason":"again","resolutionEvidenceIds":[]}`,
		3,
	); code != 409 {
		t.Error("resolve twice")
	}

	if code, res := write(
		srv,
		hq(),
		"alerts.resolve",
		`{"alertId":"`+warn+`","resolutionReason":"direct","resolutionEvidenceIds":["`+back+`"]}`,
		1,
	); code != 200 ||
		data(res)["status"] != statusResolved {
		t.Fatalf("resolve from open: %d %v", code, res)
	}

	if code, _ := write(srv, hq(), "alerts.acknowledge", `{"alertId":"`+norm+`"}`, 5); code != 409 {
		t.Error("stale version")
	}

	if code, _ := write(
		srv,
		customerB(),
		"alerts.acknowledge",
		`{"alertId":"`+norm+`"}`,
		1,
	); code != 403 {
		t.Error("clients do not acknowledge")
	}
	// technician without an assignment on the unit
	if code, _ := write(
		srv,
		techInt(),
		"alerts.acknowledge",
		`{"alertId":"`+norm+`"}`,
		1,
	); code != 404 {
		t.Error("technician outside scope → NOT_FOUND")
	}

	_, g = post(srv, hq(), "units.get", `{"id":"`+unit+`"}`)
	if asNum(t, data(g)["activeAlertCount"]) != 0 { // only the normal alert is left open (IR51)
		t.Fatal("resolved alerts are not active")
	}
}

// IR284: every reader of an alert sees the condition that raised it — its policy's own, or the default rule of its rule
// key — and none for an alert without a policy; a technician on the unit acknowledges with alert.read, and only
// alert.resolve resolves.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestAlertRuleAndTechnicianAcknowledge(t *testing.T) {
	srv := server(t)
	unit := newUnit(t, srv, "Rule AC")
	org := seed.ID("org-customer-b").String()
	policy := uuid.NewString()
	owner(
		t,
		`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id,
			created_by_user_id, timezone, condition)
		VALUES ($1, $2, 'alert', $3, 'Bedroom too hot', $4, $4, 'Asia/Kuala_Lumpur',
		'{"metric":"temperature","operator":"gte","threshold":30,"recoveryThreshold":28,"durationSeconds":60,`+
			`"activeWindow":null,"severity":"warning"}')`,
		policy,
		seed.ID("tenant-a"),
		seed.ID("cust-b"),
		uuid.New(),
	)

	var def string
	ownerScan(
		t,
		`SELECT id FROM monitoring.alert_policies WHERE tenant_id = $1 AND kind = 'default_alert'`,
		[]any{seed.ID("tenant-a")},
		&def,
	)

	insert := func(policy, ruleKey any, typ string) string {
		id := uuid.NewString()
		owner(
			t,
			`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, policy_id, rule_key, type, severity, status,
				cause_code, evidence_kind, evidence_text, observed_at, detected_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,'warning','open','unknown','demo_observation','demo',$8,$8)`,
			id,
			seed.ID("tenant-a"),
			unit,
			org,
			policy,
			ruleKey,
			typ,
			clock().Add(-10*time.Minute),
		)

		return id
	}
	own, viaDefault, none := insert(
		policy,
		nil,
		"sensor",
	), insert(
		def,
		"ventilation_co2",
		"sensor",
	), insert(
		nil,
		nil,
		"tamper",
	)
	rule := func(a *actor, id string) any {
		code, m := post(srv, a, "alerts.get", `{"id":"`+id+`"}`)
		if code != 200 {
			t.Fatalf("alerts.get %s: %d %v", id, code, m)
		}

		return data(m)["rule"]
	}

	want := map[string]any{
		fieldName:              "Bedroom too hot",
		fieldMetric:            metricTemperature,
		fieldOperator:          operatorAtLeast,
		fieldThreshold:         30.0,
		fieldRecoveryThreshold: 28.0,
		fieldDurationSeconds:   60.0,
	}
	for _, a := range []*actor{hq(), customerB()} {
		if got, _ := rule(a, own).(map[string]any); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("the policy's condition: %v", got)
		}
	}

	if got, _ := rule(hq(), viaDefault).(map[string]any); got["name"] != "Ventilation" ||
		got["metric"] != metricCO2 ||
		got["threshold"] != 1000.0 ||
		got["recoveryThreshold"] != 900.0 ||
		got["durationSeconds"] != 600.0 {
		t.Errorf("the default rule of the rule key: %v", got)
	}

	if got := rule(hq(), none); got != nil {
		t.Errorf("no policy, no rule: %v", got)
	}

	if _, res := post(
		srv,
		hq(),
		"alerts.list",
		`{"filters":{"unitId":"`+unit+`"}}`,
	); len(items(t, res)) != 3 ||
		items(t, res)[0]["rule"] == nil && items(t, res)[1]["rule"] == nil {
		t.Errorf("alerts.list carries the rule: %v", res)
	}
	// a technician on the unit without alert.resolve acknowledges (alert.read) but does not resolve (DD-T07)
	owner(
		t,
		`DELETE FROM maintenance.assignments WHERE technician_membership_id = $1 AND job_id IN (SELECT id FROM
			maintenance.jobs WHERE customer_org_id = $2)`,
		seed.ID("tech-internal-a"),
		seed.ID("org-customer-b"),
	)
	owner(
		t,
		`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'unit',$3) ON CONFLICT
			DO NOTHING`,
		seed.ID("tenant-a"),
		seed.ID("tech-internal-a"),
		unit,
	)
	assign(t, unit, "tech-internal-a", clock().Add(-time.Hour), clock().Add(time.Hour), "active")
	owner(
		t,
		`DELETE FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'alert.resolve'`,
		seed.ID("tech-internal-a"),
	)
	t.Cleanup(func() {
		owner(
			t,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1, $2, 'alert.resolve')
				ON CONFLICT DO NOTHING`,
			seed.ID("tenant-a"),
			seed.ID("tech-internal-a"),
		)
	})

	if code, res := write(
		srv,
		techInt(),
		"alerts.acknowledge",
		`{"alertId":"`+own+`"}`,
		1,
	); code != 200 || data(res)["status"] != statusAcknowledged ||
		data(res)["rule"] == nil {
		t.Fatalf("technician acknowledges with alert.read: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		techInt(),
		"alerts.resolve",
		`{"alertId":"`+own+`","resolutionReason":"filter replaced","resolutionEvidenceIds":[]}`,
		2,
	); code != 403 {
		t.Errorf("resolve needs alert.resolve: %d", code)
	}

	owner(
		t,
		`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1, $2, 'alert.resolve')
			ON CONFLICT DO NOTHING`,
		seed.ID("tenant-a"),
		seed.ID("tech-internal-a"),
	)

	if code, res := write(
		srv,
		techInt(),
		"alerts.resolve",
		`{"alertId":"`+own+`","resolutionReason":"filter replaced","resolutionEvidenceIds":[]}`,
		2,
	); code != 200 ||
		data(res)["status"] != statusResolved {
		t.Fatalf("resolve with alert.resolve: %d %v", code, res)
	}
}
