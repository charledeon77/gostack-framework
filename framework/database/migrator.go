/*
Purpose:
This file implements the Traveller database migration registry and runner.

Philosophy:
Migrations are self-registering Go files. Each file calls database.Register() inside its
init() function, which auto-executes when the package is imported. No manual wiring needed.
The Migrator tracks completed migrations in a gostack_migrations table and runs only pending
migrations, each wrapped in a database transaction.

Architecture:
Part of the database package. The registry is a package-level slice. Migrator coordinates
transaction-wrapped Up/Down execution and version tracking.

Choice:
Consolidated migrate/migrator.go into database/migrator.go to keep all database operations
in a single, cohesive package.

Implementation:
- Migration: struct holding version, Up, and Down functions.
- Register: self-registration entry point called from migration file init() functions.
- Migrator: coordinates Run() and Rollback() against the DB.
*/
package database

import (
	"database/sql"
	"fmt"
	"github.com/charledeon77/gostack/framework/contract"
	"sort"
)

// Migration represents a single versioned schema change with Up and Down functions.
type Migration struct {
	// Version is a unique integer identifier (e.g. 20260612001, sortable by creation time).
	Version int64

	// Up defines the forward schema change using the GoStack Schema Builder.
	Up func(s *Builder) error

	// Down defines the rollback schema change to reverse the Up operation.
	Down func(s *Builder) error
}

// registry is the global, package-level list of all self-registered migrations.
var registry []Migration

// Register adds a new migration to the global registry.
// Designed to be called from the init() function of each migration file.
func Register(version int64, up, down func(s *Builder) error) {
	registry = append(registry, Migration{
		Version: version,
		Up:      up,
		Down:    down,
	})
}

// Migrator coordinates running and rolling back database migrations.
type Migrator struct {
	db contract.Database
}

// NewMigrator constructs a Migrator bound to an active database connection.
func NewMigrator(db contract.Database) *Migrator {
	return &Migrator{db: db}
}

// Run executes all pending migrations in ascending version order.
func (m *Migrator) Run() error {
	if err := m.ensureTrackingTable(); err != nil {
		return err
	}

	completed, err := m.completedVersions()
	if err != nil {
		return err
	}

	sorted := append([]Migration(nil), registry...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})

	for _, mig := range sorted {
		if completed[mig.Version] {
			continue
		}
		if err := m.runOne(mig); err != nil {
			return fmt.Errorf("[Traveller] Migration %d failed: %w", mig.Version, err)
		}
		fmt.Printf("[Traveller] Migrated: %d\n", mig.Version)
	}

	return nil
}

// Rollback reverses the last single migration in descending version order.
func (m *Migrator) Rollback() error {
	if err := m.ensureTrackingTable(); err != nil {
		return err
	}

	completed, err := m.completedVersions()
	if err != nil {
		return err
	}

	sorted := append([]Migration(nil), registry...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version > sorted[j].Version
	})

	for _, mig := range sorted {
		if !completed[mig.Version] {
			continue
		}
		if err := m.rollbackOne(mig); err != nil {
			return fmt.Errorf("[Traveller] Rollback of %d failed: %w", mig.Version, err)
		}
		fmt.Printf("[Traveller] Rolled back: %d\n", mig.Version)
		return nil
	}

	fmt.Println("[Traveller] Nothing to roll back.")
	return nil
}

func (m *Migrator) runOne(mig Migration) error {
	tx, err := m.db.BeginTx()
	if err != nil {
		return err
	}

	builder := NewBuilder(tx, m.db.Driver())

	if err := mig.Up(builder); err != nil {
		_ = tx.Rollback()
		return err
	}

	query := "INSERT INTO gostack_migrations (version) VALUES (?)"
	drv := m.db.Driver()
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		query = "INSERT INTO gostack_migrations (version) VALUES ($1)"
	}

	if err := tx.Exec(query, mig.Version); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (m *Migrator) rollbackOne(mig Migration) error {
	tx, err := m.db.BeginTx()
	if err != nil {
		return err
	}

	builder := NewBuilder(tx, m.db.Driver())

	if err := mig.Down(builder); err != nil {
		_ = tx.Rollback()
		return err
	}

	query := "DELETE FROM gostack_migrations WHERE version = ?"
	drv := m.db.Driver()
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		query = "DELETE FROM gostack_migrations WHERE version = $1"
	}

	if err := tx.Exec(query, mig.Version); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (m *Migrator) ensureTrackingTable() error {
	driver := m.db.Driver()
	var query string
	switch driver {
	case "postgres", "cockroach", "cockroachdb":
		query = `CREATE TABLE IF NOT EXISTS gostack_migrations (
			id SERIAL PRIMARY KEY,
			version BIGINT NOT NULL UNIQUE,
			migrated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	case "sqlite", "sqlite3":
		query = `CREATE TABLE IF NOT EXISTS gostack_migrations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			version BIGINT NOT NULL UNIQUE,
			migrated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	default:
		query = `CREATE TABLE IF NOT EXISTS gostack_migrations (
			id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			version BIGINT NOT NULL UNIQUE,
			migrated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	}
	return m.db.Exec(query)
}

func (m *Migrator) completedVersions() (map[int64]bool, error) {
	result, err := m.db.Query("SELECT version FROM gostack_migrations")
	if err != nil {
		return nil, err
	}

	rows, ok := result.(*sql.Rows)
	if !ok {
		return map[int64]bool{}, nil
	}
	defer rows.Close()

	completed := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		completed[version] = true
	}
	return completed, nil
}
