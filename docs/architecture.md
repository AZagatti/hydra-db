# Architecture

## Overview

Hydra is an agent-native backend built as a modular monolith in Go. It is not a generic web framework -- it is an operating backend designed for AI agents, humans, tools, and event-driven workflows. Every subsystem (called a "head") has a specialized role -- API gateway, agent runtime, job execution, memory storage, policy enforcement -- and all heads share a common "body" of identity, observability, configuration, and state.

The codebase compiles to a single Go binary with clean internal boundaries. Heads communicate through typed interfaces and a shared event bus. When real scaling requirements emerge, the `Head` interface already defines the seams for extraction into separate services.

## The Hydra Metaphor

In mythology, the Hydra is a multi-headed organism where each head has a specialized function but all share a single body. If one head is severed, the organism survives. This maps directly to the architecture:

- **The Body** -- shared types, contracts, and infrastructure that every head depends on. The body has no business logic of its own; it exists so heads never duplicate foundational concerns.
- **The Heads** -- independent subsystems that each own a vertical slice of functionality. Each head implements the `Head` interface (`Name`, `Start`, `Stop`, `Health`) and can be started, stopped, or replaced without affecting the others.
- **The Adapters** -- the sensory organs at the edges, translating external formats (HTTP JSON, CLI input, Slack events) into the internal Envelope protocol and back again.

## Architecture Diagram

```
+-------------------------------------------------------------+
|                         HYDRA BODY                          |
|                                                             |
|   Identity | Trace | EventBus | Config | Registry | Health  |
|                                                             |
+----------+----------+-----------+----------+----------------+
|          |          |           |          |                |
| Gateway  |  Agent   | Execution |  Memory  |    Policy      |
|  Head    | Runtime  |   Plane   |  Plane   |    Head        |
|          |  Head    |   Head    |  Head    |                |
|          |          |           |          |                |
+----------+----------+-----------+----------+----------------+
|                                                             |
|                   Interface Adapters                        |
|              HTTP  |  CLI  |  Slack                         |
|                                                             |
+-------------------------------------------------------------+
```

## The Body

The body lives in `internal/body/` and provides shared contracts every head depends on. None of these types contain business logic -- they exist to eliminate duplication and enforce consistency across the organism.

### Envelope

`Envelope` is the universal message wrapper that flows between heads. It carries identity, tracing, routing, and payload metadata so every head has a consistent context without depending on transport-specific details.

**Why it exists:** Without a universal message type, each head would define its own request/response format, leading to serialization sprawl and tight coupling to transport details (HTTP headers, CLI flags, Slack event shapes). The Envelope normalizes everything so heads never need to know how a request arrived.

**Key fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | `string` | Unique message identifier (UUID) |
| `TraceID` | `string` | Correlation ID across the full request chain |
| `Actor` | `Identity` | Who originated the request |
| `Tenant` | `Tenant` | Which tenant the request belongs to |
| `Type` | `EnvelopeType` | One of: `request`, `response`, `event`, `error` |
| `Action` | `string` | The routing key (e.g., `"chat"`, `"task"`, `"memory.store"`) |
| `Payload` | `json.RawMessage` | Opaque JSON payload, decoded by the target head |
| `Metadata` | `map[string]string` | Key-value metadata (e.g., authorization header) |
| `Timestamp` | `time.Time` | When the envelope was created |

### Identity

`Identity` represents the actor behind a request. Every envelope carries an identity so all heads can make policy decisions without re-authenticating.

**Why it exists:** Hydra treats agents, humans, tools, and system processes as first-class actors. The Identity type ensures every subsystem has a consistent view of who is making a request, what kind of actor they are, and what tenant they belong to.

**Key fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | `string` | Unique actor identifier |
| `Kind` | `ActorKind` | One of: `human`, `agent`, `tool`, `system` |
| `Roles` | `[]string` | Assigned roles for RBAC (e.g., `admin`, `agent`, `human`, `readonly`) |
| `TenantID` | `string` | Tenant this actor belongs to |

### Tenant

`Tenant` scopes all data and policy within a single customer or organization.

**Why it exists:** Multi-tenancy is a first-class concern. By carrying tenant information on every envelope, heads can enforce data isolation without additional lookup logic.

**Key fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | `string` | Unique tenant identifier |
| `Name` | `string` | Human-readable tenant name |

### Span

`Span` represents a unit of work within a distributed trace, allowing heads to correlate causality across the system. Spans form parent-child trees.

**Why it exists:** If you can't trace it, it didn't happen. Spans give every head a way to record timing, status, and metadata for each operation, linked by trace ID and parent span ID.

**Key fields:**

