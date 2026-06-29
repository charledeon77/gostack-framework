package cache

import (
	"time"

	"github.com/charledeon77/gostack/framework/contract"
)

// Purpose: Provide a strictly typed, Developer-Facing API (Mory) for the cache subsystem.
// 
// Philosophy: "Is this the most type-safe, production-proof, Go-idiomatic approach possible?"
// Yes. By using Go 1.18+ Generics on the package level functions rather than on the
// interface itself, we achieve the perfect hybrid architecture. The underlying CacheStore
// driver remains simple and agnostic, while the end developer enjoys perfect compile-time
// type-safety and zero boilerplate type assertions.
// 
// Architecture:
// We expose generic functions: `Get[T any]`, `Put[T any]`, `Remember[T any]`.
// These functions accept a `contract.CacheStore` driver, allowing seamless dependency injection
// while abstracting away the interface{} (any) storage mechanics.
// 
// Choice:
// We specifically chose NOT to make the `contract.CacheStore` interface generic. If we had,
// developers would have to instantiate a new cache service for every single data type
// they wished to store (e.g. `Cache[User]`, `Cache[string]`). By making the wrapper generic,
// one global cache store can securely serve all data types.
// 
// Implementation details:
// - `Get[T]`: Executes a type assertion on the underlying `any` value. If it fails, it treats it as a miss.
// - `Remember[T]`: A classic Laravel-style closure pattern. Fetches from cache, or computes, stores, and returns.

// Get retrieves a typed value from the cache.
// If the key is missing or the value cannot be type-asserted to T, it returns the zero value and false.
func Get[T any](store contract.CacheStore, key string) (T, bool) {
	var zero T

	val, found := store.Get(key)
	if !found {
		return zero, false
	}

	typedVal, ok := val.(T)
	if !ok {
		// Logically, if the type assertion fails, it means the developer is asking for
		// the wrong type for this key. We treat it as a cache miss to be safe.
		return zero, false
	}

	return typedVal, true
}

// Put stores a typed value in the cache.
func Put[T any](store contract.CacheStore, key string, val T, ttl time.Duration) {
	store.Put(key, val, ttl)
}

// Remember attempts to get a typed value from the cache.
// If it does not exist, it executes the provided closure `cb`, stores the result
// in the cache for the given ttl, and returns the result.
func Remember[T any](store contract.CacheStore, key string, ttl time.Duration, cb func() (T, error)) (T, error) {
	val, found := Get[T](store, key)
	if found {
		return val, nil
	}

	// Not found, execute closure
	freshVal, err := cb()
	if err != nil {
		var zero T
		return zero, err
	}

	// Store and return
	Put(store, key, freshVal, ttl)
	return freshVal, nil
}
