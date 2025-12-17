package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// UpstreamClient handles fetching from the upstream registry
type UpstreamClient struct {
	httpClient     *http.Client
	maxRetries     int
	logger         *slog.Logger
	discoveryCache *DiscoveryCache
}

// NewUpstreamClient creates a new upstream client
func NewUpstreamClient(timeout time.Duration, maxRetries int, logger *slog.Logger) *UpstreamClient {
	// Create HTTP client with connection pooling and timeouts
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
		},
	}

	// Create discovery cache with 1 hour TTL
	discoveryCache := NewDiscoveryCache(1*time.Hour, httpClient, logger)

	return &UpstreamClient{
		httpClient:     httpClient,
		maxRetries:     maxRetries,
		logger:         logger,
		discoveryCache: discoveryCache,
	}
}

// getProvidersEndpoint discovers and returns the providers.v1 API endpoint for a registry
// Uses service discovery with caching
func (uc *UpstreamClient) getProvidersEndpoint(ctx context.Context, hostname string) (string, error) {
	// Try service discovery first
	discovery, err := uc.discoveryCache.DiscoverServices(ctx, hostname)
	if err != nil {
		uc.logger.DebugContext(ctx, "service discovery failed, using fallback",
			slog.String("hostname", hostname),
			slog.String("error", err.Error()))
		// Fallback: assume standard HTTPS endpoint
		return fmt.Sprintf("https://%s", hostname), fmt.Errorf("service discovery failed: %w", err)
	}

	// The ProvidersV1 field contains a path (e.g., "/v1/providers/")
	// We need to construct the full URL by combining with the hostname
	providersPath := strings.TrimSuffix(discovery.ProvidersV1, "/")
	fullURL := fmt.Sprintf("https://%s%s", hostname, providersPath)

	return fullURL, nil
}

// FetchIndex fetches the index.json for a provider
// Returns both the simplified IndexResponse and the full RegistryVersionsResponse
func (uc *UpstreamClient) FetchIndex(ctx context.Context, hostname, namespace, providerType string) (*IndexResponse, *RegistryVersionsResponse, error) {
	// Use service discovery to get the providers endpoint
	endpoint, err := uc.getProvidersEndpoint(ctx, hostname)
	if err != nil {
		uc.logger.DebugContext(ctx, "service discovery failed, using fallback",
			slog.String("hostname", hostname),
			slog.String("error", err.Error()))
		// Fallback to mirror protocol format
		url := fmt.Sprintf("https://%s/%s/%s/index.json", hostname, namespace, providerType)

		body, status, fetchErr := uc.fetch(ctx, url)
		if fetchErr != nil {
			return nil, nil, fetchErr
		}

		if status == http.StatusNotFound {
			return nil, nil, ErrNotFound
		}

		if status != http.StatusOK {
			return nil, nil, fmt.Errorf("unexpected status code: %d", status)
		}

		var response IndexResponse
		if err := json.Unmarshal(body, &response); err != nil {
			return nil, nil, fmt.Errorf("failed to parse index response: %w", err)
		}

		return &response, nil, nil
	}

	// Use discovered providers.v1 endpoint
	url := fmt.Sprintf("%s/%s/%s/versions", endpoint, namespace, providerType)

	uc.logger.DebugContext(ctx, "fetching provider versions from upstream",
		slog.String("url", url),
		slog.String("hostname", hostname),
		slog.String("namespace", namespace),
		slog.String("type", providerType))

	body, status, err := uc.fetch(ctx, url)
	if err != nil {
		return nil, nil, err
	}

	if status == http.StatusNotFound {
		return nil, nil, ErrNotFound
	}

	if status != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status code: %d", status)
	}

	// Convert registry API response to mirror protocol format
	return uc.convertRegistryAPIToIndexResponse(body)
}

