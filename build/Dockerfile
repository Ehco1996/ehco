FROM golang:1.13 as builder

# Set Environment Variables
ENV HOME /app
ENV CGO_ENABLED 0
ENV GOOS linux

WORKDIR /app
COPY ../go.mod go.sum ./
RUN go mod download
COPY .. .

# Build the Go app
RUN go build -v -a -installsuffix cgo -o ehco cli/main.go

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /bin/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/ehco .

ENTRYPOINT ["/bin/bin"]