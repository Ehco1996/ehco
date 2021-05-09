NAME=ehco
BINDIR=.dist
VERSION=$(shell git describe --tags || echo "unknown version")
BUILDTIME=$(shell date -u)
GOBUILD=CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X="github.com/Ehco1996/ehco/internal/constant/constant.Version=$(VERSION)"'

.PHONY: fmt test build tidy ensure release

fmt:
	golangci-lint run --fix

test:
	go test -count=1  -coverpkg=./internal -timeout=10s ./...

build:
	${GOBUILD} -o $(BINDIR)/$(NAME) cmd/ehco/main.go

tidy:
	cat go.mod | grep -v ' indirect' > direct.mod
	mv direct.mod go.mod
	rm go.sum || true
	go mod tidy

ensure: tidy
	go mod download


release:
	goreleaser build --skip-validate --rm-dist
