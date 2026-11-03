package ops

import (
	"testing"
	"time"
)

func TestChanges(t *testing.T) {
	strPtr := func(v string) *string { return &v }
	before, after := Changes(
		map[string]any{
			"name":  "Old",
			"modes": []string{"cool"},
			"same":  1,
			"gone":  "x",
			"nil":   nil,
		},
		map[string]any{
			"name":  "New",
			"modes": []string{"cool", "dry"},
			"same":  1,
			"added": true,
			"nil":   nil,
		},
	)

	want := map[string][2]*string{
		"name":  {strPtr("Old"), strPtr("New")},
		"modes": {strPtr(`["cool"]`), strPtr(`["cool","dry"]`)},
		"gone":  {strPtr("x"), nil},
		"added": {nil, strPtr("true")},
	}
	if len(before) != len(want) || len(after) != len(want) {
		t.Fatalf("changed keys: before %v after %v", before, after)
	}

	for k, w := range want {
		if (before[k] == nil) != (w[0] == nil) || (before[k] != nil && *before[k] != *w[0]) ||
			(after[k] == nil) != (w[1] == nil) ||
			(after[k] != nil && *after[k] != *w[1]) {
			t.Errorf("%s: %v → %v", k, before[k], after[k])
		}
	}

	if b, a := Changes(nil, nil); len(b) != 0 || len(a) != 0 {
		t.Error("no change")
	}

	before, after = Changes(
		map[string]any{"when": time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		map[string]any{"when": time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)},
	)
	if *before["when"] != "2026-09-01T00:00:00Z" || *after["when"] != "2026-09-02T00:00:00Z" {
		t.Errorf("times are shown without quotes: %v → %v", *before["when"], *after["when"])
	}

	kl := time.FixedZone("MYT", 8*3600)

	when := time.Date(2026, 9, 1, 8, 0, 0, 0, kl)
	if b, _ := Changes(
		map[string]any{"when": time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		map[string]any{"when": &when},
	); len(
		b,
	) != 0 {
		t.Errorf("the same instant in another zone is no change: %v", b)
	}

	var none *time.Time
	if b, a := Changes(
		map[string]any{"when": none},
		map[string]any{"when": &when},
	); b["when"] != nil ||
		*a["when"] != "2026-09-01T00:00:00Z" {
		t.Errorf("nil time → set: %v → %v", b, a)
	}
}

func TestMasked(t *testing.T) {
	strPtr := func(v string) *string { return &v }

	masked := Masked(
		map[string]*string{
			"contactPhone": strPtr("+60 12"),
			"Email":        strPtr("a@b"),
			"name":         strPtr("Ali"),
			"apiToken":     nil,
		},
	)
	if *masked["contactPhone"] != "***masked***" || *masked["Email"] != "***masked***" ||
		*masked["name"] != "Ali" ||
		masked["apiToken"] != nil {
		t.Fatalf("masked: %v", masked)
	}

	if Masked(nil) != nil {
		t.Error("nil stays nil")
	}
}
