// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
package http

import (
	"net/http"
	"strings"
)

// Engine represents the operational HTTP server configuration block.
// It manages the template view renderer, routing tables, and server lifecycle options.
type Engine struct {
	Router  *Router
	Tempose *Tempose
}

// NewEngine establishes an operational HTTP processing core.
//
// Parameters:
//   - router: An initialized routing context registry.
//   - tempose: A configured template view engine instance.
func NewEngine(router *Router, tempose *Tempose) *Engine {
	return &Engine{
		Router:  router,
		Tempose: tempose,
	}
}

// ServeHTTP acts as the low-level execution entry point required by Go's standard http.Server interface.
// Every single inbound network connection request shifts through this method pass.
//
// How It Works:
//  1. It monitors incoming connection request patterns against the internal Router table.
//  2. If a match is found, it instantiates the framework's custom Context block.
//  3. It populates that Context with the raw response stream, the request metadata, and the Engine's direct Tempose reference.
//  4. It dispatches execution to the controller handler seamlessly.
//  5. If no route matches, it writes a clean RFC-standard 404 Not Found error state.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	routes := e.Router.GetRoutes()

	// Dispatch key: "METHOD /path" — enables distinct GET/POST/PUT/DELETE handlers
	// on the same path without conflict.
	key := r.Method + " " + r.URL.Path

	if handler, exists := routes[key]; exists {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		handler(ctx)
		return
	}

	// Try dynamic/parameterized routing match
	if route, params := e.Router.Match(r.Method, r.URL.Path); route != nil {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		ctx.Set("params", params)
		route.Handler(ctx)
		return
	}

	// Return 405 if the path exists under a different method, 404 otherwise.
	trimmedPath := strings.Trim(r.URL.Path, "/")
	var pathSegments []string
	if trimmedPath != "" {
		pathSegments = strings.Split(trimmedPath, "/")
	}

	for _, route := range e.Router.GetDynamicRoutes() {
		if _, matched := matchRoute(route.Segments, pathSegments); matched {
			if e.Router.methodNotAllowedHandler != nil {
				ctx := &Context{
					Writer:  w,
					Request: r,
					Tempose: e.Tempose,
					Router:  e.Router,
				}
				e.Router.methodNotAllowedHandler(ctx)
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
				_, _ = w.Write([]byte("405 Method Not Allowed - GoStack Engine"))
			}
			return
		}
	}

	if e.Router.notFoundHandler != nil {
		ctx := &Context{
			Writer:  w,
			Request: r,
			Tempose: e.Tempose,
			Router:  e.Router,
		}
		e.Router.notFoundHandler(ctx)
	} else {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 Page Not Found - GoStack Engine"))
	}
}