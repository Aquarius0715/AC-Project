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

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// refreshEvery bounds how stale another process's advance can be in this process.
const refreshEvery = time.Second

// refreshTimeout bounds one refresh of the offset from the database.
const refreshTimeout = 2 * time.Second

// Clock reads the shared offset (cached for refreshEvery) and adds it to the base clock.
type Clock struct {
	pool *pgxpool.Pool
	base func() time.Time

	mu       sync.Mutex
	offset   time.Duration
	loadedAt time.Time
}

// Open makes sure the shared row exists (the first process starts the scenario at start, or on the base clock when
// start is zero; later processes keep the stored offset) and returns the clock.
func Open(
	ctx context.Context,
	pool *pgxpool.Pool,
	base func() time.Time,
	start time.Time,
) (*Clock, error) {
	clock := &Clock{pool: pool, base: base, mu: sync.Mutex{}, offset: 0, loadedAt: time.Time{}}

	off := time.Duration(0)
	if !start.IsZero() {
		off = start.Sub(base())
	}

	_, err := pool.Exec(
		ctx,
		`INSERT INTO platform.demo_clock (id, offset_ms) VALUES (true, $1) ON CONFLICT (id) DO NOTHING`,
		off.Milliseconds(),
	)
	if err != nil {
		return nil, apperr.Wrap(err, "insert platform.demo_clock")
	}

	err = clock.load(ctx)
	if err != nil {
		return nil, err
	}

	return clock, nil
}

// Now is the scenario time. A failed refresh keeps the last known offset.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	stale := c.base().Sub(c.loadedAt) >= refreshEvery
	c.mu.Unlock()

	if stale {
		ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
		_ = c.load(ctx)

		cancel()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	return c.base().Add(c.offset).UTC()
}

// Advance moves the scenario clock forward by d for every process.
func (c *Clock) Advance(ctx context.Context, d time.Duration) error {
	var offsetMS int64

	err := c.pool.QueryRow(ctx, `UPDATE platform.demo_clock SET offset_ms = offset_ms + $1, updated_at = now() WHERE id
		RETURNING offset_ms`, d.Milliseconds()).
		Scan(&offsetMS)
	if err != nil {
		return apperr.Wrap(err, "update platform.demo_clock")
	}

	c.mu.Lock()
	c.offset, c.loadedAt = time.Duration(offsetMS)*time.Millisecond, c.base()
	c.mu.Unlock()

	return nil
}

func (c *Clock) load(ctx context.Context) error {
	var offsetMS int64

	err := c.pool.QueryRow(ctx, `SELECT offset_ms FROM platform.demo_clock WHERE id`).
		Scan(&offsetMS)
	if errors.Is(
		err,
		pgx.ErrNoRows,
	) { // the row was removed (database rebuilt): keep the known offset
		c.mu.Lock()
		c.loadedAt = c.base()
		c.mu.Unlock()

		return nil
	}

	if err != nil {
		return apperr.Wrap(err, "select platform.demo_clock")
	}

	c.mu.Lock()
	c.offset, c.loadedAt = time.Duration(offsetMS)*time.Millisecond, c.base()
	c.mu.Unlock()

	return nil
}

// DefaultStart is fixture.clock (docs/04-agentic-sdlc/fixture-contract.json).
const DefaultStart = "2026-09-14T01:00:00Z"

// RealTime is the DEMO_CLOCK_START that runs the scenario on the real clock: the shared offset is 0 (the shared test
// environment, whose data are the real devices', DEC-96).
const RealTime = "now"

// StartFromEnv is the scenario start: DEMO_CLOCK_START, else fixture.clock; the zero time for RealTime, which Open
// starts at offset 0. Every process uses the same value, so whichever starts first creates the same row.
func StartFromEnv(getenv func(string) string) (time.Time, error) {
	v := getenv("DEMO_CLOCK_START")
	if v == "" {
		v = DefaultStart
	}

	if v == RealTime {
		return time.Time{}, nil
	}

	out, err := time.Parse(time.RFC3339, v)

	return out, apperr.Wrap(err, "time.Parse")
}
