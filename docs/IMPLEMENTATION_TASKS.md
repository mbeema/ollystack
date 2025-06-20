# OllyStack Implementation Tasks
## Integrated Architecture Approach

**Version:** 2.0
**Status:** Active Development
**Last Updated:** 2026-02-03

---

## Task Tracking Legend

- `[ ]` Not Started
- `[~]` In Progress
- `[x]` Completed
- `[!]` Blocked

---

## Strategic Focus

**BUILD** (Our Differentiators):
- OTel Collector Correlation Processor
- Correlation Engine & API
- AI/ML Insights Engine
- Correlation Explorer UI

**INTEGRATE** (Best-in-Class Tools):
- Grafana (Visualization)
- OTel Collector (Collection)
- Vector (Log Shipping)
- Alertmanager (Alert Routing)
- Prometheus (Metrics Scraping)
- Grafana Beyla (eBPF Zero-Code)

---

## Sprint 1: Foundation & Integration Setup

### 1.1 Database Schema Updates

#### Task 1.1.1: Add correlation_id to traces table
```sql
-- File: migrations/001_add_correlation_id.sql
ALTER TABLE ollystack.traces ADD COLUMN IF NOT EXISTS correlation_id String DEFAULT '';
ALTER TABLE ollystack.traces ADD INDEX IF NOT EXISTS idx_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3;
```
- [ ] Create migration file
- [ ] Test migration on dev ClickHouse
- [ ] Apply to production
- [ ] Verify index performance with EXPLAIN

#### Task 1.1.2: Add correlation_id to logs table
```sql
-- File: migrations/002_add_correlation_id_logs.sql
ALTER TABLE ollystack.logs ADD COLUMN IF NOT EXISTS correlation_id String DEFAULT '';
ALTER TABLE ollystack.logs ADD INDEX IF NOT EXISTS idx_log_correlation_id correlation_id TYPE bloom_filter GRANULARITY 3;
```
- [ ] Create migration file
- [ ] Test migration
- [ ] Apply to production

#### Task 1.1.3: Create correlation_summary materialized view
```sql
-- File: migrations/003_correlation_summary_mv.sql
CREATE MATERIALIZED VIEW IF NOT EXISTS ollystack.correlation_summary
ENGINE = SummingMergeTree()
ORDER BY (correlation_id, window_start)
AS SELECT
    correlation_id,
    toStartOfMinute(Timestamp) as window_start,
    min(Timestamp) as first_seen,
    max(Timestamp) as last_seen,
    groupUniqArray(ServiceName) as services,
    count() as span_count,
    countIf(StatusCode = 'ERROR') as error_count,
    sum(Duration) / count() as avg_duration,
    max(Duration) as max_duration
FROM ollystack.traces
WHERE correlation_id != ''
GROUP BY correlation_id, toStartOfMinute(Timestamp);
```
- [ ] Create migration file
- [ ] Test view population
- [ ] Verify query performance

#### Task 1.1.4: Create metric_exemplars table
```sql
-- File: migrations/004_metric_exemplars.sql
CREATE TABLE IF NOT EXISTS ollystack.metric_exemplars (
    Timestamp DateTime64(9),
    MetricName String,
    Value Float64,
    TraceId String,
    SpanId String,
    CorrelationId String,
    ServiceName String,
    Labels Map(String, String)
) ENGINE = MergeTree()
ORDER BY (MetricName, Timestamp)
TTL Timestamp + INTERVAL 30 DAY;
```
- [ ] Create migration file
- [ ] Test table creation
- [ ] Document exemplar ingestion flow

---

### 1.2 OTel Collector Correlation Processor (BUILD)

This is our **core differentiator** - a custom OTel Collector processor.

#### Task 1.2.1: Create OTel Collector Processor Project Structure
```bash
# Directory structure
otel-processor-correlation/
├── go.mod
├── go.sum
├── factory.go          # Processor factory
├── config.go           # Configuration
├── processor.go        # Main logic
├── correlation.go      # Correlation ID generation
├── README.md
└── testdata/
    └── config.yaml
```
- [ ] Initialize Go module
- [ ] Create project structure
- [ ] Add OTel Collector dependencies

#### Task 1.2.2: Implement Correlation Processor Config
**File:** `otel-processor-correlation/config.go`

```go
package correlationprocessor

import (
    "go.opentelemetry.io/collector/component"
)

type Config struct {
    // GenerateIfMissing creates correlation ID if not found
    GenerateIfMissing bool `mapstructure:"generate_if_missing"`

    // IDPrefix for generated correlation IDs (default: "olly")
    IDPrefix string `mapstructure:"id_prefix"`

    // Headers to check for existing correlation ID
    ExtractFromHeaders []string `mapstructure:"extract_from_headers"`

    // Check OTel baggage for correlation ID
    ExtractFromBaggage bool `mapstructure:"extract_from_baggage"`

    // Attribute name to store correlation ID
    AttributeName string `mapstructure:"attribute_name"`
}

func createDefaultConfig() component.Config {
    return &Config{
        GenerateIfMissing:  true,
        IDPrefix:           "olly",
        ExtractFromHeaders: []string{"X-Correlation-ID", "X-Request-ID", "x-correlation-id"},
        ExtractFromBaggage: true,
        AttributeName:      "correlation_id",
    }
}
```
- [ ] Create config.go
- [ ] Add validation logic
- [ ] Write config tests

#### Task 1.2.3: Implement Correlation Processor Logic
**File:** `otel-processor-correlation/processor.go`

```go
package correlationprocessor

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "strconv"
    "time"

    "go.opentelemetry.io/collector/pdata/pcommon"
    "go.opentelemetry.io/collector/pdata/plog"
    "go.opentelemetry.io/collector/pdata/pmetric"
    "go.opentelemetry.io/collector/pdata/ptrace"
    "go.uber.org/zap"
)

type correlationProcessor struct {
    config *Config
    logger *zap.Logger
}

// ProcessTraces adds correlation_id to all spans
func (p *correlationProcessor) ProcessTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
    resourceSpans := td.ResourceSpans()
    for i := 0; i < resourceSpans.Len(); i++ {
        rs := resourceSpans.At(i)
        scopeSpans := rs.ScopeSpans()

        // Try to get correlation ID from resource attributes first
        correlationID := p.extractFromResource(rs.Resource())

        for j := 0; j < scopeSpans.Len(); j++ {
            ss := scopeSpans.At(j)
            spans := ss.Spans()

            for k := 0; k < spans.Len(); k++ {
                span := spans.At(k)

                // Try to extract from span attributes
                if correlationID == "" {
                    correlationID = p.extractFromSpan(span)
                }

                // Generate if still missing
                if correlationID == "" && p.config.GenerateIfMissing {
                    // Use trace ID as seed for consistency within a trace
                    correlationID = p.generateFromTraceID(span.TraceID())
                }

                // Set correlation_id attribute
                if correlationID != "" {
                    span.Attributes().PutStr(p.config.AttributeName, correlationID)
                }
            }
        }

        // Also add to resource attributes for consistency
        if correlationID != "" {
            rs.Resource().Attributes().PutStr(p.config.AttributeName, correlationID)
        }
    }
    return td, nil
}

// ProcessLogs adds correlation_id to all log records
func (p *correlationProcessor) ProcessLogs(ctx context.Context, ld plog.Logs) (plog.Logs, error) {
    resourceLogs := ld.ResourceLogs()
    for i := 0; i < resourceLogs.Len(); i++ {
        rl := resourceLogs.At(i)
        scopeLogs := rl.ScopeLogs()

        correlationID := p.extractFromResource(rl.Resource())

        for j := 0; j < scopeLogs.Len(); j++ {
            sl := scopeLogs.At(j)
            logs := sl.LogRecords()

            for k := 0; k < logs.Len(); k++ {
                log := logs.At(k)

                // Try to extract from log attributes
                if correlationID == "" {
                    if v, ok := log.Attributes().Get(p.config.AttributeName); ok {
                        correlationID = v.Str()
                    }
                }

                // Try from body if JSON
                if correlationID == "" {
                    correlationID = p.extractFromLogBody(log)
                }

                // Generate from trace ID if available
                if correlationID == "" && !log.TraceID().IsEmpty() {
                    correlationID = p.generateFromTraceID(log.TraceID())
                }

                // Set correlation_id attribute
                if correlationID != "" {
                    log.Attributes().PutStr(p.config.AttributeName, correlationID)
                }
            }
        }
    }
    return ld, nil
}

// generateFromTraceID creates consistent correlation ID from trace ID
func (p *correlationProcessor) generateFromTraceID(traceID pcommon.TraceID) string {
    ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
    // Use first 8 bytes of trace ID for consistency
    suffix := hex.EncodeToString(traceID[:4])
    return fmt.Sprintf("%s-%s-%s", p.config.IDPrefix, ts, suffix)
}

// generateNew creates a completely new correlation ID
func (p *correlationProcessor) generateNew() string {
    ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
    random := make([]byte, 4)
    rand.Read(random)
    return fmt.Sprintf("%s-%s-%s", p.config.IDPrefix, ts, hex.EncodeToString(random))
}
```
- [ ] Implement processor.go
- [ ] Add ProcessMetrics for exemplars
- [ ] Write unit tests
- [ ] Test with sample data

#### Task 1.2.4: Create Factory and Register Processor
**File:** `otel-processor-correlation/factory.go`

```go
package correlationprocessor

import (
    "context"

    "go.opentelemetry.io/collector/component"
    "go.opentelemetry.io/collector/consumer"
    "go.opentelemetry.io/collector/processor"
    "go.opentelemetry.io/collector/processor/processorhelper"
)

const (
    typeStr   = "ollystack_correlation"
    stability = component.StabilityLevelBeta
)

func NewFactory() processor.Factory {
    return processor.NewFactory(
        typeStr,
        createDefaultConfig,
        processor.WithTraces(createTracesProcessor, stability),
        processor.WithLogs(createLogsProcessor, stability),
        processor.WithMetrics(createMetricsProcessor, stability),
    )
}

func createTracesProcessor(
    ctx context.Context,
    set processor.CreateSettings,
    cfg component.Config,
    nextConsumer consumer.Traces,
) (processor.Traces, error) {
    proc := &correlationProcessor{
        config: cfg.(*Config),
        logger: set.Logger,
    }
    return processorhelper.NewTracesProcessor(
        ctx, set, cfg, nextConsumer, proc.ProcessTraces)
}
```
- [ ] Create factory.go
- [ ] Register all signal types
- [ ] Build custom collector with processor
- [ ] Test in Docker

