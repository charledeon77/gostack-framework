/*
Purpose:
This file implements the central AuthManager (Guard) coordinating multiple authentication guards.

Philosophy:
A single application often requires multiple auth strategies (e.g. cookie sessions for web views,
bearer tokens for mobile/API clients). Our AuthManager serves as a thread-safe registry mapping
named strategies (Guards) and falling back to a configured default strategy. This provides
a unified entry point similar to Laravel's Auth facade.

Architecture:
Part of the auth package. Coordinates implementations of contract.Guard.

Choice:
We chose to implement delegation helpers (Check, User, Login, Logout) directly on the AuthManager
delegating to the default guard, keeping simple configurations extremely clean.

Implementation:
- AuthManager: holds mutex-locked map of Guards with Register, Guard, and delegation helpers.
*/
package auth

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"net/http"
	"sync"
)

// AuthManager coordinates authentication across multiple named guards.
type AuthManager struct {
	mu           sync.RWMutex
	guards       map[string]contract.Guard
	defaultGuard string
}

// NewAuthManager initializes a fresh authentication manager coordinator.
func NewAuthManager(defaultGuard string) *AuthManager {
	return &AuthManager{
		guards:       make(map[string]contract.Guard),
		defaultGuard: defaultGuard,
	}
}

// Register maps a named guard to a specific authentication strategy.
func (m *AuthManager) Register(name string, guard contract.Guard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.guards[name] = guard
}

// Guard resolves a registered guard by name, falling back to the configured default guard.
// Returns an error instead of panicking when the named guard is not registered (F-065).
func (m *AuthManager) Guard(name string) (contract.Guard, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		name = m.defaultGuard
	}

	guard, exists := m.guards[name]
	if !exists {
		return nil, fmt.Errorf("auth: guard [%s] is not registered", name)
	}
	return guard, nil
}

// Check delegates to the default guard. Returns false on configuration error.
func (m *AuthManager) Check(r *http.Request) bool {
	g, err := m.Guard("")
	if err != nil {
		return false
	}
	return g.Check(r)
}

// User delegates to the default guard. Returns (nil, false) on configuration error.
func (m *AuthManager) User(r *http.Request) (contract.Authenticatable, bool) {
	g, err := m.Guard("")
	if err != nil {
		return nil, false
	}
	return g.User(r)
}

// Login delegates to the default guard.
func (m *AuthManager) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	g, err := m.Guard("")
	if err != nil {
		return err
	}
	return g.Login(w, r, user)
}

// Logout delegates to the default guard.
func (m *AuthManager) Logout(w http.ResponseWriter, r *http.Request) error {
	g, err := m.Guard("")
	if err != nil {
		return err
	}
	return g.Logout(w, r)
}

// cacheUser writes the authenticated user into the request context's AuthCache (D4).
func cacheUser(r *http.Request, user contract.Authenticatable) {
	if cacheVal := r.Context().Value(contract.UserContextKey); cacheVal != nil {
		if cache, ok := cacheVal.(*contract.AuthCache); ok {
			cache.User = user
			cache.Loaded = true
		}
	}
}
