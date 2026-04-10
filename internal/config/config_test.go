package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Fatalf("expected default host 0.0.0.0, got %s", cfg.Host)
	}
	if cfg.StorageType != "filesystem" {
		t.Fatalf("expected default storage type filesystem, got %s", cfg.StorageType)
	}
	if cfg.CacheDir != "/var/cache/specular" {
		t.Fatalf("expected default cache dir /var/cache/specular, got %s", cfg.CacheDir)
	}
	if cfg.BaseURL != "https://specular.example.com" {
		t.Fatalf("expected default base URL https://specular.example.com, got %s", cfg.BaseURL)
	}
	if cfg.LogLevel != "info" || cfg.LogFormat != "json" {
		t.Fatalf("expected default log level info and format json, got %s/%s", cfg.LogLevel, cfg.LogFormat)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("SPECULAR_PORT", "9090")
	t.Setenv("SPECULAR_HOST", "127.0.0.1")
	t.Setenv("SPECULAR_READ_TIMEOUT", "10s")
	t.Setenv("SPECULAR_WRITE_TIMEOUT", "11s")
	t.Setenv("SPECULAR_SHUTDOWN_TIMEOUT", "12s")
	t.Setenv("SPECULAR_STORAGE_TYPE", "memory")
	t.Setenv("SPECULAR_CACHE_DIR", "/tmp/specular-cache")
	t.Setenv("SPECULAR_UPSTREAM_TIMEOUT", "13s")
	t.Setenv("SPECULAR_UPSTREAM_MAX_RETRIES", "5")
	t.Setenv("SPECULAR_INDEX_TTL", "30m")
	t.Setenv("SPECULAR_BASE_URL", "https://example.com")
	t.Setenv("SPECULAR_LOG_LEVEL", "debug")
	t.Setenv("SPECULAR_LOG_FORMAT", "text")
	t.Setenv("SPECULAR_METRICS_ENABLED", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Fatalf("expected host 127.0.0.1, got %s", cfg.Host)
	}
	if cfg.ReadTimeout != 10*time.Second || cfg.WriteTimeout != 11*time.Second || cfg.ShutdownTimeout != 12*time.Second {
		t.Fatalf("unexpected timeouts: read %v write %v shutdown %v", cfg.ReadTimeout, cfg.WriteTimeout, cfg.ShutdownTimeout)
	}
	if cfg.StorageType != "memory" || cfg.CacheDir != "/tmp/specular-cache" {
		t.Fatalf("unexpected storage settings: type %s cache %s", cfg.StorageType, cfg.CacheDir)
	}
	if cfg.UpstreamTimeout != 13*time.Second || cfg.MaxRetries != 5 {
		t.Fatalf("unexpected upstream settings: timeout %v retries %d", cfg.UpstreamTimeout, cfg.MaxRetries)
	}
	if cfg.IndexTTL != 30*time.Minute {
		t.Fatalf("expected index TTL 30m, got %v", cfg.IndexTTL)
	}
	if cfg.BaseURL != "https://example.com" {
		t.Fatalf("expected base URL https://example.com, got %s", cfg.BaseURL)
	}
	if cfg.LogLevel != "debug" || cfg.LogFormat != "text" {
		t.Fatalf("unexpected logging settings: level %s format %s", cfg.LogLevel, cfg.LogFormat)
	}
	if cfg.MetricsEnabled {
		t.Fatalf("expected metrics disabled")
	}
}

func TestLoadInvalidEnv(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		errorOn string
	}{
		{name: "port", envKey: "SPECULAR_PORT", envVal: "abc", errorOn: "SPECULAR_PORT must be a valid integer"},
		{name: "read timeout", envKey: "SPECULAR_READ_TIMEOUT", envVal: "notaduration", errorOn: "SPECULAR_READ_TIMEOUT must be a valid duration"},
		{name: "write timeout", envKey: "SPECULAR_WRITE_TIMEOUT", envVal: "1x", errorOn: "SPECULAR_WRITE_TIMEOUT must be a valid duration"},
		{name: "shutdown timeout", envKey: "SPECULAR_SHUTDOWN_TIMEOUT", envVal: "1x", errorOn: "SPECULAR_SHUTDOWN_TIMEOUT must be a valid duration"},
		{name: "upstream timeout", envKey: "SPECULAR_UPSTREAM_TIMEOUT", envVal: "1x", errorOn: "SPECULAR_UPSTREAM_TIMEOUT must be a valid duration"},
		{name: "max retries", envKey: "SPECULAR_UPSTREAM_MAX_RETRIES", envVal: "one", errorOn: "SPECULAR_UPSTREAM_MAX_RETRIES must be a valid integer"},
		{name: "index ttl", envKey: "SPECULAR_INDEX_TTL", envVal: "notaduration", errorOn: "SPECULAR_INDEX_TTL must be a valid duration"},
		{name: "metrics", envKey: "SPECULAR_METRICS_ENABLED", envVal: "maybe", errorOn: "SPECULAR_METRICS_ENABLED must be true or false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.envKey, tt.envVal)
			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for %s", tt.envKey)
			}
			if !strings.Contains(err.Error(), tt.errorOn) {
				t.Fatalf("expected error to contain %q, got %q", tt.errorOn, err.Error())
			}
		})
	}
}

func TestValidateAggregatesErrors(t *testing.T) {
	cfg := &Config{
		Port:            0,
		Host:            " ",
		ReadTimeout:     -1,
		WriteTimeout:    0,
		ShutdownTimeout: 0,
		StorageType:     "fs",
		CacheDir:        "",
		UpstreamTimeout: 0,
		MaxRetries:      -1,
		IndexTTL:        -1,
		BaseURL:         "http://",
		LogLevel:        "nope",
		LogFormat:       "xml",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation to fail")
	}

	checks := []string{
		"port must be between 1 and 65535",
		"host must not be empty",
		"read timeout must be positive",
		"write timeout must be positive",
		"shutdown timeout must be positive",
		"upstream timeout must be positive",
		"max retries must not be negative",
		"index TTL must not be negative",
		"cache directory must not be empty",
		"base URL must be a valid URL with scheme and host",
		"log level must be debug, info, warn, or error",
		"log format must be json or text",
		"storage type must be filesystem or memory",
	}

	for _, msg := range checks {
		if !strings.Contains(err.Error(), msg) {
			t.Fatalf("expected error to include %q, got %q", msg, err.Error())
		}
	}
}

func TestValidateBaseURLMissingHost(t *testing.T) {
	cfg := &Config{
		Port:            8080,
		Host:            "localhost",
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		ShutdownTimeout: time.Second,
		StorageType:     "filesystem",
		CacheDir:        "/tmp",
		UpstreamTimeout: time.Second,
		MaxRetries:      1,
		BaseURL:         "http://",
		LogLevel:        "info",
		LogFormat:       "json",
		MetricsEnabled:  true,
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base URL must be a valid URL with scheme and host") {
		t.Fatalf("expected base URL validation error, got %v", err)
	}
}

func TestValidateHostWhitespace(t *testing.T) {
	cfg := &Config{
		Port:            8080,
		Host:            "   ",
		ReadTimeout:     time.Second,
		WriteTimeout:    time.Second,
		ShutdownTimeout: time.Second,
		StorageType:     "filesystem",
		CacheDir:        "/tmp",
		UpstreamTimeout: time.Second,
		MaxRetries:      1,
		BaseURL:         "http://localhost:8080",
		LogLevel:        "info",
		LogFormat:       "json",
		MetricsEnabled:  true,
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "host must not be empty") {
		t.Fatalf("expected host validation error, got %v", err)
	}
}
