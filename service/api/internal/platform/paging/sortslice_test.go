package paging_test

import (
	"cmp"
	"encoding/json"
	"slices"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// The sort values the slice tests ask for.
const (
	sortAsc = "asc"
	keyRank = "rank"
)

type sortRow struct {
	id   string
	rank int
}

func sortKeys() map[string]func(a, b sortRow) int {
	return map[string]func(a, b sortRow) int{
		"id":    func(a, b sortRow) int { return cmp.Compare(a.id, b.id) },
		keyRank: func(a, b sortRow) int { return cmp.Compare(a.rank, b.rank) },
	}
}

func sortIDs(rows []sortRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.id)
	}

	return ids
}

func TestSortSlice(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sort *paging.SortSpec
		want []string
	}{
		{"nil sort keeps the order", nil, []string{"c", "a", "b", "d"}},
		{
			"asc, ties by id asc",
			&paging.SortSpec{Field: keyRank, Direction: sortAsc},
			[]string{"a", "c", "b", "d"},
		},
		{
			"desc, ties still by id asc",
			&paging.SortSpec{Field: keyRank, Direction: sortDesc},
			[]string{"d", "b", "a", "c"},
		},
		{
			"id desc",
			&paging.SortSpec{Field: "id", Direction: sortDesc},
			[]string{"d", "c", "b", "a"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			rows := []sortRow{{"c", 1}, {"a", 1}, {"b", 2}, {"d", 3}}

			err := paging.SortSlice(rows, testCase.sort, sortKeys())
			if err != nil {
				t.Fatalf("sort: %v", err)
			}

			if got := sortIDs(rows); !slices.Equal(got, testCase.want) {
				t.Fatalf("order %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestSortSliceRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sort  *paging.SortSpec
		field string
	}{
		{&paging.SortSpec{Field: keyCreatedAt, Direction: sortAsc}, "sort.field"},
		{&paging.SortSpec{Field: keyRank, Direction: "up"}, "sort.direction"},
		{&paging.SortSpec{Field: keyRank, Direction: ""}, "sort.direction"},
	}

	for _, testCase := range cases {
		rows := []sortRow{{"b", 1}, {"a", 1}}

		err := paging.SortSlice(rows, testCase.sort, sortKeys())

		fieldErrors := apperr.From(err).FieldErrors
		if code(err) != apperr.Validation || fieldErrors[testCase.field] != apperr.KeyInvalid {
			t.Fatalf("%+v: got %v, want VALIDATION on %s", testCase.sort, err, testCase.field)
		}

		if got := sortIDs(rows); !slices.Equal(got, []string{"b", "a"}) {
			t.Fatalf("%+v: a refused sort reordered the items: %v", testCase.sort, got)
		}
	}
}

func TestNoFilters(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{"", "null", "{}", "  {}  ", " null\n"} {
		query := paging.Query{Cursor: nil, Limit: nil, Sort: nil, Filters: json.RawMessage(raw)}
		if query.HasFilters() || paging.NoFilters(query, "filters") != nil {
			t.Fatalf("%q carries no filter", raw)
		}
	}

	for _, raw := range []string{`{"status":"open"}`, `[]`} {
		query := paging.Query{Cursor: nil, Limit: nil, Sort: nil, Filters: json.RawMessage(raw)}

		err := paging.NoFilters(query, "filters")
		if !query.HasFilters() || code(err) != apperr.Validation ||
			apperr.From(err).FieldErrors["filters"] != apperr.KeyInvalid {
			t.Fatalf("%q: got %v, want VALIDATION on filters", raw, err)
		}
	}
}

func TestSortSliceWithoutAnIDKey(t *testing.T) {
	t.Parallel()

	rows := []sortRow{{"c", 1}, {"a", 1}, {"b", 0}}
	keys := map[string]func(a, b sortRow) int{keyRank: sortKeys()[keyRank]}

	err := paging.SortSlice(rows, &paging.SortSpec{Field: keyRank, Direction: sortAsc}, keys)
	if err != nil {
		t.Fatalf("sort: %v", err)
	}

	if got := sortIDs(rows); !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Fatalf("ties keep their order without an id key: %v", got)
	}
}
