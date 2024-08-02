package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Kafka    KafkaConfig    `mapstructure:"kafka"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Alerting AlertingConfig `mapstructure:"alerting"`
	Rules    RulesConfig    `mapstructure:"rules"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	MetricsPort int `mapstructure:"metrics_port"`
}

// KafkaConfig holds Kafka consumer configuration
type KafkaConfig struct {
	Brokers      []string `mapstructure:"brokers"`
	GroupID      string   `mapstructure:"group_id"`
	MetricsTopic string   `mapstructure:"metrics_topic"`
	LogsTopic    string   `mapstructure:"logs_topic"`
	TracesTopic  string   `mapstructure:"traces_topic"`
	AlertsTopic  string   `mapstructure:"alerts_topic"`
}

// RedisConfig holds Redis configuration for state management
type RedisConfig struct {
	Address     string `mapstructure:"address"`
	Password    string `mapstructure:"password"`
	DB          int    `mapstructure:"db"`
	MaxRetries  int    `mapstructure:"max_retries"`
	PoolSize    int    `mapstructure:"pool_size"`
}

// AlertingConfig holds alerting configuration
type AlertingConfig struct {
	AlertManagerURL string            `mapstructure:"alertmanager_url"`
	WebhookURL      string            `mapstructure:"webhook_url"`
	SlackWebhook    string            `mapstructure:"slack_webhook"`
	PagerDutyKey    string            `mapstructure:"pagerduty_key"`
	Labels          map[string]string `mapstructure:"labels"`
}

// RulesConfig holds rules engine configuration
type RulesConfig struct {
	RulesPath       string `mapstructure:"rules_path"`
	ClickHouseURL   string `mapstructure:"clickhouse_url"`
	EvaluationLimit int    `mapstructure:"evaluation_limit"` // Max rules to evaluate per second
}

// Load loads configuration from file and environment
func Load(cfgFile string) (*Config, error) {
	v := viper.New()

	setDefaults(v)

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/ollystack/")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
	}

	v.SetEnvPrefix("OLLYSTACK")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	// Override with environment variables
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		v.Set("kafka.brokers", strings.Split(brokers, ","))
	}
	if redisAddr := os.Getenv("REDIS_URL"); redisAddr != "" {
		v.Set("redis.address", redisAddr)
	}
	if amURL := os.Getenv("ALERTMANAGER_URL"); amURL != "" {
		v.Set("alerting.alertmanager_url", amURL)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Server
	v.SetDefault("server.metrics_port", 8890)

	// Kafka
	v.SetDefault("kafka.brokers", []string{"localhost:9092"})
	v.SetDefault("kafka.group_id", "ollystack-realtime-processor")
	v.SetDefault("kafka.metrics_topic", "ollystack-metrics")
	v.SetDefault("kafka.logs_topic", "ollystack-logs")
	v.SetDefault("kafka.traces_topic", "ollystack-traces")
	v.SetDefault("kafka.alerts_topic", "ollystack-alerts")

	// Redis
	v.SetDefault("redis.address", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("redis.max_retries", 3)
	v.SetDefault("redis.pool_size", 10)

	// Alerting
	v.SetDefault("alerting.alertmanager_url", "http://alertmanager:9093")
	v.SetDefault("alerting.labels", map[string]string{
		"source": "ollystack",
	})

	// Rules
	v.SetDefault("rules.rules_path", "/etc/ollystack/rules")
	v.SetDefault("rules.clickhouse_url", "http://clickhouse:8123")
	v.SetDefault("rules.evaluation_limit", 1000)
}
