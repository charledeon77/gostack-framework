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
	"github.com/charledeon77/gostack-framework/framework/contract"
	"net/http"
	"strings"
)

// TokenGuard handles stateless authentication using API bearer tokens.
type TokenGuard struct {
	provider        contract.UserProvider
	allowQueryToken bool // Explicit opt-in only. Query tokens leak into access logs and referrers (SEC-30).
}

// NewTokenGuard constructs a new TokenGuard with a user retrieval provider.
func NewTokenGuard(provider contract.UserProvider) *TokenGuard {
	return &TokenGuard{provider: provider, allowQueryToken: false}
}

// AllowQueryToken enables token extraction from URL query parameter ?api_token=.
// WARNING: Passing tokens in URLs exposes credentials in proxy logs and browser history (SEC-30).
func (g *TokenGuard) AllowQueryToken(allow bool) *TokenGuard {
	g.allowQueryToken = allow
	return g
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
		cacheUser(r, nil)
		return nil, false
	}

	// 3. Resolve user using the Credentials hook
	user, err := g.provider.RetrieveByCredentials(map[string]any{"token": token})
	if err != nil || user == nil {
		cacheUser(r, nil)
		return nil, false
	}

	cacheUser(r, user)
	return user, true
}

// Login is a stateless no-op for TokenGuard.
func (g *TokenGuard) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	cacheUser(r, user)
	return nil
}

// Logout is a stateless no-op for TokenGuard.
func (g *TokenGuard) Logout(w http.ResponseWriter, r *http.Request) error {
	cacheUser(r, nil)
	return nil
}

func (g *TokenGuard) getToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	if g.allowQueryToken {
		return r.URL.Query().Get("api_token")
	}
	return ""
}
