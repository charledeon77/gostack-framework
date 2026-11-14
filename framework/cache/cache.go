/*
Mory — GoStack Generic-Typed In-Memory & Distributed Cache Subsystem.

Naming Metaphor: Why Mory?
The GoStack Cache Subsystem was named Mory (a phonetic play on "Memory" and Latin "Memoria")
because it acts as the application's instant photographic recall. Instead of repeatedly querying
expensive physical disks or external network services, Mory stores frequently accessed knowledge
in ultra-fast memory circuits, allowing the application to recall critical state in nanoseconds.

Purpose: Provide a strictly typed, Developer-Facing API (Mory) for the cache subsystem.

Philosophy: "Is this the most type-safe, production-proof, Go-idiomatic approach possible?"
Yes. By using Go 1.18+ Generics on the package level functions rather than on the
interface itself, we achieve the perfect hybrid architecture. The underlying CacheStore
driver remains simple and agnostic, while the end developer enjoys perfect compile-time
type-safety and zero boilerplate type assertions.

Architecture:
We expose generic functions: `Get[T any]`, `Put[T any]`, `Remember[T any]`.
These functions accept a `contract.CacheStore` driver, allowing seamless dependency injection
while abstracting away the interface{} (any) storage mechanics.

Choice:
We specifically chose NOT to make the `contract.CacheStore` interface generic. If we had,
developers would have to instantiate a new cache service for every single data type
they wished to store (e.g. `Cache[User]`, `Cache[string]`). By making the wrapper generic,
one global cache store can securely serve all data types.

Implementation details:
- `Get[T]`: Executes a type assertion on the underlying `any` value. If it fails, it treats it as a miss.
- `Remember[T]`: A classic Laravel-style closure pattern. Fetches from cache, or computes, stores, and returns.
*/
package cache

import (
	"errors"
	"fmt"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// ErrTypeMismatch is returned when the stored value cannot be converted to T.
var ErrTypeMismatch = errors.New("cache: type assertion failed")

// Get retrieves a typed value from the cache.
// Returns (zero, contract.ErrCacheMiss) if the key does not exist.
// Returns (zero, ErrTypeMismatch) if the value is not of type T.
// Returns (zero, error) if the underlying store encountered an operational failure.
// SEC-46: Outages are surfaced as real errors rather than masked as misses.
func Get[T any](store contract.CacheStore, key string) (T, error) {
	var zero T

	val, err := store.Get(key)
	if err != nil {
		return zero, err
	}

	typedVal, ok := val.(T)
	if !ok {
		return zero, fmt.Errorf("%w: expected %T, got %T", ErrTypeMismatch, zero, val)
	}

	return typedVal, nil
}

// Put stores a typed value in the cache.
// SEC-46: Returns any serialization or operational error from the store.
func Put[T any](store contract.CacheStore, key string, val T, ttl time.Duration) error {
	return store.Put(key, val, ttl)
}

// Remember attempts to get a typed value from the cache.
// If the key is not found (contract.ErrCacheMiss), it executes the provided closure `cb`,
// stores the result in the cache for the given ttl, and returns the result.
// If the cache encounters an operational error (e.g. network/Redis down), it surfaces the error.
func Remember[T any](store contract.CacheStore, key string, ttl time.Duration, cb func() (T, error)) (T, error) {
	var zero T

	val, err := Get[T](store, key)
	if err == nil {
		return val, nil
	}

	// If the error is NOT a cache miss (e.g. Redis connection died or timeout),
	// surface the error immediately so callers know the cache is failing.
	if !errors.Is(err, contract.ErrCacheMiss) {
		return zero, fmt.Errorf("cache lookup failed: %w", err)
	}

	// Key not found in cache, execute closure
	freshVal, err := cb()
	if err != nil {
		return zero, err
	}

	// Store and return
	if putErr := Put(store, key, freshVal, ttl); putErr != nil {
		return freshVal, fmt.Errorf("cache store failed: %w", putErr)
	}
	return freshVal, nil
}
