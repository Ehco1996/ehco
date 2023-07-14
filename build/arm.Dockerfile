FROM golang:1.20 as builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Build the Go app
COPY . .
RUN --mount=type=cache,target=/home/runner/go/pkg/mod \
    --mount=type=cache,target=/home/runner/.cache/go-build \
    make build

RUN GOOS=linux GOARCH=arm make build

FROM debian:buster-slim
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /ehco
ENTRYPOINT ["/ehco"]