#### Task 1.2.5: Build Custom OTel Collector
**File:** `otel-collector-custom/builder-config.yaml`

```yaml
dist:
  name: ollystack-collector
  description: OllyStack Custom OTel Collector
  output_path: ./dist
  otelcol_version: 0.96.0

exporters:
  - gomod: go.opentelemetry.io/collector/exporter/otlpexporter v0.96.0
  - gomod: go.opentelemetry.io/collector/exporter/otlphttpexporter v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/clickhouseexporter v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter v0.96.0

receivers:
  - gomod: go.opentelemetry.io/collector/receiver/otlpreceiver v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/prometheusreceiver v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/receiver/fluentforwardreceiver v0.96.0

processors:
  - gomod: go.opentelemetry.io/collector/processor/batchprocessor v0.96.0
  - gomod: go.opentelemetry.io/collector/processor/memorylimiterprocessor v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/resourceprocessor v0.96.0
  - gomod: github.com/open-telemetry/opentelemetry-collector-contrib/processor/tailsamplingprocessor v0.96.0
  # Our custom processor
  - gomod: github.com/ollystack/otel-processor-correlation v0.1.0

extensions:
  - gomod: go.opentelemetry.io/collector/extension/healthcheckextension v0.96.0
  - gomod: go.opentelemetry.io/collector/extension/pprofextension v0.96.0
```
- [ ] Create builder config
- [ ] Build with `ocb` (OTel Collector Builder)
- [ ] Create Dockerfile
- [ ] Test collector image

---

### 1.3 Grafana Integration (INTEGRATE)

#### Task 1.3.1: Create Grafana Datasource Configuration
**File:** `deploy/kubernetes/grafana/datasources.yaml`

```yaml
apiVersion: 1

datasources:
  # ClickHouse for direct queries
  - name: OllyStack-ClickHouse
    type: grafana-clickhouse-datasource
    uid: ollystack-clickhouse
    url: http://clickhouse:8123
    jsonData:
      defaultDatabase: ollystack
      protocol: http
    editable: true

  # Tempo for trace visualization (optional, for native Grafana trace view)
  - name: Tempo
    type: tempo
    uid: tempo
    url: http://tempo:3200
    jsonData:
      httpMethod: GET
      tracesToLogs:
        datasourceUid: 'ollystack-clickhouse'
        tags: ['correlation_id', 'service.name']
        mappedTags:
          - key: correlation_id
            value: correlation_id
        mapTagNamesEnabled: true
        spanStartTimeShift: '-1h'
        spanEndTimeShift: '1h'
        filterByTraceID: true
      tracesToMetrics:
        datasourceUid: 'prometheus'
        tags:
          - key: service.name
            value: service
      serviceMap:
        datasourceUid: 'prometheus'
      nodeGraph:
        enabled: true

  # Prometheus for metrics
  - name: Prometheus
    type: prometheus
    uid: prometheus
    url: http://prometheus:9090
    jsonData:
      httpMethod: POST
      exemplarTraceIdDestinations:
        - name: trace_id
          datasourceUid: tempo
        - name: correlation_id
          datasourceUid: ollystack-clickhouse
          urlDisplayLabel: 'View in OllyStack'
          url: '${__value.raw}'

  # OllyStack API for correlation queries
  - name: OllyStack-API
    type: marcusolsson-json-datasource
    uid: ollystack-api
    url: http://ollystack-api:8080/api/v1
    jsonData:
      queryParams: ''
```
- [ ] Create datasources.yaml
- [ ] Test ClickHouse connection
- [ ] Configure trace-to-logs linking
- [ ] Verify exemplar linking

#### Task 1.3.2: Create Grafana Dashboard for Correlation
**File:** `deploy/kubernetes/grafana/dashboards/correlation-overview.json`

```json
{
  "title": "OllyStack - Correlation Overview",
  "uid": "ollystack-correlation",
  "panels": [
    {
      "title": "Correlation Lookup",
      "type": "text",
      "gridPos": {"h": 3, "w": 24, "x": 0, "y": 0},
      "options": {
        "content": "Enter a correlation ID to view all related traces, logs, and metrics.\n\n**Quick Link:** [Open in OllyStack Correlation Explorer](/ollystack/correlate/${correlation_id})",
        "mode": "markdown"
      }
    },
    {
      "title": "Traces for Correlation",
      "type": "table",
      "datasource": {"uid": "ollystack-clickhouse"},
      "gridPos": {"h": 8, "w": 12, "x": 0, "y": 3},
      "targets": [
        {
          "rawSql": "SELECT Timestamp, ServiceName, SpanName, Duration/1000000 as duration_ms, StatusCode FROM ollystack.traces WHERE correlation_id = '${correlation_id}' ORDER BY Timestamp"
        }
      ]
    },
    {
      "title": "Logs for Correlation",
      "type": "logs",
      "datasource": {"uid": "ollystack-clickhouse"},
      "gridPos": {"h": 8, "w": 12, "x": 12, "y": 3},
      "targets": [
        {
          "rawSql": "SELECT Timestamp, ServiceName, SeverityText, Body FROM ollystack.logs WHERE correlation_id = '${correlation_id}' ORDER BY Timestamp"
        }
      ]
    }
  ],
  "templating": {
    "list": [
      {
        "name": "correlation_id",
        "type": "textbox",
        "label": "Correlation ID"
      }
    ]
  }
}
```
- [ ] Create correlation dashboard JSON
- [ ] Add service map panel
- [ ] Add latency breakdown panel
- [ ] Test with sample data

#### Task 1.3.3: Grafana Helm Values
**File:** `deploy/kubernetes/grafana/values.yaml`

```yaml
grafana:
  replicas: 2

  persistence:
    enabled: true
    size: 10Gi

  plugins:
    - grafana-clickhouse-datasource
    - marcusolsson-json-datasource

  datasources:
    datasources.yaml:
      apiVersion: 1
      datasources:
        # See datasources.yaml above

  dashboardProviders:
    dashboardproviders.yaml:
      apiVersion: 1
      providers:
        - name: 'ollystack'
          orgId: 1
          folder: 'OllyStack'
          type: file
          disableDeletion: false
          editable: true
          options:
            path: /var/lib/grafana/dashboards/ollystack

  dashboards:
    ollystack:
      correlation-overview:
        file: dashboards/correlation-overview.json
      service-map:
        file: dashboards/service-map.json
      error-analysis:
        file: dashboards/error-analysis.json

  sidecar:
    dashboards:
      enabled: true
    datasources:
      enabled: true

  ingress:
    enabled: true
    hosts:
      - grafana.ollystack.local

  env:
    GF_AUTH_ANONYMOUS_ENABLED: "false"
    GF_SECURITY_ADMIN_PASSWORD: "${GRAFANA_ADMIN_PASSWORD}"
```
- [ ] Create Helm values file
- [ ] Deploy Grafana
- [ ] Verify plugins installed
- [ ] Import dashboards

---

### 1.4 Vector Log Collection (INTEGRATE)

#### Task 1.4.1: Vector Configuration
**File:** `deploy/kubernetes/vector/values.yaml`

```yaml
role: Agent

customConfig:
  data_dir: /vector-data-dir

  sources:
    kubernetes_logs:
      type: kubernetes_logs

    internal_metrics:
      type: internal_metrics

  transforms:
    parse_logs:
      type: remap
      inputs: ["kubernetes_logs"]
      source: |
        # Try to parse JSON logs
        parsed, err = parse_json(.message)
        if err == null {
          . = merge(., parsed)
        }

        # Extract correlation_id from various field names
        .correlation_id = .correlation_id ?? .correlationId ?? .request_id ?? .requestId ?? ""

        # Normalize timestamp
        .timestamp = .timestamp ?? now()

        # Normalize severity
        .severity = downcase(.level ?? .severity ?? .loglevel ?? "info")

        # Map to OTel severity numbers
        .severity_number = if .severity == "trace" { 1 }
          else if .severity == "debug" { 5 }
          else if .severity == "info" { 9 }
          else if .severity == "warn" || .severity == "warning" { 13 }
          else if .severity == "error" { 17 }
          else if .severity == "fatal" || .severity == "critical" { 21 }
          else { 0 }

        # Add Kubernetes metadata as resource attributes
        .resource.k8s.namespace.name = .kubernetes.pod_namespace
        .resource.k8s.pod.name = .kubernetes.pod_name
        .resource.k8s.container.name = .kubernetes.container_name
        .resource.service.name = .kubernetes.pod_labels."app.kubernetes.io/name" ?? .kubernetes.pod_labels.app ?? "unknown"

    filter_noise:
      type: filter
      inputs: ["parse_logs"]
      condition: |
        # Filter out health checks and noise
        !contains(string!(.message), "health") &&
        !contains(string!(.message), "/ready") &&
        !contains(string!(.message), "/live")

  sinks:
    otel_collector:
      type: opentelemetry
      inputs: ["filter_noise"]
      endpoint: http://otel-collector:4317
      protocol: grpc
      compression: gzip

    # Debug sink (optional)
    console:
      type: console
      inputs: ["filter_noise"]
      encoding:
        codec: json
      condition: '{{ env "VECTOR_DEBUG" }} == "true"'

resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi

tolerations:
  - operator: Exists
```
- [ ] Create Vector values file
- [ ] Deploy Vector DaemonSet
- [ ] Test log collection
- [ ] Verify correlation_id extraction

---

### 1.5 Alertmanager Integration (INTEGRATE)

#### Task 1.5.1: Alertmanager Configuration
**File:** `deploy/kubernetes/alertmanager/config.yaml`

