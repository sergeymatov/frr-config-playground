FROM golang:1.24 AS builder

# Set workdir
WORKDIR /app

# Copy source files
COPY . .

# Build the Go binary
RUN go mod tidy && go build -o /app/frr-config config.go

# Runtime stage
FROM ubuntu:latest

# Install FRR and required tools
RUN apt-get update && apt-get install -y frr frr-pythontools bash

# Copy binary from build stage
COPY --from=builder /app/frr-config /usr/local/bin/frr-config

# Run frr-config on startup
CMD ["/usr/local/bin/frr-config"]

