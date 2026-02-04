package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/elisiariocouto/specular/internal/metrics"
	"github.com/go-chi/chi/v5/middleware"
)

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get request ID from context (set by chi middleware)
			requestID := middleware.GetReqID(r.Context())

			logger.InfoContext(r.Context(),
				fmt.Sprintf("request started [request_id=%s method=%s path=%s remote_addr=%s]",
					requestID, r.Method, r.URL.Path, r.RemoteAddr),
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
			)

			// Wrap response writer to capture status code and response size
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			start := time.Now()
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start)

			logger.InfoContext(r.Context(),
				fmt.Sprintf("request completed [request_id=%s method=%s path=%s status_code=%d duration=%s response_size=%d]",
					requestID, r.Method, r.URL.Path, wrapped.statusCode, duration, wrapped.responseSize),
				slog.String("request_id", requestID),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status_code", wrapped.statusCode),
				slog.Duration("duration", duration),
				slog.Int64("response_size", wrapped.responseSize),
			)
		})
	}
}

// MetricsMiddleware records metrics for HTTP requests
func MetricsMiddleware(m *metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap response writer to capture status code and response size
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Get request size
			reqSize := r.ContentLength
			if reqSize < 0 {
				reqSize = 0
			}

			start := time.Now()
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start).Seconds()

			// Normalize path for metrics (don't include provider-specific parts)
			metricsPath := r.URL.Path
			if strings.Contains(metricsPath, "/archive-downloads/") {
				metricsPath = "/archive-downloads/*"
			}

			m.RecordHTTPRequest(r.Method, metricsPath, wrapped.statusCode, duration, reqSize, wrapped.responseSize)
		})
	}
}

// RecoveryMiddleware recovers from panics and logs them
func RecoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID := middleware.GetReqID(r.Context())
					errStr := fmt.Sprintf("%v", err)
					logger.ErrorContext(r.Context(),
						fmt.Sprintf("panic recovered [request_id=%s error=%s]", requestID, errStr),
						slog.String("request_id", requestID),
						slog.String("error", errStr),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	responseSize int64
}

// WriteHeader captures the status code
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response size
func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.responseSize += int64(n)
	return n, err
}

// Flush flushes the response writer if it supports it
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
