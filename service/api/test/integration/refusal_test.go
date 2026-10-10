package integration

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// refusal checks that a VALIDATION response names the field a case is about, so a body that fails on another rule
// does not pass; want is "field=messageKey" (a case without one fails).
func refusal(t *testing.T, name string, code int, m map[string]any, want string) {
	t.Helper()
	field, key, ok := strings.Cut(want, "=")
	fe, _ := m["fieldErrors"].(map[string]any)
	if !ok || code != 422 || fe[field] != key {
		t.Errorf("%s: %d %v, want %s", name, code, fe, want)
	}
}

// fields lists a response's field errors as "field=key" pairs in field order, joined by commas.
func fields(m map[string]any) string {
	fe, _ := m["fieldErrors"].(map[string]any)
	out := make([]string, 0, len(fe))
	for k, v := range fe {
		out = append(out, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}
