# Hydra -- Agent-Native Backend Organism

> *"Yell draws the interface, Hydra operates the organism, TardigradeDB remembers the trauma."*

## The Trinity Stack

Hydra is designed to compose with two other projects for a complete agent-native platform:

```
┌─────────────────────────────────────┐
│        YELL (UI Layer)              │  github.com/jared-openclawbot/yell-landing
│  Declarative YAML → HTML/React      │  Declarative component schema, linter,
│  Schema validation, SSR             │  live playground with GitHub API fetch
└──────────────┬──────────────────────┘
               │ HTTP API
┌──────────────▼──────────────────────┐
│        HYDRA (Backend Plane)        │  github.com/Azagatti/hydra-db
│  Agent runtime, jobs, events,       │  Go backend: gateway, agent runtime,
│  auth (RBAC), HTTP adapter          │  execution plane, policy engine
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│      TARDIGRADEDB (Memory)          │  github.com/Eldriss-Studio/tardigrade-db
│  Persistent KV cache, latent         │  Rust: mem_write/mem_read, Q4/Q8
│  retrieval, multi-tier memory        │  compression, AKL governance
└─────────────────────────────────────┘
```

Configure Hydra to use TardigradeDB as the memory provider:

```yaml
memory:
  provider: tardigrade
  tdb_url: http://localhost:8765   # TardigradeDB HTTP API server
```

## What is Hydra

Hydra is a backend platform designed for AI agents, humans, tools, and event-driven workflows. It is built as a modular monolith where each subsystem is a "head" with a specialized role -- API gateway, agent runtime, job execution, memory storage, policy enforcement -- all sharing a common "body" of identity, observability, configuration, and state.

This is not a generic web framework. Hydra is an agent-native operating backend: it enforces policy structurally (not via prompts), executes durable jobs with retries and DAGs, maintains typed memory with confidence scoring, and traces every action end-to-end. The architecture assumes autonomous agents as first-class citizens alongside human operators.

The codebase is a single Go binary with clean internal boundaries. Heads communicate through typed interfaces and a shared event bus. When real scaling requirements emerge, the `Head` interface already defines the seams for extraction into separate services.

## Architecture

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

The body provides shared contracts every head depends on: the universal `Envelope` for request/response normalization, `Identity` for actor/tenant context, `Span` for distributed tracing, `EventBus` for internal pub/sub, and `Registry` for head lifecycle management.

## Heads

| Head | Role | Metaphor |
|------|------|----------|
| **Gateway** | API gateway, routing, auth middleware, rate limiting | Mouth -- receives all inbound traffic |
| **Agent Runtime** | Agent lifecycle, tool attachment, structured context, retry/timeout execution | Thinking -- manages autonomous agents |
| **Execution Plane** | Queues, jobs, state machines, DAGs, delayed scheduling, concurrency limits | Muscle -- runs background work |
| **Memory Plane** | Store, retrieve, search typed memories with confidence scoring | Memory -- long-term organism recall |
| **Policy** | AuthN (JWT + API key), AuthZ (RBAC), budgets, tool ACLs, audit logging | Immune system -- structural guardrails |
| **Adapters** | HTTP, CLI, Slack -- translate external formats to/from Envelope | Translation -- speaks the outside world |

## Quick Start

```bash
git clone https://github.com/azagatti/hydra-db.git
cd hydra-db
cp configs/hydra.example.yaml configs/hydra.yaml
make build
make run
```

The server starts on `localhost:8080` by default.

## API Endpoints

All endpoints accept `POST` with a JSON body and return a JSON response.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/chat` | Chat with an agent |
| `POST` | `/api/v1/task` | Submit a background task |
| `POST` | `/api/v1/memory.store` | Store a memory |
| `POST` | `/api/v1/memory.search` | Search memories |
| `POST` | `/api/v1/health` | Get system health |

### Examples

**Health check:**

```bash
curl -X POST http://localhost:8080/api/v1/health
```

**Chat with an agent:**

```bash
curl -X POST http://localhost:8080/api/v1/chat \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{"message": "Hello", "context": {}}'
```

## Configuration

Copy and edit the example config:

```bash
cp configs/hydra.example.yaml configs/hydra.yaml
```

Config structure:

```yaml
hydra:
  name: hydra
  version: "0.1.0"
  log_level: info

gateway:
  host: localhost
  port: 8080
  read_timeout: 30
  write_timeout: 30

policy:
  default_budget: 10000
  rate_limit: 100

logging:
  level: info
  format: json
```

Environment variable overrides use the `HYDRA_` prefix with double-underscore delimiters for nesting. For example:

```bash
HYDRA_GATEWAY_PORT=9090
HYDRA_LOGGING_LEVEL=debug
HYDRA_POLICY_DEFAULT_BUDGET=50000
```

## LoCoMo Benchmark

Hydra includes a benchmark suite based on [LoCoMo](https://github.com/snap-research/locomo) (Long-term Conversational Memory), the standard evaluation for agent memory systems.

### Baseline Results (April 2026)

| Strategy | Precision | Recall | F1 | Notes |
|----------|-----------|--------|----|-------|
| **basic** (brute-force) | 0.24% | 99.7% | 0.48% | Returns all turns, no filtering |
| **llm** (classified) | 4.74% | 60.4% | 7.31% | LLM-assigned types, confidence, tags |

For comparison, top systems on LoCoMo score 88-92% F1. The gap represents the opportunity for improvements in embeddings (#15), temporal reasoning (#17), multi-hop retrieval (#18), and memory consolidation (#19).

### Running the Benchmark

```bash
# Basic strategy (no LLM needed)
make bench-locomo

