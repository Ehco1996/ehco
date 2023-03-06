FROM golang:1.19 as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

# Build the Go app
COPY . .
RUN GOOS=linux GOARCH=arm make build

FROM multiarch/alpine:armhf-edge
# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco /bin/ehco
ENTRYPOINT ["/bin/ehco"]
