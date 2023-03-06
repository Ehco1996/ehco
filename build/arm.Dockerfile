FROM golang:1.19 as builder

# Set Environment Variables
ENV HOME /app
ENV CGO_ENABLED 0
ENV GOOS linux
ENV GOARCH=arm
ENV GOROOT_BOOTSTRAP=/usr/local/go

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Build the Go app
RUN make build

FROM multiarch/alpine:armhf-edge

WORKDIR /bin/

# Copy the pre-built binary file from the previous stage
COPY --from=builder /app/dist/ehco .

ENTRYPOINT ["/bin/ehco"]
