# Build stage - uses OTel Collector Builder (ocb)
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Install OTel Collector Builder
RUN go install go.opentelemetry.io/collector/cmd/builder@v0.96.0

WORKDIR /build

# Copy builder config
COPY otel-collector-custom/builder-config.yaml .

# Copy custom processor
COPY otel-processor-correlation ./otel-processor-correlation

# Build the custom collector
RUN builder --config=builder-config.yaml

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S otel && adduser -S otel -G otel

WORKDIR /otel

# Copy the built collector binary
COPY --from=builder /build/dist/ollystack-collector /otel/ollystack-collector

# Copy default config
COPY otel-collector-custom/config.yaml /etc/otel/config.yaml

# Set ownership
RUN chown -R otel:otel /otel /etc/otel

USER otel

EXPOSE 4317 4318 8888 13133 55679 1777

ENTRYPOINT ["/otel/ollystack-collector"]
CMD ["--config=/etc/otel/config.yaml"]
