/*
Purpose:
This file implements Cross-Site Request Forgery (CSRF) protection middleware for GoStack.

Philosophy:
Stateful, cookie-session authenticated applications are vulnerable to CSRF attacks where malicious
third-party pages trigger unwanted mutations. Our CSRF middleware defends against this by generating
a unique, cryptographically secure token stored in the session, and checking all unsafe state-changing
methods (POST, PUT, DELETE, PATCH) for a matching token in request headers or POST form bodies.

Architecture:
Part of the auth package. Employs the standard http.Middleware functional onion hook.
Uses crypto/rand to generate 32-byte secure hexadecimal string tokens.

Choice:
We chose to synchronize the session token with an un-encrypted client cookie (XSRF-TOKEN) and check
both the "X-CSRF-Token" and "X-XSRF-TOKEN" headers plus the "_token" POST parameter to support
both standard HTML forms and Javascript-based API clients (Axios, Fetch).

Implementation:
- CSRF: middleware generator verifying CSRF tokens for mutating requests.
- generateCSRFToken: private cryptographic randomness helper.
*/
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/http"
	netHTTP "net/http"
)

// CSRF builds middleware that enforces Cross-Site Request Forgery protection.
// It matches tokens from the session against header/POST form parameters.
func CSRF(cookieName string) http.Middleware {
	if cookieName == "" {
		cookieName = "XSRF-TOKEN"
	}
	return func(ctx *http.Context, next http.NextHandler) error {
		// 1. Resolve session from request-scoped context
		sessVal := ctx.Get("session")
		if sessVal == nil {
			ctx.Writer.WriteHeader(netHTTP.StatusForbidden)
			_, _ = ctx.Writer.Write([]byte("CSRF Error: Session is uninitialized"))
			return nil
		}
		sess, ok := sessVal.(contract.Session)
		if !ok {
			ctx.Writer.WriteHeader(netHTTP.StatusForbidden)
			_, _ = ctx.Writer.Write([]byte("CSRF Error: Session is invalid"))
			return nil
		}

		// 2. Fetch or seed a unique CSRF token in the session
		tokenVal := sess.Get("csrf_token")
		var token string
		if tokenVal == nil {
			token = generateCSRFToken()
			sess.Set("csrf_token", token)
		} else {
			token = tokenVal.(string)
		}

		// Expose token to request context values for view builders / page forms
		ctx.Set("csrf_token", token)

		// Set companion XSRF cookie so frontend JS clients can read it
		netHTTP.SetCookie(ctx.Writer, &netHTTP.Cookie{
			Name:     cookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: false, // Readable by client scripts (standard pattern for Axios/SPAs)
			Secure:   false,
		})

		// 3. Safe methods bypass validation check
		method := ctx.Request.Method
		if method == "GET" || method == "HEAD" || method == "OPTIONS" || method == "TRACE" {
			return next(ctx)
		}

		// 4. Validate token for mutations (POST, PUT, DELETE, PATCH)
		requestToken := ctx.Request.Header.Get("X-CSRF-Token")
		if requestToken == "" {
			requestToken = ctx.Request.Header.Get("X-XSRF-TOKEN")
		}
		if requestToken == "" {
			requestToken = ctx.Post("_token")
		}

		if requestToken == "" || requestToken != token {
			ctx.Writer.WriteHeader(netHTTP.StatusForbidden)
			_, _ = ctx.Writer.Write([]byte("403 Forbidden - CSRF Token Verification Failed"))
			return nil
		}

		return next(ctx)
	}
}

func generateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("csrf: failed to generate secure token")
	}
	return hex.EncodeToString(b)
}
