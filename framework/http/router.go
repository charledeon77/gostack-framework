// Package http (Navigator, Bridge, Tempose, and Glide) houses the pipeline, routing, and context tracking systems
// governing the lifecycle of incoming web and API traffic.
package http

// Router acts as the central traffic controller for the GoStack framework.
//
// DESIGN PHILOSOPHY:
// Unlike traditional static routers, the GoStack Router is "Pipeline-Aware."
// Every route is treated as an isolated execution unit consisting of a
// linear middleware onion shell and a terminal controller action.
//
// ROUTING MECHANICS:
// Routes are keyed as "METHOD /path" (e.g. "GET /users", "POST /users") so
// the engine can dispatch by both HTTP verb and path with a single map lookup.
type Router struct {
	// routes maps "METHOD /path" to a pre-compiled pipeline execution closure.
	routes map[string]func(ctx *Context) error
}

// NewRouter initializes a fresh, memory-isolated routing table.
func NewRouter() *Router {
	return &Router{
		routes: make(map[string]func(ctx *Context) error),
	}
}

// register is the internal helper that compiles a route pipeline and stores it
// under the "METHOD /path" key, shared by all public verb methods.
func (r *Router) register(method, path string, handler func(ctx *Context), middleware []Middleware) {
	pipeline := NewPipeline()
	pipeline.Through(middleware...)
	r.routes[method+" "+path] = func(ctx *Context) error {
		return pipeline.Run(ctx, func(c *Context) error {
			handler(c)
			return nil
		})
	}
}

// Get registers a GET route.
func (r *Router) Get(path string, handler func(ctx *Context), middleware ...Middleware) {
	r.register("GET", path, handler, middleware)
}

// Post registers a POST route.
func (r *Router) Post(path string, handler func(ctx *Context), middleware ...Middleware) {
	r.register("POST", path, handler, middleware)
}

// Put registers a PUT route.
func (r *Router) Put(path string, handler func(ctx *Context), middleware ...Middleware) {
	r.register("PUT", path, handler, middleware)
}

// Patch registers a PATCH route.
func (r *Router) Patch(path string, handler func(ctx *Context), middleware ...Middleware) {
	r.register("PATCH", path, handler, middleware)
}

// Delete registers a DELETE route.
func (r *Router) Delete(path string, handler func(ctx *Context), middleware ...Middleware) {
	r.register("DELETE", path, handler, middleware)
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
//
// Example:
//
//	router.Group("/api", authMiddleware, func(g *RouteGroup) {
//	    g.Get("/users",  usersHandler)
//	    g.Post("/users", createHandler)
//	})
func (r *Router) Group(prefix string, middleware []Middleware, fn func(g *RouteGroup)) {
	g := &RouteGroup{
		router:     r,
		prefix:     prefix,
		middleware: middleware,
	}
	fn(g)
}

// Get registers a GET route scoped to the group's prefix and middleware.
func (g *RouteGroup) Get(path string, handler func(ctx *Context), extra ...Middleware) {
	g.router.register("GET", g.prefix+path, handler, append(g.middleware, extra...))
}

// Post registers a POST route scoped to the group.
func (g *RouteGroup) Post(path string, handler func(ctx *Context), extra ...Middleware) {
	g.router.register("POST", g.prefix+path, handler, append(g.middleware, extra...))
}

// Put registers a PUT route scoped to the group.
func (g *RouteGroup) Put(path string, handler func(ctx *Context), extra ...Middleware) {
	g.router.register("PUT", g.prefix+path, handler, append(g.middleware, extra...))
}

// Patch registers a PATCH route scoped to the group.
func (g *RouteGroup) Patch(path string, handler func(ctx *Context), extra ...Middleware) {
	g.router.register("PATCH", g.prefix+path, handler, append(g.middleware, extra...))
}

// Delete registers a DELETE route scoped to the group.
func (g *RouteGroup) Delete(path string, handler func(ctx *Context), extra ...Middleware) {
	g.router.register("DELETE", g.prefix+path, handler, append(g.middleware, extra...))
}

// GetRoutes returns the raw registry table, keyed as "METHOD /path".
// Used by the Engine during the HTTP request lifecycle.
func (r *Router) GetRoutes() map[string]func(ctx *Context) error {
	return r.routes
}