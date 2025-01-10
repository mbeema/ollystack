# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o /ollystack-agent \
    ./cmd/agent

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack

# Copy binary
COPY --from=builder /ollystack-agent /usr/local/bin/ollystack-agent

# Copy default config
COPY config/agent.yaml /etc/ollystack/agent.yaml

# Set ownership
RUN chown -R ollystack:ollystack /etc/ollystack

USER ollystack

EXPOSE 8888

ENTRYPOINT ["/usr/local/bin/ollystack-agent"]
CMD ["--config", "/etc/ollystack/agent.yaml"]
