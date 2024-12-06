//! eBPF probe management.

use anyhow::{Context, Result};
use aya::Bpf;
use std::sync::Arc;
use tokio::sync::mpsc;
use tracing::{debug, info, warn};

use crate::config::Config;
use crate::exporters::OtlpExporter;

pub mod network;
pub mod syscalls;
pub mod runtime;

pub use network::NetworkProbe;
pub use syscalls::SyscallProbe;
pub use runtime::RuntimeProbe;

/// Events emitted by eBPF probes.
#[derive(Debug, Clone)]
pub enum ProbeEvent {
    /// Network event (connection, request, response)
    Network(NetworkEvent),
    /// Syscall event
    Syscall(SyscallEvent),
    /// Runtime event (GC, allocations)
    Runtime(RuntimeEvent),
}

#[derive(Debug, Clone)]
pub struct NetworkEvent {
    pub timestamp: u64,
    pub pid: u32,
    pub comm: String,
    pub src_addr: String,
    pub src_port: u16,
    pub dst_addr: String,
    pub dst_port: u16,
    pub protocol: String,
    pub method: Option<String>,
    pub path: Option<String>,
    pub status_code: Option<u16>,
    pub latency_ns: u64,
    pub bytes_sent: u64,
    pub bytes_recv: u64,
}

#[derive(Debug, Clone)]
pub struct SyscallEvent {
    pub timestamp: u64,
    pub pid: u32,
    pub comm: String,
    pub syscall: String,
    pub latency_ns: u64,
    pub retval: i64,
}

#[derive(Debug, Clone)]
pub struct RuntimeEvent {
    pub timestamp: u64,
    pub pid: u32,
    pub event_type: String,
    pub data: serde_json::Value,
}

/// Manages all eBPF probes.
pub struct ProbeManager {
    config: Config,
    exporter: Arc<OtlpExporter>,
    event_tx: mpsc::Sender<ProbeEvent>,
    event_rx: mpsc::Receiver<ProbeEvent>,
    network_probe: Option<NetworkProbe>,
    syscall_probe: Option<SyscallProbe>,
    runtime_probe: Option<RuntimeProbe>,
}

impl ProbeManager {
    pub fn new(config: Config, exporter: OtlpExporter) -> Result<Self> {
        let (event_tx, event_rx) = mpsc::channel(10000);

        Ok(Self {
            config,
            exporter: Arc::new(exporter),
            event_tx,
            event_rx,
            network_probe: None,
            syscall_probe: None,
            runtime_probe: None,
        })
    }

    /// Load network probes for TCP/UDP, HTTP, gRPC, DNS monitoring.
    pub fn load_network_probes(&mut self) -> Result<()> {
        let probe = NetworkProbe::new(&self.config.probes.network, self.event_tx.clone())?;
        self.network_probe = Some(probe);
        Ok(())
    }

    /// Load syscall probes for file I/O, process lifecycle monitoring.
    pub fn load_syscall_probes(&mut self) -> Result<()> {
        let probe = SyscallProbe::new(&self.config.probes.syscalls, self.event_tx.clone())?;
        self.syscall_probe = Some(probe);
        Ok(())
    }

    /// Load runtime probes for language-specific profiling.
    pub fn load_runtime_probes(&mut self) -> Result<()> {
        let probe = RuntimeProbe::new(&self.config.probes.runtime, self.event_tx.clone())?;
        self.runtime_probe = Some(probe);
        Ok(())
    }

    /// Run the probe manager, processing events and exporting telemetry.
    pub async fn run(&mut self) -> Result<()> {
        let exporter = self.exporter.clone();

        // Spawn event processing task
        let mut event_rx = std::mem::replace(
            &mut self.event_rx,
            mpsc::channel(1).1,
        );

        tokio::spawn(async move {
            while let Some(event) = event_rx.recv().await {
                if let Err(e) = exporter.export_event(&event).await {
                    warn!("Failed to export event: {}", e);
                }
            }
        });

        // Handle shutdown
        tokio::signal::ctrl_c().await?;
        info!("Shutting down eBPF Agent");

        Ok(())
    }
}
