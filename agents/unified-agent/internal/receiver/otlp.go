// Package receiver handles incoming telemetry from applications
package receiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/ollystack/unified-agent/internal/pipeline"
	"github.com/ollystack/unified-agent/internal/types"
)

// OTLPConfig configures the OTLP receiver
type OTLPConfig struct {
	GRPCPort int
	HTTPPort int
}

// OTLPReceiver receives OTLP telemetry from applications
type OTLPReceiver struct {
	config   OTLPConfig
	pipeline *pipeline.Pipeline
	logger   *zap.Logger

	grpcServer *grpc.Server
	httpServer *http.Server
}

// NewOTLPReceiver creates a new OTLP receiver
func NewOTLPReceiver(cfg OTLPConfig, p *pipeline.Pipeline, logger *zap.Logger) (*OTLPReceiver, error) {
	return &OTLPReceiver{
		config:   cfg,
		pipeline: p,
		logger:   logger,
	}, nil
}

// Start begins receiving OTLP data
func (r *OTLPReceiver) Start(ctx context.Context) error {
	errCh := make(chan error, 2)

	// Start gRPC server
	if r.config.GRPCPort > 0 {
		go func() {
			if err := r.startGRPC(ctx); err != nil {
				errCh <- fmt.Errorf("gRPC server: %w", err)
			}
		}()
	}

	// Start HTTP server
	if r.config.HTTPPort > 0 {
		go func() {
			if err := r.startHTTP(ctx); err != nil {
				errCh <- fmt.Errorf("HTTP server: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		r.shutdown()
		return nil
	case err := <-errCh:
		return err
	}
}

func (r *OTLPReceiver) startGRPC(ctx context.Context) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", r.config.GRPCPort))
	if err != nil {
		return err
	}

	r.grpcServer = grpc.NewServer()

	// Register OTLP services
	// Note: In production, you would use the official OTLP protobuf definitions
	// For simplicity, we'll handle JSON over HTTP and implement gRPC stubs

	r.logger.Info("OTLP gRPC receiver started", zap.Int("port", r.config.GRPCPort))

	go func() {
		<-ctx.Done()
		r.grpcServer.GracefulStop()
	}()

	return r.grpcServer.Serve(lis)
}

func (r *OTLPReceiver) startHTTP(ctx context.Context) error {
	mux := http.NewServeMux()

	// OTLP HTTP endpoints
	mux.HandleFunc("/v1/traces", r.handleTraces)
	mux.HandleFunc("/v1/metrics", r.handleMetrics)
	mux.HandleFunc("/v1/logs", r.handleLogs)

	r.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", r.config.HTTPPort),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	r.logger.Info("OTLP HTTP receiver started", zap.Int("port", r.config.HTTPPort))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r.httpServer.Shutdown(shutdownCtx)
	}()

	if err := r.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (r *OTLPReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Parse OTLP traces
	spans, err := r.parseOTLPTraces(body)
	if err != nil {
		r.logger.Debug("Failed to parse traces", zap.Error(err))
		http.Error(w, "Invalid traces", http.StatusBadRequest)
		return
	}

	// Process spans
	for _, span := range spans {
		r.pipeline.ProcessTrace(span)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}

func (r *OTLPReceiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	metrics, err := r.parseOTLPMetrics(body)
	if err != nil {
		r.logger.Debug("Failed to parse metrics", zap.Error(err))
		http.Error(w, "Invalid metrics", http.StatusBadRequest)
		return
	}

	for _, m := range metrics {
		r.pipeline.ProcessMetric(m)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}

func (r *OTLPReceiver) handleLogs(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, 10*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	logs, err := r.parseOTLPLogs(body)
	if err != nil {
		r.logger.Debug("Failed to parse logs", zap.Error(err))
		http.Error(w, "Invalid logs", http.StatusBadRequest)
		return
	}

	for _, l := range logs {
		r.pipeline.ProcessLog(l)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{}`))
}

// OTLP JSON parsing (simplified)

func (r *OTLPReceiver) parseOTLPTraces(body []byte) ([]types.Span, error) {
	var payload struct {
		ResourceSpans []struct {
			Resource struct {
				Attributes []struct {
					Key   string `json:"key"`
					Value struct {
						StringValue string `json:"stringValue"`
					} `json:"value"`
				} `json:"attributes"`
			} `json:"resource"`
			ScopeSpans []struct {
				Spans []struct {
					TraceID           string `json:"traceId"`
					SpanID            string `json:"spanId"`
					ParentSpanID      string `json:"parentSpanId"`
					Name              string `json:"name"`
					Kind              int    `json:"kind"`
					StartTimeUnixNano int64  `json:"startTimeUnixNano"`
					EndTimeUnixNano   int64  `json:"endTimeUnixNano"`
					Attributes        []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
							IntValue    int64  `json:"intValue"`
						} `json:"value"`
					} `json:"attributes"`
					Status struct {
						Code    int    `json:"code"`
						Message string `json:"message"`
					} `json:"status"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var spans []types.Span

	for _, rs := range payload.ResourceSpans {
		// Extract service name from resource
		serviceName := ""
		for _, attr := range rs.Resource.Attributes {
			if attr.Key == "service.name" {
				serviceName = attr.Value.StringValue
				break
			}
		}

		for _, ss := range rs.ScopeSpans {
			for _, s := range ss.Spans {
				attrs := make(map[string]string)
				for _, a := range s.Attributes {
					if a.Value.StringValue != "" {
						attrs[a.Key] = a.Value.StringValue
					} else {
						attrs[a.Key] = fmt.Sprintf("%d", a.Value.IntValue)
					}
				}

				startTime := time.Unix(0, s.StartTimeUnixNano)
				endTime := time.Unix(0, s.EndTimeUnixNano)

				spans = append(spans, types.Span{
					TraceID:       s.TraceID,
					SpanID:        s.SpanID,
					ParentSpanID:  s.ParentSpanID,
					Name:          s.Name,
					Kind:          types.SpanKind(s.Kind),
					StartTime:     startTime,
					EndTime:       endTime,
					Duration:      endTime.Sub(startTime),
					Status:        types.SpanStatus(s.Status.Code),
					StatusMessage: s.Status.Message,
					Service:       serviceName,
					Attributes:    attrs,
				})
			}
		}
	}

	return spans, nil
}

func (r *OTLPReceiver) parseOTLPMetrics(body []byte) ([]types.Metric, error) {
	var payload struct {
		ResourceMetrics []struct {
			ScopeMetrics []struct {
				Metrics []struct {
					Name  string `json:"name"`
					Unit  string `json:"unit"`
					Gauge *struct {
						DataPoints []struct {
							TimeUnixNano int64   `json:"timeUnixNano"`
							AsDouble     float64 `json:"asDouble"`
							Attributes   []struct {
								Key   string `json:"key"`
								Value struct {
									StringValue string `json:"stringValue"`
								} `json:"value"`
							} `json:"attributes"`
						} `json:"dataPoints"`
					} `json:"gauge"`
					Sum *struct {
						DataPoints []struct {
							TimeUnixNano int64   `json:"timeUnixNano"`
							AsDouble     float64 `json:"asDouble"`
							Attributes   []struct {
								Key   string `json:"key"`
								Value struct {
									StringValue string `json:"stringValue"`
								} `json:"value"`
							} `json:"attributes"`
						} `json:"dataPoints"`
					} `json:"sum"`
				} `json:"metrics"`
			} `json:"scopeMetrics"`
		} `json:"resourceMetrics"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var metrics []types.Metric

	for _, rm := range payload.ResourceMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				// Handle gauge
				if m.Gauge != nil {
					for _, dp := range m.Gauge.DataPoints {
						labels := make(map[string]string)
						for _, a := range dp.Attributes {
							labels[a.Key] = a.Value.StringValue
						}

						metrics = append(metrics, types.Metric{
							Name:      m.Name,
							Value:     dp.AsDouble,
							Timestamp: time.Unix(0, dp.TimeUnixNano),
							Labels:    labels,
							Type:      types.MetricTypeGauge,
							Unit:      m.Unit,
						})
					}
				}

				// Handle sum (counter)
				if m.Sum != nil {
					for _, dp := range m.Sum.DataPoints {
						labels := make(map[string]string)
						for _, a := range dp.Attributes {
							labels[a.Key] = a.Value.StringValue
						}

						metrics = append(metrics, types.Metric{
							Name:      m.Name,
							Value:     dp.AsDouble,
							Timestamp: time.Unix(0, dp.TimeUnixNano),
							Labels:    labels,
							Type:      types.MetricTypeCounter,
							Unit:      m.Unit,
						})
					}
				}
			}
		}
	}

	return metrics, nil
}

func (r *OTLPReceiver) parseOTLPLogs(body []byte) ([]types.LogRecord, error) {
	var payload struct {
		ResourceLogs []struct {
			ScopeLogs []struct {
				LogRecords []struct {
					TimeUnixNano   int64  `json:"timeUnixNano"`
					SeverityNumber int    `json:"severityNumber"`
					SeverityText   string `json:"severityText"`
					Body           struct {
						StringValue string `json:"stringValue"`
					} `json:"body"`
					TraceID    string `json:"traceId"`
					SpanID     string `json:"spanId"`
					Attributes []struct {
						Key   string `json:"key"`
						Value struct {
							StringValue string `json:"stringValue"`
						} `json:"value"`
					} `json:"attributes"`
				} `json:"logRecords"`
			} `json:"scopeLogs"`
		} `json:"resourceLogs"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	var logs []types.LogRecord

	for _, rl := range payload.ResourceLogs {
		for _, sl := range rl.ScopeLogs {
			for _, lr := range sl.LogRecords {
				attrs := make(map[string]string)
				for _, a := range lr.Attributes {
					attrs[a.Key] = a.Value.StringValue
				}

				logs = append(logs, types.LogRecord{
					Timestamp:  time.Unix(0, lr.TimeUnixNano),
					Body:       lr.Body.StringValue,
					Severity:   types.Severity(lr.SeverityNumber / 4), // OTLP uses 1-24, we use 0-5
					TraceID:    lr.TraceID,
					SpanID:     lr.SpanID,
					Attributes: attrs,
				})
			}
		}
	}

	return logs, nil
}

func (r *OTLPReceiver) shutdown() {
	if r.grpcServer != nil {
		r.grpcServer.GracefulStop()
	}
}
