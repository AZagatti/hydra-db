package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/azagatti/hydra-db/internal/execution"
	"github.com/azagatti/hydra-db/internal/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLifecycle_Chat(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	reqBody := `{"message":"hello hydra","actor":{"id":"user-1","kind":"human","roles":["admin"],"tenant_id":"t-1"},"tenant":{"id":"t-1","name":"test"}}`
	rec := postAction(t, fix.Gateway, "chat", "trace-chat-1", reqBody)

	assert.Equal(t, http.StatusOK, rec.Code)

	env := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, "response", string(env.Type))
	assert.Equal(t, "chat", env.Action)
	assert.Equal(t, "trace-chat-1", env.TraceID)
	assert.Equal(t, "user-1", env.Actor.ID)
	assert.Equal(t, "t-1", env.Tenant.ID)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "hello hydra", payload["echo"])
	assert.NotEmpty(t, payload["agent_id"])
}

func TestLifecycle_Task(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	reqBody := `{"type":"test_task","data":{"key":"value"},"actor":{"id":"user-1","kind":"human","roles":["human"],"tenant_id":"t-1"},"tenant":{"id":"t-1"}}`
	rec := postAction(t, fix.Gateway, "task", "trace-task-1", reqBody)

	assert.Equal(t, http.StatusOK, rec.Code)

	env := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, "response", string(env.Type))
	assert.Equal(t, "task", env.Action)
	assert.Equal(t, "trace-task-1", env.TraceID)

	var payload map[string]string
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.NotEmpty(t, payload["job_id"])
	assert.Equal(t, "pending", payload["state"])

	assert.Eventually(t, func() bool {
		job, err := fix.Execution.GetJob(payload["job_id"])
		if err != nil {
			return false
		}
		return job.State == execution.JobCompleted
	}, 2*time.Second, 50*time.Millisecond)
}

func TestLifecycle_MemoryStoreAndRecall(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	storeBody := `{"type":"episodic","content":{"text":"remember this"},"tags":["test","integration"],"actor_id":"actor-1","tenant_id":"tenant-1"}`
	rec := postAction(t, fix.Gateway, "memory.store", "trace-mem-1", storeBody)

	assert.Equal(t, http.StatusOK, rec.Code)

	storeEnv := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, "response", string(storeEnv.Type))
	assert.Equal(t, "memory.store", storeEnv.Action)
	assert.Equal(t, "trace-mem-1", storeEnv.TraceID)

	var storePayload map[string]string
	require.NoError(t, json.Unmarshal(storeEnv.Payload, &storePayload))
	assert.NotEmpty(t, storePayload["memory_id"])
	assert.Equal(t, "episodic", storePayload["type"])

	searchBody := `{"actor_id":"actor-1","tenant_id":"tenant-1"}`
	rec = postAction(t, fix.Gateway, "memory.search", "trace-mem-2", searchBody)

	assert.Equal(t, http.StatusOK, rec.Code)

	searchEnv := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, "response", string(searchEnv.Type))
	assert.Equal(t, "memory.search", searchEnv.Action)
	assert.Equal(t, "trace-mem-2", searchEnv.TraceID)

	var searchPayload struct {
		Results []struct {
			ID       string   `json:"id"`
			Type     string   `json:"type"`
			ActorID  string   `json:"actor_id"`
			TenantID string   `json:"tenant_id"`
			Tags     []string `json:"tags"`
		} `json:"results"`
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(searchEnv.Payload, &searchPayload))
	assert.Equal(t, 1, searchPayload.Count)
	require.Len(t, searchPayload.Results, 1)
	assert.Equal(t, "actor-1", searchPayload.Results[0].ActorID)
	assert.Equal(t, "tenant-1", searchPayload.Results[0].TenantID)
	assert.Equal(t, "episodic", searchPayload.Results[0].Type)
	assert.Contains(t, searchPayload.Results[0].Tags, "test")
}

func TestLifecycle_Health(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	rec := postAction(t, fix.Gateway, "health", "trace-health-1", "{}")

	assert.Equal(t, http.StatusOK, rec.Code)

	env := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, "response", string(env.Type))
	assert.Equal(t, "health", env.Action)
	assert.Equal(t, "trace-health-1", env.TraceID)

	var payload struct {
		Status  string   `json:"status"`
		Heads   []string `json:"heads"`
		Reports []struct {
			Head   string `json:"head"`
			Status string `json:"status"`
		} `json:"reports"`
	}
	require.NoError(t, json.Unmarshal(env.Payload, &payload))
	assert.Equal(t, "healthy", payload.Status)

	headSet := make(map[string]bool, len(payload.Heads))
	for _, h := range payload.Heads {
		headSet[h] = true
	}
	assert.True(t, headSet["gateway"])
	assert.True(t, headSet["agent"])
	assert.True(t, headSet["execution"])
	assert.True(t, headSet["memory"])
	assert.True(t, headSet["policy"])

	for _, report := range payload.Reports {
		assert.Equal(t, "healthy", report.Status, "head %s should be healthy", report.Head)
	}
}

func TestLifecycle_TraceIDPropagation(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	customTrace := "my-custom-trace-id-12345"

	reqBody := `{"message":"trace test","actor":{"id":"user-1","kind":"human","roles":["admin"],"tenant_id":"t-1"},"tenant":{"id":"t-1"}}`
	rec := postAction(t, fix.Gateway, "chat", customTrace, reqBody)

	assert.Equal(t, http.StatusOK, rec.Code)

	env := decodeEnvelope(t, rec.Body.Bytes())
	assert.Equal(t, customTrace, env.TraceID)

	auditEntries := fix.Policy.QueryAuditLog(policy.AuditFilter{
		TraceID: customTrace,
	})
	require.Len(t, auditEntries, 1)
	assert.Equal(t, customTrace, auditEntries[0].TraceID)
	assert.True(t, auditEntries[0].Allowed)
}

func TestLifecycle_PolicyEnforcement(t *testing.T) {
	fix, cleanup := setupHydra(t)
	defer cleanup()

	reqBody := `{"message":"should be denied","actor":{"id":"readonly-user","kind":"human","roles":["readonly"],"tenant_id":"t-1"},"tenant":{"id":"t-1"}}`
	rec := postAction(t, fix.Gateway, "chat", "trace-policy-1", reqBody)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var errResp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errResp))
	assert.Contains(t, errResp["error"], "permission denied")
	assert.Equal(t, "trace-policy-1", errResp["trace_id"])

	auditEntries := fix.Policy.QueryAuditLog(policy.AuditFilter{
		TraceID: "trace-policy-1",
	})
	require.Len(t, auditEntries, 1)
	assert.False(t, auditEntries[0].Allowed)
	assert.Equal(t, "readonly-user", auditEntries[0].ActorID)
	assert.Equal(t, "chat", auditEntries[0].Action)
}
