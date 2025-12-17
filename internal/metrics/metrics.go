package metrics

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics
type Metrics struct {
	// HTTP request metrics
	HTTPRequestsTotal  prometheus.CounterVec
	HTTPRequestDuration prometheus.HistogramVec
	HTTPRequestSize     prometheus.HistogramVec
	HTTPResponseSize    prometheus.HistogramVec

	// Cache metrics
	CacheHitsTotal   prometheus.CounterVec
	CacheMissesTotal prometheus.CounterVec

	// Upstream metrics
	UpstreamRequestsTotal    prometheus.CounterVec
	UpstreamRequestDuration  prometheus.HistogramVec
	UpstreamErrors           prometheus.CounterVec

	// Storage metrics
	StorageOperationsTotal   prometheus.CounterVec
	StorageOperationDuration prometheus.HistogramVec

	// Error metrics
	ErrorsTotal prometheus.CounterVec
}

// New creates and registers all metrics
func New() *Metrics {
	m := &Metrics{
		HTTPRequestsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),

		HTTPRequestDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "speculum_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "path"},
		),

		HTTPRequestSize: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "speculum_http_request_size_bytes",
				Help:    "HTTP request size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path"},
		),

		HTTPResponseSize: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "speculum_http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: prometheus.ExponentialBuckets(100, 10, 8),
			},
			[]string{"method", "path", "status"},
		),

		CacheHitsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"cache_type"},
		),

		CacheMissesTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"cache_type"},
		),

		UpstreamRequestsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_upstream_requests_total",
				Help: "Total number of upstream registry requests",
			},
			[]string{"status"},
		),

		UpstreamRequestDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "speculum_upstream_request_duration_seconds",
				Help:    "Upstream request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),

		UpstreamErrors: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_upstream_errors_total",
				Help: "Total number of upstream errors",
			},
			[]string{"error_type"},
		),

		StorageOperationsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_storage_operations_total",
				Help: "Total number of storage operations",
			},
			[]string{"operation", "status"},
		),

		StorageOperationDuration: *promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "speculum_storage_operation_duration_seconds",
				Help:    "Storage operation duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation"},
		),

		ErrorsTotal: *promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "speculum_errors_total",
				Help: "Total number of errors",
			},
			[]string{"component", "error_type"},
		),
	}

	return m
}

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, path string, status int, duration float64, reqSize, respSize int64) {
	statusStr := fmt.Sprintf("%d", status)
	m.HTTPRequestsTotal.WithLabelValues(method, path, statusStr).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	m.HTTPRequestSize.WithLabelValues(method, path).Observe(float64(reqSize))
	m.HTTPResponseSize.WithLabelValues(method, path, statusStr).Observe(float64(respSize))
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cacheType string) {
	m.CacheHitsTotal.WithLabelValues(cacheType).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cacheType string) {
	m.CacheMissesTotal.WithLabelValues(cacheType).Inc()
}

// RecordUpstreamRequest records an upstream request
func (m *Metrics) RecordUpstreamRequest(status int, duration float64, endpoint string) {
	statusStr := fmt.Sprintf("%d", status)
	m.UpstreamRequestsTotal.WithLabelValues(statusStr).Inc()
	m.UpstreamRequestDuration.WithLabelValues(endpoint).Observe(duration)
}

// RecordUpstreamError records an upstream error
func (m *Metrics) RecordUpstreamError(errorType string) {
	m.UpstreamErrors.WithLabelValues(errorType).Inc()
}

// RecordStorageOperation records a storage operation
func (m *Metrics) RecordStorageOperation(operation, status string, duration float64) {
	m.StorageOperationsTotal.WithLabelValues(operation, status).Inc()
	m.StorageOperationDuration.WithLabelValues(operation).Observe(duration)
}

// RecordError records an error
func (m *Metrics) RecordError(component, errorType string) {
	m.ErrorsTotal.WithLabelValues(component, errorType).Inc()
}
