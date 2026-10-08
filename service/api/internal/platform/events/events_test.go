package events

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/db"
)

func testDB(t *testing.T) *db.TxManager {
	t.Helper()
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}
	m, err := db.Open(context.Background(), url, "")
	if err != nil {
		t.Skip("database not available:", err)
	}
	t.Cleanup(m.Close)
	return m
}

func emit(t *testing.T, m *db.TxManager, tenant uuid.UUID, typ, payload string) uuid.UUID {
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
	c := &Consumer{Name: "test-" + uuid.NewString(), DB: m, Batch: 1, Handlers: map[string]Handler{typ: func(ctx context.Context, tx pgx.Tx, e Event) error {
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
