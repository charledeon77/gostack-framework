/*
Purpose:
This file is the project-local command-line entrypoint for the GoStack framework.
It bootstraps configuration parameters, registers core CLI commands (migrations,
rollbacks, generators), and executes commands based on terminal inputs.

Philosophy:
We believe local command execution (`go run cmd/gostack/main.go`) is superior to
global binaries in statically compiled languages. It guarantees that code generators,
migration scripts, and model schemas are compiled with the active project's exact dependency
versions and package registrations, keeping command environments safe and version-aligned.

Architecture:
Acts as the CLI entrypoint (main package). It binds and runs the `console.Kernel`
with our core commands, bridging OS terminal inputs (`os.Args`) directly to CLI runners.

Choice:
We chose this local entrypoint design over external CLI wrappers to support compile-time
checking of custom project migrations and registration blocks before command runner execution.

Implementation:
- main(): Loads database environment configs, executes database connection handshake (optional),
  registers Migrate, Rollback, MakeMigration, and MakeController commands, and runs the kernel.
*/
package main

import (
	"fmt"
	"github.com/charledeon77/gostack"
	"github.com/charledeon77/gostack/framework/console"
	"github.com/charledeon77/gostack/framework/foundation"
	"log"
	"os"
)

func main() {
	// 1. Initialize configuration values from local environment variables
	driver := foundation.Get("DB_DRIVER", "mysql")
	dsn := foundation.Get("DB_DSN", "")

	// 2. Connect database if credentials are provided.
	// Note: Generator commands (make:migration, make:controller) do not require a database connection,
	// so we log a warning instead of fatalling if connection configuration is missing or invalid.
	if dsn != "" {
		if err := gostack.InitDatabase(driver, dsn); err != nil {
			log.Printf("[GoStack CLI Warning] Database connection failed: %v\n", err)
		}
	}

	// 3. Construct the CLI console kernel and register core actions
	kernel := console.NewKernel()
	kernel.Register(&console.NewCommand{})
	kernel.Register(&console.ServeCommand{})
	kernel.Register(&console.PreviewCommand{})
	kernel.Register(&console.MigrateCommand{})
	kernel.Register(&console.RollbackCommand{})
	kernel.Register(&console.MakeMigrationCommand{})
	kernel.Register(&console.MakeControllerCommand{})
	kernel.Register(&console.MakeAuthCommand{})
	kernel.Register(&console.MakeModelCommand{})
	kernel.Register(&console.MakeRequestCommand{})
	kernel.Register(&console.MakeMiddlewareCommand{})
	kernel.Register(&console.MakeMailCommand{})
	kernel.Register(&console.MakeSeederCommand{})
	kernel.Register(&console.SeedCommand{})
	kernel.Register(&console.AddComponentCommand{})


	// 4. Bind kernel runner to CLI terminal inputs
	if err := kernel.Run(os.Args[1:]); err != nil {
		fmt.Printf("[GoStack CLI Error] Command failed: %v\n", err)
		os.Exit(1)
	}
}
