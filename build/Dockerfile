FROM golang:1.19 as builder
WORKDIR /app
COPY . .

RUN --mount=type=cache,target=/root/go/pkg/mod \
    make build

FROM debian:buster-slim
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /ehco
CMD ["/ehco"]
