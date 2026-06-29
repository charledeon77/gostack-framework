// Package foundation (Citadel) provides the core orchestration layer for the Gostack framework.
// It is responsible for booting the application, managing lifecycle events,
// and coordinating between the HTTP and database layers.
package foundation

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
	ghttp "github.com/charledeon77/gostack-framework/framework/http"
)

// Kernel acts as the heart of the framework. It holds the application's
// configuration and coordinates the startup process.
// It bridges the gap between the database infrastructure and the routing engine.
type Kernel struct {
	// DB provides access to the database layer throughout the application lifecycle.
	DB contract.Database

	// Router manages incoming HTTP requests and directs them to the correct logic.
	Router *ghttp.Router

	// onShutdown holds optional cleanup callbacks registered via OnShutdown().
	onShutdown []func()
}

// NewKernel creates a new instance of the application kernel.
// By injecting the database and router here, we ensure that the kernel
// remains decoupled from specific implementations.
//
// Parameters:
//   - db: A concrete database adapter implementation that satisfies the contract.Database interface.
//   - router: The configured HTTP router instance.
func NewKernel(db contract.Database, router *ghttp.Router) *Kernel {
	return &Kernel{
		DB:     db,
		Router: router,
	}
}

// OnShutdown registers a cleanup callback to be invoked during graceful shutdown.
// Use this to close NoSQL clients (Mongo, Neo4j, Cassandra) or any other resources.
//
// Example:
//
//	kernel.OnShutdown(func() {
//	    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	    defer cancel()
//	    gostack.Mongo.Disconnect(ctx)
//	})
func (k *Kernel) OnShutdown(fn func()) {
	k.onShutdown = append(k.onShutdown, fn)
}

// shutdown executes all registered cleanup callbacks.
func (k *Kernel) shutdown() {
	for _, fn := range k.onShutdown {
		fn()
	}
	if k.DB != nil {
		_ = k.DB.Close()
	}
}

// Run starts the HTTP server with graceful shutdown support.
// It listens for OS signals (SIGINT, SIGTERM) and performs a clean drain of
// in-flight requests within a 10-second window before stopping.
//
// Parameters:
//   - addr: The network address socket (e.g., ":8080") to listen and serve traffic on.
//
// Returns:
//   - An error if the network socket cannot be claimed, bound, or handled safely.
func (k *Kernel) Run(addr string) error {
	fmt.Printf("[GoStack Core] Server runtime engine initializing on network terminal address %s...\n", addr)

	viewEngine := ghttp.NewTempose()
	runtimeEngine := ghttp.NewEngine(k.Router, viewEngine)

	srv := &http.Server{
		Addr:    addr,
		Handler: runtimeEngine,
	}

	// Start serving in a goroutine so we can wait for signals below.
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Block until OS sends SIGINT or SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case <-quit:
		fmt.Println("\n[GoStack Core] Shutdown signal received. Draining in-flight requests (10s timeout)...")
	}

	// Give active requests up to 10 seconds to finish.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("[GoStack Core] Forced shutdown after timeout: %v\n", err)
	}

	k.shutdown()
	fmt.Println("[GoStack Core] Server shut down cleanly.")
	return nil
}