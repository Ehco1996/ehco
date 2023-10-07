//go:build tools
// +build tools

package tools

// pin for go.mod
import (
	_ "github.com/envoyproxy/protoc-gen-validate"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/golangci/golangci-lint/pkg/commands"
	_ "mvdan.cc/gofumpt"
)
