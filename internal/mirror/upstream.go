package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
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
func NewUpstreamClient(timeout time.Duration, maxRetries int, discoveryCacheTTL time.Duration, logger *slog.Logger) *UpstreamClient {
	// Create HTTP client with connection pooling and timeouts
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Create discovery cache with configurable TTL
	discoveryCache := NewDiscoveryCache(discoveryCacheTTL, httpClient, logger)

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
		// Fallback to mirror protocol format
		url := fmt.Sprintf("https://%s/%s/%s/index.json", hostname, namespace, providerType)

		uc.logger.DebugContext(ctx, "service discovery failed, using mirror protocol fallback",
			slog.String("url", url),
			slog.String("error", err.Error()))

		body, status, fetchErr := uc.fetch(ctx, url)
		if fetchErr != nil {
			return nil, nil, fetchErr
		}

		if statusErr := checkStatusCode(status); statusErr != nil {
			return nil, nil, statusErr
		}

		var response IndexResponse
		if err := parseJSON(body, &response, "index"); err != nil {
			return nil, nil, err
		}

		return &response, nil, nil
	}

	// Use discovered providers.v1 endpoint
	url := fmt.Sprintf("%s/%s/%s/versions", endpoint, namespace, providerType)

	uc.logger.DebugContext(ctx, "fetching provider versions from upstream",
		slog.String("url", url))

	body, status, err := uc.fetch(ctx, url)
	if err != nil {
		return nil, nil, err
	}

	if statusErr := checkStatusCode(status); statusErr != nil {
		return nil, nil, statusErr
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

	if statusErr := checkStatusCode(status); statusErr != nil {
		return nil, statusErr
	}

	var response VersionResponse
	if err := parseJSON(body, &response, "version"); err != nil {
		return nil, err
	}

	return &response, nil
}

// FetchArchive fetches a provider archive from a URL with retry logic
// The archiveURL must be an absolute URL
func (uc *UpstreamClient) FetchArchive(ctx context.Context, archiveURL string) (io.ReadCloser, error) {
	// Validate URL
	parsedURL, err := url.Parse(archiveURL)
	if err != nil {
		return nil, fmt.Errorf("invalid archive URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("archive URL must use http or https scheme, got: %s", parsedURL.Scheme)
	}

	if parsedURL.Host == "" {
		return nil, fmt.Errorf("archive URL must have a host")
	}

	resp, status, err := uc.doRequestWithRetry(ctx, archiveURL)
	if err != nil {
		return nil, err
	}

	// Check for HTTP errors - return status code in error message
	if status != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", status)
	}

	return resp.Body, nil
}

// exponentialBackoff waits for exponential backoff duration, respecting context cancellation
func exponentialBackoff(ctx context.Context, attempt int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Duration(1<<uint(attempt)) * time.Second):
		return nil
	}
}

// checkStatusCode validates HTTP status and returns appropriate error
func checkStatusCode(status int) error {
	if status == http.StatusNotFound {
		return ErrNotFound
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", status)
	}
	return nil
}

// parseJSON unmarshals JSON data into a target struct
func parseJSON(data []byte, v interface{}, responseType string) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to parse %s response: %w", responseType, err)
	}
	return nil
}

// doRequestWithRetry performs an HTTP GET request with exponential backoff retry logic
// Returns the HTTP response (caller is responsible for closing the body) and status code
// Note: Returns response on both success (2xx-3xx) and client errors (4xx), only retries on server errors (5xx) or network errors
func (uc *UpstreamClient) doRequestWithRetry(ctx context.Context, url string) (*http.Response, int, error) {
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
			lastStatus = 0
			// Only retry on network errors if we have attempts left
			if attempt < uc.maxRetries {
				if backoffErr := exponentialBackoff(ctx, attempt); backoffErr != nil {
					return nil, 0, backoffErr
				}
				continue
			}
			// Last attempt failed
			return nil, 0, fmt.Errorf("failed to fetch: %w", lastErr)
		}

		lastStatus = resp.StatusCode

		// Don't retry on client errors (4xx) or success (2xx-3xx) - return immediately
		if resp.StatusCode < 500 {
			return resp, resp.StatusCode, nil
		}

		// For 5xx errors with retries left, backoff and retry
		if attempt < uc.maxRetries {
			resp.Body.Close()
			if backoffErr := exponentialBackoff(ctx, attempt); backoffErr != nil {
				return nil, resp.StatusCode, backoffErr
			}
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		// Final attempt with 5xx error - return response for caller to handle
		return resp, resp.StatusCode, nil
	}

	// Should not reach here
	if lastErr != nil {
		return nil, lastStatus, lastErr
	}
	return nil, lastStatus, fmt.Errorf("unexpected state: max retries exceeded")
}

// fetch performs an HTTP GET request with retry logic, returning the full response body
func (uc *UpstreamClient) fetch(ctx context.Context, url string) ([]byte, int, error) {
	resp, status, err := uc.doRequestWithRetry(ctx, url)
	if err != nil {
		return nil, status, err
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, status, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, status, nil
}

// convertRegistryAPIToIndexResponse converts registry API response to mirror protocol IndexResponse
// Also returns the full RegistryVersionsResponse for caching
func (uc *UpstreamClient) convertRegistryAPIToIndexResponse(data []byte) (*IndexResponse, *RegistryVersionsResponse, error) {
	var registryResponse RegistryVersionsResponse

	if err := parseJSON(data, &registryResponse, "registry API"); err != nil {
		return nil, nil, err
	}

	// Convert to mirror protocol format
	versions := make(map[string]VersionInfo)
	for _, v := range registryResponse.Versions {
		versions[v.Version] = VersionInfo{}
	}

	return &IndexResponse{Versions: versions}, &registryResponse, nil
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

	if statusErr := checkStatusCode(status); statusErr != nil {
		return nil, statusErr
	}

	var info DownloadInfo
	if err := parseJSON(body, &info, "download info"); err != nil {
		return nil, err
	}

	uc.logger.DebugContext(ctx, "received download URL from registry",
		slog.String("download_url", info.DownloadURL))

	return &info, nil
}