```yaml
global:
  resolve_timeout: 5m
  slack_api_url_file: /etc/alertmanager/secrets/slack-webhook

templates:
  - '/etc/alertmanager/templates/*.tmpl'

route:
  group_by: ['alertname', 'service', 'namespace']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 4h
  receiver: 'default'

  routes:
    # Critical alerts go to PagerDuty
    - match:
        severity: critical
      receiver: 'pagerduty-critical'
      continue: true

    # Errors with correlation ID get special handling
    - match:
        has_correlation_id: "true"
      receiver: 'slack-correlated'

    # Warnings go to Slack
    - match:
        severity: warning
      receiver: 'slack-warnings'

receivers:
  - name: 'default'
    slack_configs:
      - channel: '#alerts'
        send_resolved: true
        title: '{{ template "slack.title" . }}'
        text: '{{ template "slack.text" . }}'
        actions:
          - type: button
            text: 'View in Grafana'
            url: '{{ template "grafana.url" . }}'
          - type: button
            text: 'View in OllyStack'
            url: '{{ template "ollystack.url" . }}'

  - name: 'slack-correlated'
    slack_configs:
      - channel: '#alerts-correlated'
        send_resolved: true
        title: '{{ .GroupLabels.alertname }} - Correlated Alert'
        text: |
          {{ range .Alerts }}
          *Service:* {{ .Labels.service }}
          *Namespace:* {{ .Labels.namespace }}
          *Correlation ID:* `{{ .Labels.correlation_id }}`
          *Summary:* {{ .Annotations.summary }}

          :mag: <https://ollystack.example.com/correlate/{{ .Labels.correlation_id }}|View Full Context in OllyStack>
          :chart_with_upwards_trend: <https://grafana.example.com/d/ollystack-correlation?var-correlation_id={{ .Labels.correlation_id }}|View in Grafana>
          {{ end }}
        actions:
          - type: button
            text: 'Investigate in OllyStack'
            url: 'https://ollystack.example.com/correlate/{{ (index .Alerts 0).Labels.correlation_id }}'
            style: primary

  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key_file: /etc/alertmanager/secrets/pagerduty-key
        severity: critical
        description: '{{ .GroupLabels.alertname }}: {{ .CommonAnnotations.summary }}'
        details:
          service: '{{ .GroupLabels.service }}'
          namespace: '{{ .GroupLabels.namespace }}'
          correlation_id: '{{ .GroupLabels.correlation_id }}'
          ollystack_url: 'https://ollystack.example.com/correlate/{{ .GroupLabels.correlation_id }}'
          runbook_url: '{{ .CommonAnnotations.runbook_url }}'

  - name: 'slack-warnings'
    slack_configs:
      - channel: '#warnings'
        send_resolved: true

inhibit_rules:
  - source_match:
      severity: 'critical'
    target_match:
      severity: 'warning'
    equal: ['alertname', 'service']
```
- [ ] Create Alertmanager config
- [ ] Create Slack message templates
- [ ] Deploy Alertmanager
- [ ] Test alert routing

#### Task 1.5.2: Create Alert Message Template
**File:** `deploy/kubernetes/alertmanager/templates/ollystack.tmpl`

```
{{ define "slack.title" }}
[{{ .Status | toUpper }}{{ if eq .Status "firing" }}:{{ .Alerts.Firing | len }}{{ end }}] {{ .GroupLabels.alertname }}
{{ end }}

{{ define "slack.text" }}
{{ range .Alerts }}
*Alert:* {{ .Labels.alertname }}
*Service:* {{ .Labels.service }}
*Severity:* {{ .Labels.severity }}
{{ if .Labels.correlation_id }}*Correlation ID:* `{{ .Labels.correlation_id }}`{{ end }}
*Description:* {{ .Annotations.description }}
*Started:* {{ .StartsAt.Format "2006-01-02 15:04:05" }}
{{ end }}
{{ end }}

{{ define "ollystack.url" }}
{{ if (index .Alerts 0).Labels.correlation_id }}
https://ollystack.example.com/correlate/{{ (index .Alerts 0).Labels.correlation_id }}
{{ else }}
https://ollystack.example.com/alerts
{{ end }}
{{ end }}

{{ define "grafana.url" }}
https://grafana.example.com/d/ollystack-overview?var-service={{ (index .Alerts 0).Labels.service }}
{{ end }}
```
- [ ] Create template file
- [ ] Test template rendering
- [ ] Verify links work

---

## Sprint 2: OllyStack Core API (BUILD)

### 2.1 Correlation Engine

#### Task 2.1.1: Create Correlation Package
**File:** `api-server/internal/correlation/correlation.go`

```go
package correlation

import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "strconv"
    "time"
)

const (
    HeaderCorrelationID = "X-Correlation-ID"
    HeaderRequestID     = "X-Request-ID"
    ContextKey          = "correlation_id"
    Prefix              = "olly"
)

// Generate creates a new correlation ID
// Format: olly-{timestamp_base36}-{random_8hex}
func Generate() string {
    ts := strconv.FormatInt(time.Now().UnixMilli(), 36)
    random := make([]byte, 4)
    rand.Read(random)
    return fmt.Sprintf("%s-%s-%s", Prefix, ts, hex.EncodeToString(random))
}

// FromContext extracts correlation ID from context
func FromContext(ctx context.Context) string {
    if v := ctx.Value(ContextKey); v != nil {
        if s, ok := v.(string); ok {
            return s
        }
    }
    return ""
}

// WithContext adds correlation ID to context
func WithContext(ctx context.Context, correlationID string) context.Context {
    return context.WithValue(ctx, ContextKey, correlationID)
}

// IsValid checks if a correlation ID is valid
func IsValid(id string) bool {
    return len(id) > 0 && len(id) < 100
}

// IsOllyStackGenerated checks if ID was generated by OllyStack
func IsOllyStackGenerated(id string) bool {
    return len(id) > 5 && id[:4] == Prefix+"-"
}
```
- [ ] Create correlation package
- [ ] Add unit tests
- [ ] Document usage

#### Task 2.1.2: Create Correlation Engine
**File:** `api-server/internal/correlation/engine.go`

```go
package correlation

import (
    "context"
    "encoding/json"
    "sync"
    "time"

    "github.com/ClickHouse/clickhouse-go/v2"
    "github.com/redis/go-redis/v9"
    "go.uber.org/zap"
)

type Engine struct {
    db     clickhouse.Conn
    cache  *redis.Client
    logger *zap.Logger
}

type CorrelatedContext struct {
    CorrelationID string            `json:"correlation_id"`
    TimeRange     TimeRange         `json:"time_range"`
    Summary       CorrelationSummary `json:"summary"`
    Traces        []TraceSpan       `json:"traces"`
    Logs          []LogEntry        `json:"logs"`
    Metrics       []MetricPoint     `json:"metrics"`
    Services      []string          `json:"services"`
    Errors        []ErrorInfo       `json:"errors"`
    Timeline      []TimelineEvent   `json:"timeline"`
}

type TimeRange struct {
    Start time.Time `json:"start"`
    End   time.Time `json:"end"`
}

type CorrelationSummary struct {
    TotalSpans    int     `json:"total_spans"`
    TotalLogs     int     `json:"total_logs"`
    TotalMetrics  int     `json:"total_metrics"`
    ErrorCount    int     `json:"error_count"`
    ServiceCount  int     `json:"service_count"`
    DurationMs    float64 `json:"duration_ms"`
}

func NewEngine(db clickhouse.Conn, cache *redis.Client, logger *zap.Logger) *Engine {
    return &Engine{db: db, cache: cache, logger: logger}
}

// GetFullContext returns all signals for a correlation ID
func (e *Engine) GetFullContext(ctx context.Context, correlationID string) (*CorrelatedContext, error) {
    // Check cache first
    cacheKey := "corr:" + correlationID
    if cached, err := e.cache.Get(ctx, cacheKey).Result(); err == nil {
        var result CorrelatedContext
        if err := json.Unmarshal([]byte(cached), &result); err == nil {
            return &result, nil
        }
    }

    // Parallel fetch from ClickHouse
    var (
        traces   []TraceSpan
        logs     []LogEntry
        metrics  []MetricPoint
        wg       sync.WaitGroup
        tracesErr, logsErr, metricsErr error
    )

    wg.Add(3)

    go func() {
        defer wg.Done()
        traces, tracesErr = e.fetchTraces(ctx, correlationID)
    }()

    go func() {
        defer wg.Done()
        logs, logsErr = e.fetchLogs(ctx, correlationID)
    }()

    go func() {
        defer wg.Done()
        metrics, metricsErr = e.fetchMetricExemplars(ctx, correlationID)
    }()

    wg.Wait()

    // Check for errors
    if tracesErr != nil {
        return nil, tracesErr
    }

    // Build response
    result := &CorrelatedContext{
        CorrelationID: correlationID,
        Traces:        traces,
        Logs:          logs,
        Metrics:       metrics,
        Services:      e.extractServices(traces),
        Errors:        e.extractErrors(traces, logs),
        Timeline:      e.buildTimeline(traces, logs, metrics),
        TimeRange:     e.calculateTimeRange(traces, logs),
        Summary:       e.buildSummary(traces, logs, metrics),
    }

    // Cache for 5 minutes
    if data, err := json.Marshal(result); err == nil {
        e.cache.Set(ctx, cacheKey, data, 5*time.Minute)
    }

    return result, nil
}

func (e *Engine) fetchTraces(ctx context.Context, correlationID string) ([]TraceSpan, error) {
    query := `
        SELECT
            Timestamp,
            TraceId,
            SpanId,
            ParentSpanId,
            SpanName,
            SpanKind,
            ServiceName,
            Duration,
            StatusCode,
            StatusMessage,
            SpanAttributes
        FROM ollystack.traces
        WHERE correlation_id = $1
        ORDER BY Timestamp ASC
    `

    rows, err := e.db.Query(ctx, query, correlationID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var traces []TraceSpan
    for rows.Next() {
        var t TraceSpan
        if err := rows.ScanStruct(&t); err != nil {
            continue
        }
        traces = append(traces, t)
    }
    return traces, nil
}

func (e *Engine) fetchLogs(ctx context.Context, correlationID string) ([]LogEntry, error) {
    query := `
        SELECT
            Timestamp,
            TraceId,
            SpanId,
            SeverityText,
            SeverityNumber,
            ServiceName,
            Body,
            Attributes
        FROM ollystack.logs
        WHERE correlation_id = $1
        ORDER BY Timestamp ASC
    `

    rows, err := e.db.Query(ctx, query, correlationID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var logs []LogEntry
    for rows.Next() {
        var l LogEntry
        if err := rows.ScanStruct(&l); err != nil {
            continue
        }
        logs = append(logs, l)
    }
    return logs, nil
}

func (e *Engine) buildTimeline(traces []TraceSpan, logs []LogEntry, metrics []MetricPoint) []TimelineEvent {
    var events []TimelineEvent

    // Add trace events
    for _, t := range traces {
        events = append(events, TimelineEvent{
            Timestamp:   t.Timestamp,
            Type:        "span",
            Service:     t.ServiceName,
            Description: t.SpanName,
            SpanID:      t.SpanId,
            DurationMs:  float64(t.Duration) / 1e6,
            IsError:     t.StatusCode == "ERROR",
        })
    }

    // Add log events
    for _, l := range logs {
        events = append(events, TimelineEvent{
            Timestamp:   l.Timestamp,
            Type:        "log",
            Service:     l.ServiceName,
            Description: truncate(l.Body, 100),
            Level:       l.SeverityText,
            IsError:     l.SeverityNumber >= 17,
        })
    }

    // Sort by timestamp
    sort.Slice(events, func(i, j int) bool {
        return events[i].Timestamp.Before(events[j].Timestamp)
    })

    return events
}
```
- [ ] Create engine.go
- [ ] Implement all fetch methods
- [ ] Add timeline builder
- [ ] Write integration tests

