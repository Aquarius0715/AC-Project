package ops_test

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// The audited fields of the change tests.
const (
	fieldName  = "name"
	fieldModes = "modes"
	fieldWhen  = "when"
)

// sameText says whether two display values are both absent or the same text.
func sameText(got, want *string) bool {
	return (got == nil) == (want == nil) && (got == nil || *got == *want)
}

func TestChanges(t *testing.T) {
	t.Parallel()

	before, after := ops.Changes(
		map[string]any{
			fieldName:  "Old",
			fieldModes: []string{"cool"},
			"same":     1,
			"gone":     "x",
			"nil":      nil,
		},
		map[string]any{
			fieldName:  "New",
			fieldModes: []string{"cool", "dry"},
			"same":     1,
			"added":    true,
			"nil":      nil,
		},
	)

	want := map[string][2]*string{
		fieldName:  {new("Old"), new("New")},
		fieldModes: {new(`["cool"]`), new(`["cool","dry"]`)},
		"gone":     {new("x"), nil},
		"added":    {nil, new("true")},
	}
	if len(before) != len(want) || len(after) != len(want) {
		t.Fatalf("changed keys: before %v after %v", before, after)
	}

	for key, pair := range want {
		if !sameText(before[key], pair[0]) || !sameText(after[key], pair[1]) {
			t.Errorf("%s: %v → %v", key, before[key], after[key])
		}
	}

	if before, after := ops.Changes(nil, nil); len(before) != 0 || len(after) != 0 {
		t.Error("no change")
	}
}

// TestChangesOfTimes: times compare as instants and are shown without quotes; a nil time is no value.
func TestChangesOfTimes(t *testing.T) {
	t.Parallel()

	before, after := ops.Changes(
		map[string]any{fieldWhen: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		map[string]any{fieldWhen: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)},
	)
	if *before[fieldWhen] != "2026-09-01T00:00:00Z" || *after[fieldWhen] != "2026-09-02T00:00:00Z" {
		t.Errorf("times are shown without quotes: %v → %v", *before[fieldWhen], *after[fieldWhen])
	}

	kl := time.FixedZone("MYT", 8*3600)

	when := time.Date(2026, 9, 1, 8, 0, 0, 0, kl)
	if before, _ := ops.Changes(
		map[string]any{fieldWhen: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
		map[string]any{fieldWhen: &when},
	); len(
		before,
	) != 0 {
		t.Errorf("the same instant in another zone is no change: %v", before)
	}

	var none *time.Time
	if before, after := ops.Changes(
		map[string]any{fieldWhen: none},
		map[string]any{fieldWhen: &when},
	); before[fieldWhen] != nil ||
		*after[fieldWhen] != "2026-09-01T00:00:00Z" {
		t.Errorf("nil time → set: %v → %v", before, after)
	}
}

func TestMasked(t *testing.T) {
	t.Parallel()

	masked := ops.Masked(
		map[string]*string{
			"contactPhone": new("+60 12"),
			"Email":        new("a@b"),
			fieldName:      new("Ali"),
			"apiToken":     nil,
		},
	)
	if *masked["contactPhone"] != "***masked***" || *masked["Email"] != "***masked***" ||
		*masked[fieldName] != "Ali" ||
		masked["apiToken"] != nil {
		t.Fatalf("masked: %v", masked)
	}

	if ops.Masked(nil) != nil {
		t.Error("nil stays nil")
	}
}
