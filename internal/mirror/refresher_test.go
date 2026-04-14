package mirror

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestIndexRefresher_TryRefresh_Dedup(t *testing.T) {
	r := NewIndexRefresher()
	defer r.Shutdown()

	started := make(chan struct{})
	proceed := make(chan struct{})

	// First call should start
	ok := r.TryRefresh("registry.terraform.io", "hashicorp", "aws", func(ctx context.Context) {
		close(started)
		<-proceed
	})
	if !ok {
		t.Fatal("first TryRefresh should return true")
	}

	<-started // wait for goroutine to be running

	// Second call for same provider should be deduplicated
	ok = r.TryRefresh("registry.terraform.io", "hashicorp", "aws", func(ctx context.Context) {
		t.Error("deduplicated refresh should not run")
	})
	if ok {
		t.Error("second TryRefresh for same provider should return false")
	}

	close(proceed)
}

func TestIndexRefresher_DifferentProviders(t *testing.T) {
	r := NewIndexRefresher()
	defer r.Shutdown()

	var count atomic.Int32
	done := make(chan struct{}, 2)

	for _, provider := range []string{"aws", "google"} {
		ok := r.TryRefresh("registry.terraform.io", "hashicorp", provider, func(ctx context.Context) {
			count.Add(1)
			done <- struct{}{}
		})
		if !ok {
			t.Errorf("TryRefresh for %s should return true", provider)
		}
	}

	for range 2 {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for refresh goroutines")
		}
	}

	if got := count.Load(); got != 2 {
		t.Errorf("expected 2 refreshes, got %d", got)
	}
}

func TestIndexRefresher_Shutdown_CancelsContext(t *testing.T) {
	r := NewIndexRefresher()

	ctxCancelled := make(chan struct{})

	r.TryRefresh("registry.terraform.io", "hashicorp", "aws", func(ctx context.Context) {
		<-ctx.Done()
		close(ctxCancelled)
	})

	r.Shutdown()

	select {
	case <-ctxCancelled:
		// Context was cancelled
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not cancel context within timeout")
	}
}

func TestIndexRefresher_AllowsRetryAfterCompletion(t *testing.T) {
	r := NewIndexRefresher()
	defer r.Shutdown()

	done := make(chan struct{})
	ok := r.TryRefresh("registry.terraform.io", "hashicorp", "aws", func(ctx context.Context) {
		close(done)
	})
	if !ok {
		t.Fatal("first TryRefresh should return true")
	}

	<-done // wait for first refresh to complete

	// Small sleep to allow the deferred cleanup to run
	time.Sleep(10 * time.Millisecond)

	// Should be able to refresh again after completion
	done2 := make(chan struct{})
	ok = r.TryRefresh("registry.terraform.io", "hashicorp", "aws", func(ctx context.Context) {
		close(done2)
	})
	if !ok {
		t.Error("TryRefresh after completion should return true")
	}

	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second refresh did not complete within timeout")
	}
}