#### Task 2.1.3: Create Correlation Handler
**File:** `api-server/internal/handlers/correlation.go`

```go
package handlers

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "ollystack/internal/correlation"
)

// GetCorrelatedContext godoc
// @Summary Get full correlated context
// @Description Returns all traces, logs, and metrics for a correlation ID
// @Tags correlation
// @Produce json
// @Param correlation_id path string true "Correlation ID"
// @Success 200 {object} correlation.CorrelatedContext
// @Router /api/v1/correlate/{correlation_id} [get]
func (h *Handler) GetCorrelatedContext(c *gin.Context) {
    correlationID := c.Param("correlation_id")
    if correlationID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "correlation_id required"})
        return
    }

    ctx := c.Request.Context()
    result, err := h.correlationEngine.GetFullContext(ctx, correlationID)
    if err != nil {
        h.logger.Error("failed to get correlation context", zap.Error(err))
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    if result.Summary.TotalSpans == 0 && result.Summary.TotalLogs == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "correlation_id not found"})
        return
    }

    c.JSON(http.StatusOK, result)
}

// GetCorrelationTimeline godoc
// @Summary Get correlation timeline
// @Description Returns chronological timeline of all events
// @Tags correlation
// @Produce json
// @Param correlation_id path string true "Correlation ID"
// @Success 200 {array} correlation.TimelineEvent
// @Router /api/v1/correlate/{correlation_id}/timeline [get]
func (h *Handler) GetCorrelationTimeline(c *gin.Context) {
    correlationID := c.Param("correlation_id")

    ctx := c.Request.Context()
    result, err := h.correlationEngine.GetFullContext(ctx, correlationID)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }

    c.JSON(http.StatusOK, result.Timeline)
}
```
- [ ] Create handler file
- [ ] Add to router
- [ ] Write API tests
- [ ] Add OpenAPI documentation

#### Task 2.1.4: Add Routes
**File:** `api-server/cmd/server/main.go` (update)

```go
// Add correlation routes
correlation := v1.Group("/correlate")
{
    correlation.GET("/:correlation_id", handler.GetCorrelatedContext)
    correlation.GET("/:correlation_id/timeline", handler.GetCorrelationTimeline)
    correlation.GET("/:correlation_id/impact", handler.GetCorrelationImpact)
}

// Add AI routes
ai := v1.Group("/ai")
{
    ai.POST("/analyze", handler.AIAnalyze)
    ai.POST("/nlq", handler.NaturalLanguageQuery)
    ai.POST("/rca", handler.RootCauseAnalysis)
}
```
- [ ] Update router
- [ ] Test all endpoints
- [ ] Verify with curl

---

## Sprint 3: AI/ML Engine (BUILD)

### 3.1 AI Engine Foundation

#### Task 3.1.1: Create AI Engine Project Structure
```bash
ai-engine/
├── pyproject.toml
├── Dockerfile
├── ollystack/
│   ├── __init__.py
│   ├── main.py
│   ├── config.py
│   ├── api/
│   │   ├── __init__.py
│   │   └── routes.py
│   ├── rca/
│   │   ├── __init__.py
│   │   ├── analyzer.py
│   │   └── critical_path.py
│   ├── anomaly/
│   │   ├── __init__.py
│   │   ├── detector.py
│   │   └── baseline.py
│   ├── nlq/
│   │   ├── __init__.py
│   │   └── query.py
│   └── llm/
│       ├── __init__.py
│       ├── provider.py
│       └── prompts.py
└── tests/
```
- [ ] Create project structure
- [ ] Add pyproject.toml with dependencies
- [ ] Create Dockerfile

#### Task 3.1.2: Implement LLM Provider Abstraction
**File:** `ai-engine/ollystack/llm/provider.py`

```python
from abc import ABC, abstractmethod
from typing import Optional
import os

import openai
import anthropic


class LLMProvider(ABC):
    @abstractmethod
    async def complete(self, prompt: str, system: str = None) -> str:
        pass

    @abstractmethod
    async def complete_json(self, prompt: str, system: str = None) -> dict:
        pass


class OpenAIProvider(LLMProvider):
    def __init__(self, api_key: str = None, model: str = "gpt-4o"):
        self.client = openai.AsyncOpenAI(api_key=api_key or os.getenv("OPENAI_API_KEY"))
        self.model = model

    async def complete(self, prompt: str, system: str = None) -> str:
        messages = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        response = await self.client.chat.completions.create(
            model=self.model,
            messages=messages
        )
        return response.choices[0].message.content

    async def complete_json(self, prompt: str, system: str = None) -> dict:
        messages = []
        if system:
            messages.append({"role": "system", "content": system})
        messages.append({"role": "user", "content": prompt})

        response = await self.client.chat.completions.create(
            model=self.model,
            messages=messages,
            response_format={"type": "json_object"}
        )
        import json
        return json.loads(response.choices[0].message.content)


class ClaudeProvider(LLMProvider):
    def __init__(self, api_key: str = None, model: str = "claude-3-opus-20240229"):
        self.client = anthropic.AsyncAnthropic(api_key=api_key or os.getenv("ANTHROPIC_API_KEY"))
        self.model = model

    async def complete(self, prompt: str, system: str = None) -> str:
        response = await self.client.messages.create(
            model=self.model,
            max_tokens=4096,
            system=system or "",
            messages=[{"role": "user", "content": prompt}]
        )
        return response.content[0].text


def get_provider(provider_name: str = None) -> LLMProvider:
    provider_name = provider_name or os.getenv("LLM_PROVIDER", "openai")

    if provider_name == "openai":
        return OpenAIProvider()
    elif provider_name == "claude":
        return ClaudeProvider()
    else:
        raise ValueError(f"Unknown LLM provider: {provider_name}")
```
- [ ] Create provider abstraction
- [ ] Implement OpenAI provider
- [ ] Implement Claude provider
- [ ] Add local/Ollama option

#### Task 3.1.3: Implement Root Cause Analyzer
**File:** `ai-engine/ollystack/rca/analyzer.py`

```python
from dataclasses import dataclass
from typing import List, Optional
import json

from ollystack.llm.provider import LLMProvider
from ollystack.llm.prompts import RCA_SYSTEM_PROMPT


@dataclass
class RCAResult:
    root_cause: str
    confidence: float
    evidence: List[str]
    affected_services: List[str]
    recommendations: List[str]
    timeline_summary: str


class RootCauseAnalyzer:
    def __init__(self, llm: LLMProvider, db_client):
        self.llm = llm
        self.db = db_client

    async def analyze(self, correlation_id: str) -> RCAResult:
        # Fetch context from API
        context = await self._fetch_context(correlation_id)

        # Run traditional analysis
        critical_path = self._find_critical_path(context["traces"])
        error_origin = self._find_error_origin(context)
        anomalies = await self._detect_anomalies(context)

        # Build prompt
        prompt = self._build_prompt(context, critical_path, error_origin, anomalies)

        # Get LLM analysis
        result = await self.llm.complete_json(prompt, RCA_SYSTEM_PROMPT)

        return RCAResult(
            root_cause=result["root_cause"],
            confidence=result["confidence"],
            evidence=result["evidence"],
            affected_services=list(set(t["ServiceName"] for t in context["traces"])),
            recommendations=result["recommendations"],
            timeline_summary=result["timeline_summary"]
        )

    def _find_critical_path(self, traces: List[dict]) -> List[dict]:
        """Find the longest/slowest execution path."""
        if not traces:
            return []

        # Build span tree
        spans_by_id = {t["SpanId"]: t for t in traces}
        children = {}
        root = None

        for t in traces:
            parent = t.get("ParentSpanId", "")
            if not parent:
                root = t
            else:
                children.setdefault(parent, []).append(t)

        # DFS to find longest path
        def find_longest(span):
            span_children = children.get(span["SpanId"], [])
            if not span_children:
                return [span], span["Duration"]

            max_path = []
            max_duration = 0
            for child in span_children:
                path, duration = find_longest(child)
                if duration > max_duration:
                    max_path = path
                    max_duration = duration

            return [span] + max_path, span["Duration"] + max_duration

        if root:
            path, _ = find_longest(root)
            return path
        return traces[:5]  # Fallback

    def _find_error_origin(self, context: dict) -> Optional[dict]:
        """Find the first error in the chain."""
        # Check traces for errors
        for t in context["traces"]:
            if t.get("StatusCode") == "ERROR":
                return {
                    "type": "trace",
                    "service": t["ServiceName"],
                    "operation": t["SpanName"],
                    "message": t.get("StatusMessage", ""),
                    "timestamp": t["Timestamp"]
                }

        # Check logs for errors
        for l in context["logs"]:
            if l.get("SeverityNumber", 0) >= 17:
                return {
                    "type": "log",
                    "service": l["ServiceName"],
                    "message": l["Body"][:200],
                    "timestamp": l["Timestamp"]
                }

        return None
```
- [ ] Implement analyzer
- [ ] Add critical path finder
- [ ] Create test cases

---

## Sprint 4: Correlation Explorer UI (BUILD)

