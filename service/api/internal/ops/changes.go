package ops

import (
	"encoding/json"
	"strings"
	"time"
)

// Changes returns the fields whose value differs between before and after, as display strings (strings as they are,
// other values as JSON, nil as null): the AuditEntry Before / After of a write (DD-A16 before/after values).
func Changes(before, after map[string]any) (map[string]*string, map[string]*string) {
	changedBefore, changedAfter := map[string]*string{}, map[string]*string{}

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
			changedBefore[k], changedAfter[k] = x, y
		}
	}

	return changedBefore, changedAfter
}

func show(value any) *string {
	if value == nil {
		return nil
	}

	if s, ok := value.(string); ok {
		return &s
	}

	if s, ok := value.(*string); ok {
		return s
	}
	// times compare as instants: a +08:00 input and the UTC value read back are the same time
	if t, ok := value.(*time.Time); ok && t != nil {
		value = *t
	}

	if t, ok := value.(time.Time); ok {
		value = t.UTC()
	}

	raw, err := json.Marshal(value)
	if err != nil || string(raw) == "null" {
		return nil
	}

	text := string(raw)

	var str string // values that marshal to a JSON string (IDs, times) are shown without the quotes
	if json.Unmarshal(raw, &str) == nil {
		text = str
	}

	return &text
}

// sensitive returns the field name parts whose values never reach the audit log in clear (DD-A16: mask secrets and
// contacts in before/after).
func sensitive() []string {
	return []string{
		"password",
		"secret",
		"token",
		"phone",
		"email",
		"contact",
		"totp",
		"iban",
		"card",
	}
}

// Masked replaces sensitive values with ***masked*** (a changed sensitive field stays visible as changed).
func Masked(values map[string]*string) map[string]*string {
	if len(values) == 0 {
		return nil
	}

	parts := sensitive()

	out := make(map[string]*string, len(values))
	for field, value := range values {
		lk := strings.ToLower(field)
		for _, part := range parts {
			if value != nil && strings.Contains(lk, part) {
				masked := "***masked***"
				value = &masked

				break
			}
		}

		out[field] = value
	}

	return out
}
