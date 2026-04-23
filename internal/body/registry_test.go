package body

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockHead struct {
	name    string
	health  HealthReport
	startFn func(ctx context.Context) error
	stopFn  func(ctx context.Context) error
}

func (m *mockHead) Name() string { return m.name }
func (m *mockHead) Start(ctx context.Context) error {
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	return nil
}
func (m *mockHead) Stop(ctx context.Context) error {
	if m.stopFn != nil {
		return m.stopFn(ctx)
	}
	return nil
}
func (m *mockHead) Health() HealthReport { return m.health }

func newMockHead(name string, status HealthStatus) *mockHead {
	return &mockHead{
		name: name,
		health: HealthReport{
			Head:   name,
			Status: status,
		},
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	h := newMockHead("api", HealthHealthy)

	err := r.Register(h)
	require.NoError(t, err)

	got, err := r.Get("api")
	require.NoError(t, err)
	assert.Equal(t, h, got)
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(newMockHead("api", HealthHealthy))

	err := r.Register(newMockHead("api", HealthHealthy))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	_, err := r.Get("missing")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	h1 := newMockHead("api", HealthHealthy)
	h2 := newMockHead("worker", HealthHealthy)
	require.NoError(t, r.Register(h1))
	require.NoError(t, r.Register(h2))

	list := r.List()
	assert.Len(t, list, 2)
	assert.Contains(t, list, h1)
	assert.Contains(t, list, h2)
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(newMockHead("api", HealthHealthy)))
	require.NoError(t, r.Register(newMockHead("worker", HealthHealthy)))

	names := r.Names()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "api")
	assert.Contains(t, names, "worker")
}

func TestRegistry_StartAll(t *testing.T) {
	r := NewRegistry()
	var started sync.Map
	h1 := &mockHead{
		name: "api",
		startFn: func(_ context.Context) error {
			started.Store("api", true)
			return nil
		},
	}
	h2 := &mockHead{
		name: "worker",
		startFn: func(_ context.Context) error {
			started.Store("worker", true)
			return nil
		},
	}
	require.NoError(t, r.Register(h1))
	require.NoError(t, r.Register(h2))

	err := r.StartAll(context.Background())
	require.NoError(t, err)

	_, ok1 := started.Load("api")
	_, ok2 := started.Load("worker")
	assert.True(t, ok1)
	assert.True(t, ok2)
}

func TestRegistry_StartAll_Error(t *testing.T) {
	r := NewRegistry()
	expectedErr := fmt.Errorf("boom")

	h1 := &mockHead{
		name: "api",
		startFn: func(_ context.Context) error {
			return expectedErr
		},
	}
	h2 := &mockHead{
		name: "worker",
		startFn: func(_ context.Context) error {
			return nil
		},
	}
	require.NoError(t, r.Register(h1))
	require.NoError(t, r.Register(h2))

	err := r.StartAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestRegistry_StopAll(t *testing.T) {
	r := NewRegistry()
	var stopped sync.Map
	h1 := &mockHead{
		name: "api",
		stopFn: func(_ context.Context) error {
			stopped.Store("api", true)
			return nil
		},
	}
	h2 := &mockHead{
		name: "worker",
		stopFn: func(_ context.Context) error {
			stopped.Store("worker", true)
			return nil
		},
	}
	require.NoError(t, r.Register(h1))
	require.NoError(t, r.Register(h2))

	err := r.StopAll(context.Background())
	require.NoError(t, err)

	_, ok1 := stopped.Load("api")
	_, ok2 := stopped.Load("worker")
	assert.True(t, ok1)
	assert.True(t, ok2)
}

func TestRegistry_StopAll_CollectsErrors(t *testing.T) {
	r := NewRegistry()
	h1 := &mockHead{
		name: "api",
		stopFn: func(_ context.Context) error {
			return fmt.Errorf("err1")
		},
	}
	h2 := &mockHead{
		name: "worker",
		stopFn: func(_ context.Context) error {
			return fmt.Errorf("err2")
		},
	}
	require.NoError(t, r.Register(h1))
	require.NoError(t, r.Register(h2))

	err := r.StopAll(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "err1")
	assert.Contains(t, err.Error(), "err2")
}

func TestRegistry_HealthAll(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(newMockHead("api", HealthHealthy)))
	require.NoError(t, r.Register(newMockHead("worker", HealthDegraded)))

	reports := r.HealthAll()
	assert.Len(t, reports, 2)

	statuses := map[string]HealthStatus{}
	for _, hr := range reports {
		statuses[hr.Head] = hr.Status
	}
	assert.Equal(t, HealthHealthy, statuses["api"])
	assert.Equal(t, HealthDegraded, statuses["worker"])
}

func TestRegistry_IsHealthy_AllHealthy(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(newMockHead("api", HealthHealthy)))
	require.NoError(t, r.Register(newMockHead("worker", HealthHealthy)))

	assert.True(t, r.IsHealthy())
}

func TestRegistry_IsHealthy_OneUnhealthy(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(newMockHead("api", HealthHealthy)))
	require.NoError(t, r.Register(newMockHead("worker", HealthUnhealthy)))

	assert.False(t, r.IsHealthy())
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup

	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("head-%d", i)
			_ = r.Register(newMockHead(name, HealthHealthy))
		}(i)
	}

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.List()
		}()
	}

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Names()
		}()
	}

	wg.Wait()

	assert.Len(t, r.List(), 100)
}
