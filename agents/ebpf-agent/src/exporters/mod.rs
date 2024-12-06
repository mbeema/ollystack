//! OTLP exporter for sending telemetry data.

use anyhow::Result;
use opentelemetry::trace::{TraceError, TracerProvider};
use opentelemetry_sdk::{
    trace::{self, Tracer},
    Resource,
};
use opentelemetry_otlp::WithExportConfig;
use std::time::Duration;
use tonic::metadata::MetadataMap;
use tracing::debug;

use crate::config::CollectorConfig;
use crate::probes::ProbeEvent;

/// OTLP exporter for traces and metrics.
pub struct OtlpExporter {
    tracer: Tracer,
}

impl OtlpExporter {
    pub async fn new(config: &CollectorConfig) -> Result<Self> {
        let mut metadata = MetadataMap::new();
        for (key, value) in &config.headers {
            if let Ok(key) = key.parse() {
                if let Ok(value) = value.parse() {
                    metadata.insert(key, value);
                }
            }
        }

        let exporter = opentelemetry_otlp::new_exporter()
            .tonic()
            .with_endpoint(&config.endpoint)
            .with_timeout(Duration::from_secs(10));

        let tracer_provider = opentelemetry_otlp::new_pipeline()
            .tracing()
            .with_exporter(exporter)
            .with_trace_config(
                trace::config().with_resource(Resource::new(vec![
                    opentelemetry::KeyValue::new("service.name", "ollystack-ebpf-agent"),
                    opentelemetry::KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
                ])),
            )
            .install_batch(opentelemetry_sdk::runtime::Tokio)?;

        let tracer = tracer_provider.tracer("ollystack-ebpf-agent");

        Ok(Self { tracer })
    }

    /// Export a probe event as telemetry.
    pub async fn export_event(&self, event: &ProbeEvent) -> Result<()> {
        use opentelemetry::trace::Tracer;

        match event {
            ProbeEvent::Network(e) => {
                debug!(
                    pid = e.pid,
                    src = %e.src_addr,
                    dst = %e.dst_addr,
                    protocol = %e.protocol,
                    "Network event"
                );

                // Create span for network event
                let _span = self.tracer.start(format!("{} {}", e.protocol, e.path.as_deref().unwrap_or("/")));
            }
            ProbeEvent::Syscall(e) => {
                debug!(
                    pid = e.pid,
                    syscall = %e.syscall,
                    latency_ns = e.latency_ns,
                    "Syscall event"
                );
            }
            ProbeEvent::Runtime(e) => {
                debug!(
                    pid = e.pid,
                    event_type = %e.event_type,
                    "Runtime event"
                );
            }
        }

        Ok(())
    }
}
