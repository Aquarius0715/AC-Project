package iotbridge_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pradita/ac-project/service/api/internal/iotbridge"
)

// The jobs of one key run in the order they were submitted, whatever the number of workers.
func TestDispatcherKeepsTheOrderOfAKey(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(8, 4)

	var (
		lock sync.Mutex
		seen = map[string][]int{}
	)

	keys := []string{"a", "b", "c", "d", "e"}
	for step := range 200 {
		for _, key := range keys {
			dispatcher.Submit(key, func() {
				lock.Lock()
				defer lock.Unlock()

				seen[key] = append(seen[key], step)
			})
		}
	}

	dispatcher.Close()

	for _, key := range keys {
		got := seen[key]
		if len(got) != 200 {
			t.Fatalf("key %s: %d jobs ran, want 200", key, len(got))
		}

		for i, step := range got {
			if step != i {
				t.Fatalf("key %s: job %d ran at position %d", key, step, i)
			}
		}
	}
}

// A slow job of one key does not hold up the jobs of the keys on other workers: this is what a single handler could not
// do, and what made the broker drop messages for the bridge under load.
func TestDispatcherRunsDifferentKeysSideBySide(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(16, 1)
	release := make(chan struct{})
	done := make(chan string, 64)

	const slowKey = "slow"

	dispatcher.Submit(slowKey, func() { <-release })

	submitted := 0

	for i := range 64 {
		key := fmt.Sprintf("device-%d", i)
		if dispatcher.Index(key) == dispatcher.Index(slowKey) {
			continue // it would wait behind the slow job, as it should
		}

		dispatcher.Submit(key, func() { done <- key })

		submitted++
	}

	for range submitted {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("a job of another worker waited behind the blocked one")
		}
	}

	close(release)
	dispatcher.Close()

	if submitted < 40 {
		t.Fatalf("only %d keys were submitted, the test did not exercise the workers", submitted)
	}
}

// Submit waits when the worker's queue is full (the pressure goes back to the broker) and Close waits for what was
// submitted.
func TestDispatcherCloseWaitsForTheQueue(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(1, 2)

	var (
		lock sync.Mutex
		ran  int
	)

	for range 20 {
		dispatcher.Submit("k", func() {
			time.Sleep(time.Millisecond)
			lock.Lock()
			ran++
			lock.Unlock()
		})
	}

	dispatcher.Close()

	if ran != 20 {
		t.Fatalf("%d jobs ran before Close returned, want 20", ran)
	}
}

// At least one worker and one place in each queue, whatever the arguments.
func TestDispatcherNeedsOneWorker(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(0, 0)
	ran := false

	dispatcher.Submit("k", func() { ran = true })
	dispatcher.Close()

	if !ran {
		t.Fatal("the job did not run")
	}
}

// A handler still running when the bridge stops may submit after Close: the job is dropped, without a panic.
func TestDispatcherDropsAJobAfterClose(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(2, 2)
	dispatcher.Close()

	ran := false

	if dispatcher.Submit("k", func() { ran = true }) {
		t.Error("Submit took a job after Close")
	}

	dispatcher.Close() // closing again does nothing

	if ran {
		t.Fatal("a job submitted after Close ran")
	}
}

// Submits racing with Close never panic, and every job that was taken runs.
func TestDispatcherSubmitRacesWithClose(t *testing.T) {
	t.Parallel()

	dispatcher := iotbridge.NewDispatcher(4, 1)

	var (
		submitters sync.WaitGroup
		taken, ran atomic.Int64
	)

	for sender := range 8 {
		submitters.Go(func() {
			for i := range 200 {
				if dispatcher.Submit(fmt.Sprintf("%d-%d", sender, i), func() { ran.Add(1) }) {
					taken.Add(1)
				}
			}
		})
	}

	dispatcher.Close()
	submitters.Wait()

	if ran.Load() != taken.Load() {
		t.Fatalf("%d jobs taken, %d ran", taken.Load(), ran.Load())
	}
}
