package paging

import (
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
		{Query{Sort: &Sort{Field: "id", Direction: "up"}}, cond, 1, apperr.Validation},
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
	if s, _ := OrderBy(&Sort{Field: "name", Direction: "desc"}, allowed, ""); s != "display_name DESC, id ASC" {
		t.Fatal(s)
	}
	if s, _ := OrderBy(&Sort{Field: "id", Direction: "desc"}, allowed, ""); s != "id DESC" {
		t.Fatal(s)
	}
	if _, err := OrderBy(&Sort{Field: "evil; drop", Direction: "asc"}, allowed, ""); code(err) != apperr.Validation {
		t.Fatal("unknown sort field")
	}
}
