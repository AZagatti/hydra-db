package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTool struct {
	name string
}

func (m *mockTool) Name() string { return m.name }
func (m *mockTool) Execute(_ context.Context, input json.RawMessage) (json.RawMessage, error) {
	return input, nil
}

func identity() body.Identity {
	return body.Identity{ID: "actor-1", Kind: body.ActorHuman, TenantID: "t-1"}
}

func tenant() body.Tenant {
	return body.Tenant{ID: "t-1", Name: "test"}
}

func TestAgent_NewAgent(t *testing.T) {
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	})

	assert.NotEmpty(t, a.ID)
	assert.Equal(t, "test", a.Name)
	assert.Equal(t, StateCreated, a.State)
	assert.WithinDuration(t, time.Now(), a.CreatedAt, time.Second)
}

func TestAgent_WithTools(t *testing.T) {
	tool := &mockTool{name: "t1"}
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	}, WithTools(tool))

	require.Len(t, a.Tools, 1)
	assert.Equal(t, "t1", a.Tools[0].Name())
}

func TestAgent_WithContext(t *testing.T) {
	ac := NewContext(identity(), tenant())
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	}, WithContext(ac))

	assert.Equal(t, ac, a.Context)
	assert.Equal(t, "actor-1", a.Context.Actor.ID)
}

func TestContext_SetGet(t *testing.T) {
	ac := NewContext(identity(), tenant())

	ac.Set("key", "value")
	v, ok := ac.Get("key")
	assert.True(t, ok)
	assert.Equal(t, "value", v)

	_, ok = ac.Get("missing")
	assert.False(t, ok)
}

func TestContext_SetGetVar(t *testing.T) {
	ac := NewContext(identity(), tenant())

	ac.SetVar("env", "prod")
	v, ok := ac.GetVar("env")
	assert.True(t, ok)
	assert.Equal(t, "prod", v)

	_, ok = ac.GetVar("missing")
	assert.False(t, ok)
}

func TestToolRegistry_RegisterAndGet(t *testing.T) {
	r := NewToolRegistry()
	tool := &mockTool{name: "echo"}

	require.NoError(t, r.Register(tool))

	got, err := r.Get("echo")
	require.NoError(t, err)
	assert.Equal(t, tool, got)
}

func TestToolRegistry_Get_NotFound(t *testing.T) {
	r := NewToolRegistry()

	_, err := r.Get("nope")
	assert.Error(t, err)
}

func TestToolRegistry_List(t *testing.T) {
	r := NewToolRegistry()
	_ = r.Register(&mockTool{name: "a"})
	_ = r.Register(&mockTool{name: "b"})

	list := r.List()
	assert.ElementsMatch(t, []string{"a", "b"}, list)
}

func TestExecutor_Success(t *testing.T) {
	e := NewExecutor()
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		return "ok", nil
	})

	err := e.Execute(context.Background(), a)
	require.NoError(t, err)
	assert.Equal(t, StateDone, a.State)
	assert.Equal(t, "ok", a.Result)
}

func TestExecutor_Timeout(t *testing.T) {
	e := NewExecutor(WithTimeout(50 * time.Millisecond))
	a := NewAgent("test", func(ctx context.Context, _ *Agent) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	err := e.Execute(context.Background(), a)
	assert.Error(t, err)
	assert.Equal(t, StateTimedOut, a.State)
}

func TestExecutor_Retry(t *testing.T) {
	var attempts atomic.Int32

	e := NewExecutor(WithMaxRetries(2), WithTimeout(5*time.Second))
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		n := attempts.Add(1)
		if n < 2 {
			return nil, errors.New("transient")
		}
		return "recovered", nil
	})

	err := e.Execute(context.Background(), a)
	require.NoError(t, err)
	assert.Equal(t, StateDone, a.State)
	assert.Equal(t, "recovered", a.Result)
}

func TestExecutor_RetryExhausted(t *testing.T) {
	e := NewExecutor(WithMaxRetries(2), WithTimeout(5*time.Second))
	a := NewAgent("test", func(_ context.Context, _ *Agent) (any, error) {
		return nil, errors.New("always fails")
	})

	err := e.Execute(context.Background(), a)
	assert.Error(t, err)
	assert.Equal(t, StateFailed, a.State)
}

func TestRuntime_Name(t *testing.T) {
	rt := NewRuntime()
	assert.Equal(t, "agent", rt.Name())
}

func TestRuntime_Spawn(t *testing.T) {
	rt := NewRuntime()
	a, err := rt.Spawn(context.Background(), "worker", func(_ context.Context, _ *Agent) (any, error) {
		return 42, nil
	})

	require.NoError(t, err)
	assert.Equal(t, StateDone, a.State)
	assert.Equal(t, 42, a.Result)
}

func TestRuntime_GetAgent(t *testing.T) {
	rt := NewRuntime()
	spawned, _ := rt.Spawn(context.Background(), "worker", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	})

	got, err := rt.GetAgent(spawned.ID)
	require.NoError(t, err)
	assert.Equal(t, spawned.ID, got.ID)

	_, err = rt.GetAgent("nonexistent")
	assert.Error(t, err)
}

func TestRuntime_ListAgents(t *testing.T) {
	rt := NewRuntime()
	_, _ = rt.Spawn(context.Background(), "a1", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	})
	_, _ = rt.Spawn(context.Background(), "a2", func(_ context.Context, _ *Agent) (any, error) {
		return nil, nil
	})

	list := rt.ListAgents()
	assert.Len(t, list, 2)
}

func TestRuntime_RegisterTool(t *testing.T) {
	rt := NewRuntime()
	tool := &mockTool{name: "calc"}

	require.NoError(t, rt.RegisterTool(tool))

	got, err := rt.registry.Get("calc")
	require.NoError(t, err)
	assert.Equal(t, tool, got)
}
