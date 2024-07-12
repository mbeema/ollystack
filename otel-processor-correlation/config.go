// Copyright 2026 OllyStack
// SPDX-License-Identifier: Apache-2.0

package correlationprocessor

import (
	"errors"

	"go.opentelemetry.io/collector/component"
)

// Config defines configuration for the correlation processor.
type Config struct {
	// GenerateIfMissing creates correlation ID if not found in incoming data
	GenerateIfMissing bool `mapstructure:"generate_if_missing"`

	// IDPrefix for generated correlation IDs (default: "olly")
	IDPrefix string `mapstructure:"id_prefix"`

	// ExtractFromHeaders lists header names to check for existing correlation ID
	ExtractFromHeaders []string `mapstructure:"extract_from_headers"`

	// ExtractFromBaggage enables extraction from W3C baggage
	ExtractFromBaggage bool `mapstructure:"extract_from_baggage"`

	// BaggageKey is the key to look for in W3C baggage (default: "correlation_id")
	BaggageKey string `mapstructure:"baggage_key"`

	// AttributeName is the attribute name to store correlation ID (default: "correlation_id")
	AttributeName string `mapstructure:"attribute_name"`

	// PropagateToResource copies correlation_id to resource attributes
	PropagateToResource bool `mapstructure:"propagate_to_resource"`

	// DeriveFromTraceID uses trace_id as seed for generating consistent correlation IDs
	DeriveFromTraceID bool `mapstructure:"derive_from_trace_id"`
}

var _ component.Config = (*Config)(nil)

// Validate checks if the processor configuration is valid.
func (cfg *Config) Validate() error {
	if cfg.AttributeName == "" {
		return errors.New("attribute_name cannot be empty")
	}
	if cfg.IDPrefix == "" {
		return errors.New("id_prefix cannot be empty")
	}
	return nil
}

// createDefaultConfig returns the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		GenerateIfMissing: true,
		IDPrefix:          "olly",
		ExtractFromHeaders: []string{
			"X-Correlation-ID",
			"X-Request-ID",
			"x-correlation-id",
			"x-request-id",
			"correlation-id",
			"request-id",
		},
		ExtractFromBaggage:  true,
		BaggageKey:          "correlation_id",
		AttributeName:       "correlation_id",
		PropagateToResource: true,
		DeriveFromTraceID:   true,
	}
}
