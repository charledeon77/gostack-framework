package cache

import (
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// Purpose: Provide a lightning-fast, zero-dependency caching driver for GoStack.
//
// Philosophy: While Redis is great for distributed systems, a single-instance Go app
// can benefit massively from an in-memory cache without the operational overhead of Redis.
// Providing this out-of-the-box ensures a fast baseline performance for all GoStack apps.
//
// Architecture:
// MemoryStore implements the `contract.CacheStore` interface.
// It wraps a standard Go `map[string]item` and guards it with a `sync.RWMutex`.
//
// Choice:
// We chose to use a background cleanup goroutine instead of relying solely on passive 
// expiration (checking on `Get`). This prevents memory leaks if an expired key is never 
// requested again. The `RWMutex` ensures read-heavy workloads don't bottleneck on locks.
//
// Implementation details:
// - We store `item` structs that contain both the value (`any`) and expiration time (`int64`).
// - The cleanup routine uses a `time.Ticker` set to 5 minutes by default to purge stale data.

type item struct {
	value      any
	expiration int64 // Unix timestamp in nanoseconds. 0 means no expiration.
}

// MemoryStore is the in-memory implementation of CacheStore.
type MemoryStore struct {
	mu    sync.RWMutex
	items map[string]item
}

// NewMemoryStore creates a new MemoryStore and starts an optional cleanup routine.
func NewMemoryStore() contract.CacheStore {
	store := &MemoryStore{
		items: make(map[string]item),
	}
	// Start a background goroutine to purge expired keys periodically
	go store.startCleanup(5 * time.Minute)
	return store
}

// Get retrieves a value from the cache.
func (m *MemoryStore) Get(key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	it, found := m.items[key]
	if !found {
		return nil, false
	}

	if it.expiration > 0 && time.Now().UnixNano() > it.expiration {
		// Found but expired
		return nil, false
	}

	return it.value, true
}

// Put stores a value in the cache.
func (m *MemoryStore) Put(key string, val any, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixNano()
	}

	m.items[key] = item{
		value:      val,
		expiration: exp,
	}
}

// Forget removes a key from the cache.
func (m *MemoryStore) Forget(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
}

// Flush removes all items from the cache.
func (m *MemoryStore) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]item)
}

// startCleanup periodically removes expired items to prevent memory leaks.
func (m *MemoryStore) startCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		m.cleanup()
	}
}

func (m *MemoryStore) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UnixNano()
	for k, v := range m.items {
		if v.expiration > 0 && now > v.expiration {
			delete(m.items, k)
		}
	}
}
