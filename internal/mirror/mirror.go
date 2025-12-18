package mirror

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/elisiariocouto/speculum/internal/storage"
)

// Mirror handles caching and proxying of Terraform providers
type Mirror struct {
	storage  storage.Storage
	upstream *UpstreamClient
	baseURL  string
}

// NewMirror creates a new mirror service
func NewMirror(store storage.Storage, upstream *UpstreamClient, baseURL string) *Mirror {
	return &Mirror{
		storage:  store,
		upstream: upstream,
		baseURL:  baseURL,
	}
}

// GetIndex returns the index for a provider, using cache or fetching from upstream
func (m *Mirror) GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	// Try to get from cache
	cachedData, err := m.storage.GetIndex(ctx, hostname, namespace, providerType)
	if err == nil {
		return cachedData, nil
	}

	// Cache miss, fetch from upstream
	indexResponse, versionsResponse, err := m.upstream.FetchIndex(ctx, hostname, namespace, providerType)
	if err != nil {
		return nil, err
	}

	// Marshal index response to JSON
	data, err := json.Marshal(indexResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index response: %w", err)
	}

	// Store index in cache (non-blocking, errors are logged elsewhere)
	_ = m.storage.PutIndex(ctx, hostname, namespace, providerType, data)

	// Also cache the full versions response if available
	if versionsResponse != nil {
		versionsData, err := json.Marshal(versionsResponse)
		if err == nil {
			_ = m.storage.PutVersionsResponse(ctx, hostname, namespace, providerType, versionsData)
		}
	}

	return data, nil
}

// GetVersion returns the version for a provider, using cache or fetching from upstream
// It also rewrites archive URLs to point to this mirror
func (m *Mirror) GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	// Try to get from cache
	cachedData, err := m.storage.GetVersion(ctx, hostname, namespace, providerType, version)
	if err == nil {
		// Return cached data (URLs are already correct from when we built it)
		return cachedData, nil
	}

	// Cache miss, try to fetch from upstream
	response, err := m.upstream.FetchVersion(ctx, hostname, namespace, providerType, version)
	if err != nil {
		// If upstream returns ErrNotFound, build from cached versions response
		if err == ErrNotFound {
			return m.buildVersionFromCache(ctx, hostname, namespace, providerType, version)
		}
		return nil, err
	}

	// Marshal response to JSON
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal version response: %w", err)
	}

	// Store in cache (non-blocking, errors are logged elsewhere)
	_ = m.storage.PutVersion(ctx, hostname, namespace, providerType, version, data)

	// Rewrite archive URLs
	return m.rewriteArchiveURLs(ctx, hostname, namespace, providerType, version, data)
}

// buildVersionFromCache builds a version.json response from the cached versions response
// This avoids making multiple API calls to the upstream registry
func (m *Mirror) buildVersionFromCache(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	// Get cached versions response
	versionsData, err := m.storage.GetVersionsResponse(ctx, hostname, namespace, providerType)
	if err != nil {
		return nil, fmt.Errorf("no cached versions response available: %w", err)
	}

	// Parse versions response
	var versionsResp RegistryVersionsResponse
	if err := json.Unmarshal(versionsData, &versionsResp); err != nil {
		return nil, fmt.Errorf("failed to parse versions response: %w", err)
	}

	// Find requested version
	var platforms []RegistryPlatform
	for _, v := range versionsResp.Versions {
		if v.Version == version {
			platforms = v.Platforms
			break
		}
	}

	if len(platforms) == 0 {
		return nil, ErrNotFound
	}

	// Build version response without hashes (they're optional!)
	response := &VersionResponse{
		Archives: make(map[string]Archive),
	}

	for _, platform := range platforms {
		platformKey := fmt.Sprintf("%s_%s", platform.OS, platform.Arch)
		filename := fmt.Sprintf("terraform-provider-%s_%s_%s_%s.zip",
			providerType, version, platform.OS, platform.Arch)

		// Build URL pointing to mirror's download endpoint
		archiveURL := m.buildDownloadURL(hostname, namespace, providerType, version, platform.OS, platform.Arch, filename)

		response.Archives[platformKey] = Archive{
			URL:    archiveURL,
			Hashes: nil, // Omit hashes - they're optional!
		}
	}

	// Marshal and cache
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal version response: %w", err)
	}

	// Store in cache (non-blocking, errors are logged elsewhere)
	_ = m.storage.PutVersion(ctx, hostname, namespace, providerType, version, data)

	return data, nil
}

