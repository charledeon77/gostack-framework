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
	"github.com/charledeon77/gostack-framework"
	"github.com/charledeon77/gostack-framework/framework/console"
	"github.com/charledeon77/gostack-framework/framework/foundation"
	"log"
	"os"
)

func main() {
	// 1. Initialize configuration values from local environment variables
	driver := foundation.Get("DB_DRIVER", "mysql")
	dsn := foundation.Get("DB_DSN", "")

	// 2. Connect database if credentials are provided.
	var dbErr error
	if dsn != "" {
		dbErr = gostack.InitDatabase(driver, dsn)
	}

	// 3. Construct the CLI console kernel and register core actions
	kernel := console.NewKernel()
	for _, cmd := range []console.Command{
		&console.NewCommand{},
		&console.ServeCommand{},
		&console.DevCommand{},
		&console.LangSearchCommand{},
		&console.MigrateCommand{},
		&console.RollbackCommand{},
		&console.MakeMigrationCommand{},
		&console.MakeControllerCommand{},
		&console.MakeAuthCommand{},
		&console.MakeModelCommand{},
		&console.MakeRequestCommand{},
		&console.MakeMiddlewareCommand{},
		&console.MakeMailCommand{},
		&console.MakeSeederCommand{},
		&console.SeedCommand{},
		&console.AddComponentCommand{},
		&console.MakeEventCommand{},
		&console.MakeJobCommand{},
		&console.MakePolicyCommand{},
		&console.MakeProviderCommand{},
	} {
		if err := kernel.Register(cmd); err != nil {
			fmt.Printf("[GoStack CLI Error] Failed to register command: %v\n", err)
			os.Exit(1)
		}
	}

	// D21: Commands that declare a database requirement must fail closed.
	// Querying the command's own DatabaseDependent capability eliminates string drift.
	if len(os.Args) > 1 {
		subcommand := os.Args[1]
		if cmd, exists := kernel.Get(subcommand); exists {
			if req, ok := cmd.(console.DatabaseDependent); ok && req.RequiresDatabase() {
				if dsn == "" {
					fmt.Printf("[GoStack CLI Error] Command %q requires a database connection, but DB_DSN is empty.\n", subcommand)
					os.Exit(1)
				}
				if dbErr != nil {
					fmt.Printf("[GoStack CLI Error] Command %q requires a database connection, but connection failed: %v\n", subcommand, dbErr)
					os.Exit(1)
				}
			}
		}
	}
	if dbErr != nil {
		log.Printf("[GoStack CLI Warning] Database connection failed: %v\n", dbErr)
	}


	// 4. Bind kernel runner to CLI terminal inputs
	if err := kernel.Run(os.Args[1:]); err != nil {
		fmt.Printf("[GoStack CLI Error] Command failed: %v\n", err)
		os.Exit(1)
	}
}
