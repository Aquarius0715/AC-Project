package maintenance

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// TestSortFilterStatuses pins the filterCare.list order: the status sort ranks unknown (and any other status), ok, due
// soon, overdue — desc by default — with equal ranks by unit ascending in both directions; the id sort is by unit only,
// reversed for desc.
func TestSortFilterStatuses(t *testing.T) {
	t.Parallel()

	unit := func(n string) uuid.UUID { return uuid.MustParse("00000000-0000-0000-0000-0000000000" + n) }
	sortOn := func(field, direction string) *paging.SortSpec {
		return &paging.SortSpec{Field: field, Direction: direction}
	}
	rows := [][2]string{
		{"05", "bogus"},
		{"03", filterOK},
		{"06", filterOverdue},
		{"02", filterDueSoon},
		{"04", filterUnknown},
		{"01", filterOverdue},
	}
	statuses := func() []FilterStatus {
		out := make([]FilterStatus, 0, len(rows))
		for _, row := range rows {
			filterStatus := new(FilterStatus)
			filterStatus.UnitID, filterStatus.Status = unit(row[0]), row[1]
			out = append(out, *filterStatus)
		}

		return out
	}

	for name, testCase := range map[string]struct {
		sortBy *paging.SortSpec
		desc   bool
		want   []string
	}{
		"default":     {nil, true, []string{"01", "06", "02", "03", "04", "05"}},
		"status desc": {sortOn(fieldStatus, sortDesc), true, []string{"01", "06", "02", "03", "04", "05"}},
		"status asc":  {sortOn(fieldStatus, "asc"), false, []string{"04", "05", "03", "02", "01", "06"}},
		"id asc":      {sortOn("id", "asc"), false, []string{"01", "02", "03", "04", "05", "06"}},
		"id desc":     {sortOn("id", sortDesc), true, []string{"06", "05", "04", "03", "02", "01"}},
	} {
		got := statuses()
		sortFilterStatuses(got, testCase.sortBy, testCase.desc)

		ids := make([]string, 0, len(got))
		for _, filterStatus := range got {
			ids = append(ids, filterStatus.UnitID.String()[34:])
		}

		if !slices.Equal(ids, testCase.want) {
			t.Errorf("%s: %v, want %v", name, ids, testCase.want)
		}
	}
}
