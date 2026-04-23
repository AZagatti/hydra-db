package execution

import (
	"context"
	"sync"
	"time"
)

// Scheduler manages delayed job delivery by holding jobs until their RunAfter
// time, then flushing them into the queue.
type Scheduler struct {
	queue   Queue
	mu      sync.Mutex
	pending []*Job
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewScheduler creates a Scheduler that dispatches ready jobs to the given
// Queue.
func NewScheduler(queue Queue) *Scheduler {
	return &Scheduler{
		queue: queue,
		done:  make(chan struct{}),
	}
}

// Start begins the background tick loop that flushes due jobs.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	go s.run(ctx)
}

// Stop cancels the background loop and waits for it to finish.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

// Schedule either enqueues the job immediately if it is due, or holds it for
// delayed delivery.
func (s *Scheduler) Schedule(job *Job) error {
	if job.RunAfter.IsZero() || !time.Now().Before(job.RunAfter) {
		return s.queue.Enqueue(job)
	}
	s.mu.Lock()
	s.pending = append(s.pending, job)
	s.mu.Unlock()
	return nil
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

func (s *Scheduler) flush() {
	s.mu.Lock()
	var ready []*Job
	var remaining []*Job

	now := time.Now()
	for _, job := range s.pending {
		if !now.Before(job.RunAfter) {
			ready = append(ready, job)
		} else {
			remaining = append(remaining, job)
		}
	}
	s.pending = remaining
	s.mu.Unlock()

	for _, job := range ready {
		_ = s.queue.Enqueue(job)
	}
}
