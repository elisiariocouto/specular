package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/elisiariocouto/speculum/internal/metrics"
	"github.com/elisiariocouto/speculum/internal/mirror"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handlers holds dependencies for HTTP handlers
type Handlers struct {
	mirror  *mirror.Mirror
	metrics *metrics.Metrics
	logger  *slog.Logger
}

// NewHandlers creates a new handlers instance
func NewHandlers(m *mirror.Mirror, metrics *metrics.Metrics, logger *slog.Logger) *Handlers {
	return &Handlers{
		mirror:  m,
		metrics: metrics,
		logger:  logger,
	}
}

// MetadataHandler handles index.json, version.json, and archive requests
// Routes: /:hostname/:namespace/:type/index.json, /:hostname/:namespace/:type/:version.json, or /:hostname/:namespace/:type/archive.zip
func (h *Handlers) MetadataHandler(w http.ResponseWriter, r *http.Request) {
	tail := chi.URLParam(r, "*")

	// Check if this is an index.json request
	if tail == "index.json" {
		h.IndexHandler(w, r)
		return
	}

	// Check if tail matches version.json pattern (e.g., "6.26.0.json")
	if strings.HasSuffix(tail, ".json") {
		// Extract version from tail by removing the .json suffix
		version := strings.TrimSuffix(tail, ".json")
		h.VersionHandlerWithParams(w, r, version)
		return
	}

	// Note: Archive downloads are now handled by the dedicated /download endpoint
	// Archive URLs in version.json point to /download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}
	// This makes the old .zip handling here obsolete

	// Not a valid request
	http.Error(w, "Not Found", http.StatusNotFound)
}

// IndexHandler handles GET /:hostname/:namespace/:type/index.json
func (h *Handlers) IndexHandler(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	namespace := chi.URLParam(r, "namespace")
	providerType := chi.URLParam(r, "type")

	h.logger.InfoContext(r.Context(), "index request",
		slog.String("hostname", hostname),
		slog.String("namespace", namespace),
		slog.String("type", providerType),
	)

	start := time.Now()
	data, err := h.mirror.GetIndex(r.Context(), hostname, namespace, providerType)
	duration := time.Since(start).Seconds()

	if err != nil {
		if err == mirror.ErrNotFound {
			h.metrics.RecordCacheMiss("index")
			h.logger.InfoContext(r.Context(), "provider not found",
				slog.String("hostname", hostname),
				slog.String("namespace", namespace),
				slog.String("type", providerType),
			)
			http.NotFound(w, r)
			return
		}

		h.metrics.RecordError("index_handler", "fetch_failed")
		h.logger.ErrorContext(r.Context(), "failed to get index",
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.metrics.RecordCacheHit("index")
	h.metrics.RecordUpstreamRequest(http.StatusOK, duration, "index")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if _, err := w.Write(data); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to write response", slog.String("error", err.Error()))
	}
}

// VersionHandlerWithParams handles version requests with explicit version parameter
func (h *Handlers) VersionHandlerWithParams(w http.ResponseWriter, r *http.Request, version string) {
	hostname := chi.URLParam(r, "hostname")
	namespace := chi.URLParam(r, "namespace")
	providerType := chi.URLParam(r, "type")

	h.logger.InfoContext(r.Context(), "version request",
		slog.String("hostname", hostname),
		slog.String("namespace", namespace),
		slog.String("type", providerType),
		slog.String("version", version),
	)

	start := time.Now()
	data, err := h.mirror.GetVersion(r.Context(), hostname, namespace, providerType, version)
	duration := time.Since(start).Seconds()

	if err != nil {
		if err == mirror.ErrNotFound {
			h.metrics.RecordCacheMiss("version")
			h.logger.InfoContext(r.Context(), "version not found",
				slog.String("hostname", hostname),
				slog.String("namespace", namespace),
				slog.String("type", providerType),
				slog.String("version", version),
			)
			http.NotFound(w, r)
			return
		}

		h.metrics.RecordError("version_handler", "fetch_failed")
		h.logger.ErrorContext(r.Context(), "failed to get version",
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
			slog.String("version", version),
			slog.String("error", err.Error()),
		)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.metrics.RecordCacheHit("version")
	h.metrics.RecordUpstreamRequest(http.StatusOK, duration, "version")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if _, err := w.Write(data); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to write response", slog.String("error", err.Error()))
	}
}

// VersionHandler handles GET /:hostname/:namespace/:type/:version.json (legacy, kept for compatibility)
func (h *Handlers) VersionHandler(w http.ResponseWriter, r *http.Request) {
	version := chi.URLParam(r, "version")
	h.VersionHandlerWithParams(w, r, version)
}

// DownloadHandler handles archive downloads with explicit parameters
// Route: /download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}
func (h *Handlers) DownloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract all parameters from URL
	hostname := chi.URLParam(r, "hostname")
	namespace := chi.URLParam(r, "namespace")
	providerType := chi.URLParam(r, "type")
	version := chi.URLParam(r, "version")
	os := chi.URLParam(r, "os")
	arch := chi.URLParam(r, "arch")
	filename := chi.URLParam(r, "filename")

	// Construct cache path
	archivePath := fmt.Sprintf("%s/%s/%s/%s", hostname, namespace, providerType, filename)

	h.logger.InfoContext(r.Context(), "download request",
		slog.String("hostname", hostname),
		slog.String("namespace", namespace),
		slog.String("type", providerType),
		slog.String("version", version),
		slog.String("os", os),
		slog.String("arch", arch),
		slog.String("filename", filename))

	start := time.Now()
	reader, err := h.mirror.GetArchive(r.Context(), hostname, namespace, providerType, version, os, arch, archivePath)
	duration := time.Since(start).Seconds()

	if err != nil {
		if err == io.EOF || err == mirror.ErrNotFound {
			h.metrics.RecordCacheMiss("archive")
			h.logger.InfoContext(r.Context(), "archive not found",
				slog.String("path", archivePath))
			http.NotFound(w, r)
			return
		}

		h.metrics.RecordError("download_handler", "fetch_failed")
		h.logger.ErrorContext(r.Context(), "failed to get archive",
			slog.String("path", archivePath),
			slog.String("error", err.Error()))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	h.metrics.RecordCacheHit("archive")
	h.metrics.RecordUpstreamRequest(http.StatusOK, duration, "archive")

	// Set response headers for archive download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year cache for immutable archives
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

	// Stream the archive to the client
	if _, err := io.Copy(w, reader); err != nil {
		h.logger.ErrorContext(r.Context(), "failed to stream archive", slog.String("error", err.Error()))
	}
}

// HealthHandler handles GET /health
func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status":"ok"}`)
}

// MetricsHandler returns the Prometheus metrics handler
// Returns 404 if metrics are disabled
func (h *Handlers) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.metrics == nil {
			http.NotFound(w, r)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
	})
}
