/*
Purpose:
This file implements Session-based authentication (SessionGuard) for the GoStack framework.

Philosophy:
Stateful web applications rely on cookie-based session identifiers to map requests to users.
Our SessionGuard provides a secure wrapper that handles setting keys in the session, pulling
user IDs out of the session, and calling the registered UserProvider. To prevent repeated
database hits during a single request, the guard utilizes an in-context cache container.

Architecture:
Part of the auth package. Implements contract.Guard. Interacts with contract.Session and
contract.UserProvider to load and persist authentication status.

Choice:
We chose request context caching using a pointer-based mutable AuthCache struct to prevent
N+1 DB lookup scenarios in middlewares/controllers without mutating request contexts repeatedly.

Implementation:
- SessionGuard: stores credentials checks, caching, manual Login, Logout, and Attempt actions.
*/
package auth

import (
	"github.com/charledeon77/gostack/framework/contract"
	"net/http"
)

// SessionGuard handles authentication using cookie-based session state.
type SessionGuard struct {
	name       string
	provider   contract.UserProvider
	sessionKey string
}

// NewSessionGuard builds a new SessionGuard targeting a named guard context.
func NewSessionGuard(name string, provider contract.UserProvider) *SessionGuard {
	return &SessionGuard{
		name:       name,
		provider:   provider,
		sessionKey: "auth_" + name + "_user_id",
	}
}

// Check verifies if the incoming request contains an authenticated session user.
func (g *SessionGuard) Check(r *http.Request) bool {
	_, ok := g.User(r)
	return ok
}

// User returns the authenticated user context, retrieving from session or per-request cache.
func (g *SessionGuard) User(r *http.Request) (contract.Authenticatable, bool) {
	// 1. Inspect request context cache first to save database lookup overhead
	if cacheVal := r.Context().Value(contract.UserContextKey); cacheVal != nil {
		if cache, ok := cacheVal.(*contract.AuthCache); ok && cache.Loaded {
			return cache.User, cache.User != nil
		}
	}

	// 2. Resolve active session from context
	sess, err := g.getSession(r)
	if err != nil {
		return nil, false
	}

	// 3. Extract the cached user ID
	userID := sess.Get(g.sessionKey)
	if userID == nil {
		g.cacheUser(r, nil)
		return nil, false
	}

	// 4. Resolve user via Provider contract
	user, err := g.provider.RetrieveByID(userID)
	if err != nil || user == nil {
		g.cacheUser(r, nil)
		return nil, false
	}

	// 5. Cache for subsequent calls within this request lifecycle
	g.cacheUser(r, user)
	return user, true
}

// Login commits user details to session, signing them in.
func (g *SessionGuard) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	sess, err := g.getSession(r)
	if err != nil {
		return err
	}

	sess.Set(g.sessionKey, user.GetID())
	g.cacheUser(r, user)
	return nil
}

// Attempt validates credentials against the provider and logs the user in if correct.
func (g *SessionGuard) Attempt(w http.ResponseWriter, r *http.Request, credentials map[string]any) error {
	user, err := g.provider.RetrieveByCredentials(credentials)
	if err != nil {
		return err
	}
	if !g.provider.ValidateCredentials(user, credentials) {
		return ErrInvalidCredentials
	}
	return g.Login(w, r, user)
}


// Logout deletes user ID from session.
func (g *SessionGuard) Logout(w http.ResponseWriter, r *http.Request) error {
	sess, err := g.getSession(r)
	if err != nil {
		return err
	}

	sess.Delete(g.sessionKey)
	g.cacheUser(r, nil)
	return nil
}

func (g *SessionGuard) getSession(r *http.Request) (contract.Session, error) {
	val := r.Context().Value(contract.SessionContextKey)
	if val == nil {
		return nil, ErrSessionUninitialized
	}
	sess, ok := val.(contract.Session)
	if !ok {
		return nil, ErrSessionUninitialized
	}
	return sess, nil
}

func (g *SessionGuard) cacheUser(r *http.Request, user contract.Authenticatable) {
	if cacheVal := r.Context().Value(contract.UserContextKey); cacheVal != nil {
		if cache, ok := cacheVal.(*contract.AuthCache); ok {
			cache.User = user
			cache.Loaded = true
		}
	}
}
