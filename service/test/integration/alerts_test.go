package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/internal/seed"
)

func newAlert(t *testing.T, unit, org, severity string, at time.Time) string {
	id := uuid.NewString()
	// one open alert per unit / policy / rule / type (alerts_one_open_per_rule): use the severity as the rule key
	owner(t, `INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, rule_key, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at)
		VALUES ($1,$2,$3,$4,$5,'sensor',$5,'open','unknown','demo_observation','demo',$6,$6)`, id, seed.ID("tenant-a"), unit, org, severity, at)
	return id
}

func TestAlerts(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Alert AC")
	org := seed.ID("org-customer-b").String()
	crit := newAlert(t, u, org, "critical", clock.Add(-10*time.Minute))
	warn := newAlert(t, u, org, "warning", clock.Add(-5*time.Minute))
	norm := newAlert(t, u, org, "normal", clock.Add(-time.Minute))

	ids := func(m map[string]any) []string {
		out := []string{}
		for _, it := range items(m) {
			out = append(out, it["id"].(string))
		}
		return out
	}
	_, m := post(s, &hq, "alerts.list", `{"filters":{"unitId":"`+u+`"}}`)
	if got := ids(m); strings.Join(got, ",") != strings.Join([]string{crit, warn, norm}, ",") {
		t.Fatalf("default severity desc: %v", got)
	}
	_, m = post(s, &hq, "alerts.list", `{"filters":{"unitId":"`+u+`"},"sort":{"field":"severity","direction":"asc"}}`)
	if got := ids(m); got[0] != norm || got[2] != crit {
		t.Fatalf("severity asc: %v", got)
	}
	for body, n := range map[string]int{
		`{"filters":{"unitIds":["` + u + `"],"severity":"warning"}}`: 1,
		`{"filters":{"unitId":"` + u + `","status":"open"}}`:         3,
		`{"filters":{"unitId":"` + u + `","from":"` + clock.Add(-6*time.Minute).Format(time.RFC3339) + `","to":"` + clock.Format(time.RFC3339) + `"}}`: 2,
		`{"filters":{"unitId":"` + u + `","customerId":"` + seed.ID("cust-b").String() + `"}}`:                                                         3,
		`{"filters":{"propertyId":"` + seed.ID("property-home-b").String() + `","unitId":"` + u + `"}}`:                                                3,
		`{"filters":{"unitId":"` + u + `","customerId":"` + uuid.NewString() + `"}}`:                                                                   0,
	} {
		if _, m := post(s, &hq, "alerts.list", body); len(items(m)) != n {
			t.Errorf("%s: %d want %d", body, len(items(m)), n)
		}
	}
	for _, b := range []string{`{"filters":{"severity":"info"}}`, `{"filters":{"status":"x"}}`, `{"filters":{"from":"2026-09-14T01:00:00Z","to":"2026-09-14T00:00:00Z"}}`, `{"filters":{"z":1}}`, `{"sort":{"field":"title","direction":"asc"}}`} {
		if code, _ := post(s, &hq, "alerts.list", b); code != 422 {
			t.Errorf("%s: %d", b, code)
		}
	}
	// scope: customer-b sees, customer-a does not; units.list shows the active count
	if code, _ := post(s, &customerB, "alerts.get", `{"id":"`+crit+`"}`); code != 200 {
		t.Error("customer-b reads its alert")
	}
	if code, _ := post(s, &customerA, "alerts.get", `{"id":"`+crit+`"}`); code != 404 {
		t.Error("customer-a must not read customer-b's alert")
	}
	_, g := post(s, &hq, "units.get", `{"id":"`+u+`"}`)
	if data(g)["activeAlertCount"].(float64) != 2 { // IR51: normal alerts are not counted
		t.Fatalf("activeAlertCount: %v", data(g)["activeAlertCount"])
	}
	// acknowledge → resolve, with state and version rules
	if code, m := write(s, &hq, "alerts.acknowledge", `{"alertId":"`+crit+`"}`, 1); code != 200 || data(m)["status"] != "acknowledged" || data(m)["acknowledgedAt"] == nil {
		t.Fatalf("ack: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "alerts.acknowledge", `{"alertId":"`+crit+`"}`, 2); code != 409 {
		t.Error("ack twice")
	}
	if code, _ := write(s, &hq, "alerts.resolve", `{"alertId":"`+crit+`","resolutionReason":"   ","resolutionEvidenceIds":[]}`, 2); code != 422 {
		t.Error("blank reason")
	}
	if code, _ := write(s, &hq, "alerts.resolve", `{"alertId":"`+crit+`","resolutionReason":"`+strings.Repeat("r", 1001)+`","resolutionEvidenceIds":[]}`, 2); code != 422 {
		t.Error("reason too long")
	}
	if code, _ := write(s, &hq, "alerts.resolve", `{"alertId":"`+crit+`","resolutionReason":"fixed"}`, 2); code != 422 {
		t.Error("evidence ids required")
	}
	if code, m := write(s, &hq, "alerts.resolve", `{"alertId":"`+crit+`","resolutionReason":" window closed ","resolutionEvidenceIds":["`+uuid.NewString()+`"]}`, 2); code != 200 || data(m)["resolutionReason"] != "window closed" {
		t.Fatalf("resolve: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "alerts.resolve", `{"alertId":"`+crit+`","resolutionReason":"again","resolutionEvidenceIds":[]}`, 3); code != 409 {
		t.Error("resolve twice")
	}
	if code, m := write(s, &hq, "alerts.resolve", `{"alertId":"`+warn+`","resolutionReason":"direct","resolutionEvidenceIds":[]}`, 1); code != 200 || data(m)["status"] != "resolved" {
		t.Fatalf("resolve from open: %d", code)
	}
	if code, _ := write(s, &hq, "alerts.acknowledge", `{"alertId":"`+norm+`"}`, 5); code != 409 {
		t.Error("stale version")
	}
	if code, _ := write(s, &customerB, "alerts.acknowledge", `{"alertId":"`+norm+`"}`, 1); code != 403 {
		t.Error("clients do not acknowledge")
	}
	// technician without an assignment on the unit
	if code, _ := write(s, &techInt, "alerts.acknowledge", `{"alertId":"`+norm+`"}`, 1); code != 404 {
		t.Error("technician outside scope → NOT_FOUND")
	}
	_, g = post(s, &hq, "units.get", `{"id":"`+u+`"}`)
	if data(g)["activeAlertCount"].(float64) != 0 { // only the normal alert is left open (IR51)
		t.Fatal("resolved alerts are not active")
	}
}
