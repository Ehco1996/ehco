FROM golang:1.19 as builder
WORKDIR /app
COPY . .

RUN --mount=type=cache,target=/root/go/pkg/mod \
    make build

RUN ./dist/ehco -v

FROM alpine:latest
RUN apk --no-cache add ca-certificates
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /ehco
CMD ["/ehco"]
