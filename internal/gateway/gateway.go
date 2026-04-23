package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/azagatti/hydra-db/internal/body"
	"github.com/google/uuid"
)

// HandlerFunc processes an incoming Envelope and returns a response Envelope.
// It is the core contract for route handlers registered in the Gateway.
type HandlerFunc func(ctx context.Context, env *body.Envelope) (*body.Envelope, error)

// Gateway is an HTTP-based request router that maps action names to HandlerFunc
// implementations, translating between HTTP and the Envelope protocol.
type Gateway struct {
	cfg     body.GatewayConfig
	mux     *http.ServeMux
	server  *http.Server
	routes  map[string]HandlerFunc
	mu      sync.RWMutex
	running bool
}

// NewGateway creates a Gateway bound to the given configuration, wiring up the
// default /api/v1/{action} route.
func NewGateway(cfg body.GatewayConfig) *Gateway {
	g := &Gateway{
		cfg:    cfg,
		mux:    http.NewServeMux(),
		routes: make(map[string]HandlerFunc),
	}
	g.mux.HandleFunc("/api/v1/{action}", g.handleAction)
	return g
}

// Name implements body.Head, identifying this plane as "gateway".
func (g *Gateway) Name() string {
	return "gateway"
}

// Start binds to the configured TCP address and serves HTTP requests in the
// background.
func (g *Gateway) Start(_ context.Context) error {
	addr := fmt.Sprintf("%s:%d", g.cfg.Host, g.cfg.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway listen: %w", err)
	}

	g.server = &http.Server{
		Handler:      g,
		ReadTimeout:  time.Duration(g.cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(g.cfg.WriteTimeout) * time.Second,
	}

	g.mu.Lock()
	g.running = true
	g.mu.Unlock()

	go func() {
		if err := g.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway serve error", "error", err)
		}
		g.mu.Lock()
		g.running = false
		g.mu.Unlock()
	}()

	return nil
}

// Stop gracefully shuts down the HTTP server with a 5-second deadline.
func (g *Gateway) Stop(_ context.Context) error {
	g.mu.Lock()
	g.running = false
	g.mu.Unlock()

	if g.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return g.server.Shutdown(shutdownCtx)
	}
	return nil
}

// Health reports whether the HTTP server is currently accepting connections.
func (g *Gateway) Health() body.HealthReport {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.running {
		return body.HealthReport{
			Head:   g.Name(),
			Status: body.HealthHealthy,
		}
	}
	return body.HealthReport{
		Head:   g.Name(),
		Status: body.HealthUnhealthy,
		Detail: "not running",
	}
}

// RegisterHandler maps an action name to a HandlerFunc so the Gateway can
// dispatch incoming requests to the correct handler.
func (g *Gateway) RegisterHandler(action string, handler HandlerFunc) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.routes[action] = handler
}

// ServeHTTP implements http.Handler, delegating to the internal mux while
// recovering from panics to avoid crashing the server.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			writeError(w, http.StatusInternalServerError, "", "internal server error")
		}
	}()
	g.mux.ServeHTTP(w, r)
}

func (g *Gateway) handleAction(w http.ResponseWriter, r *http.Request) {
	setCORS(w)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	action := r.PathValue("action")

	traceID := r.Header.Get("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
	}

	g.mu.RLock()
	handler, ok := g.routes[action]
	g.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, traceID, fmt.Sprintf("no handler for action: %s", action))
		return
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, traceID, "method not allowed")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, traceID, "failed to read request body")
		return
	}

	if len(bodyBytes) > 0 {
		var raw map[string]any
		if err := json.Unmarshal(bodyBytes, &raw); err != nil {
			writeError(w, http.StatusBadRequest, traceID, "invalid JSON body")
			return
		}
	}

	authHeader := r.Header.Get("Authorization")

	env := &body.Envelope{
		ID:        uuid.New().String(),
		TraceID:   traceID,
		Type:      body.EnvelopeRequest,
		Action:    action,
		Payload:   json.RawMessage(bodyBytes),
		Metadata:  make(map[string]string),
		Timestamp: time.Now(),
	}
	if authHeader != "" {
		env.Metadata["authorization"] = authHeader
	}

	resp, err := handler(r.Context(), env)
	if err != nil {
		writeError(w, http.StatusInternalServerError, traceID, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func setCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Trace-ID")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, traceID, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":    message,
		"trace_id": traceID,
	})
}
