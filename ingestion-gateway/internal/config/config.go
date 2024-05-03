package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the ingestion gateway
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	ClickHouse ClickHouseConfig `mapstructure:"clickhouse"`
	RateLimit  RateLimitConfig  `mapstructure:"rate_limit"`
	Tenancy    TenancyConfig    `mapstructure:"tenancy"`
	Sampling   SamplingConfig   `mapstructure:"sampling"`
}

// ClickHouseConfig holds ClickHouse connection configuration
type ClickHouseConfig struct {
	Host          string        `mapstructure:"host"`
	Port          int           `mapstructure:"port"`
	Database      string        `mapstructure:"database"`
	Username      string        `mapstructure:"username"`
	Password      string        `mapstructure:"password"`
	MaxOpenConns  int           `mapstructure:"max_open_conns"`
	MaxIdleConns  int           `mapstructure:"max_idle_conns"`
	DialTimeout   time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout   time.Duration `mapstructure:"read_timeout"`
	WriteTimeout  time.Duration `mapstructure:"write_timeout"`
	Compression   string        `mapstructure:"compression"`
	BatchSize     int           `mapstructure:"batch_size"`
	FlushInterval time.Duration `mapstructure:"flush_interval"`
	MaxRetries    int           `mapstructure:"max_retries"`
	RetryInterval time.Duration `mapstructure:"retry_interval"`
}

// SamplingConfig holds sampling configuration
type SamplingConfig struct {
	Enabled         bool    `mapstructure:"enabled"`
	DefaultRate     float64 `mapstructure:"default_rate"`
	ErrorRate       float64 `mapstructure:"error_rate"`
	SlowRate        float64 `mapstructure:"slow_rate"`
	SlowThresholdMs int64   `mapstructure:"slow_threshold_ms"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	GRPCPort    int `mapstructure:"grpc_port"`
	HTTPPort    int `mapstructure:"http_port"`
	MetricsPort int `mapstructure:"metrics_port"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool    `mapstructure:"enabled"`
	DefaultRPS        float64 `mapstructure:"default_rps"`        // Requests per second
	DefaultBurst      int     `mapstructure:"default_burst"`      // Burst allowance
	DefaultBytesPerSec int64  `mapstructure:"default_bytes_per_sec"` // Bytes per second
}

// TenancyConfig holds multi-tenancy configuration
type TenancyConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	TenantHeader    string `mapstructure:"tenant_header"`    // Header to extract tenant ID
	DefaultTenantID string `mapstructure:"default_tenant_id"`
}

// Load loads configuration from file and environment
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/ollystack/")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
	}

	// Environment variables
	v.SetEnvPrefix("OLLYSTACK")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found, use defaults + env vars
	}

	// Override with environment variables for sensitive values
	if host := os.Getenv("CLICKHOUSE_HOST"); host != "" {
		v.Set("clickhouse.host", host)
	}
	if user := os.Getenv("CLICKHOUSE_USER"); user != "" {
		v.Set("clickhouse.username", user)
	}
	if pass := os.Getenv("CLICKHOUSE_PASSWORD"); pass != "" {
		v.Set("clickhouse.password", pass)
	}
	if db := os.Getenv("CLICKHOUSE_DATABASE"); db != "" {
		v.Set("clickhouse.database", db)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.grpc_port", 4317)
	v.SetDefault("server.http_port", 4318)
	v.SetDefault("server.metrics_port", 8888)

	// ClickHouse defaults
	v.SetDefault("clickhouse.host", "localhost")
	v.SetDefault("clickhouse.port", 9000)
	v.SetDefault("clickhouse.database", "ollystack")
	v.SetDefault("clickhouse.username", "default")
	v.SetDefault("clickhouse.password", "")
	v.SetDefault("clickhouse.max_open_conns", 10)
	v.SetDefault("clickhouse.max_idle_conns", 5)
	v.SetDefault("clickhouse.dial_timeout", 10*time.Second)
	v.SetDefault("clickhouse.read_timeout", 60*time.Second)
	v.SetDefault("clickhouse.write_timeout", 60*time.Second)
	v.SetDefault("clickhouse.compression", "lz4")
	v.SetDefault("clickhouse.batch_size", 10000)
	v.SetDefault("clickhouse.flush_interval", 1*time.Second)
	v.SetDefault("clickhouse.max_retries", 3)
	v.SetDefault("clickhouse.retry_interval", 100*time.Millisecond)

	// Rate limit defaults
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.default_rps", 10000)              // 10k requests/sec per tenant
	v.SetDefault("rate_limit.default_burst", 20000)            // 2x burst
	v.SetDefault("rate_limit.default_bytes_per_sec", 104857600) // 100MB/sec per tenant

	// Tenancy defaults
	v.SetDefault("tenancy.enabled", true)
	v.SetDefault("tenancy.tenant_header", "X-Tenant-ID")
	v.SetDefault("tenancy.default_tenant_id", "default")

	// Sampling defaults
	v.SetDefault("sampling.enabled", true)
	v.SetDefault("sampling.default_rate", 0.1)      // 10% default sampling
	v.SetDefault("sampling.error_rate", 1.0)        // 100% of errors
	v.SetDefault("sampling.slow_rate", 1.0)         // 100% of slow requests
	v.SetDefault("sampling.slow_threshold_ms", 1000) // 1 second
}

func validate(cfg *Config) error {
	if cfg.ClickHouse.Host == "" {
		return fmt.Errorf("ClickHouse host must be configured")
	}
	if cfg.ClickHouse.Port <= 0 || cfg.ClickHouse.Port > 65535 {
		return fmt.Errorf("invalid ClickHouse port: %d", cfg.ClickHouse.Port)
	}
	if cfg.Server.GRPCPort <= 0 || cfg.Server.GRPCPort > 65535 {
		return fmt.Errorf("invalid gRPC port: %d", cfg.Server.GRPCPort)
	}
	if cfg.Server.HTTPPort <= 0 || cfg.Server.HTTPPort > 65535 {
		return fmt.Errorf("invalid HTTP port: %d", cfg.Server.HTTPPort)
	}
	return nil
}
