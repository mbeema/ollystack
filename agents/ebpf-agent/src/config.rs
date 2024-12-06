//! Configuration for the eBPF agent.

use anyhow::Result;
use serde::Deserialize;
use std::path::Path;

#[derive(Debug, Clone, Deserialize)]
pub struct Config {
    pub collector: CollectorConfig,
    pub probes: ProbesConfig,
    pub filters: FiltersConfig,
    #[serde(default)]
    pub resource: ResourceConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct CollectorConfig {
    pub endpoint: String,
    #[serde(default)]
    pub insecure: bool,
    #[serde(default)]
    pub headers: std::collections::HashMap<String, String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ProbesConfig {
    pub network: NetworkProbeConfig,
    pub syscalls: SyscallProbeConfig,
    pub runtime: RuntimeProbeConfig,
}

#[derive(Debug, Clone, Deserialize)]
pub struct NetworkProbeConfig {
    #[serde(default = "default_true")]
    pub enabled: bool,
    #[serde(default)]
    pub protocols: Vec<String>,
    #[serde(default)]
    pub ports: Vec<u16>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SyscallProbeConfig {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub track: Vec<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeProbeConfig {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub languages: Vec<String>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct FiltersConfig {
    #[serde(default)]
    pub include_namespaces: Vec<String>,
    #[serde(default)]
    pub exclude_namespaces: Vec<String>,
    #[serde(default)]
    pub include_pids: Vec<u32>,
    #[serde(default)]
    pub exclude_pids: Vec<u32>,
}

#[derive(Debug, Clone, Deserialize, Default)]
pub struct ResourceConfig {
    #[serde(default)]
    pub attributes: std::collections::HashMap<String, String>,
}

fn default_true() -> bool {
    true
}

impl Config {
    pub fn load(path: &Path) -> Result<Self> {
        let builder = config::Config::builder()
            .add_source(config::File::from(path).required(false))
            .add_source(config::Environment::with_prefix("OLLYSTACK").separator("_"))
            .set_default("collector.endpoint", "localhost:4317")?
            .set_default("collector.insecure", true)?
            .set_default("probes.network.enabled", true)?
            .set_default("probes.network.protocols", vec!["http", "grpc", "dns"])?
            .set_default("probes.syscalls.enabled", false)?
            .set_default("probes.runtime.enabled", false)?;

        let config = builder.build()?;
        Ok(config.try_deserialize()?)
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            collector: CollectorConfig {
                endpoint: "localhost:4317".to_string(),
                insecure: true,
                headers: Default::default(),
            },
            probes: ProbesConfig {
                network: NetworkProbeConfig {
                    enabled: true,
                    protocols: vec!["http".to_string(), "grpc".to_string(), "dns".to_string()],
                    ports: vec![],
                },
                syscalls: SyscallProbeConfig {
                    enabled: false,
                    track: vec![],
                },
                runtime: RuntimeProbeConfig {
                    enabled: false,
                    languages: vec![],
                },
            },
            filters: Default::default(),
            resource: Default::default(),
        }
    }
}
