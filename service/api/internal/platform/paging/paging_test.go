package paging

import (
	"errors"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func ip(i int) *int       { return &i }
func sp(s string) *string { return &s }

func code(err error) apperr.Code {
	if err == nil {
		return ""
	}
	return apperr.From(err).Code
}

func TestResolveAndNext(t *testing.T) {
	cond := map[string]any{"propertyId": "p1"}
	w, err := Resolve(Query{}, cond, 1, 1)
	if err != nil || w.Limit != 25 || w.Offset != 0 {
		t.Fatalf("defaults: %+v %v", w, err)
	}
	if w.Next(25) != nil || w.Next(10) != nil {
		t.Fatal("no next page at the end")
	}
	w, _ = Resolve(Query{Limit: ip(2)}, cond, 1, 1)
	next := w.Next(5)
	if next == nil {
		t.Fatal("next")
	}
	w2, err := Resolve(Query{Limit: ip(2), Cursor: next}, cond, 1, 1)
	if err != nil || w2.Offset != 2 {
		t.Fatalf("second page: %+v %v", w2, err)
	}
	if w3 := w2.Next(5); w3 == nil {
		t.Fatal("third page")
	} else if w4, _ := Resolve(Query{Limit: ip(2), Cursor: w3}, cond, 1, 1); w4.Next(5) != nil || w4.Offset != 4 {
		t.Fatal("last page")
	}
	cases := []struct {
		q    Query
		cond any
		sv   int
		want apperr.Code
	}{
		{Query{Limit: ip(0)}, cond, 1, apperr.Validation},
		{Query{Limit: ip(101)}, cond, 1, apperr.Validation},
		{Query{Sort: &SortSpec{Field: "id", Direction: "up"}}, cond, 1, apperr.Validation},
		{Query{Limit: ip(2), Cursor: sp("!!")}, cond, 1, apperr.Validation},
		{Query{Limit: ip(2), Cursor: sp("e30")}, cond, 1, apperr.Validation},                          // {} → hash mismatch
		{Query{Limit: ip(2), Cursor: next}, map[string]any{"propertyId": "p2"}, 1, apperr.Validation}, // changed conditions
		{Query{Limit: ip(3), Cursor: next}, cond, 1, apperr.Validation},                               // changed limit
		{Query{Limit: ip(2), Cursor: next}, cond, 2, apperr.Conflict},                                 // scope changed
		{Query{Limit: ip(2), Cursor: sp("")}, cond, 1, ""},
	}
	for i, c := range cases {
		if _, err := Resolve(c.q, c.cond, c.sv, 1); code(err) != c.want {
			t.Errorf("case %d: got %v want %s", i, err, c.want)
		}
	}
}

func TestOrderBy(t *testing.T) {
	allowed := map[string]string{"id": "id", "name": "display_name"}
	if s, _ := OrderBy(nil, allowed, "id ASC"); s != "id ASC" {
		t.Fatal(s)
	}
	if s, _ := OrderBy(&SortSpec{Field: "name", Direction: "desc"}, allowed, ""); s != "display_name DESC, id ASC" {
		t.Fatal(s)
	}
	if s, _ := OrderBy(&SortSpec{Field: "id", Direction: "desc"}, allowed, ""); s != "id DESC" {
		t.Fatal(s)
	}
	if _, err := OrderBy(&SortSpec{Field: "evil; drop", Direction: "asc"}, allowed, ""); code(err) != apperr.Validation {
		t.Fatal("unknown sort field")
	}
}

// TestOrderByDirection: only asc and desc reach the ORDER BY; anything else — an injection attempt among them — is
// VALIDATION on sort.direction before any SQL is built (IR365).
func TestOrderByDirection(t *testing.T) {
	allowed := map[string]string{"id": "r.id", "createdAt": "r.created_at"}
	for dir, want := range map[string]string{"asc": "r.created_at ASC, r.id ASC", "desc": "r.created_at DESC, r.id ASC"} {
		if got, err := OrderBy(&SortSpec{Field: "createdAt", Direction: dir}, allowed, ""); err != nil || got != want {
			t.Errorf("%s: %q %v", dir, got, err)
		}
	}
	for _, dir := range []string{"DESC", "", "sideways", "desc, (SELECT pg_sleep(5))", "asc; DROP TABLE x --"} {
		got, err := OrderBy(&SortSpec{Field: "createdAt", Direction: dir}, allowed, "")
		var de *apperr.DomainError
		if got != "" || !errors.As(err, &de) || de.FieldErrors["sort.direction"] != "error.invalid" {
			t.Errorf("direction %q: %q %v", dir, got, err)
		}
	}
	if got, err := Direction(&SortSpec{Direction: "desc"}); err != nil || got != "DESC" {
		t.Errorf("Direction desc: %q %v", got, err)
	}
}

// TestContains: the search term is matched as written, anywhere in the value (IR388).
func TestContains(t *testing.T) {
	for in, want := range map[string]string{
		"filter":   `%filter%`,
		"  mesh  ": `%mesh%`,
		"50%":      `%50\%%`,
		"a_b":      `%a\_b%`,
		`C:\x`:     `%C:\\x%`,
		"%_%":      `%\%\_\%%`,
	} {
		if got := Contains(in); got != want {
			t.Errorf("Contains(%q) = %q, want %q", in, got, want)
		}
	}
}
