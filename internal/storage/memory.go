package storage

import (
	"bytes"
	"context"
	"io"
	"sync"
)

// MemoryStorage implements Storage using an in-memory map
// Useful for testing without filesystem dependencies
type MemoryStorage struct {
	mu           sync.RWMutex
	data         map[string][]byte
	archives     map[string][]byte
	h1Hashes     map[string]string
	upstreamURLs map[string]string
}

// NewMemoryStorage creates a new in-memory storage backend
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:         make(map[string][]byte),
		archives:     make(map[string][]byte),
		h1Hashes:     make(map[string]string),
		upstreamURLs: make(map[string]string),
	}
}

// GetIndex retrieves the cached index.json for a provider
func (m *MemoryStorage) GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	key := indexKey(hostname, namespace, providerType)
	return m.get(key)
}

// PutIndex stores the index.json for a provider
func (m *MemoryStorage) PutIndex(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	key := indexKey(hostname, namespace, providerType)
	return m.put(key, data)
}

// GetVersion retrieves the cached version.json for a specific provider version
func (m *MemoryStorage) GetVersion(ctx context.Context, hostname, namespace, providerType, version string) ([]byte, error) {
	key := versionKey(hostname, namespace, providerType, version)
	return m.get(key)
}

// PutVersion stores the version.json for a specific provider version
func (m *MemoryStorage) PutVersion(ctx context.Context, hostname, namespace, providerType, version string, data []byte) error {
	key := versionKey(hostname, namespace, providerType, version)
	return m.put(key, data)
}

// GetArchive retrieves a cached provider archive
func (m *MemoryStorage) GetArchive(ctx context.Context, path string) (io.ReadCloser, error) {
	m.mu.RLock()
	data, ok := m.archives[path]
	m.mu.RUnlock()

	if !ok {
		return nil, io.EOF
	}

	// Return a copy wrapped in a ReadCloser
	return io.NopCloser(bytes.NewReader(bytes.Clone(data))), nil
}

// PutArchive stores a provider archive
func (m *MemoryStorage) PutArchive(ctx context.Context, path string, data io.Reader) error {
	// Read all data into memory
	content, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.archives[path] = content
	m.mu.Unlock()

	return nil
}

// ExistsArchive checks if an archive exists
func (m *MemoryStorage) ExistsArchive(ctx context.Context, path string) (bool, error) {
	m.mu.RLock()
	_, ok := m.archives[path]
	m.mu.RUnlock()
	return ok, nil
}

// Helper functions

func indexKey(hostname, namespace, providerType string) string {
	return "index:" + hostname + ":" + namespace + ":" + providerType
}

func versionKey(hostname, namespace, providerType, version string) string {
	return "version:" + hostname + ":" + namespace + ":" + providerType + ":" + version
}

func (m *MemoryStorage) get(key string) ([]byte, error) {
	m.mu.RLock()
	data, ok := m.data[key]
	m.mu.RUnlock()

	if !ok {
		return nil, io.EOF
	}

	return bytes.Clone(data), nil
}

func (m *MemoryStorage) put(key string, data []byte) error {
	m.mu.Lock()
	m.data[key] = bytes.Clone(data)
	m.mu.Unlock()
	return nil
}

// GetH1Hash retrieves the h1: hash for an archive
func (m *MemoryStorage) GetH1Hash(ctx context.Context, path string) (string, error) {
	m.mu.RLock()
	hash, ok := m.h1Hashes[path]
	m.mu.RUnlock()

	if !ok {
		return "", nil // Hash not found is not an error
	}

	return hash, nil
}

// PutH1Hash stores the h1: hash for an archive
func (m *MemoryStorage) PutH1Hash(ctx context.Context, path string, h1Hash string) error {
	m.mu.Lock()
	m.h1Hashes[path] = h1Hash
	m.mu.Unlock()
	return nil
}

// GetUpstreamURL retrieves the upstream URL for an archive
func (m *MemoryStorage) GetUpstreamURL(ctx context.Context, path string) (string, error) {
	m.mu.RLock()
	url, ok := m.upstreamURLs[path]
	m.mu.RUnlock()

	if !ok {
		return "", nil // Not found is not an error
	}
	return url, nil
}

// PutUpstreamURL stores the upstream URL for an archive
func (m *MemoryStorage) PutUpstreamURL(ctx context.Context, path string, upstreamURL string) error {
	m.mu.Lock()
	m.upstreamURLs[path] = upstreamURL
	m.mu.Unlock()
	return nil
}

// Clear removes all data from memory storage (useful for testing)
func (m *MemoryStorage) Clear() {
	m.mu.Lock()
	m.data = make(map[string][]byte)
	m.archives = make(map[string][]byte)
	m.h1Hashes = make(map[string]string)
	m.upstreamURLs = make(map[string]string)
	m.mu.Unlock()
}
