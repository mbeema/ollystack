//! Stream processing engine.

use anyhow::Result;
use std::sync::Arc;
use tokio::sync::{mpsc, broadcast};
use tracing::{info, debug, warn};

use crate::config::Config;
use crate::operators::{Operator, OperatorChain};
use crate::analytics::{AnomalyDetector, TraceCorrelator, TopologyDiscovery};
use crate::sinks::{StorageSink, AlertSink};

/// Telemetry data types.
#[derive(Debug, Clone)]
pub enum TelemetryData {
    Traces(Vec<Span>),
    Metrics(Vec<Metric>),
    Logs(Vec<LogRecord>),
}

#[derive(Debug, Clone)]
pub struct Span {
    pub trace_id: String,
    pub span_id: String,
    pub parent_span_id: Option<String>,
    pub operation_name: String,
    pub service_name: String,
    pub start_time: i64,
    pub end_time: i64,
    pub status: SpanStatus,
    pub attributes: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone)]
pub enum SpanStatus {
    Ok,
    Error(String),
}

#[derive(Debug, Clone)]
pub struct Metric {
    pub name: String,
    pub value: f64,
    pub timestamp: i64,
    pub labels: std::collections::HashMap<String, String>,
    pub metric_type: MetricType,
}

#[derive(Debug, Clone)]
pub enum MetricType {
    Counter,
    Gauge,
    Histogram,
    Summary,
}

#[derive(Debug, Clone)]
pub struct LogRecord {
    pub timestamp: i64,
    pub severity: String,
    pub body: String,
    pub trace_id: Option<String>,
    pub span_id: Option<String>,
    pub attributes: std::collections::HashMap<String, String>,
}

/// The main stream processing engine.
pub struct StreamEngine {
    config: Config,
    data_tx: mpsc::Sender<TelemetryData>,
    data_rx: mpsc::Receiver<TelemetryData>,
    alert_tx: broadcast::Sender<Alert>,
    operators: Vec<Box<dyn Operator + Send + Sync>>,
    anomaly_detector: Option<AnomalyDetector>,
    trace_correlator: Option<TraceCorrelator>,
    topology_discovery: Option<TopologyDiscovery>,
    storage_sink: Option<StorageSink>,
    alert_sink: Option<AlertSink>,
}

#[derive(Debug, Clone)]
pub struct Alert {
    pub id: String,
    pub name: String,
    pub severity: AlertSeverity,
    pub message: String,
    pub timestamp: i64,
    pub labels: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone)]
pub enum AlertSeverity {
    Info,
    Warning,
    Critical,
}

impl StreamEngine {
    pub async fn new(config: Config) -> Result<Self> {
        let (data_tx, data_rx) = mpsc::channel(10000);
        let (alert_tx, _) = broadcast::channel(1000);

        Ok(Self {
            config,
            data_tx,
            data_rx,
            alert_tx,
            operators: Vec::new(),
            anomaly_detector: None,
            trace_correlator: None,
            topology_discovery: None,
            storage_sink: None,
            alert_sink: None,
        })
    }

    /// Build the processing pipeline based on configuration.
    pub fn build_pipeline(&mut self) -> Result<()> {
        // Initialize analytics components
        if self.config.analytics.anomaly_detection.enabled {
            self.anomaly_detector = Some(AnomalyDetector::new(
                &self.config.analytics.anomaly_detection,
            )?);
            info!("Anomaly detection enabled");
        }

        if self.config.analytics.trace_correlation.enabled {
            self.trace_correlator = Some(TraceCorrelator::new(
                &self.config.analytics.trace_correlation,
            )?);
            info!("Trace correlation enabled");
        }

        if self.config.analytics.topology_discovery.enabled {
            self.topology_discovery = Some(TopologyDiscovery::new(
                &self.config.analytics.topology_discovery,
            )?);
            info!("Topology discovery enabled");
        }

        // Initialize sinks
        self.storage_sink = Some(StorageSink::new(&self.config.sinks.storage)?);
        self.alert_sink = Some(AlertSink::new(
            &self.config.sinks.alerts,
            self.alert_tx.clone(),
        )?);

        Ok(())
    }

    /// Run the stream processing engine.
    pub async fn run(&mut self) -> Result<()> {
        // Start OTLP receiver
        let data_tx = self.data_tx.clone();
        tokio::spawn(async move {
            // In production, this would start a gRPC server to receive OTLP data
            // For now, we simulate data
        });

        // Process incoming data
        while let Some(data) = self.data_rx.recv().await {
            self.process_data(data).await?;
        }

        Ok(())
    }

    /// Process a batch of telemetry data.
    async fn process_data(&mut self, data: TelemetryData) -> Result<()> {
        match &data {
            TelemetryData::Traces(spans) => {
                debug!(span_count = spans.len(), "Processing traces");

                // Run trace correlation
                if let Some(correlator) = &mut self.trace_correlator {
                    correlator.process(spans)?;
                }

                // Update topology
                if let Some(topology) = &mut self.topology_discovery {
                    topology.process(spans)?;
                }

                // Check for anomalies
                if let Some(detector) = &self.anomaly_detector {
                    if let Some(alert) = detector.check_traces(spans)? {
                        self.alert_tx.send(alert)?;
                    }
                }
            }
            TelemetryData::Metrics(metrics) => {
                debug!(metric_count = metrics.len(), "Processing metrics");

                // Check for anomalies
                if let Some(detector) = &self.anomaly_detector {
                    if let Some(alert) = detector.check_metrics(metrics)? {
                        self.alert_tx.send(alert)?;
                    }
                }
            }
            TelemetryData::Logs(logs) => {
                debug!(log_count = logs.len(), "Processing logs");
            }
        }

        // Store data
        if let Some(sink) = &self.storage_sink {
            sink.write(&data).await?;
        }

        Ok(())
    }
}
