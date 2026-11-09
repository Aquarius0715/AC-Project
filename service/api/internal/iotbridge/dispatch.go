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
	lock   sync.RWMutex // keeps Submit from a closed queue
	closed bool
}

// NewDispatcher starts workers workers, each with a queue of depth jobs; at least one of each.
func NewDispatcher(workers, depth int) *Dispatcher {
	workers = max(workers, 1)
	depth = max(depth, 1)

	dispatcher := &Dispatcher{
		queues: make([]chan func(), workers),
		count:  uint32(workers), //nolint:gosec // G115: workers is at least 1 and a small number
		wg:     sync.WaitGroup{},
		lock:   sync.RWMutex{},
		closed: false,
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

// Submit queues job on the worker of key and reports whether it was taken. It waits while that worker's queue is full,
// which holds the caller back and so lets the pressure go back to the broker. After Close it drops the job: a handler
// still running when the bridge stops must not panic on a closed queue.
func (d *Dispatcher) Submit(key string, job func()) bool {
	d.lock.RLock()
	defer d.lock.RUnlock()

	if d.closed {
		return false
	}

	d.queues[d.Index(key)] <- job

	return true
}

// Close refuses further jobs, waits for every job taken to run, then stops the workers. Closing again does nothing.
func (d *Dispatcher) Close() {
	d.lock.Lock()

	if d.closed {
		d.lock.Unlock()

		return
	}

	d.closed = true

	for _, queue := range d.queues {
		close(queue)
	}

	d.lock.Unlock()
	d.wg.Wait()
}
