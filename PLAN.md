# Hydra-DB — Plano de Implementação

## Visão Geral

Hydra-DB é uma plataforma de backend nativa para agentes de IA, projetada como um monólito modular em Go. Oferece um sistema de memória hierárquico, runtime de agentes configurável, plano de execução de tools, gateway HTTP e motor de políticas.

---

## Arquitetura

```
┌─────────────────────────────────────────────────────────┐
│                         Head                             │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │ Gateway  │ │  Agent   │ │ Execution│ │   Policy    │  │
│  │  (HTTP)  │ │ Runtime  │ │  Plane   │ │   Engine    │  │
│  └──────────┘ └──────────┘ └──────────┘ └─────────────┘  │
│  ┌──────────────────────────────────────────────────────┐ │
│  │                    Memory Plane                     │ │
│  │              (inmemory | tardigrade)                │ │
│  └──────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                         Body                             │
│  Identity · Trace · EventBus · Config · Registry · Health │
└─────────────────────────────────────────────────────────┘
```

---

## Stack Tecnológico

- **Linguagem:** Go 1.26
- **Build:** `go build -v ./...`
- **Testes:** `go test -race -count=1 ./...`
- **Linting:** `golangci-lint` (v2.11.4, config-free defaults)
- **CLI:** `cmd/hydra/main.go`
- **Configuração:** `configs/hydra.example.yaml`

---

## Funcionalidades por Plano

### Fase 1 — Fundação ✅

- [x] `body.Head` interface (seams para extração futura)
- [x] Memory plane com `Provider` interface
- [x] Provider in-memory (map-backed)
- [x] Provider tardigrade (HTTP para TDB Flask server)
- [x] Configuração via YAML (`configs/hydra.example.yaml`)
- [x] Gateway HTTP adapter
- [x] Agent runtime básico
- [x] Tool registry (sem tools registradas)
- [x] Execution plane com rate limiting (declarado, não implementado)
- [x] Policy head básico
- [x] README com Trinity Stack (Yell + Hydra + TardigradeDB)
- [x] CI com GitHub Actions (golangci-lint + go build + go test)

### Fase 2 — Integrações

- [x] TardigradeDB Provider (#1) — `internal/memory/tdb/provider.go`
- [ ] Go bindings nativos para TDB (cgo → Rust engine)
- [ ] Docker build otimizado (multi-stage, distroless)
- [ ] healthcheck endpoint completo (`GET /health`)
- [ ] Métricas (Prometheus? OpenTelemetry?)

### Fase 3 — Runtime de Agentes

- [ ] Tool registry — registrar tools built-in (HTTP, TDB, eval)
- [ ] Execution plane — rate limiting implementado
- [ ] Event bus — pub/sub para eventos entre heads
- [ ] Tracing distributed (OpenTelemetry)

### Fase 4 — Governance

- [ ] Background governance jobs (issue #12)
- [ ] Batch write API (issue #10)
- [ ] Relay caching (issue #13)
- [ ] Multi-model dimensions (issue #14)

---

## Issues Abertos

| # | Título | Prioridade | Status |
|---|--------|------------|--------|
| 1 | Memory Plane: TardigradeDB Provider | Alta | ✅ Fechado |
| 2 | Gap analysis — 10 lacunas identificadas | Média | 🔵 Aberto |
| 3 | README: Trinity Stack | Baixa | ✅ Fechado |
| — | Go bindings para TDB | Alta | 🟡 Backlog |
| — | Docker multi-stage | Média | 🟡 Backlog |
| — | Rate limiting implementado | Média | 🟡 Backlog |
| — | Tool registry built-in | Alta | 🟡 Backlog |

---

## Trinity Stack

```
Yell (UI)     → Hydra-DB (Backend API/events)  → TardigradeDB (KV Memory)
jared-openclawbot/yell-landing                eldriss/tardigrade-db
```

- **Yell:** Framework declarativo YAML para UI (componente registry, SSR, linter)
- **Hydra-DB:** Runtime de agentes com memory plane conectável
- **TardigradeDB:** Engine de memória vetorial em Rust (Python → futuro Go native)

---

## Configuração

```yaml
# configs/hydra.example.yaml
memory:
  provider: tardigrade  # or "inmemory"
  tardigrade:
    url: "http://localhost:8765"
    dir: ".tdb"

gateway:
  host: "0.0.0.0"
  port: 8080
  rate_limit: 100  # declared, not yet implemented

agent:
  model: "gpt-4"
  max_steps: 100
```

---

## Variáveis de Ambiente

| Variável | Descrição |
|----------|-----------|
| `HYDRA_CONFIG` | Caminho para arquivo YAML de config (default: `configs/hydra.yaml`) |
| `HYDRA_MEMORY_PROVIDER` | Override provider (`inmemory` \| `tardigrade`) |
| `HYDRA_TDB_URL` | URL do servidor TDB (para provider tardigrade) |
