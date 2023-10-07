
.PHONY: setup-tools

setup-tools: golangci-lint gofumpt

GOINSTALL := CGO_ENABLED=0 GOBIN="$(CURDIR)/bin" go install

golangci-lint: go.mod
	@$(GOINSTALL) "github.com/golangci/golangci-lint/cmd/golangci-lint"

gofumpt: go.mod
	@$(GOINSTALL) "mvdan.cc/gofumpt"
