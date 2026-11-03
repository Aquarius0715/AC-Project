package democlock

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// scratch creates an empty database with only platform.demo_clock, so the shared row of the dev database "ac"
// (used by a running local API) is never touched.
func scratch(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx := context.Background()

	admin := os.Getenv("AC_TEST_ADMIN_DATABASE_URL")
	if admin == "" {
		admin = "postgres://postgres:local@localhost:5432/postgres?sslmode=disable"
	}

	ac, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	const name = "ac_democlock_test"
	for _, q := range []string{"DROP DATABASE IF EXISTS " + name + " WITH (FORCE)", "CREATE DATABASE " + name} {
		_, err := ac.Exec(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
	}

	cfg, _ := pgxpool.ParseConfig(admin)
	cfg.ConnConfig.Database = name

	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := p.Exec(
		ctx,
		`CREATE SCHEMA platform; CREATE TABLE platform.demo_clock (id boolean PRIMARY KEY DEFAULT true CHECK (id), offset_ms
			bigint NOT NULL, updated_at timestamptz NOT NULL DEFAULT now())`,
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	t.Cleanup(func() {
		p.Close()
		ac.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") //nolint:errcheck
		ac.Close(ctx)
	})

	return p
}

func TestSharedAcrossProcessesAndRestarts(t *testing.T) {
	ctx := context.Background()
	p := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	base := func() time.Time { return wall }
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

	api, err := Open(ctx, p, base, start)
	if err != nil {
		t.Fatal(err)
	}

	if got := api.Now(); !got.Equal(start) {
		t.Fatalf("first open: %v, want %v", got, start)
	}
	// the API jumps forward; a worker that opens later with the same start keeps the stored offset
	advanceErr := api.Advance(ctx, 2*time.Hour)
	if advanceErr != nil {
		t.Fatal(advanceErr)
	}

	worker, err := Open(ctx, p, base, start)
	if err != nil {
		t.Fatal(err)
	}

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
	again, err := Open(ctx, p, base, start)
	if err != nil {
		t.Fatal(err)
	}

	if got := again.Now(); !got.Equal(want) {
		t.Fatalf("restart %v, want %v", got, want)
	}
}

func TestNowKeepsOffsetWhenDatabaseFails(t *testing.T) {
	ctx := context.Background()
	p := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	c, err := Open(ctx, p, func() time.Time { return wall }, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := p.Exec(ctx, `UPDATE platform.demo_clock SET offset_ms = 60000`)
	if execErr != nil {
		t.Fatal(execErr)
	}

	p.Close()

	wall = wall.Add(5 * time.Second)
	if got := c.Now(); !got.Equal(wall) {
		t.Fatalf("Now = %v, want the cached zero offset %v", got, wall)
	}
}

func TestMissingRowKeepsOffset(t *testing.T) {
	ctx := context.Background()
	p := scratch(t)
	wall := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	start := time.Date(2026, 9, 14, 1, 0, 0, 0, time.UTC)

	c, err := Open(ctx, p, func() time.Time { return wall }, start)
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := p.Exec(ctx, `DELETE FROM platform.demo_clock`)
	if execErr != nil {
		t.Fatal(execErr)
	}

	wall = wall.Add(5 * time.Second)
	if got := c.Now(); !got.Equal(start.Add(5 * time.Second)) {
		t.Fatalf("Now = %v", got)
	}
}

func TestStartFromEnv(t *testing.T) {
	s, err := StartFromEnv(
		func(string) string { return "" },
	)
	if err != nil ||
		s.Format(time.RFC3339) != DefaultStart {
		t.Fatalf("default = %v, %v", s, err)
	}

	_, err = StartFromEnv(func(string) string { return "soon" })
	if err == nil {
		t.Fatal("want parse error")
	}
}
