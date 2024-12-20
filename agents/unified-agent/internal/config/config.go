// Package config handles agent configuration with sensible defaults
// optimized for minimal resource usage.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level agent configuration
type Config struct {
	// Agent identification
	Agent AgentConfig `yaml:"agent"`

	// Collection modules
	Metrics MetricsConfig `yaml:"metrics"`
	Logs    LogsConfig    `yaml:"logs"`
	Traces  TracesConfig  `yaml:"traces"`

	// Processing
	Aggregation  AggregationConfig  `yaml:"aggregation"`
	Sampling     SamplingConfig     `yaml:"sampling"`
	Cardinality  CardinalityConfig  `yaml:"cardinality"`
	Enrichment   EnrichmentConfig   `yaml:"enrichment"`

	// Export
	Export ExportConfig `yaml:"export"`

	// Resource limits
	Resources ResourceConfig `yaml:"resources"`
}

// AgentConfig identifies this agent instance
type AgentConfig struct {
	// Hostname override (auto-detected if empty)
	Hostname string `yaml:"hostname"`

	// Service name for self-telemetry
	ServiceName string `yaml:"service_name"`

	// Environment (production, staging, development)
	Environment string `yaml:"environment"`

	// Custom tags added to all telemetry
	Tags map[string]string `yaml:"tags"`

	// Health check endpoint
	HealthPort int `yaml:"health_port"`
}

// MetricsConfig controls host metrics collection
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`

	// Collection interval (default: 15s, min: 5s)
	Interval time.Duration `yaml:"interval"`

	// Collectors to enable
	Collectors struct {
		CPU        bool `yaml:"cpu"`
		Memory     bool `yaml:"memory"`
		Disk       bool `yaml:"disk"`
		Network    bool `yaml:"network"`
		Filesystem bool `yaml:"filesystem"`
		Process    bool `yaml:"process"`
		Container  bool `yaml:"container"`
	} `yaml:"collectors"`

	// Process monitoring
	Process struct {
		// Only track processes matching these patterns
		Include []string `yaml:"include"`
		// Exclude processes matching these patterns
		Exclude []string `yaml:"exclude"`
		// Max processes to track (0 = unlimited)
		MaxProcesses int `yaml:"max_processes"`
	} `yaml:"process"`

	// Container monitoring
	Container struct {
		// Docker socket path
		DockerSocket string `yaml:"docker_socket"`
		// Include container labels as tags
		IncludeLabels []string `yaml:"include_labels"`
	} `yaml:"container"`
}

// LogsConfig controls log collection
type LogsConfig struct {
	Enabled bool `yaml:"enabled"`

	// Log sources
	Sources []LogSource `yaml:"sources"`

	// Pattern deduplication (huge bandwidth savings)
	Deduplication struct {
		Enabled bool `yaml:"enabled"`
		// Window for deduplication
		Window time.Duration `yaml:"window"`
		// Max unique patterns to track
		MaxPatterns int `yaml:"max_patterns"`
	} `yaml:"deduplication"`

	// Multi-line log handling
	Multiline struct {
		// Pattern to detect log start
		Pattern string `yaml:"pattern"`
		// Max lines to combine
		MaxLines int `yaml:"max_lines"`
	} `yaml:"multiline"`
}

// LogSource defines a log collection source
type LogSource struct {
	// Type: file, journald, docker, kubernetes
	Type string `yaml:"type"`

	// Path pattern (for file type)
	Path string `yaml:"path"`

	// Service name to assign
	Service string `yaml:"service"`

	// Include/exclude patterns
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`

	// Parse JSON logs
	ParseJSON bool `yaml:"parse_json"`

	// Extract trace context
	ExtractTraceContext bool `yaml:"extract_trace_context"`
}

// TracesConfig controls trace collection
type TracesConfig struct {
	Enabled bool `yaml:"enabled"`

	// OTLP receiver settings
	OTLP struct {
		// gRPC port (default: 4317)
		GRPCPort int `yaml:"grpc_port"`
		// HTTP port (default: 4318)
		HTTPPort int `yaml:"http_port"`
	} `yaml:"otlp"`

	// Beyla eBPF integration
	Beyla struct {
		Enabled bool `yaml:"enabled"`
		// Path to Beyla binary (if not in PATH)
		BinaryPath string `yaml:"binary_path"`
		// Config file for Beyla
		ConfigPath string `yaml:"config_path"`
	} `yaml:"beyla"`
}

