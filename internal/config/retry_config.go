package config

import (
	"os"
	"strconv"
	"time"
)

// RetryConfig holds retry configuration
type RetryConfig struct {
	// MaxAttempts per request (default: 3)
	MaxAttempts int
	// SameProviderMaxRetries for retrying same provider
	SameProviderMaxRetries int
	// BaseDelayMs for exponential backoff
	BaseDelayMs int
	// MaxDelayMs caps backoff
	MaxDelayMs int
	// Jitter factor (0.0-1.0)
	Jitter float64
	// RespectRetryAfter from provider headers
	RespectRetryAfter bool
	// MaxRetryAfterMs caps retry-after duration
	MaxRetryAfterMs int
}

// LoadRetryConfig loads retry configuration from environment
func LoadRetryConfig() RetryConfig {
	cfg := RetryConfig{
		MaxAttempts:           3,
		SameProviderMaxRetries: 1,
		BaseDelayMs:           250,
		MaxDelayMs:            8000,
		Jitter:                0.5,
		RespectRetryAfter:     true,
		MaxRetryAfterMs:       60000,
	}

	if val := os.Getenv("RETRY_MAX_ATTEMPTS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxAttempts = n
		}
	}
	if val := os.Getenv("RETRY_BASE_DELAY_MS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.BaseDelayMs = n
		}
	}
	if val := os.Getenv("RETRY_MAX_DELAY_MS"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			cfg.MaxDelayMs = n
		}
	}

	return cfg
}

// CooldownConfig holds cooldown configuration
type CooldownConfig struct {
	// Enabled toggles cooldown feature
	Enabled bool
	// DefaultDuration for cooldowns
	DefaultDuration time.Duration
	// RateLimitDuration for rate limit cooldowns
	RateLimitDuration time.Duration
	// PaymentDuration for billing errors
	PaymentDuration time.Duration
	// Error5xxDuration for server errors
	Error5xxDuration time.Duration
}

// LoadCooldownConfig loads cooldown configuration
func LoadCooldownConfig() CooldownConfig {
	cfg := CooldownConfig{
		Enabled:             true,
		DefaultDuration:     30 * time.Second,
		RateLimitDuration:   5 * time.Second,
		PaymentDuration:     300 * time.Second,
		Error5xxDuration:    30 * time.Second,
	}

	if val := os.Getenv("COOLDOWN_ENABLED"); val != "" {
		cfg.Enabled = val == "true"
	}

	return cfg
}
