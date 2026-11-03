package integration

import (
	"testing"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestUnitDetail(t *testing.T) {
	srv := server(t)
	code, res := post(
		srv,
		&customerA,
		"units.get",
		`{"id":"`+seed.ID("unit-online-rto").String()+`"}`,
	)

	doc := data(res)
	if code != 200 || doc["capabilities"] == nil ||
		asObj(t, doc["effectiveControlPolicy"])["state"] != "unrestricted" ||
		asObj(t, doc["controlAvailability"])["state"] != "available" ||
		len(
			asArr(t, doc["components"]),
		) != 18 { // indoor 8 + outdoor 5 + electrical 5 (serviceScope)
		t.Fatalf("detail: %d %v", code, res)
	}

	loc := asObj(t, doc["location"])

	path := asArr(t, loc["pathLabels"])
	if len(path) != 3 || path[0] != "Home A" || path[1] != "1F" || path[2] != "Bedroom" {
		t.Errorf("path labels: %v", path)
	}

	if _, ok := doc["pendingCommandIds"].([]any); !ok {
		t.Error("pendingCommandIds array")
	}
	// the seeded applied temperature limit restricts the Lobby AC (IR46)
	_, res = post(srv, &customerA, "units.get", `{"id":"`+seed.ID("unit-limited").String()+`"}`)

	p := asObj(t, data(res)["effectiveControlPolicy"])
	if p["state"] != "restricted" || p["phase"] != "applied" ||
		p["reasonKey"] != "control.restriction_active" ||
		asObj(t, p["policy"])["kind"] != "temperature_limit" {
		t.Errorf("restricted policy: %v", p)
	}

	if path := asArr(t, asObj(t, data(res)["location"])["pathLabels"]); len(path) != 1 {
		t.Errorf("unit without space: %v", path)
	}
}