| Field | Type | Purpose |
|-------|------|---------|
| `ID` | `string` | Unique span identifier |
| `TraceID` | `string` | Correlation ID for the entire trace |
| `ParentID` | `string` | Parent span ID for hierarchical traces |
| `Head` | `string` | Which head produced this span |
| `Action` | `string` | What operation was performed |
| `Status` | `SpanStatus` | One of: `ok`, `error`, `pending` |
| `StartedAt` | `time.Time` | When the span began |
| `EndedAt` | `time.Time` | When the span completed |
| `Metadata` | `map[string]string` | Diagnostic key-value pairs |

### EventBus

`EventBus` is the contract for pub/sub messaging between heads. The in-memory implementation delivers events via buffered channels with non-blocking semantics (slow consumers have events dropped).

**Why it exists:** Heads need to react to side effects without tight coupling. When an agent completes, the agent runtime publishes an event; any head that cares about agent completions can subscribe without the agent runtime knowing or caring who is listening.

**Key methods:**

| Method | Purpose |
|--------|---------|
| `Publish(ctx, Event)` | Send an event to all subscribers of its type |
| `Subscribe(eventType)` | Get a channel that receives events of that type |
| `Unsubscribe(eventType, ch)` | Remove and close a subscriber channel |
| `Close()` | Shut down the bus and close all channels |

### Config

`Config` is the top-level configuration for a Hydra instance, loaded from YAML with environment variable overrides via koanf.

**Why it exists:** Centralizing configuration in a single validated struct prevents heads from inventing their own config loading mechanisms and ensures the entire organism starts with consistent settings.

**Sections:**

- `HydraConfig` -- service name, version, log level
- `GatewayConfig` -- bind host, port, read/write timeouts
- `PolicyConfig` -- default budget, rate limit
- `LoggingConfig` -- log level, output format

### Registry

`Registry` tracks all active heads and provides lifecycle management (start all, stop all, health aggregation).

**Why it exists:** The organism needs a single point of control for startup, shutdown, and health reporting. Without a registry, each head would need to be managed independently, and aggregation (e.g., "is the whole system healthy?") would be ad hoc.

**Key methods:**

| Method | Purpose |
|--------|---------|
| `Register(head)` | Add a head to the registry |
| `Get(name)` | Retrieve a head by name |
| `List()` | Get all registered heads |
| `StartAll(ctx)` | Start every head in order |
| `StopAll(ctx)` | Stop every head, collecting errors |
| `HealthAll()` | Collect health reports from every head |
| `IsHealthy()` | True only when every head reports healthy |

### Health

`HealthReport` is the health check result for a single head, with a status (`healthy`, `degraded`, `unhealthy`) and optional detail string.

**Why it exists:** Monitoring systems need a consistent health check format. Each head reports its own health, and the registry aggregates them into a whole-organism health view.

## The Heads

### Gateway Head

**Metaphor:** The Mouth -- receives all inbound traffic.

**Package:** `internal/gateway`

**Key types:**

| Type | Purpose |
|------|---------|
| `Gateway` | HTTP-based request router mapping action names to handlers |
| `HandlerFunc` | `func(ctx, *Envelope) (*Envelope, error)` -- the core handler contract |

**Responsibilities:**

- Binds to a TCP address and serves HTTP requests
- Routes `POST /api/v1/{action}` to registered `HandlerFunc` implementations
- Applies CORS headers and validates HTTP methods
- Generates or preserves trace IDs via the `X-Trace-ID` header
- Converts raw HTTP requests into `Envelope` instances
- Converts response envelopes back to JSON
- Recovers from panics to avoid crashing the server

**What enters:** Raw HTTP requests (`POST /api/v1/{action}`).

**What leaves:** JSON responses (success envelopes or error objects with `error` and `trace_id` fields).

### Agent Runtime Head

**Metaphor:** The Thinking -- manages autonomous agents.

**Package:** `internal/agent`

**Key types:**

| Type | Purpose |
|------|---------|
| `Runtime` | Top-level agent plane managing lifecycles, tool registration, and execution |
| `Agent` | A single autonomous unit of work with identity, state, context, and tools |
| `Func` | `func(ctx, *Agent) (any, error)` -- the function an agent executes |
| `Context` | Session-scoped context carrying identity, tenant, and inter-tool memory |
| `Executor` | Runs agent functions with configurable retry and timeout policies |
| `Tool` | Interface for callable capabilities an agent can invoke |
| `ToolRegistry` | Thread-safe catalog of tools |
| `State` | Agent lifecycle: `created`, `running`, `done`, `failed`, `timed_out` |

**Responsibilities:**

- Spawns agents with structured context (not prompt sludge)
- Registers and discovers tools via the tool registry
- Executes agent functions with exponential backoff retries and per-attempt timeouts
- Tracks agent state transitions throughout the lifecycle
- Cancels in-flight agents on shutdown