### 4.1 Frontend Components

#### Task 4.1.1: Create Correlation Explorer Page
**File:** `web-ui/src/pages/CorrelationExplorerPage.tsx`

```tsx
import React, { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Search, Clock, AlertTriangle, Activity } from 'lucide-react';
import { CorrelationTimeline } from '../components/correlation/Timeline';
import { ServiceFlow } from '../components/correlation/ServiceFlow';
import { LogsPanel } from '../components/correlation/LogsPanel';
import { AIInsightsPanel } from '../components/correlation/AIInsights';

export function CorrelationExplorerPage() {
  const [correlationId, setCorrelationId] = useState('');
  const [searchInput, setSearchInput] = useState('');

  const { data, isLoading, error } = useQuery({
    queryKey: ['correlation', correlationId],
    queryFn: async () => {
      const res = await fetch(`/api/v1/correlate/${correlationId}`);
      if (!res.ok) throw new Error('Correlation not found');
      return res.json();
    },
    enabled: !!correlationId,
  });

  const handleSearch = () => {
    setCorrelationId(searchInput);
  };

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <h1 className="text-2xl font-bold mb-6">Correlation Explorer</h1>

      {/* Search Bar */}
      <div className="flex gap-2 mb-6">
        <input
          type="text"
          placeholder="Enter Correlation ID (e.g., olly-2k8f9x3-a7b2c4d1)"
          className="flex-1 px-4 py-2 border rounded-lg focus:ring-2 focus:ring-blue-500"
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
        <button
          onClick={handleSearch}
          className="px-6 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700"
        >
          <Search className="w-5 h-5" />
        </button>
      </div>

      {isLoading && (
        <div className="text-center py-12">
          <div className="animate-spin w-8 h-8 border-4 border-blue-500 border-t-transparent rounded-full mx-auto" />
          <p className="mt-4 text-gray-600">Loading correlation data...</p>
        </div>
      )}

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700">
          Correlation ID not found. Please check the ID and try again.
        </div>
      )}

      {data && (
        <>
          {/* Summary Cards */}
          <div className="grid grid-cols-4 gap-4 mb-6">
            <SummaryCard
              icon={<Activity className="w-5 h-5 text-blue-500" />}
              label="Services"
              value={data.summary.service_count}
            />
            <SummaryCard
              icon={<Clock className="w-5 h-5 text-green-500" />}
              label="Duration"
              value={`${data.summary.duration_ms.toFixed(0)}ms`}
            />
            <SummaryCard
              icon={<Activity className="w-5 h-5 text-purple-500" />}
              label="Spans"
              value={data.summary.total_spans}
            />
            <SummaryCard
              icon={<AlertTriangle className="w-5 h-5 text-red-500" />}
              label="Errors"
              value={data.summary.error_count}
              highlight={data.summary.error_count > 0}
            />
          </div>

          {/* Timeline */}
          <div className="bg-white rounded-lg shadow mb-6">
            <h2 className="text-lg font-semibold p-4 border-b">Timeline</h2>
            <CorrelationTimeline events={data.timeline} />
          </div>

          {/* Service Flow */}
          <div className="bg-white rounded-lg shadow mb-6">
            <h2 className="text-lg font-semibold p-4 border-b">Service Flow</h2>
            <ServiceFlow traces={data.traces} />
          </div>

          {/* Tabs */}
          <div className="bg-white rounded-lg shadow">
            <Tabs>
              <Tab label="Traces" count={data.traces.length}>
                <TracesTable traces={data.traces} />
              </Tab>
              <Tab label="Logs" count={data.logs.length}>
                <LogsPanel logs={data.logs} />
              </Tab>
              <Tab label="AI Insights">
                <AIInsightsPanel correlationId={correlationId} />
              </Tab>
            </Tabs>
          </div>
        </>
      )}
    </div>
  );
}
```
- [ ] Create page component
- [ ] Implement search functionality
- [ ] Add summary cards
- [ ] Style with Tailwind

#### Task 4.1.2: Create Timeline Component
**File:** `web-ui/src/components/correlation/Timeline.tsx`

```tsx
import React from 'react';
import { formatDistanceToNow } from 'date-fns';

interface TimelineEvent {
  timestamp: string;
  type: 'span' | 'log' | 'metric';
  service: string;
  description: string;
  level?: string;
  duration_ms?: number;
  is_error?: boolean;
}

interface Props {
  events: TimelineEvent[];
}

export function CorrelationTimeline({ events }: Props) {
  if (!events.length) {
    return <div className="p-4 text-gray-500">No events found</div>;
  }

  const startTime = new Date(events[0].timestamp).getTime();
  const endTime = new Date(events[events.length - 1].timestamp).getTime();
  const totalDuration = endTime - startTime;

  return (
    <div className="p-4">
      {/* Time ruler */}
      <div className="flex justify-between text-xs text-gray-500 mb-2">
        <span>0ms</span>
        <span>{(totalDuration / 2).toFixed(0)}ms</span>
        <span>{totalDuration.toFixed(0)}ms</span>
      </div>

      {/* Events */}
      <div className="space-y-2">
        {events.map((event, index) => {
          const eventTime = new Date(event.timestamp).getTime();
          const offset = ((eventTime - startTime) / totalDuration) * 100;

          return (
            <div key={index} className="flex items-center gap-2">
              {/* Service label */}
              <div className="w-24 text-sm text-gray-600 truncate">
                {event.service}
              </div>

              {/* Timeline bar */}
              <div className="flex-1 h-6 bg-gray-100 rounded relative">
                <div
                  className={`absolute h-full rounded ${
                    event.is_error
                      ? 'bg-red-500'
                      : event.type === 'span'
                      ? 'bg-blue-500'
                      : 'bg-green-500'
                  }`}
                  style={{
                    left: `${offset}%`,
                    width: event.duration_ms
                      ? `${(event.duration_ms / totalDuration) * 100}%`
                      : '4px',
                    minWidth: '4px',
                  }}
                  title={event.description}
                />
              </div>

              {/* Duration */}
              <div className="w-20 text-xs text-right text-gray-500">
                {event.duration_ms ? `${event.duration_ms.toFixed(1)}ms` : '-'}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
```
- [ ] Create timeline component
- [ ] Add zoom/pan controls
- [ ] Add hover tooltips

---

## Deployment Tasks

### Infrastructure (Single VM - EC2)
- [x] Create Terraform configuration for single VM deployment
- [x] Configure user-data script with Docker setup
- [x] Add swap configuration (2GB) to user-data
- [x] Deploy on-demand t3.medium instance (us-east-1c)
- [x] Configure persistent API config at `/opt/ollystack/config/api/`

### Deploy OTel Collector
- [x] Deploy OTel Collector container (ollystack-collector)
- [x] Configure ClickHouse exporter
- [ ] Build custom collector image with correlation processor
- [ ] Push to container registry
- [ ] Deploy to Kubernetes (future)

### Deploy Storage Layer
- [x] Deploy ClickHouse container (ollystack-clickhouse)
- [x] Deploy Redis container (ollystack-redis)
- [x] Configure ClickHouse with ollystack database and user

### Deploy OllyStack Components
- [x] Deploy API server (ollystack-api:8080)
- [x] Deploy Web UI (ollystack-web:3000)
- [x] Fix Dockerfile CMD issue (removed `serve` command)
- [x] Configure API with correct ClickHouse credentials
- [ ] Deploy AI engine (Python FastAPI)
- [ ] Configure ingress/load balancer

### Deploy Grafana Stack (Future)
- [ ] Deploy Grafana with Helm
- [ ] Configure datasources
- [ ] Import dashboards
- [ ] Test trace-to-logs linking

### Deploy Vector (Future)
- [ ] Deploy Vector DaemonSet
- [ ] Verify log collection
- [ ] Test correlation_id extraction

### Deploy Alertmanager (Future)
- [ ] Deploy Alertmanager
- [ ] Configure Slack integration
- [ ] Test alert routing

---

## Sprint 5: Research-Based Improvements

Based on industry research, papers, and 2024-2025 best practices.

### 5.1 Causal Inference for Root Cause Analysis (Critical)

**Finding:** LLMs hallucinate root causes. Use causal graphs for identification, LLM only for explanation.

#### Task 5.1.1: Integrate DoWhy Library
**File:** `ai-engine/ollystack/rca/causal_graph.py`

```python
from dowhy import CausalModel
import networkx as nx
from typing import List, Dict, Optional
from dataclasses import dataclass

@dataclass
class CausalRCAResult:
    root_cause_service: str
    root_cause_metric: str
    confidence: float
    causal_path: List[str]
    counterfactual: str
    llm_explanation: str

class CausalGraphRCA:
    """
    Causal inference-based RCA using DoWhy.
    LLM is used ONLY for explanation, not for root cause identification.
    """

    def __init__(self, llm_provider, clickhouse_client):
        self.llm = llm_provider
        self.db = clickhouse_client

    async def analyze(self, correlation_id: str) -> CausalRCAResult:
        # 1. Build service dependency graph from traces
        dependency_graph = await self._build_dependency_graph(correlation_id)

        # 2. Collect metric time series for each service
        metrics = await self._collect_service_metrics(correlation_id)

        # 3. Build causal model
        causal_model = self._build_causal_model(dependency_graph, metrics)

        # 4. Identify root cause using causal inference (NOT LLM)
        root_cause = self._identify_root_cause(causal_model)

        # 5. Generate counterfactual analysis
        counterfactual = self._counterfactual_analysis(causal_model, root_cause)

        # 6. Use LLM ONLY for human-readable explanation
        explanation = await self._generate_explanation(root_cause, counterfactual)

        return CausalRCAResult(
            root_cause_service=root_cause.service,
            root_cause_metric=root_cause.metric,
            confidence=root_cause.confidence,
            causal_path=root_cause.path,
            counterfactual=counterfactual,
            llm_explanation=explanation
        )

    async def _build_dependency_graph(self, correlation_id: str) -> nx.DiGraph:
        """Build service dependency graph from trace parent-child relationships."""
        query = """
            SELECT DISTINCT
                ServiceName as child_service,
                parent.ServiceName as parent_service
            FROM ollystack.traces t
            LEFT JOIN ollystack.traces parent
                ON t.ParentSpanId = parent.SpanId
                AND t.correlation_id = parent.correlation_id
            WHERE t.correlation_id = $1
                AND parent.ServiceName IS NOT NULL
        """
        rows = await self.db.execute(query, correlation_id)

        graph = nx.DiGraph()
        for row in rows:
            graph.add_edge(row['parent_service'], row['child_service'])
        return graph

    def _build_causal_model(self, graph: nx.DiGraph, metrics: Dict) -> CausalModel:
        """Build DoWhy causal model from dependency graph."""
        # Convert NetworkX graph to DoWhy format
        # Nodes are services, edges represent causal relationships
        model = CausalModel(
            data=metrics,
            graph=graph,
            treatment='anomaly_source',
            outcome='error_rate'
        )
        return model

    def _identify_root_cause(self, model: CausalModel):
        """Use causal inference to identify root cause."""
        # Identify effect
        identified_estimand = model.identify_effect()

        # Estimate causal effect
        estimate = model.estimate_effect(
            identified_estimand,
            method_name="backdoor.linear_regression"
        )

        # Refute the estimate
        refutation = model.refute_estimate(
            identified_estimand,
            estimate,
            method_name="random_common_cause"
        )

        return self._extract_root_cause(estimate, refutation)

    def _counterfactual_analysis(self, model: CausalModel, root_cause) -> str:
        """
        Answer: 'What would have happened if root cause didn't occur?'
        """
        counterfactual = model.do(
            intervention={root_cause.metric: root_cause.normal_value}
        )
        return f"If {root_cause.service}.{root_cause.metric} had been normal, error rate would be {counterfactual['error_rate']:.2%}"
```

