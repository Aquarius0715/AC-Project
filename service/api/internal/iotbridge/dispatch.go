package iotbridge

import (
	"hash/fnv"
	"sync"
)

// Dispatcher runs jobs on a fixed set of workers, and a key always goes to the same worker: the jobs of one key (one
// device) run in the order they were submitted, while the jobs of different keys run side by side. The bridge hands
// every message of the broker to it, so that a slow call to the API for one device does not hold up the messages of the
// others, and the broker's queue for the bridge does not overflow (it drops what does not fit).
type Dispatcher struct {
	queues []chan func()
	count  uint32 // len(queues), for the hash
	wg     sync.WaitGroup
}

// NewDispatcher starts workers workers, each with a queue of depth jobs; at least one of each.
func NewDispatcher(workers, depth int) *Dispatcher {
	workers = max(workers, 1)
	depth = max(depth, 1)

	dispatcher := &Dispatcher{
		queues: make([]chan func(), workers),
		count:  uint32(workers), //nolint:gosec // G115: workers is at least 1 and a small number
		wg:     sync.WaitGroup{},
	}

	for i := range dispatcher.queues {
		queue := make(chan func(), depth)
		dispatcher.queues[i] = queue

		dispatcher.wg.Go(func() {
			for job := range queue {
				job()
			}
		})
	}

	return dispatcher
}

// Index is the worker a key goes to.
func (d *Dispatcher) Index(key string) int {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(key)) // a hash.Hash never fails to write

	return int(sum.Sum32() % d.count)
}

// Submit queues job on the worker of key. It waits while that worker's queue is full, which holds the caller back and
// so lets the pressure go back to the broker. It must not be called after Close.
func (d *Dispatcher) Submit(key string, job func()) {
	d.queues[d.Index(key)] <- job
}

// Close waits for every submitted job to run, then stops the workers.
func (d *Dispatcher) Close() {
	for _, queue := range d.queues {
		close(queue)
	}

	d.wg.Wait()
}
