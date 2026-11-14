/*
Guard — GoStack Identity, Authentication & Security Sentinel Subsystem.

Naming Metaphor: Why Guard?
The GoStack Authentication Subsystem was named Guard because a "guard" stands vigilance at
the fortress gates, challenging every visitor, demanding proof of identity, verifying credentials,
and turning away imposters. Guard protects user sessions and API tokens, coordinating password
validation, session security, and CSRF defense to ensure that only authorized actors pass through.

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
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// SessionGuard handles authentication using cookie-based session state.
type SessionGuard struct {
	name       string
	provider   contract.UserProvider
	sessionKey string
	store      contract.SessionStore
	cookieName string
}

// NewSessionGuard builds a new SessionGuard targeting a named guard context.
func NewSessionGuard(name string, provider contract.UserProvider, storeAndCookie ...any) *SessionGuard {
	sg := &SessionGuard{
		name:       name,
		provider:   provider,
		sessionKey: "auth_" + name + "_user_id",
		cookieName: "gostack_session",
	}
	if len(storeAndCookie) > 0 {
		if s, ok := storeAndCookie[0].(contract.SessionStore); ok {
			sg.store = s
		}
	}
	if len(storeAndCookie) > 1 {
		if cn, ok := storeAndCookie[1].(string); ok && cn != "" {
			sg.cookieName = cn
		}
	}
	return sg
}

// SetStore attaches a SessionStore instance directly to the guard.
func (g *SessionGuard) SetStore(store contract.SessionStore) *SessionGuard {
	g.store = store
	return g
}

// SetCookieName sets the session cookie name used by this guard.
func (g *SessionGuard) SetCookieName(name string) *SessionGuard {
	g.cookieName = name
	return g
}

func (g *SessionGuard) getStore(r *http.Request) contract.SessionStore {
	if g.store != nil {
		return g.store
	}
	if val := r.Context().Value(contract.SessionStoreContextKey); val != nil {
		if store, ok := val.(contract.SessionStore); ok {
			return store
		}
	}
	return nil
}

func (g *SessionGuard) getCookieName(r *http.Request) string {
	if g.cookieName != "" {
		return g.cookieName
	}
	if val := r.Context().Value(contract.SessionCookieNameContextKey); val != nil {
		if name, ok := val.(string); ok && name != "" {
			return name
		}
	}
	return "gostack_session"
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
		cacheUser(r, nil)
		return nil, false
	}

	// 4. Resolve user via Provider contract
	user, err := g.provider.RetrieveByID(userID)
	if err != nil || user == nil {
		cacheUser(r, nil)
		return nil, false
	}

	// 5. Cache for subsequent calls within this request lifecycle
	cacheUser(r, user)
	return user, true
}

// Login commits user details to session, signing them in.
// Rotates the session ID to prevent session fixation attacks (SEC-03).
func (g *SessionGuard) Login(w http.ResponseWriter, r *http.Request, user contract.Authenticatable) error {
	sess, err := g.getSession(r)
	if err != nil {
		return err
	}

	store := g.getStore(r)
	cookieName := g.getCookieName(r)

	targetSess := sess
	if store != nil {
		newSess, rotateErr := store.Rotate(sess.ID())
		if rotateErr != nil {
			return rotateErr
		}
		targetSess = newSess

		// Re-emit session cookie with newly rotated session ID
		isSecure := true
		appEnv := strings.ToLower(os.Getenv("APP_ENV"))
		if appEnv == "local" || appEnv == "development" {
			isSecure = r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
		}
		maxAge := 7200
		if lifetimeStr := os.Getenv("SESSION_LIFETIME"); lifetimeStr != "" {
			if mins, parseErr := strconv.Atoi(lifetimeStr); parseErr == nil && mins > 0 {
				maxAge = mins * 60
			}
		}

		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    targetSess.ID(),
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   maxAge,
		})
	}

	targetSess.Set(g.sessionKey, user.GetID())
	if store != nil {
		if saveErr := store.Save(targetSess); saveErr != nil {
			return saveErr
		}
	}
	cacheUser(r, user)
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

// Logout deletes user session, destroys it server-side, and clears the client cookie (SEC-03).
func (g *SessionGuard) Logout(w http.ResponseWriter, r *http.Request) error {
	sess, err := g.getSession(r)
	if err != nil {
		return err
	}

	store := g.getStore(r)
	cookieName := g.getCookieName(r)

	if store != nil {
		_ = store.Destroy(sess.ID())
	}
	sess.Clear()

	// Clear client-side cookie
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	cacheUser(r, nil)
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
