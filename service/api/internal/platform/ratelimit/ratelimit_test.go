package ratelimit

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// IR375: a fixed window per minute and key, on a store all instances share
func TestWindow(t *testing.T) {
	mr := miniredis.RunT(t)
	store, err := NewValkey("redis://" + mr.Addr())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 9, 0, 15, 0, time.UTC)
	w := &Window{Store: store, Limit: 3, Window: time.Minute, Prefix: "rl:w:", Now: func() time.Time { return now }}
	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		if wait, err := w.Allow(ctx, "t1:m1"); err != nil || wait != 0 {
			t.Fatalf("write %d: wait %d %v", i, wait, err)
		}
	}
	// the fourth in the minute waits until the window ends (45 s after 09:00:15)
	if wait, err := w.Allow(ctx, "t1:m1"); err != nil || wait != 45 {
		t.Fatalf("over the limit: wait %d %v", wait, err)
	}
	// another caller has its own count
	if wait, _ := w.Allow(ctx, "t1:m2"); wait != 0 {
		t.Errorf("another membership waits %d", wait)
	}
	// a second instance on the same store sees the same count
	other := &Window{Store: store, Limit: 3, Window: time.Minute, Prefix: "rl:w:", Now: func() time.Time { return now }}
	if wait, _ := other.Allow(ctx, "t1:m1"); wait == 0 {
		t.Error("another instance does not share the count")
	}
	// the counter expires: the key carries a TTL of two windows
	if ttl := mr.TTL("rl:w:t1:m1:" + strconv.FormatInt(now.Truncate(time.Minute).Unix(), 10)); ttl != 2*time.Minute {
		t.Errorf("ttl %v", ttl)
	}
	// the next minute starts a new window
	now = now.Add(time.Minute)
	if wait, _ := w.Allow(ctx, "t1:m1"); wait != 0 {
		t.Errorf("next minute waits %d", wait)
	}
	// at the very end of a window the wait is at least one second
	now = time.Date(2026, 10, 11, 9, 2, 59, 999_000_000, time.UTC)
	for range 3 {
		_, _ = w.Allow(ctx, "t1:m3")
	}
	if wait, _ := w.Allow(ctx, "t1:m3"); wait != 1 {
		t.Errorf("last instant: wait %d", wait)
	}
	// an unreachable store is an error: the caller decides (the Core API fails open)
	mr.Close()
	if _, err := w.Allow(ctx, "t1:m1"); err == nil {
		t.Error("closed store: no error")
	}
}

func TestMemory(t *testing.T) {
	m := &Memory{}
	for i := int64(1); i <= 2; i++ {
		if n, _ := m.Incr(context.Background(), "k", time.Minute); n != i {
			t.Fatalf("incr %d: %d", i, n)
		}
	}
}
