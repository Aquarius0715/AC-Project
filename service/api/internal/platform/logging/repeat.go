package logging

import (
	"sync"
	"time"
)

// Repeats keeps a polling loop's failure log readable (IR429): the same failure is logged once, then at most once per
// Every with how often it recurred, and the first success after a failure is logged as a recovery. Without it a loop
// that polls every second logs a line per poll for as long as the database is away.
type Repeats struct {
	Logf  func(string, ...any)
	Every time.Duration // default one minute
	Now   func() time.Time

	mu       sync.Mutex
	last     string
	loggedAt time.Time
	repeated int
}

func (r *Repeats) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Failed logs msg unless it repeats the failure logged less than Every ago.
func (r *Repeats) Failed(msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	every := r.Every
	if every <= 0 {
		every = time.Minute
	}
	now := r.now()
	if msg == r.last && now.Sub(r.loggedAt) < every {
		r.repeated++
		return
	}
	if msg == r.last && r.repeated > 0 {
		r.Logf("%s (repeated %d times)", msg, r.repeated)
	} else {
		r.Logf("%s", msg)
	}
	r.last, r.loggedAt, r.repeated = msg, now, 0
}

// OK notes a success: after a failure it logs the recovery once.
func (r *Repeats) OK(what string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.last == "" {
		return
	}
	r.Logf("%s recovered", what)
	r.last, r.repeated = "", 0
}
