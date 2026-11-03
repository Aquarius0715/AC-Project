package db

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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

	p := &ops.Principal{
		TenantID:     uuid.Must(uuid.NewV7()),
		MembershipID: uuid.Must(uuid.NewV7()),
		UserID: uuid.Must(
			uuid.NewV7(),
		),
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}

	p.Role = "admin"
	_, err := m.Writer.Exec(
		context.Background(),
		`INSERT INTO platform.tenants (id, name) VALUES ($1, 'test')`,
		p.TenantID,
	)
	if err != nil {
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
	got, err := s.Begin(ctx, p, "units.save", key, hash)
	if err != nil || got != nil {
		t.Fatalf("first begin: %v %s", err, got)
	}

	_, err = s.Begin(
		ctx,
		p,
		"units.save",
		key,
		hash,
	)
	if apperr.From(err).Code != apperr.Conflict ||
		apperr.From(err).RetryAfterSeconds == nil {
		t.Fatalf("in progress must conflict with retryAfter: %v", err)
	}

	_, err = s.Begin(
		ctx,
		p,
		"units.save",
		key,
		"ffff",
	)
	if apperr.From(
		err,
	).Code != apperr.Conflict {
		t.Fatalf("other hash must conflict: %v", err)
	}

	err = s.Complete(ctx, p, "units.save", key, json.RawMessage(`{"data":1}`))
	if err != nil {
		t.Fatal(err)
	}

	got, err = s.Begin(ctx, p, "units.save", key, hash)
	if err != nil || string(got) != `{"data": 1}` && string(got) != `{"data":1}` {
		t.Fatalf("replay: %v %s", err, got)
	}
	// abort releases an in-progress key only
	k2 := "key-" + uuid.NewString()[:8]
	_, _ = s.Begin(ctx, p, "units.save", k2, hash)
	s.Abort(ctx, p, "units.save", k2)

	beginGot, beginErr := s.Begin(ctx, p, "units.save", k2, hash)
	if beginErr != nil || beginGot != nil {
		t.Fatalf("after abort: %v", beginErr)
	}
	// expired keys are reusable (the keys are under the tenant's row-level security, IR377: edit them in its transaction)
	runErr := m.Run(ctx, false, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`UPDATE platform.idempotency_keys SET expires_at = now() - interval '1 second' WHERE key=$1`,
			key,
		)

		return err
	})
	if runErr != nil {
		t.Fatal(runErr)
	}

	beginGot, beginErr = s.Begin(ctx, p, "units.list", key, "aa")
	if beginErr != nil || beginGot != nil {
		t.Fatalf("expired reuse: %v %s", beginErr, beginGot)
	}
	// bad hash and anonymous no-ops
	_, beginErr = s.Begin(ctx, p, "x", "key-bad-hash", "zz")
	if beginErr == nil {
		t.Fatal("bad hex must fail")
	}

	beginGot, beginErr = s.Begin(ctx, nil, "demo.reset", key, hash)
	if beginGot != nil || beginErr != nil {
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
	c := &ops.Call{
		Principal:       p,
		Now:             time.Now().UTC(),
		CorrelationID:   "corr-test-" + agg.String()[:8],
		Tx:              nil,
		Bearer:          "",
		Queries:         nil,
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	}
	c.Audit(
		ops.AuditEntry{
			Action:      "units.save",
			TargetKind:  "unit",
			TargetID:    agg.String(),
			NextVersion: &v,
			Reason:      "r", PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)
	c.Emit(
		ops.Event{
			AggregateType: "unit",
			AggregateID:   agg,
			Type:          "UnitChanged",
			Payload:       map[string]int{"version": 2},
		},
	)
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: agg, Type: "UnitArchived", Payload: nil})

	runErr := m.Run(
		ctx,
		false,
		p,
		func(tx pgx.Tx) error { return Recorder{}.Record(ctx, tx, c, "units.save") },
	)
	if runErr != nil {
		t.Fatal(runErr)
	}

	var n int

	_ = m.Writer.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE aggregate_id=$1`, agg).
		Scan(&n)
	if n != 2 {
		t.Fatalf("outbox rows %d", n)
	}
	// the audit entry is published for identity's consumer in the same transaction (IR196)
	_ = m.Writer.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE event_type = 'AuditRecorded' AND tenant_id = $1 AND payload->>'targetId' = $2
		AND payload->>'action' = 'units.save' AND payload->>'reason' = 'r' AND (payload->>'nextVersion')::int = 2`, p.TenantID, agg.String()).
		Scan(&n)
	if n != 1 {
		t.Fatalf("AuditRecorded events %d", n)
	}
	// row-level security on the audit log
	runErr = m.Run(ctx, false, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, action, target_kind, target_id, occurred_at, correlation_id, result)
			VALUES (current_setting('app.tenant_id')::uuid, 'test', 'admin', 'units.save', 'unit', $1, now(), 'corr', 'success')`,
			agg.String(),
		)

		return err
	})
	if runErr != nil {
		t.Fatal(runErr)
	}

	count := func(pp *ops.Principal) int {
		var k int

		err := m.Run(ctx, true, pp, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE target_id=$1`, agg.String()).
				Scan(&k)
		})
		if err != nil {
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

	if (Recorder{}).Record(
		ctx,
		nil,
		&ops.Call{
			Tx:              nil,
			Principal:       nil,
			Now:             time.Time{},
			CorrelationID:   "",
			Bearer:          "",
			Queries:         nil,
			ExpectedVersion: nil,
			Candidates:      nil,
			Events:          nil,
			Audits:          nil,
		},
		"x",
	) != nil {
		t.Fatal("anonymous record")
	}
	// audit log is append-only
	err = m.Run(ctx, false, p, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`UPDATE audit.audit_log SET reason='x' WHERE target_id=$1`,
			agg.String(),
		)

		return err
	})
	if err == nil {
		t.Fatal("audit update must fail")
	}
}

func TestOpenErrors(t *testing.T) {
	_, err := Open(context.Background(), "::bad", "")
	if err == nil {
		t.Fatal("bad writer url")
	}

	m := testDB(t)

	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}

	_, err = Open(context.Background(), url, "::bad")
	if err == nil {
		t.Fatal("bad reader url")
	}

	two, err := Open(context.Background(), url, url+"&application_name=reader")
	if err != nil || two.Reader == two.Writer {
		t.Fatal("separate reader pool")
	}

	two.Close()

	_ = m
}
