package events

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

var (
	errTenantContextMissing = errors.New("tenant context missing")
	errTransient            = errors.New("transient")
	errNeverApplies         = errors.New("never applies")
)

// testRunner is the minimal ops.Runner of these tests (the db package depends on events): one transaction with the
// tenant context set.
type testRunner struct{ pool *pgxpool.Pool }

func (r testRunner) Run(
	ctx context.Context,
	_ bool,
	principal *ops.Principal,
	callback func(pgx.Tx) error,
) error {
	return apperr.Wrap(pgx.BeginFunc(ctx, r.pool, func(txn pgx.Tx) error {
		if principal != nil {
			_, err := txn.Exec(
				ctx,
				"SELECT set_config('app.tenant_id', $1, true)",
				principal.TenantID.String(),
			)
			if err != nil {
				return apperr.Wrap(err, "select set_config")
			}
		}

		return callback(txn)
	}), "transaction")
}

func testDB(t *testing.T) testRunner {
	t.Helper()

	url := os.Getenv("AC_TEST_DATABASE_URL")
	if url == "" {
		//nolint:gosec // local test database password, not a secret
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

// firstTenant is the seeded tenant the test events belong to; the test is skipped without one.
func firstTenant(t *testing.T, runner testRunner) uuid.UUID {
	t.Helper()

	ctx := context.Background()

	var tenant uuid.UUID

	err := runner.Run(ctx, true, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM platform.tenants ORDER BY id LIMIT 1`).Scan(&tenant)
	})
	if err != nil {
		t.Skip("no tenant seeded:", err)
	}

	return tenant
}

// tenantPrincipal is the context of a transaction in the tenant, as a consumer applies an event.
func tenantPrincipal(tenant uuid.UUID) *ops.Principal {
	return &ops.Principal{
		TenantID:          tenant,
		Principal:         authz.Principal{Role: "", Permissions: nil, ClientRole: ""},
		MembershipID:      uuid.UUID{},
		UserID:            uuid.UUID{},
		OrgID:             uuid.UUID{},
		ScopeVersion:      0,
		Scopes:            nil,
		Employment:        "",
		Timezone:          "",
		SessionsRevokedAt: nil,
	}
}

func emit(t *testing.T, runner testRunner, tenant uuid.UUID, typ, payload string) uuid.UUID {
	t.Helper()

	var eventID uuid.UUID

	err := runner.Run(context.Background(), false, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id,
			event_type, payload, correlation_id)
			VALUES ($1, 'test', $2, $3, $4, 'corr') RETURNING id`, tenant, uuid.New(), typ, payload).
			Scan(&eventID)
	})
	if err != nil {
		t.Fatal(err)
	}

	return eventID
}

// drainExpect runs one Drain and fails the test unless it applied want events and returned an error exactly when
// wantErr is set.
func drainExpect(t *testing.T, consumer *Consumer, want int, wantErr bool, step string) {
	t.Helper()

	applied, err := consumer.Drain(context.Background())
	if applied != want || (err != nil) != wantErr {
		t.Fatalf("%s: n=%d err=%v", step, applied, err)
	}
}

// orderHandler is the handler of TestConsumerOrderDedupAndRetry: it checks the tenant context, fails event 2 while
// fail is set and records the events it applied.
type orderHandler struct {
	tenant uuid.UUID
	fail   bool
	seen   []int
}

func (h *orderHandler) apply(ctx context.Context, txn pgx.Tx, e Event) error {
	var payload struct{ N int }

	err := e.Decode(&payload)
	if err != nil {
		return err
	}

	var got string

	err = txn.QueryRow(ctx, `SELECT current_setting('app.tenant_id')`).
		Scan(&got)
	if err != nil ||
		got != h.tenant.String() {
		return errTenantContextMissing
	}

	if payload.N == 2 && h.fail {
		return errTransient
	}

	h.seen = append(h.seen, payload.N)

	return nil
}

// expectSeen fails the test unless the handler applied exactly the events want, in order.
func (h *orderHandler) expectSeen(t *testing.T, step string, want ...int) {
	t.Helper()

	if !slices.Equal(h.seen, want) {
		t.Fatalf("%s: seen=%v, want %v", step, h.seen, want)
	}
}