Hydra also ships with built-in tools under `internal/agent/tools/`, including `llm.complete`. That tool delegates prompt execution to the HTTP client in `internal/llm`, which talks to the optional Node.js sidecar in `tools/llm-sidecar/`.

**What enters:** An agent name, a `Func`, and optional configuration (tools, context).

**What leaves:** A completed `Agent` with its `Result` and final `State`.

### Execution Plane Head

**Metaphor:** The Muscle -- runs background work.

**Package:** `internal/execution`

**Key types:**

| Type | Purpose |
|------|---------|
| `Plane` | Accepts jobs, schedules them, and runs them through a handler |
| `Job` | A unit of work with payload, retry config, and lifecycle timestamps |
| `DAG` | Directed acyclic graph of jobs with dependency edges |
| `Queue` | Interface for job queues (enqueue, dequeue, ack, nack) |
| `InMemoryQueue` | Buffered, channel-backed queue implementation |
| `Scheduler` | Manages delayed job delivery by holding jobs until their `RunAfter` time |
| `JobHandler` | `func(ctx, *Job) error` -- processes a single job |
| `JobState` | Job lifecycle: `pending`, `running`, `completed`, `failed`, `retrying` |

**Responsibilities:**

- Accepts individual jobs and DAGs of interdependent jobs
- Schedules jobs for immediate or delayed execution
- Processes jobs from the queue with automatic retries and exponential backoff
- Performs topological sort on DAGs and detects cycles
- Tracks job state transitions throughout the lifecycle

**What enters:** `Job` or `DAG` instances submitted to the plane.

**What leaves:** Completed or failed jobs with results/errors, queryable by ID.

### Memory Plane Head

**Metaphor:** The Memory -- long-term organism recall.

**Package:** `internal/memory`

**Key types:**

| Type | Purpose |
|------|---------|
| `Plane` | Exposes Store, Recall, Search, and Forget operations |
| `Provider` | Interface for storage backends (Write, Read, Search, Delete) |
| `Memory` | The fundamental unit of stored memory with typed content and confidence scoring |
| `SearchQuery` | Constrains search by type, tags, ownership, time range, and confidence |
| `Type` | Memory classification: `episodic`, `semantic`, `operational`, `working` |

**Responsibilities:**

- Stores typed memory records with confidence scoring
- Retrieves memories by ID
- Searches memories by type, tags, actor, tenant, and time range
- Deletes (forgets) memories by ID
- Delegates storage to pluggable `Provider` backends

**What enters:** `Memory` records (via Store) or `SearchQuery` filters (via Search).

**What leaves:** Stored memory IDs, retrieved `Memory` records, or search result arrays.

### Policy Head

**Metaphor:** The Immune System -- structural guardrails.

**Package:** `internal/policy`

**Key types:**

| Type | Purpose |
|------|---------|
| `Engine` | Central policy decision point for permissions, budgets, tool ACLs, and audit |
| `Role` | Named set of permissions |
| `Permission` | Fine-grained capability (e.g., `agent:create`, `task:execute`) |
| `BudgetEntry` | Tracks remaining execution budget per actor |
| `ToolACL` | Controls which tools each actor is permitted to invoke |
| `AuditEntry` | Records a single policy decision for review and compliance |
| `AuditFilter` | Constrains audit log queries |

**Responsibilities:**

- Checks permissions via role-based access control (RBAC)
- Enforces execution budgets per actor
- Restricts tool access per actor via allow-lists
- Maintains an append-only audit trail of all policy decisions
- Ships with built-in default roles: `admin`, `agent`, `human`, `readonly`

**What enters:** Actor identity and requested permission/budget/tool.

**What leaves:** Allow/deny decisions, audit records.

### Adapters (Edge Heads)

**Metaphor:** Translation -- speaks the outside world.

**Package:** `internal/adapter`

**Key types:**

| Type | Package | Purpose |
|------|---------|---------|
| `Adapter` | `internal/adapter` | Interface: `ConvertToEnvelope` / `ConvertFromEnvelope` |
| `http.Adapter` | `internal/adapter/http` | JSON HTTP request bodies to/from Envelopes |
| `cli.Adapter` | `internal/adapter/cli` | CLI-style JSON input to Envelopes, human-readable output |
| `slack.Adapter` | `internal/adapter/slack` | Slack event payloads to Envelopes, with HMAC-SHA256 signature verification |

**Responsibilities:**

- Translate external formats into the internal Envelope protocol
- Translate response Envelopes back into transport-specific formats
- Extract actor identity and tenant from transport-specific fields
- Handle transport-specific concerns (e.g., Slack signature verification)

**What enters:** Raw bytes and metadata from the external transport.

