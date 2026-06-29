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
func (m *AuthManager) Guard(name string) contract.Guard {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if name == "" {
		name = m.defaultGuard
	}

	guard, exists := m.guards[name]
	if !exists {
		panic(fmt.Sprintf("auth: guard [%s] is not registered", name))
	}
	return guard
}

// Check delegates to the default guard.
func (m *AuthManager) Check(r *http.Request) bool {
	return m.Guard("").Check(r)
}

// User delegates to the default guard.
func (m *AuthManager) User(r *http.Request) (contract.Authenticatable, bool) {
	return m.Guard("").User(r)
}

// Login delegates to the default guard.
func (m *AuthManager) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	return m.Guard("").Login(w, r, user)
}

// Logout delegates to the default guard.
func (m *AuthManager) Logout(w http.ResponseWriter, r *http.Request) error {
	return m.Guard("").Logout(w, r)
}
