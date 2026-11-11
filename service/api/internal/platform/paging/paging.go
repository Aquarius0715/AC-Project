// Package paging implements Page<T> cursors (deterministic contracts: default 25, maximum 100; the cursor binds the
// normalized query conditions, scopeVersion, snapshotVersion and offset; changed conditions are VALIDATION).
package paging

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// The page sizes: the default and the largest a query may ask for.
const (
	DefaultLimit = 25
	MaxLimit     = 100
	// maxOffset bounds the offset a cursor may carry: the cursor is not signed, and an offset near the largest integer
	// would wrap Offset+Limit negative in the handlers that slice a list in memory.
	maxOffset = 1 << 31
)

// The sort directions, and the field that names a wrong one.
const (
	directionAsc       = "asc"
	directionDesc      = "desc"
	fieldSortDirection = "sort.direction"
)

// SortSpec is Query.sort.
type SortSpec struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// Query is the common Query input (filters are decoded by each operation).
type Query struct {
	Cursor  *string         `json:"cursor,omitempty"`
	Limit   *int            `json:"limit,omitempty"`
	Sort    *SortSpec       `json:"sort,omitempty"`
	Filters json.RawMessage `json:"filters,omitempty" swaggertype:"object"`
}

// HasFilters reports whether the query carries any filter (an empty object and null carry none).
func (q Query) HasFilters() bool {
	raw := strings.TrimSpace(string(q.Filters))

	return raw != "" && raw != "null" && raw != "{}"
}

// NoFilters is VALIDATION on field when an operation without catalogued filters receives some (SR06: unknown keys).
func NoFilters(q Query, field string) error {
	if q.HasFilters() {
		return apperr.Fields(map[string]string{field: apperr.KeyInvalid})
	}

	return nil
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
func Resolve(query Query, conditions any, scopeVersion, snapshot int) (Window, error) {
	window := Window{
		Limit:    DefaultLimit,
		Snapshot: snapshot,
		scope:    scopeVersion,
		Offset:   0,
		hash:     "",
	}

	limit, err := pageLimit(query)
	if err != nil {
		return window, err
	}

	window.Limit = limit

	if query.Sort != nil {
		if query.Sort.Direction != directionAsc && query.Sort.Direction != directionDesc {
			return window, apperr.Fields(map[string]string{fieldSortDirection: apperr.KeyInvalid})
		}
	}

	window.hash, err = queryHash(conditions, query.Sort, window.Limit)
	if err != nil {
		return window, err
	}

	if query.Cursor == nil || *query.Cursor == "" {
		return window, nil
	}

	window.Offset, err = cursorOffset(*query.Cursor, window.hash, scopeVersion, snapshot)

	return window, err
}

// Next returns the cursor for the following page or nil at the end.
func (w Window) Next(total int) *string {
	if w.Offset+w.Limit >= total {
		return nil
	}

	body, err := json.Marshal(
		cursor{Hash: w.hash, Scope: w.scope, Snapshot: w.Snapshot, Offset: w.Offset + w.Limit},
	)
	if err != nil {
		return nil // never: a cursor holds a string and integers, which always encode
	}

	s := base64.RawURLEncoding.EncodeToString(body)

	return &s
}

// pageLimit is the page size a query asks for: DefaultLimit when it asks for none.
func pageLimit(query Query) (int, error) {
	if query.Limit == nil {
		return DefaultLimit, nil
	}

	if *query.Limit < 1 || *query.Limit > MaxLimit {
		return DefaultLimit, apperr.Fields(map[string]string{"limit": apperr.KeyRange})
	}

	return *query.Limit, nil
}

// queryHash binds a cursor to the query: its conditions, sort and limit.
func queryHash(conditions any, sortSpec *SortSpec, limit int) (string, error) {
	body, err := json.Marshal(struct {
		C any       `json:"c"`
		S *SortSpec `json:"s"`
		L int       `json:"l"`
	}{conditions, sortSpec, limit})
	if err != nil {
		return "", apperr.Wrap(err, "json.Marshal")
	}

	sum := sha256.Sum256(body)

	return hex.EncodeToString(sum[:8]), nil
}

// cursorOffset is the offset in a cursor of the query with that hash, at the scope and snapshot versions of the
// request.
func cursorOffset(token, hash string, scopeVersion, snapshot int) (int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)

	var decoded cursor
	if err != nil || json.Unmarshal(raw, &decoded) != nil || decoded.Offset < 0 ||
		decoded.Offset > maxOffset {
		return 0, apperr.Fields(map[string]string{"cursor": apperr.KeyInvalid})
	}

	if decoded.Hash != hash {
		return 0, apperr.Fields(map[string]string{"cursor": "error.queryChanged"})
	}

	if decoded.Scope != scopeVersion || decoded.Snapshot != snapshot {
		return 0, apperr.E(apperr.Conflict, "error.cursorExpired")
	}

	return decoded.Offset, nil
}

