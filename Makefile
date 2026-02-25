.PHONY: help build run test lint fmt vet tidy install-tools pre-commit-install clean \
        docker-build docker-up docker-down docker-logs docker-clean

BINARY := openconverse
GO     := go
LINT   := golangci-lint

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'


build:
	$(GO) build -o $(BINARY) ./...

run:
	$(GO) run .


test:
	$(GO) test -race -coverprofile=coverage.out ./...

test-verbose:
	$(GO) test -v -race ./...

lint:
	$(LINT) run ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

tidy:
	$(GO) mod tidy
	$(GO) mod verify

check: fmt vet lint test


install-tools:
	@echo "Installing golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin
	@echo "Installing pre-commit..."
	@pip install pre-commit --break-system-packages
	@echo "Done. Run 'make pre-commit-install' to activate git hooks."

pre-commit-install:
	pre-commit install


clean:
	rm -f $(BINARY) coverage.out


docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f

docker-clean:
	docker compose down -v --rmi local
