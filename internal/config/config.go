package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	UpstreamTimeout   time.Duration
	MaxRetries        int
	DiscoveryCacheTTL time.Duration

	// Mirror configuration
	BaseURL  string
	IndexTTL time.Duration

	// Observability
	LogLevel       string
	LogFormat      string
	MetricsEnabled bool
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		// Defaults
		Port:              8080,
		Host:              "0.0.0.0",
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		ShutdownTimeout:   30 * time.Second,
		StorageType:       "filesystem",
		CacheDir:          "/var/cache/specular",
		UpstreamTimeout:   60 * time.Second,
		MaxRetries:        3,
		DiscoveryCacheTTL: 1 * time.Hour,
		BaseURL:           "https://specular.example.com",
		IndexTTL:          1 * time.Hour,
		LogLevel:          "info",
		LogFormat:         "json",
		MetricsEnabled:    true,
	}

	// Override with environment variables
	if err := setEnvInt("SPECULAR_PORT", &cfg.Port, "must be a valid integer"); err != nil {
		return nil, err
	}

	if v := os.Getenv("SPECULAR_HOST"); v != "" {
		cfg.Host = v
	}

	if err := setEnvDuration("SPECULAR_READ_TIMEOUT", &cfg.ReadTimeout, "must be a valid duration (e.g., 30s)"); err != nil {
		return nil, err
	}

	if err := setEnvDuration("SPECULAR_WRITE_TIMEOUT", &cfg.WriteTimeout, "must be a valid duration (e.g., 30s)"); err != nil {
		return nil, err
	}

	if err := setEnvDuration("SPECULAR_SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout, "must be a valid duration (e.g., 30s)"); err != nil {
		return nil, err
	}

	if v := os.Getenv("SPECULAR_STORAGE_TYPE"); v != "" {
		cfg.StorageType = v
	}

	if v := os.Getenv("SPECULAR_CACHE_DIR"); v != "" {
		cfg.CacheDir = v
	}

	if err := setEnvDuration("SPECULAR_UPSTREAM_TIMEOUT", &cfg.UpstreamTimeout, "must be a valid duration (e.g., 60s)"); err != nil {
		return nil, err
	}

	if err := setEnvInt("SPECULAR_UPSTREAM_MAX_RETRIES", &cfg.MaxRetries, "must be a valid integer"); err != nil {
		return nil, err
	}

	if err := setEnvDuration("SPECULAR_DISCOVERY_CACHE_TTL", &cfg.DiscoveryCacheTTL, "must be a valid duration (e.g., 1h)"); err != nil {
		return nil, err
	}

	if err := setEnvDuration("SPECULAR_INDEX_TTL", &cfg.IndexTTL, "must be a valid duration (e.g., 1h)"); err != nil {
		return nil, err
	}

	if v := os.Getenv("SPECULAR_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}

	if v := os.Getenv("SPECULAR_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("SPECULAR_LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}

	if err := setEnvBool("SPECULAR_METRICS_ENABLED", &cfg.MetricsEnabled, "must be true or false"); err != nil {
		return nil, err
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that configuration values are valid
func (c *Config) Validate() error {
	var errs []error

	if c.Port < 1 || c.Port > 65535 {
		errs = append(errs, errors.New("port must be between 1 and 65535"))
	}

	if strings.TrimSpace(c.Host) == "" {
		errs = append(errs, errors.New("host must not be empty"))
	}

	if c.ReadTimeout <= 0 {
		errs = append(errs, errors.New("read timeout must be positive"))
	}

	if c.WriteTimeout <= 0 {
		errs = append(errs, errors.New("write timeout must be positive"))
	}

	if c.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown timeout must be positive"))
	}

	if c.UpstreamTimeout <= 0 {
		errs = append(errs, errors.New("upstream timeout must be positive"))
	}

	if c.MaxRetries < 0 {
		errs = append(errs, errors.New("max retries must not be negative"))
	}

	if c.IndexTTL < 0 {
		errs = append(errs, errors.New("index TTL must not be negative"))
	}

	if c.CacheDir == "" {
		errs = append(errs, errors.New("cache directory must not be empty"))
	}

	if c.BaseURL == "" {
		errs = append(errs, errors.New("base URL must not be empty"))
	} else {
		parsed, err := url.Parse(c.BaseURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			errs = append(errs, errors.New("base URL must be a valid URL with scheme and host"))
		}
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLogLevels[c.LogLevel] {
		errs = append(errs, errors.New("log level must be debug, info, warn, or error"))
	}

	validLogFormats := map[string]bool{
		"json": true,
		"text": true,
	}
	if !validLogFormats[c.LogFormat] {
		errs = append(errs, errors.New("log format must be json or text"))
	}

	validStorageTypes := map[string]bool{
		"filesystem": true,
		"memory":     true,
	}
	if !validStorageTypes[c.StorageType] {
		errs = append(errs, errors.New("storage type must be filesystem or memory"))
	}

	return errors.Join(errs...)
}

func setEnvInt(key string, target *int, errMsg string) error {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("%s %s", key, errMsg)
		}
		*target = parsed
	}
	return nil
}

func setEnvDuration(key string, target *time.Duration, errMsg string) error {
	if v := os.Getenv(key); v != "" {
		duration, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%s %s", key, errMsg)
		}
		*target = duration
	}
	return nil
}

func setEnvBool(key string, target *bool, errMsg string) error {
	if v := os.Getenv(key); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("%s %s", key, errMsg)
		}
		*target = parsed
	}

	return nil
}
