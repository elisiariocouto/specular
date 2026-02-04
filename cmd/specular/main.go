package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/elisiariocouto/specular/internal/config"
	"github.com/elisiariocouto/specular/internal/logger"
	"github.com/elisiariocouto/specular/internal/metrics"
	"github.com/elisiariocouto/specular/internal/mirror"
	"github.com/elisiariocouto/specular/internal/server"
	"github.com/elisiariocouto/specular/internal/storage"
	"github.com/elisiariocouto/specular/internal/version"
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

	log.InfoContext(context.Background(),
		fmt.Sprintf("Specular starting [version=%s commit=%s build_date=%s port=%d host=%s storage_type=%s cache_dir=%s base_url=%s]",
			version.Version, version.Commit, version.BuildDate, cfg.Port, cfg.Host, cfg.StorageType, cfg.CacheDir, cfg.BaseURL),
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
			log.ErrorContext(context.Background(),
				fmt.Sprintf("Failed to initialize filesystem storage [error=%s]", err.Error()),
				slog.String("error", err.Error()))
			os.Exit(1)
		}
		storageBackend = st
		log.InfoContext(context.Background(),
			fmt.Sprintf("Filesystem storage initialized [cache_dir=%s]", cfg.CacheDir),
			slog.String("cache_dir", cfg.CacheDir))
	case "memory":
		storageBackend = storage.NewMemoryStorage()
		log.InfoContext(context.Background(), "In-memory storage initialized")
	default:
		log.ErrorContext(context.Background(),
			fmt.Sprintf("Unknown storage type [storage_type=%s]", cfg.StorageType),
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
		m = metrics.Noop()
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
				log.ErrorContext(context.Background(),
					fmt.Sprintf("Server error [error=%s]", err.Error()),
					slog.String("error", err.Error()))
				os.Exit(1)
			}
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	sig := <-sigChan
	log.InfoContext(context.Background(),
		fmt.Sprintf("Received signal [signal=%s]", sig.String()),
		slog.String("signal", sig.String()))

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.ErrorContext(context.Background(),
			fmt.Sprintf("Shutdown error [error=%s]", err.Error()),
			slog.String("error", err.Error()))
		os.Exit(1)
	}

	log.InfoContext(context.Background(), "Specular shutdown complete")
}
