package socialhub

import (
	"fmt"
	"sync"
)

// Purpose: To provide a unified OAuth2 authentication system (like Laravel Socialite).
// Philosophy: "Zero External Bloat" while leveraging battle-tested standards. We use
// `golang.org/x/oauth2` internally to handle the complex state/nonce handshakes safely,
// but expose a dead-simple `SocialHub.Driver("github").Redirect()` API to the developer.
// Architecture:
// - Provider: An interface that all OAuth drivers (Google, GitHub) must implement.
// - Hub: A centralized registry holding initialized provider drivers.
// Choice:
// The Hub uses a concurrent `sync.RWMutex` map. This allows developers to register custom
// enterprise providers at runtime while ensuring thread-safety.

// Hub manages OAuth2 provider registrations.
type Hub struct {
	providers map[string]Provider
	mu        sync.RWMutex
}

// New creates a new SocialHub instance.
func New() *Hub {
	return &Hub{
		providers: make(map[string]Provider),
	}
}

// Register adds a new configured provider driver to the hub.
func (h *Hub) Register(name string, provider Provider) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.providers[name] = provider
}

// Driver retrieves a configured provider by name.
// It returns an error if the provider is not registered.
func (h *Hub) Driver(name string) (Provider, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	provider, exists := h.providers[name]
	if !exists {
		return nil, fmt.Errorf("socialhub: driver '%s' is not registered", name)
	}
	return provider, nil
}
