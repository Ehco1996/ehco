.PHONY: tools lint fmt test test-e2e build tidy release

NAME=ehco
BINDIR=dist

PACKAGE=github.com/Ehco1996/ehco/internal/constant
BUILDTIME=$(shell date +"%Y-%m-%d-%T")
BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr -d '\040\011\012\015\n')
REVISION=$(shell git rev-parse HEAD)

# Pin VERSION to the most recent nightly tag so `make build` (and Docker, etc.)
# self-reports the current in-progress release line, e.g. `1.1.7-next` while
# v1.1.7 is being prepared. goreleaser still injects its own GORELEASER_CURRENT_TAG
# for actual release artifacts; this only kicks in for non-goreleaser builds.
#
# Resolution order:
#   1. nearest reachable nightly tag matching v*-next
#   2. nearest reachable stable tag (rare: only between a release and the next nightly cron)
#   3. empty -> falls back to the constant.Version source default
GIT_DESCRIBE_VERSION := $(shell git describe --tags --abbrev=0 --match 'v*-next' 2>/dev/null \
	|| git describe --tags --abbrev=0 --match 'v*' --exclude='*-*' 2>/dev/null)
VERSION := $(patsubst v%,%,$(GIT_DESCRIBE_VERSION))

ifeq ($(VERSION),)
VERSION_LDFLAG :=
else
VERSION_LDFLAG := -X $(PACKAGE).Version=$(VERSION)
endif


PACKAGE_LIST  := go list ./...
FILES         := $(shell find . -name "*.go" -type f)
FAIL_ON_STDOUT := awk '{ print } END { if (NR > 0) { exit 1 } }'

# Detect OS and set appropriate build tags for node_exporter
# These tags are only needed for Linux, macOS uses different collectors
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	BUILD_TAG_FOR_NODE_EXPORTER="nofibrechannel,nomountstats"
else
	BUILD_TAG_FOR_NODE_EXPORTER=""
endif

# -w -s 参数的解释：You will get the smallest binaries if you compile with -ldflags '-w -s'. The -w turns off DWARF debugging information
# for more information, please refer to https://stackoverflow.com/questions/22267189/what-does-the-w-flag-mean-when-passed-in-via-the-ldflags-option-to-the-go-comman
GOBUILD=CGO_ENABLED=0 go build -tags ${BUILD_TAG_FOR_NODE_EXPORTER} -trimpath -ldflags="-w -s ${VERSION_LDFLAG} -X ${PACKAGE}.GitBranch=${BRANCH} -X ${PACKAGE}.GitRevision=${REVISION} -X ${PACKAGE}.BuildTime=${BUILDTIME}"


tools:
	@echo "run setup tools"
	make -C tools setup-tools

lint: tools
	@echo "run lint"
	tools/bin/golangci-lint run

fmt: tools
	@echo "golangci-lint run --fix"
	@tools/bin/golangci-lint run --fix
	@echo "gofmt (simplify)"
	@tools/bin/gofumpt -l -w $(FILES) 2>&1 | $(FAIL_ON_STDOUT)

test:
	go test -tags ${BUILD_TAG_FOR_NODE_EXPORTER} -v -count=1 -timeout=3m ./...

# Just the pkg/xray e2e suite — runs three protocols × {tcp, udp where supported}
# plus vless+REALITY against a self-spun client xray + echo backend. Uses real
# sockets; takes ~15s end to end.
test-e2e:
	go test -tags ${BUILD_TAG_FOR_NODE_EXPORTER} -v -count=1 -timeout=3m -run TestE2E ./pkg/xray/...

build:
	${GOBUILD} -o $(BINDIR)/$(NAME) cmd/ehco/main.go

build-arm:
	GOARCH=arm GOOS=linux ${GOBUILD} -o $(BINDIR)/$(NAME) cmd/ehco/main.go

build-linux-amd64:
	GOARCH=amd64 GOOS=linux ${GOBUILD} -o $(BINDIR)/$(NAME)_amd64 cmd/ehco/main.go

tidy:
	go mod tidy
