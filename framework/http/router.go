/*
Navigator — GoStack HTTP Router & Path Resolution Subsystem.

Naming Metaphor: Why Navigator?
The GoStack HTTP Router was named Navigator because a "navigator" charts courses through
uncharted waters, calculating routes and guiding vessels swiftly and safely to their destination.
Navigator inspects the incoming HTTP request stream, traverses radix route segments, resolves dynamic
path parameters, and charts the execution journey straight to the designated presentation controller.
*/
package http

import (
	"fmt"
	"log"
	"strings"
)

// Route represents a registered HTTP path pattern.
type Route struct {
	RouteName string
	Method    string
	Pattern   string
	Segments  []string
	HasParams bool
	Handler   func(ctx *Context) error
	router    *Router
}

// Name registers a logical name for the route, allowing dynamic URL building.
func (r *Route) Name(name string) *Route {
	r.RouteName = name
	if r.router != nil {
		r.router.namedRoutes[name] = r
	}
	return r
}

// Router acts as the central traffic controller for the GoStack framework.
type Router struct {
	routes                  map[string]func(ctx *Context) error
	dynamicRoutes           []*Route
	namedRoutes             map[string]*Route
	notFoundHandler         func(ctx *Context)
	methodNotAllowedHandler func(ctx *Context)
	errorHandler            func(ctx *Context, err error)
}

// NewRouter initializes a fresh, memory-isolated routing table.
func NewRouter() *Router {
	return &Router{
		routes:        make(map[string]func(ctx *Context) error),
		dynamicRoutes: make([]*Route, 0),
		namedRoutes:   make(map[string]*Route),
	}
}

// SetNotFoundHandler registers a custom handler for 404 Not Found responses.
func (r *Router) SetNotFoundHandler(handler func(ctx *Context)) {
	r.notFoundHandler = handler
}

// SetMethodNotAllowedHandler registers a custom handler for 405 Method Not Allowed responses.
func (r *Router) SetMethodNotAllowedHandler(handler func(ctx *Context)) {
	r.methodNotAllowedHandler = handler
}

// SetErrorHandler registers a custom handler for unhandled errors produced by routes or middlewares (SEC-05).
func (r *Router) SetErrorHandler(handler func(ctx *Context, err error)) {
	r.errorHandler = handler
}

// Handle registers a route that returns an error directly, dispatching it cleanly to error handlers (SEC-05).
func (r *Router) Handle(method, path string, handler func(ctx *Context) error, middleware ...Middleware) *Route {
	return r.registerWithError(method, path, handler, middleware)
}

// register compiles a route pipeline and stores it in the routing registry.
// It warns if a duplicate method+path combination is registered.
func (r *Router) register(method, path string, handler func(ctx *Context), middleware []Middleware) *Route {
	return r.registerWithError(method, path, func(c *Context) error {
		handler(c)
		return nil
	}, middleware)
}

func (r *Router) registerWithError(method, path string, handler func(ctx *Context) error, middleware []Middleware) *Route {
	pipeline := NewPipeline()
	pipeline.Through(middleware...)
	runPipeline := func(ctx *Context) error {
		return pipeline.Run(ctx, func(c *Context) error {
			return handler(c)
		})
	}

	segments, hasParams := parsePattern(path)

	// ── Conflict Detection ──────────────────────────────────────────────────────
	// Check static routes map for exact duplicates.
	staticKey := method + " " + path
	if _, exists := r.routes[staticKey]; exists {
		log.Printf("[GoStack Router] WARNING: Duplicate route detected: %s %s — previous registration overwritten.", method, path)
	}
	// Check dynamic routes for parameter pattern duplicates.
	for _, existing := range r.dynamicRoutes {
		if existing.Method == method && existing.Pattern == path {
			log.Printf("[GoStack Router] WARNING: Duplicate dynamic route detected: %s %s — previous registration overwritten.", method, path)
			break
		}
	}

	route := &Route{
		Method:    method,
		Pattern:   path,
		Segments:  segments,
		HasParams: hasParams,
		Handler:   runPipeline,
		router:    r,
	}
	r.dynamicRoutes = append(r.dynamicRoutes, route)

	if !hasParams {
		r.routes[method+" "+path] = runPipeline
	}
	return route
}

// Get registers a GET route.
func (r *Router) Get(path string, handler func(ctx *Context), middleware ...Middleware) *Route {
	return r.register("GET", path, handler, middleware)
}

// Post registers a POST route.
func (r *Router) Post(path string, handler func(ctx *Context), middleware ...Middleware) *Route {
	return r.register("POST", path, handler, middleware)
}

// Put registers a PUT route.
func (r *Router) Put(path string, handler func(ctx *Context), middleware ...Middleware) *Route {
	return r.register("PUT", path, handler, middleware)
}

// Patch registers a PATCH route.
func (r *Router) Patch(path string, handler func(ctx *Context), middleware ...Middleware) *Route {
	return r.register("PATCH", path, handler, middleware)
}

// Delete registers a DELETE route.
func (r *Router) Delete(path string, handler func(ctx *Context), middleware ...Middleware) *Route {
	return r.register("DELETE", path, handler, middleware)
}

// RouteGroup holds a shared prefix and a stack of group-scoped middleware that
// are prepended to every route registered through the group.
type RouteGroup struct {
	router     *Router
	prefix     string
	middleware []Middleware
}

// Group creates a new RouteGroup. All routes registered via the callback receive
// the given prefix and share the supplied middleware stack.
func (r *Router) Group(prefix string, middleware []Middleware, fn func(g *RouteGroup)) {
	g := &RouteGroup{
		router:     r,
		prefix:     prefix,
		middleware: middleware,
	}
	fn(g)
}

