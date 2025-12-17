package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Port            int
	Host            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration

	// Storage configuration
	StorageType string
	CacheDir    string

	// Upstream configuration
	UpstreamRegistry string
	UpstreamTimeout  time.Duration
	MaxRetries       int

	// Mirror configuration
	BaseURL string

	// Observability
	LogLevel       string
	LogFormat      string
	MetricsEnabled bool
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		// Defaults
		Port:             8080,
		Host:             "0.0.0.0",
		ReadTimeout:      30 * time.Second,
		WriteTimeout:     30 * time.Second,
		ShutdownTimeout:  30 * time.Second,
		StorageType:      "filesystem",
		CacheDir:         "/var/cache/speculum",
		UpstreamRegistry: "https://registry.terraform.io",
		UpstreamTimeout:  60 * time.Second,
		MaxRetries:       3,
		BaseURL:          "http://localhost:8080",
		LogLevel:         "info",
		LogFormat:        "json",
		MetricsEnabled:   true,
	}

	// Override with environment variables
	if v := os.Getenv("SPECULUM_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("SPECULUM_PORT must be a valid integer")
		}
		cfg.Port = port
	}

	if v := os.Getenv("SPECULUM_HOST"); v != "" {
		cfg.Host = v
	}

	if v := os.Getenv("SPECULUM_READ_TIMEOUT"); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.New("SPECULUM_READ_TIMEOUT must be a valid duration (e.g., 30s)")
		}
		cfg.ReadTimeout = duration
	}

	if v := os.Getenv("SPECULUM_WRITE_TIMEOUT"); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.New("SPECULUM_WRITE_TIMEOUT must be a valid duration (e.g., 30s)")
		}
		cfg.WriteTimeout = duration
	}

	if v := os.Getenv("SPECULUM_SHUTDOWN_TIMEOUT"); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.New("SPECULUM_SHUTDOWN_TIMEOUT must be a valid duration (e.g., 30s)")
		}
		cfg.ShutdownTimeout = duration
	}

	if v := os.Getenv("SPECULUM_STORAGE_TYPE"); v != "" {
		cfg.StorageType = v
	}

	if v := os.Getenv("SPECULUM_CACHE_DIR"); v != "" {
		cfg.CacheDir = v
	}

	if v := os.Getenv("SPECULUM_UPSTREAM_REGISTRY"); v != "" {
		cfg.UpstreamRegistry = v
	}

	if v := os.Getenv("SPECULUM_UPSTREAM_TIMEOUT"); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return nil, errors.New("SPECULUM_UPSTREAM_TIMEOUT must be a valid duration (e.g., 60s)")
		}
		cfg.UpstreamTimeout = duration
	}

	if v := os.Getenv("SPECULUM_UPSTREAM_MAX_RETRIES"); v != "" {
		retries, err := strconv.Atoi(v)
		if err != nil {
			return nil, errors.New("SPECULUM_UPSTREAM_MAX_RETRIES must be a valid integer")
		}
		cfg.MaxRetries = retries
	}

	if v := os.Getenv("SPECULUM_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}

	if v := os.Getenv("SPECULUM_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("SPECULUM_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}

	if v := os.Getenv("SPECULUM_METRICS_ENABLED"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, errors.New("SPECULUM_METRICS_ENABLED must be true or false")
		}
		cfg.MetricsEnabled = enabled
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that configuration values are valid
func (c *Config) Validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	if c.ReadTimeout <= 0 {
		return errors.New("read timeout must be positive")
	}

	if c.WriteTimeout <= 0 {
		return errors.New("write timeout must be positive")
	}

	if c.ShutdownTimeout <= 0 {
		return errors.New("shutdown timeout must be positive")
	}

	if c.UpstreamTimeout <= 0 {
		return errors.New("upstream timeout must be positive")
	}

	if c.MaxRetries < 0 {
		return errors.New("max retries must not be negative")
	}

	if c.CacheDir == "" {
		return errors.New("cache directory must not be empty")
	}

	if c.BaseURL == "" {
		return errors.New("base URL must not be empty")
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		return errors.New("log level must be debug, info, warn, or error")
	}

	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.LogFormat] {
		return errors.New("log format must be json or text")
	}

	return nil
}
