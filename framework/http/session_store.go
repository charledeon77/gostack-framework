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
	netHTTP "net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

var validSessionIDRegex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// IsValidSessionID validates that a session ID matches the required 64-character hexadecimal format.
func IsValidSessionID(id string) bool {
	return validSessionIDRegex.MatchString(id)
}

// MemorySession represents an active user session stored in local server memory.
//
// DESIGN RATIONALE:
// In HTTP servers, handlers execute concurrently. MemorySession uses a read-write lock (sync.RWMutex)
// to permit concurrent readers (e.g., loading multiple fields during rendering) while securing
// write actions (e.g., logging in or adding items to a basket).
type MemorySession struct {
	id           string
	mu           sync.RWMutex
	data         map[string]any
	lastActivity time.Time
}

// NewMemorySession creates a fresh instanced MemorySession with the given ID.
func NewMemorySession(id string) *MemorySession {
	return &MemorySession{
		id:           id,
		data:         make(map[string]any),
		lastActivity: time.Now(),
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

// All returns a shallow copy of all key-value pairs stored in the session.
func (s *MemorySession) All() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make(map[string]any, len(s.data))
	for k, v := range s.data {
		res[k] = v
	}
	return res
}

// InMemoryStore implements the contract.SessionStore interface by keeping
// active sessions stored in a thread-safe map in server RAM equipped with automatic TTL sweeping (SEC-14).
type InMemoryStore struct {
	mu          sync.RWMutex
	sessions    map[string]contract.Session
	activities  map[string]time.Time
	ttl         time.Duration
	stopSweeper chan struct{}
}

// NewInMemoryStore returns a pointer to an initialized InMemoryStore with active background TTL sweep.
func NewInMemoryStore(lifetime ...time.Duration) *InMemoryStore {
	ttl := 2 * time.Hour
	if len(lifetime) > 0 && lifetime[0] > 0 {
		ttl = lifetime[0]
	}

	store := &InMemoryStore{
		sessions:    make(map[string]contract.Session),
		activities:  make(map[string]time.Time),
		ttl:         ttl,
		stopSweeper: make(chan struct{}),
	}

	// Active background sweeper (SEC-14)
	go store.sweeper()

	return store
}

func (store *InMemoryStore) sweeper() {
	ticker := time.NewTicker(store.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			store.Sweep()
		case <-store.stopSweeper:
			return
		}
	}
}

// Sweep purges sessions that have exceeded their TTL.
func (store *InMemoryStore) Sweep() {
	store.mu.Lock()
	defer store.mu.Unlock()

	cutoff := time.Now().Add(-store.ttl)
	for id, act := range store.activities {
		if act.Before(cutoff) {
			delete(store.sessions, id)
			delete(store.activities, id)
		}
	}
}

// ActiveCount returns current number of retained sessions for testing.
func (store *InMemoryStore) ActiveCount() int {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return len(store.sessions)
}

// Close terminates the background sweeper goroutine.
func (store *InMemoryStore) Close() {
	close(store.stopSweeper)
}

// Exists reports whether the session ID is currently active in the store (SEC-16).
func (store *InMemoryStore) Exists(id string) bool {
	store.mu.RLock()
	defer store.mu.RUnlock()
	_, exists := store.sessions[id]
	return exists
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
	store.activities[id] = time.Now()
	return s, nil
}

// Rotate implements contract.SessionStore without any runtime type assertions (SEC-04).
func (store *InMemoryStore) Rotate(oldID string) (contract.Session, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	newID := generateSessionID()
	newSess := NewMemorySession(newID)

	if oldSess, exists := store.sessions[oldID]; exists {
		for k, v := range oldSess.All() {
			newSess.Set(k, v)
		}
		delete(store.sessions, oldID)
		delete(store.activities, oldID)
	}

	store.sessions[newID] = newSess
	store.activities[newID] = time.Now()
	return newSess, nil
}

// Save commits the given session structure state into the store registry without type assertions (SEC-04).
func (store *InMemoryStore) Save(s contract.Session) error {
	if s == nil {
		return fmt.Errorf("session store: cannot save nil session")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.sessions[s.ID()] = s
	store.activities[s.ID()] = time.Now()
	return nil
}

// Destroy invalidates and completely deletes a session record from the memory store registry.
func (store *InMemoryStore) Destroy(id string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.sessions, id)
	delete(store.activities, id)
	return nil
}

// RegenerateSessionID issues a brand new session ID for the given session, transfers
// all existing data, and removes the old session entry across any store driver (SEC-04).
func RegenerateSessionID(ctx *Context, store contract.SessionStore, sess contract.Session, cookieName string) (contract.Session, error) {
	newSess, err := store.Rotate(sess.ID())
	if err != nil {
		return nil, err
	}

	// Update context reference.
	ctx.Set("session", newSess)

	// Determine secure flag from TLS or proxy headers
	isSecure := ctx.Request.TLS != nil || ctx.Request.Header.Get("X-Forwarded-Proto") == "https"
	if strings.ToLower(os.Getenv("APP_ENV")) == "local" {
		isSecure = false
	}

	// Emit updated cookie
	netHTTP.SetCookie(ctx.Writer, &netHTTP.Cookie{
		Name:     cookieName,
		Value:    newSess.ID(),
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: netHTTP.SameSiteLaxMode,
	})

	return newSess, nil
}
