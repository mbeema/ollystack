// Package config provides configuration management for the Universal Agent.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config represents the complete agent configuration.
type Config struct {
	// Collector configuration
	Collector CollectorConfig `yaml:"collector" mapstructure:"collector"`

	// Metrics collection configuration
	Metrics MetricsConfig `yaml:"metrics" mapstructure:"metrics"`

	// Logs collection configuration
	Logs LogsConfig `yaml:"logs" mapstructure:"logs"`

	// Discovery configuration
	Discovery DiscoveryConfig `yaml:"discovery" mapstructure:"discovery"`

	// Resource attributes
	Resource ResourceConfig `yaml:"resource" mapstructure:"resource"`

	// Agent internal settings
	Agent AgentConfig `yaml:"agent" mapstructure:"agent"`
}

// CollectorConfig defines the OTLP collector connection settings.
type CollectorConfig struct {
	// Endpoint is the OTLP collector address (e.g., "localhost:4317")
	Endpoint string `yaml:"endpoint" mapstructure:"endpoint"`

	// Insecure disables TLS
	Insecure bool `yaml:"insecure" mapstructure:"insecure"`

	// Headers to send with requests
	Headers map[string]string `yaml:"headers" mapstructure:"headers"`

	// Compression type: none, gzip
	Compression string `yaml:"compression" mapstructure:"compression"`

	// Timeout for export requests
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// Retry configuration
	Retry RetryConfig `yaml:"retry" mapstructure:"retry"`
}

// RetryConfig defines retry behavior for failed exports.
type RetryConfig struct {
	Enabled     bool          `yaml:"enabled" mapstructure:"enabled"`
	MaxAttempts int           `yaml:"max_attempts" mapstructure:"max_attempts"`
	InitialWait time.Duration `yaml:"initial_wait" mapstructure:"initial_wait"`
	MaxWait     time.Duration `yaml:"max_wait" mapstructure:"max_wait"`
}

// MetricsConfig defines metrics collection settings.
type MetricsConfig struct {
	// Enabled toggles metrics collection
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Interval between metric collections
	Interval time.Duration `yaml:"interval" mapstructure:"interval"`

	// Collectors to enable
	Collectors MetricCollectorsConfig `yaml:"collectors" mapstructure:"collectors"`
}

// MetricCollectorsConfig defines which metric collectors to enable.
type MetricCollectorsConfig struct {
	CPU        bool `yaml:"cpu" mapstructure:"cpu"`
	Memory     bool `yaml:"memory" mapstructure:"memory"`
	Disk       bool `yaml:"disk" mapstructure:"disk"`
	Network    bool `yaml:"network" mapstructure:"network"`
	Process    bool `yaml:"process" mapstructure:"process"`
	Container  bool `yaml:"container" mapstructure:"container"`
	Filesystem bool `yaml:"filesystem" mapstructure:"filesystem"`
	Load       bool `yaml:"load" mapstructure:"load"`
}

// LogsConfig defines log collection settings.
type LogsConfig struct {
	// Enabled toggles log collection
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Paths to watch for logs (glob patterns supported)
	Paths []string `yaml:"paths" mapstructure:"paths"`

	// Exclude patterns
	Exclude []string `yaml:"exclude" mapstructure:"exclude"`

	// Include multiline patterns
	Multiline MultilineConfig `yaml:"multiline" mapstructure:"multiline"`

	// Journald collection (Linux)
	Journald JournaldConfig `yaml:"journald" mapstructure:"journald"`

	// Windows Event Log collection
	WindowsEventLog WindowsEventLogConfig `yaml:"windows_event_log" mapstructure:"windows_event_log"`
}

// MultilineConfig defines multiline log handling.
type MultilineConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Pattern string `yaml:"pattern" mapstructure:"pattern"`
	Negate  bool   `yaml:"negate" mapstructure:"negate"`
	Match   string `yaml:"match" mapstructure:"match"` // "after" or "before"
}

// JournaldConfig defines journald collection settings.
type JournaldConfig struct {
	Enabled bool     `yaml:"enabled" mapstructure:"enabled"`
	Units   []string `yaml:"units" mapstructure:"units"`
}

