package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// failingRunner is an ops.Runner whose database is unreachable.
type failingRunner struct{}

func (failingRunner) Run(context.Context, bool, *ops.Principal, func(pgx.Tx) error) error {
	return errors.New("no database")
}

// A consumer without handlers has nothing to poll: Drain applies nothing and Run only waits for its context.
func TestConsumerWithoutHandlers(t *testing.T) {
	c := &Consumer{Name: "idle"}
	if n, err := c.Drain(context.Background()); n != 0 || err != nil {
		t.Fatalf("Drain without handlers: %d %v", n, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		c.Run(ctx, time.Millisecond) // polls until the context ends
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after the context was cancelled")
	}
}

// A failing poll is logged with the consumer name and does not stop the loop.
func TestConsumerRunLogsFailures(t *testing.T) {
	var mu sync.Mutex
	var logged []string
	c := &Consumer{Name: "broken", DB: failingRunner{}, Handlers: map[string]Handler{"X": func(context.Context, pgx.Tx, Event) error { return nil }},
		Logf: func(f string, a ...any) {
			mu.Lock()
			logged = append(logged, fmt.Sprintf(f, a...))
			mu.Unlock()
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	c.Run(ctx, time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(logged) == 0 || !strings.Contains(logged[0], "consumer broken: no database") {
		t.Fatalf("failures are logged with the consumer name: %q", logged)
	}
}
