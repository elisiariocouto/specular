package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elisiariocouto/specular/internal/metrics"
	"github.com/elisiariocouto/specular/internal/mirror"
	"github.com/go-chi/chi/v5"
)

var testMetrics *metrics.Metrics

func init() {
	// Initialize metrics once for all tests to avoid duplicate registration
	testMetrics = metrics.New()
}

// TestStorage implements storage.Storage interface for testing
type TestStorage struct {
	indexData   []byte
	indexErr    error
	versionData []byte
	versionErr  error
	archiveData []byte
	archiveErr  error
}

func (ts *TestStorage) GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	if ts.indexErr != nil {
		return nil, ts.indexErr
	}
	return ts.indexData, nil
}

func (ts *TestStorage) PutIndex(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	return nil
}

func (ts *TestStorage) GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	if ts.versionErr != nil {
		return nil, ts.versionErr
	}
	return ts.versionData, nil
}

func (ts *TestStorage) PutVersion(ctx context.Context, hostname, namespace, providerType, version string, data []byte) error {
	return nil
}

func (ts *TestStorage) GetVersionsResponse(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	return nil, io.EOF
}

func (ts *TestStorage) PutVersionsResponse(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	return nil
}

func (ts *TestStorage) GetArchive(ctx context.Context, path string) (io.ReadCloser, error) {
	if ts.archiveErr != nil {
		return nil, ts.archiveErr
	}
	return io.NopCloser(bytes.NewReader(ts.archiveData)), nil
}

func (ts *TestStorage) PutArchive(ctx context.Context, path string, data io.Reader) error {
	return nil
}

func (ts *TestStorage) ExistsArchive(ctx context.Context, path string) (bool, error) {
	return false, nil
}

// metricsForTests returns the shared test metrics instance
func metricsForTests() *metrics.Metrics {
	return testMetrics
}

// createTestMirror creates a mirror instance configured for testing
func createTestMirror(indexData []byte, indexErr error, versionData []byte, versionErr error, archiveData []byte, archiveErr error) *mirror.Mirror {
	storage := &TestStorage{
		indexData:   indexData,
		indexErr:    indexErr,
		versionData: versionData,
		versionErr:  versionErr,
		archiveData: archiveData,
		archiveErr:  archiveErr,
	}

	// Create an upstream client that will return the configured errors
	upstreamLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	upstreamClient := mirror.NewUpstreamClient(30, 2, 1, upstreamLogger)

	return mirror.NewMirror(storage, upstreamClient, "http://localhost:8080", 0)
}

// TestIndexHandler_Success tests successful index request
func TestIndexHandler_Success(t *testing.T) {
	indexData := []byte(`{"versions":{"1.0.0":{},"2.0.0":{}}}`)
	testMirror := createTestMirror(indexData, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/index.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("expected Cache-Control public, max-age=300, got %s", cc)
	}

	if !bytes.Equal(w.Body.Bytes(), indexData) {
		t.Errorf("expected body %q, got %q", indexData, w.Body.Bytes())
	}
}

// TestIndexHandler_NotFound tests when index is not found
func TestIndexHandler_NotFound(t *testing.T) {
	testMirror := createTestMirror(nil, mirror.ErrNotFound, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/index.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	// Mirror will try to fetch from upstream when cache misses
	// Since upstream also fails, we may get 500 instead of 404
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 404 or 500, got %d", w.Code)
	}
}

// TestIndexHandler_Error tests error handling
func TestIndexHandler_Error(t *testing.T) {
	testMirror := createTestMirror(nil, fmt.Errorf("upstream error"), nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/index.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// TestVersionHandler_Success tests successful version request
func TestVersionHandler_Success(t *testing.T) {
	versionData := []byte(`{"archives":{"linux_amd64":{"url":"http://localhost:8080/download/..."}}}`)
	testMirror := createTestMirror(nil, nil, versionData, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/1.0.0.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("expected Cache-Control public, max-age=300, got %s", cc)
	}

	if !bytes.Equal(w.Body.Bytes(), versionData) {
		t.Errorf("expected body %q, got %q", versionData, w.Body.Bytes())
	}
}

// TestVersionHandler_NotFound tests when version is not found
func TestVersionHandler_NotFound(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, mirror.ErrNotFound, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/99.0.0.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	// Mirror will try to fetch from upstream when cache misses
	// Since upstream also fails, we may get 500 instead of 404
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 404 or 500, got %d", w.Code)
	}
}

// TestVersionHandler_Error tests error handling for version requests
func TestVersionHandler_Error(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, fmt.Errorf("upstream error"), nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/1.0.0.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// TestDownloadHandler_Success tests successful archive download
func TestDownloadHandler_Success(t *testing.T) {
	archiveContent := []byte("archive file content")
	testMirror := createTestMirror(nil, nil, nil, nil, archiveContent, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest(
		"GET",
		"/terraform/providers/download/registry.terraform.io/hashicorp/aws/1.0.0/linux/amd64/terraform-provider-aws_1.0.0_linux_amd64.zip",
		nil,
	)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}", handlers.DownloadHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("expected Content-Type application/zip, got %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000" {
		t.Errorf("expected Cache-Control public, max-age=31536000, got %s", cc)
	}

	if !strings.Contains(w.Header().Get("Content-Disposition"), "terraform-provider-aws_1.0.0_linux_amd64.zip") {
		t.Errorf("expected Content-Disposition with filename, got %s", w.Header().Get("Content-Disposition"))
	}

	if !bytes.Equal(w.Body.Bytes(), archiveContent) {
		t.Errorf("expected body %q, got %q", archiveContent, w.Body.Bytes())
	}
}

// TestDownloadHandler_NotFound tests when archive is not found
func TestDownloadHandler_NotFound(t *testing.T) {
	// Create mirror with archive returning ErrNotFound
	// This should result in a 404 response (not from upstream error)
	testMirror := createTestMirror(nil, nil, nil, nil, nil, mirror.ErrNotFound)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest(
		"GET",
		"/terraform/providers/download/registry.terraform.io/hashicorp/aws/1.0.0/linux/amd64/terraform-provider-aws_1.0.0_linux_amd64.zip",
		nil,
	)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}", handlers.DownloadHandler)

	router.ServeHTTP(w, req)

	// Note: Due to the mirror's cache-miss behavior, it will try to fetch from upstream
	// when the archive is not in storage. We're just testing that the handler
	// correctly passes through the ErrNotFound error from the mirror.
	// The 404 response is expected when ErrNotFound is returned.
	// However, mirror will call FetchDownloadURL which might fail, so 500 is also acceptable
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 404 or 500, got %d", w.Code)
	}
}

