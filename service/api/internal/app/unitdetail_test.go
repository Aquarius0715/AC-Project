package app

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestUnitDetail(t *testing.T) {
	s := server(t)
	code, m := post(s, &customerA, "units.get", `{"id":"`+seed.ID("unit-online-rto").String()+`"}`)
	d := data(m)
	if code != 200 || d["capabilities"] == nil || d["effectiveControlPolicy"].(map[string]any)["state"] != "unrestricted" ||
		d["controlAvailability"].(map[string]any)["state"] != "available" || len(d["components"].([]any)) != 18 { // indoor 8 + outdoor 5 + electrical 5 (serviceScope)
		t.Fatalf("detail: %d %v", code, m)
	}
	loc := d["location"].(map[string]any)
	path := loc["pathLabels"].([]any)
	if len(path) != 3 || path[0] != "Home A" || path[1] != "1F" || path[2] != "Bedroom" {
		t.Errorf("path labels: %v", path)
	}
	if _, ok := d["pendingCommandIds"].([]any); !ok {
		t.Error("pendingCommandIds array")
	}
	// the seeded applied temperature limit restricts the Lobby AC (IR46)
	_, m = post(s, &customerA, "units.get", `{"id":"`+seed.ID("unit-limited").String()+`"}`)
	p := data(m)["effectiveControlPolicy"].(map[string]any)
	if p["state"] != "restricted" || p["phase"] != "applied" || p["reasonKey"] != "control.restriction_active" || p["policy"].(map[string]any)["kind"] != "temperature_limit" {
		t.Errorf("restricted policy: %v", p)
	}
	if path := data(m)["location"].(map[string]any)["pathLabels"].([]any); len(path) != 1 {
		t.Errorf("unit without space: %v", path)
	}
}
