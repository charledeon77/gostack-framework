/*
Purpose:
Package cors provides standard Cross-Origin Resource Sharing (CORS) middleware for GoStack (PAR-09).
It handles preflight OPTIONS requests, allowed origins, headers, methods, credentials, and max-age.
*/
package cors

import (
	"net/http"
	"strconv"
	"strings"
)

// Config defines the configuration options for the CORS middleware.
type Config struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowCredentials bool
	MaxAge           int // in seconds
}

// DefaultConfig returns safe, common defaults for web APIs.
func DefaultConfig() Config {
	return Config{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowedHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With", "X-CSRF-Token"},
		ExposedHeaders: []string{"Content-Length", "X-Request-ID"},
		MaxAge:         86400, // 24 hours
	}
}

// Middleware creates a standard HTTP middleware handler applying the CORS policy.
func Middleware(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Non-CORS request, pass through
				next.ServeHTTP(w, r)
				return
			}

			// Validate and set origin
			allowedOrigin := isOriginAllowed(origin, cfg.AllowedOrigins)
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				if cfg.AllowCredentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if len(cfg.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
				}
			}

			// Handle preflight OPTIONS request
			if r.Method == http.MethodOptions {
				if len(cfg.AllowedMethods) > 0 {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
				}
				if len(cfg.AllowedHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
				}
				if cfg.MaxAge > 0 {
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.MaxAge))
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func isOriginAllowed(origin string, allowed []string) string {
	for _, o := range allowed {
		if o == "*" {
			return "*"
		}
		if strings.EqualFold(o, origin) {
			return origin
		}
	}
	return ""
}

