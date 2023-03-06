FROM golang:1.19 as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Build the Go app
COPY . .
# RUN GOOS=linux make build
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    make build

FROM alpine:latest
RUN apk --no-cache add ca-certificates
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /bin/ehco

ENTRYPOINT ["/bin/ehco"]
