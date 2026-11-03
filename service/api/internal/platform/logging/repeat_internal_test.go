package logging

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestRepeats: a repeated failure is logged once a minute with its count, another failure at once, and the recovery
// once.
func TestRepeats(t *testing.T) {
	t.Parallel()

	var lines []string

	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	repeats := &Repeats{
		Logf:     func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) },
		Now:      func() time.Time { return now },
		Every:    0,
		mu:       sync.Mutex{},
		last:     "",
		loggedAt: time.Time{},
		repeated: 0,
	}
	repeats.OK("tick") // nothing failed yet: nothing to say

	for range 30 {
		repeats.Failed("tick failed: db down")

		now = now.Add(time.Second)
	}

	now = now.Add(31 * time.Second)

	repeats.Failed("tick failed: db down")
	repeats.Failed("tick failed: other")
	repeats.OK("tick")
	repeats.OK("tick")

	want := []string{
		"tick failed: db down",
		"tick failed: db down (repeated 29 times)",
		"tick failed: other",
		"tick recovered",
	}
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("lines:\n%q\nwant\n%q", lines, want)
	}
}
