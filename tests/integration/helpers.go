// Package integration provides end-to-end tests for the Hydra platform.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/azagatti/hydra-db/internal/agent"
	"github.com/azagatti/hydra-db/internal/body"
	"github.com/azagatti/hydra-db/internal/execution"
	"github.com/azagatti/hydra-db/internal/gateway"
	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
	"github.com/azagatti/hydra-db/internal/policy"
)

// HydraFix holds references to all Hydra heads for integration testing.
type HydraFix struct {
	Gateway   *gateway.Gateway
	Registry  *body.Registry
	Agent     *agent.Runtime
	Execution *execution.Plane
	Memory    *memory.Plane
	Policy    *policy.Engine
	Bus       *body.InMemoryEventBus
}

func setupHydra(t *testing.T) (*HydraFix, func()) {
	t.Helper()

	cfg := body.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 0

	registry := body.NewRegistry()
	bus := body.NewInMemoryEventBus()

	gw := gateway.NewGateway(cfg.Gateway)
	rt := agent.NewRuntime()
	execPlane := execution.NewPlane()
	memPlane := memory.NewPlane(inmemory.NewProvider())
	pe := policy.NewEngine(cfg.Policy)

	for _, h := range []body.Head{gw, rt, execPlane, memPlane, pe} {
		if err := registry.Register(h); err != nil {
			t.Fatalf("register %s: %v", h.Name(), err)
		}
	}

	registerHandlers(gw, rt, execPlane, memPlane, pe, registry, bus)

	ctx := context.Background()
	for _, h := range []body.Head{gw, rt, execPlane, memPlane, pe} {
		if err := h.Start(ctx); err != nil {
			t.Fatalf("start %s: %v", h.Name(), err)
		}
	}

	fix := &HydraFix{
		Gateway:   gw,
		Registry:  registry,
		Agent:     rt,
		Execution: execPlane,
		Memory:    memPlane,
		Policy:    pe,
		Bus:       bus,
	}

	cleanup := func() {
		sCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = gw.Stop(sCtx)
		_ = execPlane.Stop(sCtx)
		_ = rt.Stop(sCtx)
		_ = memPlane.Stop(sCtx)
		_ = pe.Stop(sCtx)
		bus.Close()
	}

	return fix, cleanup
}

