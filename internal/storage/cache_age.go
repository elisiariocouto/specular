package storage

import (
	"context"
	"time"
)

// CacheAgeChecker provides the ability to check when cached data was last written.
// This is separate from the Storage interface to avoid breaking existing implementations.
// Storage backends that support TTL-based revalidation should implement this interface.
type CacheAgeChecker interface {
	// IndexAge returns the age of the cached index.json for a provider.
	// Returns the time since the file was last written/updated.
	// Returns zero duration and false if the index is not cached.
	IndexAge(ctx context.Context, hostname, namespace, providerType string) (age time.Duration, exists bool, err error)
}
