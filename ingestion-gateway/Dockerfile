# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=1.0.0" \
    -o /ingestion-gateway \
    ./cmd/gateway

# Final stage - minimal image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack

WORKDIR /app

# Copy binary
COPY --from=builder /ingestion-gateway /app/ingestion-gateway

# Copy default config
COPY config/config.yaml /etc/ollystack/config.yaml

# Switch to non-root user
USER ollystack

# Ports
EXPOSE 4317 4318 8888

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:4318/health || exit 1

ENTRYPOINT ["/app/ingestion-gateway"]
CMD ["--config", "/etc/ollystack/config.yaml"]
