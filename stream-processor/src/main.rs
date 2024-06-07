//! OllyStack Stream Processor
//!
//! Real-time stream processing engine for telemetry data.
//! Provides:
//! - Anomaly detection
//! - Trace correlation
//! - Service topology discovery
//! - Metrics aggregation

use anyhow::Result;
use clap::Parser;
use std::path::PathBuf;
use tracing::info;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

mod config;
mod engine;
mod operators;
mod analytics;
mod sinks;

use config::Config;
use engine::StreamEngine;

#[derive(Parser, Debug)]
#[command(name = "ollystack-stream-processor")]
#[command(about = "Real-time stream processing for telemetry data")]
#[command(version)]
struct Args {
    /// Configuration file path
    #[arg(short, long, default_value = "/etc/ollystack/stream-processor.yaml")]
    config: PathBuf,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // Initialize logging
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".to_string()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    info!("Starting OllyStack Stream Processor");

    // Load configuration
    let config = Config::load(&args.config)?;

    info!(
        otlp_endpoint = %config.sources.otlp.endpoint,
        "Configuration loaded"
    );

    // Create and run the stream engine
    let mut engine = StreamEngine::new(config).await?;

    // Build the processing pipeline
    engine.build_pipeline()?;

    // Run the engine
    info!("Stream Processor running - press Ctrl+C to stop");
    engine.run().await?;

    Ok(())
}
