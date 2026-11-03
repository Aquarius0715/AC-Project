package paging_test

import (
	"errors"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// The sort values the tests ask for.
const (
	sortDesc      = "desc"
	sortCreatedAt = "createdAt"
)

func code(err error) apperr.Code {
	if err == nil {
		return ""
	}

	return apperr.From(err).Code
}

func TestResolveDefaults(t *testing.T) {
	t.Parallel()

	cond := map[string]any{"propertyId": "p1"}

	window, err := paging.Resolve(paging.Query{Cursor: nil, Limit: nil, Sort: nil, Filters: nil}, cond, 1, 1)
	if err != nil || window.Limit != 25 || window.Offset != 0 {
		t.Fatalf("defaults: %+v %v", window, err)
	}

	if window.Next(25) != nil || window.Next(10) != nil {
		t.Fatal("no next page at the end")
	}
}

func TestResolveAndNext(t *testing.T) {
	t.Parallel()

	cond := map[string]any{"propertyId": "p1"}

	window, _ := paging.Resolve(paging.Query{Limit: new(2), Cursor: nil, Sort: nil, Filters: nil}, cond, 1, 1)

	next := window.Next(5)
	if next == nil {
		t.Fatal("next")
	}

	w2, err := paging.Resolve(paging.Query{Limit: new(2), Cursor: next, Sort: nil, Filters: nil}, cond, 1, 1)
	if err != nil || w2.Offset != 2 {
		t.Fatalf("second page: %+v %v", w2, err)
	}

	if w3 := w2.Next(5); w3 == nil {
		t.Fatal("third page")
	} else if w4, _ := paging.Resolve(paging.Query{Limit: new(2), Cursor: w3, Sort: nil, Filters: nil}, cond, 1, 1); w4.Next(5) != nil ||
		w4.Offset != 4 {
		t.Fatal("last page")
	}
}

func TestResolveRefuses(t *testing.T) {
	t.Parallel()

	cond := map[string]any{"propertyId": "p1"}
	window, _ := paging.Resolve(paging.Query{Limit: new(2), Cursor: nil, Sort: nil, Filters: nil}, cond, 1, 1)
	next := window.Next(5)

	cases := []struct {
		q    paging.Query
		cond any
		sv   int
		want apperr.Code
	}{
		{paging.Query{Limit: new(0), Cursor: nil, Sort: nil, Filters: nil}, cond, 1, apperr.Validation},
		{paging.Query{Limit: new(101), Cursor: nil, Sort: nil, Filters: nil}, cond, 1, apperr.Validation},
		{
			paging.Query{
				Sort:    &paging.SortSpec{Field: "id", Direction: "up"},
				Cursor:  nil,
				Limit:   nil,
				Filters: nil,
			},
			cond,
			1,
			apperr.Validation,
		},
		{
			paging.Query{Limit: new(2), Cursor: new("!!"), Sort: nil, Filters: nil},
			cond,
			1,
			apperr.Validation,
		},
		{
			paging.Query{Limit: new(2), Cursor: new("e30"), Sort: nil, Filters: nil},
			cond,
			1,
			apperr.Validation,
		}, // {} → hash mismatch
		{
			paging.Query{Limit: new(2), Cursor: next, Sort: nil, Filters: nil},
			map[string]any{"propertyId": "p2"},
			1,
			apperr.Validation,
		}, // changed conditions
		{
			paging.Query{Limit: new(3), Cursor: next, Sort: nil, Filters: nil},
			cond,
			1,
			apperr.Validation,
		}, // changed limit
		{
			paging.Query{Limit: new(2), Cursor: next, Sort: nil, Filters: nil},
			cond,
			2,
			apperr.Conflict,
		}, // scope changed
		{paging.Query{Limit: new(2), Cursor: new(""), Sort: nil, Filters: nil}, cond, 1, ""},
	}
	for i, c := range cases {
		_, err := paging.Resolve(c.q, c.cond, c.sv, 1)
		if code(err) != c.want {
			t.Errorf("case %d: got %v want %s", i, err, c.want)
		}
	}
}

func TestOrderBy(t *testing.T) {
	t.Parallel()

	allowed := map[string]string{"id": "id", "name": "display_name"}
	if s, _ := paging.OrderBy(nil, allowed, "id ASC"); s != "id ASC" {
		t.Fatal(s)
	}

	if s, _ := paging.OrderBy(
		&paging.SortSpec{Field: "name", Direction: sortDesc},
		allowed,
		"",
	); s != "display_name DESC, id ASC" {
		t.Fatal(s)
	}

	if s, _ := paging.OrderBy(&paging.SortSpec{Field: "id", Direction: sortDesc}, allowed, ""); s != "id DESC" {
		t.Fatal(s)
	}

	_, err := paging.OrderBy(
		&paging.SortSpec{Field: "evil; drop", Direction: "asc"},
		allowed,
		"",
	)
	if code(
		err,
	) != apperr.Validation {
		t.Fatal("unknown sort field")
	}
}

// TestOrderByDirection: only asc and desc reach the ORDER BY; anything else — an injection attempt among them — is
// VALIDATION on sort.direction before any SQL is built (IR365).
func TestOrderByDirection(t *testing.T) {
	t.Parallel()

	allowed := map[string]string{"id": "r.id", sortCreatedAt: "r.created_at"}
	for dir, want := range map[string]string{"asc": "r.created_at ASC, r.id ASC", sortDesc: "r.created_at DESC, r.id ASC"} {
		got, err := paging.OrderBy(
			&paging.SortSpec{Field: sortCreatedAt, Direction: dir},
			allowed,
			"",
		)
		if err != nil ||
			got != want {
			t.Errorf("%s: %q %v", dir, got, err)
		}
	}

	for _, dir := range []string{"DESC", "", "sideways", "desc, (SELECT pg_sleep(5))", "asc; DROP TABLE x --"} {
		got, err := paging.OrderBy(&paging.SortSpec{Field: sortCreatedAt, Direction: dir}, allowed, "")

		var de *apperr.DomainError
		if got != "" || !errors.As(err, &de) ||
			de.FieldErrors["sort.direction"] != apperr.KeyInvalid {
			t.Errorf("direction %q: %q %v", dir, got, err)
		}
	}

	got, err := paging.Direction(&paging.SortSpec{Direction: sortDesc, Field: ""})
	if err != nil || got != "DESC" {
		t.Errorf("Direction desc: %q %v", got, err)
	}
}

// TestContains: the search term is matched as written, anywhere in the value (IR388).
func TestContains(t *testing.T) {
	t.Parallel()

	for input, want := range map[string]string{
		"filter":   `%filter%`,
		"  mesh  ": `%mesh%`,
		"50%":      `%50\%%`,
		"a_b":      `%a\_b%`,
		`C:\x`:     `%C:\\x%`,
		"%_%":      `%\%\_\%%`,
	} {
		if got := paging.Contains(input); got != want {
			t.Errorf("Contains(%q) = %q, want %q", input, got, want)
		}
	}
}
