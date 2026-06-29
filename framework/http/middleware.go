// Package http (Navigator, Bridge, Tempose, and Glide) houses the pipeline, routing, and context tracking systems
// governing the lifecycle of incoming web and API traffic.
package http

import (
	"fmt"
	"time"

	netHTTP "net/http"
)

// Middleware defines the strict functional contract for all GoStack interceptor layers.
//
// DESIGN PHILOSOPHY:
// GoStack implements an "Onion Architecture" for middleware execution. Each layer 
// acts as a protective shell around the controller action. By passing a 'next'
// callback, we allow middleware to execute logic both BEFORE and AFTER the 
// internal request handling occurs, enabling powerful cross-cutting concerns
// like logging execution time, header manipulation, and response modification.
type Middleware func(ctx *Context, next NextHandler) error

// NextHandler represents the recursive execution gateway to the next inner 
// layer of the onion or the final core controller action.
type NextHandler func(ctx *Context) error

// Pipeline coordinates and executes a sequential chain of middleware layers.
//
// ARCHITECTURAL RATIONALE:
// 1. Decoupled Pipeline: By separating the pipeline execution from the Router, 
//    we ensure that middleware can be tested in total isolation from HTTP routing logic.
// 2. Sequential Traversal: Unlike functional closure nesting (which creates 
//    deep, difficult-to-debug stack traces), the Pipeline uses a recursive 
//    pointer-based traversal. This maps cleanly to the expected O(n) execution 
//    order of an onion architecture.
// 3. Short-Circuiting: Because the middleware returns an error, any layer
//    can stop the request chain (e.g., Auth failure) by returning an error 
//    instead of invoking the 'next' gateway.
type Pipeline struct {
	middleware []Middleware
}

// NewPipeline initializes an empty, ready-to-populate middleware stack.
func NewPipeline() *Pipeline {
	return &Pipeline{
		middleware: make([]Middleware, 0),
	}
}

// Through registers one or more middleware interceptors into the pipeline stack.
// Returns the pointer to the pipeline to facilitate fluent, chained configuration.
func (p *Pipeline) Through(m ...Middleware) *Pipeline {
	p.middleware = append(p.middleware, m...)
	return p
}

// Run executes the middleware chain sequentially, terminating at the final 
// destination handler (usually a controller action).
//
// EXECUTION FLOW:
// It uses a recursive 'nexter' helper to build a chain of execution. Each 
// index in the pipeline is captured by a closure, ensuring that even if 
// multiple requests flow through the pipeline concurrently, their internal 
// index state remains isolated on their respective call stacks.
func (p *Pipeline) Run(ctx *Context, destination NextHandler) error {
	var nexter func(index int) NextHandler

	nexter = func(index int) NextHandler {
		return func(c *Context) error {
			// Base Case: If we have exhausted the middleware list, 
			// execute the core controller action.
			if index >= len(p.middleware) {
				return destination(c)
			}

			// Recursive Case: Fetch the current middleware and prepare 
			// the 'next' gateway for the subsequent layer.
			current := p.middleware[index]
			next := nexter(index + 1)

			// Execute the current layer with its dedicated next-step gateway.
			return current(c, next)
		}
	}

	// Trigger the initial outer shell of the onion.
	return nexter(0)(ctx)
}

// statusCapture wraps http.ResponseWriter to intercept the written status code.
type statusCapture struct {
	netHTTP.ResponseWriter
	code int
}

func (sc *statusCapture) WriteHeader(code int) {
	sc.code = code
	sc.ResponseWriter.WriteHeader(code)
}

// Logger is a production-ready request logging middleware.
// It prints the HTTP method, path, response status code, and elapsed duration
// for every request that passes through it.
//
// Usage:
//
//	router.Get("/", homeHandler, http.Logger)
//	// or applied globally:
//	router.Group("/api", []http.Middleware{http.Logger}, func(g *http.RouteGroup) { ... })
func Logger(ctx *Context, next NextHandler) error {
	start := time.Now()

	// Wrap the underlying ResponseWriter so we can capture the status code.
	sc := &statusCapture{ResponseWriter: ctx.Writer, code: netHTTP.StatusOK}
	ctx.Writer = sc

	err := next(ctx)

	elapsed := time.Since(start)
	fmt.Printf("[GoStack] %s %s → %d (%s)\n",
		ctx.Request.Method,
		ctx.Request.URL.Path,
		sc.code,
		elapsed.Round(time.Millisecond),
	)
	return err
}

// RequireAuth blocks access to downstream handlers if the request session
// does not contain a positive "authenticated" value.
//
// DESIGN RATIONALE:
// In fullstack web applications, protecting routes behind auth gates is a critical security concern.
// By placing this middleware early in the execution chain, we ensure unauthorized access is blocked
// before reaching sensitive controllers.
//
// CONFLICT RESOLUTION:
// Because the current package is named 'http', the standard library 'net/http' package is imported
// with the alias 'netHTTP' to avoid compiler namespace collisions.
func RequireAuth(ctx *Context, next NextHandler) error {
	sessVal := ctx.Get("session")
	if sessVal == nil {
		// Session middleware was not run; block access for safety.
		ctx.Writer.WriteHeader(netHTTP.StatusUnauthorized)
		_, _ = ctx.Writer.Write([]byte("Unauthorized: Session uninitialized"))
		return nil
	}

	type sessionInterface interface {
		Get(key string) any
	}
	sess, ok := sessVal.(sessionInterface)
	if !ok || sess.Get("authenticated") != true {
		ctx.Writer.WriteHeader(netHTTP.StatusUnauthorized)
		_, _ = ctx.Writer.Write([]byte("Unauthorized"))
		return nil
	}

	return next(ctx)
}