- [ ] Add DoWhy to pyproject.toml dependencies
- [ ] Implement CausalGraphRCA class
- [ ] Build dependency graph from traces
- [ ] Implement counterfactual analysis
- [ ] Write tests with mock data
- [ ] Benchmark against pure-LLM approach

#### Task 5.1.2: Chain-of-Event Analysis
**File:** `ai-engine/ollystack/rca/chain_of_event.py`

```python
from dataclasses import dataclass
from typing import List
import numpy as np

@dataclass
class Event:
    timestamp: float
    service: str
    event_type: str  # span_start, span_end, log, metric_anomaly
    attributes: dict
    is_anomaly: bool

@dataclass
class EventChain:
    events: List[Event]
    causal_weight: float
    interpretation: str

class ChainOfEventRCA:
    """
    Interpretable RCA using event chains.
    Based on FSE 2024 research paper.
    """

    def analyze(self, correlation_id: str) -> List[EventChain]:
        # 1. Transform multi-modal data into unified events
        events = self._transform_to_events(correlation_id)

        # 2. Learn weighted causal graph between events
        causal_graph = self._learn_causal_graph(events)

        # 3. Extract event chains leading to errors
        chains = self._extract_chains(causal_graph, events)

        # 4. Rank chains by causal weight
        ranked_chains = sorted(chains, key=lambda c: c.causal_weight, reverse=True)

        # 5. Generate interpretations aligned with SRE experience
        for chain in ranked_chains:
            chain.interpretation = self._interpret_chain(chain)

        return ranked_chains

    def _transform_to_events(self, correlation_id: str) -> List[Event]:
        """Transform traces, logs, metrics into unified event stream."""
        events = []

        # Add span events
        for span in self._fetch_spans(correlation_id):
            events.append(Event(
                timestamp=span.start_time,
                service=span.service_name,
                event_type='span_start',
                attributes={'operation': span.operation},
                is_anomaly=span.status == 'ERROR'
            ))

        # Add log events
        for log in self._fetch_logs(correlation_id):
            events.append(Event(
                timestamp=log.timestamp,
                service=log.service_name,
                event_type='log',
                attributes={'level': log.severity, 'message': log.body[:100]},
                is_anomaly=log.severity_number >= 17
            ))

        # Add metric anomaly events
        for anomaly in self._detect_metric_anomalies(correlation_id):
            events.append(Event(
                timestamp=anomaly.timestamp,
                service=anomaly.service,
                event_type='metric_anomaly',
                attributes={'metric': anomaly.metric_name, 'value': anomaly.value},
                is_anomaly=True
            ))

        return sorted(events, key=lambda e: e.timestamp)

    def _learn_causal_graph(self, events: List[Event]) -> np.ndarray:
        """Learn weighted adjacency matrix representing causal relationships."""
        n = len(events)
        weights = np.zeros((n, n))

        for i, event_i in enumerate(events):
            for j, event_j in enumerate(events):
                if i >= j:
                    continue  # Only consider forward causation

                # Calculate causal weight based on:
                # 1. Temporal proximity
                # 2. Service dependency
                # 3. Event type relationship
                time_weight = self._temporal_weight(event_i, event_j)
                dep_weight = self._dependency_weight(event_i, event_j)
                type_weight = self._type_weight(event_i, event_j)

                weights[i, j] = time_weight * dep_weight * type_weight

        return weights

    def _interpret_chain(self, chain: EventChain) -> str:
        """Generate SRE-friendly interpretation."""
        steps = []
        for i, event in enumerate(chain.events):
            if event.event_type == 'metric_anomaly':
                steps.append(f"{i+1}. {event.service} showed anomaly in {event.attributes['metric']}")
            elif event.event_type == 'log' and event.is_anomaly:
                steps.append(f"{i+1}. {event.service} logged error: {event.attributes['message'][:50]}")
            elif event.event_type == 'span_start' and event.is_anomaly:
                steps.append(f"{i+1}. {event.service}.{event.attributes['operation']} failed")

        return " → ".join(steps)
```

- [ ] Implement ChainOfEventRCA class
- [ ] Transform multi-modal data to events
- [ ] Learn causal graph weights
- [ ] Generate interpretable chains
- [ ] Add to RCA API endpoint

---

### 5.2 OpenTelemetry Semantic Conventions Compliance

**Finding:** Using standard attribute names enables Grafana auto-detection and cross-vendor compatibility.

#### Task 5.2.1: Create Semantic Conventions Constants
**File:** `api-server/internal/semconv/attributes.go`

```go
package semconv

// HTTP Semantic Conventions (Stable as of 2024)
// See: https://opentelemetry.io/docs/specs/semconv/http/
const (
    // Request attributes
    HTTPRequestMethod      = "http.request.method"       // GET, POST, etc.
    HTTPRequestBodySize    = "http.request.body.size"
    HTTPRoute              = "http.route"                 // /api/users/:id (NOT target!)

    // Response attributes
    HTTPResponseStatusCode = "http.response.status_code"
    HTTPResponseBodySize   = "http.response.body.size"

    // Client/Server
    ServerAddress          = "server.address"
    ServerPort             = "server.port"
    ClientAddress          = "client.address"

    // DEPRECATED - do not use
    // HTTPMethod          = "http.method"              // Use http.request.method
    // HTTPTarget          = "http.target"              // Use http.route
    // HTTPStatusCode      = "http.status_code"         // Use http.response.status_code
)

// Database Semantic Conventions (Stable as of 2025)
// See: https://opentelemetry.io/docs/specs/semconv/database/
const (
    DBSystem              = "db.system"                  // postgresql, mysql, redis
    DBCollectionName      = "db.collection.name"         // table or collection name
    DBOperationName       = "db.operation.name"          // SELECT, INSERT, etc.
    DBQueryText           = "db.query.text"              // sanitized query
    DBResponseStatusCode  = "db.response.status_code"

    // DEPRECATED
    // DBName             = "db.name"                   // Use db.namespace
    // DBStatement        = "db.statement"              // Use db.query.text
)

// Service/Resource Semantic Conventions
const (
    ServiceName           = "service.name"
    ServiceVersion        = "service.version"
    ServiceNamespace      = "service.namespace"
    DeploymentEnvironment = "deployment.environment"
)

// OllyStack Custom Attributes
const (
    CorrelationID         = "correlation_id"
    TenantID              = "tenant_id"
)
```

- [ ] Create semconv package
- [ ] Document all standard attributes
- [ ] Add migration guide for deprecated attributes

#### Task 5.2.2: Update OTel Processor for Semantic Conventions
**File:** `otel-processor-correlation/semconv_transform.go`

```go
package correlationprocessor

import (
    "go.opentelemetry.io/collector/pdata/pcommon"
)

// TransformToStableConventions migrates deprecated attributes to stable ones
func (p *correlationProcessor) TransformToStableConventions(attrs pcommon.Map) {
    // HTTP: http.method → http.request.method
    if v, ok := attrs.Get("http.method"); ok {
        attrs.PutStr("http.request.method", v.Str())
        attrs.Remove("http.method")
    }

    // HTTP: http.status_code → http.response.status_code
    if v, ok := attrs.Get("http.status_code"); ok {
        attrs.PutInt("http.response.status_code", v.Int())
        attrs.Remove("http.status_code")
    }

    // HTTP: http.target → http.route (if route-like)
    if v, ok := attrs.Get("http.target"); ok {
        target := v.Str()
        if isRouteLike(target) {
            attrs.PutStr("http.route", target)
        }
        attrs.Remove("http.target")
    }

    // DB: db.statement → db.query.text
    if v, ok := attrs.Get("db.statement"); ok {
        attrs.PutStr("db.query.text", sanitizeQuery(v.Str()))
        attrs.Remove("db.statement")
    }

    // DB: db.name → db.namespace
    if v, ok := attrs.Get("db.name"); ok {
        attrs.PutStr("db.namespace", v.Str())
        attrs.Remove("db.name")
    }
}

func isRouteLike(target string) bool {
    // Check if target looks like a route (has path parameters)
    // /api/users/123 → /api/users/:id
    return strings.Contains(target, "/")
}

func sanitizeQuery(query string) string {
    // Remove sensitive values from queries
    // SELECT * FROM users WHERE password='secret' → SELECT * FROM users WHERE password=?
    return sanitizer.Sanitize(query)
}
```

- [ ] Implement semantic convention transformer
- [ ] Add to processor pipeline
- [ ] Test with legacy instrumentation

#### Task 5.2.3: Add Attribute Validation
**File:** `otel-processor-correlation/validation.go`

