# agent-gogo

[![Go Reference](https://pkg.go.dev/badge/github.com/SukeyByte/agent-gogo.svg)](https://pkg.go.dev/github.com/SukeyByte/agent-gogo)
[![Go Report Card](https://goreportcard.com/badge/github.com/SukeyByte/agent-gogo)](https://goreportcard.com/report/github.com/SukeyByte/agent-gogo)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25-blue.svg)](go.mod)

An experimental Go Agent Runtime with a Web Console.

The goal is not to build another chat wrapper — it is to build a **recoverable, observable, cost-aware execution runtime** for long-running agent tasks.

## Architecture

```text
User / Channel
  -> Chain Router          (intent classification, complexity routing)
  -> Context Builder       (function/skill/persona/memory assembly)
  -> Planner / Validator   (task decomposition + capability check)
  -> Task DAG / Scheduler  (dependency-aware task ordering)
  -> Executor / Tool Runtime (LLM-driven tool actions)
  -> Observer              (tool result interpretation)
  -> Tester                (acceptance criteria verification)
  -> Reviewer              (quality gate)
  -> Task Awareness / Memory (cross-task learning, project digest)
  -> State Store           (SQLite + artifacts)
```

The runtime treats tasks as **durable state**, not as temporary chat messages. Tool calls, observations, tests, reviews, and repair decisions are recorded as events so projects can be resumed and audited.

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 20+ (for Web Console frontend)
- An LLM API key (e.g. DeepSeek)

### Install & Run

```bash
# Clone
git clone https://github.com/SukeyByte/agent-gogo.git
cd agent-gogo

# Configure
cp configs/config.example.yaml configs/config.yaml
cp .env.example .env
# Edit .env to set your API key

# Run web console
make web
# Open http://127.0.0.1:8080

# Or run as CLI agent
make run
```

### CLI Usage

```bash
# Set your API key
export DEEPSEEK_API_KEY="..."

# Run with a natural-language goal
go run ./cmd/agent-gogo "help me build a REST API in Go"

# Enable shell access for engineering tasks
AGENT_GOGO_ALLOW_SHELL=true go run ./cmd/agent-gogo "refactor the auth module and run tests"
```

## Repository Layout

```text
cmd/agent-gogo/          CLI entrypoint
internal/app/            Application bootstrap
internal/domain/         Core domain models (Project, Task, Session, Event)
internal/runtime/        Runtime service (orchestration, context, events)
internal/planner/        LLM-based task planner
internal/executor/       Task executor (generic + specialized)
internal/validator/      Capability validation
internal/scheduler/      Dependency-aware task scheduler
internal/tester/         Acceptance criteria verification
internal/reviewer/       Quality review gate
internal/observer/       Tool result interpretation
internal/tools/          Tool runtime (code, file, git, shell, browser, memory)
internal/contextbuilder/ ContextPack assembly and serialization
internal/chain/          Intent classification and routing
internal/intent/         Intent analysis
internal/function/       Function/tool registry
internal/skill/          Skill registry and discovery
internal/persona/        Persona registry and discovery
internal/memory/         Memory index and search
internal/taskaware/      Project digest and task-aware context
internal/session/        Session lifecycle management
internal/browser/        Browser automation runtime
internal/capability/     Capability registry and checks
internal/codeindex/      Repository map and symbol index
internal/communication/  Channel-agnostic communication layer
internal/channels/       Concrete channel adapters and console APIs
internal/config/         Configuration loading
internal/provider/       LLM provider abstraction
internal/observability/  Structured logging (JSONL)
internal/store/          SQLite persistence layer
internal/prompts/        Default prompt templates
web/frontend/            Vue 3 + Vite web console
configs/                 Example configuration
migrations/              SQLite schema migrations
docs/                    Design documents and milestone results
```

## Development

```bash
make help          # Show all available commands
make build         # Build the binary
make test          # Run tests
make test-cover    # Run tests with coverage report
make lint          # Run linter
make check         # Run fmt + vet + test
make frontend      # Build frontend
make dev           # Run web console without building
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full contribution guide.

## Documentation

- [PRD (Product Requirements)](docs/prd/go_agent_runtime_prd.md)
- [Architecture Design](docs/design/architecture.md)
- [Core Principles](docs/design/core_principles.md)
- [Milestones & Roadmap](docs/roadmap/milestones.md)

## Community

- [Code of Conduct](CODE_OF_CONDUCT.md)
- [Contributing Guide](CONTRIBUTING.md)
- [Security Policy](SECURITY.md)

## License

This project is licensed under the [MIT License](LICENSE).
