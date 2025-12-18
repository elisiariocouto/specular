package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/elisiariocouto/speculum/internal/config"
	"github.com/elisiariocouto/speculum/internal/logger"
	"github.com/elisiariocouto/speculum/internal/metrics"
	"github.com/elisiariocouto/speculum/internal/mirror"
	"github.com/elisiariocouto/speculum/internal/server"
	"github.com/elisiariocouto/speculum/internal/storage"
	"github.com/elisiariocouto/speculum/internal/version"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Setup logger
	log := logger.SetupLogger(cfg.LogLevel, cfg.LogFormat)

	log.InfoContext(context.Background(), "Speculum starting",
		slog.String("version", version.Version),
		slog.String("commit", version.Commit),
		slog.String("build_date", version.BuildDate),
		slog.Int("port", cfg.Port),
		slog.String("host", cfg.Host),
		slog.String("storage_type", cfg.StorageType),
		slog.String("cache_dir", cfg.CacheDir),
		slog.String("base_url", cfg.BaseURL),
	)

	// Initialize storage backend
	var storageBackend storage.Storage
	switch cfg.StorageType {
	case "filesystem":
		st, err := storage.NewFilesystemStorage(cfg.CacheDir)
		if err != nil {
			log.ErrorContext(context.Background(), "Failed to initialize filesystem storage",
				slog.String("error", err.Error()))
			os.Exit(1)
		}
		storageBackend = st
		log.InfoContext(context.Background(), "Filesystem storage initialized",
			slog.String("cache_dir", cfg.CacheDir))
	case "memory":
		storageBackend = storage.NewMemoryStorage()
		log.InfoContext(context.Background(), "In-memory storage initialized")
	default:
		log.ErrorContext(context.Background(), "Unknown storage type",
			slog.String("storage_type", cfg.StorageType))
		os.Exit(1)
	}

	// Initialize upstream client
	upstreamClient := mirror.NewUpstreamClient(
		cfg.UpstreamTimeout,
		cfg.MaxRetries,
		cfg.DiscoveryCacheTTL,
		log,
	)

	// Initialize mirror service
	mirrorService := mirror.NewMirror(storageBackend, upstreamClient, cfg.BaseURL)

	// Initialize metrics conditionally
	var m *metrics.Metrics
	if cfg.MetricsEnabled {
		m = metrics.New()
		log.InfoContext(context.Background(), "metrics enabled")
	} else {
		m = nil
		log.InfoContext(context.Background(), "metrics disabled")
	}

	// Create HTTP server
	httpServer := server.New(
		cfg.Host,
		cfg.Port,
		cfg.ReadTimeout,
		cfg.WriteTimeout,
		mirrorService,
		m,
		log,
	)

	// Start server in a goroutine
	go func() {
		if err := httpServer.Start(); err != nil {
			if err.Error() != "http: Server closed" {
				log.ErrorContext(context.Background(), "Server error",
					slog.String("error", err.Error()))
				os.Exit(1)
			}
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigChan
	log.InfoContext(context.Background(), "Received signal",
		slog.String("signal", sig.String()))

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.ErrorContext(context.Background(), "Shutdown error",
			slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.InfoContext(context.Background(), "Speculum shutdown complete")
}
