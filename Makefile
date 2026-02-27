.PHONY: help build run test test-verbose lint fmt vet tidy check install-tools pre-commit-install clean \
        docker-build docker-up docker-down docker-logs docker-clean

BINARY  := openconverse
GO      := go
LINT    := golangci-lint
COMPOSE := docker compose -f compose.dev.yml

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'


build: ## Build the binary
	$(GO) build -o $(BINARY) ./...

run: ## Run the application
	$(GO) run .

test: ## Run tests with race detector and coverage
	$(GO) test -race -coverprofile=coverage.out ./...

test-verbose: ## Run tests with verbose output
	$(GO) test -v -race ./...

lint: ## Run golangci-lint
	$(LINT) run ./...

fmt: ## Format Go source files
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy and verify go modules
	$(GO) mod tidy
	$(GO) mod verify

check: fmt vet lint test ## Run fmt, vet, lint and tests

install-tools: ## Install golangci-lint and pre-commit
	@echo "Installing golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin
	@echo "Installing pre-commit..."
	@pip install pre-commit --break-system-packages
	@echo "Done. Run 'make pre-commit-install' to activate git hooks."

pre-commit-install: ## Install pre-commit git hooks
	pre-commit install

clean: ## Remove binary and coverage output
	rm -f $(BINARY) coverage.out

docker-build: ## Build Docker images
	$(COMPOSE) build

docker-up: ## Start containers in detached mode
	$(COMPOSE) up -d

docker-down: ## Stop containers
	$(COMPOSE) down

docker-logs: ## Tail container logs
	$(COMPOSE) logs -f

docker-clean: ## Stop containers and remove volumes and images
	$(COMPOSE) down -v --rmi local
