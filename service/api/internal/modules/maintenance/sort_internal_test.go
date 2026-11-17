package maintenance

import (
	"slices"
	"testing"
	"time"
)

// TestSortItems pins the IR23/IR34 jobs.list order: desc reverses the key only, a null severity or due date stays last
// in both directions, and equal keys fall back to id ascending.
func TestSortItems(t *testing.T) {
	t.Parallel()

	early := time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)

	items := func() []listItem {
		return []listItem{
			{rank: 2, sev: -1, due: nil, id: "e", item: nil},
			{rank: 1, sev: 2, due: &late, id: "d", item: nil},
			{rank: 2, sev: 1, due: &early, id: "c", item: nil},
			{rank: 1, sev: -1, due: nil, id: "b", item: nil},
			{rank: 3, sev: 1, due: &late, id: "a", item: nil},
		}
	}

	for name, testCase := range map[string]struct {
		field string
		desc  bool
		want  []string
	}{
		"status asc":    {fieldStatus, false, []string{"b", "d", "c", "e", "a"}},
		"status desc":   {fieldStatus, true, []string{"a", "c", "e", "b", "d"}},
		"severity asc":  {"severity", false, []string{"a", "c", "d", "b", "e"}},
		"severity desc": {"severity", true, []string{"d", "a", "c", "b", "e"}},
		"dueAt asc":     {fieldDueAt, false, []string{"c", "a", "d", "b", "e"}},
		"dueAt desc":    {fieldDueAt, true, []string{"a", "d", "c", "b", "e"}},
		"id desc":       {"id", true, []string{"e", "d", "c", "b", "a"}},
	} {
		got := items()
		sortItems(got, testCase.field, testCase.desc)

		ids := make([]string, 0, len(got))
		for _, item := range got {
			ids = append(ids, item.id)
		}

		if !slices.Equal(ids, testCase.want) {
			t.Errorf("%s: %v, want %v", name, ids, testCase.want)
		}
	}
}
