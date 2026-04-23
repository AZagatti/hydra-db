# API Reference

## Overview

All Hydra endpoints use `POST` with a JSON body and return JSON responses. The URL path determines the action:

```
POST /api/v1/{action}
```

Where `{action}` is one of: `health`, `chat`, `task`, `memory.store`, `memory.search`.

## Authentication

Actor identity is provided in the request body (not via headers). Each request includes an `actor` object with the caller's identity:

```json
{
  "actor": {
    "id": "user-001",
    "kind": "human",
    "roles": ["agent"],
    "tenant_id": "tenant-acme"
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `actor.id` | string | No | Unique actor identifier. Defaults to `"anonymous"`. |
| `actor.kind` | string | No | Actor type: `human`, `agent`, `tool`, `system`. Defaults to `"human"`. |
| `actor.roles` | string[] | No | Role names for RBAC. Determines permissions. |
| `actor.tenant_id` | string | No | Tenant this actor belongs to. Must match `tenant.id` if both provided. |

The `Authorization` header (if present) is captured in the envelope metadata but is not directly used for authentication in the current implementation.

## Request Format

All requests use `Content-Type: application/json`:

```
POST /api/v1/{action} HTTP/1.1
Content-Type: application/json
X-Trace-ID: optional-trace-id

{...json body...}
```

## Response Format

Successful responses return `200 OK` with an Envelope JSON object:

```json
{
  "id": "response-uuid",
  "trace_id": "trace-id",
  "actor": {"id": "user-001", "kind": "human", "roles": ["agent"], "tenant_id": "tenant-acme"},
  "tenant": {"id": "tenant-acme", "name": "Acme Corp"},
  "type": "response",
  "action": "chat",
  "payload": {...action-specific result...},
  "timestamp": "2026-04-23T10:30:00Z"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique response envelope ID |
| `trace_id` | string | Correlation trace ID (matches request) |
| `actor` | object | Actor identity from the request |
| `tenant` | object | Tenant from the request |
| `type` | string | Always `"response"` for success |
| `action` | string | The action that was performed |
| `payload` | object | Action-specific result data |
| `timestamp` | string | ISO 8601 timestamp |

## Error Format

Errors return the appropriate HTTP status code with:

```json
{
  "error": "human-readable error message",
  "trace_id": "trace-id-for-correlation"
}
```

| HTTP Status | When |
|-------------|------|
| 400 | Invalid JSON body |
| 404 | Unknown action (no handler registered) |
| 405 | Method not POST |
| 500 | Handler error (permission denied, execution failure, etc.) |

---

## POST /api/v1/health

Returns the health status of all Hydra heads.

**Request:** No body required.

**Response:**

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | Overall status: `"healthy"` or `"unhealthy"` |
| `heads` | string[] | Names of all registered heads |
| `reports` | object[] | Per-head health reports |

Each health report:

| Field | Type | Description |
|-------|------|-------------|
| `head` | string | Head name |
| `status` | string | `"healthy"`, `"degraded"`, or `"unhealthy"` |
| `detail` | string | Optional detail message |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/health
```

```json
{
  "id": "a1b2c3d4-...",
  "trace_id": "e5f6g7h8-...",
  "actor": {"id": "", "kind": "", "roles": null, "tenant_id": ""},
  "tenant": {"id": "", "name": ""},
  "type": "response",
  "action": "health",
  "payload": {
    "status": "healthy",
    "heads": ["gateway", "agent", "execution", "memory", "policy"],
    "reports": [
      {"head": "gateway", "status": "healthy"},
      {"head": "agent", "status": "healthy"},
      {"head": "execution", "status": "healthy"},
      {"head": "memory", "status": "healthy"},
      {"head": "policy", "status": "healthy", "detail": "policy engine operational"}
    ]
  },
  "timestamp": "2026-04-23T10:30:00Z"
}
```

---

## POST /api/v1/chat

Chat with an agent. Spawns an ephemeral agent that processes the message and returns a result.

**Required permission:** `agent:create` (requires `agent` or `admin` role).

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `message` | string | Yes | The message to send to the agent. |
| `actor` | object | No | Actor identity. Defaults to anonymous human with `"human"` role. |
| `tenant` | object | No | Tenant. Defaults to `{id: "default", name: "default"}`. |

**Response payload:**

| Field | Type | Description |
|-------|------|-------------|
| `echo` | string | Echo of the original message (MVP behavior). |
| `agent_id` | string | UUID of the agent that processed the message. |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -H "X-Trace-ID: my-trace-123" \
  -d '{
    "message": "Hello, Hydra!",
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

Response:

```json
{
  "id": "b2c3d4e5-...",
  "trace_id": "my-trace-123",
  "actor": {"id": "user-001", "kind": "human", "roles": ["agent"], "tenant_id": "tenant-acme"},
  "tenant": {"id": "tenant-acme", "name": "Acme Corp"},
  "type": "response",
  "action": "chat",
  "payload": {"echo": "Hello, Hydra!", "agent_id": "f7g8h9i0-..."},
  "timestamp": "2026-04-23T10:30:00Z"
}
```

---

## POST /api/v1/task

Submit a background task for asynchronous execution. The task is enqueued and processed by the execution plane.

**Required permission:** `task:execute` (requires `agent`, `human`, or `admin` role).

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Task type identifier (e.g., `"email"`, `"report"`). |
| `data` | object | No | Arbitrary JSON payload for the task. |
| `actor` | object | No | Actor identity. Defaults to anonymous human. |
| `tenant` | object | No | Tenant. Defaults to `{id: "default", name: "default"}`. |

**Response payload:**

| Field | Type | Description |
|-------|------|-------------|
| `job_id` | string | UUID of the submitted job. |
| `state` | string | Initial state: `"pending"`. |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/task \
  -H "Content-Type: application/json" \
  -d '{
    "type": "generate_report",
    "data": {"report_type": "monthly", "month": "2026-03"},
    "actor": {
      "id": "user-001",
      "kind": "human",
      "roles": ["human"],
      "tenant_id": "tenant-acme"
    },
    "tenant": {
      "id": "tenant-acme",
      "name": "Acme Corp"
    }
  }'
```

Response:

```json
{
  "id": "c3d4e5f6-...",
  "trace_id": "...",
  "actor": {"id": "user-001", "kind": "human", "roles": ["human"], "tenant_id": "tenant-acme"},
  "tenant": {"id": "tenant-acme", "name": "Acme Corp"},
  "type": "response",
  "action": "task",
  "payload": {"job_id": "j1k2l3m4-...", "state": "pending"},
  "timestamp": "2026-04-23T10:30:00Z"
}
```

---

## POST /api/v1/memory.store

Store a typed memory record in the memory plane.

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | No | Memory type: `episodic`, `semantic`, `operational`, `working`. Defaults to `"semantic"`. |
| `content` | object | No | Arbitrary JSON content to store. |
| `tags` | string[] | No | Tags for categorization and retrieval. |
| `actor_id` | string | No | Actor ID that owns this memory. Defaults to `"anonymous"`. |
| `tenant_id` | string | No | Tenant this memory belongs to. Defaults to `"default"`. |

**Response payload:**

| Field | Type | Description |
|-------|------|-------------|
| `memory_id` | string | UUID of the stored memory record. |
| `type` | string | The memory type that was stored. |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/memory.store \
  -H "Content-Type: application/json" \
  -d '{
    "type": "semantic",
    "content": {"fact": "Hydra is an agent-native backend built in Go"},
    "tags": ["architecture", "go"],
    "actor_id": "user-001",
    "tenant_id": "tenant-acme"
  }'
