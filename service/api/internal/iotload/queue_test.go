package iotload_test

import (
	"sync"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/iotload"
)

// A full queue refuses a value, and a queue that is closed refuses it without a panic: a late command can still be
// answered after the generator has stopped.
func TestQueueRefusesWhenFullOrClosed(t *testing.T) {
	t.Parallel()

	queue := iotload.NewQueue[int](2)

	if !queue.Offer(1) || !queue.Offer(2) {
		t.Fatal("a queue with room refused a value")
	}

	if queue.Offer(3) {
		t.Fatal("a full queue took a value")
	}

	queue.Close()

	if queue.Offer(4) {
		t.Fatal("a closed queue took a value")
	}

	var got []int
	for v := range queue.Values() {
		got = append(got, v)
	}

	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("the queue held %v, want [1 2]", got)
	}

	queue.Close() // closing twice is harmless
}

// Offers racing with Close never panic and every accepted value is read.
func TestQueueOfferRacesWithClose(t *testing.T) {
	t.Parallel()

	queue := iotload.NewQueue[int](1 << 12)

	var (
		offers   sync.WaitGroup
		accepted int64
		lock     sync.Mutex
	)

	for range 8 {
		offers.Go(func() {
			for i := range 1000 {
				if queue.Offer(i) {
					lock.Lock()
					accepted++
					lock.Unlock()
				}
			}
		})
	}

	queue.Close()
	offers.Wait()

	var read int64
	for range queue.Values() {
		read++
	}

	if read != accepted {
		t.Fatalf("%d values accepted, %d read", accepted, read)
	}
}
