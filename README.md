# Hydra

**The agent-native backend organism.**

Hydra is a backend platform for AI agents, autonomous workflows, and human-agent collaboration. It is not a web framework with agent features. It is a new category of infrastructure designed from scratch for a world where the primary actors are not humans clicking buttons, but agents executing plans.

> *"Yell draws the interface, Hydra operates the organism, TardigradeDB remembers the trauma."*

[![CI](https://github.com/AZagatti/hydra-db/actions/workflows/ci.yml/badge.svg)](https://github.com/AZagatti/hydra-db/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/AZagatti/hydra-db)](https://goreportcard.com/report/github.com/AZagatti/hydra-db)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

---

## The problem

Traditional backends are built around the request-response cycle: a human makes a request, the server responds. This model breaks down when your primary users are AI agents that execute multi-step plans over hours, call external tools, maintain memory across sessions, collaborate with other agents, and need structural safety enforcement -- not just prompt-level guardrails.

Agent frameworks (LangChain, CrewAI) handle how agents *think*. Orchestration platforms (Temporal, BullMQ) handle how tasks *run*. Web frameworks (Express, FastAPI) handle how HTTP *routes*. But none of them answer: **how do you safely operate an autonomous agent in production?**

Hydra answers this. It is the execution environment, the policy boundary, the memory substrate, the audit trail, and the multi-channel interface -- all in one coherent system.

## How Hydra is different

| Concern | Traditional Backend | Agent Framework | Hydra |
|---------|-------------------|-----------------|-------|
| Primary actor | Human | LLM | Agent, human, tool, or system |
| Safety model | Auth middleware | Prompt instructions | Structural policy enforcement |
| Execution model | Request-response | Chain-of-thought | Durable agent execution with retries |
| Memory | Database queries | Context window | Typed memory with confidence scoring |
| Channels | HTTP | Chat interface | HTTP, CLI, Slack, webhooks, events |
| Observability | Request logs | Token logs | Full execution traces with lineage |
| Unit of work | HTTP request | LLM call | Agent execution plan |

**Hydra does not replace agent frameworks. It provides the operating environment they run inside.** You can run LangChain inside Hydra's agent runtime. Hydra handles everything the framework doesn't: permissions, budgets, audit, memory, routing, multi-channel delivery, and crash recovery.

## The architecture

Hydra is a **modular monolith** -- a single Go binary with six specialized subsystems called **heads**, all sharing one **body** of identity, observability, and state. The organism metaphor is structural, not decorative.

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

**The body** provides shared contracts every head depends on: the universal `Envelope` for request normalization, `Identity` for actor/tenant context, `Span` for distributed tracing, `EventBus` for internal pub/sub, and `Registry` for lifecycle management. This is what makes Hydra one organism, not six random services.

**Each head has a specialized role:**

| Head | What it does | Why it matters |
|------|-------------|----------------|
| **Gateway** | Receives all inbound traffic, normalizes into Envelopes, routes to handlers | One entry point for everything -- HTTP, Slack, CLI, webhooks |
| **Agent Runtime** | Spawns agents, manages lifecycle, attaches tools, handles retry/timeout | Agents are first-class citizens with structured context, not prompt hacks |
| **Execution Plane** | Queues, jobs, DAGs, delayed scheduling, retries with backoff | Background work is durable, ordered, and recoverable |
| **Memory Plane** | Store, retrieve, search typed memories with confidence scoring | Agents remember across sessions, but the system questions what it recalls |
| **Policy Engine** | AuthN, AuthZ (RBAC), budgets, tool ACLs, audit logging | Guardrails enforced structurally, not via prompt instructions |
| **Adapters** | HTTP, CLI, Slack -- translate external formats to/from Envelope | Core is channel-agnostic; adding a new channel means writing one adapter |

Heads communicate through typed interfaces. The body makes them feel like one system. When scaling demands it, the `Head` interface defines clean seams for extraction into separate services -- but you don't pay that complexity until you need it.

## Philosophy in brief

Hydra is built on specific architectural convictions, not generic best practices:

1. **Backend-enforced policy, not prompt-only policy.** Prompts can be ignored. Structural enforcement cannot.
2. **Agent-native, not chatbot-native.** Agents execute plans. Chatbots respond to messages. These are different.
3. **Structured context, not prompt sludge.** Typed data structures that can be validated, filtered, and audited -- not 50KB text dumps.
4. **Memory-aware, but memory-skeptical.** Memory is useful. It is also dangerous. Confidence scores, timestamps, and type tags keep it honest.
5. **Observability is not optional.** If you cannot trace an action, it didn't happen. Every action has a correlation ID, span, and audit entry.
6. **Human override always exists.** No matter how autonomous the system, a human can intercept and override any action.
7. **The architecture earns its complexity.** Every abstraction solves a real problem. Nothing is ceremony.

Read the full [Philosophy document](docs/philosophy.md) for the detailed reasoning behind every decision.

## Installation

### Binary release (recommended)

Download the latest binary from [GitHub Releases](https://github.com/AZagatti/hydra-db/releases):

```bash
# Linux
curl -LO https://github.com/AZagatti/hydra-db/releases/latest/download/hydra-linux-amd64
chmod +x hydra-linux-amd64
sudo mv hydra-linux-amd64 /usr/local/bin/hydra

# macOS (Apple Silicon)
curl -LO https://github.com/AZagatti/hydra-db/releases/latest/download/hydra-darwin-arm64
chmod +x hydra-darwin-arm64
sudo mv hydra-darwin-arm64 /usr/local/bin/hydra

# macOS (Intel)
curl -LO https://github.com/AZagatti/hydra-db/releases/latest/download/hydra-darwin-amd64
chmod +x hydra-darwin-amd64
sudo mv hydra-darwin-amd64 /usr/local/bin/hydra
```

### Go install

```bash
go install github.com/AZagatti/hydra-db/cmd/hydra@latest
```

### Docker

```bash
docker run -p 8080:8080 ghcr.io/azagatti/hydra-db:latest
```

### Build from source

```bash
git clone https://github.com/AZagatti/hydra-db.git
cd hydra-db
make build
./bin/hydra
```

## Quick Start

After installing, create a config file and run:

```bash
# Create config (use default values)
cp configs/hydra.example.yaml configs/hydra.yaml

# Start the server
hydra
```

The server starts on `localhost:8080` by default.

```bash
# Check system health
curl -X POST http://localhost:8080/api/v1/health

# Chat with an agent (requires agent role)
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"message":"hello","actor":{"id":"agent-1","kind":"agent","roles":["agent"],"tenant_id":"default"},"tenant":{"id":"default"}}'

# Store a memory
curl -X POST http://localhost:8080/api/v1/memory.store \
  -H "Content-Type: application/json" \
  -d '{"content":"deployment succeeded","type":"episodic","actor":{"id":"agent-1","kind":"agent","roles":["agent"],"tenant_id":"default"},"tenant":{"id":"default"}}'
```

See [docs/api.md](docs/api.md) for the full endpoint reference.

## Documentation

| Document | Description |
|----------|-------------|
| [Philosophy](docs/philosophy.md) | Why Hydra exists, what makes it different, core beliefs |
| [Architecture](docs/architecture.md) | Heads, body types, design decisions, package map |
| [Request Lifecycle](docs/request-lifecycle.md) | The 16-step end-to-end flow through the organism |
| [Configuration](docs/configuration.md) | All config options, env var overrides, examples |
| [API Reference](docs/api.md) | Endpoints, request/response formats, curl examples |
| [OpenAPI Spec](api/openapi.yaml) | Machine-readable API specification |

## Development

```bash
make install     # Install dev tools (golangci-lint, goimports, lefthook)
make test        # Run all tests with race detector
make lint        # Run golangci-lint
make cover       # Generate HTML coverage report
make build       # Build binary to bin/hydra
make run         # Build and run
```

## Project Structure

```
hydra-db/
├── cmd/hydra/main.go           # Entrypoint -- wires all heads
├── internal/
│   ├── body/                   # Shared core: Envelope, Identity, Span, EventBus, Config, Registry
│   ├── gateway/                # Head 1: API gateway, routing, middleware
│   ├── agent/                  # Head 2: Agent runtime, lifecycle, tools, context
│   ├── execution/              # Head 3: Queue, jobs, DAGs, scheduler, retry
│   ├── memory/                 # Head 4: Memory plane + in-memory provider
│   ├── policy/                 # Head 5: Auth, RBAC, budgets, tool ACLs, audit
│   └── adapter/                # Head 6: HTTP, CLI, Slack adapters
├── api/openapi.yaml            # OpenAPI 3.1 specification
├── configs/hydra.example.yaml  # Example configuration
├── docs/                       # Architecture, philosophy, API docs
├── tests/integration/          # End-to-end lifecycle tests
├── .github/workflows/          # CI (build/test/lint) + release workflows
├── lefthook.yml                # Pre-commit hooks (goimports, vet, test)
└── Makefile                    # Build, test, lint, install targets
```

## Status

**MVP / v0.1 -- in development.**

**Included:** Gateway with routing and middleware, single ephemeral agent runtime with tools, execution plane with queue/DAG/scheduler/retry, policy engine with AuthN/AuthZ/budgets/audit, memory plane with in-memory provider, HTTP/CLI/Slack adapters, 157 tests with race detection, full tracing, and integration tests.

**Not yet:** Multi-agent trees, TardigradeDB integration, WebSocket streaming, semantic search, MCP adapter, distributed deployment.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, commit conventions, and code standards.

## License

[MIT](LICENSE)
