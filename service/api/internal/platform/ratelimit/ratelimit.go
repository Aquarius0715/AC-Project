// Package ratelimit bounds how often one caller may write (network architecture §6: writes 60 per minute per session;
// backend Go design §6: RATE_LIMITED). A fixed window per minute counts the writes of a key in a store that every
// instance of every service shares (Valkey, ElastiCache in production), so the limit holds across instances and
// services alike (IR375).
package ratelimit

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store keeps the counters.
type Store interface {
	// Incr adds one to key, which expires after ttl, and returns the new count.
	Incr(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

// Window allows at most Limit calls per Window for one key.
type Window struct {
	Store  Store
	Limit  int
	Window time.Duration
	Prefix string           // namespace of the keys in the store
	Now    func() time.Time // wall time; the business clock of the demo may jump and is not used
}

// Allow counts one call of key and returns the whole seconds to wait when the key is over its limit (0: allowed).
func (w *Window) Allow(ctx context.Context, key string) (int, error) {
	now := time.Now()
	if w.Now != nil {
		now = w.Now()
	}
	start := now.Truncate(w.Window)
	n, err := w.Store.Incr(ctx, fmt.Sprintf("%s%s:%d", w.Prefix, key, start.Unix()), 2*w.Window)
	if err != nil {
		return 0, err
	}
	if n <= int64(w.Limit) {
		return 0, nil
	}
	return max(1, int(math.Ceil(start.Add(w.Window).Sub(now).Seconds()))), nil
}

// Valkey is a Store on Valkey or Redis.
type Valkey struct{ Client *redis.Client }

// NewValkey connects to a redis:// or rediss:// URL.
func NewValkey(url string) (Valkey, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return Valkey{}, err
	}
	opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = time.Second, 500*time.Millisecond, 500*time.Millisecond
	return Valkey{Client: redis.NewClient(opt)}, nil
}

// Incr implements Store: INCR and EXPIRE in one transaction.
func (v Valkey) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := v.Client.TxPipeline()
	n := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return n.Val(), nil
}

// Memory is a Store in this process, for tests: its counters do not expire.
type Memory struct {
	mu sync.Mutex
	n  map[string]int64
}

// Incr implements Store.
func (m *Memory) Incr(_ context.Context, key string, _ time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.n == nil {
		m.n = map[string]int64{}
	}
	m.n[key]++
	return m.n[key], nil
}