```go
package correlationprocessor

import (
    "go.uber.org/zap"
)

type AttributeValidator struct {
    logger *zap.Logger
    strict bool  // If true, reject invalid attributes
}

func (v *AttributeValidator) Validate(attrs pcommon.Map) []ValidationWarning {
    var warnings []ValidationWarning

    // Check for deprecated attributes
    deprecated := []string{
        "http.method", "http.status_code", "http.target",
        "db.statement", "db.name",
        "net.peer.ip", "net.peer.port",
    }

    for _, attr := range deprecated {
        if _, ok := attrs.Get(attr); ok {
            warnings = append(warnings, ValidationWarning{
                Attribute: attr,
                Message:   "Deprecated attribute, will be auto-migrated",
                Severity:  "WARN",
            })
        }
    }

    // Check for high-cardinality attributes that shouldn't be indexed
    highCardinality := []string{"http.target", "http.url", "db.query.text"}
    for _, attr := range highCardinality {
        if v, ok := attrs.Get(attr); ok && len(v.Str()) > 1000 {
            warnings = append(warnings, ValidationWarning{
                Attribute: attr,
                Message:   "High-cardinality attribute value, consider using route instead",
                Severity:  "WARN",
            })
        }
    }

    return warnings
}
```

- [ ] Implement validation logic
- [ ] Add metrics for deprecated attribute usage
- [ ] Create Grafana dashboard for convention compliance

---

### 5.3 W3C Baggage for Correlation ID Propagation

**Finding:** W3C Baggage is the standard way to propagate custom context across services.

#### Task 5.3.1: Update Correlation Processor for Baggage
**File:** `otel-processor-correlation/baggage.go`

```go
package correlationprocessor

import (
    "go.opentelemetry.io/otel/baggage"
)

const (
    BaggageCorrelationID = "correlation_id"
    BaggageTenantID      = "tenant_id"
)

// ExtractFromBaggage extracts correlation_id from W3C Baggage header
func (p *correlationProcessor) ExtractFromBaggage(ctx context.Context) string {
    bag := baggage.FromContext(ctx)

    // Check for correlation_id member
    member := bag.Member(BaggageCorrelationID)
    if member.Key() != "" {
        return member.Value()
    }

    return ""
}

// InjectToBaggage adds correlation_id to W3C Baggage for downstream propagation
func (p *correlationProcessor) InjectToBaggage(ctx context.Context, correlationID string) context.Context {
    member, err := baggage.NewMember(BaggageCorrelationID, correlationID)
    if err != nil {
        p.logger.Warn("failed to create baggage member", zap.Error(err))
        return ctx
    }

    bag, err := baggage.New(member)
    if err != nil {
        p.logger.Warn("failed to create baggage", zap.Error(err))
        return ctx
    }

    return baggage.ContextWithBaggage(ctx, bag)
}

// ParseBaggageHeader parses the W3C Baggage header string
// Format: correlation_id=olly-abc123,tenant_id=acme,user_id=12345
func ParseBaggageHeader(header string) map[string]string {
    result := make(map[string]string)

    parts := strings.Split(header, ",")
    for _, part := range parts {
        kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
        if len(kv) == 2 {
            result[kv[0]] = kv[1]
        }
    }

    return result
}
```

- [ ] Implement baggage extraction/injection
- [ ] Update middleware to use baggage
- [ ] Test cross-service propagation

#### Task 5.3.2: Update Frontend RUM for Baggage
**File:** `web-ui/src/lib/correlation.ts`

```typescript
import { context, propagation } from '@opentelemetry/api';

/**
 * W3C Baggage propagation for correlation ID.
 * This is the standard way to propagate custom context.
 */
export class CorrelationPropagator {
  private correlationId: string;

  constructor() {
    this.correlationId = this.getOrCreateCorrelationId();
  }

  private getOrCreateCorrelationId(): string {
    // Check session storage first
    let id = sessionStorage.getItem('olly_correlation_id');
    if (!id) {
      id = this.generate();
      sessionStorage.setItem('olly_correlation_id', id);
    }
    return id;
  }

  private generate(): string {
    const timestamp = Date.now().toString(36);
    const random = Math.random().toString(36).substring(2, 10);
    return `olly-${timestamp}-${random}`;
  }

  /**
   * Get headers to inject into fetch/XHR requests.
   * Uses W3C Baggage format.
   */
  getHeaders(): Record<string, string> {
    return {
      // W3C Baggage header (standard)
      'baggage': `correlation_id=${this.correlationId}`,
      // Also include custom header for backwards compatibility
      'X-Correlation-ID': this.correlationId,
    };
  }

  /**
   * Inject correlation into fetch options.
   */
  injectIntoFetch(options: RequestInit = {}): RequestInit {
    const headers = new Headers(options.headers);
    const correlationHeaders = this.getHeaders();

    Object.entries(correlationHeaders).forEach(([key, value]) => {
      headers.set(key, value);
    });

    return { ...options, headers };
  }

  getCorrelationId(): string {
    return this.correlationId;
  }

  /**
   * Start a new correlation (e.g., for a new user flow).
   */
  newCorrelation(): string {
    this.correlationId = this.generate();
    sessionStorage.setItem('olly_correlation_id', this.correlationId);
    return this.correlationId;
  }
}

// Singleton instance
export const correlation = new CorrelationPropagator();

// Patch fetch globally
const originalFetch = window.fetch;
window.fetch = function(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  return originalFetch(input, correlation.injectIntoFetch(init));
};
```

- [ ] Implement CorrelationPropagator class
- [ ] Patch fetch for automatic propagation
- [ ] Test with backend services

---

### 5.4 Security: Trace Context Validation

**Finding:** Malicious actors can forge trace headers to manipulate data or exploit vulnerabilities.

#### Task 5.4.1: Implement Context Validation
**File:** `otel-processor-correlation/security.go`

```go
package correlationprocessor

import (
    "regexp"
    "strings"
)

type SecurityConfig struct {
    // MaxCorrelationIDLength prevents DoS via huge IDs
    MaxCorrelationIDLength int `mapstructure:"max_correlation_id_length"`

    // TrustedPrefixes - only accept correlation IDs with these prefixes from external sources
    TrustedPrefixes []string `mapstructure:"trusted_prefixes"`

    // RejectExternalContext - ignore all incoming trace context from external sources
    RejectExternalContext bool `mapstructure:"reject_external_context"`

    // SanitizeInternalIDs - don't propagate internal IDs to external services
    SanitizeInternalIDs bool `mapstructure:"sanitize_internal_ids"`

    // ExternalEndpoints - list of external endpoint patterns
    ExternalEndpoints []string `mapstructure:"external_endpoints"`
}

func DefaultSecurityConfig() SecurityConfig {
    return SecurityConfig{
        MaxCorrelationIDLength: 100,
        TrustedPrefixes:        []string{"olly-"},
        RejectExternalContext:  false,
        SanitizeInternalIDs:    true,
        ExternalEndpoints:      []string{"*.external.com", "api.thirdparty.*"},
    }
}

type ContextValidator struct {
    config SecurityConfig
    logger *zap.Logger
}

func (v *ContextValidator) ValidateCorrelationID(id string, source string) (string, error) {
    // Check length
    if len(id) > v.config.MaxCorrelationIDLength {
        v.logger.Warn("correlation ID exceeds max length",
            zap.String("id", id[:50]+"..."),
            zap.String("source", source))
        return "", ErrCorrelationIDTooLong
    }

    // Check for injection attempts
    if v.containsInjection(id) {
        v.logger.Warn("potential injection in correlation ID",
            zap.String("id", id),
            zap.String("source", source))
        return "", ErrPotentialInjection
    }

    // Validate format if from external source
    if source == "external" && !v.isTrustedPrefix(id) {
        // Sanitize but accept external IDs
        return v.sanitize(id), nil
    }

    // Validate OllyStack format
    if strings.HasPrefix(id, "olly-") {
        if !v.isValidOllyFormat(id) {
            v.logger.Warn("invalid OllyStack correlation ID format",
                zap.String("id", id))
            return "", ErrInvalidFormat
        }
    }

    return id, nil
}

func (v *ContextValidator) containsInjection(id string) bool {
    // Check for common injection patterns
    patterns := []string{
        `<script`,           // XSS
        `javascript:`,       // XSS
        `\x00`,              // Null byte
        `; DROP`,            // SQL injection
        `' OR '1'='1`,       // SQL injection
        `../`,               // Path traversal
    }

    lower := strings.ToLower(id)
    for _, pattern := range patterns {
        if strings.Contains(lower, pattern) {
            return true
        }
    }
    return false
}

func (v *ContextValidator) isValidOllyFormat(id string) bool {
    // Format: olly-{timestamp_base36}-{random_8hex}
    // Example: olly-2k8f9x3-a7b2c4d1
    pattern := regexp.MustCompile(`^olly-[a-z0-9]{6,10}-[a-f0-9]{8}$`)
    return pattern.MatchString(id)
}

func (v *ContextValidator) sanitize(id string) string {
    // Remove any non-alphanumeric characters except dash and underscore
    re := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
    return re.ReplaceAllString(id, "")
}

func (v *ContextValidator) ShouldPropagateToExternal(endpoint string, id string) bool {
    if !v.config.SanitizeInternalIDs {
        return true
    }

    // Don't propagate internal trace context to external services
    for _, pattern := range v.config.ExternalEndpoints {
        if matchEndpoint(endpoint, pattern) {
            v.logger.Debug("not propagating internal ID to external endpoint",
                zap.String("endpoint", endpoint))
            return false
        }
    }
    return true
}

var (
    ErrCorrelationIDTooLong = errors.New("correlation ID exceeds maximum length")
    ErrPotentialInjection   = errors.New("potential injection attempt detected")
    ErrInvalidFormat        = errors.New("invalid correlation ID format")
)
```

- [ ] Implement ContextValidator
- [ ] Add injection detection
- [ ] Add format validation
- [ ] Add external endpoint detection
- [ ] Write security tests

---

### 5.5 Metric Exemplars Implementation

**Finding:** Exemplars link metrics to traces - configure SDK properly for automatic attachment.

#### Task 5.5.1: Configure Exemplar Storage in ClickHouse
**File:** `migrations/005_exemplars_table.sql`

