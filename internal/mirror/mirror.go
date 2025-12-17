package mirror

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/elisiariocouto/speculum/internal/storage"
	"golang.org/x/mod/sumdb/dirhash"
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
	response, err := m.upstream.FetchIndex(ctx, hostname, namespace, providerType)
	if err != nil {
		return nil, err
	}

	// Marshal response to JSON
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index response: %w", err)
	}

	// Store in cache (non-blocking, errors are logged elsewhere)
	_ = m.storage.PutIndex(ctx, hostname, namespace, providerType, data)

	return data, nil
}

// GetVersion returns the version for a provider, using cache or fetching from upstream
// It also rewrites archive URLs to point to this mirror
func (m *Mirror) GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	// Try to get from cache
	cachedData, err := m.storage.GetVersion(ctx, hostname, namespace, providerType, version)
	if err == nil {
		// Even if cached, we might need to rewrite URLs and include h1: hashes
		return m.rewriteArchiveURLsWithH1(ctx, hostname, namespace, providerType, cachedData)
	}

	// Cache miss, fetch from upstream
	response, err := m.upstream.FetchVersion(ctx, hostname, namespace, providerType, version)
	if err != nil {
		return nil, err
	}

	// Marshal response to JSON
	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal version response: %w", err)
	}

	// Store in cache (non-blocking, errors are logged elsewhere)
	_ = m.storage.PutVersion(ctx, hostname, namespace, providerType, version, data)

	// Rewrite archive URLs and include h1: hashes
	return m.rewriteArchiveURLsWithH1(ctx, hostname, namespace, providerType, data)
}

// GetArchive returns a provider archive, using cache or fetching from upstream
func (m *Mirror) GetArchive(ctx context.Context, archivePath string) (io.ReadCloser, error) {
	// Try to get from cache
	reader, err := m.storage.GetArchive(ctx, archivePath)
	if err == nil {
		return reader, nil
	}

	// Cache miss, get the upstream URL
	upstreamURL, err := m.storage.GetUpstreamURL(ctx, archivePath)
	if err != nil || upstreamURL == "" {
		return nil, fmt.Errorf("archive not found and upstream URL not available")
	}

	// Fetch from upstream
	archiveReader, err := m.upstream.FetchArchive(ctx, upstreamURL)
	if err != nil {
		return nil, err
	}
	defer archiveReader.Close()

	// Read archive data into memory so we can compute h1: hash before caching
	archiveData, err := io.ReadAll(archiveReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	// Compute h1: hash from archive contents
	h1Hash, err := computeH1Hash(archiveData)
	if err != nil {
		// Log error but don't fail - h1: hash is best-effort
		// The archive will still be cached and served, but without h1: hash
	} else {
		// Store the h1: hash for future use
		_ = m.storage.PutH1Hash(ctx, archivePath, h1Hash)
	}

	// Store in cache
	if err := m.storage.PutArchive(ctx, archivePath, bytes.NewReader(archiveData)); err != nil {
		return nil, fmt.Errorf("failed to cache archive: %w", err)
	}

	// Return cached file
	return m.storage.GetArchive(ctx, archivePath)
}

// rewriteArchiveURLsWithH1 rewrites archive URLs and includes h1: hashes if available
// URLs are rewritten to match terraform providers mirror structure: hostname/namespace/type/filename.zip
func (m *Mirror) rewriteArchiveURLsWithH1(ctx context.Context, hostname, namespace, providerType string, data []byte) ([]byte, error) {
	var response VersionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse version response: %w", err)
	}

	// Rewrite URLs and add h1 hashes if available
	for platform, archive := range response.Archives {
		if archive.URL != "" {
			// Store the original upstream URL
			upstreamURL := archive.URL

			// Extract just the filename from the original URL
			filename := m.extractFilename(archive.URL)

			// Construct the local archive path following terraform providers mirror structure
			archivePath := fmt.Sprintf("%s/%s/%s/%s", hostname, namespace, providerType, filename)

			// Store the mapping from local path to upstream URL
			_ = m.storage.PutUpstreamURL(ctx, archivePath, upstreamURL)

			// Rewrite URL to point to this mirror
			archive.URL = fmt.Sprintf("%s/%s", strings.TrimSuffix(m.baseURL, "/"), archivePath)

			// Check if we have a cached h1 hash for this archive
			h1Hash, err := m.storage.GetH1Hash(ctx, archivePath)
			if err == nil && h1Hash != "" {
				// Add h1 hash to the hashes array if not already present
				hasH1 := false
				for _, hash := range archive.Hashes {
					if strings.HasPrefix(hash, "h1:") {
						hasH1 = true
						break
					}
				}
				if !hasH1 {
					archive.Hashes = append(archive.Hashes, h1Hash)
				}
			}

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

// computeH1Hash computes the h1: hash for a provider archive using the Go dirhash algorithm
// Terraform extracts the zip first and then uses HashDir, not HashZip, to avoid the bug
// where HashZip includes directory entries while HashDir doesn't.
// See: https://github.com/golang/go/issues/53448
func computeH1Hash(archiveData []byte) (string, error) {
	// Create a temporary directory to extract the archive
	tempDir, err := os.MkdirTemp("", "speculum-hash-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Extract the zip archive to the temporary directory
	zipReader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		return "", fmt.Errorf("failed to read zip archive: %w", err)
	}

	for _, file := range zipReader.File {
		// Construct the full path
		path := filepath.Join(tempDir, file.Name)

		// Check for zip slip vulnerability
		if !strings.HasPrefix(path, filepath.Clean(tempDir)+string(os.PathSeparator)) {
			return "", fmt.Errorf("invalid file path in archive: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			// Create directory
			if err := os.MkdirAll(path, file.Mode()); err != nil {
				return "", fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Create parent directory if needed
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Extract file
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return "", fmt.Errorf("failed to create file: %w", err)
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return "", fmt.Errorf("failed to open file in archive: %w", err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()

		if err != nil {
			return "", fmt.Errorf("failed to extract file: %w", err)
		}
	}

	// Compute the h1 hash using dirhash.HashDir on the extracted directory
	hash, err := dirhash.HashDir(tempDir, "", dirhash.Hash1)
	if err != nil {
		return "", fmt.Errorf("failed to compute h1 hash: %w", err)
	}

	return hash, nil
}
