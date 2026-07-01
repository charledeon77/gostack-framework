package auth

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/http"
	netHTTP "net/http"
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

// AuthThrottle is a specialized rate-limiter for authentication endpoints.
//
// Unlike a generic IP-based throttle, AuthThrottle keys on a composite of the
// client IP and the submitted credential (email/username) — blocking both
// IP-based brute-force attacks and distributed credential-stuffing attacks
// where many IPs target the same account.
//
// Usage:
//
//	router.Post("/login", loginHandler, auth.AuthThrottle(5, time.Minute))
func AuthThrottle(limit int, period time.Duration) http.Middleware {
	type clientEntry struct {
		timestamps []time.Time
	}
	var mu sync.Mutex
	clients := make(map[string]*clientEntry)

	return func(ctx *http.Context, next http.NextHandler) error {
		ip := resolveClientIP(ctx.Request)

		// Extract the credential field from the posted form or JSON body.
		// We inspect common field names used for login identifiers.
		credential := ""
		if err := ctx.Request.ParseForm(); err == nil {
			for _, field := range []string{"email", "username", "login", "identifier"} {
				if v := ctx.Request.FormValue(field); v != "" {
					credential = strings.ToLower(strings.TrimSpace(v))
					break
				}
			}
		}

		// Build the composite throttle key: IP + credential (or IP-only as fallback).
		key := ip
		if credential != "" {
			key = fmt.Sprintf("%s|%s", ip, credential)
		}

		mu.Lock()
		entry, exists := clients[key]
		if !exists {
			entry = &clientEntry{}
			clients[key] = entry
		}

		now := time.Now()
		cutoff := now.Add(-period)
		var active []time.Time
		for _, ts := range entry.timestamps {
			if ts.After(cutoff) {
				active = append(active, ts)
			}
		}

		if len(active) >= limit {
			mu.Unlock()
			ctx.Writer.Header().Set("Retry-After", fmt.Sprintf("%d", int(period.Seconds())))
			ctx.Writer.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
			ctx.Writer.Header().Set("X-RateLimit-Remaining", "0")
			ctx.Writer.WriteHeader(netHTTP.StatusTooManyRequests)
			_ = ctx.JSON(netHTTP.StatusTooManyRequests, map[string]string{
				"error":   "Too Many Requests",
				"message": "Login attempt limit exceeded. Please wait before trying again.",
			})
			return nil
		}

		entry.timestamps = append(active, now)
		mu.Unlock()

		return next(ctx)
	}
}

// resolveClientIP reads the originating IP address from proxy headers then remote addr.
func resolveClientIP(r *netHTTP.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if comma := strings.Index(ip, ","); comma != -1 {
			return strings.TrimSpace(ip[:comma])
		}
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
