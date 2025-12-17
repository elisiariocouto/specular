package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/elisiariocouto/speculum/internal/metrics"
	"github.com/elisiariocouto/speculum/internal/mirror"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server represents the HTTP server
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates and configures a new HTTP server
func New(
	host string,
	port int,
	readTimeout time.Duration,
	writeTimeout time.Duration,
	m *mirror.Mirror,
	metrics *metrics.Metrics,
	logger *slog.Logger,
) *Server {
	router := chi.NewRouter()

	// Global middleware
	router.Use(middleware.RequestID)
	router.Use(RecoveryMiddleware(logger))
	router.Use(LoggingMiddleware(logger))
	router.Use(MetricsMiddleware(metrics))

	// Create handlers
	handlers := NewHandlers(m, metrics, logger)

	// Routes
	router.Get("/health", handlers.HealthHandler)
	router.Handle("/metrics", handlers.MetricsHandler())

	// Terraform provider mirror protocol endpoints under /terraform/providers base path
	// This allows for future support of other registries (e.g., /docker/registries, /npm, /pypi)
	router.Route("/terraform/providers", func(r chi.Router) {
		// GET /terraform/providers/:hostname/:namespace/:type/* (catches index.json, version.json, and archives)
		// Use wildcard to handle dots in version numbers (e.g., 6.26.0.json) and zip files
		r.Get("/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)
	})

	// 404 handler
	router.NotFound(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":"not found"}`)
	}))

	httpServer := &http.Server{
		Addr:         net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		Handler:      router,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  120 * time.Second,
	}

	return &Server{
		httpServer: httpServer,
		logger:     logger,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.InfoContext(context.Background(), "starting HTTP server",
		slog.String("address", s.httpServer.Addr),
	)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.InfoContext(ctx, "shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
