package memory_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
)

func TestMemory_NewMemory(t *testing.T) {
	content := json.RawMessage(`{"message":"hello"}`)
	mem := memory.NewMemory(memory.Episodic, content, "actor-1", "tenant-1")

	assert.NotEmpty(t, mem.ID)
	assert.Equal(t, memory.Episodic, mem.Type)
	assert.Equal(t, content, mem.Content)
	assert.Equal(t, "actor-1", mem.ActorID)
	assert.Equal(t, "tenant-1", mem.TenantID)
	assert.Equal(t, 1.0, mem.Confidence)
	assert.WithinDuration(t, time.Now(), mem.CreatedAt, time.Second)
	assert.WithinDuration(t, time.Now(), mem.AccessedAt, time.Second)
	assert.True(t, mem.ExpiresAt.IsZero())
}

func TestNewMemory_Types(t *testing.T) {
	content := json.RawMessage(`{}`)
	types := []memory.Type{
		memory.Episodic,
		memory.Semantic,
		memory.Operational,
		memory.Working,
	}
	for _, mt := range types {
		mem := memory.NewMemory(mt, content, "a", "t")
		assert.Equal(t, mt, mem.Type)
	}
}

func TestInMemoryProvider_WriteAndRead(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{"data":"test"}`), "a1", "t1")
	require.NoError(t, p.Write(ctx, mem))

	got, err := p.Read(ctx, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, mem.ID, got.ID)
	assert.Equal(t, mem.Content, got.Content)
}

func TestInMemoryProvider_Write_Duplicate(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem := memory.NewMemory(memory.Semantic, json.RawMessage(`{}`), "a", "t")
	require.NoError(t, p.Write(ctx, mem))
	assert.Error(t, p.Write(ctx, mem))
}

func TestInMemoryProvider_Read_NotFound(t *testing.T) {
	p := inmemory.NewProvider()
	_, err := p.Read(t.Context(), "nonexistent")
	assert.Error(t, err)
}

func TestInMemoryProvider_Read_UpdatesAccessedAt(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	originalAccessed := mem.AccessedAt
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, p.Write(ctx, mem))
	got, err := p.Read(ctx, mem.ID)
	require.NoError(t, err)
	assert.True(t, got.AccessedAt.After(originalAccessed))
}

func TestInMemoryProvider_Delete(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	require.NoError(t, p.Write(ctx, mem))
	require.NoError(t, p.Delete(ctx, mem.ID))
	_, err := p.Read(ctx, mem.ID)
	assert.Error(t, err)
}

func TestInMemoryProvider_Delete_NotFound(t *testing.T) {
	p := inmemory.NewProvider()
	err := p.Delete(t.Context(), "nonexistent")
	assert.Error(t, err)
}

func TestInMemoryProvider_Search_ByType(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	writeMem(t, p, memory.Episodic, "a", "t")
	writeMem(t, p, memory.Semantic, "a", "t")

	results, err := p.Search(ctx, memory.SearchQuery{Type: memory.Episodic})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, memory.Episodic, results[0].Type)
}

func TestInMemoryProvider_Search_ByTags(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem1 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	mem1.Tags = []string{"go", "test"}
	require.NoError(t, p.Write(ctx, mem1))

	mem2 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	mem2.Tags = []string{"go"}
	require.NoError(t, p.Write(ctx, mem2))

	results, err := p.Search(ctx, memory.SearchQuery{Tags: []string{"go", "test"}})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, mem1.ID, results[0].ID)
}

func TestInMemoryProvider_Search_ByActor(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	writeMem(t, p, memory.Episodic, "actor-1", "t")
	writeMem(t, p, memory.Episodic, "actor-2", "t")

	results, err := p.Search(ctx, memory.SearchQuery{ActorID: "actor-1"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "actor-1", results[0].ActorID)
}

func TestInMemoryProvider_Search_ByTenant(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	writeMem(t, p, memory.Episodic, "a", "tenant-1")
	writeMem(t, p, memory.Episodic, "a", "tenant-2")

	results, err := p.Search(ctx, memory.SearchQuery{TenantID: "tenant-1"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "tenant-1", results[0].TenantID)
}

func TestInMemoryProvider_Search_ByConfidence(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem1 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	mem1.Confidence = 0.9
	require.NoError(t, p.Write(ctx, mem1))

	mem2 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	mem2.Confidence = 0.3
	require.NoError(t, p.Write(ctx, mem2))

	results, err := p.Search(ctx, memory.SearchQuery{MinConfidence: 0.5})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, mem1.ID, results[0].ID)
}

func TestInMemoryProvider_Search_BySince(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem1 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	mem1.CreatedAt = time.Now().Add(-2 * time.Hour)
	require.NoError(t, p.Write(ctx, mem1))

	mem2 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	require.NoError(t, p.Write(ctx, mem2))

	results, err := p.Search(ctx, memory.SearchQuery{Since: time.Now().Add(-1 * time.Hour)})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, mem2.ID, results[0].ID)
}

func TestInMemoryProvider_Search_Limit(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	for i := 0; i < 10; i++ {
		writeMem(t, p, memory.Episodic, "a", "t")
	}

	results, err := p.Search(ctx, memory.SearchQuery{Limit: 3})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestInMemoryProvider_Search_NoResults(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	writeMem(t, p, memory.Episodic, "a", "t")

	results, err := p.Search(ctx, memory.SearchQuery{ActorID: "nonexistent"})
	require.NoError(t, err)
	assert.Len(t, results, 0)
}

func TestInMemoryProvider_Search_CombinedFilters(t *testing.T) {
	p := inmemory.NewProvider()
	ctx := t.Context()

	mem1 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "actor-1", "tenant-1")
	mem1.Tags = []string{"important"}
	mem1.Confidence = 0.9
	require.NoError(t, p.Write(ctx, mem1))

	mem2 := memory.NewMemory(memory.Semantic, json.RawMessage(`{}`), "actor-1", "tenant-1")
	mem2.Tags = []string{"important"}
	mem2.Confidence = 0.9
	require.NoError(t, p.Write(ctx, mem2))

	mem3 := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "actor-2", "tenant-1")
	mem3.Tags = []string{"important"}
	mem3.Confidence = 0.9
	require.NoError(t, p.Write(ctx, mem3))

	results, err := p.Search(ctx, memory.SearchQuery{
		Type:          memory.Episodic,
		ActorID:       "actor-1",
		TenantID:      "tenant-1",
		Tags:          []string{"important"},
		MinConfidence: 0.5,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, mem1.ID, results[0].ID)
}

func TestPlane_Name(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	assert.Equal(t, "memory", plane.Name())
}

func TestPlane_StartStop(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	ctx := t.Context()

	require.NoError(t, plane.Start(ctx))
	require.NoError(t, plane.Stop(ctx))
}

func TestPlane_StoreAndRecall(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	ctx := t.Context()

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{"msg":"hi"}`), "a", "t")
	require.NoError(t, plane.Store(ctx, mem))

	got, err := plane.Recall(ctx, mem.ID)
	require.NoError(t, err)
	assert.Equal(t, mem.ID, got.ID)
}

func TestPlane_Search(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	ctx := t.Context()

	writeMem(t, inmemory.NewProvider(), memory.Episodic, "a", "t")

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "actor-x", "t")
	require.NoError(t, plane.Store(ctx, mem))

	results, err := plane.Search(ctx, memory.SearchQuery{ActorID: "actor-x"})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, mem.ID, results[0].ID)
}

func TestPlane_Forget(t *testing.T) {
	plane := memory.NewPlane(inmemory.NewProvider())
	ctx := t.Context()

	mem := memory.NewMemory(memory.Episodic, json.RawMessage(`{}`), "a", "t")
	require.NoError(t, plane.Store(ctx, mem))
	require.NoError(t, plane.Forget(ctx, mem.ID))

	_, err := plane.Recall(ctx, mem.ID)
	assert.Error(t, err)
}

func writeMem(t *testing.T, p *inmemory.Provider, mt memory.Type, actorID, tenantID string) {
	t.Helper()
	mem := memory.NewMemory(mt, json.RawMessage(`{}`), actorID, tenantID)
	require.NoError(t, p.Write(t.Context(), mem))
}
