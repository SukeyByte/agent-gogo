.PHONY: build run test lint clean frontend dev help

BINARY  := bin/agent-gogo
CMD     := ./cmd/agent-gogo
GO      := go
GOFLAGS := -v

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'

build: ## Build the binary
	$(GO) build $(GOFLAGS) -o $(BINARY) $(CMD)

run: build ## Build and run the CLI agent
	./$(BINARY)

web: build ## Build and run the web console
	./$(BINARY) web --addr 127.0.0.1:8080

dev: ## Run web console in dev mode (no build)
	$(GO) run $(CMD) web --addr 127.0.0.1:8080

test: ## Run Go tests
	$(GO) test ./...

test-cover: ## Run tests with coverage report
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

lint: ## Run golangci-lint (install first: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

frontend: ## Install frontend deps and build
	cd web/frontend && npm install && npm run build

frontend-dev: ## Run frontend dev server
	cd web/frontend && npm run dev

clean: ## Remove build artifacts
	rm -rf bin/ dist/ coverage.out coverage.html

tidy: ## Tidy Go modules
	$(GO) mod tidy

fmt: ## Format Go code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

check: fmt vet test ## Run all checks (fmt + vet + test)