// Get registers a GET route scoped to the group's prefix and middleware.
func (g *RouteGroup) Get(path string, handler func(ctx *Context), extra ...Middleware) *Route {
	return g.router.register("GET", g.prefix+path, handler, append(g.middleware, extra...))
}

// Post registers a POST route scoped to the group.
func (g *RouteGroup) Post(path string, handler func(ctx *Context), extra ...Middleware) *Route {
	return g.router.register("POST", g.prefix+path, handler, append(g.middleware, extra...))
}

// Put registers a PUT route scoped to the group.
func (g *RouteGroup) Put(path string, handler func(ctx *Context), extra ...Middleware) *Route {
	return g.router.register("PUT", g.prefix+path, handler, append(g.middleware, extra...))
}

// Patch registers a PATCH route scoped to the group.
func (g *RouteGroup) Patch(path string, handler func(ctx *Context), extra ...Middleware) *Route {
	return g.router.register("PATCH", g.prefix+path, handler, append(g.middleware, extra...))
}

// Delete registers a DELETE route scoped to the group.
func (g *RouteGroup) Delete(path string, handler func(ctx *Context), extra ...Middleware) *Route {
	return g.router.register("DELETE", g.prefix+path, handler, append(g.middleware, extra...))
}

// GetRoutes returns the raw registry table, keyed as "METHOD /path".
func (r *Router) GetRoutes() map[string]func(ctx *Context) error {
	return r.routes
}

// GetDynamicRoutes returns all registered routes, both static and parameterized.
func (r *Router) GetDynamicRoutes() []*Route {
	return r.dynamicRoutes
}

// Match searches for a matching dynamic route given an HTTP method and request path.
func (r *Router) Match(method, path string) (*Route, map[string]string) {
	trimmed := strings.Trim(path, "/")
	var pathSegments []string
	if trimmed != "" {
		pathSegments = strings.Split(trimmed, "/")
	}

	for _, route := range r.dynamicRoutes {
		if route.Method != method {
			continue
		}
		params, matched := matchRoute(route.Segments, pathSegments)
		if matched {
			return route, params
		}
	}
	return nil, nil
}

// URL generates a path pattern for the named route, replacing parameters with values.
func (r *Router) URL(name string, params map[string]string) (string, error) {
	route, exists := r.namedRoutes[name]
	if !exists {
		return "", fmt.Errorf("route %q not found", name)
	}

	pattern := route.Pattern
	parts := strings.Split(pattern, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			paramKey := strings.TrimPrefix(part, ":")
			val, ok := params[paramKey]
			if !ok {
				return "", fmt.Errorf("missing parameter %q for route %q", paramKey, name)
			}
			parts[i] = val
		} else if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			paramKey := part[1 : len(part)-1]
			val, ok := params[paramKey]
			if !ok {
				return "", fmt.Errorf("missing parameter %q for route %q", paramKey, name)
			}
			parts[i] = val
		} else if part == "*" {
			val, ok := params["*"]
			if !ok {
				return "", fmt.Errorf("missing wildcard * parameter for route %q", name)
			}
			// Strip leading slash if supplied to prevent duplicate slashes
			val = strings.TrimPrefix(val, "/")
			parts[i] = val
		}
	}
	return strings.Join(parts, "/"), nil
}

func parsePattern(pattern string) ([]string, bool) {
	trimmed := strings.Trim(pattern, "/")
	if trimmed == "" {
		return []string{}, false
	}
	segments := strings.Split(trimmed, "/")
	hasParams := false
	for _, seg := range segments {
		if strings.HasPrefix(seg, ":") || (strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")) || seg == "*" {
			hasParams = true
		}
	}
	return segments, hasParams
}

func matchRoute(patternSegments []string, pathSegments []string) (map[string]string, bool) {
	params := make(map[string]string)
	hasWildcard := len(patternSegments) > 0 && patternSegments[len(patternSegments)-1] == "*"

	if !hasWildcard && len(patternSegments) != len(pathSegments) {
		return nil, false
	}

	for i, patSeg := range patternSegments {
		if patSeg == "*" {
			params["*"] = "/" + strings.Join(pathSegments[i:], "/")
			return params, true
		}

		if i >= len(pathSegments) {
			return nil, false
		}

		reqSeg := pathSegments[i]

		if strings.HasPrefix(patSeg, ":") {
			paramName := strings.TrimPrefix(patSeg, ":")
			params[paramName] = reqSeg
		} else if strings.HasPrefix(patSeg, "{") && strings.HasSuffix(patSeg, "}") {
			paramName := patSeg[1 : len(patSeg)-1]
			params[paramName] = reqSeg
		} else if patSeg != reqSeg {
			return nil, false
		}
	}

	return params, true
}

// AllowedMethods returns all HTTP methods registered for the given path across static and dynamic routes (SEC-05).
func (r *Router) AllowedMethods(path string) []string {
	seen := make(map[string]bool)
	var methods []string

	// Check static routes
	for key := range r.routes {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 && parts[1] == path {
			if !seen[parts[0]] {
				seen[parts[0]] = true
				methods = append(methods, parts[0])
			}
		}
	}

	// Check dynamic routes
	trimmedPath := strings.Trim(path, "/")
	var pathSegments []string
	if trimmedPath != "" {
		pathSegments = strings.Split(trimmedPath, "/")
	}
	for _, route := range r.dynamicRoutes {
		if _, matched := matchRoute(route.Segments, pathSegments); matched {
			if !seen[route.Method] {
				seen[route.Method] = true
				methods = append(methods, route.Method)
			}
		}
	}

	return methods
}