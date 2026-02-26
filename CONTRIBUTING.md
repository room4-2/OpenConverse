# Contributing to OpenConverse

Thanks for taking the time to contribute. This document covers everything you need to get started.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Setup](#development-setup)
- [Making Changes](#making-changes)
- [Code Style](#code-style)
- [Testing](#testing)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Reporting Issues](#reporting-issues)

---

## Getting Started

Before opening a pull request for a significant change (new feature, refactor, API change), please **open an issue first** to discuss the approach. This avoids duplicated effort and keeps the project coherent.

Small fixes (typos, obvious bugs, documentation) can go straight to a PR.

## Development Setup

### Prerequisites

- Go 1.24.3+
- A [Google AI Studio](https://aistudio.google.com/) API key for Gemini
- `pip` (for pre-commit)
- Docker + Docker Compose (optional, for container testing)

### Clone and configure

```bash
git clone https://github.com/room4-2/OpenConverse.git
cd OpenConverse

cp .env.example .env
# Edit .env and set GEMINI_API_KEY
```

### Install development tools

```bash
make install-tools       # installs golangci-lint and pre-commit
make pre-commit-install  # activates git hooks
```

This installs:
- `golangci-lint`  static analysis and style enforcement
- `pre-commit`  runs checks automatically before every commit

### Run the server

```bash
make run
```

---

## Making Changes

1. Fork the repository and create a feature branch off `main`:

   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes. Keep commits focused — one logical change per commit.

3. Before committing, run the full check suite:

   ```bash
   make check   # fmt + vet + lint + test
   ```

4. Push your branch and open a pull request against `main`.

### Branch naming

| Type | Pattern | Example |
|---|---|---|
| Feature | `feature/<name>` | `feature/twilio-recording` |
| Bug fix | `fix/<name>` | `fix/session-leak` |
| Docs | `docs/<name>` | `docs/contributing` |
| Refactor | `refactor/<name>` | `refactor/gemini-proxy` |

---

## Code Style

All Go code must pass the linters configured in `.golangci.yml`. The key rules:

- **Formatting**: `gofmt` and `goimports`  run `make fmt` before committing
- **Error handling**: always check returned errors (`errcheck`)
- **Exported symbols**: must have doc comments (`revive/exported`)
- **Context**: HTTP requests must carry a context (`noctx`, `contextcheck`)
- **HTTP responses**: response bodies must be closed (`bodyclose`)

Run linting manually:

```bash
make lint    # golangci-lint
make vet     # go vet
make fmt     # gofmt + goimports
```

The pre-commit hooks run `go-fmt`, `go-vet`, and `golangci-lint` automatically on staged files.

---

## Testing

```bash
make test          # run all tests with race detector and coverage
make test-verbose  # same, with verbose output
```

- Write tests for any new behaviour you introduce.
- Existing tests must continue to pass.
- The race detector (`-race`) is always enabled — avoid data races.

---

## Submitting a Pull Request

1. Ensure `make check` passes cleanly.
2. Update the README if your change affects configuration, endpoints, or the audio pipeline.
3. Open the PR against `main` with a clear title and description:
   - **What** changed and **why**
   - Link to any related issue (`Closes #123`)
4. A maintainer will review the PR. Expect feedback and be ready to iterate.

---

## Reporting Issues

Use [GitHub Issues](https://github.com/room4-2/OpenConverse/issues). Please include:

- Go version (`go version`)
- OpenConverse version or commit hash
- Steps to reproduce
- Expected vs. actual behaviour
- Relevant logs or error output
