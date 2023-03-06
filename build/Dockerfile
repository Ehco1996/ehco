FROM golang:1.19 as build-base

WORKDIR /app

COPY go.* .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .

# RUN GOOS=linux make build
FROM build-base AS builder
RUN --mount=type=bind,target=.,rw \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /bin/ehco

ENTRYPOINT ["/bin/ehco"]
