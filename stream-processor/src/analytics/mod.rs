//! Analytics components for real-time data analysis.

use anyhow::Result;
use std::collections::HashMap;

use crate::config::{AnomalyDetectionConfig, TraceCorrelationConfig, TopologyDiscoveryConfig};
use crate::engine::{Alert, AlertSeverity, Span, Metric};

/// Anomaly detection using statistical and ML methods.
pub struct AnomalyDetector {
    config: AnomalyDetectionConfig,
    baseline_metrics: HashMap<String, MetricBaseline>,
}

struct MetricBaseline {
    mean: f64,
    std_dev: f64,
    count: u64,
}

impl AnomalyDetector {
    pub fn new(config: &AnomalyDetectionConfig) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
            baseline_metrics: HashMap::new(),
        })
    }

    pub fn check_traces(&self, spans: &[Span]) -> Result<Option<Alert>> {
        // Check for error rates
        let error_count = spans.iter()
            .filter(|s| matches!(s.status, crate::engine::SpanStatus::Error(_)))
            .count();

        let error_rate = error_count as f64 / spans.len() as f64;

        if error_rate > 0.1 {
            return Ok(Some(Alert {
                id: uuid::Uuid::new_v4().to_string(),
                name: "High Error Rate".to_string(),
                severity: AlertSeverity::Critical,
                message: format!("Error rate is {:.1}%", error_rate * 100.0),
                timestamp: chrono::Utc::now().timestamp_millis(),
                labels: HashMap::new(),
            }));
        }

        // Check for high latency
        let avg_latency: f64 = spans.iter()
            .map(|s| (s.end_time - s.start_time) as f64)
            .sum::<f64>() / spans.len() as f64;

        if avg_latency > 1_000_000_000.0 { // 1 second in nanoseconds
            return Ok(Some(Alert {
                id: uuid::Uuid::new_v4().to_string(),
                name: "High Latency".to_string(),
                severity: AlertSeverity::Warning,
                message: format!("Average latency is {:.0}ms", avg_latency / 1_000_000.0),
                timestamp: chrono::Utc::now().timestamp_millis(),
                labels: HashMap::new(),
            }));
        }

        Ok(None)
    }

    pub fn check_metrics(&self, metrics: &[Metric]) -> Result<Option<Alert>> {
        // Z-score based anomaly detection
        for metric in metrics {
            if let Some(baseline) = self.baseline_metrics.get(&metric.name) {
                let z_score = (metric.value - baseline.mean) / baseline.std_dev;
                if z_score.abs() > 3.0 {
                    return Ok(Some(Alert {
                        id: uuid::Uuid::new_v4().to_string(),
                        name: format!("Anomaly in {}", metric.name),
                        severity: AlertSeverity::Warning,
                        message: format!(
                            "Value {} deviates significantly from baseline (z-score: {:.2})",
                            metric.value, z_score
                        ),
                        timestamp: chrono::Utc::now().timestamp_millis(),
                        labels: metric.labels.clone(),
                    }));
                }
            }
        }

        Ok(None)
    }
}

/// Trace correlation for building complete traces from spans.
pub struct TraceCorrelator {
    config: TraceCorrelationConfig,
    traces: HashMap<String, Vec<Span>>,
}

impl TraceCorrelator {
    pub fn new(config: &TraceCorrelationConfig) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
            traces: HashMap::new(),
        })
    }

    pub fn process(&mut self, spans: &[Span]) -> Result<()> {
        for span in spans {
            self.traces
                .entry(span.trace_id.clone())
                .or_insert_with(Vec::new)
                .push(span.clone());
        }

        // Clean up old traces
        self.cleanup_old_traces();

        Ok(())
    }

    fn cleanup_old_traces(&mut self) {
        let cutoff = chrono::Utc::now().timestamp_millis() -
            (self.config.window_seconds * 1000) as i64;

        self.traces.retain(|_, spans| {
            spans.iter().any(|s| s.end_time > cutoff)
        });
    }

    pub fn get_trace(&self, trace_id: &str) -> Option<&Vec<Span>> {
        self.traces.get(trace_id)
    }
}

/// Topology discovery for building service dependency maps.
pub struct TopologyDiscovery {
    config: TopologyDiscoveryConfig,
    services: HashMap<String, ServiceInfo>,
    edges: HashMap<String, EdgeInfo>,
}

#[derive(Debug, Clone)]
pub struct ServiceInfo {
    pub name: String,
    pub span_count: u64,
    pub error_count: u64,
    pub total_latency_ns: u64,
}

#[derive(Debug, Clone)]
pub struct EdgeInfo {
    pub source: String,
    pub target: String,
    pub request_count: u64,
    pub error_count: u64,
}

impl TopologyDiscovery {
    pub fn new(config: &TopologyDiscoveryConfig) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
            services: HashMap::new(),
            edges: HashMap::new(),
        })
    }

    pub fn process(&mut self, spans: &[Span]) -> Result<()> {
        for span in spans {
            // Update service info
            let service = self.services
                .entry(span.service_name.clone())
                .or_insert_with(|| ServiceInfo {
                    name: span.service_name.clone(),
                    span_count: 0,
                    error_count: 0,
                    total_latency_ns: 0,
                });

            service.span_count += 1;
            service.total_latency_ns += (span.end_time - span.start_time) as u64;

            if matches!(span.status, crate::engine::SpanStatus::Error(_)) {
                service.error_count += 1;
            }

            // Update edges (if we can determine the caller)
            if let Some(peer_service) = span.attributes.get("peer.service") {
                let edge_key = format!("{}:{}", span.service_name, peer_service);
                let edge = self.edges
                    .entry(edge_key)
                    .or_insert_with(|| EdgeInfo {
                        source: span.service_name.clone(),
                        target: peer_service.clone(),
                        request_count: 0,
                        error_count: 0,
                    });

                edge.request_count += 1;
                if matches!(span.status, crate::engine::SpanStatus::Error(_)) {
                    edge.error_count += 1;
                }
            }
        }

        Ok(())
    }

    pub fn get_services(&self) -> &HashMap<String, ServiceInfo> {
        &self.services
    }

    pub fn get_edges(&self) -> &HashMap<String, EdgeInfo> {
        &self.edges
    }
}