// GetArchive returns a provider archive, using cache or fetching from upstream on-demand
// Takes explicit parameters for on-demand fetching instead of relying on stored URLs
func (m *Mirror) GetArchive(ctx context.Context, hostname, namespace, providerType, version, os, arch, archivePath string) (io.ReadCloser, error) {
	// Try to get from cache
	reader, err := m.storage.GetArchive(ctx, archivePath)
	if err == nil {
		return reader, nil
	}

	// Cache miss - fetch download URL from registry API
	downloadInfo, err := m.upstream.FetchDownloadURL(ctx, hostname, namespace, providerType, version, os, arch)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Fetch archive from upstream
	archiveReader, err := m.upstream.FetchArchive(ctx, downloadInfo.DownloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch archive: %w", err)
	}
	defer archiveReader.Close()

	// Read archive data into memory for caching
	archiveData, err := io.ReadAll(archiveReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	// Store in cache
	if err := m.storage.PutArchive(ctx, archivePath, bytes.NewReader(archiveData)); err != nil {
		return nil, fmt.Errorf("failed to cache archive: %w", err)
	}

	// Return cached file
	return m.storage.GetArchive(ctx, archivePath)
}

// rewriteArchiveURLs rewrites archive URLs to point to this mirror
// For mirror protocol registries only (not used for service discovery-based registries)
func (m *Mirror) rewriteArchiveURLs(ctx context.Context, hostname, namespace, providerType, version string, data []byte) ([]byte, error) {
	var response VersionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse version response: %w", err)
	}

	// Rewrite URLs to point to mirror's download endpoint
	for platform, archive := range response.Archives {
		if archive.URL != "" {
			// Extract just the filename from the original URL
			filename := m.extractFilename(archive.URL)

			// Build URL pointing to mirror's download endpoint
			// We need to parse OS/arch from platform key (e.g., "linux_amd64" -> linux, amd64)
			parts := strings.Split(platform, "_")
			if len(parts) == 2 {
				os, arch := parts[0], parts[1]

				// Build URL pointing to download endpoint
				archiveURL := m.buildDownloadURL(hostname, namespace, providerType, version, os, arch, filename)

				archive.URL = archiveURL
			}

			// Keep upstream hashes if present (but don't compute our own)
			response.Archives[platform] = archive
		}
	}

	// Marshal back to JSON
	rewritten, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rewritten response: %w", err)
	}

	return rewritten, nil
}

// extractFilename extracts just the filename from an archive URL
// For example: https://releases.hashicorp.com/terraform-provider-aws/5.0.0/terraform-provider-aws_5.0.0_linux_amd64.zip
// Returns: terraform-provider-aws_5.0.0_linux_amd64.zip
func (m *Mirror) extractFilename(archiveURL string) string {
	u, err := url.Parse(archiveURL)
	if err != nil {
		// Fall back to extracting from string
		parts := strings.Split(archiveURL, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return archiveURL
	}

	// Get the last component of the path
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return "archive.zip"
}

// buildDownloadURL constructs a download URL for a provider archive
func (m *Mirror) buildDownloadURL(hostname, namespace, providerType, version, os, arch, filename string) string {
	return fmt.Sprintf("%s/download/%s/%s/%s/%s/%s/%s/%s",
		strings.TrimSuffix(m.baseURL, "/"),
		hostname, namespace, providerType, version, os, arch, filename)
}
