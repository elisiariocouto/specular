package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/elisiariocouto/speculum/internal/metrics"
	"github.com/go-chi/chi/v5/middleware"
)

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get request ID from context (set by chi middleware)
			requestID := middleware.GetReqID(r.Context())

			logger.InfoContext(r.Context(), "request started",
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

			logger.InfoContext(r.Context(), "request completed",
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
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.String("request_id", requestID),
						slog.String("error", fmt.Sprintf("%v", err)),
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

// CloseNotifier returns a channel that receives notification when the client closes the connection
func (rw *responseWriter) CloseNotify() <-chan bool {
	if cn, ok := rw.ResponseWriter.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return make(<-chan bool)
}

// ReadCloserWithSize wraps an io.ReadCloser and tracks bytes read
type ReadCloserWithSize struct {
	io.ReadCloser
	bytesRead int64
}

// Read reads from the underlying reader and tracks bytes read
func (r *ReadCloserWithSize) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.bytesRead += int64(n)
	return n, err
}

// BytesRead returns the total bytes read
func (r *ReadCloserWithSize) BytesRead() int64 {
	return r.bytesRead
}
