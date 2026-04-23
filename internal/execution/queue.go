package execution

import (
	"context"
	"fmt"
	"sync"
)

// JobHandler processes a single Job, returning an error if the job should be
// retried or marked as failed.
type JobHandler func(ctx context.Context, job *Job) error

// Queue is the abstraction for a job queue, supporting enqueue, dequeue, and
// acknowledgment semantics.
type Queue interface {
	Enqueue(job *Job) error
	Dequeue() (*Job, error)
	Ack(jobID string) error
	Nack(jobID string) error
	Size() int
	Close()
}

// InMemoryQueue is a buffered, channel-backed Queue implementation suitable for
// single-process deployments.
type InMemoryQueue struct {
	mu     sync.RWMutex
	ch     chan *Job
	closed bool
}

// NewInMemoryQueue creates a queue with a 1024-slot buffer.
func NewInMemoryQueue() *InMemoryQueue {
	return &InMemoryQueue{
		ch: make(chan *Job, 1024),
	}
}

// Enqueue pushes a job onto the queue, returning an error if the queue is full
// or closed.
func (q *InMemoryQueue) Enqueue(job *Job) error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.closed {
		return fmt.Errorf("queue is closed")
	}
	select {
	case q.ch <- job:
		return nil
	default:
		return fmt.Errorf("queue is full")
	}
}

// Dequeue pops the next available job, returning nil if the queue is empty.
func (q *InMemoryQueue) Dequeue() (*Job, error) {
	select {
	case job, ok := <-q.ch:
		if !ok {
			return nil, fmt.Errorf("queue is closed")
		}
		return job, nil
	default:
		return nil, nil
	}
}

// Ack acknowledges successful processing of a job. This is a no-op for the
// in-memory implementation but fulfills the Queue contract.
func (q *InMemoryQueue) Ack(_ string) error {
	return nil
}

// Nack signals that a job should be reprocessed. This is a no-op for the
// in-memory implementation but fulfills the Queue contract.
func (q *InMemoryQueue) Nack(_ string) error {
	return nil
}

// Size returns the number of jobs currently buffered in the queue.
func (q *InMemoryQueue) Size() int {
	return len(q.ch)
}

// Close drains and closes the underlying channel, preventing further enqueues.
func (q *InMemoryQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.closed {
		close(q.ch)
		q.closed = true
	}
}
