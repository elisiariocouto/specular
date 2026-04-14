package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/exp/slices"
)

// FilesystemStorage implements Storage using the local filesystem
type FilesystemStorage struct {
	cacheDir string
}

// NewFilesystemStorage creates a new filesystem storage backend
func NewFilesystemStorage(cacheDir string) (*FilesystemStorage, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &FilesystemStorage{
		cacheDir: cacheDir,
	}, nil
}

// GetIndex retrieves the cached index.json for a provider
func (fs *FilesystemStorage) GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	if err := validateProviderPath(hostname, namespace, providerType); err != nil {
		return nil, err
	}
	path := fs.indexPath(hostname, namespace, providerType)
	return fs.readFile(ctx, path)
}

// PutIndex stores the index.json for a provider
func (fs *FilesystemStorage) PutIndex(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	if err := validateProviderPath(hostname, namespace, providerType); err != nil {
		return err
	}
	path := fs.indexPath(hostname, namespace, providerType)
	return fs.writeFileAtomic(ctx, path, data)
}

// GetVersion retrieves the cached version.json for a specific provider version
func (fs *FilesystemStorage) GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	if err := validateProviderPath(hostname, namespace, providerType); err != nil {
		return nil, err
	}
	if version == "" {
		return nil, errors.New("version cannot be empty")
	}
	path := fs.versionPath(hostname, namespace, providerType, version)
	return fs.readFile(ctx, path)
}

// PutVersion stores the version.json for a specific provider version
func (fs *FilesystemStorage) PutVersion(ctx context.Context, hostname, namespace, providerType, version string, data []byte) error {
	if err := validateProviderPath(hostname, namespace, providerType); err != nil {
		return err
	}
	if version == "" {
		return errors.New("version cannot be empty")
	}
	path := fs.versionPath(hostname, namespace, providerType, version)
	return fs.writeFileAtomic(ctx, path, data)
}

// GetArchive retrieves a cached provider archive
func (fs *FilesystemStorage) GetArchive(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath := fs.archivePath(path)
	file, err := os.Open(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	return file, nil
}

// PutArchive stores a provider archive
func (fs *FilesystemStorage) PutArchive(ctx context.Context, path string, data io.Reader) error {
	if path == "" {
		return errors.New("archive path cannot be empty")
	}
	fullPath := fs.archivePath(path)
	return fs.atomicWrite(fullPath, func(f *os.File) error {
		_, err := io.Copy(f, data)
		return err
	})
}

// ExistsArchive checks if an archive exists
func (fs *FilesystemStorage) ExistsArchive(ctx context.Context, path string) (bool, error) {
	fullPath := fs.archivePath(path)
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// IndexAge returns the age of the cached index.json by checking file modification time.
func (fs *FilesystemStorage) IndexAge(_ context.Context, hostname, namespace, providerType string) (time.Duration, bool, error) {
	if err := validateProviderPath(hostname, namespace, providerType); err != nil {
		return 0, false, err
	}
	path := fs.indexPath(hostname, namespace, providerType)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to stat index file: %w", err)
	}
	return time.Since(info.ModTime()), true, nil
}

// GetVersionsResponse retrieves the cached full versions API response
func (fs *FilesystemStorage) GetVersionsResponse(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	path := fs.versionsResponsePath(hostname, namespace, providerType)
	return fs.readFile(ctx, path)
}

// PutVersionsResponse stores the full versions API response
func (fs *FilesystemStorage) PutVersionsResponse(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	path := fs.versionsResponsePath(hostname, namespace, providerType)
	return fs.writeFileAtomic(ctx, path, data)
}

// Helper methods

// indexPath constructs the filesystem path for an index.json file
// Matches terraform providers mirror structure: hostname/namespace/type/index.json
func (fs *FilesystemStorage) indexPath(hostname, namespace, providerType string) string {
	return filepath.Join(
		fs.cacheDir,
		hostname,
		namespace,
		providerType,
		"index.json",
	)
}

// versionPath constructs the filesystem path for a version.json file
// Matches terraform providers mirror structure: hostname/namespace/type/VERSION.json
func (fs *FilesystemStorage) versionPath(hostname, namespace, providerType, version string) string {
	return filepath.Join(
		fs.cacheDir,
		hostname,
		namespace,
		providerType,
		fmt.Sprintf("%s.json", version),
	)
}

// versionsResponsePath constructs the filesystem path for the full versions API response
// Stored in internal cache: .specular-internal/hostname/namespace/type/versions.json
func (fs *FilesystemStorage) versionsResponsePath(hostname, namespace, providerType string) string {
	return filepath.Join(
		fs.cacheDir,
		".specular-internal",
		hostname,
		namespace,
		providerType,
		"versions.json",
	)
}

// archivePath constructs the filesystem path for an archive file
// Archives are stored alongside metadata: hostname/namespace/type/archives/...
func (fs *FilesystemStorage) archivePath(path string) string {
	// Sanitize path to prevent directory traversal attacks
	sanitized := filepath.Clean(path)
	if strings.Contains(sanitized, "..") {
		sanitized = strings.ReplaceAll(sanitized, "..", "")
	}
	sanitized = strings.TrimPrefix(sanitized, "/")

	return filepath.Join(fs.cacheDir, sanitized)
}

// readFile reads a file from disk, respecting context cancellation
func (fs *FilesystemStorage) readFile(ctx context.Context, path string) ([]byte, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure path is within cache directory
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %w", err)
	}

	absCacheDir, err := filepath.Abs(fs.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve cache directory: %w", err)
	}

	if !strings.HasPrefix(absPath, absCacheDir) {
		return nil, errors.New("path is outside cache directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// atomicWrite is a helper that writes to a file atomically using a temporary file and rename
// The writeFunc should write data to the provided file and return an error if writing fails
func (fs *FilesystemStorage) atomicWrite(path string, writeFunc func(*os.File) error) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to temporary file first, then rename (atomic)
	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if err := writeFunc(tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath) // Clean up temp file on write error
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath) // Clean up temp file on close error
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomically move temp file to final location
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // Clean up temp file on rename error
		return fmt.Errorf("failed to finalize write: %w", err)
	}

	return nil
}

// writeFileAtomic writes a file atomically using a temporary file
func (fs *FilesystemStorage) writeFileAtomic(ctx context.Context, path string, data []byte) error {
	return fs.atomicWrite(path, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// validateProviderPath checks that provider path components are valid
func validateProviderPath(hostname, namespace, providerType string) error {
	if hostname == "" || namespace == "" || providerType == "" {
		return errors.New("hostname, namespace, and providerType cannot be empty")
	}
	// Reject paths with suspicious characters that could cause issues
	for _, component := range []string{hostname, namespace, providerType} {
		if slices.Contains([]rune(component), '/') || slices.Contains([]rune(component), '\\') {
			return fmt.Errorf("invalid character in path component: %s", component)
		}
	}
	return nil
}
