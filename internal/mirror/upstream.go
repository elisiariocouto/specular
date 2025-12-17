package mirror

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UpstreamClient handles fetching from the upstream registry
type UpstreamClient struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// NewUpstreamClient creates a new upstream client
func NewUpstreamClient(baseURL string, timeout time.Duration, maxRetries int) *UpstreamClient {
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

	return &UpstreamClient{
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: maxRetries,
	}
}

// FetchIndex fetches the index.json for a provider
func (uc *UpstreamClient) FetchIndex(ctx context.Context, hostname, namespace, providerType string) (*IndexResponse, error) {
	var url string

	// Handle registry.terraform.io's native API format
	if hostname == "registry.terraform.io" || uc.baseURL == "https://registry.terraform.io" {
		// Use registry.terraform.io's v1 API: /v1/providers/:namespace/:type/versions
		url = fmt.Sprintf("%s/v1/providers/%s/%s/versions", uc.baseURL, namespace, providerType)
	} else {
		// Use provider network mirror protocol format
		path := fmt.Sprintf("%s/%s/%s/index.json", hostname, namespace, providerType)
		url = uc.buildURL(path)
	}

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

	// Convert registry.terraform.io API response to mirror protocol format
	if hostname == "registry.terraform.io" || uc.baseURL == "https://registry.terraform.io" {
		return uc.convertRegistryAPIToIndexResponse(body)
	}

	var response IndexResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse index response: %w", err)
	}

	return &response, nil
}

// FetchVersion fetches the version.json for a specific provider version
func (uc *UpstreamClient) FetchVersion(ctx context.Context, hostname, namespace, providerType, version string) (*VersionResponse, error) {
	// Handle registry.terraform.io's native API format
	if hostname == "registry.terraform.io" || uc.baseURL == "https://registry.terraform.io" {
		// Use Provider Registry Protocol to fetch platform-specific downloads
		return uc.convertRegistryAPIToVersionResponse(ctx, namespace, providerType, version)
	}

	// Use provider network mirror protocol format for other registries
	path := fmt.Sprintf("%s/%s/%s/%s.json", hostname, namespace, providerType, version)
	url := uc.buildURL(path)

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
func (uc *UpstreamClient) FetchArchive(ctx context.Context, archiveURL string) (io.ReadCloser, error) {
	// If the URL is relative, make it absolute
	if !isAbsoluteURL(archiveURL) {
		baseURL, err := url.Parse(uc.baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}

		archiveURLPath, err := url.Parse(archiveURL)
		if err != nil {
			return nil, fmt.Errorf("invalid archive URL: %w", err)
		}

		archiveURL = baseURL.ResolveReference(archiveURLPath).String()
	}

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

// convertRegistryAPIToIndexResponse converts registry.terraform.io API response to mirror protocol IndexResponse
func (uc *UpstreamClient) convertRegistryAPIToIndexResponse(data []byte) (*IndexResponse, error) {
	var registryResponse struct {
		Versions []struct {
			Version string `json:"version"`
		} `json:"versions"`
	}

	if err := json.Unmarshal(data, &registryResponse); err != nil {
		return nil, fmt.Errorf("failed to parse registry API response: %w", err)
	}

	// Convert to mirror protocol format
	versions := make(map[string]interface{})
	for _, v := range registryResponse.Versions {
		versions[v.Version] = struct{}{}
	}

	return &IndexResponse{
		Versions: versions,
	}, nil
}

// convertRegistryAPIToVersionResponse fetches platform-specific downloads using the Provider Registry Protocol
// This requires making multiple requests to /v1/providers/:namespace/:type/:version/download/:os/:arch
func (uc *UpstreamClient) convertRegistryAPIToVersionResponse(ctx context.Context, namespace, providerType, version string) (*VersionResponse, error) {
	// Common platforms that Terraform providers typically support
	// We try these and skip if they don't exist
	platforms := [][2]string{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "386"},
		{"freebsd", "amd64"},
		{"openbsd", "amd64"},
	}

	archives := make(map[string]Archive)

	// Fetch download info for each platform
	for _, platform := range platforms {
		os, arch := platform[0], platform[1]
		downloadURL := fmt.Sprintf("%s/v1/providers/%s/%s/%s/download/%s/%s", uc.baseURL, namespace, providerType, version, os, arch)

		body, status, err := uc.fetch(ctx, downloadURL)
		if err != nil {
			// Log but don't fail - some platforms might not be available
			continue
		}

		// Skip if this platform doesn't exist (404)
		if status == http.StatusNotFound {
			continue
		}

		if status != http.StatusOK {
			continue
		}

		var downloadInfo struct {
			DownloadURL string `json:"download_url"`
			Shasum      string `json:"shasum"`
		}

		if err := json.Unmarshal(body, &downloadInfo); err != nil {
			continue
		}

		platformKey := fmt.Sprintf("%s_%s", os, arch)
		archives[platformKey] = Archive{
			URL: downloadInfo.DownloadURL,
			Hashes: []string{
				fmt.Sprintf("zh:%s", downloadInfo.Shasum),
			},
		}
	}

	if len(archives) == 0 {
		return nil, fmt.Errorf("no platforms found for provider version %s/%s/%s", namespace, providerType, version)
	}

	return &VersionResponse{
		Archives: archives,
	}, nil
}

// buildURL builds a complete URL from the base URL and path
func (uc *UpstreamClient) buildURL(path string) string {
	return uc.baseURL + "/" + path
}

// isAbsoluteURL checks if a URL is absolute
func isAbsoluteURL(rawURL string) bool {
	_, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.IsAbs()
}
