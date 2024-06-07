//! Output sinks for processed data.

use anyhow::Result;
use tokio::sync::broadcast;

use crate::config::{StorageSinkConfig, AlertSinkConfig};
use crate::engine::{TelemetryData, Alert};

/// Storage sink for writing to ClickHouse or other storage backends.
pub struct StorageSink {
    config: StorageSinkConfig,
    // In production, this would hold a ClickHouse client
}

impl StorageSink {
    pub fn new(config: &StorageSinkConfig) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
        })
    }

    pub async fn write(&self, data: &TelemetryData) -> Result<()> {
        match data {
            TelemetryData::Traces(spans) => {
                self.write_traces(spans).await?;
            }
            TelemetryData::Metrics(metrics) => {
                self.write_metrics(metrics).await?;
            }
            TelemetryData::Logs(logs) => {
                self.write_logs(logs).await?;
            }
        }
        Ok(())
    }

    async fn write_traces(&self, spans: &[crate::engine::Span]) -> Result<()> {
        // In production, this would insert into ClickHouse:
        // INSERT INTO traces (trace_id, span_id, ...) VALUES (?, ?, ...)
        tracing::debug!(count = spans.len(), "Writing traces to storage");
        Ok(())
    }

    async fn write_metrics(&self, metrics: &[crate::engine::Metric]) -> Result<()> {
        tracing::debug!(count = metrics.len(), "Writing metrics to storage");
        Ok(())
    }

    async fn write_logs(&self, logs: &[crate::engine::LogRecord]) -> Result<()> {
        tracing::debug!(count = logs.len(), "Writing logs to storage");
        Ok(())
    }
}

/// Alert sink for sending alerts to external systems.
pub struct AlertSink {
    config: AlertSinkConfig,
    alert_rx: broadcast::Receiver<Alert>,
}

impl AlertSink {
    pub fn new(config: &AlertSinkConfig, alert_tx: broadcast::Sender<Alert>) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
            alert_rx: alert_tx.subscribe(),
        })
    }

    pub async fn run(&mut self) -> Result<()> {
        while let Ok(alert) = self.alert_rx.recv().await {
            self.send_alert(&alert).await?;
        }
        Ok(())
    }

    async fn send_alert(&self, alert: &Alert) -> Result<()> {
        tracing::info!(
            id = %alert.id,
            name = %alert.name,
            severity = ?alert.severity,
            "Sending alert"
        );

        // Send to webhook if configured
        if let Some(webhook_url) = &self.config.webhook_url {
            self.send_webhook(webhook_url, alert).await?;
        }

        // Send to Slack if configured
        if let Some(slack_webhook) = &self.config.slack_webhook {
            self.send_slack(slack_webhook, alert).await?;
        }

        Ok(())
    }

    async fn send_webhook(&self, url: &str, alert: &Alert) -> Result<()> {
        // HTTP POST to webhook URL
        tracing::debug!(url = %url, "Sending alert to webhook");
        Ok(())
    }

    async fn send_slack(&self, webhook: &str, alert: &Alert) -> Result<()> {
        // Format and send to Slack
        let message = format!(
            ":{}:  *{}*\n{}",
            match alert.severity {
                crate::engine::AlertSeverity::Critical => "red_circle",
                crate::engine::AlertSeverity::Warning => "large_orange_circle",
                crate::engine::AlertSeverity::Info => "large_blue_circle",
            },
            alert.name,
            alert.message
        );

        tracing::debug!(webhook = %webhook, message = %message, "Sending alert to Slack");
        Ok(())
    }
}
