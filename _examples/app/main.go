// Package main serves as the primary operational entry point for the GoStack framework application.
// It is strictly responsible for manual dependency graph construction, coordinating the lifecycle 
// of storage adapters, service registries, routing infrastructures, and presentation controllers, 
// before finally booting up the centralized framework Kernel.
package main

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"github.com/charledeon77/gostack-framework/framework/database"
	"github.com/charledeon77/gostack-framework/framework/foundation"
	"github.com/charledeon77/gostack-framework/framework/http"
	"github.com/charledeon77/gostack-framework/internal/controller"
	"log"
)

// main coordinates and orchestrates the critical startup sequence of the GoStack application.
// 
// The system initialization sequence adheres to a strict, logical linear pipeline to ensure total 
// architectural integrity and system stability prior to exposing the application server to external traffic:
// 1. Presentation Asset Engine Initialization (Tempose View Registry)
// 2. Persistent Infrastructure Connectivity (Database Driver Pool)
// 3. Service Registration & Control Layer (IoC Dependency Injection Container)
// 4. HTTP Layer Configuration (Multiplexer & Route Mapping via Path A)
// 5. Controller Association & Composition Lifecycle
// 6. Orchestrated Application Framework Bootup (Kernel Execution)
func main() {
	// 1. Initialize the View Engine (Tempose).
	// We instantiate Tempose as a pure rendering engine, ensuring it remains isolated 
	// from network-level protocols and completely dedicated to high-performance view execution.
	temposeEngine := http.NewTempose()

	// Register all compiled component views, styles, and scripts.
	RegisterComponents(temposeEngine)

	// 2. Initialize the Infrastructure Storage Layer (Database Adapter).
	// Credentials and driver are loaded from environment variables, keeping secrets
	// out of source code. Set DB_DRIVER ("mysql", "postgres") and DB_DSN in your .env file.
	dbDriver := foundation.Get("DB_DRIVER", "mysql")
	dbDSN := foundation.Get("DB_DSN", "")
	if dbDSN == "" {
		log.Fatal("Critical: DB_DSN environment variable is not set. Please configure your database connection.")
	}
	db, err := database.NewSQLAdapter(dbDriver, dbDSN)
	if err != nil {
		log.Fatalf("Critical: System boot failed. Could not establish core database connection pool: %v", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	// We bind our active view engine and database adapter to the central IoC container.
	container := foundation.NewContainer()
	container.BindSingleton("db", func(c *foundation.Container) any {
		return db
	})
	container.BindSingleton("tempose", func(c *foundation.Container) any {
		return temposeEngine
	})

	// 4. Initialize the HTTP Routing Infrastructure.
	// This creates our custom multiplexer configured for Path A, allowing handlers to receive 
	// our unified *http.Context wrapper natively.
	router := http.NewRouter()

	// 5. Instantiate the Home Presentation Controller.
	// We resolve the required persistent storage dependency directly from the safe container, 
	// feeding the generic contract.Database interface straight into the controller constructor.
	rawDB, err := container.Resolve("db")
	if err != nil {
		log.Fatalf("Critical: Failed to resolve DB dependency from container: %v", err)
	}
	dbInstance := rawDB.(contract.Database)
	home := controller.NewHomeController(dbInstance)

	// 6. Register application endpoints via our clean, native Path A API.
	//
	// Architectural Milestone:
	// We are passing our native home.Users handler, followed directly by our refactored 
	// ExampleLogger middleware hook. The router automatically compiles them into a unified execution chain.
	router.Get("/users", home.Users, http.Logger)

	// 7. Orchestrate Framework Orchestration & Boot up the Application Kernel.
	// Port is configurable via the APP_PORT environment variable, defaulting to :8080.
	kernel := foundation.NewKernel(dbInstance, router)
	port := ":" + foundation.Get("APP_PORT", "8080")
	fmt.Printf("GoStack fullstack engine starting up securely on port %s...\n", port)

	if err := kernel.Run(port); err != nil {
		log.Fatalf("Critical Error: Core framework server crashed or failed to bind to socket address: %v", err)
	}
}