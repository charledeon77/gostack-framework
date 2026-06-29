/*
Purpose:
This file implements the session lifecycle middleware and clean context helper accessors.
It bridges incoming HTTP client cookie requests with our backend SessionStore interfaces.

Philosophy:
State resolution must be completely transparent to the application. By integrating session loading
as an HTTP middleware layer, controllers get automatic, risk-free access to user session stores
on demand, ensuring the developer experience remains fluid and standard-library oriented.

Architecture:
Acts as a standard GoStack Middleware. It runs early in the request routing pipeline to:
1. Inspect request cookies.
2. Resolve or generate unique session tokens.
3. Bind session references directly to the request Context values.
4. Set response cookies before handlers flush headers.
5. Save state changes back to the store after the pipeline completes.

Choice:
We chose to generate 32-byte cryptographically secure hexadecimal IDs using crypto/rand rather
than basic timestamp counters or UUIDs to protect GoStack applications against session hijacking
and brute-force guessing attacks out-of-the-box. We commit the cookie prior to running next(ctx)
because standard Go response streams lock headers upon the first Write/WriteHeader invocation.

Implementation:
- SessionMiddleware: Middleware generator registering the lifecycle hooks.
- GetSession: Static helper method to fetch resolved session pointers cleanly from Context.
- generateSessionID: Private cryptographic utility to seed new session ID strings.
*/
package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/charledeon77/gostack-framework/framework/contract"
	netHTTP "net/http"
)

// SessionMiddleware builds an HTTP middleware interceptor targeting session lifecycles.
//
// PARAMETERS:
//   - store: The target contract.SessionStore instance to persist session data.
//   - cookieName: The identifier key string of the cookie (default: "gostack_session").
//
// RETURNS:
//   - A Middleware function to hook directly into the router setup.
func SessionMiddleware(store contract.SessionStore, cookieName string) Middleware {
	if cookieName == "" {
		cookieName = "gostack_session"
	}
	
	return func(ctx *Context, next NextHandler) error {
		var sessionID string
		
		// 1. Inspect request cookies for existing session tokens
		cookie, err := ctx.Request.Cookie(cookieName)
		if err == nil && cookie != nil && cookie.Value != "" {
			sessionID = cookie.Value
		} else {
			// 2. Generate a secure fallback ID if missing
			sessionID = generateSessionID()
		}

		// 3. Retrieve or create session via the store contract
		sess, err := store.Load(sessionID)
		if err != nil {
			return err
		}

		// 4. Inject the session reference into the request-scoped Context
		ctx.Set("session", sess)
		ctx.Request = ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), contract.SessionContextKey, sess))

		// 5. Commit response cookie headers before next() triggers WriteHeader/Write
		netHTTP.SetCookie(ctx.Writer, &netHTTP.Cookie{
			Name:     cookieName,
			Value:    sessionID,
			Path:     "/",
			HttpOnly: true,
			Secure:   false, // Set false for local development compatibility
		})

		// 6. Execute subsequent middlewares and handler controllers
		err = next(ctx)

		// 7. Persist session updates back to the storage adapter
		if saveErr := store.Save(sess); saveErr != nil {
			if err == nil {
				return saveErr
			}
		}

		return err
	}
}

// GetSession extracts the active resolved session context out of the Context values repository.
//
// PARAMETERS:
//   - ctx: The active HTTP context pointer passed to your route handler.
//
// RETURNS:
//   - The active contract.Session struct. Returns nil if the session middleware was not registered.
func GetSession(ctx *Context) contract.Session {
	val := ctx.Get("session")
	if val == nil {
		return nil
	}
	sess, ok := val.(contract.Session)
	if !ok {
		return nil
	}
	return sess
}

// generateSessionID builds a cryptographically secure 32-byte hexadecimal random token string.
func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("session: failed to generate secure random session ID")
	}
	return hex.EncodeToString(b)
}