```sql
-- Exemplars table for metric-to-trace correlation
CREATE TABLE IF NOT EXISTS ollystack.metric_exemplars (
    Timestamp DateTime64(9) CODEC(Delta, ZSTD(1)),
    MetricName LowCardinality(String),
    MetricType LowCardinality(String),  -- gauge, counter, histogram
    BucketBound Float64,                 -- For histogram buckets
    Value Float64,
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    CorrelationId String CODEC(ZSTD(1)),
    ServiceName LowCardinality(String),
    Labels Map(String, String) CODEC(ZSTD(1)),

    INDEX idx_trace_id TraceId TYPE bloom_filter GRANULARITY 3,
    INDEX idx_correlation_id CorrelationId TYPE bloom_filter GRANULARITY 3,
    INDEX idx_metric_name MetricName TYPE set(100) GRANULARITY 3
)
ENGINE = MergeTree()
PARTITION BY toYYYYMMDD(Timestamp)
ORDER BY (MetricName, ServiceName, Timestamp)
TTL Timestamp + INTERVAL 30 DAY
SETTINGS index_granularity = 8192;

-- View to join exemplars with traces
CREATE VIEW IF NOT EXISTS ollystack.exemplar_traces AS
SELECT
    e.Timestamp as ExemplarTime,
    e.MetricName,
    e.Value,
    e.BucketBound,
    e.CorrelationId,
    t.TraceId,
    t.SpanId,
    t.ServiceName,
    t.SpanName,
    t.Duration,
    t.StatusCode
FROM ollystack.metric_exemplars e
LEFT JOIN ollystack.traces t
    ON e.TraceId = t.TraceId
    AND e.SpanId = t.SpanId;
```

- [ ] Create exemplars table
- [ ] Create joined view
- [ ] Update ClickHouse exporter config

#### Task 5.5.2: Add Exemplar Query Endpoint
**File:** `api-server/internal/handlers/exemplars.go`

```go
package handlers

import (
    "net/http"
    "time"

    "github.com/gin-gonic/gin"
)

type ExemplarResponse struct {
    MetricName    string            `json:"metric_name"`
    Value         float64           `json:"value"`
    Timestamp     time.Time         `json:"timestamp"`
    TraceID       string            `json:"trace_id"`
    SpanID        string            `json:"span_id"`
    CorrelationID string            `json:"correlation_id"`
    ServiceName   string            `json:"service_name"`
    Labels        map[string]string `json:"labels"`
}

// GetExemplarsForMetric returns exemplars (trace links) for a metric
// GET /api/v1/metrics/{metric_name}/exemplars?start=...&end=...
func (h *Handler) GetExemplarsForMetric(c *gin.Context) {
    metricName := c.Param("metric_name")
    start := c.Query("start")
    end := c.Query("end")
    service := c.Query("service")

    query := `
        SELECT
            Timestamp,
            MetricName,
            Value,
            TraceId,
            SpanId,
            CorrelationId,
            ServiceName,
            Labels
        FROM ollystack.metric_exemplars
        WHERE MetricName = $1
            AND Timestamp >= parseDateTimeBestEffort($2)
            AND Timestamp <= parseDateTimeBestEffort($3)
    `
    args := []interface{}{metricName, start, end}

    if service != "" {
        query += " AND ServiceName = $4"
        args = append(args, service)
    }

    query += " ORDER BY Timestamp DESC LIMIT 100"

    rows, err := h.db.Query(c.Request.Context(), query, args...)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    defer rows.Close()

    var exemplars []ExemplarResponse
    for rows.Next() {
        var e ExemplarResponse
        if err := rows.ScanStruct(&e); err != nil {
            continue
        }
        exemplars = append(exemplars, e)
    }

    c.JSON(http.StatusOK, gin.H{
        "metric_name": metricName,
        "exemplars":   exemplars,
        "count":       len(exemplars),
    })
}

// GetTraceFromExemplar returns the full trace for an exemplar
// GET /api/v1/exemplars/{trace_id}/trace
func (h *Handler) GetTraceFromExemplar(c *gin.Context) {
    traceID := c.Param("trace_id")

    // Redirect to correlation endpoint if correlation_id available
    correlationID, err := h.getCorrelationIDForTrace(c.Request.Context(), traceID)
    if err == nil && correlationID != "" {
        c.Redirect(http.StatusTemporaryRedirect,
            fmt.Sprintf("/api/v1/correlate/%s", correlationID))
        return
    }

    // Otherwise return just the trace
    c.Redirect(http.StatusTemporaryRedirect,
        fmt.Sprintf("/api/v1/traces/%s", traceID))
}
```

- [ ] Create exemplars endpoint
- [ ] Add to API routes
- [ ] Create Grafana panel for exemplar visualization

---

### 5.6 Profiling Integration (Future Phase)

**Finding:** OTel profiling is now stable - enables CPU/memory correlation with traces.

#### Task 5.6.1: Add Profiling Pipeline (Future)
**File:** `deploy/kubernetes/otel-collector/profiling-config.yaml`

```yaml
# Future: OTel Collector profiling pipeline
# Note: Profiling support stabilized in OTel 2024-2025

receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

  # Profiling receiver (when stable)
  # pyroscope:
  #   endpoint: 0.0.0.0:4040

processors:
  batch:
    timeout: 10s

  # Link profiles to traces
  # profile_correlation:
  #   trace_id_attribute: true
  #   correlation_id_attribute: true

exporters:
  # Future: Profiling exporter
  # clickhouse/profiles:
  #   endpoint: tcp://clickhouse:9000
  #   database: ollystack
  #   profiles_table_name: profiles

service:
  pipelines:
    # Future profiling pipeline
    # profiles:
    #   receivers: [pyroscope]
    #   processors: [profile_correlation, batch]
    #   exporters: [clickhouse/profiles]
```

- [ ] Document profiling architecture
- [ ] Reserve schema for profiles table
- [ ] Plan integration with Grafana Pyroscope

---

## Sprint 6: Production Hardening (Enhanced)

### 6.1 High-Cardinality Protection

#### Task 6.1.1: Add Cardinality Limiter
**File:** `otel-processor-correlation/cardinality.go`

```go
package correlationprocessor

import (
    "sync"
    "time"
)

type CardinalityLimiter struct {
    maxUniqueValues int
    window          time.Duration
    values          map[string]map[string]time.Time  // attribute -> value -> last_seen
    mu              sync.RWMutex
}

func NewCardinalityLimiter(maxValues int, window time.Duration) *CardinalityLimiter {
    limiter := &CardinalityLimiter{
        maxUniqueValues: maxValues,
        window:          window,
        values:          make(map[string]map[string]time.Time),
    }

    // Start cleanup goroutine
    go limiter.cleanup()

    return limiter
}

func (l *CardinalityLimiter) Check(attribute string, value string) bool {
    l.mu.Lock()
    defer l.mu.Unlock()

    if _, ok := l.values[attribute]; !ok {
        l.values[attribute] = make(map[string]time.Time)
    }

    // Check if value exists
    if _, exists := l.values[attribute][value]; exists {
        l.values[attribute][value] = time.Now()
        return true
    }

    // Check cardinality limit
    if len(l.values[attribute]) >= l.maxUniqueValues {
        return false  // Reject new value
    }

    l.values[attribute][value] = time.Now()
    return true
}

func (l *CardinalityLimiter) cleanup() {
    ticker := time.NewTicker(l.window / 10)
    for range ticker.C {
        l.mu.Lock()
        cutoff := time.Now().Add(-l.window)
        for attr, values := range l.values {
            for val, lastSeen := range values {
                if lastSeen.Before(cutoff) {
                    delete(values, val)
                }
            }
            if len(values) == 0 {
                delete(l.values, attr)
            }
        }
        l.mu.Unlock()
    }
}
```

- [ ] Implement cardinality limiter
- [ ] Add to processor pipeline
- [ ] Create metrics for dropped high-cardinality values

---

## Quick Reference

### Key Files

| Component | Path |
|-----------|------|
| OTel Correlation Processor | `otel-processor-correlation/` |
| Correlation Engine | `api-server/internal/correlation/` |
| AI Engine | `ai-engine/ollystack/` |
| Causal RCA | `ai-engine/ollystack/rca/causal_graph.py` |
| Semantic Conventions | `api-server/internal/semconv/` |
| Grafana Config | `deploy/kubernetes/grafana/` |
| Vector Config | `deploy/kubernetes/vector/` |
| Alertmanager Config | `deploy/kubernetes/alertmanager/` |

### New Dependencies

```toml
# ai-engine/pyproject.toml - add:
[project.dependencies]
dowhy = "^0.11"           # Causal inference
networkx = "^3.2"         # Graph algorithms
numpy = "^1.26"
scipy = "^1.12"
```

### Commands

```bash
# Build custom OTel Collector
cd otel-processor-correlation
ocb --config builder-config.yaml

# Run API server
cd api-server && go run cmd/server/main.go

# Run AI engine
cd ai-engine && python -m ollystack.main

# Deploy all with Helm
helm upgrade --install ollystack ./deploy/kubernetes/ollystack -f values.yaml

# Test correlation lookup
curl http://localhost:8080/api/v1/correlate/olly-test-12345678

# Test causal RCA
curl -X POST http://localhost:8080/api/v1/ai/rca \
  -H "Content-Type: application/json" \
  -d '{"correlation_id": "olly-test-12345678"}'
```

---

*Last updated: 2026-02-03 - Integrated Architecture v2.1 with Research Improvements*

| Component | Path |
|-----------|------|
| OTel Correlation Processor | `otel-processor-correlation/` |
| Correlation Engine | `api-server/internal/correlation/` |
| AI Engine | `ai-engine/ollystack/` |
| Grafana Config | `deploy/kubernetes/grafana/` |
| Vector Config | `deploy/kubernetes/vector/` |
| Alertmanager Config | `deploy/kubernetes/alertmanager/` |

### Commands

```bash
# Build custom OTel Collector
cd otel-processor-correlation
ocb --config builder-config.yaml

# Run API server
cd api-server && go run cmd/server/main.go

# Run AI engine
cd ai-engine && python -m ollystack.main

# Deploy all with Helm
helm upgrade --install ollystack ./deploy/kubernetes/ollystack -f values.yaml

# Test correlation lookup
curl http://localhost:8080/api/v1/correlate/olly-test-12345678
```

---

*Last updated: 2026-02-03 - Integrated Architecture v2.0*
