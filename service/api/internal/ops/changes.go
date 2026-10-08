package ops

import (
	"encoding/json"
	"strings"
)

// Changes returns the fields whose value differs between before and after, as display strings (strings as they are,
// other values as JSON, nil as null): the AuditEntry Before / After of a write (DD-A16 before/after values).
func Changes(before, after map[string]any) (map[string]*string, map[string]*string) {
	b, a := map[string]*string{}, map[string]*string{}
	keys := map[string]bool{}
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}
	for k := range keys {
		x, y := show(before[k]), show(after[k])
		if (x == nil) != (y == nil) || (x != nil && *x != *y) {
			b[k], a[k] = x, y
		}
	}
	return b, a
}

func show(v any) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	if s, ok := v.(*string); ok {
		return s
	}
	raw, err := json.Marshal(v)
	if err != nil || string(raw) == "null" {
		return nil
	}
	s := string(raw)
	return &s
}

// sensitive are the field name parts whose values never reach the audit log in clear (DD-A16: mask secrets and
// contacts in before/after).
var sensitive = []string{"password", "secret", "token", "phone", "email", "contact", "totp", "iban", "card"}

// Masked replaces sensitive values with ***masked*** (a changed sensitive field stays visible as changed).
func Masked(m map[string]*string) map[string]*string {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]*string, len(m))
	for k, v := range m {
		lk := strings.ToLower(k)
		for _, s := range sensitive {
			if v != nil && strings.Contains(lk, s) {
				masked := "***masked***"
				v = &masked
				break
			}
		}
		out[k] = v
	}
	return out
}
