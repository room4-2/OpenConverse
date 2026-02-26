//go:build tools

// This file tracks development tool dependencies so they are pinned in go.sum.
// Install all tools with: make install-tools
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
)
