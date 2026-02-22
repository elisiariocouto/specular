package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestUpstreamClient(server *httptest.Server) *UpstreamClient {
	client := server.Client()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &UpstreamClient{
		httpClient:     client,
		maxRetries:     2,
		logger:         logger,
		discoveryCache: NewDiscoveryCache(1*time.Second, client, logger),
	}
}

func TestNewUpstreamClient(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := NewUpstreamClient(30*time.Second, 3, 1*time.Hour, logger)

	if client == nil {
		t.Errorf("expected non-nil client")
		return
	}
	if client.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", client.maxRetries)
	}
	if client.httpClient == nil {
		t.Errorf("expected non-nil http client")
	}
	if client.discoveryCache == nil {
		t.Errorf("expected non-nil discovery cache")
	}
}

func TestFetchArchive_ValidURL(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("archive content"))
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	archiveURL := fmt.Sprintf("https://%s/provider.zip", u.Host)

	body, err := client.FetchArchive(context.Background(), archiveURL)
	if err != nil {
		t.Fatalf("FetchArchive failed: %v", err)
	}
	defer body.Close()

	data, _ := io.ReadAll(body)
	if string(data) != "archive content" {
		t.Errorf("expected 'archive content', got %s", string(data))
	}
}

func TestFetchArchive_InvalidURL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	client := &UpstreamClient{
		httpClient: &http.Client{},
		maxRetries: 2,
		logger:     logger,
	}

	tests := []struct {
		name      string
		url       string
		wantError string
	}{
		{
			name:      "no scheme",
			url:       "localhost/file.zip",
			wantError: "must use http or https scheme",
		},
		{
			name:      "invalid scheme",
			url:       "ftp://example.com/file.zip",
			wantError: "must use http or https scheme",
		},
		{
			name:      "no host",
			url:       "https:///file.zip",
			wantError: "must have a host",
		},
		{
			name:      "malformed URL",
			url:       "ht!tp://[invalid",
			wantError: "invalid archive URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.FetchArchive(context.Background(), tt.url)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestFetchArchive_HTTPError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	archiveURL := fmt.Sprintf("https://%s/notfound.zip", u.Host)

	_, err := client.FetchArchive(context.Background(), archiveURL)
	if err == nil {
		t.Errorf("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got %v", err)
	}
}

func TestExponentialBackoff_Success(t *testing.T) {
	ctx := context.Background()

	start := time.Now()
	err := exponentialBackoff(ctx, 0)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if elapsed < 1*time.Second {
		t.Errorf("backoff duration too short: %v", elapsed)
	}
}

func TestExponentialBackoff_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := exponentialBackoff(ctx, 0)
	if err == nil {
		t.Errorf("expected context cancelled error")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestFetch_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version": "1.0.0"}`))
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	body, status, err := client.fetch(context.Background(), server.URL)

	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if !strings.Contains(string(body), "version") {
		t.Errorf("unexpected body: %s", string(body))
	}
}

func TestFetch_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	_, status, err := client.fetch(context.Background(), server.URL)

	if err != nil {
		t.Fatalf("fetch returned error for 404: %v", err)
	}
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestFetch_ServerError_WithRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Write([]byte("success"))
		}
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	body, status, err := client.fetch(context.Background(), server.URL)

	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls due to retries, got %d", callCount)
	}
	if string(body) != "success" {
		t.Errorf("expected 'success', got %s", string(body))
	}
}

func TestFetch_MaxRetriesExceeded(t *testing.T) {
	callCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	client.maxRetries = 2

	_, status, _ := client.fetch(context.Background(), server.URL)

	// With maxRetries=2, we should get 3 attempts (initial + 2 retries)
	if callCount != 3 {
		t.Errorf("expected 3 calls with maxRetries=2, got %d", callCount)
	}

	if status != http.StatusInternalServerError {
		t.Errorf("expected 500 status, got %d", status)
	}
}

func TestFetch_ContextCancellation(t *testing.T) {
	blocker := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
		w.Write([]byte("response"))
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	ctx, cancel := context.WithCancel(context.Background())

	// Start fetch in goroutine
	done := make(chan error)
	go func() {
		_, _, err := client.fetch(ctx, server.URL)
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()
	close(blocker)

	err := <-done
	if err == nil {
		t.Errorf("expected context cancellation error")
	}
}

func TestFetchIndex_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/terraform.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"providers.v1": "/v1/providers/",
			})
		} else if strings.Contains(r.URL.Path, "/versions") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(RegistryVersionsResponse{
				Versions: []RegistryVersion{
					{Version: "1.0.0"},
					{Version: "1.0.1"},
				},
			})
		}
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	hostname := u.Host

	indexResp, _, err := client.FetchIndex(context.Background(), hostname, "hashicorp", "aws")
	if err != nil {
		t.Fatalf("FetchIndex failed: %v", err)
	}

	if indexResp == nil {
		t.Errorf("expected non-nil index response")
		return
	}
	if _, ok := indexResp.Versions["1.0.0"]; !ok {
		t.Errorf("expected version 1.0.0 in response")
	}
}

func TestFetchIndex_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	hostname := u.Host

	_, _, err := client.FetchIndex(context.Background(), hostname, "hashicorp", "nonexistent")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFetchVersion_ServiceDiscovery(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/terraform.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"providers.v1": "/v1/providers/",
			})
		}
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	hostname := u.Host

	// With service discovery enabled, should return ErrNotFound
	_, err := client.FetchVersion(context.Background(), hostname, "hashicorp", "aws", "1.0.0")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound with service discovery, got %v", err)
	}
}

func TestFetchDownloadURL_Success(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/terraform.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"providers.v1": "/v1/providers/",
			})
		} else if strings.Contains(r.URL.Path, "/download/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(DownloadInfo{
				DownloadURL: "https://releases.hashicorp.com/terraform-provider-aws/1.0.0/terraform-provider-aws_1.0.0_linux_amd64.zip",
				Shasum:      "abcd1234",
			})
		}
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	hostname := u.Host

	info, err := client.FetchDownloadURL(context.Background(), hostname, "hashicorp", "aws", "1.0.0", "linux", "amd64")
	if err != nil {
		t.Fatalf("FetchDownloadURL failed: %v", err)
	}

	if info == nil {
		t.Errorf("expected non-nil download info")
		return
	}
	if !strings.Contains(info.DownloadURL, "terraform-provider-aws") {
		t.Errorf("unexpected download URL: %s", info.DownloadURL)
	}
}

func TestFetchDownloadURL_NotFound(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/terraform.json" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"providers.v1": "/v1/providers/",
			})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestUpstreamClient(server)
	u, _ := url.Parse(server.URL)
	hostname := u.Host

	_, err := client.FetchDownloadURL(context.Background(), hostname, "hashicorp", "nonexistent", "1.0.0", "linux", "amd64")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
