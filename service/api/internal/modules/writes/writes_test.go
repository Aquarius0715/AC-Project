package writes

import (
	"strings"
	"testing"
)

// TestIdempotencyKeyCountsCharacters: the key is 8–128 characters, as the contract says, not bytes; a key of 40 emoji is
// 40 characters (160 bytes) and is accepted.
func TestIdempotencyKeyCountsCharacters(t *testing.T) {
	cases := map[string]struct {
		key string
		ok  bool
	}{
		"7 characters":         {strings.Repeat("a", 7), false},
		"8 characters":         {strings.Repeat("a", 8), true},
		"128 characters":       {strings.Repeat("a", 128), true},
		"129 characters":       {strings.Repeat("a", 129), false},
		"40 emoji (160 bytes)": {strings.Repeat("😀", 40), true},
		"129 emoji":            {strings.Repeat("😀", 129), false},
	}
	for name, c := range cases {
		fe := (&Input{Operation: "jobs.create", IdempotencyKey: c.key}).Validate()
		if got := fe["idempotencyKey"] == ""; got != c.ok {
			t.Errorf("%s: accepted=%v, want %v (fields %v)", name, got, c.ok, fe)
		}
	}
}
