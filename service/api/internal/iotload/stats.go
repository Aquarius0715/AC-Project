// Package iotload holds the parts of the IoT load generator (cmd/iotload) that need no broker or database: the pace of
// the telemetry, the percentiles of the delays it measures, and the summary it writes.
package iotload

import (
	"math"
	"slices"
	"time"
)

// Percentile is the pct-th percentile (0–100) of the delays by nearest rank; zero for no delay.
func Percentile(delays []time.Duration, pct float64) time.Duration {
	if len(delays) == 0 {
		return 0
	}

	sorted := slices.Clone(delays)
	slices.Sort(sorted)

	rank := int(math.Ceil(pct / percentScale * float64(len(sorted))))
	rank = min(max(rank, 1), len(sorted))

	return sorted[rank-1]
}

const percentScale = 100

// Spacing is the time between two messages when `devices` devices each send every `interval`: the messages are spread
// evenly, one device after the other, instead of all at the same moment. Zero for no device.
func Spacing(interval time.Duration, devices int) time.Duration {
	if devices < 1 {
		return 0
	}

	return interval / time.Duration(devices)
}

// RatePerSecond is how many messages a second `devices` devices make when each sends every `interval`.
func RatePerSecond(interval time.Duration, devices int) float64 {
	if interval <= 0 {
		return 0
	}

	return float64(devices) / interval.Seconds()
}

// Delays summarizes the delays of the probe readings, in milliseconds.
type Delays struct {
	Sent    int     `json:"sent"`
	Arrived int     `json:"arrived"`
	Lost    int     `json:"lost"`
	P50     float64 `json:"p50Ms"`
	P95     float64 `json:"p95Ms"`
	P99     float64 `json:"p99Ms"`
	Max     float64 `json:"maxMs"`
}

// Summarize turns the delays of the readings that arrived and the number sent into a Delays.
func Summarize(arrived []time.Duration, sent int) Delays {
	toMillis := func(d time.Duration) float64 { return float64(d.Microseconds()) / microsPerMilli }

	return Delays{
		Sent:    sent,
		Arrived: len(arrived),
		Lost:    sent - len(arrived),
		P50:     toMillis(Percentile(arrived, p50)),
		P95:     toMillis(Percentile(arrived, p95)),
		P99:     toMillis(Percentile(arrived, p99)),
		Max:     toMillis(Percentile(arrived, percentScale)),
	}
}

const (
	microsPerMilli = 1000
	p50            = 50
	p95            = 95
	p99            = 99
)

// Report is what a run writes at its end (iotload.json).
type Report struct {
	DurationSeconds float64 `json:"durationSeconds"`
	Devices         int     `json:"devices"`
	IntervalSeconds float64 `json:"intervalSeconds"`
	TargetPerSecond float64 `json:"targetPerSecond"`
	Published       int64   `json:"published"` // telemetry batches the broker took
	PublishFailed   int64   `json:"publishFailed"`
	Dropped         int64   `json:"dropped"` // batches the generator could not hand to the broker in time
	Heartbeats      int64   `json:"heartbeats"`
	Commands        int64   `json:"commandsAnswered"`
	// Stored is how many readings of the run the database holds after the drain, of Expected (one a sensor and
	// minute, plus the probes)
	Expected     int64   `json:"expectedReadings"`
	Stored       int64   `json:"storedReadings"`
	DrainSeconds float64 `json:"drainSeconds"`
	Probes       Delays  `json:"probes"`
}
