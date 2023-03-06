FROM golang:1.19 as build-base

WORKDIR /app

COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# RUN GOOS=linux make build
FROM build-base AS builder
WORKDIR /app
RUN --mount=type=bind,target=.,rw \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    mkdir dist && make build

RUN pwd && ls -alh

FROM alpine:latest
RUN apk --no-cache add ca-certificates
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /bin/ehco

ENTRYPOINT ["/bin/ehco"]