**What leaves:** `Envelope` instances (inbound) or transport-specific bytes (outbound).

## Design Decisions

### Why Modular Monolith (Not Microservices)

Hydra starts as a modular monolith because the overhead of microservices (network calls, serialization, deployment complexity, distributed debugging) is not justified at MVP scale. The `Head` interface provides the extraction seam: when a head needs independent scaling, it can be pulled out into a separate service without changing its contract. Heads already communicate through typed interfaces, not shared databases.

### Why Go

Go provides the right tradeoffs for a backend that needs to be a single binary:

- Fast compilation and straightforward cross-compilation
- Goroutines and channels for concurrent job processing and event bus
- Strong standard library (`net/http` with Go 1.22+ routing, `encoding/json`, `log/slog`)
- Static typing catches errors at compile time, critical for policy enforcement code
- Single binary output simplifies deployment

### Why Structured Context (Not Prompt Sludge)

Agents receive typed `Context` objects carrying identity, tenant, session, and key-value memory -- not raw text blobs stuffed into a prompt. This means:

- Policy decisions can be made structurally (does this actor have the right role?) rather than by parsing prompt text
- Memory retrieval is queryable by type, tags, and confidence, not by hoping the LLM remembers
- Inter-tool communication happens through typed data, not through prompt engineering

### Why Backend-Enforced Policy (Not Prompt-Only)

Prompt-only policy (telling an LLM "don't do X") is a suggestion, not a guarantee. Hydra enforces policy structurally:

- Permission checks happen in code before any agent is spawned
- Budget limits are tracked numerically, not approximately
- Tool access is controlled by allow-lists, not by instructions
- Every decision is recorded in an audit trail

A compromised or confused agent cannot bypass policy because policy is enforced outside the agent's control.

### Why Single Binary

A single binary simplifies deployment, eliminates service mesh complexity, and makes the system easy to run locally for development. The internal package boundaries keep the code modular even though it compiles as one unit. When extraction is needed, the `Head` interface defines where to cut.

The core runtime still follows that model, but some optional developer workflows now depend on auxiliary processes. The LoCoMo LLM benchmark path and `llm.complete` use the Node.js sidecar in `tools/llm-sidecar/` as an external boundary for provider auth and completion requests.

## Package Map

| Package | Purpose | Key Types |
|---------|---------|-----------|
| `cmd/hydra` | Entrypoint | `main()` |
| `cmd/bench-locomo` | LoCoMo benchmark CLI | `main()` |
| `bench/locomo` | Benchmark ingestion, query planning, scoring, and reporting | `Strategy`, `LLMStrategy`, `BenchResult`, `QuestionScore` |
| `internal/body` | Shared core: envelope, identity, trace, eventbus, config, registry, health | `Envelope`, `Identity`, `Tenant`, `Span`, `EventBus`, `Config`, `Registry`, `HealthReport`, `Head` |
| `internal/gateway` | API gateway head -- HTTP routing, middleware, envelope conversion | `Gateway`, `HandlerFunc` |
| `internal/agent` | Agent runtime head -- agent lifecycle, tools, structured context, execution | `Runtime`, `Agent`, `Func`, `Context`, `Executor`, `Tool`, `ToolRegistry` |
| `internal/agent/tools` | Built-in tool implementations, including LLM completion | `LLMCompleteTool` |
| `internal/execution` | Execution plane head -- queues, jobs, DAGs, scheduler, retries | `Plane`, `Job`, `DAG`, `Queue`, `InMemoryQueue`, `Scheduler`, `JobHandler` |
| `internal/llm` | HTTP client for the external LLM sidecar | `Client`, `CompleteRequest`, `CompleteResponse`, `Usage` |
| `internal/memory` | Memory plane head -- typed storage with confidence scoring | `Plane`, `Provider`, `Memory`, `SearchQuery`, `Type` |
| `internal/memory/inmemory` | In-memory storage provider | `Provider` (in-memory implementation) |
| `internal/policy` | Policy engine head -- AuthN, AuthZ, budgets, tool ACLs, audit | `Engine`, `Role`, `Permission`, `BudgetEntry`, `ToolACL`, `AuditEntry` |
| `internal/adapter` | Adapter interface | `Adapter` |
| `internal/adapter/http` | HTTP JSON adapter | `Adapter` |
| `internal/adapter/cli` | CLI adapter | `Adapter` |
| `internal/adapter/slack` | Slack event adapter | `Adapter` |
| `configs/` | Configuration files | `hydra.example.yaml`, `hydra.yaml` |
| `tools/llm-sidecar/` | Optional Node.js sidecar for LLM completions | `/health`, `/complete` |
| `tests/integration/` | End-to-end lifecycle tests | `TestLifecycle` |
