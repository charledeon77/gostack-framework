// Package foundation (Citadel) serves as the core booting and structural bedrock of the 
// GoStack framework, managing low-level application lifecycles and diagnostics.
package foundation

import (
	"fmt"
	"sync"
)

// Container implements a high-performance, thread-safe Inversion of Control (IoC)
// Service Container based on the Explicit Factory Design Pattern.
//
// DESIGN PHILOSOPHY & ARCHITECTURAL CHOICES:
// 1. Explicit Factory Functions vs Reflection:
//    GoStack explicitly avoids dynamic, runtime reflection-based containers (e.g., Laravel's automatic injection).
//    Reflection degrades execution speed and converts compile-time security into unstable runtime panics.
//    Instead, we map explicit string keys to highly efficient factory function pointers.
//
// 2. Explicit Factory Functions vs Compile-Time Code Generation:
//    While tools like Google Wire provide zero runtime overhead by generating static initializations,
//    they introduce friction into the developer workflow by requiring constant CLI compilation passes.
//    GoStack balances maximum speed with architectural flexibility by managing bindings dynamically in local RAM.
//
// 3. Thread Safety & Concurrent Map Mutations:
//    Go's native maps are structurally unsafe for concurrent read/write mutations. Because GoStack is
//    engineered for high-concurrency HTTP architectures, our internal registries are fully shielded by a
//    sync.RWMutex. We use granular RLock() transitions for lightning-fast concurrent reads, and strict Lock()
//    barriers for single-instance structural state initialization mutations.
type Container struct {
	mu sync.RWMutex

	// cache stores fully materialized, active Singleton service instances.
	cache map[string]any

	// singletonFactories stores the dormant, unresolved structural blueprints for Singletons.
	// Once resolved for the first time, the factory is purged from this map to optimize memory.
	singletonFactories map[string]func(c *Container) any

	// transientFactories stores recipes that run freshly on every single invocation pass.
	transientFactories map[string]func(c *Container) any
}

// NewContainer initializes an empty, production-ready framework IoC storage instance.
func NewContainer() *Container {
	return &Container{
		cache:              make(map[string]any),
		singletonFactories: make(map[string]func(c *Container) any),
		transientFactories: make(map[string]func(c *Container) any),
	}
}

// BindSingleton registers a shared structural dependency blueprint into the container.
//
// LIFECYCLE BEHAVIOR:
// A Singleton is a deferred single-instance object. Passing a factory recipe into this method
// does NOT allocate memory for the service immediately. The service remains dormant until the
// application explicitly requests it via Resolve(). Once built, it is stored in the cache, and
// the same shared instance is returned across all subsequent lifecycle calls.
//
// PARAMETERS:
//   - key: A unique string identifier representing the service binding (e.g., "db", "config").
//   - factory: A custom callback function housing the exact instantiation mechanics for the service.
func (c *Container) BindSingleton(key string, factory func(c *Container) any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Wipe any existing cached instance to allow dynamic runtime service re-bindings if required
	delete(c.cache, key)
	delete(c.transientFactories, key)

	c.singletonFactories[key] = factory
}

// BindTransient registers a volatile dependency blueprint into the container.
//
// LIFECYCLE BEHAVIOR:
// Transients represent short-lived, state-free, or request-scoped helpers. Unlike Singletons,
// the container does not cache transient records. Every single time Resolve() is executed against
// a transient key, the factory function fires freshly, resulting in a unique object allocation.
//
// PARAMETERS:
//   - key: A unique string identifier representing the service binding (e.g., "validator", "formatter").
//   - factory: A custom callback function executing the instantiation mechanics on every call.
func (c *Container) BindTransient(key string, factory func(c *Container) any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, key)
	delete(c.singletonFactories, key)

	c.transientFactories[key] = factory
}

// Resolve fetches, builds, or extracts a registered service out of the container warehouse.
//
// RESOLUTION RESOLVING PIPELINE STEP-BY-STEP:
// 1. High-Performance Cache Lookups: Checks if the target key is already an active, warmed Singleton.
// 2. On-The-Fly Singleton Materialization: If a blueprint factory exists, we acquire a write lock,
//    execute the factory to materialize the service, store it inside the cache, and clean up the factory reference.
// 3. Volatile Transient Processing: If a transient registry matches, we execute the factory directly.
// 4. Safe Error Routing: If the key cannot be found, we return a structured error instead of panicking.
//
// NESTED DEPENDENCY INJECTION (CHAINING):
// Because the container passes its own instance pointer `c` straight into the executing factories,
// dependencies can dynamically resolve other dependencies out of the container during their own construction loop.
func (c *Container) Resolve(key string) (any, error) {
	// --- STEP 1: READ-LOCK BOUNDARY (Fast-Path Cache Inspection) ---
	c.mu.RLock()
	if instance, exists := c.cache[key]; exists {
		c.mu.RUnlock()
		return instance, nil
	}

	// Check if a transient recipe exists under the current high-performance read lock
	if factory, exists := c.transientFactories[key]; exists {
		c.mu.RUnlock()
		return factory(c), nil
	}
	c.mu.RUnlock()

	// --- STEP 2: WRITE-LOCK BOUNDARY (State Mutation Processing) ---
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check cache state to eliminate race-conditions caused during lock execution handovers
	if instance, exists := c.cache[key]; exists {
		return instance, nil
	}

	// Materialize Singleton if a factory recipe is present
	if factory, exists := c.singletonFactories[key]; exists {
		instance := factory(c)
		c.cache[key] = instance
		delete(c.singletonFactories, key) // Optimize memory by removing the single-use factory pointer
		return instance, nil
	}

	return nil, fmt.Errorf("foundation.Container: service signature [%s] cannot be resolved (missing binding recipe)", key)
}

// Has executes a non-blocking, concurrent inspection to verify if a service key is bound.
func (c *Container) Has(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, inCache := c.cache[key]
	_, inSingletons := c.singletonFactories[key]
	_, inTransients := c.transientFactories[key]

	return inCache || inSingletons || inTransients
}