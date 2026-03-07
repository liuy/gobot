package memory

import (
	"testing"
	"time"
)

// waitFor polls condition until true or timeout.
// Useful for async operations in tests.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

// waitForMessage waits for a message to be searchable in cold storage.
func waitForMessage(t *testing.T, cache *MemoryCache, query string, expectedCount int) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool {
		results, err := cache.Search(query)
		if err != nil {
			return false
		}
		return len(results) >= expectedCount
	})
}

// waitForHotUpdate waits for hot memory to have at least expectedKeywords.
func waitForHotUpdate(t *testing.T, cache *MemoryCache, expectedKeywords int) {
	t.Helper()
	waitFor(t, 2*time.Second, func() bool {
		hot, err := cache.GetHot()
		if err != nil {
			return false
		}
		return len(hot.RecentKeywords) >= expectedKeywords
	})
}