# Run the CLI directly with custom flags
go run ./cmd/bench-locomo --limit 5
go run ./cmd/bench-locomo --data /path/to/locomo10.json --json > locomo-report.json

# LLM strategy (requires sidecar)
make sidecar-install
npx @mariozechner/pi-ai login openai-codex   # authenticate once
make sidecar-start                            # in a separate terminal
make bench-locomo STRATEGY=llm LIMIT=1        # run on 1 sample
go run ./cmd/bench-locomo --strategy llm --sidecar-url http://localhost:3100 --limit 1
```

CLI flags:

- `--data`: path to `locomo10.json` (downloads automatically when empty)
- `--json`: emit JSON instead of the table report
- `--limit`: process only the first N samples
- `--strategy`: `basic` or `llm`
- `--sidecar-url`: override the sidecar URL for the `llm` strategy

If `--data` is omitted, Hydra downloads the dataset on first run and caches it under `bench/locomo/testdata/`.

### LLM Sidecar

The LLM sidecar (`tools/llm-sidecar/`) is a Node.js HTTP service wrapping [pi-ai](https://github.com/badlogic/pi-mono/tree/main/packages/ai) for unified multi-provider LLM access with OAuth support.

```bash
make sidecar-install   # npm install
make sidecar-start     # runs on :3100
```

Relevant environment variables:

- `LLM_SIDECAR_PORT`: sidecar listen port (default `3100`)
- `PI_AI_AUTH_PATH`: explicit `auth.json` path for pi-ai credentials
- `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`: API-key auth fallback
- `ANTHROPIC_MODEL` / `OPENAI_MODEL`: override default provider models

### LLM Integration Tests

```bash
export LLM_SIDECAR_URL=http://localhost:3100
go test ./tests/integration -run TestLLMAgent_ClassifyAndStoreMemories -v
```

## Development

```bash
make test           # Run tests (with race detector)
make lint           # Run golangci-lint
make cover          # Generate coverage report (coverage.html)
make build          # Build binary to bin/hydra
make run            # Build and run
make bench-locomo   # Run LoCoMo memory benchmark
make clean          # Remove build artifacts
```

Testing follows TDD: table-driven tests with `testify`, integration tests in `tests/integration/`, coverage target above 80%.

## Project Structure

```
hydra-db/
├── cmd/
│   ├── hydra/main.go              # Main server entry point
│   └── bench-locomo/main.go       # LoCoMo benchmark CLI
├── bench/locomo/                  # Benchmark: types, dataset, ingest, query, scoring
├── internal/
│   ├── body/                      # Shared core: envelope, identity, trace, eventbus, config, registry, health
│   ├── gateway/                   # API gateway head
│   ├── agent/                     # Agent runtime head
│   │   └── tools/                 # Built-in tools (llm.complete)
│   ├── execution/                 # Execution plane head
│   ├── llm/                       # LLM sidecar client
│   ├── memory/                    # Memory plane head (+ inmemory/ and tdb/ providers)
│   ├── policy/                    # Policy engine head
│   └── adapter/                   # Interface adapters (http/, cli/, slack/)
├── tools/llm-sidecar/             # Node.js LLM sidecar (pi-ai)
├── tests/integration/             # End-to-end lifecycle tests
├── configs/
│   └── hydra.example.yaml
├── go.mod
├── Makefile
├── .golangci.yml
└── PLAN.md
```

## Design Principles

1. Agent-native, not chatbot-native -- built for autonomous execution, not just Q&A
2. Backend-enforced policy, not prompt-only policy -- structural guardrails, not text instructions
3. Durable execution over ad hoc loops -- state machines, retries, not while(true)
4. Memory-aware, but memory-skeptical -- store what matters, question what you retrieve
5. Channel-agnostic core, channel-specific edges -- core knows nothing about Slack/HTTP
6. Replaceable heads, shared body -- swap a head without touching the organism
7. Event-driven where useful, synchronous where necessary -- don't over-async
8. Observability first -- if you can't trace it, it didn't happen
9. Human override always possible -- any agent action can be intercepted by a human
10. Graceful degradation -- a head failure shouldn't kill the organism
11. Structured context instead of giant prompt sludge -- typed data, not text blobs

## Stack

| Concern | Technology |
|---------|-----------|
| Language | Go |
| HTTP server | `net/http` (stdlib, Go 1.22+ routing) |
| HTTP client | `resty` |
| Config | `koanf` + YAML |
| Logging | `slog` (stdlib) |
| Testing | `testify` |
| Validation | `go-playground/validator` |
| Linting | `golangci-lint` |

## Status

**MVP / v1 -- in development.**

Included: API gateway with routing and middleware, single ephemeral agent runtime with tools, execution plane with queue/DAG/scheduler/retry, policy engine with AuthN/AuthZ/budgets/audit, memory plane with in-memory provider, HTTP/CLI/Slack adapters, full tracing and integration tests.

Not yet: multi-agent trees, TardigradeDB integration, WebSocket streaming, semantic search, MCP adapter, distributed deployment. See [PLAN.md](PLAN.md) for the full roadmap.

## License

MIT License (see LICENSE file).
