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
	seq        uint64
}

// MemoryStore is the in-memory implementation of CacheStore.
type MemoryStore struct {
	mu         sync.RWMutex
	items      map[string]item
	maxEntries int
	stopChan   chan struct{}
	seqCounter uint64
}

const DefaultMaxMemoryEntries = 10000

// NewMemoryStore creates a new MemoryStore with default capacity and starts an automatic cleanup routine.
func NewMemoryStore() contract.CacheStore {
	return NewMemoryStoreWithCapacity(DefaultMaxMemoryEntries)
}

// NewMemoryStoreWithCapacity creates a new MemoryStore with custom max capacity (F-029).
func NewMemoryStoreWithCapacity(maxEntries int) *MemoryStore {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxMemoryEntries
	}
	store := &MemoryStore{
		items:      make(map[string]item),
		maxEntries: maxEntries,
		stopChan:   make(chan struct{}),
	}
	// Start a background goroutine to purge expired keys periodically (F-028: supports clean shutdown)
	go store.startCleanup(5 * time.Minute)
	return store
}

// Close gracefully terminates the background sweeper goroutine (F-028).
func (m *MemoryStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-m.stopChan:
		// already closed
	default:
		close(m.stopChan)
	}
	return nil
}

// Get retrieves a value from the cache.
// Returns (nil, contract.ErrCacheMiss) if missing or expired.
// D28: Deletes expired entries on read so expired keys do not remain resident until the sweeper.
func (m *MemoryStore) Get(key string) (any, error) {
	m.mu.RLock()
	it, found := m.items[key]
	if !found {
		m.mu.RUnlock()
		return nil, contract.ErrCacheMiss
	}

	if it.expiration > 0 && time.Now().UnixNano() > it.expiration {
		m.mu.RUnlock()
		m.mu.Lock()
		// Double check under write lock before deleting
		if it2, ok := m.items[key]; ok && it2.expiration > 0 && time.Now().UnixNano() > it2.expiration {
			delete(m.items, key)
		}
		m.mu.Unlock()
		return nil, contract.ErrCacheMiss
	}
	m.mu.RUnlock()

	return it.value, nil
}

// Put stores a value in the cache. Enforces max entries capacity to prevent OOM (F-029).
// D29: Evicts in deterministic order: expired first, then nearest expiry, then oldest insertion.
func (m *MemoryStore) Put(key string, val any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var exp int64
	if ttl > 0 {
		exp = time.Now().Add(ttl).UnixNano()
	}

	// If at capacity and key is new, clean expired entries first
	if _, exists := m.items[key]; !exists && len(m.items) >= m.maxEntries {
		now := time.Now().UnixNano()
		for k, v := range m.items {
			if v.expiration > 0 && now > v.expiration {
				delete(m.items, k)
			}
		}
		// If still at capacity, evict in defined order: nearest expiry first; if all permanent, oldest seq
		if len(m.items) >= m.maxEntries {
			var candidateKey string
			var earliestExpiry int64 = -1
			var oldestSeq uint64 = ^uint64(0)
			var oldestKey string

			for k, v := range m.items {
				if v.expiration > 0 {
					if earliestExpiry == -1 || v.expiration < earliestExpiry {
						earliestExpiry = v.expiration
						candidateKey = k
					}
				}
				if v.seq < oldestSeq {
					oldestSeq = v.seq
					oldestKey = k
				}
			}

			if candidateKey != "" {
				delete(m.items, candidateKey)
			} else if oldestKey != "" {
				delete(m.items, oldestKey)
			}
		}
	}

	m.seqCounter++
	m.items[key] = item{
		value:      val,
		expiration: exp,
		seq:        m.seqCounter,
	}
	return nil
}

// Forget removes a key from the cache.
func (m *MemoryStore) Forget(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
	return nil
}

// Flush removes all items from the cache.
func (m *MemoryStore) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]item)
	return nil
}

// startCleanup periodically removes expired items to prevent memory leaks (F-028).
func (m *MemoryStore) startCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.stopChan:
			return
		}
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