// AggregationConfig controls local aggregation
type AggregationConfig struct {
	Enabled bool `yaml:"enabled"`

	// Aggregation window (default: 60s)
	// Metrics are aggregated over this window before sending
	Window time.Duration `yaml:"window"`

	// Metrics aggregation
	Metrics struct {
		// Compute these aggregates: min, max, sum, count, avg, p50, p90, p99
		Aggregates []string `yaml:"aggregates"`
		// Drop raw data points after aggregation
		DropRaw bool `yaml:"drop_raw"`
	} `yaml:"metrics"`

	// Logs aggregation
	Logs struct {
		// Group similar logs and send count
		GroupSimilar bool `yaml:"group_similar"`
		// Similarity threshold (0.0-1.0)
		SimilarityThreshold float64 `yaml:"similarity_threshold"`
	} `yaml:"logs"`
}

// SamplingConfig controls adaptive sampling
type SamplingConfig struct {
	Enabled bool `yaml:"enabled"`

	// Target data rate (bytes/second)
	// Agent will adjust sampling to stay under this
	TargetRate int64 `yaml:"target_rate"`

	// Traces sampling
	Traces struct {
		// Base sampling rate (0.0-1.0)
		Rate float64 `yaml:"rate"`
		// Always sample errors
		AlwaysSampleErrors bool `yaml:"always_sample_errors"`
		// Always sample slow requests (> threshold)
		SlowThreshold time.Duration `yaml:"slow_threshold"`
		// Sample new/rare operations at higher rate
		RareOperationBoost float64 `yaml:"rare_operation_boost"`
	} `yaml:"traces"`

	// Logs sampling
	Logs struct {
		// Sample rate for INFO logs
		InfoRate float64 `yaml:"info_rate"`
		// Sample rate for DEBUG logs
		DebugRate float64 `yaml:"debug_rate"`
		// Always keep ERROR and above
		AlwaysKeepErrors bool `yaml:"always_keep_errors"`
	} `yaml:"logs"`
}

// CardinalityConfig prevents metric explosion
type CardinalityConfig struct {
	Enabled bool `yaml:"enabled"`

	// Max unique time series per metric
	MaxSeriesPerMetric int `yaml:"max_series_per_metric"`

	// Max unique values per label
	MaxLabelValues map[string]int `yaml:"max_label_values"`

	// Labels to always drop (high cardinality)
	DropLabels []string `yaml:"drop_labels"`

	// Alert when cardinality exceeds threshold
	AlertThreshold int `yaml:"alert_threshold"`
}

// EnrichmentConfig controls metadata enrichment
type EnrichmentConfig struct {
	// Add hostname to all telemetry
	AddHostname bool `yaml:"add_hostname"`

	// Kubernetes metadata
	Kubernetes struct {
		Enabled bool `yaml:"enabled"`
		// Labels to include as tags
		IncludeLabels []string `yaml:"include_labels"`
		// Annotations to include as tags
		IncludeAnnotations []string `yaml:"include_annotations"`
	} `yaml:"kubernetes"`

	// Cloud provider metadata
	Cloud struct {
		Enabled bool `yaml:"enabled"`
		// Provider: auto, aws, azure, gcp
		Provider string `yaml:"provider"`
	} `yaml:"cloud"`

	// Custom static tags
	StaticTags map[string]string `yaml:"static_tags"`
}

