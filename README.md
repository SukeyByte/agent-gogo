# agent-gogo

agent-gogo is an experimental Go Agent Runtime with a Web Console.

The goal is not to build another chat wrapper. The goal is to build a recoverable, observable, cost-aware execution runtime for long-running agent tasks.

## Status

This project is now a runnable runtime slice. The repository contains the PRD, architecture notes, durable SQLite task state, ContextBuilder, Chain Router, planner/validator/scheduler loop, browser and story executors, Tool Runtime, Communication Runtime, and JSONL observability logs.

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
internal/demo/         Demo-only executors and examples
internal/executor/     Generic executors such as BrowserExecutor and StoryExecutor
.claude/skills/        Pulled Claude-compatible story skills indexed locally
migrations/            SQLite migrations
logs/                  Runtime JSONL logs, created locally
data/artifacts/        Tool-created documents and memory files, created locally
```

## Development

```bash
go test ./...
go run ./cmd/agent-gogo
```

## CLI Flows

```bash
export DEEPSEEK_API_KEY="..."
go run ./cmd/agent-gogo plan "目标"
go run ./cmd/agent-gogo answer-url https://example.com "问题"
go run ./cmd/agent-gogo write-story "我希望完成一个短篇推理小说的编写"
```

`write-story` uses DeepSeek through the provider interface, generates an ephemeral novelist persona at runtime, searches the local Claude-compatible `SKILL.md` index from `storage.skill_roots`, builds a ContextPack, writes the story via `document.write`, saves key points via `memory.save`, sends the final text to the configured channel, and writes chain/prompt/tool logs to `storage.log_path`.

The repo includes two pulled upstream skills for the story acceptance flow: `chapter-writing` for prose generation and `plot-structure` for arc, timeline, and foreshadowing work.

API keys are read from environment variables such as `DEEPSEEK_API_KEY` or `AGENT_GOGO_LLM_API_KEY`; do not commit real keys to config files.

## License

TBD before the first public release.
