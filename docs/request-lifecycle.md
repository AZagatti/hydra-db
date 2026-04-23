# Request Lifecycle

## Overview

Every request to Hydra follows the same path: an external actor sends a request through an adapter, the adapter normalizes it into an `Envelope`, the gateway routes it to the appropriate handler, policy is enforced, work is performed, and the result flows back through the adapter to the caller. This document traces the complete 16-step lifecycle.

## The 16-Step Flow

### Step 1: External Actor Sends Request via Adapter

An external actor (human, agent, tool, or system) initiates a request. The request arrives through one of the adapter edges: HTTP (`POST /api/v1/{action}`), CLI (JSON input), or Slack (event webhook).

The actor does not speak Hydra's internal protocol. They speak HTTP JSON, CLI arguments, or Slack events.

### Step 2: Adapter Normalizes into Envelope

The adapter (`internal/adapter/http`, `cli`, or `slack`) parses the raw transport payload and converts it into a standard `Envelope`. This involves:

- Extracting actor identity (who is making the request)
- Extracting tenant information (which organization)
- Determining the action (e.g., `chat`, `task`, `memory.store`)
- Marshaling the raw payload into `json.RawMessage`
- Creating a new `Envelope` with generated ID and timestamp

After this step, the request is transport-agnostic. No head will ever need to know whether the request came from HTTP, CLI, or Slack.

### Step 3: Gateway Receives Envelope, Applies Middleware

The Gateway (`internal/gateway`) receives the incoming HTTP request at `/api/v1/{action}`. It extracts the action from the URL path, reads the request body, and constructs an `Envelope` with a generated ID, trace ID, and the raw payload.

The gateway wraps the handler in panic recovery to ensure a crashing handler cannot bring down the server.

### Step 4: CORS and Method Validation

The gateway sets CORS headers on every response:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: POST, GET, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization, X-Trace-ID
```

If the method is `OPTIONS`, the gateway returns `204 No Content` immediately (preflight response). If the method is not `POST` (after the OPTIONS check), the gateway returns `405 Method Not Allowed`.

### Step 5: Trace ID Generation/Preservation

The gateway checks for an `X-Trace-ID` header in the request. If present, its value is used as the trace ID for the entire request chain. If absent, a new UUID is generated.

This trace ID is propagated to the response envelope and to any events published during processing, enabling end-to-end correlation across logs, spans, and audit entries.

### Step 6: Route Lookup by Action

The gateway looks up the action (extracted from the URL path) in its internal route table. If no handler is registered for the action, the gateway returns `404` with:

```json
{"error": "no handler for action: xyz", "trace_id": "..."}
```

### Step 7: Policy Engine Checks Permissions

The registered handler begins by decoding the envelope payload into a typed request struct. It then calls the Policy Engine (`pe.CheckPermission`) to verify that the actor's roles grant the required permission for the action:

- `chat` requires `agent:create`
- `task` requires `task:execute`
- `memory.search` requires `memory:read`
- `memory.store` and `health` have no permission check

The engine iterates through the actor's roles and checks if any role includes the required permission.

### Step 8: Policy Engine Checks Budget

For actions that consume resources, the policy engine checks whether the actor has remaining execution budget (`pe.CheckBudget`). Actors without an explicit budget entry are considered unlimited.

Budget tracking prevents runaway agents from consuming unbounded resources. Each action can consume a configurable amount from the actor's remaining budget.

### Step 9: Audit Log Records the Check

Regardless of the outcome, the policy engine records an `AuditEntry` via `pe.Audit`:

- If allowed: `Audit(traceID, actorID, action, true, "allowed")`
- If denied: `Audit(traceID, actorID, action, false, "permission denied")`

Every policy decision is recorded for compliance and debugging. The audit log is queryable by trace ID, actor ID, and time range.

### Step 10: Memory Plane Fetches Relevant Context

For agent actions, the handler queries the Memory Plane to fetch relevant context for the agent. This involves calling `memPlane.Search` with a query scoped to the actor's tenant and any relevant tags.

The fetched memories provide the agent with historical context -- previous interactions, stored facts, operational knowledge -- without relying on the LLM's training data.

### Step 11: Agent Runtime Spawns Agent with Context

The handler creates an agent `Context` (`agent.NewContext`) carrying the actor's identity, tenant, and a new session ID. It then calls `rt.Spawn` with:

- A name (e.g., `"echo-agent"`)
- A `Func` -- the function the agent will execute
- Options including the context and any tools

The runtime assigns the agent a unique ID, records it internally, and begins execution.

### Step 12: Agent Executes with Tools

The agent's `Func` is executed by the `Executor` with the configured timeout (default: 60 seconds) and retry policy (default: 0 retries). The agent can:

- Access its `Context` for identity, tenant, and session data
- Use inter-tool memory (`ctx.Set`/`ctx.Get`) for passing data between operations
- Invoke registered tools from the `ToolRegistry`

If the function exceeds the timeout, the agent state is set to `timed_out`. If it returns an error, the executor retries with exponential backoff up to `MaxRetries`.

### Step 13: Execution Plane Handles Async Subtasks

If the primary action triggers background work (e.g., submitting a task via the `task` action), the handler submits a `Job` to the Execution Plane. The plane:

- Registers the job in its internal map
- Passes it to the `Scheduler` for immediate or delayed scheduling
- The scheduler enqueues the job when its `RunAfter` time is reached
- The processing loop dequeues and executes the job with the registered `JobHandler`
- Failed jobs are retried with exponential backoff up to `MaxAttempts`

### Step 14: Agent Produces Result

When the agent function returns successfully, the executor sets the agent state to `done` and records the result. The handler receives the completed `Agent` with its `Result` field populated.

### Step 15: Memory Plane Stores What to Remember

After the agent completes, the handler may store relevant information in the Memory Plane via `memPlane.Store`. This creates a typed `Memory` record with:

- Content (the data to remember)
- Type (`episodic`, `semantic`, `operational`, or `working`)
- Tags for later retrieval
- Confidence score
- Actor and tenant ownership

The handler also publishes an event to the `EventBus` (e.g., `agent.completed`, `memory.stored`) so other heads can react to the outcome.

### Step 16: Adapter Converts Result Back, Response Delivered

The handler constructs a response `Envelope` carrying the result payload and the same trace ID as the request. The gateway serializes this as JSON and writes it to the HTTP response with status `200 OK`.

If any step produced an error, the gateway writes an error response:

```json
{"error": "error message", "trace_id": "..."}
```

The trace ID is included in every response (success or failure) so the caller can correlate the response with their logs.

## Example: Chat Request

This example traces a `POST /api/v1/chat` request end-to-end with concrete data.

**Request:**

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -H "X-Trace-ID: trace-abc-123" \
  -d '{
    "message": "What is Hydra?",
    "actor": {
      "id": "user-001",
      "kind": "human",
      "roles": ["agent"],
      "tenant_id": "tenant-acme"
    },
    "tenant": {
      "id": "tenant-acme",
      "name": "Acme Corp"
    }
  }'
```

