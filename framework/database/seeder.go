/*
Purpose:
This file implements the GoStack database seeding registry.

Philosophy:
Seeding should be modular and easy to execute. By registering seeders via self-registering
init() calls, we keep database population decoupled from the main CLI execution engine.

Architecture:
Part of the database package. Seeders implement a simple Seeder interface and are invoked
by the console db:seed command.
*/
package database

import "fmt"

// Seeder defines the contract for populating database tables with seed data.
type Seeder interface {
	Run() error
}

// seederRegistry holds all registered seeder implementations by name.
var seederRegistry = make(map[string]Seeder)

// RegisterSeeder adds a seeder to the global registry.
// Typically called from init() in generated seeder files.
func RegisterSeeder(name string, s Seeder) {
	seederRegistry[name] = s
}

// RunSeeder runs a single seeder by name.
func RunSeeder(name string) error {
	seeder, exists := seederRegistry[name]
	if !exists {
		return fmt.Errorf("seeder '%s' not found in registry", name)
	}
	return seeder.Run()
}

// RunAllSeeders runs all registered seeders. If "DatabaseSeeder" exists,
// it runs only DatabaseSeeder. Otherwise, it runs all registered seeders.
func RunAllSeeders() error {
	if ds, exists := seederRegistry["DatabaseSeeder"]; exists {
		fmt.Println("[GoStack CLI] Running DatabaseSeeder...")
		return ds.Run()
	}

	if len(seederRegistry) == 0 {
		fmt.Println("[GoStack CLI] No seeders registered.")
		return nil
	}

	for name, seeder := range seederRegistry {
		fmt.Printf("[GoStack CLI] Running seeder: %s...\n", name)
		if err := seeder.Run(); err != nil {
			return fmt.Errorf("seeder %s failed: %w", name, err)
		}
	}
	return nil
}