```

Response:

```json
{
  "id": "d4e5f6g7-...",
  "trace_id": "...",
  "actor": {"id": "user-001", "kind": "", "roles": null, "tenant_id": ""},
  "tenant": {"id": "tenant-acme", "name": ""},
  "type": "response",
  "action": "memory.store",
  "payload": {"memory_id": "m1n2o3p4-...", "type": "semantic"},
  "timestamp": "2026-04-23T10:30:00Z"
}
```

---

## POST /api/v1/memory.search

Search for stored memories matching the given criteria.

**Required permission:** `memory:read` (requires `readonly`, `human`, `agent`, or `admin` role).

**Request:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | No | Filter by memory type: `episodic`, `semantic`, `operational`, `working`. |
| `tags` | string[] | No | Filter by tags. |
| `actor_id` | string | No | Filter by owning actor. |
| `tenant_id` | string | No | Filter by tenant. |
| `limit` | int | No | Maximum number of results to return. |

**Response payload:**

| Field | Type | Description |
|-------|------|-------------|
| `count` | int | Number of results returned. |
| `results` | object[] | Array of matching memory records. |

Each memory record:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Memory UUID |
| `type` | string | Memory type |
| `content` | object | Stored JSON content |
| `tags` | string[] | Tags |
| `confidence` | float | Confidence score (0.0--1.0) |
| `actor_id` | string | Owning actor |
| `tenant_id` | string | Owning tenant |
| `created_at` | string | ISO 8601 creation timestamp |
| `accessed_at` | string | ISO 8601 last access timestamp |

**Example:**

```bash
curl -X POST http://localhost:8080/api/v1/memory.search \
  -H "Content-Type: application/json" \
  -d '{
    "type": "semantic",
    "tags": ["architecture"],
    "tenant_id": "tenant-acme",
    "limit": 10
  }'
