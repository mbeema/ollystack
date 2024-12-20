# OllyStack Unified Agent
# Multi-stage build for minimal image size

# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /ollystack-agent \
    ./cmd/agent

# Final stage - minimal image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack

# Create directories
RUN mkdir -p /etc/ollystack /var/lib/ollystack-agent/buffer && \
    chown -R ollystack:ollystack /var/lib/ollystack-agent

# Copy binary
COPY --from=builder /ollystack-agent /usr/local/bin/ollystack-agent

# Copy default config
COPY config/agent.yaml /etc/ollystack/agent.yaml

# Set user
USER ollystack

# Expose ports
# 4317 - OTLP gRPC
# 4318 - OTLP HTTP
# 8888 - Health/metrics
EXPOSE 4317 4318 8888

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8888/health || exit 1

ENTRYPOINT ["/usr/local/bin/ollystack-agent"]
CMD ["-c", "/etc/ollystack/agent.yaml"]
