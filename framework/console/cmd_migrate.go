/*
Purpose:
This file implements the `migrate` and `migrate:rollback` CLI commands.

Philosophy:
Database updates should be simple, single-command operations. By exposing these
routines to the CLI, developers can update or revert database schemas quickly.

Architecture:
Implements console.Command. Resolves the database adapter from the global gostack.DB instance.

Implementation:
- MigrateCommand: triggers database.Migrator.Run() to run all pending migrations.
- RollbackCommand: triggers database.Migrator.Rollback() to revert the latest migration.
*/
package console

import (
	"fmt"
	"github.com/charledeon77/gostack-framework"
	"github.com/charledeon77/gostack-framework/framework/database"
)

// MigrateCommand implements the console.Command interface for the "migrate" CLI action.
type MigrateCommand struct{}

// Name returns the CLI trigger string for this command.
func (c *MigrateCommand) Name() string { return "migrate" }

// Description returns the human-readable help text shown in the CLI command listing.
func (c *MigrateCommand) Description() string { return "Run all pending database migrations" }

// Execute runs all pending migrations against the active database connection.
func (c *MigrateCommand) Execute(args []string) error {
	if gostack.DB == nil {
		return fmt.Errorf("migration failed: database connection is uninitialized")
	}
	fmt.Println("[GoStack CLI] Running database migrations...")
	migrator := database.NewMigrator(gostack.DB)
	if err := migrator.Run(); err != nil {
		return fmt.Errorf("migration run failed: %w", err)
	}
	fmt.Println("[GoStack CLI] Migrations completed successfully.")
	return nil
}

// RollbackCommand implements the console.Command interface for the "migrate:rollback" CLI action.
type RollbackCommand struct{}

// Name returns the CLI trigger string for this command.
func (c *RollbackCommand) Name() string { return "migrate:rollback" }

// Description returns the human-readable help text shown in the CLI command listing.
func (c *RollbackCommand) Description() string { return "Roll back the latest database migration batch" }

// Execute runs the rollback operation against the active database connection.
func (c *RollbackCommand) Execute(args []string) error {
	if gostack.DB == nil {
		return fmt.Errorf("rollback failed: database connection is uninitialized")
	}
	fmt.Println("[GoStack CLI] Rolling back the latest database migration batch...")
	migrator := database.NewMigrator(gostack.DB)
	if err := migrator.Rollback(); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}
	fmt.Println("[GoStack CLI] Rollback completed successfully.")
	return nil
}

// SeedCommand implements the console.Command interface for the "db:seed" CLI action.
type SeedCommand struct{}

// Name returns the CLI trigger string for this command.
func (c *SeedCommand) Name() string { return "db:seed" }

// Description returns the human-readable help text shown in the CLI command listing.
func (c *SeedCommand) Description() string { return "Seed the database with records (Usage: db:seed [SeederName])" }

// Execute runs the seeding operation.
func (c *SeedCommand) Execute(args []string) error {
	if gostack.DB == nil {
		return fmt.Errorf("seeding failed: database connection is uninitialized")
	}

	if len(args) > 0 {
		seederName := args[0]
		fmt.Printf("[GoStack CLI] Running database seeder: %s...\n", seederName)
		if err := database.RunSeeder(seederName); err != nil {
			return fmt.Errorf("seeder failed: %w", err)
		}
	} else {
		fmt.Println("[GoStack CLI] Running all database seeders...")
		if err := database.RunAllSeeders(); err != nil {
			return fmt.Errorf("seeding failed: %w", err)
		}
	}

	fmt.Println("[GoStack CLI] Seeding completed successfully.")
	return nil
}