```

Response:

```json
{
  "id": "e5f6g7h8-...",
  "trace_id": "...",
  "actor": {"id": "", "kind": "", "roles": null, "tenant_id": ""},
  "tenant": {"id": "", "name": ""},
  "type": "response",
  "action": "memory.search",
  "payload": {
    "count": 1,
    "results": [
      {
        "id": "m1n2o3p4-...",
        "type": "semantic",
        "content": {"fact": "Hydra is an agent-native backend built in Go"},
        "tags": ["architecture", "go"],
        "confidence": 1.0,
        "actor_id": "user-001",
        "tenant_id": "tenant-acme",
        "created_at": "2026-04-23T10:25:00Z",
        "accessed_at": "2026-04-23T10:25:00Z"
      }
    ]
  },
  "timestamp": "2026-04-23T10:30:00Z"
}
```

---

## Roles and Permissions

Hydra ships with four built-in roles. Each role grants a set of permissions:

| Role | Permissions | Description |
|------|------------|-------------|
| `admin` | `agent:create`, `task:execute`, `memory:read`, `memory:write`, `admin:all` | Full access to all operations. |
| `agent` | `agent:create`, `task:execute`, `memory:read`, `memory:write` | Autonomous agent with task and memory access. |
| `human` | `task:execute`, `memory:read` | Human operator who can submit tasks and read memories. |
| `readonly` | `memory:read` | Read-only access to memories. |

**Permission-to-endpoint mapping:**

| Endpoint | Required Permission | Allowed Roles |
|----------|-------------------|---------------|
| `POST /api/v1/health` | None (open) | Any |
| `POST /api/v1/chat` | `agent:create` | `agent`, `admin` |
| `POST /api/v1/task` | `task:execute` | `human`, `agent`, `admin` |
| `POST /api/v1/memory.store` | None (open) | Any |
| `POST /api/v1/memory.search` | None (open in current implementation) | Any |

## Trace IDs

Every request is assigned a trace ID for end-to-end correlation. The trace ID appears in:

- The response JSON (`trace_id` field)
- Error responses (`trace_id` field)
- Audit log entries
- Event bus events

**Providing a trace ID:** Include an `X-Trace-ID` header in the request:

```
X-Trace-ID: my-custom-trace-id
```

**Auto-generation:** If no `X-Trace-ID` header is provided, Hydra generates a UUID and uses it as the trace ID. The generated trace ID is returned in the response so the caller can use it for correlation.
