package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/elisiariocouto/specular/internal/metrics"
	"github.com/elisiariocouto/specular/internal/mirror"
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

// writeJSONResponse is a helper that writes JSON response with standard headers
func writeJSONResponse(w http.ResponseWriter, data []byte, cacheMaxAge string) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", cacheMaxAge)
	_, err := w.Write(data)
	return err
}

// handleRequest is a helper that handles the common request/error/metrics pattern
// It takes a fetch function that retrieves the data and a write function that writes the response
func (h *Handlers) handleRequest(
	w http.ResponseWriter,
	r *http.Request,
	resourceType string,
	logAttrs []slog.Attr,
	fetchData func() (any, error),
	writeResponse func(any) error,
) {
	// Convert attrs to both message parts and structured fields
	attrs := make([]any, len(logAttrs))
	msgParts := make([]string, 0, len(logAttrs))
	for i, attr := range logAttrs {
		attrs[i] = attr
		msgParts = append(msgParts, fmt.Sprintf("%s=%v", attr.Key, attr.Value))
	}
	enrichedMsg := fmt.Sprintf("%s request [%s]", resourceType, strings.Join(msgParts, " "))

	// Log request
	h.logger.InfoContext(r.Context(), enrichedMsg, attrs...)

	// Fetch data and measure duration
	start := time.Now()
	data, err := fetchData()
	duration := time.Since(start).Seconds()

	// Handle errors
	if err != nil {
		if err == mirror.ErrNotFound || err == io.EOF {
			h.metrics.RecordCacheMiss(resourceType)
			notFoundMsg := fmt.Sprintf("%s not found [%s]", resourceType, strings.Join(msgParts, " "))
			h.logger.InfoContext(r.Context(), notFoundMsg, attrs...)
			http.NotFound(w, r)
			return
		}

		h.metrics.RecordError(resourceType+"_handler", "fetch_failed")
		errMsg := fmt.Sprintf("failed to get %s [%s error=%s]", resourceType, strings.Join(msgParts, " "), err.Error())
		h.logger.ErrorContext(r.Context(), errMsg,
			append(attrs, slog.String("error", err.Error()))...)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Record success metrics
	h.metrics.RecordCacheHit(resourceType)
	h.metrics.RecordUpstreamRequest(http.StatusOK, duration, resourceType)

	// Write response
	if err := writeResponse(data); err != nil {
		h.logger.ErrorContext(r.Context(),
			fmt.Sprintf("failed to write response [error=%s]", err.Error()),
			slog.String("error", err.Error()))
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

	h.handleRequest(w, r, "index",
		[]slog.Attr{
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
		},
		func() (any, error) {
			return h.mirror.GetIndex(r.Context(), hostname, namespace, providerType)
		},
		func(data any) error {
			return writeJSONResponse(w, data.([]byte), "public, max-age=300")
		},
	)
}

// VersionHandlerWithParams handles version requests with explicit version parameter
func (h *Handlers) VersionHandlerWithParams(w http.ResponseWriter, r *http.Request, version string) {
	hostname := chi.URLParam(r, "hostname")
	namespace := chi.URLParam(r, "namespace")
	providerType := chi.URLParam(r, "type")

	h.handleRequest(w, r, "version",
		[]slog.Attr{
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
			slog.String("version", version),
		},
		func() (any, error) {
			return h.mirror.GetVersion(r.Context(), hostname, namespace, providerType, version)
		},
		func(data any) error {
			return writeJSONResponse(w, data.([]byte), "public, max-age=300")
		},
	)
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

	h.handleRequest(w, r, "archive",
		[]slog.Attr{
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
			slog.String("version", version),
			slog.String("os", os),
			slog.String("arch", arch),
			slog.String("filename", filename),
		},
		func() (any, error) {
			return h.mirror.GetArchive(r.Context(), hostname, namespace, providerType, version, os, arch, archivePath)
		},
		func(data any) error {
			reader := data.(io.ReadCloser)
			defer reader.Close()

			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year cache for immutable archives
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

			_, err := io.Copy(w, reader)
			return err
		},
	)
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
		if !h.metrics.Enabled() {
			http.NotFound(w, r)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
	})
}
