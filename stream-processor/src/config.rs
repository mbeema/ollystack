//! Configuration for the stream processor.

use anyhow::Result;
use serde::Deserialize;
use std::path::Path;

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    pub sources: SourcesConfig,
    pub analytics: AnalyticsConfig,
    pub sinks: SinksConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SourcesConfig {
    pub otlp: OtlpSourceConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct OtlpSourceConfig {
    pub endpoint: String,
    #[serde(default = "default_port")]
    pub port: u16,
}

fn default_port() -> u16 {
    4317
}

#[derive(Debug, Clone, Deserialize)]
pub struct AnalyticsConfig {
    pub anomaly_detection: AnomalyDetectionConfig,
    pub trace_correlation: TraceCorrelationConfig,
    pub topology_discovery: TopologyDiscoveryConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct AnomalyDetectionConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_algorithm")]
    pub algorithm: String,
    #[serde(default = "default_sensitivity")]
    pub sensitivity: f64,
}

fn default_true() -> bool { true }
fn default_algorithm() -> String { "isolation_forest".to_string() }
fn default_sensitivity() -> f64 { 0.95 }

#[derive(Debug, Clone, Deserialize)]
pub struct TraceCorrelationConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_window")]
    pub window_seconds: u64,
}

fn default_window() -> u64 { 300 }

#[derive(Debug, Clone, Deserialize)]
pub struct TopologyDiscoveryConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default = "default_update_interval")]
    pub update_interval_seconds: u64,
}

fn default_update_interval() -> u64 { 10 }

#[derive(Debug, Clone, Deserialize)]
pub struct SinksConfig {
    pub storage: StorageSinkConfig,
    pub alerts: AlertSinkConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct StorageSinkConfig {
    #[serde(default = "default_storage_type")]
    pub storage_type: String,
    pub endpoint: String,
    pub database: String,
    #[serde(default)]
    pub username: Option<String>,
    #[serde(default)]
    pub password: Option<String>,
}

fn default_storage_type() -> String { "clickhouse".to_string() }

#[derive(Debug, Clone, Deserialize)]
pub struct AlertSinkConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default)]
    pub webhook_url: Option<String>,
    #[serde(default)]
    pub slack_webhook: Option<String>,
}

impl Config {
    pub fn load(path: &Path) -> Result<Self> {
        let builder = config::Config::builder()
            .add_source(config::File::from(path).required(false))
            .add_source(config::Environment::with_prefix("OLLYSTACK").separator("_"))
            .set_default("sources.otlp.endpoint", "0.0.0.0")?
            .set_default("sources.otlp.port", 4317)?
            .set_default("analytics.anomaly_detection.enabled", true)?
            .set_default("analytics.trace_correlation.enabled", true)?
            .set_default("analytics.topology_discovery.enabled", true)?
            .set_default("sinks.storage.storage_type", "clickhouse")?
            .set_default("sinks.storage.endpoint", "localhost:9000")?
            .set_default("sinks.storage.database", "ollystack")?
            .set_default("sinks.alerts.enabled", true)?;

        let config = builder.build()?;
        Ok(config.try_deserialize()?)
    }
}
