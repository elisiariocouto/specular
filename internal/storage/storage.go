package storage

import (
	"context"
	"io"
)

// Storage defines the interface for storing and retrieving cached data
type Storage interface {
	// GetIndex retrieves the cached index.json for a provider
	// Returns io.EOF if not found
	GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error)

	// PutIndex stores the index.json for a provider
	PutIndex(ctx context.Context, hostname, namespace, providerType string, data []byte) error

	// GetVersion retrieves the cached version.json for a specific provider version
	// Returns io.EOF if not found
	GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error)

	// PutVersion stores the version.json for a specific provider version
	PutVersion(ctx context.Context, hostname, namespace, providerType, version string, data []byte) error

	// GetArchive retrieves a cached provider archive
	// Returns io.EOF if not found
	// Caller is responsible for closing the returned ReadCloser
	GetArchive(ctx context.Context, path string) (io.ReadCloser, error)

	// PutArchive stores a provider archive
	PutArchive(ctx context.Context, path string, data io.Reader) error

	// ExistsArchive checks if an archive exists
	ExistsArchive(ctx context.Context, path string) (bool, error)

	// GetH1Hash retrieves the h1: hash for an archive
	// Returns empty string if not found
	GetH1Hash(ctx context.Context, path string) (string, error)

	// PutH1Hash stores the h1: hash for an archive
	PutH1Hash(ctx context.Context, path string, h1Hash string) error

	// GetUpstreamURL retrieves the upstream URL for an archive
	GetUpstreamURL(ctx context.Context, path string) (string, error)

	// PutUpstreamURL stores the upstream URL for an archive
	PutUpstreamURL(ctx context.Context, path string, upstreamURL string) error
}
