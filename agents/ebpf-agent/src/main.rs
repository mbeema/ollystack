//! OllyStack eBPF Agent
//!
//! Zero-code kernel-level instrumentation for:
//! - Network traffic (TCP/UDP, HTTP, gRPC, DNS)
//! - System calls (file I/O, process lifecycle)
//! - Runtime profiling (Go, Java, Python, Node.js)

use anyhow::{Context, Result};
use clap::Parser;
use std::path::PathBuf;
use tracing::{info, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

mod config;
mod exporters;
mod probes;

use config::Config;
use exporters::OtlpExporter;
use probes::ProbeManager;

#[derive(Parser, Debug)]
#[command(name = "ollystack-ebpf-agent")]
#[command(about = "Zero-code kernel-level instrumentation agent")]
#[command(version)]
struct Args {
    /// Configuration file path
    #[arg(short, long, default_value = "/etc/ollystack/ebpf-agent.yaml")]
    config: PathBuf,

    /// Enable verbose logging
    #[arg(short, long)]
    verbose: bool,
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    // Initialize logging
    let log_level = if args.verbose { "debug" } else { "info" };
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| log_level.to_string()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    info!("Starting OllyStack eBPF Agent");

    // Load configuration
    let config = Config::load(&args.config)
        .context("Failed to load configuration")?;

    info!(
        endpoint = %config.collector.endpoint,
        "Configuration loaded"
    );

    // Check for root privileges (required for eBPF)
    if !is_root() {
        warn!("Running without root privileges - some features may not work");
    }

    // Initialize OTLP exporter
    let exporter = OtlpExporter::new(&config.collector)
        .await
        .context("Failed to create OTLP exporter")?;

    // Initialize probe manager
    let mut probe_manager = ProbeManager::new(config.clone(), exporter)
        .context("Failed to create probe manager")?;

    // Load and attach probes
    if config.probes.network.enabled {
        probe_manager
            .load_network_probes()
            .context("Failed to load network probes")?;
        info!("Network probes loaded");
    }

    if config.probes.syscalls.enabled {
        probe_manager
            .load_syscall_probes()
            .context("Failed to load syscall probes")?;
        info!("Syscall probes loaded");
    }

    if config.probes.runtime.enabled {
        probe_manager
            .load_runtime_probes()
            .context("Failed to load runtime probes")?;
        info!("Runtime probes loaded");
    }

    // Start processing events
    info!("eBPF Agent running - press Ctrl+C to stop");
    probe_manager.run().await?;

    Ok(())
}

fn is_root() -> bool {
    unsafe { libc::geteuid() == 0 }
}
