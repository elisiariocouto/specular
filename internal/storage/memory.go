package storage

import (
	"bytes"
	"context"
	"io"
	"math"
	"sync"
	"time"
)

// MemoryStorage implements Storage using an in-memory map
// Useful for testing without filesystem dependencies
type MemoryStorage struct {
	mu                sync.RWMutex
	data              map[string][]byte
	archives          map[string][]byte
	versionsResponses map[string][]byte
	timestamps        map[string]time.Time
}

// NewMemoryStorage creates a new in-memory storage backend
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		data:              make(map[string][]byte),
		archives:          make(map[string][]byte),
		versionsResponses: make(map[string][]byte),
		timestamps:        make(map[string]time.Time),
	}
}

// GetIndex retrieves the cached index.json for a provider
func (m *MemoryStorage) GetIndex(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	key := indexKey(hostname, namespace, providerType)
	return m.get(key)
}

// PutIndex stores the index.json for a provider
func (m *MemoryStorage) PutIndex(_ context.Context, hostname, namespace, providerType string, data []byte) error {
	key := indexKey(hostname, namespace, providerType)
	m.mu.Lock()
	m.data[key] = bytes.Clone(data)
	m.timestamps[key] = time.Now()
	m.mu.Unlock()
	return nil
}

// IndexAge returns the age of the cached index.json for a provider.
func (m *MemoryStorage) IndexAge(_ context.Context, hostname, namespace, providerType string) (time.Duration, bool, error) {
	key := indexKey(hostname, namespace, providerType)
	m.mu.RLock()
	_, dataExists := m.data[key]
	ts, tsExists := m.timestamps[key]
	m.mu.RUnlock()
	if !dataExists {
		return 0, false, nil
	}
	if !tsExists {
		// Data exists but no timestamp (written before TTL feature).
		// Treat as maximally old to trigger a refresh.
		return time.Duration(math.MaxInt64), true, nil
	}
	return time.Since(ts), true, nil
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

// GetVersionsResponse retrieves the cached full versions API response
func (m *MemoryStorage) GetVersionsResponse(ctx context.Context, hostname, namespace, providerType string) ([]byte, error) {
	key := versionsResponseKey(hostname, namespace, providerType)
	return m.get(key)
}

// PutVersionsResponse stores the full versions API response
func (m *MemoryStorage) PutVersionsResponse(ctx context.Context, hostname, namespace, providerType string, data []byte) error {
	key := versionsResponseKey(hostname, namespace, providerType)
	return m.put(key, data)
}

// Helper functions

func indexKey(hostname, namespace, providerType string) string {
	return "index:" + hostname + ":" + namespace + ":" + providerType
}

func versionKey(hostname, namespace, providerType, version string) string {
	return "version:" + hostname + ":" + namespace + ":" + providerType + ":" + version
}

func versionsResponseKey(hostname, namespace, providerType string) string {
	return "versions_response:" + hostname + ":" + namespace + ":" + providerType
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

// Clear removes all data from memory storage (useful for testing)
func (m *MemoryStorage) Clear() {
	m.mu.Lock()
	m.data = make(map[string][]byte)
	m.archives = make(map[string][]byte)
	m.versionsResponses = make(map[string][]byte)
	m.timestamps = make(map[string]time.Time)
	m.mu.Unlock()
}