// OrderBy maps a sort to SQL with the id tie-break; allowed maps API fields to SQL columns.
func OrderBy(sortSpec *SortSpec, allowed map[string]string, def string) (string, error) {
	if sortSpec == nil {
		return def, nil
	}

	col, ok := allowed[sortSpec.Field]
	if !ok {
		return "", apperr.Fields(map[string]string{"sort.field": apperr.KeyInvalid})
	}

	dir, err := Direction(sortSpec)
	if err != nil {
		return "", err
	}

	if sortSpec.Field == "id" {
		return col + " " + dir, nil
	}

	tie := "id"
	if idCol, ok := allowed["id"]; ok {
		tie = idCol // qualified tie-breaker, unambiguous in joins
	}

	return col + " " + dir + ", " + tie + " ASC", nil
}

// Direction is the SQL keyword of a sort direction. Only asc and desc ever reach a query; anything else is VALIDATION
// on sort.direction here, so no ORDER BY depends on Resolve having refused it first (IR365).
func Direction(s *SortSpec) (string, error) {
	switch s.Direction {
	case directionAsc:
		return "ASC", nil
	case directionDesc:
		return "DESC", nil
	}

	return "", apperr.Fields(map[string]string{fieldSortDirection: apperr.KeyInvalid})
}

// likeEscape takes the LIKE wildcards of a search term literally; the backslash is PostgreSQL's default escape
// character.
func likeEscape() *strings.Replacer {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
}

// Contains is the LIKE / ILIKE pattern of filters.search: the trimmed term anywhere in the value. A %, _ or \ in the
// term is matched as itself, so "50%" finds "50%" and not every value with "50" in it (IR388).
func Contains(search string) string {
	return "%" + likeEscape().Replace(strings.TrimSpace(search)) + "%"
}

// SortSlice orders in-memory items by the requested sort: keys compares two items per allowed field (like
// strings.Compare); ties fall back to the "id" comparator when there is one. A nil sort keeps the current order; a
// field outside keys is VALIDATION.
func SortSlice[T any](items []T, sortSpec *SortSpec, keys map[string]func(a, b T) int) error {
	if sortSpec == nil {
		return nil
	}

	cmp, ok := keys[sortSpec.Field]
	if !ok {
		return apperr.Fields(map[string]string{"sort.field": apperr.KeyInvalid})
	}

	if sortSpec.Direction != directionAsc && sortSpec.Direction != directionDesc {
		return apperr.Fields(map[string]string{fieldSortDirection: apperr.KeyInvalid})
	}

	byID := keys["id"]

	slices.SortStableFunc(items, func(left, right T) int {
		order := cmp(left, right)
		if sortSpec.Direction == directionDesc {
			order = -order
		}

		if order == 0 && byID != nil && sortSpec.Field != "id" {
			order = byID(left, right)
		}

		return order
	})

	return nil
}
