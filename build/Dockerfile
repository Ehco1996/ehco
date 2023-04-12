FROM golang:1.19 as builder
WORKDIR /app
COPY . .

RUN --mount=type=cache,target=/home/runner/go/pkg/mod \
    --mount=type=cache,target=/home/runner/.cache/go-build \
    make build

FROM debian:buster-slim
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /ehco
ENTRYPOINT ["/ehco"]
