# OllyStack API Server - Multi-stage Dockerfile
# Supports both AMD64 and ARM64 (Graviton)

# =============================================================================
# Stage 1: Build
# =============================================================================
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy source code and download dependencies
COPY go.mod ./
COPY . .
RUN go mod tidy && go mod download

# Build binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w \
    -X main.Version=${VERSION} \
    -X main.Commit=${COMMIT} \
    -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /api-server ./cmd/server

# =============================================================================
# Stage 2: Runtime
# =============================================================================
FROM alpine:3.19

ARG VERSION=dev

LABEL org.opencontainers.image.title="OllyStack API Server"
LABEL org.opencontainers.image.description="OllyStack observability platform API server"
LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.source="https://github.com/ollystack/ollystack"

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata wget

# Create non-root user
RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack

WORKDIR /app

# Copy binary from builder
COPY --from=builder /api-server /app/api-server

# Copy any config files
COPY --from=builder /app/configs ./configs

# Set ownership
RUN chown -R ollystack:ollystack /app

# Switch to non-root user
USER ollystack

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://localhost:8080/health || exit 1

# Run
ENTRYPOINT ["/app/api-server"]
CMD []
