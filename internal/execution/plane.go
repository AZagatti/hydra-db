package execution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/azagatti/hydra-db/internal/body"
)

// Plane is the execution plane that accepts jobs, schedules them, and runs them
// through a registered handler with automatic retries and exponential backoff.
type Plane struct {
	mu        sync.RWMutex
	jobs      map[string]*Job
	queue     Queue
	scheduler *Scheduler
	handler   JobHandler
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewPlane creates an execution Plane backed by an in-memory queue and
// scheduler.
func NewPlane() *Plane {
	q := NewInMemoryQueue()
	return &Plane{
		jobs:      make(map[string]*Job),
		queue:     q,
		scheduler: NewScheduler(q),
		done:      make(chan struct{}),
		handler:   func(_ context.Context, _ *Job) error { return nil },
	}
}

// Name implements body.Head, identifying this plane as "execution".
func (p *Plane) Name() string {
	return "execution"
}

// Start launches the scheduler and the background job-processing loop.
func (p *Plane) Start(ctx context.Context) error {
	ctx, p.cancel = context.WithCancel(ctx)
	p.scheduler.Start(ctx)
	go p.process(ctx)
	return nil
}

// Stop cancels the processing loop and waits for it to finish, then tears down
// the scheduler and queue.
func (p *Plane) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	<-p.done
	p.scheduler.Stop()
	p.queue.Close()
	return nil
}

// Health reports the execution plane as healthy.
func (p *Plane) Health() body.HealthReport {
	return body.HealthReport{
		Head:   p.Name(),
		Status: body.HealthHealthy,
	}
}

// Submit registers a job and schedules it for execution.
func (p *Plane) Submit(job *Job) error {
	p.mu.Lock()
	p.jobs[job.ID] = job
	p.mu.Unlock()
	return p.scheduler.Schedule(job)
}

// SubmitDAG registers all jobs in a DAG and schedules them in topological order
// so dependencies are respected.
func (p *Plane) SubmitDAG(dag *DAG) error {
	order, err := dag.TopologicalSort()
	if err != nil {
		return fmt.Errorf("topological sort: %w", err)
	}

	p.mu.Lock()
	for _, id := range order {
		if job, ok := dag.nodes[id]; ok {
			p.jobs[id] = job
		}
	}
	p.mu.Unlock()

	for _, id := range order {
		if err := p.scheduler.Schedule(dag.nodes[id]); err != nil {
			return fmt.Errorf("schedule job %q: %w", id, err)
		}
	}

	return nil
}

// GetJob returns a copy of the job with the given ID for status inspection.
func (p *Plane) GetJob(id string) (*Job, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	job, ok := p.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job %q not found", id)
	}
	cp := *job
	return &cp, nil
}

// SetHandler replaces the function used to process dequeued jobs.
func (p *Plane) SetHandler(handler JobHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handler = handler
}

func (p *Plane) process(ctx context.Context) {
	defer close(p.done)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		job, err := p.queue.Dequeue()
		if err != nil || job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
				continue
			}
		}

		p.mu.RLock()
		handler := p.handler
		p.mu.RUnlock()

		p.mu.Lock()
		job.State = JobRunning
		job.Attempts++
		job.StartedAt = time.Now()
		p.mu.Unlock()

		if err := handler(ctx, job); err != nil {
			p.mu.Lock()
			job.Error = err.Error()
			if job.Attempts < job.MaxAttempts {
				job.State = JobRetrying
				job.FinishedAt = time.Now()
				backoff := time.Duration(job.Attempts*job.Attempts) * 100 * time.Millisecond
				job.RunAfter = time.Now().Add(backoff)
				p.mu.Unlock()

				_ = p.scheduler.Schedule(job)
			} else {
				job.State = JobFailed
				job.FinishedAt = time.Now()
				p.mu.Unlock()
			}
		} else {
			p.mu.Lock()
			job.State = JobCompleted
			job.FinishedAt = time.Now()
			p.mu.Unlock()
		}
	}
}
