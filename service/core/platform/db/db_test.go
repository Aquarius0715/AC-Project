package db

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
)

// Integration tests run against the compose PostgreSQL (docker compose --profile schema up postgres db-schema).
func testDB(t *testing.T) *TxManager {
	t.Helper()
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	m, err := Open(ctx, url, url)
	if err == nil {
		err = m.Writer.Ping(ctx)
	}
	if err != nil {
		t.Skip("database not available:", err)
	}
	t.Cleanup(m.Close)
	return m
}

func principal(t *testing.T, m *TxManager) *ops.Principal {
	t.Helper()
	p := &ops.Principal{TenantID: uuid.Must(uuid.NewV7()), MembershipID: uuid.Must(uuid.NewV7()), UserID: uuid.Must(uuid.NewV7())}
	p.Role = "admin"
	if _, err := m.Writer.Exec(context.Background(), `INSERT INTO platform.tenants (id, name) VALUES ($1, 'test')`, p.TenantID); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestIdempotencyLifecycle(t *testing.T) {
	m := testDB(t)
	ctx := context.Background()
	p := principal(t, m)
	s := Idempotency{M: m}
	key, hash := "key-"+uuid.NewString()[:8], "0a0b"
	if got, err := s.Begin(ctx, p, "units.save", key, hash); err != nil || got != nil {
		t.Fatalf("first begin: %v %s", err, got)
	}
	if _, err := s.Begin(ctx, p, "units.save", key, hash); apperr.From(err).Code != apperr.Conflict || apperr.From(err).RetryAfterSeconds == nil {
		t.Fatalf("in progress must conflict with retryAfter: %v", err)
	}
	if _, err := s.Begin(ctx, p, "units.save", key, "ffff"); apperr.From(err).Code != apperr.Conflict {
		t.Fatalf("other hash must conflict: %v", err)
	}
	if err := s.Complete(ctx, p, "units.save", key, json.RawMessage(`{"data":1}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Begin(ctx, p, "units.save", key, hash)
	if err != nil || string(got) != `{"data": 1}` && string(got) != `{"data":1}` {
		t.Fatalf("replay: %v %s", err, got)
	}
	// abort releases an in-progress key only
	k2 := "key-" + uuid.NewString()[:8]
	_, _ = s.Begin(ctx, p, "units.save", k2, hash)
	s.Abort(ctx, p, "units.save", k2)
	if got, err := s.Begin(ctx, p, "units.save", k2, hash); err != nil || got != nil {
		t.Fatalf("after abort: %v", err)
	}
	// expired keys are reusable
	if _, err := m.Writer.Exec(ctx, `UPDATE platform.idempotency_keys SET expires_at = now() - interval '1 second' WHERE key=$1`, key); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Begin(ctx, p, "units.list", key, "aa"); err != nil || got != nil {
		t.Fatalf("expired reuse: %v %s", err, got)
	}
	// bad hash and anonymous no-ops
	if _, err := s.Begin(ctx, p, "x", "key-bad-hash", "zz"); err == nil {
		t.Fatal("bad hex must fail")
	}
	if got, err := s.Begin(ctx, nil, "demo.reset", key, hash); got != nil || err != nil {
		t.Fatal("anonymous begin is a no-op")
	}
	if s.Complete(ctx, nil, "x", key, nil) != nil {
		t.Fatal("anonymous complete is a no-op")
	}
	s.Abort(ctx, nil, "x", key)
}

func TestRecorderAndRLS(t *testing.T) {
	m := testDB(t)
	ctx := context.Background()
	p := principal(t, m)
	other := principal(t, m)
	agg := uuid.Must(uuid.NewV7())
	v := 2
	c := &ops.Call{Principal: p, Now: time.Now().UTC(), CorrelationID: "corr-test-" + agg.String()[:8]}
	c.Audit(ops.AuditEntry{Action: "units.save", TargetKind: "unit", TargetID: agg.String(), NextVersion: &v, Reason: "r"})
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: agg, Type: "UnitChanged", Payload: map[string]int{"version": 2}})
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: agg, Type: "UnitArchived"})
	if err := m.Run(ctx, false, p, func(tx pgx.Tx) error { return Recorder{}.Record(ctx, tx, c, "units.save") }); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = m.Writer.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE aggregate_id=$1`, agg).Scan(&n)
	if n != 2 {
		t.Fatalf("outbox rows %d", n)
	}
	count := func(pp *ops.Principal) int {
		var k int
		if err := m.Run(ctx, true, pp, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE target_id=$1`, agg.String()).Scan(&k)
		}); err != nil {
			t.Fatal(err)
		}
		return k
	}
	if count(p) != 1 {
		t.Fatal("own tenant sees its audit row")
	}
	if count(other) != 0 {
		t.Fatal("RLS must hide another tenant's audit row")
	}
	// read-only transactions reject writes; anonymous recorder is a no-op
	err := m.Run(ctx, true, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO platform.tenants (name) VALUES ('x')`)
		return err
	})
	if apperr.From(err).Code != apperr.Unavailable {
		t.Fatalf("read-only write: %v", err)
	}
	if (Recorder{}).Record(ctx, nil, &ops.Call{}, "x") != nil {
		t.Fatal("anonymous record")
	}
	// audit log is append-only
	err = m.Run(ctx, false, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE audit.audit_log SET reason='x' WHERE target_id=$1`, agg.String())
		return err
	})
	if err == nil {
		t.Fatal("audit update must fail")
	}
}

func TestOpenErrors(t *testing.T) {
	if _, err := Open(context.Background(), "::bad", ""); err == nil {
		t.Fatal("bad writer url")
	}
	m := testDB(t)
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}
	if _, err := Open(context.Background(), url, "::bad"); err == nil {
		t.Fatal("bad reader url")
	}
	two, err := Open(context.Background(), url, url+"&application_name=reader")
	if err != nil || two.Reader == two.Writer {
		t.Fatal("separate reader pool")
	}
	two.Close()
	_ = m
}
