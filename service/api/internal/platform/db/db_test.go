package db_test

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
	"github.com/pradita/ac-project/service/api/internal/platform/db"
)

// unitKind is the aggregate type and audit target kind of the recorded operation.
const unitKind = "unit"

// Integration tests run against the compose PostgreSQL (docker compose --profile schema up postgres db-schema).
func testDB(t *testing.T) *db.TxManager {
	t.Helper()

	url := testURL()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	txm, err := db.Open(ctx, url, url)
	if err == nil {
		err = txm.Writer.Ping(ctx)
	}

	if err != nil {
		t.Skip("database not available:", err)
	}

	t.Cleanup(txm.Close)

	return txm
}

// testURL is the application login of the test database.
func testURL() string {
	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		//nolint:gosec // local test database password, not a secret
		url = "postgres://ac_app_login:local@localhost:5432/ac_test?sslmode=disable"
	}

	return url
}

func principal(t *testing.T, txm *db.TxManager) *ops.Principal {
	t.Helper()

	caller := &ops.Principal{
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

	caller.Role = "admin"

	_, err := txm.Writer.Exec(
		context.Background(),
		`INSERT INTO platform.tenants (id, name) VALUES ($1, 'test')`,
		caller.TenantID,
	)
	if err != nil {
		t.Fatal(err)
	}

	return caller
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestIdempotencyLifecycle(t *testing.T) {
	txm := testDB(t)
	caller := principal(t, txm)
	idempotency := db.Idempotency{M: txm}

	key, hash := "key-"+uuid.NewString()[:8], "0a0b"

	checkClaimAndReplay(t, idempotency, caller, key, hash)
	checkAbortReleases(t, idempotency, caller, hash)
	checkExpiredReuse(t, txm, caller, key)
	checkAnonymousNoOps(t, idempotency, caller, key, hash)
}

// checkClaimAndReplay claims a new key, is refused while it is in progress or with another hash, and replays the
// completed response.
func checkClaimAndReplay(
	t *testing.T,
	idempotency db.Idempotency,
	caller *ops.Principal,
	key, hash string,
) {
	t.Helper()

	ctx := context.Background()

	got, err := idempotency.Begin(ctx, caller, "units.save", key, hash)
	if err != nil || got != nil {
		t.Fatalf("first begin: %v %s", err, got)
	}

	_, err = idempotency.Begin(
		ctx,
		caller,
		"units.save",
		key,
		hash,
	)
	if apperr.From(err).Code != apperr.Conflict ||
		apperr.From(err).RetryAfterSeconds == nil {
		t.Fatalf("in progress must conflict with retryAfter: %v", err)
	}

	_, err = idempotency.Begin(
		ctx,
		caller,
		"units.save",
		key,
		"ffff",
	)
	if apperr.From(
		err,
	).Code != apperr.Conflict {
		t.Fatalf("other hash must conflict: %v", err)
	}

	err = idempotency.Complete(ctx, caller, "units.save", key, json.RawMessage(`{"data":1}`))
	if err != nil {
		t.Fatal(err)
	}

	got, err = idempotency.Begin(ctx, caller, "units.save", key, hash)
	if err != nil || string(got) != `{"data": 1}` && string(got) != `{"data":1}` {
		t.Fatalf("replay: %v %s", err, got)
	}
}

// checkAbortReleases shows that abort releases an in-progress key only.
func checkAbortReleases(
	t *testing.T,
	idempotency db.Idempotency,
	caller *ops.Principal,
	hash string,
) {
	t.Helper()

	ctx := context.Background()
	k2 := "key-" + uuid.NewString()[:8]
	_, _ = idempotency.Begin(ctx, caller, "units.save", k2, hash)
	idempotency.Abort(ctx, caller, "units.save", k2)

	beginGot, beginErr := idempotency.Begin(ctx, caller, "units.save", k2, hash)
	if beginErr != nil || beginGot != nil {
		t.Fatalf("after abort: %v", beginErr)
	}
}

// checkExpiredReuse shows that expired keys are reusable (the keys are under the tenant's row-level security,
// IR377: edit them in its transaction).
func checkExpiredReuse(t *testing.T, txm *db.TxManager, caller *ops.Principal, key string) {
	t.Helper()

	ctx := context.Background()

	runErr := txm.Run(ctx, false, caller, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`UPDATE platform.idempotency_keys SET expires_at = now() - interval '1 second' WHERE key=$1`,
			key,
		)

		return apperr.Wrap(err, "update platform.idempotency_keys")
	})
	if runErr != nil {
		t.Fatal(runErr)
	}

	beginGot, beginErr := db.Idempotency{M: txm}.Begin(ctx, caller, "units.list", key, "aa")
	if beginErr != nil || beginGot != nil {
		t.Fatalf("expired reuse: %v %s", beginErr, beginGot)
	}
}

