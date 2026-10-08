// Package democlock is the demo scenario clock shared by the Core API and the workers (IR36, IR168): wall-clock time
// plus one offset stored in platform.demo_clock, so every process sees the same business time, demo.advanceClock
// jumps are seen by the scheduler, and a restart never moves the clock backwards. Production never creates the row.
package democlock

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// refreshEvery bounds how stale another process's advance can be in this process.
const refreshEvery = time.Second

// Clock reads the shared offset (cached for refreshEvery) and adds it to the base clock.
type Clock struct {
	pool *pgxpool.Pool
	base func() time.Time

	mu       sync.Mutex
	offset   time.Duration
	loadedAt time.Time
}

// Open makes sure the shared row exists (the first process starts the scenario at start; later processes keep the
// stored offset) and returns the clock.
func Open(ctx context.Context, pool *pgxpool.Pool, base func() time.Time, start time.Time) (*Clock, error) {
	c := &Clock{pool: pool, base: base}
	off := time.Duration(0)
	if !start.IsZero() {
		off = start.Sub(base())
	}
	if _, err := pool.Exec(ctx, `INSERT INTO platform.demo_clock (id, offset_ms) VALUES (true, $1) ON CONFLICT (id) DO NOTHING`, off.Milliseconds()); err != nil {
		return nil, err
	}
	if err := c.load(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Clock) load(ctx context.Context) error {
	var ms int64
	err := c.pool.QueryRow(ctx, `SELECT offset_ms FROM platform.demo_clock WHERE id`).Scan(&ms)
	if errors.Is(err, pgx.ErrNoRows) { // the row was removed (database rebuilt): keep the known offset
		c.mu.Lock()
		c.loadedAt = c.base()
		c.mu.Unlock()
		return nil
	}
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.offset, c.loadedAt = time.Duration(ms)*time.Millisecond, c.base()
	c.mu.Unlock()
	return nil
}

// Now is the scenario time. A failed refresh keeps the last known offset.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	stale := c.base().Sub(c.loadedAt) >= refreshEvery
	c.mu.Unlock()
	if stale {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = c.load(ctx)
		cancel()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base().Add(c.offset).UTC()
}

// Advance moves the scenario clock forward by d for every process.
func (c *Clock) Advance(ctx context.Context, d time.Duration) error {
	var ms int64
	if err := c.pool.QueryRow(ctx, `UPDATE platform.demo_clock SET offset_ms = offset_ms + $1, updated_at = now() WHERE id RETURNING offset_ms`, d.Milliseconds()).Scan(&ms); err != nil {
		return err
	}
	c.mu.Lock()
	c.offset, c.loadedAt = time.Duration(ms)*time.Millisecond, c.base()
	c.mu.Unlock()
	return nil
}

// DefaultStart is fixture.clock (docs/04-agentic-sdlc/fixture-contract.json).
const DefaultStart = "2026-09-14T01:00:00Z"

// StartFromEnv is the scenario start: DEMO_CLOCK_START, else fixture.clock. Every process uses the same value, so
// whichever starts first creates the same row.
func StartFromEnv(getenv func(string) string) (time.Time, error) {
	v := getenv("DEMO_CLOCK_START")
	if v == "" {
		v = DefaultStart
	}
	return time.Parse(time.RFC3339, v)
}
