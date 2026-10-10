package integration

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// reading stores one measurement of the unit and returns its ID (the measurement's event ID).
func reading(t *testing.T, unit, metric string, value float64, symbol string, at time.Time, quality string) string {
	t.Helper()
	id := uuid.NewString()
	owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $7, 'measured', $8, $5, $9)`, seed.ID("tenant-a"), unit, uuid.NewString(), metric, at, value, symbol, quality, id)
	return id
}

// IR327 / IR66 / DD-A05 item 6: an alert's evidence candidates are its own evidence at detection and the unit's valid
// remeasurements after detection — of the rule's metric when the alert has a rule — newest first; an alert without a
// policy resolves only with at least one candidate, a policy alert also without; only resolvers read the candidates.
func TestAlertEvidence(t *testing.T) {
	s := server(t)
	u := newUnit(t, s, "Evidence AC")
	org := seed.ID("org-customer-b").String()
	free := newAlert(t, u, org, "critical", clock.Add(-30*time.Minute)) // no policy
	detected := uuid.NewString()
	owner(t, `UPDATE monitoring.alerts SET evidence_ids = ARRAY[$2::uuid] WHERE id = $1`, free, detected)
	before := reading(t, u, "temperature", 31, "°C", clock.Add(-40*time.Minute), "valid") // before the detection
	after := reading(t, u, "temperature", 26, "°C", clock.Add(-5*time.Minute), "valid")
	twin := reading(t, u, "humidity", 55, "%", clock.Add(-5*time.Minute), "valid") // observed with it: the ID decides
	suspect := reading(t, u, "temperature", 99, "°C", clock.Add(-4*time.Minute), "suspect")
	co2 := reading(t, u, "co2", 650, "ppm", clock.Add(-3*time.Minute), "valid")
	estimate := reading(t, u, "temperature", 25, "°C", clock.Add(-2*time.Minute), "valid") // estimated, not a remeasurement
	owner(t, `UPDATE monitoring.measurements SET origin = 'estimated' WHERE event_id = $1`, estimate)
	owner(t, `UPDATE monitoring.measurements SET origin = 'inspection' WHERE event_id = $1`, co2) // recorded on site: counts

	evidence := func(a *actor, alert string) (int, map[string]any) {
		return post(s, a, "alerts.evidence", `{"alertId":"`+alert+`","query":{}}`)
	}
	code, m := evidence(&hq, free)
	if code != 200 {
		t.Fatalf("evidence: %d %v", code, m)
	}
	kinds := func(m map[string]any) string {
		got := []string{}
		for _, it := range items(m) {
			got = append(got, it["kind"].(string)+":"+it["id"].(string))
		}
		return strings.Join(got, ",")
	}
	byID := func(m map[string]any, id string) map[string]any {
		for _, it := range items(m) {
			if it["id"] == id {
				return it
			}
		}
		return nil
	}
	pair := []string{after, twin}
	slices.Sort(pair)
	// newest first, then by ID; any metric without a rule
	if got, want := kinds(m), "remeasurement:"+co2+",remeasurement:"+pair[0]+",remeasurement:"+pair[1]+",detection:"+detected; got != want {
		t.Fatalf("candidates: %s want %s", got, want)
	}
	if it := byID(m, after); it["metric"] != "temperature" || it["value"].(float64) != 26 || it["unit"] != "°C" || it["origin"] != "measured" || it["quality"] != "valid" || it["eventType"] != nil {
		t.Errorf("remeasurement fields: %v", it)
	}
	if it := byID(m, detected); it["metric"] != nil || it["value"] != nil || it["origin"] != nil || it["observedAt"] == nil {
		t.Errorf("detection fields: %v", it)
	}
	if code, m := post(s, &hq, "alerts.evidence", `{"alertId":"`+free+`","query":{"limit":1}}`); code != 200 || len(items(m)) != 1 || data(m)["total"].(float64) != 4 || data(m)["nextCursor"] == nil {
		t.Errorf("paged: %d %v", code, m)
	}
	// the allowed sorts: observedAt (oldest first: the detection) and id
	if _, m := post(s, &hq, "alerts.evidence", `{"alertId":"`+free+`","query":{"sort":{"field":"observedAt","direction":"asc"}}}`); !strings.HasPrefix(kinds(m), "detection:"+detected+",") || !strings.HasSuffix(kinds(m), co2) {
		t.Errorf("observedAt asc: %s", kinds(m))
	}
	all := []string{co2, after, twin, detected}
	slices.Sort(all)
	if _, m := post(s, &hq, "alerts.evidence", `{"alertId":"`+free+`","query":{"sort":{"field":"id","direction":"desc"}}}`); items(m)[0]["id"] != all[3] || items(m)[3]["id"] != all[0] {
		t.Errorf("id desc: %s", kinds(m))
	}
	if code, m := post(s, &hq, "alerts.evidence", `{"alertId":"00000000-0000-0000-0000-000000000000","query":{}}`); code != 422 || m["fieldErrors"].(map[string]any)["alertId"] != "error.required" {
		t.Errorf("no alert: %d %v", code, m)
	}
	if code, _ := post(s, &hq, "alerts.evidence", `{"alertId":"`+free+`","query":{"filters":{"kind":"x"}}}`); code != 422 {
		t.Error("no filters")
	}
	if code, _ := evidence(&customerB, free); code != 403 {
		t.Errorf("clients do not resolve: %d", code)
	}
	if code, _ := evidence(&hq, uuid.NewString()); code != 404 {
		t.Error("unknown alert")
	}

	// the resolution: at least one candidate for an alert without a policy, and candidates only
	resolve := func(alert string, v int, ids ...string) (int, map[string]any) {
		list := "[]"
		if len(ids) > 0 {
			list = `["` + ids[0]
			for _, id := range ids[1:] {
				list += `","` + id
			}
			list += `"]`
		}
		return write(s, &hq, "alerts.resolve", `{"alertId":"`+alert+`","resolutionReason":"window closed","resolutionEvidenceIds":`+list+`}`, v)
	}
	for name, c := range map[string]struct {
		ids []string
		key string
	}{
		"none":             {nil, "errors.evidence_required"},
		"unknown":          {[]string{uuid.NewString()}, "errors.evidence_unknown"},
		"before detection": {[]string{before}, "errors.evidence_unknown"},
		"suspect reading":  {[]string{suspect}, "errors.evidence_unknown"},
		"estimate":         {[]string{estimate}, "errors.evidence_unknown"},
		"twice":            {[]string{after, after}, "error.invalid"},
		"one of two wrong": {[]string{after, uuid.NewString()}, "errors.evidence_unknown"},
	} {
		if code, m := resolve(free, 1, c.ids...); code != 422 || m["fieldErrors"].(map[string]any)["resolutionEvidenceIds"] != c.key {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	if code, m := resolve(free, 1, after, detected); code != 200 || data(m)["status"] != "resolved" {
		t.Fatalf("resolve with candidates: %d %v", code, m)
	}

	// a policy alert: the remeasurements are of its rule's metric, and it may be resolved by hand without evidence
	policy := uuid.NewString()
	owner(t, `INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone, condition)
		VALUES ($1, $2, 'alert', $3, 'Bedroom too hot', $4, $4, 'Asia/Kuala_Lumpur',
		'{"metric":"temperature","operator":"gte","threshold":30,"recoveryThreshold":28,"durationSeconds":60,"activeWindow":null,"severity":"warning"}')`,
		policy, seed.ID("tenant-a"), seed.ID("cust-b"), uuid.New())
	ruled := uuid.NewString()
	owner(t, `INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, policy_id, type, severity, status, cause_code, evidence_kind, evidence_text, observed_at, detected_at)
		VALUES ($1, $2, $3, $4, $5, 'sensor', 'warning', 'open', 'unknown', 'demo_observation', 'demo', $6, $6)`, ruled, seed.ID("tenant-a"), u, org, policy, clock.Add(-20*time.Minute))
	_, m = evidence(&hq, ruled)
	if len(items(m)) != 1 || items(m)[0]["id"] != after {
		t.Errorf("rule alert: temperature readings only: %v", m)
	}
	if code, _ := resolve(ruled, 1, co2); code != 422 {
		t.Error("another metric is not the rule alert's evidence")
	}
	if code, m := resolve(ruled, 1); code != 200 || data(m)["status"] != "resolved" {
		t.Errorf("policy alert by hand: %d %v", code, m)
	}
}
