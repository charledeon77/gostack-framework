// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
package http

import (
	"errors"
	"fmt"
	"log"
	netHTTP "net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

type headResponseWriter struct {
	netHTTP.ResponseWriter
}

func (h *headResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

// Engine represents the operational HTTP server configuration block.
// It manages the template view renderer, routing tables, and server lifecycle options.
type Engine struct {
	Router                 *Router
	Tempose                *Tempose
	SecurityHeadersConfig  SecurityHeadersConfig
	DisableSecurityHeaders bool
}

// NewEngine establishes an operational HTTP processing core with default security headers mounted (SEC-12).
//
// Parameters:
//   - router: An initialized routing context registry.
//   - tempose: A configured template view engine instance.
func NewEngine(router *Router, tempose *Tempose) *Engine {
	return &Engine{
		Router:                router,
		Tempose:               tempose,
		SecurityHeadersConfig: DefaultSecurityHeadersConfig(),
	}
}

// ServeHTTP acts as the low-level execution entry point required by Go's standard http.Server interface.
func (e *Engine) ServeHTTP(w netHTTP.ResponseWriter, r *netHTTP.Request) {
	// Global Crash-Catcher & Panic Recovery Shield (SEC-05)
	defer func() {
		if rec := recover(); rec != nil {
			stack := debug.Stack()
			log.Printf("[GoStack Critical Panic Recovered] %v\nStack Trace:\n%s", rec, stack)

			// Never drop the TCP socket ungracefully — return a standardized 500 response
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(netHTTP.StatusInternalServerError)
			_, _ = w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>500 Internal Server Error</title><style>body{font-family:sans-serif;background:#0b0f19;color:#f8fafc;padding:40px;display:flex;justify-content:center;align-items:center;height:80vh;} .card{background:#1e293b;padding:30px;border-radius:12px;border:1px solid #334155;max-width:600px;text-align:center;} h1{color:#ef4444;margin-top:0;} p{color:#94a3b8;line-height:1.6;}</style></head>
<body><div class="card"><h1>500 Internal Server Error</h1><p>An unexpected condition was encountered while processing this request. The application kernel intercepted the crash safely.</p></div></body></html>`)))
		}
	}()

	// Automatic production security headers at engine level (SEC-12)
	if !e.DisableSecurityHeaders {
		ApplySecurityHeaders(w, r, e.SecurityHeadersConfig)
	}

	// Enforce request body size limit (SEC-09)
	maxBody := int64(8 * 1024 * 1024) // 8MB default
	if customSize := os.Getenv("MAX_REQUEST_BODY_SIZE"); customSize != "" {
		if val, err := strconv.ParseInt(customSize, 10, 64); err == nil && val > 0 {
			maxBody = val
		}
	}
	if r.Body != nil {
		r.Body = netHTTP.MaxBytesReader(w, r.Body, maxBody)
	}

	dispatch := func(ctx *Context, fn func(*Context) error) {
		if err := fn(ctx); err != nil {
			var maxBytesErr *netHTTP.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(netHTTP.StatusRequestEntityTooLarge)
				_, _ = w.Write([]byte("413 Request Entity Too Large: request body exceeds configured limit"))
				return
			}
			if e.Router != nil && e.Router.errorHandler != nil {
				e.Router.errorHandler(ctx, err)
				return
			}
			w.WriteHeader(netHTTP.StatusInternalServerError)
			_, _ = w.Write([]byte("500 Internal Server Error - GoStack Engine"))
		}
	}

	effectiveMethod := r.Method
	effectiveWriter := w

	// HTTP Method Spoofing (PAR-10): Allow POST forms with _method=PUT/PATCH/DELETE
	// or X-HTTP-Method-Override header to match the overridden HTTP method.
	if r.Method == netHTTP.MethodPost {
		var override string
		if header := r.Header.Get("X-HTTP-Method-Override"); header != "" {
			override = header
		} else if query := r.URL.Query().Get("_method"); query != "" {
			override = query
		} else if r.PostForm != nil {
			override = r.PostFormValue("_method")
		} else if strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			_ = r.ParseForm()
			override = r.PostFormValue("_method")
		}

		if override != "" {
			upper := strings.ToUpper(override)
			if upper == netHTTP.MethodPut || upper == netHTTP.MethodPatch || upper == netHTTP.MethodDelete {
				effectiveMethod = upper
			}
		}
	}

	// Support HEAD method: route to GET if no explicit HEAD handler, discard response body (SEC-05)
	if r.Method == "HEAD" {
		if _, hasHead := e.Router.GetRoutes()["HEAD "+r.URL.Path]; !hasHead {
			if route, _ := e.Router.Match("HEAD", r.URL.Path); route == nil {
				effectiveMethod = "GET"
				effectiveWriter = &headResponseWriter{ResponseWriter: w}
			}
		}
	}

	routes := e.Router.GetRoutes()
	key := effectiveMethod + " " + r.URL.Path

	if handler, exists := routes[key]; exists {
		ctx := &Context{
			Writer:  effectiveWriter,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		dispatch(ctx, handler)
		return
	}

	// Try dynamic/parameterized routing match
	if route, params := e.Router.Match(effectiveMethod, r.URL.Path); route != nil {
		ctx := &Context{
			Writer:  effectiveWriter,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		ctx.Set("params", params)
		dispatch(ctx, route.Handler)
		return
	}

	// Collect all allowed methods for this path (static and dynamic) (SEC-05)
	allowed := e.Router.AllowedMethods(r.URL.Path)

	// Automatic OPTIONS handling (SEC-05)
	if r.Method == "OPTIONS" {
		if len(allowed) > 0 {
			hasOptions := false
			for _, m := range allowed {
				if m == "OPTIONS" {
					hasOptions = true
					break
				}
			}
			if !hasOptions {
				allowed = append(allowed, "OPTIONS")
			}
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			w.WriteHeader(netHTTP.StatusNoContent)
			return
		}
	}

	// 405 Method Not Allowed with Allow header (SEC-05)
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		if e.Router.methodNotAllowedHandler != nil {
			ctx := &Context{
				Writer:  w,
				Request: r,
				Tempose: e.Tempose,
				Router:  e.Router,
			}
			e.Router.methodNotAllowedHandler(ctx)
		} else {
			w.WriteHeader(netHTTP.StatusMethodNotAllowed)
			_, _ = w.Write([]byte("405 Method Not Allowed - GoStack Engine"))
		}
		return
	}

	// 404 Not Found
	if e.Router.notFoundHandler != nil {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		e.Router.notFoundHandler(ctx)
	} else {
		w.WriteHeader(netHTTP.StatusNotFound)
		_, _ = w.Write([]byte("404 Page Not Found - GoStack Engine"))
	}
}