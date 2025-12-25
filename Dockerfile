# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o canopy-rpc-mock .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /build/canopy-rpc-mock .

# Expose port range for multiple chains
# Default is 60000-60009 (supports up to 10 chains)
EXPOSE 60000-60009

# Default command with configurable flags via environment variables
ENTRYPOINT ["/app/canopy-rpc-mock"]

# Default flags (can be overridden)
CMD ["-chains", "2", "-blocks", "25", "-start-port", "60000", "-start-chain-id", "5"]