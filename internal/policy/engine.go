package policy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/azagatti/hydra-db/internal/body"
)

// Permission represents a fine-grained capability that can be granted to a Role.
type Permission string

const (
	// PermCreateAgent allows creating new agent instances.
	PermCreateAgent Permission = "agent:create"
	// PermExecuteTask allows dispatching tasks for execution.
	PermExecuteTask Permission = "task:execute"
	// PermReadMemory allows reading from the memory plane.
	PermReadMemory Permission = "memory:read"
	// PermWriteMemory allows writing to the memory plane.
	PermWriteMemory Permission = "memory:write"
	// PermAdminAll grants unrestricted access to every operation.
	PermAdminAll Permission = "admin:all"
)

// Role groups a set of Permissions under a named identity for assignment to actors.
type Role struct {
	Name        string
	Permissions []Permission
}

// BudgetEntry tracks the remaining execution budget for a single actor within a
// tenant, enabling cost-based rate limiting.
type BudgetEntry struct {
	ActorID   string
	TenantID  string
	Remaining int
	MaxBudget int
}

// ToolACL controls which tools each actor is permitted to invoke.
type ToolACL struct {
	AllowedTools map[string][]string
}

// AuditEntry records a single policy decision for later review and compliance.
type AuditEntry struct {
	ID        string
	TraceID   string
	ActorID   string
	Action    string
	Allowed   bool
	Reason    string
	Timestamp time.Time
}

// AuditFilter constrains the results returned by QueryAuditLog.
type AuditFilter struct {
	TraceID string
	ActorID string
	Allowed *bool
	Since   time.Time
	Limit   int
}

// Engine is the central policy decision point. It evaluates permissions, enforces
// budgets, restricts tool access, and maintains an audit trail.
type Engine struct {
	mu      sync.RWMutex
	roles   map[string]*Role
	budgets map[string]*BudgetEntry
	toolACL ToolACL
	audit   []AuditEntry
	config  body.PolicyConfig
}

// NewEngine creates a policy Engine with empty stores for roles, budgets, and
// audit entries.
func NewEngine(cfg body.PolicyConfig) *Engine {
	return &Engine{
		roles:   make(map[string]*Role),
		budgets: make(map[string]*BudgetEntry),
		toolACL: ToolACL{AllowedTools: make(map[string][]string)},
		audit:   make([]AuditEntry, 0),
		config:  cfg,
	}
}

// Name implements body.Head, identifying this plane as "policy".
func (e *Engine) Name() string {
	return "policy"
}

// Start initializes the engine by registering the built-in default roles.
func (e *Engine) Start(_ context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, role := range e.DefaultRoles() {
		r := role
		e.roles[r.Name] = &r
	}
	return nil
}

// Stop is a no-op; the engine holds no long-lived resources.
func (e *Engine) Stop(_ context.Context) error {
	return nil
}

// Health reports the engine as healthy because all checks are in-memory.
func (e *Engine) Health() body.HealthReport {
	return body.HealthReport{
		Head:   "policy",
		Status: body.HealthHealthy,
		Detail: "policy engine operational",
	}
}

// RegisterRole adds a new Role to the engine, rejecting duplicates.
func (e *Engine) RegisterRole(role *Role) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, exists := e.roles[role.Name]; exists {
		return fmt.Errorf("role %q already registered", role.Name)
	}
	e.roles[role.Name] = role
	return nil
}

// CheckPermission returns whether the actor possesses the requested permission
// through any of its assigned roles.
func (e *Engine) CheckPermission(actor body.Identity, perm Permission) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, roleName := range actor.Roles {
		role, ok := e.roles[roleName]
		if !ok {
			continue
		}
		for _, p := range role.Permissions {
			if p == perm {
				return true, nil
			}
		}
	}
	return false, nil
}

// CheckBudget returns whether the actor still has remaining execution budget.
// Actors without a budget entry are considered unlimited.
func (e *Engine) CheckBudget(actorID string) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	entry, ok := e.budgets[actorID]
	if !ok {
		return true, nil
	}
	return entry.Remaining > 0, nil
}

// ConsumeBudget deducts the given amount from the actor's remaining budget,
// failing if the budget is insufficient.
func (e *Engine) ConsumeBudget(actorID string, amount int) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry, ok := e.budgets[actorID]
	if !ok {
		return fmt.Errorf("no budget set for actor %q", actorID)
	}
	if entry.Remaining < amount {
		return fmt.Errorf("insufficient budget: have %d, need %d", entry.Remaining, amount)
	}
	entry.Remaining -= amount
	return nil
}

// SetBudget creates or overwrites the execution budget for an actor within a
// tenant.
func (e *Engine) SetBudget(actorID, tenantID string, maxBudget int) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.budgets[actorID] = &BudgetEntry{
		ActorID:   actorID,
		TenantID:  tenantID,
		Remaining: maxBudget,
		MaxBudget: maxBudget,
	}
}

// CheckToolAccess returns whether the actor is allowed to use the named tool.
// Actors without an explicit allow-list are permitted all tools.
func (e *Engine) CheckToolAccess(actorID, toolName string) (bool, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	allowed, exists := e.toolACL.AllowedTools[actorID]
	if !exists {
		return true, nil
	}
	for _, t := range allowed {
		if t == toolName {
			return true, nil
		}
	}
	return false, nil
}

// SetToolAllowList restricts an actor to only the given set of tools.
func (e *Engine) SetToolAllowList(actorID string, tools []string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.toolACL.AllowedTools[actorID] = tools
}

// Audit appends a policy decision record to the in-memory audit trail.
func (e *Engine) Audit(traceID, actorID, action string, allowed bool, reason string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.audit = append(e.audit, AuditEntry{
		TraceID:   traceID,
		ActorID:   actorID,
		Action:    action,
		Allowed:   allowed,
		Reason:    reason,
		Timestamp: time.Now(),
	})
}

// QueryAuditLog returns audit entries matching the given filter criteria.
func (e *Engine) QueryAuditLog(filter AuditFilter) []AuditEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var result []AuditEntry
	for _, entry := range e.audit {
		if filter.TraceID != "" && entry.TraceID != filter.TraceID {
			continue
		}
		if filter.ActorID != "" && entry.ActorID != filter.ActorID {
			continue
		}
		if filter.Allowed != nil && entry.Allowed != *filter.Allowed {
			continue
		}
		if !filter.Since.IsZero() && entry.Timestamp.Before(filter.Since) {
			continue
		}
		result = append(result, entry)
		if filter.Limit > 0 && len(result) >= filter.Limit {
			break
		}
	}
	return result
}

// DefaultRoles returns the standard role set shipped with Hydra: admin, agent,
// human, and readonly.
func (e *Engine) DefaultRoles() []Role {
	return []Role{
		{
			Name: "admin",
			Permissions: []Permission{
				PermCreateAgent,
				PermExecuteTask,
				PermReadMemory,
				PermWriteMemory,
				PermAdminAll,
			},
		},
		{
			Name: "agent",
			Permissions: []Permission{
				PermCreateAgent,
				PermExecuteTask,
				PermReadMemory,
				PermWriteMemory,
			},
		},
		{
			Name: "human",
			Permissions: []Permission{
				PermExecuteTask,
				PermReadMemory,
			},
		},
		{
			Name: "readonly",
			Permissions: []Permission{
				PermReadMemory,
			},
		},
	}
}
