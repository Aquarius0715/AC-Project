package integration

import (
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