**Step-by-step trace:**

1. **External actor:** The human with ID `user-001` sends a POST request via `curl`.
2. **Adapter:** The HTTP adapter is not directly involved here because the gateway handles the raw HTTP. The gateway constructs the envelope.
3. **Gateway receives:** The gateway's `handleAction` method is invoked. Action = `"chat"`.
4. **CORS:** Headers are set. Method is `POST` (not `OPTIONS`), so processing continues.
5. **Trace ID:** The `X-Trace-ID: trace-abc-123` header is present, so the trace ID is preserved.
6. **Route lookup:** The gateway finds a registered handler for action `"chat"`.
7. **Permission check:** The handler decodes the payload and checks if actor `user-001` with role `"agent"` has permission `agent:create`. The `agent` role includes this permission. **Allowed.**
8. **Budget check:** No explicit budget is set for `user-001`, so the check passes (unlimited).
9. **Audit:** The policy engine records: `Audit("trace-abc-123", "user-001", "chat", true, "allowed")`.
10. **Memory fetch:** (In the current implementation, this step is deferred; a production handler would search for relevant context.)
11. **Agent spawn:** The handler creates an agent context with identity `{ID: "user-001", Kind: "human", Roles: ["agent"], TenantID: "tenant-acme"}` and tenant `{ID: "tenant-acme", Name: "Acme Corp"}`. It spawns an `"echo-agent"` that returns `{"echo": "What is Hydra?", "agent_id": "<uuid>"}`.
12. **Agent executes:** The agent function runs immediately and returns the echo result.
13. **Async subtasks:** None in this request.
14. **Result:** The agent completes with state `done`, result: `{"echo": "What is Hydra?", "agent_id": "a1b2c3d4-..."}`.
15. **Memory store:** (Deferred in current implementation.)
16. **Response:** The gateway returns `200 OK` with:

```json
{
  "id": "<response-uuid>",
  "trace_id": "trace-abc-123",
  "actor": {"id": "user-001", "kind": "human", "roles": ["agent"], "tenant_id": "tenant-acme"},
  "tenant": {"id": "tenant-acme", "name": "Acme Corp"},
  "type": "response",
  "action": "chat",
  "payload": {"echo": "What is Hydra?", "agent_id": "a1b2c3d4-..."},
  "timestamp": "2026-04-23T10:30:00Z"
}
```

## Example: Policy Denial

This example shows what happens when an actor lacks the required permission.

**Request:**

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Do something",
    "actor": {
      "id": "user-002",
      "kind": "human",
      "roles": ["readonly"],
      "tenant_id": "tenant-acme"
    },
    "tenant": {"id": "tenant-acme"}
  }'
```

**Step-by-step trace:**

1. **External actor:** `user-002` sends a POST request.
2. **Gateway:** Action = `"chat"`, body parsed successfully.
3. **CORS:** Passes.
4. **Trace ID:** Auto-generated (no `X-Trace-ID` header).
5. **Route lookup:** Handler for `"chat"` found.
6. **Permission check:** The handler checks if actor `user-002` with role `"readonly"` has permission `agent:create`. The `readonly` role only includes `memory:read`. **Denied.**
7. **Audit:** The policy engine records: `Audit("<trace-id>", "user-002", "chat", false, "permission denied")`.
8. **Error response:** The handler returns an error: `"permission denied: agent:create"`. The gateway writes:

```json
{"error": "permission denied: agent:create", "trace_id": "<auto-generated-trace-id>"}
```

The request never reaches the agent runtime, execution plane, or memory plane. Policy denial is early and structural.
