// Package paging implements Page<T> cursors (deterministic contracts: default 25, maximum 100; the cursor binds the
// normalized query conditions, scopeVersion, snapshotVersion and offset; changed conditions are VALIDATION).
package paging

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// Sort is Query.sort.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// Query is the common Query input (filters are decoded by each operation).
type Query struct {
	Cursor  *string         `json:"cursor,omitempty"`
	Limit   *int            `json:"limit,omitempty"`
	Sort    *Sort           `json:"sort,omitempty"`
	Filters json.RawMessage `json:"filters,omitempty"`
}

// Page is Page<T>.
type Page[T any] struct {
	Items           []T     `json:"items"`
	NextCursor      *string `json:"nextCursor"`
	Total           int     `json:"total"`
	SnapshotVersion int     `json:"snapshotVersion"`
}

type cursor struct {
	Hash     string `json:"h"`
	Scope    int    `json:"s"`
	Snapshot int    `json:"v"`
	Offset   int    `json:"o"`
}

// Window is the resolved paging window.
type Window struct {
	Limit, Offset, Snapshot int
	hash                    string
	scope                   int
}

// Resolve validates limit and cursor. conditions is the normalized filter+sort representation of the request.
func Resolve(q Query, conditions any, scopeVersion, snapshot int) (Window, error) {
	w := Window{Limit: DefaultLimit, Snapshot: snapshot, scope: scopeVersion}
	if q.Limit != nil {
		if *q.Limit < 1 || *q.Limit > MaxLimit {
			return w, apperr.Fields(map[string]string{"limit": "error.range"})
		}
		w.Limit = *q.Limit
	}
	if q.Sort != nil {
		if q.Sort.Direction != "asc" && q.Sort.Direction != "desc" {
			return w, apperr.Fields(map[string]string{"sort.direction": "error.invalid"})
		}
	}
	b, _ := json.Marshal(struct {
		C any   `json:"c"`
		S *Sort `json:"s"`
		L int   `json:"l"`
	}{conditions, q.Sort, w.Limit})
	sum := sha256.Sum256(b)
	w.hash = hex.EncodeToString(sum[:8])
	if q.Cursor == nil || *q.Cursor == "" {
		return w, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(*q.Cursor)
	var c cursor
	if err != nil || json.Unmarshal(raw, &c) != nil || c.Offset < 0 {
		return w, apperr.Fields(map[string]string{"cursor": "error.invalid"})
	}
	if c.Hash != w.hash {
		return w, apperr.Fields(map[string]string{"cursor": "error.queryChanged"})
	}
	if c.Scope != scopeVersion || c.Snapshot != snapshot {
		return w, apperr.E(apperr.Conflict, "error.cursorExpired")
	}
	w.Offset = c.Offset
	return w, nil
}

// Next returns the cursor for the following page or nil at the end.
func (w Window) Next(total int) *string {
	if w.Offset+w.Limit >= total {
		return nil
	}
	b, _ := json.Marshal(cursor{Hash: w.hash, Scope: w.scope, Snapshot: w.Snapshot, Offset: w.Offset + w.Limit})
	s := base64.RawURLEncoding.EncodeToString(b)
	return &s
}

// OrderBy maps a sort to SQL with the id tie-break; allowed maps API fields to SQL columns.
func OrderBy(s *Sort, allowed map[string]string, def string) (string, error) {
	if s == nil {
		return def, nil
	}
	col, ok := allowed[s.Field]
	if !ok {
		return "", apperr.Fields(map[string]string{"sort.field": "error.invalid"})
	}
	dir := strings.ToUpper(s.Direction)
	if s.Field == "id" {
		return col + " " + dir, nil
	}
	tie := "id"
	if idCol, ok := allowed["id"]; ok {
		tie = idCol // qualified tie-breaker, unambiguous in joins
	}
	return col + " " + dir + ", " + tie + " ASC", nil
}
