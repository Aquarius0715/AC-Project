package iotload

import "sync"

// Queue is a bounded queue that never blocks a producer and never panics when it is offered a value after it has been
// closed: the generator's handlers for the broker's commands go on running for a moment after the generator has
// stopped, and sending on a channel that is closed panics.
type Queue[T any] struct {
	lock   sync.RWMutex
	closed bool
	values chan T
}

// NewQueue makes a queue with room for size values.
func NewQueue[T any](size int) *Queue[T] {
	return &Queue[T]{lock: sync.RWMutex{}, closed: false, values: make(chan T, size)}
}

// Offer adds a value and reports whether it was taken: false when the queue is full or closed.
func (q *Queue[T]) Offer(value T) bool {
	q.lock.RLock()
	defer q.lock.RUnlock()

	if q.closed {
		return false
	}

	select {
	case q.values <- value:
		return true
	default:
		return false
	}
}

// Values is the channel the consumers read; it is closed, after the values it holds, by Close.
func (q *Queue[T]) Values() <-chan T { return q.values }

// Close refuses further values and ends Values once it is drained. Closing again does nothing.
func (q *Queue[T]) Close() {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.closed {
		return
	}

	q.closed = true

	close(q.values)
}
