//! Syscall probes for monitoring file I/O, process lifecycle.

use anyhow::Result;
use tokio::sync::mpsc;

use crate::config::SyscallProbeConfig;
use super::{ProbeEvent, SyscallEvent};

/// Syscall probe for monitoring system calls.
pub struct SyscallProbe {
    config: SyscallProbeConfig,
    event_tx: mpsc::Sender<ProbeEvent>,
}

impl SyscallProbe {
    pub fn new(config: &SyscallProbeConfig, event_tx: mpsc::Sender<ProbeEvent>) -> Result<Self> {
        Ok(Self {
            config: config.clone(),
            event_tx,
        })
    }

    /// Attach probes for file operations.
    pub fn attach_file_probes(&mut self) -> Result<()> {
        // Attach to openat, read, write, close syscalls
        Ok(())
    }

    /// Attach probes for process lifecycle.
    pub fn attach_process_probes(&mut self) -> Result<()> {
        // Attach to fork, exec, exit syscalls
        Ok(())
    }
}
