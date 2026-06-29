/*
Purpose:
This file implements stateless API Token-based authentication (TokenGuard) for GoStack.

Philosophy:
REST APIs and headless microservices should remain stateless. Our TokenGuard inspects each
incoming request's headers and query parameters for active bearer tokens. Like the SessionGuard,
it leverages request-scoped caching to optimize DB access during a single request lifecycle.

Architecture:
Part of the auth package. Implements contract.Guard. Interacts with contract.UserProvider's
RetrieveByCredentials mapping token credentials directly.

Choice:
We chose to look for tokens in both the "Authorization: Bearer <token>" header and the
"api_token" URL query parameter to maximize client request flexibility.

Implementation:
- TokenGuard: stateless check, User retrieval, getToken parser, and no-op Login/Logout hooks.
*/
package auth

import (
	"github.com/charledeon77/gostack/framework/contract"
	"net/http"
	"strings"
)

// TokenGuard handles stateless authentication using API bearer tokens.
type TokenGuard struct {
	provider contract.UserProvider
}

// NewTokenGuard constructs a new TokenGuard with a user retrieval provider.
func NewTokenGuard(provider contract.UserProvider) *TokenGuard {
	return &TokenGuard{provider: provider}
}

// Check validates if the request contains a valid API token.
func (g *TokenGuard) Check(r *http.Request) bool {
	_, ok := g.User(r)
	return ok
}

// User extracts the bearer token, retrieves the matching user, and caches them in request context.
func (g *TokenGuard) User(r *http.Request) (contract.Authenticatable, bool) {
	// 1. Inspect request context cache first
	if cacheVal := r.Context().Value(contract.UserContextKey); cacheVal != nil {
		if cache, ok := cacheVal.(*contract.AuthCache); ok && cache.Loaded {
			return cache.User, cache.User != nil
		}
	}

	// 2. Extract token from Header or Query parameters
	token := g.getToken(r)
	if token == "" {
		g.cacheUser(r, nil)
		return nil, false
	}

	// 3. Resolve user using the Credentials hook
	user, err := g.provider.RetrieveByCredentials(map[string]any{"token": token})
	if err != nil || user == nil {
		g.cacheUser(r, nil)
		return nil, false
	}

	g.cacheUser(r, user)
	return user, true
}

// Login is a stateless no-op for TokenGuard.
func (g *TokenGuard) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	g.cacheUser(r, user)
	return nil
}

// Logout is a stateless no-op for TokenGuard.
func (g *TokenGuard) Logout(w http.ResponseWriter, r *http.Request) error {
	g.cacheUser(r, nil)
	return nil
}

func (g *TokenGuard) getToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return r.URL.Query().Get("api_token")
}

func (g *TokenGuard) cacheUser(r *http.Request, user contract.Authenticatable) {
	if cacheVal := r.Context().Value(contract.UserContextKey); cacheVal != nil {
		if cache, ok := cacheVal.(*contract.AuthCache); ok {
			cache.User = user
			cache.Loaded = true
		}
	}
}