// FetchVersion fetches the version.json for a specific provider version
// For registries with service discovery, this returns ErrNotFound to signal
// that version.json should be built from cached versions response
func (uc *UpstreamClient) FetchVersion(ctx context.Context, hostname, namespace, providerType, version string) (*VersionResponse, error) {
	// Check if this registry supports service discovery
	_, err := uc.getProvidersEndpoint(ctx, hostname)
	if err == nil {
		// Registry has service discovery - version.json should be built from cache
		uc.logger.DebugContext(ctx, "registry uses service discovery, version will be built from cache",
			slog.String("hostname", hostname),
			slog.String("namespace", namespace),
			slog.String("type", providerType),
			slog.String("version", version))
		return nil, ErrNotFound
	}

	// Fallback: use provider network mirror protocol format
	url := fmt.Sprintf("https://%s/%s/%s/%s.json", hostname, namespace, providerType, version)

	uc.logger.DebugContext(ctx, "fetching version metadata from mirror protocol",
		slog.String("url", url))

	body, status, err := uc.fetch(ctx, url)
	if err != nil {
		return nil, err
	}

	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", status)
	}

	var response VersionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse version response: %w", err)
	}

	return &response, nil
}

// FetchArchive fetches a provider archive from a URL
// The archiveURL must be an absolute URL
func (uc *UpstreamClient) FetchArchive(ctx context.Context, archiveURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, archiveURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := uc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch archive: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// fetch performs an HTTP GET request with retry logic
func (uc *UpstreamClient) fetch(ctx context.Context, url string) ([]byte, int, error) {
	var lastErr error
	var lastStatus int

	for attempt := 0; attempt <= uc.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := uc.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < uc.maxRetries {
				// Exponential backoff
				select {
				case <-ctx.Done():
					return nil, 0, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt)) * time.Second):
					continue
				}
			}
			continue
		}

		lastStatus = resp.StatusCode
		defer resp.Body.Close()

		// Don't retry on client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
			}
			return body, resp.StatusCode, nil
		}

		// Retry on server errors (5xx) and service unavailable
		if resp.StatusCode >= 500 {
			if attempt < uc.maxRetries {
				select {
				case <-ctx.Done():
					return nil, resp.StatusCode, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt)) * time.Second):
					continue
				}
			}
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		// Success
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
		}
		return body, resp.StatusCode, nil
	}

	if lastErr != nil {
		return nil, lastStatus, lastErr
	}
	return nil, lastStatus, fmt.Errorf("max retries exceeded for URL: %s", url)
}

// convertRegistryAPIToIndexResponse converts registry API response to mirror protocol IndexResponse
// Also returns the full RegistryVersionsResponse for caching
func (uc *UpstreamClient) convertRegistryAPIToIndexResponse(data []byte) (*IndexResponse, *RegistryVersionsResponse, error) {
	var registryResponse RegistryVersionsResponse

	if err := json.Unmarshal(data, &registryResponse); err != nil {
		return nil, nil, fmt.Errorf("failed to parse registry API response: %w", err)
	}

	// Convert to mirror protocol format
	versions := make(map[string]interface{})
	for _, v := range registryResponse.Versions {
		versions[v.Version] = struct{}{}
	}

	indexResponse := &IndexResponse{
		Versions: versions,
	}

	return indexResponse, &registryResponse, nil
}

// FetchDownloadURL fetches the download information for a specific provider version and platform
func (uc *UpstreamClient) FetchDownloadURL(ctx context.Context, hostname, namespace, providerType, version, os, arch string) (*DownloadInfo, error) {
	// Get providers endpoint via service discovery
	endpoint, err := uc.getProvidersEndpoint(ctx, hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}

	// Build download API URL: {endpoint}/{namespace}/{type}/{version}/download/{os}/{arch}
	url := fmt.Sprintf("%s/%s/%s/%s/download/%s/%s",
		endpoint, namespace, providerType, version, os, arch)

	uc.logger.DebugContext(ctx, "fetching download URL from registry",
		slog.String("url", url),
		slog.String("os", os),
		slog.String("arch", arch))

	body, status, err := uc.fetch(ctx, url)
	if err != nil {
		return nil, err
	}

	if status == http.StatusNotFound {
		return nil, ErrNotFound
	}

	if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", status)
	}

	var info DownloadInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("failed to parse download info: %w", err)
	}

	uc.logger.DebugContext(ctx, "received download URL from registry",
		slog.String("download_url", info.DownloadURL))

	return &info, nil
}

