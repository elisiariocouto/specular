package mirror

import (
	"context"
	"sync"
)

// providerKey uniquely identifies a provider for refresh deduplication.
type providerKey struct {
	hostname     string
	namespace    string
	providerType string
}

// IndexRefresher manages background refresh of stale index data.
// It ensures only one refresh goroutine runs per provider at a time.
type IndexRefresher struct {
	mu       sync.Mutex
	inflight map[providerKey]bool
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewIndexRefresher creates a new IndexRefresher.
func NewIndexRefresher() *IndexRefresher {
	ctx, cancel := context.WithCancel(context.Background())
	return &IndexRefresher{
		inflight: make(map[providerKey]bool),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// TryRefresh attempts to launch a background refresh for the given provider.
// Returns true if a refresh was started, false if one is already in-flight.
// The refreshFn is called in a new goroutine with the refresher's context
// (not the HTTP request context) so it survives after the response is sent.
func (r *IndexRefresher) TryRefresh(hostname, namespace, providerType string, refreshFn func(ctx context.Context)) bool {
	key := providerKey{hostname, namespace, providerType}

	r.mu.Lock()
	if r.inflight[key] {
		r.mu.Unlock()
		return false
	}
	r.inflight[key] = true
	r.mu.Unlock()

	r.wg.Go(func() {
		defer func() {
			r.mu.Lock()
			delete(r.inflight, key)
			r.mu.Unlock()
		}()
		refreshFn(r.ctx)
	})

	return true
}

// Shutdown cancels all in-flight refreshes and waits for them to finish.
func (r *IndexRefresher) Shutdown() {
	r.cancel()
	r.wg.Wait()
}
