FROM golang:1.20 as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Build the Go app
COPY . .
RUN --mount=type=cache,target=/home/runner/go/pkg/mod \
    --mount=type=cache,target=/home/runner/.cache/go-build \
    build-arm

FROM debian:12-slim

RUN apt update && apt install -y --no-install-recommends ca-certificates curl glibc-source libc6

WORKDIR /bin/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco .

ENTRYPOINT ["ehco"]