// ExportConfig controls data export
type ExportConfig struct {
	// OTLP endpoint
	Endpoint string `yaml:"endpoint"`

	// Use gRPC (true) or HTTP (false)
	UseGRPC bool `yaml:"use_grpc"`

	// TLS configuration
	TLS struct {
		Enabled    bool   `yaml:"enabled"`
		CertFile   string `yaml:"cert_file"`
		KeyFile    string `yaml:"key_file"`
		CAFile     string `yaml:"ca_file"`
		SkipVerify bool   `yaml:"skip_verify"`
	} `yaml:"tls"`

	// Authentication
	Auth struct {
		// API key header
		APIKey string `yaml:"api_key"`
		// Bearer token
		BearerToken string `yaml:"bearer_token"`
	} `yaml:"auth"`

	// Batching
	Batch struct {
		// Max batch size
		MaxSize int `yaml:"max_size"`
		// Max wait time before sending
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"batch"`

	// Retry configuration
	Retry struct {
		Enabled     bool          `yaml:"enabled"`
		MaxAttempts int           `yaml:"max_attempts"`
		InitialWait time.Duration `yaml:"initial_wait"`
		MaxWait     time.Duration `yaml:"max_wait"`
	} `yaml:"retry"`

	// Disk buffer for reliability
	Buffer struct {
		Enabled bool   `yaml:"enabled"`
		Path    string `yaml:"path"`
		MaxSize int64  `yaml:"max_size"` // bytes
	} `yaml:"buffer"`
}

// ResourceConfig limits agent resource usage
type ResourceConfig struct {
	// Max memory (bytes, 0 = no limit)
	MaxMemory int64 `yaml:"max_memory"`

	// Max CPU cores (0 = no limit)
	MaxCPU float64 `yaml:"max_cpu"`

	// GOMAXPROCS setting
	MaxProcs int `yaml:"max_procs"`
}

// Load reads configuration from file and environment
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		// Expand environment variables
		expanded := os.ExpandEnv(string(data))

		if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
			return nil, fmt.Errorf("parse config: %w", err)
		}
	}

	// Override with environment variables
	cfg.applyEnvOverrides()

	// Validate
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns sensible defaults optimized for low resource usage
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			ServiceName: "ollystack-agent",
			Environment: "production",
			HealthPort:  8888,
		},
		Metrics: MetricsConfig{
			Enabled:  true,
			Interval: 15 * time.Second,
			Collectors: struct {
				CPU        bool `yaml:"cpu"`
				Memory     bool `yaml:"memory"`
				Disk       bool `yaml:"disk"`
				Network    bool `yaml:"network"`
				Filesystem bool `yaml:"filesystem"`
				Process    bool `yaml:"process"`
				Container  bool `yaml:"container"`
			}{
				CPU:        true,
				Memory:     true,
				Disk:       true,
				Network:    true,
				Filesystem: true,
				Process:    false, // Disabled by default (high overhead)
				Container:  true,
			},
			Process: struct {
				Include      []string `yaml:"include"`
				Exclude      []string `yaml:"exclude"`
				MaxProcesses int      `yaml:"max_processes"`
			}{
				MaxProcesses: 50,
			},
			Container: struct {
				DockerSocket  string   `yaml:"docker_socket"`
				IncludeLabels []string `yaml:"include_labels"`
			}{
				DockerSocket: "/var/run/docker.sock",
			},
		},
		Logs: LogsConfig{
			Enabled: true,
			Deduplication: struct {
				Enabled     bool          `yaml:"enabled"`
				Window      time.Duration `yaml:"window"`
				MaxPatterns int           `yaml:"max_patterns"`
			}{
				Enabled:     true,
				Window:      60 * time.Second,
				MaxPatterns: 10000,
			},
		},
		Traces: TracesConfig{
			Enabled: true,
			OTLP: struct {
				GRPCPort int `yaml:"grpc_port"`
				HTTPPort int `yaml:"http_port"`
			}{
				GRPCPort: 4317,
				HTTPPort: 4318,
			},
		},
		Aggregation: AggregationConfig{
			Enabled: true,
			Window:  60 * time.Second,
			Metrics: struct {
				Aggregates []string `yaml:"aggregates"`
				DropRaw    bool     `yaml:"drop_raw"`
			}{
				Aggregates: []string{"min", "max", "sum", "count", "avg"},
				DropRaw:    true, // Big bandwidth savings
			},
			Logs: struct {
				GroupSimilar        bool    `yaml:"group_similar"`
				SimilarityThreshold float64 `yaml:"similarity_threshold"`
			}{
				GroupSimilar:        true,
				SimilarityThreshold: 0.9,
			},
		},
		Sampling: SamplingConfig{
			Enabled:    true,
			TargetRate: 1024 * 1024, // 1MB/s default
			Traces: struct {
				Rate               float64       `yaml:"rate"`
				AlwaysSampleErrors bool          `yaml:"always_sample_errors"`
				SlowThreshold      time.Duration `yaml:"slow_threshold"`
				RareOperationBoost float64       `yaml:"rare_operation_boost"`
			}{
				Rate:               0.1, // 10% default
				AlwaysSampleErrors: true,
				SlowThreshold:      time.Second,
				RareOperationBoost: 5.0,
			},
			Logs: struct {
				InfoRate         float64 `yaml:"info_rate"`
				DebugRate        float64 `yaml:"debug_rate"`
				AlwaysKeepErrors bool    `yaml:"always_keep_errors"`
			}{
				InfoRate:         0.1,
				DebugRate:        0.01,
				AlwaysKeepErrors: true,
			},
		},
		Cardinality: CardinalityConfig{
			Enabled:            true,
			MaxSeriesPerMetric: 10000,
			MaxLabelValues: map[string]int{
				"user_id":    0,    // Never use as label
				"request_id": 0,    // Never use as label
				"session_id": 0,    // Never use as label
				"trace_id":   0,    // Never use as label
				"endpoint":   1000, // Limit endpoint cardinality
			},
			DropLabels:     []string{"password", "token", "secret", "key"},
			AlertThreshold: 5000,
		},
		Enrichment: EnrichmentConfig{
			AddHostname: true,
			Kubernetes: struct {
				Enabled            bool     `yaml:"enabled"`
				IncludeLabels      []string `yaml:"include_labels"`
				IncludeAnnotations []string `yaml:"include_annotations"`
			}{
				Enabled:       true,
				IncludeLabels: []string{"app", "version", "team"},
			},
			Cloud: struct {
				Enabled  bool   `yaml:"enabled"`
				Provider string `yaml:"provider"`
			}{
				Enabled:  true,
				Provider: "auto",
			},
		},
		Export: ExportConfig{
			Endpoint: "localhost:4317",
			UseGRPC:  true,
			Batch: struct {
				MaxSize int           `yaml:"max_size"`
				Timeout time.Duration `yaml:"timeout"`
			}{
				MaxSize: 1000,
				Timeout: 5 * time.Second,
			},
			Retry: struct {
				Enabled     bool          `yaml:"enabled"`
				MaxAttempts int           `yaml:"max_attempts"`
				InitialWait time.Duration `yaml:"initial_wait"`
				MaxWait     time.Duration `yaml:"max_wait"`
			}{
				Enabled:     true,
				MaxAttempts: 5,
				InitialWait: time.Second,
				MaxWait:     30 * time.Second,
			},
			Buffer: struct {
				Enabled bool   `yaml:"enabled"`
				Path    string `yaml:"path"`
				MaxSize int64  `yaml:"max_size"`
			}{
				Enabled: true,
				Path:    "/var/lib/ollystack-agent/buffer",
				MaxSize: 256 * 1024 * 1024, // 256MB
			},
		},
		Resources: ResourceConfig{
			MaxMemory: 100 * 1024 * 1024, // 100MB
			MaxCPU:    0.5,               // 50% of one core
			MaxProcs:  2,
		},
	}
}

// applyEnvOverrides applies environment variable overrides
func (c *Config) applyEnvOverrides() {
	if v := os.Getenv("OLLYSTACK_ENDPOINT"); v != "" {
		c.Export.Endpoint = v
	}
	if v := os.Getenv("OLLYSTACK_API_KEY"); v != "" {
		c.Export.Auth.APIKey = v
	}
	if v := os.Getenv("OLLYSTACK_ENVIRONMENT"); v != "" {
		c.Agent.Environment = v
	}
	if v := os.Getenv("OLLYSTACK_HOSTNAME"); v != "" {
		c.Agent.Hostname = v
	}
}

// Validate checks configuration validity
func (c *Config) Validate() error {
	if c.Metrics.Interval < 5*time.Second {
		return fmt.Errorf("metrics interval must be at least 5s")
	}
	if c.Aggregation.Window < 10*time.Second {
		return fmt.Errorf("aggregation window must be at least 10s")
	}
	if c.Export.Endpoint == "" {
		return fmt.Errorf("export endpoint is required")
	}
	return nil
}
