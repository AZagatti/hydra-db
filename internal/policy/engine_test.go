package policy

import (
	"context"
	"testing"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e := NewEngine(body.PolicyConfig{DefaultBudget: 100})
	require.NoError(t, e.Start(context.Background()))
	return e
}

func TestEngine_Name(t *testing.T) {
	e := NewEngine(body.PolicyConfig{})
	assert.Equal(t, "policy", e.Name())
}

func TestEngine_StartStop(t *testing.T) {
	e := NewEngine(body.PolicyConfig{})
	require.NoError(t, e.Start(context.Background()))
	require.NoError(t, e.Stop(context.Background()))

	hr := e.Health()
	assert.Equal(t, "policy", hr.Head)
	assert.Equal(t, body.HealthHealthy, hr.Status)
}

func TestEngine_RegisterRole(t *testing.T) {
	e := newTestEngine(t)

	role := &Role{Name: "custom", Permissions: []Permission{PermReadMemory}}
	require.NoError(t, e.RegisterRole(role))

	actor := body.Identity{ID: "a1", Roles: []string{"custom"}}
	ok, err := e.CheckPermission(actor, PermReadMemory)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestEngine_RegisterRole_Duplicate(t *testing.T) {
	e := newTestEngine(t)

	err := e.RegisterRole(&Role{Name: "admin", Permissions: []Permission{PermReadMemory}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestEngine_CheckPermission_AdminHasAll(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "admin1", Roles: []string{"admin"}}
	for _, perm := range []Permission{PermCreateAgent, PermExecuteTask, PermReadMemory, PermWriteMemory, PermAdminAll} {
		ok, err := e.CheckPermission(actor, perm)
		require.NoError(t, err)
		assert.True(t, ok, "admin should have %s", perm)
	}
}

func TestEngine_CheckPermission_AgentPerms(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "ag1", Roles: []string{"agent"}}
	allowed := []Permission{PermCreateAgent, PermExecuteTask, PermReadMemory, PermWriteMemory}
	for _, perm := range allowed {
		ok, err := e.CheckPermission(actor, perm)
		require.NoError(t, err)
		assert.True(t, ok, "agent should have %s", perm)
	}

	ok, err := e.CheckPermission(actor, PermAdminAll)
	require.NoError(t, err)
	assert.False(t, ok, "agent should not have admin:all")
}

func TestEngine_CheckPermission_HumanPerms(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "h1", Roles: []string{"human"}}
	ok, err := e.CheckPermission(actor, PermExecuteTask)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = e.CheckPermission(actor, PermReadMemory)
	require.NoError(t, err)
	assert.True(t, ok)

	for _, perm := range []Permission{PermCreateAgent, PermWriteMemory, PermAdminAll} {
		ok, err = e.CheckPermission(actor, perm)
		require.NoError(t, err)
		assert.False(t, ok, "human should not have %s", perm)
	}
}

func TestEngine_CheckPermission_ReadonlyPerms(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "ro1", Roles: []string{"readonly"}}
	ok, err := e.CheckPermission(actor, PermReadMemory)
	require.NoError(t, err)
	assert.True(t, ok)

	for _, perm := range []Permission{PermCreateAgent, PermExecuteTask, PermWriteMemory, PermAdminAll} {
		ok, err = e.CheckPermission(actor, perm)
		require.NoError(t, err)
		assert.False(t, ok, "readonly should not have %s", perm)
	}
}

func TestEngine_CheckPermission_NoRole(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "unknown", Roles: []string{"nonexistent"}}
	ok, err := e.CheckPermission(actor, PermReadMemory)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_CheckPermission_MultipleRoles(t *testing.T) {
	e := newTestEngine(t)

	actor := body.Identity{ID: "multi", Roles: []string{"readonly", "human"}}
	ok, err := e.CheckPermission(actor, PermExecuteTask)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = e.CheckPermission(actor, PermReadMemory)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = e.CheckPermission(actor, PermAdminAll)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_Budget_CheckAndConsume(t *testing.T) {
	e := newTestEngine(t)

	e.SetBudget("actor1", "tenant1", 100)

	ok, err := e.CheckBudget("actor1")
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, e.ConsumeBudget("actor1", 30))

	ok, err = e.CheckBudget("actor1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestEngine_Budget_Exhausted(t *testing.T) {
	e := newTestEngine(t)

	e.SetBudget("actor2", "tenant1", 10)
	require.NoError(t, e.ConsumeBudget("actor2", 10))

	ok, err := e.CheckBudget("actor2")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_Budget_Insufficient(t *testing.T) {
	e := newTestEngine(t)

	e.SetBudget("actor3", "tenant1", 5)
	err := e.ConsumeBudget("actor3", 10)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient budget")
}

func TestEngine_ToolAccess_Allowed(t *testing.T) {
	e := newTestEngine(t)

	e.SetToolAllowList("tool-user", []string{"bash", "grep"})

	ok, err := e.CheckToolAccess("tool-user", "bash")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestEngine_ToolAccess_Denied(t *testing.T) {
	e := newTestEngine(t)

	e.SetToolAllowList("tool-user", []string{"bash", "grep"})

	ok, err := e.CheckToolAccess("tool-user", "rm")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestEngine_ToolAccess_NoRestriction(t *testing.T) {
	e := newTestEngine(t)

	ok, err := e.CheckToolAccess("unrestricted", "anything")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestEngine_Audit_RecordAndQuery(t *testing.T) {
	e := newTestEngine(t)

	e.Audit("trace-1", "actor-a", "task:execute", true, "role granted")
	e.Audit("trace-1", "actor-b", "task:execute", false, "no role")
	e.Audit("trace-2", "actor-a", "memory:read", true, "role granted")

	entries := e.QueryAuditLog(AuditFilter{TraceID: "trace-1"})
	assert.Len(t, entries, 2)
}

func TestEngine_Audit_FilterByActor(t *testing.T) {
	e := newTestEngine(t)

	e.Audit("t1", "actor-a", "act1", true, "ok")
	e.Audit("t1", "actor-b", "act2", true, "ok")
	e.Audit("t1", "actor-a", "act3", false, "denied")

	entries := e.QueryAuditLog(AuditFilter{ActorID: "actor-a"})
	assert.Len(t, entries, 2)
}

func TestEngine_Audit_FilterByAllowed(t *testing.T) {
	e := newTestEngine(t)

	e.Audit("t1", "a1", "act1", true, "ok")
	e.Audit("t1", "a1", "act2", false, "denied")
	e.Audit("t1", "a1", "act3", true, "ok")

	denied := false
	entries := e.QueryAuditLog(AuditFilter{Allowed: &denied})
	assert.Len(t, entries, 1)
	assert.False(t, entries[0].Allowed)
}

func TestEngine_DefaultRoles(t *testing.T) {
	e := newTestEngine(t)

	expected := []string{"admin", "agent", "human", "readonly"}
	roles := e.DefaultRoles()
	names := make([]string, len(roles))
	for i, r := range roles {
		names[i] = r.Name
	}
	assert.ElementsMatch(t, expected, names)
}
