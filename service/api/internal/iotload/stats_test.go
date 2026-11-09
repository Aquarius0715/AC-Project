package iotload_test

import (
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/iotload"
)

func TestPercentileIsTheNearestRank(t *testing.T) {
	t.Parallel()

	var delays []time.Duration
	for i := 100; i >= 1; i-- { // unsorted on purpose
		delays = append(delays, time.Duration(i)*time.Millisecond)
	}

	wanted := map[float64]time.Duration{
		50: 50 * time.Millisecond, 95: 95 * time.Millisecond, 99: 99 * time.Millisecond,
		100: 100 * time.Millisecond, 0: time.Millisecond,
	}

	for pct, want := range wanted {
		if got := iotload.Percentile(delays, pct); got != want {
			t.Errorf("p%.0f = %v, want %v", pct, got, want)
		}
	}

	if iotload.Percentile(nil, 95) != 0 {
		t.Error("no delay has no percentile")
	}

	if delays[0] != 100*time.Millisecond {
		t.Error("Percentile sorted the caller's slice")
	}
}

func TestSpacingAndRate(t *testing.T) {
	t.Parallel()

	if got := iotload.Spacing(time.Minute, 3000); got != 20*time.Millisecond {
		t.Errorf("3000 devices a minute are 20 ms apart, got %v", got)
	}

	if got := iotload.RatePerSecond(time.Minute, 3000); got != 50 {
		t.Errorf("launch rate = %v, want 50 a second", got)
	}

	if got := iotload.RatePerSecond(9*time.Second, 3000); got < 333 || got > 334 {
		t.Errorf("design rate = %v, want about 333 a second", got)
	}

	if iotload.Spacing(time.Minute, 0) != 0 || iotload.RatePerSecond(0, 5) != 0 {
		t.Error("no device and no interval make no messages")
	}
}

func TestSummarizeCountsWhatNeverArrived(t *testing.T) {
	t.Parallel()

	got := iotload.Summarize(
		[]time.Duration{10 * time.Millisecond, 30 * time.Millisecond, 20 * time.Millisecond},
		5,
	)
	if got.Sent != 5 || got.Arrived != 3 || got.Lost != 2 || got.P50 != 20 || got.Max != 30 {
		t.Errorf("summary = %+v", got)
	}
}
