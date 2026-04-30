# Contributing to agent-gogo

Thanks for your interest in contributing! This guide covers the development setup, coding conventions, and PR process.

## Development Setup

```bash
# Prerequisites: Go 1.25+, Node.js 20+

# Clone and configure
git clone https://github.com/sukeke/agent-gogo.git
cd agent-gogo
cp .env.example .env

# Build and test
make build
make test

# Run web console locally
make dev
```

See `make help` for all available commands.

## Design Guidelines

1. Keep Runtime Core independent from Web Console.
2. Persist project and task state instead of relying on chat history.
3. Route all external side effects through Tool Runtime.
4. Prefer small, verifiable execution loops over broad abstractions.
5. Add interfaces only when they protect a real boundary.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Run `make fmt` and `make vet` before submitting.
- If golangci-lint is installed, run `make lint`.
- Keep functions focused; avoid god methods.
- Use table-driven tests for multi-case scenarios.

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`

Examples from this repo:
- `feat(tester): Add generic evidence tester with tool result verification`
- `fix(scheduler): Resolve deadlock on circular dependencies`
- `docs: Add M10.4 WebConsole channel integration result`

## Pull Request Process

1. Fork the repository and create a feature branch from `main`.
2. Make your changes with clear, atomic commits.
3. Ensure `make check` passes (fmt + vet + test).
4. Update relevant documentation:
   - Architecture changes -> `docs/design/architecture.md`
   - Scope changes -> `docs/prd/go_agent_runtime_prd.md`
   - New features -> update README.md if user-facing
5. Open a PR against `main` with a clear description of the problem and solution.

## Testing

- All new code should have corresponding tests.
- Run `make test` to execute the full test suite.
- Use `make test-cover` to check coverage.
- Integration tests that require external services should be skipped when the service is unavailable (use `t.Skip()` with a clear reason).

## Reporting Issues

- Use [GitHub Issues](https://github.com/sukeke/agent-gogo/issues) for bug reports and feature requests.
- See the issue templates for guidance.
- For security vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of filing a public issue.

## Questions?

Feel free to open an issue with the `question` label, or reach out via the contact information in [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
