/*
Purpose:
This file implements standard in-memory session structures that fulfill the Session and SessionStore contracts.
It handles individual request thread-safe state access and centralized map-based storage.

Philosophy:
We prioritize simplicity and execution speed. During local development or single-instance
deployments, a fast, locks-guarded memory store keeps execution overhead at zero without
requiring external database table setups or Redis infrastructure.

Architecture:
Implements framework/contract/session interfaces. The MemorySession manages isolated key-value pairs,
while InMemoryStore coordinates the persistent sessions map.

Choice:
We chose sync.RWMutex over sync.Map because it allows clean, explicit atomic locking of the entire
data block during operations like Clear() or model pointer validation, while supporting multiple
readers concurrently without runtime overhead.

Implementation:
- MemorySession: Struct representing a single active user session.
- InMemoryStore: Struct managing the global active sessions in-memory.
*/
package http

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"sync"
)

// MemorySession represents an active user session stored in local server memory.
//
// DESIGN RATIONALE:
// In HTTP servers, handlers execute concurrently. MemorySession uses a read-write lock (sync.RWMutex)
// to permit concurrent readers (e.g., loading multiple fields during rendering) while securing
// write actions (e.g., logging in or adding items to a basket).
type MemorySession struct {
	id   string
	mu   sync.RWMutex
	data map[string]any
}

// NewMemorySession creates a fresh instanced MemorySession with the given ID.
func NewMemorySession(id string) *MemorySession {
	return &MemorySession{
		id:   id,
		data: make(map[string]any),
	}
}

// ID returns the unique session token identifier.
func (s *MemorySession) ID() string {
	return s.id
}

// Get retrieves a value from the session map. It is safe for concurrent reads.
func (s *MemorySession) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data[key]
}

// Set commits a key-value pair to the session state. Safe for concurrent writes.
func (s *MemorySession) Set(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

// Delete removes a key from the session state. Safe for concurrent writes.
func (s *MemorySession) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// Clear purges all state values from the session. Safe for concurrent writes.
func (s *MemorySession) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]any)
}

// Flash stores a value in the session that will be available only on the
// immediately following request. On the next call to GetFlash, the value is
// returned and automatically removed from the session.
//
// Typical usage — store a success notice before a redirect:
//
//	sess.Flash("success", "Your post was saved!")
//	http.Redirect(w, r, "/dashboard", http.StatusFound)
//
// Then in the next request's handler:
//
//	msg := sess.GetFlash("success") // returns "Your post was saved!" and removes it
func (s *MemorySession) Flash(key string, val any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	flashKey := "__flash__" + key
	s.data[flashKey] = val
}

// GetFlash retrieves a flash value set by Flash() and immediately removes it
// from the session so it cannot be read again on subsequent requests.
// Returns nil if no flash value is stored under the given key.
func (s *MemorySession) GetFlash(key string) any {
	s.mu.Lock()
	defer s.mu.Unlock()
	flashKey := "__flash__" + key
	val, exists := s.data[flashKey]
	if !exists {
		return nil
	}
	delete(s.data, flashKey)
	return val
}

// InMemoryStore implements the contract.SessionStore interface by keeping
// active sessions stored in a thread-safe map in server RAM.
//
// WARNING:
// Because the data is not written to disk or shared, restarting the server processes
// will invalidate all active sessions, and multi-node clusters will not share session states.
type InMemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*MemorySession
}

// NewInMemoryStore returns a pointer to an initialized InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		sessions: make(map[string]*MemorySession),
	}
}

// Load retrieves a session by its unique ID from the store. 
// If the session is missing, a new session is initialized and saved.
func (store *InMemoryStore) Load(id string) (contract.Session, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	s, exists := store.sessions[id]
	if !exists {
		s = NewMemorySession(id)
		store.sessions[id] = s
	}
	return s, nil
}

// Save commits the given session structure state into the store registry.
func (store *InMemoryStore) Save(s contract.Session) error {
	memSession, ok := s.(*MemorySession)
	if !ok {
		return fmt.Errorf("session store: expected *MemorySession, got %T", s)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.sessions[s.ID()] = memSession
	return nil
}

// Destroy invalidates and completely deletes a session record from the memory store registry.
func (store *InMemoryStore) Destroy(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.sessions, id)
	return nil
}
