package democlock_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
)

// scratch creates an empty database with only platform.demo_clock, so the shared row of the dev database "ac"
// (used by a running local API) is never touched.
func scratch(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	admin := os.Getenv("AC_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		//nolint:gosec // local test database password, not a secret
		admin = "postgres://postgres:local@localhost:5432/postgres?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	const name = "ac_democlock_test"
	for _, q := range []string{"DROP DATABASE IF EXISTS " + name + " WITH (FORCE)", "CREATE DATABASE " + name} {
		_, err := conn.Exec(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg, _ := pgxpool.ParseConfig(admin)
	cfg.ConnConfig.Database = name

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := pool.Exec(
		ctx,
		`CREATE SCHEMA platform; CREATE TABLE platform.demo_clock (id boolean PRIMARY KEY DEFAULT true CHECK (id), offset_ms
			bigint NOT NULL, updated_at timestamptz NOT NULL DEFAULT now())`,
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	t.Cleanup(func() {
		pool.Close()

		_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		_ = conn.Close(ctx)
	})

	return pool
}

// open opens a clock on the scratch database and fails the test if it cannot.
func open(
	t *testing.T,
	pool *pgxpool.Pool,
	base func() time.Time,
	start time.Time,
) *democlock.Clock {
	t.Helper()

	clock, err := democlock.Open(t.Context(), pool, base, start)
	if err != nil {
		t.Fatal(err)
	}

	return clock
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_democlock_test
func TestSharedAcrossProcessesAndRestarts(t *testing.T) {
	ctx := context.Background()
	pool := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	base := func() time.Time { return wall }
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

	api := open(t, pool, base, start)
	if got := api.Now(); !got.Equal(start) {
		t.Fatalf("first open: %v, want %v", got, start)
	}
	// the API jumps forward; a worker that opens later with the same start keeps the stored offset
	advanceErr := api.Advance(ctx, 2*time.Hour)
	if advanceErr != nil {
		t.Fatal(advanceErr)
	}

	worker := open(t, pool, base, start)

	if got := worker.Now(); !got.Equal(start.Add(2 * time.Hour)) {
		t.Fatalf("worker sees %v", got)
	}
	// wall time passes: both clocks run in real time and the worker picks up a later jump after refreshEvery
	wall = wall.Add(10 * time.Second)

	advanceErr = api.Advance(ctx, time.Hour)
	if advanceErr != nil {
		t.Fatal(advanceErr)
	}

	want := start.Add(3*time.Hour + 10*time.Second)
	if got := api.Now(); !got.Equal(want) {
		t.Fatalf("api %v, want %v", got, want)
	}

	if got := worker.Now(); !got.Equal(want) {
		t.Fatalf("worker %v, want %v", got, want)
	}
	// a restarted API never moves the clock back to the scenario start
	again := open(t, pool, base, start)
	if got := again.Now(); !got.Equal(want) {
		t.Fatalf("restart %v, want %v", got, want)
	}
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_democlock_test
func TestNowKeepsOffsetWhenDatabaseFails(t *testing.T) {
	ctx := context.Background()
	pool := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	clock, err := democlock.Open(ctx, pool, func() time.Time { return wall }, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := pool.Exec(ctx, `UPDATE platform.demo_clock SET offset_ms = 60000`)
	if execErr != nil {
		t.Fatal(execErr)
	}

	pool.Close()

	wall = wall.Add(5 * time.Second)
	if got := clock.Now(); !got.Equal(wall) {
		t.Fatalf("Now = %v, want the cached zero offset %v", got, wall)
	}
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_democlock_test
func TestMissingRowKeepsOffset(t *testing.T) {
	ctx := context.Background()
	pool := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

	clock, err := democlock.Open(ctx, pool, func() time.Time { return wall }, start)
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := pool.Exec(ctx, `DELETE FROM platform.demo_clock`)
	if execErr != nil {
		t.Fatal(execErr)
	}

	wall = wall.Add(5 * time.Second)
	if got := clock.Now(); !got.Equal(start.Add(5 * time.Second)) {
		t.Fatalf("Now = %v", got)
	}
}

// TestRealTimeStart (DEC-96): a scenario that starts on the real clock (the zero start of DEMO_CLOCK_START=now) is
// stored at offset 0, so every process reads the base clock until demo.advanceClock moves it.
//
//nolint:paralleltest // every test rebuilds the same scratch database ac_democlock_test
func TestRealTimeStart(t *testing.T) {
	pool := scratch(t)
	wall := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

	clock := open(t, pool, func() time.Time { return wall }, time.Time{})
	if got := clock.Now(); !got.Equal(wall) {
		t.Fatalf("Now = %v, want the base clock %v", got, wall)
	}

	var offsetMS int64

	err := pool.QueryRow(t.Context(), `SELECT offset_ms FROM platform.demo_clock`).Scan(&offsetMS)
	if err != nil || offsetMS != 0 {
		t.Fatalf("stored offset %d ms, %v; want 0", offsetMS, err)
	}
}

func TestStartFromEnv(t *testing.T) {
	t.Parallel()

	start, err := democlock.StartFromEnv(
		func(string) string { return "" },
	)
	if err != nil ||
		start.Format(time.RFC3339) != democlock.DefaultStart {
		t.Fatalf("default = %v, %v", start, err)
	}

	start, err = democlock.StartFromEnv(func(string) string { return democlock.RealTime })
	if err != nil || !start.IsZero() {
		t.Fatalf("now = %v, %v; want the zero start", start, err)
	}

	_, err = democlock.StartFromEnv(func(string) string { return "soon" })
	if err == nil {
		t.Fatal("want parse error")
	}
}
