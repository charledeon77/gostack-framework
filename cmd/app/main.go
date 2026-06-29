// Package main serves as the primary operational entry point for the GoStack framework application.
// It is strictly responsible for manual dependency graph construction, coordinating the lifecycle 
// of storage adapters, service registries, routing infrastructures, and presentation controllers, 
// before finally booting up the centralized framework Kernel.
package main

import (
	"fmt"
	"github.com/charledeon77/gostack/framework/contract"
	"github.com/charledeon77/gostack/framework/database"
	"github.com/charledeon77/gostack/framework/foundation"
	"github.com/charledeon77/gostack/framework/http"
	"github.com/charledeon77/gostack/internal/controller"
	"log"
)

// main coordinates and orchestrates the critical startup sequence of the GoStack application.
func main() {
	// 1. Initialize the View Engine (Tempose).
	temposeEngine := http.NewTempose()

	// Register all compiled component views, styles, and scripts.
	RegisterComponents(temposeEngine)

	// 2. Initialize the Infrastructure Storage Layer (Database Adapter).
	dbDriver := foundation.Get("DB_DRIVER", "mysql")
	dbDSN := foundation.Get("DB_DSN", "")
	if dbDSN == "" {
		// Provide a default string to avoid crashing during dry builds
		dbDSN = "root:password@tcp(localhost:3306)/gostack"
	}
	db, err := database.NewSQLAdapter(dbDriver, dbDSN)
	if err != nil {
		log.Printf("[GoStack App Warning] Database connection pool setup returned: %v\n", err)
	}

	// 3. Initialize the Service Container and inject foundational dependencies.
	container := foundation.NewContainer()
	if db != nil {
		container.BindSingleton("db", func(c *foundation.Container) any {
			return db
		})
	}
	container.BindSingleton("tempose", func(c *foundation.Container) any {
		return temposeEngine
	})

	// 4. Initialize the HTTP Routing Infrastructure.
	router := http.NewRouter()

	// 5. Instantiate the Home Presentation Controller.
	var dbInstance contract.Database
	if db != nil {
		rawDB, err := container.Resolve("db")
		if err == nil {
			dbInstance = rawDB.(contract.Database)
		}
	}
	home := controller.NewHomeController(dbInstance)

	// 6. Register application endpoints.
	router.Get("/users", home.Users, http.Logger)

	// 7. Orchestrate Framework Orchestration & Boot up the Application Kernel.
	kernel := foundation.NewKernel(dbInstance, router)
	port := ":" + foundation.Get("APP_PORT", "8080")
	fmt.Printf("GoStack fullstack engine starting up securely on port %s...\n", port)

	if err := kernel.Run(port); err != nil {
		log.Fatalf("Critical Error: Core framework server crashed or failed to bind to socket address: %v", err)
	}
}