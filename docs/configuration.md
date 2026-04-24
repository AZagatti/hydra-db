# Configuration

## Overview

Hydra is configured through a YAML file with environment variable overrides. The configuration system uses [koanf](https://github.com/knadh/koanf) to merge three layers in order of precedence (highest wins):

1. **Environment variables** (`HYDRA_` prefix)
2. **Config file** (`configs/hydra.yaml`)
3. **Built-in defaults**

## Config File Location

The default config file is `configs/hydra.yaml`. Copy the example file to get started:

```bash
cp configs/hydra.example.yaml configs/hydra.yaml
```

If the config file is missing or unreadable, Hydra logs a warning and falls back to built-in defaults.

## Full Config Reference

### `hydra` -- Core Identity

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `name` | string | `"hydra"` | Service name used in logs and health reports. |
| `version` | string | `"0.1.0"` | Version string reported in startup logs and health endpoints. |
| `log_level` | string | `"info"` | Log level for the core Hydra logger. One of: `debug`, `info`, `warn`, `error`. |

### `gateway` -- HTTP Listener

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"localhost"` | TCP bind host. Use `"0.0.0.0"` to listen on all interfaces. |
| `port` | int | `8080` | TCP bind port. Must be 1--65535. |
| `read_timeout` | int | `30` | HTTP read timeout in seconds. Controls how long the server waits for the request body. Must be non-negative. |
| `write_timeout` | int | `30` | HTTP write timeout in seconds. Controls how long the server waits for the response to be written. Must be non-negative. |

### `policy` -- Policy Engine

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `default_budget` | int | `10000` | Default execution budget per actor when no explicit budget is set. Must be non-negative. |
| `rate_limit` | int | `100` | Rate limit per actor (requests per window). Must be non-negative. |

### `logging` -- Log Output

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `level` | string | `"info"` | Log verbosity. One of: `debug`, `info`, `warn`, `error`. |
| `format` | string | `"json"` | Output format. One of: `json` (structured JSON), `text` (key=value), `console`. |

## Environment Variable Overrides

Environment variables override config file values using the `HYDRA_` prefix with double-underscore (`__`) as the nesting delimiter.

**Pattern:** `HYDRA_<SECTION>__<FIELD>=<value>`

The environment variable name is transformed by:

1. Stripping the `HYDRA_` prefix
2. Converting to lowercase
3. Replacing `__` with `.` (dot notation for nesting)

### Examples

| Environment Variable | Config Path | Effect |
|---------------------|-------------|--------|
| `HYDRA_GATEWAY__PORT=9090` | `gateway.port` | Bind to port 9090 instead of 8080 |
| `HYDRA_GATEWAY__HOST=0.0.0.0` | `gateway.host` | Listen on all network interfaces |
| `HYDRA_LOGGING__LEVEL=debug` | `logging.level` | Enable debug-level logging |
| `HYDRA_LOGGING__FORMAT=text` | `logging.format` | Switch to text-format logging |
| `HYDRA_POLICY__DEFAULT_BUDGET=50000` | `policy.default_budget` | Set default budget to 50000 |
| `HYDRA_POLICY__RATE_LIMIT=200` | `policy.rate_limit` | Increase rate limit to 200 |
| `HYDRA_HYDRA__LOG_LEVEL=debug` | `hydra.log_level` | Set core Hydra log level to debug |
| `HYDRA_HYDRA__NAME=my-hydra` | `hydra.name` | Change service name |

### Nested Section Names

Note that top-level section names appear twice for sections under `hydra`. For example, to set `hydra.log_level`, the variable is `HYDRA_HYDRA__LOG_LEVEL` -- the first `HYDRA_` is the prefix, and `HYDRA__LOG_LEVEL` maps to `hydra.log_level`.

## Sidecar and LLM Integration

The optional LoCoMo LLM workflow and the built-in `llm.complete` tool use the Node.js sidecar in `tools/llm-sidecar/`. These settings are separate from Hydra's YAML config and `HYDRA_` environment variables.

### Sidecar Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `LLM_SIDECAR_PORT` | `3100` | HTTP port the sidecar listens on |
| `PI_AI_AUTH_PATH` | unset | Explicit path to a pi-ai `auth.json` file |
| `ANTHROPIC_API_KEY` | unset | Anthropic API-key fallback |
| `OPENAI_API_KEY` | unset | OpenAI API-key fallback |
| `ANTHROPIC_MODEL` | `claude-sonnet-4-20250514` | Default Anthropic model |
| `OPENAI_MODEL` | `gpt-5.4` | Default OpenAI model |
| `LLM_SIDECAR_URL` | unset | Sidecar URL used by LLM integration tests |

### Auth Discovery Order

The sidecar looks for auth credentials in this order:

1. `PI_AI_AUTH_PATH`
2. `tools/llm-sidecar/auth.json`
3. repository-root `auth.json`
4. `~/.pi-ai/auth.json`
5. `~/auth.json`

### Benchmark and Test Usage

```bash
# Start the sidecar locally
make sidecar-install
make sidecar-start

# Run the benchmark against a non-default sidecar
go run ./cmd/bench-locomo --strategy llm --sidecar-url http://localhost:3100 --limit 1

# Run the live LLM integration test intentionally
export LLM_SIDECAR_URL=http://localhost:3100
go test ./tests/integration -run TestLLMAgent_ClassifyAndStoreMemories -v
```

## Example Configs

### Minimal

The absolute minimum to get Hydra running (all defaults apply):

```yaml
# configs/hydra.yaml -- minimal
hydra:
  name: hydra
```

### Development

Suitable for local development with verbose logging:

```yaml
hydra:
  name: hydra-dev
  version: "0.1.0"
  log_level: debug

gateway:
  host: localhost
  port: 8080
  read_timeout: 30
  write_timeout: 30

policy:
  default_budget: 100000
  rate_limit: 1000

logging:
  level: debug
  format: json
```

### Production

Hardened defaults for a production deployment:

```yaml
hydra:
  name: hydra-prod
  version: "0.1.0"
  log_level: info

gateway:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 10
  write_timeout: 30

policy:
  default_budget: 10000
  rate_limit: 100

logging:
  level: info
  format: json
```

With environment variable overrides for secrets and deployment-specific values:

```bash
export HYDRA_GATEWAY__PORT=80
export HYDRA_LOGGING__LEVEL=warn
export HYDRA_POLICY__DEFAULT_BUDGET=50000
```

## Validation

Hydra validates the config on startup. If any value is invalid, the process exits with an error message:

- `gateway.port` must be 1--65535
- `hydra.log_level` must be one of `debug`, `info`, `warn`, `error`
- `logging.level` must be one of `debug`, `info`, `warn`, `error`
- `logging.format` must be one of `json`, `text`, `console`
- `gateway.read_timeout` must be non-negative
- `gateway.write_timeout` must be non-negative
- `policy.default_budget` must be non-negative
- `policy.rate_limit` must be non-negative