// checkAnonymousNoOps covers a bad hash and the no-ops without a principal.
func checkAnonymousNoOps(
	t *testing.T,
	idempotency db.Idempotency,
	caller *ops.Principal,
	key, hash string,
) {
	t.Helper()

	ctx := context.Background()

	_, beginErr := idempotency.Begin(ctx, caller, "x", "key-bad-hash", "zz")
	if beginErr == nil {
		t.Fatal("bad hex must fail")
	}

	beginGot, beginErr := idempotency.Begin(ctx, nil, "demo.reset", key, hash)
	if beginGot != nil || beginErr != nil {
		t.Fatal("anonymous begin is a no-op")
	}

	if idempotency.Complete(ctx, nil, "x", key, nil) != nil {
		t.Fatal("anonymous complete is a no-op")
	}

	idempotency.Abort(ctx, nil, "x", key)
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestRecorderAndRLS(t *testing.T) {
	txm := testDB(t)
	caller := principal(t, txm)
	other := principal(t, txm)
	agg := uuid.Must(uuid.NewV7())

	checkRecorded(t, txm, caller, agg)
	checkAuditRLS(t, txm, caller, other, agg)
	checkRefusedWrites(t, txm, caller, agg)
}

// recordedCall is a units.save call with one audit entry and two events of the unit agg.
func recordedCall(caller *ops.Principal, agg uuid.UUID) *ops.Call {
	nextVersion := 2
	call := &ops.Call{
		Principal:       caller,
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
	call.Audit(
		ops.AuditEntry{
			Action:      "units.save",
			TargetKind:  unitKind,
			TargetID:    agg.String(),
			NextVersion: &nextVersion,
			Reason:      "r", PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)
	call.Emit(
		ops.Event{
			AggregateType: unitKind,
			AggregateID:   agg,
			Type:          "UnitChanged",
			Payload:       map[string]int{"version": 2},
		},
	)
	call.Emit(
		ops.Event{AggregateType: unitKind, AggregateID: agg, Type: "UnitArchived", Payload: nil},
	)

	return call
}

// checkRecorded records the call and finds its events and its audit entry in the outbox.
func checkRecorded(t *testing.T, txm *db.TxManager, caller *ops.Principal, agg uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	call := recordedCall(caller, agg)

	runErr := txm.Run(
		ctx,
		false,
		caller,
		func(tx pgx.Tx) error { return db.Recorder{}.Record(ctx, tx, call, "units.save") },
	)
	if runErr != nil {
		t.Fatal(runErr)
	}

	var outboxRows int

	_ = txm.Writer.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE aggregate_id=$1`, agg).
		Scan(&outboxRows)
	if outboxRows != 2 {
		t.Fatalf("outbox rows %d", outboxRows)
	}
	// the audit entry is published for identity's consumer in the same transaction (IR196)
	_ = txm.Writer.QueryRow(ctx, `SELECT count(*) FROM platform.outbox WHERE event_type = 'AuditRecorded' AND tenant_id =
		$1
		AND payload->>'targetId' = $2
		AND payload->>'action' = 'units.save' AND payload->>'reason' = 'r' AND (payload->>'nextVersion')::int =
		2`, caller.TenantID, agg.String()).
		Scan(&outboxRows)
	if outboxRows != 1 {
		t.Fatalf("AuditRecorded events %d", outboxRows)
	}
}

// checkAuditRLS: row-level security on the audit log.
func checkAuditRLS(t *testing.T, txm *db.TxManager, caller, other *ops.Principal, agg uuid.UUID) {
	t.Helper()

	ctx := context.Background()

	runErr := txm.Run(ctx, false, caller, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, action, target_kind, target_id, occurred_at,
				correlation_id, result)
			VALUES (current_setting('app.tenant_id')::uuid, 'test', 'admin', 'units.save', 'unit', $1, now(), 'corr',
			'success')`,
			agg.String(),
		)

		return apperr.Wrap(err, "insert audit.audit_log")
	})
	if runErr != nil {
		t.Fatal(runErr)
	}

	count := func(pp *ops.Principal) int {
		var auditRows int

		err := txm.Run(ctx, true, pp, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE target_id=$1`, agg.String()).
				Scan(&auditRows)
		})
		if err != nil {
			t.Fatal(err)
		}

		return auditRows
	}
	if count(caller) != 1 {
		t.Fatal("own tenant sees its audit row")
	}

	if count(other) != 0 {
		t.Fatal("RLS must hide another tenant's audit row")
	}
}

// checkRefusedWrites: read-only transactions reject writes, the anonymous recorder is a no-op and the audit log is
// append-only.
func checkRefusedWrites(t *testing.T, txm *db.TxManager, caller *ops.Principal, agg uuid.UUID) {
	t.Helper()

	ctx := context.Background()

	err := txm.Run(ctx, true, caller, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO platform.tenants (name) VALUES ('x')`)

		return apperr.Wrap(err, "insert platform.tenants")
	})
	if apperr.From(err).Code != apperr.Unavailable {
		t.Fatalf("read-only write: %v", err)
	}

	if (db.Recorder{}).Record(
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
	err = txm.Run(ctx, false, caller, func(tx pgx.Tx) error {
		_, err := tx.Exec(
			ctx,
			`UPDATE audit.audit_log SET reason='x' WHERE target_id=$1`,
			agg.String(),
		)

		return apperr.Wrap(err, "update audit.audit_log")
	})
	if err == nil {
		t.Fatal("audit update must fail")
	}
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestOpenErrors(t *testing.T) {
	_, err := db.Open(context.Background(), "::bad", "")
	if err == nil {
		t.Fatal("bad writer url")
	}

	txm := testDB(t)

	url := testURL()

	_, err = db.Open(context.Background(), url, "::bad")
	if err == nil {
		t.Fatal("bad reader url")
	}

	two, err := db.Open(context.Background(), url, url+"&application_name=reader")
	if err != nil || two.Reader == two.Writer {
		t.Fatal("separate reader pool")
	}

	two.Close()

	_ = txm
}
