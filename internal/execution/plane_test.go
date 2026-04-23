package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJob_NewJob(t *testing.T) {
	payload := json.RawMessage(`{"key":"value"}`)
	job := NewJob("test", payload)

	assert.NotEmpty(t, job.ID)
	assert.Equal(t, "test", job.Type)
	assert.Equal(t, `{"key":"value"}`, string(job.Payload))
	assert.Equal(t, JobPending, job.State)
	assert.Equal(t, 3, job.MaxAttempts)
	assert.Equal(t, 0, job.Attempts)
	assert.False(t, job.CreatedAt.IsZero())
	assert.True(t, job.RunAfter.IsZero())
}

func TestJob_Options(t *testing.T) {
	runAfter := time.Now().Add(5 * time.Minute)
	payload := json.RawMessage(`{}`)
	result := json.RawMessage(`"done"`)

	job := NewJob("test", payload,
		WithMaxAttempts(5),
		WithRunAfter(runAfter),
		WithResult(result),
	)

	assert.Equal(t, 5, job.MaxAttempts)
	assert.WithinDuration(t, runAfter, job.RunAfter, time.Second)
	assert.Equal(t, `"done"`, string(job.Result))
}

func TestInMemoryQueue_EnqueueDequeue(t *testing.T) {
	q := NewInMemoryQueue()
	defer q.Close()

	job := NewJob("test", nil)
	require.NoError(t, q.Enqueue(job))

	got, err := q.Dequeue()
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
}

func TestInMemoryQueue_Size(t *testing.T) {
	q := NewInMemoryQueue()
	defer q.Close()

	assert.Equal(t, 0, q.Size())

	require.NoError(t, q.Enqueue(NewJob("t1", nil)))
	require.NoError(t, q.Enqueue(NewJob("t2", nil)))
	assert.Equal(t, 2, q.Size())
}

func TestInMemoryQueue_Close(t *testing.T) {
	q := NewInMemoryQueue()
	q.Close()

	err := q.Enqueue(NewJob("t2", nil))
	assert.Error(t, err)
}

func TestDAG_AddJob(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(NewJob("a", nil)))
	require.NoError(t, dag.AddJob(NewJob("b", nil)))

	assert.Len(t, dag.nodes, 2)
}

func TestDAG_AddJob_Duplicate(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))
	err := dag.AddJob(&Job{ID: "a"})
	assert.Error(t, err)
}

func TestDAG_AddDependency(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))
	require.NoError(t, dag.AddJob(&Job{ID: "b"}))
	require.NoError(t, dag.AddDependency("a", "b"))

	assert.Contains(t, dag.edges["a"], "b")
}

func TestDAG_AddDependency_MissingJob(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))

	err := dag.AddDependency("a", "missing")
	assert.Error(t, err)

	err = dag.AddDependency("missing", "a")
	assert.Error(t, err)
}

func TestDAG_TopologicalSort(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))
	require.NoError(t, dag.AddJob(&Job{ID: "b"}))
	require.NoError(t, dag.AddJob(&Job{ID: "c"}))
	require.NoError(t, dag.AddDependency("a", "b"))
	require.NoError(t, dag.AddDependency("b", "c"))

	order, err := dag.TopologicalSort()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestDAG_TopologicalSort_Complex(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))
	require.NoError(t, dag.AddJob(&Job{ID: "b"}))
	require.NoError(t, dag.AddJob(&Job{ID: "c"}))
	require.NoError(t, dag.AddJob(&Job{ID: "d"}))
	require.NoError(t, dag.AddDependency("a", "b"))
	require.NoError(t, dag.AddDependency("a", "c"))
	require.NoError(t, dag.AddDependency("b", "d"))
	require.NoError(t, dag.AddDependency("c", "d"))

	order, err := dag.TopologicalSort()
	require.NoError(t, err)

	pos := make(map[string]int)
	for i, id := range order {
		pos[id] = i
	}
	assert.Less(t, pos["a"], pos["b"])
	assert.Less(t, pos["a"], pos["c"])
	assert.Less(t, pos["b"], pos["d"])
	assert.Less(t, pos["c"], pos["d"])
}

func TestDAG_HasCycle(t *testing.T) {
	dag := NewDAG()
	require.NoError(t, dag.AddJob(&Job{ID: "a"}))
	require.NoError(t, dag.AddJob(&Job{ID: "b"}))

	err := dag.AddDependency("a", "b")
	assert.NoError(t, err)
	assert.False(t, dag.HasCycle())

	err = dag.AddDependency("b", "a")
	assert.Error(t, err)
	assert.False(t, dag.HasCycle())

	dag2 := NewDAG()
	require.NoError(t, dag2.AddJob(&Job{ID: "x"}))
	require.NoError(t, dag2.AddJob(&Job{ID: "y"}))
	dag2.edges["x"] = []string{"y"}
	dag2.edges["y"] = []string{"x"}
	assert.True(t, dag2.HasCycle())
}

