package logging

import (
	"fmt"
	"testing"
	"time"
)

// TestRepeats: a repeated failure is logged once a minute with its count, another failure at once, and the recovery
// once.
func TestRepeats(t *testing.T) {
	var lines []string
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	r := &Repeats{Logf: func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) }, Now: func() time.Time { return now }}
	r.OK("tick") // nothing failed yet: nothing to say
	for i := 0; i < 30; i++ {
		r.Failed("tick failed: db down")
		now = now.Add(time.Second)
	}
	now = now.Add(31 * time.Second)
	r.Failed("tick failed: db down")
	r.Failed("tick failed: other")
	r.OK("tick")
	r.OK("tick")
	want := []string{"tick failed: db down", "tick failed: db down (repeated 29 times)", "tick failed: other", "tick recovered"}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("lines:\n%q\nwant\n%q", lines, want)
	}
}
