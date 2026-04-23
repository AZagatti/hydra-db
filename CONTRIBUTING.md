# Contributing to Hydra

Thank you for your interest in contributing to Hydra. This document covers everything you need to get started.

## Prerequisites

- Go 1.26+
- [golangci-lint v2](https://golangci-lint.run/) (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)
- [lefthook](https://github.com/evilmartians/lefthook) (`go install github.com/evilmartians/lefthook@latest`)
- [goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) (`go install golang.org/x/tools/cmd/goimports@latest`)

## Quick Setup

```bash
git clone https://github.com/AZagatti/hydra-db.git
cd hydra-db
make install    # installs tools and git hooks
make test       # run all tests
make lint       # run linter
```

## Development Workflow

1. Create a branch from `main`
2. Make your changes
3. Ensure all tests pass: `make test`
4. Ensure lint is clean: `make lint`
5. Commit with conventional commit format (enforced by git hooks)
6. Push and open a pull request

## Commit Convention

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): description

feat: add new head
fix(gateway): handle nil payload
docs: update architecture guide
test(agent): add executor retry tests
refactor(memory): rename provider interface
chore: update dependencies
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `chore`, `build`, `ci`

## Project Structure

See [docs/architecture.md](docs/architecture.md) for the full architecture guide.

```
cmd/hydra/          # Entrypoint
internal/body/      # Shared types (Envelope, Identity, Trace, EventBus)
internal/gateway/   # API Gateway head
internal/agent/     # Agent Runtime head
internal/execution/ # Execution Plane head
internal/memory/    # Memory Plane head
internal/policy/    # Policy/Guardrails head
internal/adapter/   # Protocol adapters (HTTP, CLI, Slack)
tests/integration/  # End-to-end tests
```

## Code Standards

- **TDD**: Write tests first for all business logic
- **Godoc**: Every exported symbol must have a doc comment
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)`
- **Context propagation**: Every I/O function accepts `context.Context`
- **No global state**: Pass dependencies explicitly
- **No init()**: Use explicit constructors
- **Table-driven tests**: For input/output variations

## Running Tests

```bash
make test          # All tests with race detector
make cover         # Generate HTML coverage report
go test -v ./internal/body/...   # Specific package
```

## Opening a Pull Request

- Keep PRs focused on a single concern
- Include tests for new functionality
- Update documentation if needed
- Follow the PR template

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
