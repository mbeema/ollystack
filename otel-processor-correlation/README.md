# OllyStack Correlation Processor

A custom OpenTelemetry Collector processor that adds correlation IDs to traces, logs, and metrics.

## Overview

The correlation processor ensures every piece of telemetry data has a `correlation_id` attribute, enabling end-to-end request tracking across distributed systems.

## Features

- **Automatic extraction**: Extracts correlation ID from headers, attributes, W3C baggage, and log bodies
- **Consistent generation**: Derives correlation ID from trace ID for consistency within a trace
- **Multi-signal support**: Processes traces, logs, and metrics
- **Exemplar support**: Adds correlation ID to metric exemplars for trace correlation

## Configuration

```yaml
processors:
  ollystack_correlation:
    # Generate correlation ID if not found (default: true)
    generate_if_missing: true

    # Prefix for generated IDs (default: "olly")
    id_prefix: "olly"

    # Headers to check for existing correlation ID
    extract_from_headers:
      - X-Correlation-ID
      - X-Request-ID

    # Extract from W3C baggage (default: true)
    extract_from_baggage: true
    baggage_key: correlation_id

    # Attribute name to store correlation ID (default: "correlation_id")
    attribute_name: correlation_id

    # Copy to resource attributes (default: true)
    propagate_to_resource: true

    # Use trace_id to derive consistent correlation ID (default: true)
    derive_from_trace_id: true
```

## Correlation ID Format

Generated correlation IDs follow the format:
```
{prefix}-{timestamp_base36}-{suffix_hex}
```

Example: `olly-2n9c3ok-a1b2c3d4`

- `prefix`: Configurable prefix (default: "olly")
- `timestamp`: Current time in milliseconds, base36 encoded
- `suffix`: Either first 4 bytes of trace ID or random 4 bytes

## Extraction Priority

The processor looks for existing correlation IDs in this order:

1. Resource attributes
2. Span/Log attributes
3. W3C trace state/baggage
4. Log body (JSON)
5. HTTP headers (as attributes)

If not found and `generate_if_missing` is true, generates from:
1. Trace ID (if `derive_from_trace_id` is true)
2. Random generation

## Usage in Pipeline

```yaml
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [ollystack_correlation, batch]
      exporters: [clickhouse]
    logs:
      receivers: [otlp]
      processors: [ollystack_correlation, batch]
      exporters: [clickhouse]
    metrics:
      receivers: [otlp]
      processors: [ollystack_correlation, batch]
      exporters: [clickhouse]
```

## Building

Include this processor when building a custom OTel Collector using the OTel Collector Builder (ocb).