func TestPlane_Name(t *testing.T) {
	p := NewPlane()
	assert.Equal(t, "execution", p.Name())
}

func TestPlane_SubmitAndProcess(t *testing.T) {
	p := NewPlane()
	var processed atomic.Int32
	p.SetHandler(func(_ context.Context, _ *Job) error {
		processed.Add(1)
		return nil
	})

	require.NoError(t, p.Start(context.Background()))
	defer func() { _ = p.Stop(context.Background()) }()

	job := NewJob("test", nil)
	require.NoError(t, p.Submit(job))

	assert.Eventually(t, func() bool {
		return processed.Load() == 1
	}, 2*time.Second, 50*time.Millisecond, "job was not processed")
}

func TestPlane_Submit_CompletesJob(t *testing.T) {
	p := NewPlane()
	p.SetHandler(func(_ context.Context, _ *Job) error {
		return nil
	})

	require.NoError(t, p.Start(context.Background()))
	defer func() { _ = p.Stop(context.Background()) }()

	job := NewJob("test", nil)
	require.NoError(t, p.Submit(job))

	assert.Eventually(t, func() bool {
		got, _ := p.GetJob(job.ID)
		return got.State == JobCompleted
	}, 2*time.Second, 50*time.Millisecond)

	got, err := p.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobCompleted, got.State)
	assert.False(t, got.StartedAt.IsZero())
	assert.False(t, got.FinishedAt.IsZero())
}

func TestPlane_Submit_FailsJob(t *testing.T) {
	p := NewPlane()
	p.SetHandler(func(_ context.Context, _ *Job) error {
		return fmt.Errorf("boom")
	})

	require.NoError(t, p.Start(context.Background()))
	defer func() { _ = p.Stop(context.Background()) }()

	job := NewJob("test", nil, WithMaxAttempts(1))
	require.NoError(t, p.Submit(job))

	assert.Eventually(t, func() bool {
		got, _ := p.GetJob(job.ID)
		return got.State == JobFailed
	}, 2*time.Second, 50*time.Millisecond)

	got, err := p.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobFailed, got.State)
	assert.Equal(t, "boom", got.Error)
}

func TestPlane_Submit_Retry(t *testing.T) {
	p := NewPlane()
	var attempts atomic.Int32
	p.SetHandler(func(_ context.Context, _ *Job) error {
		if attempts.Add(1) < 3 {
			return fmt.Errorf("transient")
		}
		return nil
	})

	require.NoError(t, p.Start(context.Background()))
	defer func() { _ = p.Stop(context.Background()) }()

	job := NewJob("test", nil, WithMaxAttempts(3))
	require.NoError(t, p.Submit(job))

	assert.Eventually(t, func() bool {
		got, _ := p.GetJob(job.ID)
		return got.State == JobCompleted
	}, 5*time.Second, 50*time.Millisecond)

	got, err := p.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, JobCompleted, got.State)
	assert.Equal(t, int32(3), attempts.Load())
}

func TestPlane_SubmitDAG(t *testing.T) {
	p := NewPlane()
	var mu sync.Mutex
	var order []string
	p.SetHandler(func(_ context.Context, job *Job) error {
		mu.Lock()
		order = append(order, job.ID)
		mu.Unlock()
		return nil
	})

	require.NoError(t, p.Start(context.Background()))
	defer func() { _ = p.Stop(context.Background()) }()

	dag := NewDAG()
	jobA := NewJob("task_a", nil)
	jobA.ID = "a"
	jobB := NewJob("task_b", nil)
	jobB.ID = "b"
	jobC := NewJob("task_c", nil)
	jobC.ID = "c"
	require.NoError(t, dag.AddJob(jobA))
	require.NoError(t, dag.AddJob(jobB))
	require.NoError(t, dag.AddJob(jobC))
	require.NoError(t, dag.AddDependency("a", "b"))
	require.NoError(t, dag.AddDependency("b", "c"))

	require.NoError(t, p.SubmitDAG(dag))

	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 3
	}, 2*time.Second, 50*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestPlane_GetJob(t *testing.T) {
	p := NewPlane()

	job := NewJob("test", json.RawMessage(`{"x":1}`))
	require.NoError(t, p.Submit(job))

	got, err := p.GetJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, job.Type, got.Type)

	_, err = p.GetJob("nonexistent")
	assert.Error(t, err)
}
