package events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// testRunner is the minimal ops.Runner of these tests (the db package depends on events): one transaction with the
// tenant context set.
type testRunner struct{ pool *pgxpool.Pool }

func (r testRunner) Run(ctx context.Context, _ bool, p *ops.Principal, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if p != nil {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", p.TenantID.String()); err != nil {
				return err
			}
		}
		return fn(tx)
	})
}

func testDB(t *testing.T) testRunner {
	t.Helper()
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err == nil {
		err = pool.Ping(context.Background())
	}
	if err != nil {
		t.Skip("database not available:", err)
	}
	t.Cleanup(pool.Close)
	return testRunner{pool}
}

func emit(t *testing.T, m testRunner, tenant uuid.UUID, typ, payload string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := m.Run(context.Background(), false, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id)
			VALUES ($1, 'test', $2, $3, $4, 'corr') RETURNING id`, tenant, uuid.New(), typ, payload).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestConsumerOrderDedupAndRetry(t *testing.T) {
	ctx := context.Background()
	m := testDB(t)
	var tenant uuid.UUID
	if err := m.Run(ctx, true, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM platform.tenants ORDER BY id LIMIT 1`).Scan(&tenant)
	}); err != nil {
		t.Skip("no tenant seeded:", err)
	}
	typ := "Test" + uuid.NewString()[:8] // event types unique to this run
	first := emit(t, m, tenant, typ, `{"n":1}`)
	second := emit(t, m, tenant, typ, `{"n":2}`)
	emit(t, m, tenant, typ+"Other", `{}`)

	var seen []int
	fail := true
	clock := time.Now()
	c := &Consumer{Name: "test-" + uuid.NewString(), DB: m, Batch: 1, Now: func() time.Time { return clock }, Handlers: map[string]Handler{typ: func(ctx context.Context, tx pgx.Tx, e Event) error {
		var p struct{ N int }
		if err := e.Decode(&p); err != nil {
			return err
		}
		var got string
		if err := tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).Scan(&got); err != nil || got != tenant.String() {
			return errors.New("tenant context missing")
		}
		if p.N == 2 && fail {
			return errors.New("transient")
		}
		seen = append(seen, p.N)
		return nil
	}}}
	n, err := c.Drain(ctx)
	if err == nil || n != 1 || len(seen) != 1 || seen[0] != 1 {
		t.Fatalf("first drain: n=%d seen=%v err=%v", n, seen, err)
	}
	fail = false
	// the retry waits a minute after the failure (IR427)
	if n, err := c.Drain(ctx); err != nil || n != 0 || len(seen) != 1 {
		t.Fatalf("before the pause: n=%d seen=%v err=%v", n, seen, err)
	}
	clock = clock.Add(time.Minute)
	if n, err := c.Drain(ctx); err != nil || n != 1 || len(seen) != 2 || seen[1] != 2 {
		t.Fatalf("retry: n=%d seen=%v err=%v", n, seen, err)
	}
	if n, err := c.Drain(ctx); err != nil || n != 0 {
		t.Fatalf("nothing pending: n=%d err=%v", n, err)
	}
	// a second consumer gets its own copy of every event
	other := &Consumer{Name: "test-" + uuid.NewString(), DB: m, Handlers: map[string]Handler{typ: func(context.Context, pgx.Tx, Event) error { return nil }}}
	if n, err := other.Drain(ctx); err != nil || n != 2 {
		t.Fatalf("independent consumer: n=%d err=%v", n, err)
	}
	// replaying an event that is already recorded does nothing
	if done, err := c.apply(ctx, Event{ID: first, TenantID: tenant, Type: typ, Payload: []byte(`{"n":9}`)}); err != nil || done || len(seen) != 2 {
		t.Fatalf("replay: done=%v seen=%v err=%v", done, seen, err)
	}
	_ = second
}

// TestConsumerDeadLetters (IR427): an event that fails MaxAttempts times, 1, 2, 4 and 8 minutes apart, is recorded as
// dead and skipped, and the events after it are applied; until then they wait behind it.
func TestConsumerDeadLetters(t *testing.T) {
	ctx := context.Background()
	m := testDB(t)
	var tenant uuid.UUID
	if err := m.Run(ctx, true, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM platform.tenants ORDER BY id LIMIT 1`).Scan(&tenant)
	}); err != nil {
		t.Skip("no tenant seeded:", err)
	}
	typ := "Poison" + uuid.NewString()[:8]
	poison := emit(t, m, tenant, typ, `{"n":1}`)
	emit(t, m, tenant, typ, `{"n":2}`)
	clock := time.Now()
	var applied []int
	var logged []string
	c := &Consumer{Name: "test-" + uuid.NewString(), DB: m, Now: func() time.Time { return clock },
		Logf: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) },
		Handlers: map[string]Handler{typ: func(_ context.Context, _ pgx.Tx, e Event) error {
			var p struct{ N int }
			_ = e.Decode(&p)
			if p.N == 1 {
				return errors.New("never applies")
			}
			applied = append(applied, p.N)
			return nil
		}}}
	for i, pause := range []time.Duration{0, time.Minute, 2 * time.Minute, 4 * time.Minute} {
		clock = clock.Add(pause)
		if _, err := c.Drain(ctx); err == nil || len(applied) != 0 {
			t.Fatalf("attempt %d: err=%v applied=%v", i+1, err, applied)
		}
	}
	clock = clock.Add(8*time.Minute - time.Second)
	if n, err := c.Drain(ctx); n != 0 || err != nil {
		t.Fatalf("before the last pause: n=%d err=%v", n, err)
	}
	clock = clock.Add(time.Second)
	if n, err := c.Drain(ctx); n != 1 || err != nil || len(applied) != 1 || applied[0] != 2 {
		t.Fatalf("fifth attempt: n=%d err=%v applied=%v", n, err, applied)
	}
	var attempts int
	var dead bool
	if err := m.Run(ctx, true, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT attempts, dead_at IS NOT NULL FROM platform.event_failures WHERE consumer = $1 AND event_id = $2`, c.Name, poison).Scan(&attempts, &dead)
	}); err != nil || attempts != MaxAttempts || !dead {
		t.Fatalf("failure record: attempts=%d dead=%v err=%v", attempts, dead, err)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "dead-lettered") {
		t.Errorf("logged: %q", logged)
	}
	if n, err := c.Drain(ctx); n != 0 || err != nil {
		t.Errorf("after the dead letter: n=%d err=%v", n, err)
	}
}