//nolint:paralleltest // shares the test database with the other DB tests
func TestConsumerOrderDedupAndRetry(t *testing.T) {
	runner := testDB(t)
	tenant := firstTenant(t, runner)

	typ := "Test" + uuid.NewString()[:8] // event types unique to this run
	first := emit(t, runner, tenant, typ, `{"n":1}`)
	second := emit(t, runner, tenant, typ, `{"n":2}`)
	emit(t, runner, tenant, typ+"Other", `{}`)

	handler := &orderHandler{tenant: tenant, fail: true, seen: nil}
	clock := time.Now()
	consumer := &Consumer{
		Name:      "test-" + uuid.NewString(),
		DB:        runner,
		Batch:     1,
		Now:       func() time.Time { return clock },
		Handlers:  map[string]Handler{typ: handler.apply},
		Logf:      nil,
		SplitFrom: "",
	}

	drainExpect(t, consumer, 1, true, "first drain")
	handler.expectSeen(t, "first drain", 1)

	handler.fail = false
	// the retry waits a minute after the failure (IR427)
	drainExpect(t, consumer, 0, false, "before the pause")
	handler.expectSeen(t, "before the pause", 1)

	clock = clock.Add(time.Minute)

	drainExpect(t, consumer, 1, false, "retry")
	handler.expectSeen(t, "retry", 1, 2)
	drainExpect(t, consumer, 0, false, "nothing pending")
	// a second consumer gets its own copy of every event
	other := &Consumer{
		Name: "test-" + uuid.NewString(),
		DB:   runner,
		Handlers: map[string]Handler{
			typ: func(context.Context, pgx.Tx, Event) error { return nil },
		}, Batch: 0, Logf: nil, Now: nil, SplitFrom: "",
	}
	drainExpect(t, other, 2, false, "independent consumer")
	// replaying an event that is already recorded does nothing
	checkReplayIgnored(t, consumer, first, tenant, typ)
	handler.expectSeen(t, "replay", 1, 2)

	_ = second
}

// checkReplayIgnored applies an event the consumer has recorded already: the handler does not run again.
func checkReplayIgnored(t *testing.T, consumer *Consumer, eventID, tenant uuid.UUID, typ string) {
	t.Helper()

	done, applyErr := consumer.apply(
		context.Background(),
		Event{
			ID:            eventID,
			TenantID:      tenant,
			Type:          typ,
			Payload:       []byte(`{"n":9}`),
			Seq:           0,
			AggregateType: "",
			AggregateID:   uuid.UUID{},
			CorrelationID: "",
			OccurredAt:    time.Time{},
			failures:      0,
			lastFailure:   nil,
		},
	)
	if applyErr != nil || done {
		t.Fatalf("replay: done=%v err=%v", done, applyErr)
	}
}

// TestConsumerDeadLetters (IR427): an event that fails MaxAttempts times, 1, 2, 4 and 8 minutes apart, is recorded as
// dead and skipped, and the events after it are applied; until then they wait behind it.
//
//nolint:paralleltest // shares the test database with the other DB tests
func TestConsumerDeadLetters(t *testing.T) {
	runner := testDB(t)
	tenant := firstTenant(t, runner)

	typ := "Poison" + uuid.NewString()[:8]
	poison := emit(t, runner, tenant, typ, `{"n":1}`)
	emit(t, runner, tenant, typ, `{"n":2}`)

	clock := time.Now()

	var (
		applied []int
		logged  []string
	)

	consumer := &Consumer{
		Name: "test-" + uuid.NewString(), DB: runner, Now: func() time.Time { return clock },
		Logf: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) },
		Handlers: map[string]Handler{typ: func(_ context.Context, _ pgx.Tx, e Event) error {
			var payload struct{ N int }

			_ = e.Decode(&payload)
			if payload.N == 1 {
				return errNeverApplies
			}

			applied = append(applied, payload.N)

			return nil
		}}, Batch: 0, SplitFrom: "",
	}

	for i, pause := range []time.Duration{0, time.Minute, 2 * time.Minute, 4 * time.Minute} {
		clock = clock.Add(pause)

		_, err := consumer.Drain(context.Background())
		if err == nil || len(applied) != 0 {
			t.Fatalf("attempt %d: err=%v applied=%v", i+1, err, applied)
		}
	}

	clock = clock.Add(8*time.Minute - time.Second)

	drainExpect(t, consumer, 0, false, "before the last pause")

	clock = clock.Add(time.Second)

	drainExpect(t, consumer, 1, false, "fifth attempt")

	if !slices.Equal(applied, []int{2}) {
		t.Fatalf("fifth attempt: applied=%v", applied)
	}

	checkDeadLetter(t, runner, consumer.Name, poison)

	if len(logged) != 1 || !strings.Contains(logged[0], "dead-lettered") {
		t.Errorf("logged: %q", logged)
	}

	drainExpect(t, consumer, 0, false, "after the dead letter")
}

