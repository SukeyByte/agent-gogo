# agent-gogo

agent-gogo is an experimental Go Agent Runtime with a Web Console.

The goal is not to build another chat wrapper. The goal is to build a recoverable, observable, cost-aware execution runtime for long-running agent tasks.

## Status

This project is now a runnable runtime slice. The repository contains the PRD, architecture notes, durable SQLite task state, ContextBuilder, Chain Router, planner/validator/scheduler loop, GenericExecutor action loop, Capability Resolver, Observer / State Interpreter, Tool Runtime, Communication Runtime, Code Runtime, task awareness, persistent project memory, basic Web Console pages, real file/shell/test/Git engineering tools, and JSONL observability logs.

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
  -> Task Awareness / Project Memory
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
internal/capability/   Capability registry, availability checks, and tool mapping
internal/executor/     GenericExecutor plus specialized executor plugins
internal/observer/     Tool-result state interpretation and observations
internal/codeindex/    Lightweight repo map and symbol index
internal/taskaware/    Project digest, task awareness, and automatic memory extraction
.claude/skills/        Pulled Claude-compatible story skills indexed locally
migrations/            SQLite migrations
logs/                  Runtime JSONL logs, created locally
data/artifacts/        Tool-created documents and memory files, created locally
web/static/            Static source assets created by engineering workflows
web/dist/              Local deployment output, created locally
web/handlers/          Web Console HTTP handlers
web/templates/         Web Console template home
```

## Development

```bash
go test ./...
go run ./cmd/agent-gogo
go run ./cmd/agent-gogo web --addr 127.0.0.1:8080
```

## CLI Flows

```bash
export DEEPSEEK_API_KEY="..."
go run ./cmd/agent-gogo "我希望完成一个短篇推理小说的编写"
AGENT_GOGO_ALLOW_SHELL=true go run ./cmd/agent-gogo "为苏柯宇写一个个人网页并完成部署"
go run ./cmd/agent-gogo "打开 https://example.com 并回答页面里的问题"
```

The CLI accepts a natural-language goal directly. Runtime plans a task DAG, builds a ContextPack, lets GenericExecutor ask the LLM for structured tool actions, runs those actions through Tool Runtime, lets Observer interpret tool results, and then sends each implemented task through Tester and Reviewer. There is no keyword demo router in the main path.

The repo includes two pulled upstream skills for the story acceptance flow: `chapter-writing` for prose generation and `plot-structure` for arc, timeline, and foreshadowing work.

API keys are read from environment variables such as `DEEPSEEK_API_KEY` or `AGENT_GOGO_LLM_API_KEY`; do not commit real keys to config files.

Engineering work uses capability-resolved tools such as `code.index`, `file.read`, `file.write`, `file.patch`, `git.status`, `git.diff`, and optionally `test.run`. Shell execution is disabled by default; set `AGENT_GOGO_ALLOW_SHELL=true` or enable it in config, and keep commands within `security.shell_allowlist`.

W9 adds task awareness for generic agents. Before planning or executing a task, Runtime builds a deterministic Project Digest from tasks, DAG dependencies, events, observations, tests, reviews, tool evidence, and artifacts, then injects it into ContextBuilder L2. Completed or failed tasks automatically produce traceable project memories, and the next task retrieves those memories into `RelevantMemories`.

The Web Console starts with `agent-gogo web --addr 127.0.0.1:8080` and currently exposes Dashboard, Chat, Projects, Project Detail, Task Detail, Browser View, Skill, Persona, Memory, File, and Config pages backed by the SQLite runtime store.

## License

TBD before the first public release.
