# Build stage
FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /realtime-processor \
    ./cmd/processor

# Final stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -g 1000 ollystack && \
    adduser -u 1000 -G ollystack -s /bin/sh -D ollystack

WORKDIR /app

COPY --from=builder /realtime-processor /app/realtime-processor
COPY config/config.yaml /etc/ollystack/config.yaml

USER ollystack

EXPOSE 8890

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8890/health || exit 1

ENTRYPOINT ["/app/realtime-processor"]
CMD ["--config", "/etc/ollystack/config.yaml"]