// checkDeadLetter fails the test unless the consumer's failure record of the event counts MaxAttempts and is dead.
func checkDeadLetter(t *testing.T, runner testRunner, consumer string, eventID uuid.UUID) {
	t.Helper()

	ctx := context.Background()

	var (
		attempts int
		dead     bool
	)

	err := runner.Run(ctx, true, nil, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT attempts, dead_at IS NOT NULL FROM platform.event_failures WHERE consumer = $1 AND
			event_id = $2`, consumer, eventID).
			Scan(&attempts, &dead)
	})
	if err != nil || attempts != MaxAttempts || !dead {
		t.Fatalf("failure record: attempts=%d dead=%v err=%v", attempts, dead, err)
	}
}

// execAs runs one statement in its own transaction, in the principal's tenant when there is one.
func execAs(t *testing.T, runner testRunner, principal *ops.Principal, sql string, args ...any) {
	t.Helper()

	ctx := context.Background()

	err := runner.Run(
		ctx,
		false,
		principal,
		func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)

			return apperr.Wrap(err, "exec (TestHousekeep)")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
}

// countAs runs one count query in its own transaction, in the principal's tenant when there is one.
func countAs(
	t *testing.T,
	runner testRunner,
	principal *ops.Principal,
	sql string,
	args ...any,
) int {
	t.Helper()

	ctx := context.Background()

	var count int

	err := runner.Run(
		ctx,
		true,
		principal,
		func(tx pgx.Tx) error { return tx.QueryRow(ctx, sql, args...).Scan(&count) },
	)
	if err != nil {
		t.Fatal(err)
	}

	return count
}

// TestHousekeep (IR428): an old event every subscriber applied is deleted, one a subscriber still needs stays; the
// record of an event no longer in the outbox goes once it is old enough; an expired idempotency key goes. The test
// works in the year 2000 so that the rows of other tests (written at the test clock) are never old enough.
//
//nolint:paralleltest // shares the test database with the other DB tests
func TestHousekeep(t *testing.T) {
	runner := testDB(t)
	tenant := firstTenant(t, runner)
	rows := sweptRows{
		typ:       "Old" + uuid.NewString()[:8],
		consumerA: "test-" + uuid.NewString(),
		consumerB: "test-" + uuid.NewString(),
		applied:   uuid.New(),
		needed:    uuid.New(),
		gone:      uuid.New(),
		key:       "hk-" + uuid.NewString(),
	}

	rows.insert(t, runner, tenant)

	noop := func(context.Context, pgx.Tx, Event) error { return nil }
	consumers := []*Consumer{
		{
			Name:      rows.consumerA,
			Handlers:  map[string]Handler{rows.typ: noop},
			DB:        nil,
			Batch:     0,
			Logf:      nil,
			Now:       nil,
			SplitFrom: "",
		},
		{
			Name:      rows.consumerB,
			Handlers:  map[string]Handler{rows.typ: noop},
			DB:        nil,
			Batch:     0,
			Logf:      nil,
			Now:       nil,
			SplitFrom: "",
		},
	}

	swept, err := Housekeep(context.Background(), runner, housekeepDay(20), consumers)
	if err != nil {
		t.Fatal(err)
	}

	checkSwept(t, runner, tenant, rows)

	if swept.Outbox < 1 || swept.Processed < 1 || swept.Failures < 1 || swept.IdempotencyKeys < 1 {
		t.Errorf("counts: %+v", swept)
	}

	execAs(
		t,
		runner,
		nil,
		`DELETE FROM platform.processed_events WHERE consumer = ANY($1)`,
		[]string{rows.consumerA, rows.consumerB},
	)
	execAs(t, runner, nil, `DELETE FROM platform.outbox WHERE id = $1`, rows.needed)
}

// housekeepDay is day d of January 2000, the clock of TestHousekeep.
func housekeepDay(d int) time.Time { return time.Date(2000, 1, d, 0, 0, 0, 0, time.UTC) }

// sweptRows are the rows TestHousekeep writes: events of one type applied by both consumers (applied), not yet by
// consumer B (needed) and gone from the outbox (gone), and an expired idempotency key.
type sweptRows struct {
	typ                   string
	consumerA, consumerB  string
	applied, needed, gone uuid.UUID
	key                   string
}

// insert writes the rows at housekeepDay 10 (the idempotency key expired on day 2).
func (rows sweptRows) insert(t *testing.T, runner testRunner, tenant uuid.UUID) {
	t.Helper()

	for _, eventID := range []uuid.UUID{rows.applied, rows.needed} {
		execAs(
			t,
			runner,
			nil,
			`INSERT INTO platform.outbox (id, tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id,
				occurred_at)
			VALUES ($1, $2, 'test', $1, $3, '{}', 'corr', $4)`,
			eventID,
			tenant,
			rows.typ,
			housekeepDay(10),
		)
	}

	execAs(
		t,
		runner,
		nil,
		`INSERT INTO platform.processed_events (consumer, event_id, processed_at) VALUES ($1, $2, $4), ($3, $2, $4), ($1, $5,
			$4), ($1, $6, $7)`,
		rows.consumerA,
		rows.applied,
		rows.consumerB,
		housekeepDay(10),
		rows.needed,
		rows.gone,
		housekeepDay(1),
	) // b has not applied "needed"; "gone" left the outbox and is 19 days old
	execAs(
		t,
		runner,
		nil,
		`INSERT INTO platform.event_failures (consumer, event_id, attempts, last_error, last_attempt_at) VALUES ($1, $2, 1,
			'x', $3)`,
		rows.consumerA,
		rows.gone,
		housekeepDay(10),
	)
	execAs(
		t,
		runner,
		tenantPrincipal(tenant),
		`INSERT INTO platform.idempotency_keys (tenant_id, membership_id, key, operation, request_hash, status, created_at,
			expires_at)
		VALUES ($1, $2, $3, 'test', '\x00', 'completed', $4, $5)`,
		tenant,
		uuid.New(),
		rows.key,
		housekeepDay(1),
		housekeepDay(2),
	)
}

// checkSwept checks what Housekeep left of the rows of TestHousekeep.
func checkSwept(t *testing.T, runner testRunner, tenant uuid.UUID, rows sweptRows) {
	t.Helper()

	if left := countAs(
		t,
		runner,
		nil,
		`SELECT count(*) FROM platform.outbox WHERE id = ANY($1)`,
		[]uuid.UUID{rows.applied, rows.needed},
	); left != 1 ||
		countAs(
			t,
			runner,
			nil,
			`SELECT count(*) FROM platform.outbox WHERE id = $1`,
			rows.needed,
		) != 1 {
		t.Errorf("outbox after the sweep: %d left (want only the event b still needs)", left)
	}
	// the records of the deleted event are kept until they are 14 days old; "gone" and its failure record go now
	if left := countAs(
		t,
		runner,
		nil,
		`SELECT count(*) FROM platform.processed_events WHERE event_id = $1`,
		rows.gone,
	); left != 0 {
		t.Errorf("record of an event no longer in the outbox: %d", left)
	}

	if left := countAs(
		t,
		runner,
		nil,
		`SELECT count(*) FROM platform.processed_events WHERE event_id = $1`,
		rows.applied,
	); left != 2 {
		t.Errorf("records of the event deleted now: %d (kept until 14 days old)", left)
	}

	if left := countAs(
		t,
		runner,
		nil,
		`SELECT count(*) FROM platform.event_failures WHERE event_id = $1`,
		rows.gone,
	); left != 0 {
		t.Errorf("failure record of an event no longer in the outbox: %d", left)
	}

	if left := countAs(
		t,
		runner,
		tenantPrincipal(tenant),
		`SELECT count(*) FROM platform.idempotency_keys WHERE key = $1`,
		rows.key,
	); left != 0 {
		t.Errorf("expired idempotency key: %d", left)
	}
}