func registerHandlers(
	gw *gateway.Gateway,
	rt *agent.Runtime,
	execPlane *execution.Plane,
	memPlane *memory.Plane,
	pe *policy.Engine,
	registry *body.Registry,
	bus *body.InMemoryEventBus,
) {
	gw.RegisterHandler("chat", func(ctx context.Context, env *body.Envelope) (*body.Envelope, error) {
		var req struct {
			Message string        `json:"message"`
			Actor   body.Identity `json:"actor"`
			Tenant  body.Tenant   `json:"tenant"`
		}
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if req.Actor.ID == "" {
			req.Actor = body.Identity{ID: "anonymous", Kind: body.ActorHuman, Roles: []string{"human"}, TenantID: "default"}
		}
		if req.Tenant.ID == "" {
			req.Tenant = body.Tenant{ID: "default", Name: "default"}
		}

		allowed, _ := pe.CheckPermission(req.Actor, policy.PermCreateAgent)
		if !allowed {
			pe.Audit(env.TraceID, req.Actor.ID, "chat", false, "permission denied")
			return nil, fmt.Errorf("permission denied: %s", policy.PermCreateAgent)
		}
		pe.Audit(env.TraceID, req.Actor.ID, "chat", true, "allowed")

		ac := agent.NewContext(req.Actor, req.Tenant)
		a, err := rt.Spawn(ctx, "echo-agent", func(_ context.Context, ag *agent.Agent) (any, error) {
			return map[string]string{"echo": req.Message, "agent_id": ag.ID}, nil
		}, agent.WithContext(ac))
		if err != nil {
			return nil, err
		}

		_ = bus.Publish(ctx, body.Event{
			Type:    "agent.completed",
			Payload: env.Payload,
			TraceID: env.TraceID,
		})

		resp := body.NewEnvelope(body.EnvelopeResponse, "chat", req.Actor, req.Tenant)
		resp.TraceID = env.TraceID
		return resp.WithPayload(a.Result)
	})

	gw.RegisterHandler("task", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
		var req struct {
			Type   string          `json:"type"`
			Data   json.RawMessage `json:"data"`
			Actor  body.Identity   `json:"actor"`
			Tenant body.Tenant     `json:"tenant"`
		}
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if req.Actor.ID == "" {
			req.Actor = body.Identity{ID: "anonymous", Kind: body.ActorHuman, TenantID: "default"}
		}
		if req.Tenant.ID == "" {
			req.Tenant = body.Tenant{ID: "default", Name: "default"}
		}

		allowed, _ := pe.CheckPermission(req.Actor, policy.PermExecuteTask)
		if !allowed {
			pe.Audit(env.TraceID, req.Actor.ID, "task", false, "permission denied")
			return nil, fmt.Errorf("permission denied: %s", policy.PermExecuteTask)
		}
		pe.Audit(env.TraceID, req.Actor.ID, "task", true, "allowed")

		job := execution.NewJob(req.Type, req.Data)
		if err := execPlane.Submit(job); err != nil {
			return nil, err
		}

		resp := body.NewEnvelope(body.EnvelopeResponse, "task", req.Actor, req.Tenant)
		resp.TraceID = env.TraceID
		return resp.WithPayload(map[string]string{"job_id": job.ID, "state": string(execution.JobPending)})
	})

	gw.RegisterHandler("memory.store", func(ctx context.Context, env *body.Envelope) (*body.Envelope, error) {
		var req struct {
			MemoryType string          `json:"type"`
			Content    json.RawMessage `json:"content"`
			Tags       []string        `json:"tags"`
			ActorID    string          `json:"actor_id"`
			TenantID   string          `json:"tenant_id"`
		}
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}
		if req.ActorID == "" {
			req.ActorID = "anonymous"
		}
		if req.TenantID == "" {
			req.TenantID = "default"
		}

		memType := memory.Type(req.MemoryType)
		if memType == "" {
			memType = memory.Semantic
		}
		mem := memory.NewMemory(memType, req.Content, req.ActorID, req.TenantID)
		mem.Tags = req.Tags

		if err := memPlane.Store(ctx, mem); err != nil {
			return nil, err
		}

		_ = bus.Publish(ctx, body.Event{
			Type:    "memory.stored",
			Payload: env.Payload,
			TraceID: env.TraceID,
		})

		resp := body.NewEnvelope(body.EnvelopeResponse, "memory.store",
			body.Identity{ID: req.ActorID}, body.Tenant{ID: req.TenantID})
		resp.TraceID = env.TraceID
		return resp.WithPayload(map[string]string{"memory_id": mem.ID, "type": string(mem.Type)})
	})

	gw.RegisterHandler("memory.search", func(ctx context.Context, env *body.Envelope) (*body.Envelope, error) {
		var req struct {
			Type     string   `json:"type"`
			Tags     []string `json:"tags"`
			ActorID  string   `json:"actor_id"`
			TenantID string   `json:"tenant_id"`
			Limit    int      `json:"limit"`
		}
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, fmt.Errorf("invalid payload: %w", err)
		}

		results, err := memPlane.Search(ctx, memory.SearchQuery{
			Type:     memory.Type(req.Type),
			Tags:     req.Tags,
			ActorID:  req.ActorID,
			TenantID: req.TenantID,
			Limit:    req.Limit,
		})
		if err != nil {
			return nil, err
		}

		resp := body.NewEnvelope(body.EnvelopeResponse, "memory.search", body.Identity{}, body.Tenant{})
		resp.TraceID = env.TraceID
		return resp.WithPayload(map[string]any{"results": results, "count": len(results)})
	})

	gw.RegisterHandler("health", func(_ context.Context, env *body.Envelope) (*body.Envelope, error) {
		reports := registry.HealthAll()
		healthy := registry.IsHealthy()

		status := "healthy"
		if !healthy {
			status = "unhealthy"
		}

		resp := body.NewEnvelope(body.EnvelopeResponse, "health", body.Identity{}, body.Tenant{})
		resp.TraceID = env.TraceID
		return resp.WithPayload(map[string]any{
			"status":  status,
			"reports": reports,
			"heads":   registry.Names(),
		})
	})
}

func postAction(t *testing.T, h http.Handler, action, traceID, reqBody string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/"+action, strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	if traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, raw []byte) body.Envelope {
	t.Helper()
	var env body.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}