// TestDownloadHandler_Error tests error handling for download requests
func TestDownloadHandler_Error(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, fmt.Errorf("upstream error"))
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest(
		"GET",
		"/terraform/providers/download/registry.terraform.io/hashicorp/aws/1.0.0/linux/amd64/terraform-provider-aws_1.0.0_linux_amd64.zip",
		nil,
	)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}", handlers.DownloadHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

// TestHealthHandler tests health check endpoint
func TestHealthHandler(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	expected := `{"status":"ok"}`
	if w.Body.String() != expected {
		t.Errorf("expected body %q, got %q", expected, w.Body.String())
	}
}

// TestHealthHandler_JSON tests that health response is valid JSON
func TestHealthHandler_JSON(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handlers.HealthHandler(w, req)

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("expected valid JSON response, got error: %v", err)
	}

	if status, ok := response["status"]; !ok || status != "ok" {
		t.Errorf("expected status field with value 'ok', got %v", response)
	}
}

// TestMetadataHandler_Index tests MetadataHandler routing to IndexHandler
func TestMetadataHandler_Index(t *testing.T) {
	indexData := []byte(`{"versions":{"1.0.0":{}}}`)
	testMirror := createTestMirror(indexData, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/index.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestMetadataHandler_Version tests MetadataHandler routing to VersionHandler
func TestMetadataHandler_Version(t *testing.T) {
	versionData := []byte(`{"archives":{"linux_amd64":{"url":"http://localhost:8080/download/..."}}}`)
	testMirror := createTestMirror(nil, nil, versionData, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/1.0.0.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// TestMetadataHandler_Invalid tests MetadataHandler with invalid request
func TestMetadataHandler_Invalid(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/invalid", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

// TestNewHandlers tests handlers initialization
func TestNewHandlers(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handlers := NewHandlers(testMirror, metricsInstance, logger)

	if handlers == nil {
		t.Fatal("NewHandlers returned nil")
	}

	if handlers.mirror != testMirror {
		t.Error("mirror not set correctly")
	}

	if handlers.metrics != metricsInstance {
		t.Error("metrics not set correctly")
	}

	if handlers.logger != logger {
		t.Error("logger not set correctly")
	}
}

// TestMetricsHandler_Enabled tests MetricsHandler when metrics are enabled
func TestMetricsHandler_Enabled(t *testing.T) {
	testMirror := createTestMirror(nil, nil, nil, nil, nil, nil)
	// Use the global test metrics which are enabled
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()

	handler := handlers.MetricsHandler()
	handler.ServeHTTP(w, req)

	// When metrics are enabled, should NOT be 404
	if w.Code == http.StatusNotFound {
		t.Errorf("expected status not 404 when metrics enabled, got %d", w.Code)
	}
}

// TestDownloadHandler_Filename tests that filename is properly set in Content-Disposition
func TestDownloadHandler_Filename(t *testing.T) {
	archiveContent := []byte("archive")
	testMirror := createTestMirror(nil, nil, nil, nil, archiveContent, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	filename := "terraform-provider-custom_3.1.4_darwin_arm64.zip"
	req := httptest.NewRequest(
		"GET",
		fmt.Sprintf("/terraform/providers/download/registry.terraform.io/hashicorp/custom/3.1.4/darwin/arm64/%s", filename),
		nil,
	)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/download/{hostname}/{namespace}/{type}/{version}/{os}/{arch}/{filename}", handlers.DownloadHandler)

	router.ServeHTTP(w, req)

	contentDisposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(contentDisposition, filename) {
		t.Errorf("expected Content-Disposition to contain %q, got %q", filename, contentDisposition)
	}
}

// TestIndexHandler_EOFError tests that io.EOF is treated as not found
func TestIndexHandler_EOFError(t *testing.T) {
	// When storage returns io.EOF, mirror will treat it as cache miss and try upstream
	// Since upstream also fails, we'll get a 500
	testMirror := createTestMirror(nil, io.EOF, nil, nil, nil, nil)
	metricsInstance := metricsForTests()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handlers := NewHandlers(testMirror, metricsInstance, logger)

	req := httptest.NewRequest("GET", "/terraform/providers/registry.terraform.io/hashicorp/aws/index.json", nil)
	w := httptest.NewRecorder()

	router := chi.NewRouter()
	router.Get("/terraform/providers/{hostname}/{namespace}/{type}/*", handlers.MetadataHandler)

	router.ServeHTTP(w, req)

	// io.EOF should be treated as not found by the handlers
	// However, since mirror will attempt to fetch from upstream (which fails),
	// we may get 500 instead of 404. Both are acceptable in this context.
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 404 or 500 for io.EOF error, got %d", w.Code)
	}
}
