// Package config provides configuration management for the API server.
package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config holds the API server configuration.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Storage  StorageConfig  `mapstructure:"storage"`
	AI       AIConfig       `mapstructure:"ai"`
	Features FeaturesConfig `mapstructure:"features"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Address      string        `mapstructure:"address"`
	Mode         string        `mapstructure:"mode"` // debug, release
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout"`
	CORS         CORSConfig    `mapstructure:"cors"`
}

// CORSConfig holds CORS settings.
type CORSConfig struct {
	Enabled        bool     `mapstructure:"enabled"`
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedMethods []string `mapstructure:"allowed_methods"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

// AuthConfig holds authentication settings.
type AuthConfig struct {
	Enabled   bool        `mapstructure:"enabled"`
	Type      string      `mapstructure:"type"` // jwt, api_key, oauth2
	JWT       JWTConfig   `mapstructure:"jwt"`
	OAuth2    OAuth2Config `mapstructure:"oauth2"`
}

// JWTConfig holds JWT settings.
type JWTConfig struct {
	Secret     string        `mapstructure:"secret"`
	Expiration time.Duration `mapstructure:"expiration"`
	Issuer     string        `mapstructure:"issuer"`
}

// OAuth2Config holds OAuth2 settings.
type OAuth2Config struct {
	Provider     string `mapstructure:"provider"` // google, github, okta
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	RedirectURL  string `mapstructure:"redirect_url"`
}

// StorageConfig holds storage backend settings.
type StorageConfig struct {
	Metrics StorageBackendConfig `mapstructure:"metrics"`
	Logs    StorageBackendConfig `mapstructure:"logs"`
	Traces  StorageBackendConfig `mapstructure:"traces"`
}

// StorageBackendConfig holds settings for a storage backend.
type StorageBackendConfig struct {
	Type     string `mapstructure:"type"` // clickhouse, postgres, s3
	Endpoint string `mapstructure:"endpoint"`
	Database string `mapstructure:"database"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// AIConfig holds AI service settings.
type AIConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Provider string `mapstructure:"provider"` // openai, anthropic, local
	APIKey   string `mapstructure:"api_key"`
	Model    string `mapstructure:"model"`
	Endpoint string `mapstructure:"endpoint"`
}

// FeaturesConfig holds feature flags.
type FeaturesConfig struct {
	ServiceMap       bool `mapstructure:"service_map"`
	AnomalyDetection bool `mapstructure:"anomaly_detection"`
	NaturalLanguage  bool `mapstructure:"natural_language"`
}

// Load loads configuration from file and environment.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	v := viper.New()
	v.SetConfigType("yaml")

	// Environment variable bindings
	v.SetEnvPrefix("OLLYSTACK")
	v.AutomaticEnv()

	// Load from file if specified
	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath("/etc/ollystack/")
		v.AddConfigPath("$HOME/.ollystack/")
		v.AddConfigPath("./configs/")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Address:      ":8080",
			Mode:         "debug",
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
			CORS: CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Authorization", "Content-Type"},
			},
		},
		Auth: AuthConfig{
			Enabled: false,
		},
		Storage: StorageConfig{
			Metrics: StorageBackendConfig{
				Type:     "clickhouse",
				Endpoint: "localhost:9000",
				Database: "ollystack",
			},
			Logs: StorageBackendConfig{
				Type:     "clickhouse",
				Endpoint: "localhost:9000",
				Database: "ollystack",
			},
			Traces: StorageBackendConfig{
				Type:     "clickhouse",
				Endpoint: "localhost:9000",
				Database: "ollystack",
			},
		},
		AI: AIConfig{
			Enabled: true,
			Provider: "openai",
			Model: "gpt-4",
		},
		Features: FeaturesConfig{
			ServiceMap:       true,
			AnomalyDetection: true,
			NaturalLanguage:  true,
		},
	}
}
