package events_test

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
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

var errNoDatabase = errors.New("no database")

// failingRunner is an ops.Runner whose database is unreachable.
type failingRunner struct{}

func (failingRunner) Run(context.Context, bool, *ops.Principal, func(pgx.Tx) error) error {
	return errNoDatabase
}

// A consumer without handlers has nothing to poll: Drain applies nothing and Run only waits for its context.
func TestConsumerWithoutHandlers(t *testing.T) {
	t.Parallel()

	consumer := &events.Consumer{
		Name:      "idle",
		DB:        nil,
		Handlers:  nil,
		Batch:     0,
		Logf:      nil,
		Now:       nil,
		SplitFrom: "",
	}

	n, err := consumer.Drain(context.Background())
	if n != 0 || err != nil {
		t.Fatalf("Drain without handlers: %d %v", n, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	done := make(chan struct{})

	go func() {
		consumer.Run(ctx, time.Millisecond) // polls until the context ends
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
	t.Parallel()

	var (
		mutex  sync.Mutex
		logged []string
	)

	consumer := &events.Consumer{
		Name: "broken",
		DB:   failingRunner{},
		Handlers: map[string]events.Handler{
			"X": func(context.Context, pgx.Tx, events.Event) error { return nil },
		},
		Logf: func(f string, a ...any) {
			mutex.Lock()

			logged = append(logged, fmt.Sprintf(f, a...))
			mutex.Unlock()
		}, Batch: 0, Now: nil, SplitFrom: "",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	consumer.Run(ctx, time.Millisecond)
	mutex.Lock()
	defer mutex.Unlock()

	if len(logged) == 0 || !strings.HasPrefix(logged[0], "consumer broken: ") ||
		!strings.Contains(logged[0], "no database") {
		t.Fatalf("failures are logged with the consumer name and the cause: %q", logged)
	}
}
