// Package main is the entrypoint for the Hydra agent-native backend.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/azagatti/hydra-db/internal/agent"
	"github.com/azagatti/hydra-db/internal/agent/tools"
	"github.com/azagatti/hydra-db/internal/body"
	"github.com/azagatti/hydra-db/internal/execution"
	"github.com/azagatti/hydra-db/internal/gateway"
	"github.com/azagatti/hydra-db/internal/memory"
	"github.com/azagatti/hydra-db/internal/memory/inmemory"
	"github.com/azagatti/hydra-db/internal/memory/tdb"
	"github.com/azagatti/hydra-db/internal/policy"
)

func main() {
	cfg, err := body.LoadConfig("configs/hydra.yaml")
	if err != nil {
		slog.Warn("using default config", "error", err)
		cfg = body.DefaultConfig()
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg)

	registry := body.NewRegistry()
	bus := body.NewInMemoryEventBus()

	gw := gateway.NewGateway(cfg.Gateway)
	rt := agent.NewRuntime()
	execPlane := execution.NewPlane()
	// Build the memory plane with the configured provider.
	var memProvider memory.Provider
	switch cfg.Memory.Provider {
	case "tardigrade":
		tdbURL := cfg.Memory.TDBURL
		if tdbURL == "" {
			tdbURL = "http://localhost:8765"
		}
		slog.Info("using tardigrade memory provider", "url", tdbURL)
		memProvider = tdb.NewProvider(tdbURL)
	default:
		slog.Info("using in-memory provider")
		memProvider = inmemory.NewProvider()
	}
	memPlane := memory.NewPlane(memProvider)
	pe := policy.NewEngine(cfg.Policy)

	for _, head := range []body.Head{gw, rt, execPlane, memPlane, pe} {
		if err := registry.Register(head); err != nil {
			slog.Error("register head", "head", head.Name(), "error", err)
			os.Exit(1)
		}
	}

	registerHandlers(gw, rt, execPlane, memPlane, pe, registry, bus)

	// Register built-in tools so agents can discover and invoke them.
	httpTool := tools.NewHTTPRequest()
	if err := rt.RegisterTool(httpTool); err != nil {
		slog.Error("register http_request tool", "error", err)
		os.Exit(1)
	}
	memWriteTool := tools.NewMemoryWrite(memPlane)
	if err := rt.RegisterTool(memWriteTool); err != nil {
		slog.Error("register memory_write tool", "error", err)
		os.Exit(1)
	}
	memSearchTool := tools.NewMemorySearch(memPlane)
	if err := rt.RegisterTool(memSearchTool); err != nil {
		slog.Error("register memory_search tool", "error", err)
		os.Exit(1)
	}
	tdbURL := cfg.Memory.TDBURL
	if tdbURL == "" {
		tdbURL = "http://localhost:8765"
	}
	tdbTool := tools.NewTDBSearch(tdbURL)
	if err := rt.RegisterTool(tdbTool); err != nil {
		slog.Error("register tdb_search tool", "error", err)
		os.Exit(1)
	}
	slog.Info("tools registered", "tools", rt.ListTools())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := registry.StartAll(ctx); err != nil {
		slog.Error("start heads", "error", err)
		os.Exit(1)
	}

	slog.Info("hydra started",
		"name", cfg.Hydra.Name,
		"version", cfg.Hydra.Version,
		"addr", fmt.Sprintf("%s:%d", cfg.Gateway.Host, cfg.Gateway.Port),
		"heads", registry.Names(),
	)

	<-ctx.Done()
	slog.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := registry.StopAll(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
	bus.Close()
	slog.Info("goodbye")
}

func setupLogger(cfg *body.Config) {
	var level slog.Level
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(handler))
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

		allowed, err := pe.CheckPermission(req.Actor, policy.PermCreateAgent)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
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

		allowed, err := pe.CheckPermission(req.Actor, policy.PermExecuteTask)
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
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
