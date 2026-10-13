package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestDemoStateReport: a device's state report (DemoTrigger state, sent with each heartbeat) becomes its unit's observed
// state, observed at the report time, and moves the unit's last-seen time; a report without a state, with a mode the
// air conditioner has no word for, or from after the scenario clock is refused.
func TestDemoStateReport(t *testing.T) {
	s := serverWith(t, true)
	unit, device := seed.ID("unit-limited"), seed.ID("device-limited")
	var state, seen string
	ownerScan(t, `SELECT observed_state::text, COALESCE(last_seen_at::text, '') FROM assets.units WHERE id = $1`, []any{unit}, &state, &seen)
	t.Cleanup(func() {
		owner(t, `UPDATE assets.units SET observed_state = $2::jsonb, last_seen_at = NULLIF($3, '')::timestamptz WHERE id = $1`, unit, state, seen)
	})
	report := func(at time.Time, st string) (int, map[string]any) {
		return write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+at.Format(time.RFC3339)+
			`","eventType":"state","deviceId":"`+device.String()+`"`+st+`}`, 0)
	}
	at := clock.Add(-30 * time.Second)
	if code, m := report(at, `,"state":{"power":false,"celsius":27,"mode":"dry","fanLevel":"high"}`); code != 200 {
		t.Fatalf("state report: %d %v", code, m)
	}
	_, m := post(s, &hq, "units.get", `{"id":"`+unit.String()+`"}`)
	o := data(m)["observedState"].(map[string]any)
	if o["power"] != false || o["celsius"] != 27.0 || o["mode"] != "dry" || o["fanLevel"] != "high" || o["observedAt"] != at.Format(time.RFC3339) {
		t.Errorf("observed state %v", o)
	}
	var last time.Time
	ownerScan(t, `SELECT last_seen_at FROM assets.units WHERE id = $1`, []any{unit}, &last)
	if !last.Equal(at) && last.Before(at) {
		t.Errorf("last seen %v, want at least %v", last, at)
	}
	for name, c := range map[string]struct {
		at    time.Time
		state string
		want  string
	}{
		"no state":     {at, ``, "state=error.required"},
		"unknown mode": {at, `,"state":{"power":true,"celsius":24,"mode":"heat","fanLevel":"mid"}`, "state.mode=error.invalid"},
		"bad fan":      {at, `,"state":{"power":true,"celsius":24,"mode":"cool","fanLevel":"turbo"}`, "state.fanLevel=error.invalid"},
		"hot":          {at, `,"state":{"power":true,"celsius":55,"mode":"cool","fanLevel":"mid"}`, "state.celsius=error.range"},
		"future":       {clock.Add(time.Hour), `,"state":{"power":true,"celsius":24,"mode":"cool","fanLevel":"mid"}`, "occurredAt=error.range"},
	} {
		code, m := report(c.at, c.state)
		refusal(t, name, code, m, c.want)
	}
}
