# agent-gogo

agent-gogo is an experimental Go Agent Runtime with a Web Console.

The goal is not to build another chat wrapper. The goal is to build a recoverable, observable, cost-aware execution runtime for long-running agent tasks.

## Status

This project is in the design and scaffold stage. The current repository contains the product requirements, core design principles, and a minimal Go module that builds.

## Core Idea

```text
User / Channel
  -> Chain Router
  -> Context Builder
  -> Planner / Validator
  -> Task DAG / Scheduler
  -> Executor / Tool Runtime
  -> Observer
  -> Tester
  -> Reviewer
  -> State Store / Artifact Store
```

The runtime treats tasks as durable state, not as temporary chat messages. Tool calls, observations, tests, reviews, and repair decisions should be recorded as events so projects can be resumed and audited.

## Design Documents

- [PRD](docs/prd/go_agent_runtime_prd.md)
- [Core Design Principles](docs/design/core_principles.md)
- [Architecture Draft](docs/design/architecture.md)

## Repository Layout

```text
cmd/agent-gogo/        CLI entrypoint
internal/app/          Application bootstrap surface
docs/prd/              Product requirements
docs/design/           Architecture and design notes
configs/               Example configuration
skills/                Built-in or local skill definitions
personas/              Built-in or local persona definitions
migrations/            SQLite migrations
web/                   Future Web Console
```

## Development

```bash
go test ./...
go run ./cmd/agent-gogo
```

## License

TBD before the first public release.