// WindowsEventLogConfig defines Windows Event Log collection settings.
type WindowsEventLogConfig struct {
	Enabled  bool     `yaml:"enabled" mapstructure:"enabled"`
	Channels []string `yaml:"channels" mapstructure:"channels"`
}

// DiscoveryConfig defines auto-discovery settings.
type DiscoveryConfig struct {
	// Enabled toggles auto-discovery
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Kubernetes discovery
	Kubernetes bool `yaml:"kubernetes" mapstructure:"kubernetes"`

	// Docker discovery
	Docker bool `yaml:"docker" mapstructure:"docker"`

	// Process discovery
	Process bool `yaml:"process" mapstructure:"process"`
}

// ResourceConfig defines resource attributes.
type ResourceConfig struct {
	// Attributes to add to all telemetry
	Attributes map[string]string `yaml:"attributes" mapstructure:"attributes"`

	// Auto-detect cloud provider metadata
	Cloud CloudConfig `yaml:"cloud" mapstructure:"cloud"`
}

// CloudConfig defines cloud metadata detection settings.
type CloudConfig struct {
	// Enabled toggles cloud metadata detection
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Provider to detect: auto, aws, azure, gcp
	Provider string `yaml:"provider" mapstructure:"provider"`
}

// AgentConfig defines internal agent settings.
type AgentConfig struct {
	// Hostname override
	Hostname string `yaml:"hostname" mapstructure:"hostname"`

	// Log level: debug, info, warn, error
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`

	// Healthcheck settings
	Healthcheck HealthcheckConfig `yaml:"healthcheck" mapstructure:"healthcheck"`
}

// HealthcheckConfig defines healthcheck endpoint settings.
type HealthcheckConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Address string `yaml:"address" mapstructure:"address"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Collector: CollectorConfig{
			Endpoint:    "localhost:4317",
			Insecure:    true,
			Compression: "gzip",
			Timeout:     30 * time.Second,
			Retry: RetryConfig{
				Enabled:     true,
				MaxAttempts: 5,
				InitialWait: 1 * time.Second,
				MaxWait:     30 * time.Second,
			},
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Interval: 10 * time.Second,
			Collectors: MetricCollectorsConfig{
				CPU:        true,
				Memory:     true,
				Disk:       true,
				Network:    true,
				Process:    true,
				Container:  true,
				Filesystem: true,
				Load:       true,
			},
		},
		Logs: LogsConfig{
			Enabled: true,
			Paths: []string{
				"/var/log/*.log",
				"/var/log/syslog",
				"/var/log/messages",
			},
			Journald: JournaldConfig{
				Enabled: true,
			},
		},
		Discovery: DiscoveryConfig{
			Enabled:    true,
			Kubernetes: true,
			Docker:     true,
			Process:    true,
		},
		Resource: ResourceConfig{
			Attributes: map[string]string{},
			Cloud: CloudConfig{
				Enabled:  true,
				Provider: "auto",
			},
		},
		Agent: AgentConfig{
			LogLevel: "info",
			Healthcheck: HealthcheckConfig{
				Enabled: true,
				Address: ":8888",
			},
		},
	}
}

// Load loads configuration from file and environment.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	v := viper.New()
	v.SetConfigType("yaml")

	// Set defaults
	v.SetDefault("collector.endpoint", cfg.Collector.Endpoint)
	v.SetDefault("collector.insecure", cfg.Collector.Insecure)
	v.SetDefault("metrics.enabled", cfg.Metrics.Enabled)
	v.SetDefault("metrics.interval", cfg.Metrics.Interval)
	v.SetDefault("logs.enabled", cfg.Logs.Enabled)

	// Environment variable bindings
	v.SetEnvPrefix("OLLYSTACK")
	v.AutomaticEnv()

	// Load from file if specified
	if path != "" {
		v.SetConfigFile(path)
	} else {
		// Default config locations
		v.SetConfigName("agent")
		v.AddConfigPath("/etc/ollystack/")
		v.AddConfigPath("$HOME/.ollystack/")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Config file not found, using defaults
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Collector.Endpoint == "" {
		return fmt.Errorf("collector.endpoint is required")
	}

	if c.Metrics.Enabled && c.Metrics.Interval < time.Second {
		return fmt.Errorf("metrics.interval must be at least 1 second")
	}

	return nil
}

// Save saves the configuration to a file.
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
