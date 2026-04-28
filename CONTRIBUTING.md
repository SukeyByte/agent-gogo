# Contributing

agent-gogo is still early. Contributions are most useful when they help clarify the runtime architecture, reduce scope risk, or make the first execution loop easier to verify.

## Development Loop

```bash
go test ./...
go run ./cmd/agent-gogo
```

## Design Guidelines

1. Keep Runtime Core independent from Web Console.
2. Persist project and task state instead of relying on chat history.
3. Route all external side effects through Tool Runtime.
4. Prefer small, verifiable execution loops over broad abstractions.
5. Add interfaces only when they protect a real boundary.

## Documentation

When changing architecture, update the relevant file under `docs/design/`.

When changing product scope, update the PRD under `docs/prd/`.
