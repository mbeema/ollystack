package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration
type Config struct {
	Server     ServerConfig     `mapstructure:"server"`
	Kafka      KafkaConfig      `mapstructure:"kafka"`
	ClickHouse ClickHouseConfig `mapstructure:"clickhouse"`
	Batcher    BatcherConfig    `mapstructure:"batcher"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	MetricsPort int `mapstructure:"metrics_port"`
}

// KafkaConfig holds Kafka consumer configuration
type KafkaConfig struct {
	Brokers        []string `mapstructure:"brokers"`
	GroupID        string   `mapstructure:"group_id"`
	MetricsTopic   string   `mapstructure:"metrics_topic"`
	LogsTopic      string   `mapstructure:"logs_topic"`
	TracesTopic    string   `mapstructure:"traces_topic"`
	AutoOffsetReset string  `mapstructure:"auto_offset_reset"` // earliest, latest
	SessionTimeoutMs int    `mapstructure:"session_timeout_ms"`
	HeartbeatIntervalMs int `mapstructure:"heartbeat_interval_ms"`
	MaxPollRecords int      `mapstructure:"max_poll_records"`
}

// ClickHouseConfig holds ClickHouse connection configuration
type ClickHouseConfig struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	Database     string `mapstructure:"database"`
	Username     string `mapstructure:"username"`
	Password     string `mapstructure:"password"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
	DialTimeout  int    `mapstructure:"dial_timeout"`  // seconds
	ReadTimeout  int    `mapstructure:"read_timeout"`  // seconds
	WriteTimeout int    `mapstructure:"write_timeout"` // seconds
	Compression  string `mapstructure:"compression"`   // none, lz4, zstd
}

// BatcherConfig holds batching configuration
type BatcherConfig struct {
	MaxSize      int `mapstructure:"max_size"`       // Max messages per batch
	MaxBytes     int `mapstructure:"max_bytes"`      // Max bytes per batch
	FlushIntervalMs int `mapstructure:"flush_interval_ms"` // Max time to wait
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
	}

	// Override with environment variables
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		v.Set("kafka.brokers", strings.Split(brokers, ","))
	}
	if chHost := os.Getenv("CLICKHOUSE_HOST"); chHost != "" {
		v.Set("clickhouse.host", chHost)
	}
	if chUser := os.Getenv("CLICKHOUSE_USER"); chUser != "" {
		v.Set("clickhouse.username", chUser)
	}
	if chPass := os.Getenv("CLICKHOUSE_PASSWORD"); chPass != "" {
		v.Set("clickhouse.password", chPass)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.metrics_port", 8889)

	// Kafka
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.group_id", "ollystack-storage-consumer")
	v.SetDefault("kafka.metrics_topic", "ollystack-metrics")
	v.SetDefault("kafka.logs_topic", "ollystack-logs")
	v.SetDefault("kafka.traces_topic", "ollystack-traces")
	v.SetDefault("kafka.auto_offset_reset", "earliest")
	v.SetDefault("kafka.session_timeout_ms", 30000)
	v.SetDefault("kafka.heartbeat_interval_ms", 10000)
	v.SetDefault("kafka.max_poll_records", 500)

	// ClickHouse
	v.SetDefault("clickhouse.host", "localhost")
	v.SetDefault("clickhouse.port", 9000)
	v.SetDefault("clickhouse.database", "ollystack")
	v.SetDefault("clickhouse.username", "default")
	v.SetDefault("clickhouse.password", "")
	v.SetDefault("clickhouse.max_open_conns", 10)
	v.SetDefault("clickhouse.max_idle_conns", 5)
	v.SetDefault("clickhouse.dial_timeout", 10)
	v.SetDefault("clickhouse.read_timeout", 60)
	v.SetDefault("clickhouse.write_timeout", 60)
	v.SetDefault("clickhouse.compression", "lz4")

	// Batcher
	v.SetDefault("batcher.max_size", 10000)        // 10k messages
	v.SetDefault("batcher.max_bytes", 10485760)    // 10MB
	v.SetDefault("batcher.flush_interval_ms", 1000) // 1 second
}
