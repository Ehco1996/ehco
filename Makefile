.PHONY: tools lint fmt test build tidy release

NAME=ehco
BINDIR=dist

PACKAGE=github.com/Ehco1996/ehco/internal/constant
BUILDTIME=$(shell date +"%Y-%m-%d-%T")
BRANCH=$(shell git rev-parse --abbrev-ref HEAD | tr -d '\040\011\012\015\n')
REVISION=$(shell git rev-parse HEAD)


PACKAGE_LIST  := go list ./...
FILES         := $(shell find . -name "*.go" -type f)
FAIL_ON_STDOUT := awk '{ print } END { if (NR > 0) { exit 1 } }'


# -w -s 参数的解释：You will get the smallest binaries if you compile with -ldflags '-w -s'. The -w turns off DWARF debugging information
# for more information, please refer to https://stackoverflow.com/questions/22267189/what-does-the-w-flag-mean-when-passed-in-via-the-ldflags-option-to-the-go-comman
# we need CGO_ENABLED=1 because we import the node_exporter ,and we need install `glibc-source,libc6` to make it work
GOBUILD=CGO_ENABLED=1 go build -trimpath -ldflags="-w -s -X ${PACKAGE}.GitBranch=${BRANCH} -X ${PACKAGE}.GitRevision=${REVISION} -X ${PACKAGE}.BuildTime=${BUILDTIME}"

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
	go test -v -count=1  -coverpkg=./internal -timeout=10s ./...

build:
	${GOBUILD} -o $(BINDIR)/$(NAME) cmd/ehco/main.go

build-arm:
	GOARCH=arm GOOS=linux ${GOBUILD} -o $(BINDIR)/$(NAME) cmd/ehco/main.go

build-nightly:
	${GOBUILD} -o $(NAME)  cmd/ehco/main.go

build-linux-amd64:
	GOARCH=amd64 GOOS=linux ${GOBUILD} -o $(BINDIR)/$(NAME)_amd64 cmd/ehco/main.go

tidy:
	go mod tidy

release:
	goreleaser build --skip-validate --rm-dist

