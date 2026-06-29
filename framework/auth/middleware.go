/*
Purpose:
This file implements standard routing middleware filters for GoStack authentication.
It provides filters to protect routes (RequireAuth) and restrict authenticated access (Guest).

Philosophy:
Access control should be transparent, declarative, and format-aware. Middleware must seamlessly
handle both web requests (with redirects) and API requests (with JSON payloads) without
polluting the controller logic. Caching user retrieval in the request context prevents
multiple database queries on complex pipelines.

Architecture:
Part of the auth package. The middleware wraps the standard GoStack http.Middleware contract.
It uses standard request context propagation to pass user caches down the handler pipeline.

Choice:
We chose to inspect request headers (Accept / Content-Type) and guard names to auto-detect API
vs Web contexts, rather than forcing the developer to register different middleware classes.

Implementation:
- RequireAuth: checks authentication, redirecting to /login or returning 401 JSON.
- Guest: prevents authenticated access to auth routes, redirecting to /home.
*/
package auth

import (
	"context"
	"github.com/charledeon77/gostack/framework/contract"
	"github.com/charledeon77/gostack/framework/http"
	netHTTP "net/http"
	"strings"
)

// RequireAuth restricts route access to authenticated users.
// Unauthenticated requests receive a 401 JSON response or redirect to /login.
func RequireAuth(manager *AuthManager, guardName string) http.Middleware {
	return func(ctx *http.Context, next http.NextHandler) error {
		guard := manager.Guard(guardName)

		// Initialize request context cache if not already present
		req := ctx.Request
		if req.Context().Value(contract.UserContextKey) == nil {
			cache := &contract.AuthCache{}
			ctx.Request = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache))
		}

		if !guard.Check(ctx.Request) {
			accept := ctx.Request.Header.Get("Accept")
			contentType := ctx.Request.Header.Get("Content-Type")
			isAPI := strings.Contains(accept, "application/json") || 
				strings.Contains(contentType, "application/json") || 
				guardName == "api" || guardName == "token"

			if isAPI {
				return ctx.JSON(netHTTP.StatusUnauthorized, map[string]string{
					"error": "Unauthorized",
				})
			}

			// Web redirect to login
			ctx.Writer.Header().Set("Location", "/login")
			ctx.Writer.WriteHeader(netHTTP.StatusFound)
			return nil
		}

		return next(ctx)
	}
}

// Guest redirects authenticated users away from public auth pages (e.g. login/register).
func Guest(manager *AuthManager, guardName string) http.Middleware {
	return func(ctx *http.Context, next http.NextHandler) error {
		guard := manager.Guard(guardName)

		req := ctx.Request
		if req.Context().Value(contract.UserContextKey) == nil {
			cache := &contract.AuthCache{}
			ctx.Request = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache))
		}

		if guard.Check(ctx.Request) {
			// Redirect to dashboard/home if already logged in
			ctx.Writer.Header().Set("Location", "/home")
			ctx.Writer.WriteHeader(netHTTP.StatusFound)
			return nil
		}

		return next(ctx)
	}
}
