/*
Purpose:
This file contains unit tests for the Migrator in the database package.
It validates migration discovery, ascending execution order, pending migration filters,
transaction boundaries (commit on success, rollback on error), and rollbacks.

Philosophy:
We validate database engines under real-world transactional semantics using a
standard-library-compliant virtual database driver in-memory — no external dependencies.

Architecture:
Tests manipulate the package-private `registry` slice directly, register a test-only
in-memory SQL driver (`gostack_migrate_mock`), wrap standard *sql.DB in our SQLAdapter,
and invoke Migrator.Run() and Migrator.Rollback().

Implementation:
- mockMigrateDriver: implements driver.Driver, driver.Conn, driver.Tx, driver.Stmt, and driver.Rows.
- TestMigratorRunAllPending: asserts migrations execute in chronological order and commit correctly.
- TestMigratorSkipCompleted: asserts that previously run migrations are skipped.
- TestMigratorRollbackOnFailure: asserts transaction rollback and execution halt on error.
- TestMigratorRollbackLatest: asserts that Rollback reverts only the single latest migration version.
*/
package database

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
)

// ============================================================================
// 1. MOCK DRIVER STATE & IMPLEMENTATION
// ============================================================================

var (
	mockTableCreated bool
	mockVersions     []int64
	mockExecHistory  []string
	mockTxCommitted  bool
	mockTxRolledBack bool
)

func resetMockState() {
	mockTableCreated = false
	mockVersions = nil
	mockExecHistory = nil
	mockTxCommitted = false
	mockTxRolledBack = false
}

type mockMigrateResult struct{}

func (r *mockMigrateResult) LastInsertId() (int64, error) { return 1, nil }
func (r *mockMigrateResult) RowsAffected() (int64, error) { return 1, nil }

type mockMigrateRows struct {
	versions []int64
	cursor   int
}

func (r *mockMigrateRows) Columns() []string { return []string{"version"} }
func (r *mockMigrateRows) Close() error      { return nil }

func (r *mockMigrateRows) Next(dest []driver.Value) error {
	if r.cursor >= len(r.versions) {
		return io.EOF
	}
	dest[0] = r.versions[r.cursor]
	r.cursor++
	return nil
}

type mockMigrateStmt struct{ query string }

func (s *mockMigrateStmt) NumInput() int { return -1 }
func (s *mockMigrateStmt) Close() error  { return nil }

func (s *mockMigrateStmt) Exec(args []driver.Value) (driver.Result, error) {
	mockExecHistory = append(mockExecHistory, s.query)
	if strings.Contains(s.query, "CREATE TABLE") {
		mockTableCreated = true
	} else if strings.Contains(s.query, "INSERT INTO") {
		if len(args) > 0 {
			if v, ok := args[0].(int64); ok {
				mockVersions = append(mockVersions, v)
			}
		}
	} else if strings.Contains(s.query, "DELETE FROM") {
		if len(args) > 0 {
			if v, ok := args[0].(int64); ok {
				var remaining []int64
				for _, ex := range mockVersions {
					if ex != v {
						remaining = append(remaining, ex)
					}
				}
				mockVersions = remaining
			}
		}
	}
	return &mockMigrateResult{}, nil
}

func (s *mockMigrateStmt) Query(args []driver.Value) (driver.Rows, error) {
	mockExecHistory = append(mockExecHistory, s.query)
	if strings.Contains(s.query, "SELECT version") {
		return &mockMigrateRows{versions: mockVersions}, nil
	}
	return &mockMigrateRows{}, nil
}

type mockMigrateConn struct{}

func (c *mockMigrateConn) Prepare(query string) (driver.Stmt, error) {
	return &mockMigrateStmt{query: query}, nil
}
func (c *mockMigrateConn) Close() error { return nil }
func (c *mockMigrateConn) Begin() (driver.Tx, error) {
	mockTxCommitted = false
	mockTxRolledBack = false
	return &mockMigrateTx{}, nil
}

type mockMigrateTx struct{}

func (tx *mockMigrateTx) Commit() error   { mockTxCommitted = true; return nil }
func (tx *mockMigrateTx) Rollback() error { mockTxRolledBack = true; return nil }

type mockMigrateDriver struct{}

func (d *mockMigrateDriver) Open(name string) (driver.Conn, error) {
	return &mockMigrateConn{}, nil
}

func init() {
	sql.Register("gostack_migrate_mock", &mockMigrateDriver{})
}

// newMigrateTestAdapter opens a SQLAdapter using the in-memory mock driver.
func newMigrateTestAdapter(t *testing.T) *SQLAdapter {
	t.Helper()
	a, err := NewSQLAdapter("gostack_migrate_mock", "")
	if err != nil {
		t.Fatalf("Failed to initialize SQL adapter: %v", err)
	}
	return a
}

// ============================================================================
// 2. MIGRATION RUNNER TESTS
// ============================================================================

func TestMigratorRunAllPending(t *testing.T) {
	resetMockState()
	registry = nil
	var executionOrder []int64

	Register(20260612002,
		func(s *Builder) error { executionOrder = append(executionOrder, 20260612002); return nil },
		func(s *Builder) error { return nil },
	)
	Register(20260612001,
		func(s *Builder) error { executionOrder = append(executionOrder, 20260612001); return nil },
		func(s *Builder) error { return nil },
	)

	migrator := NewMigrator(newMigrateTestAdapter(t))
	if err := migrator.Run(); err != nil {
		t.Fatalf("Migrator.Run failed: %v", err)
	}

	if len(executionOrder) != 2 {
		t.Fatalf("Expected 2 executed migrations, got: %d", len(executionOrder))
	}
	if executionOrder[0] != 20260612001 || executionOrder[1] != 20260612002 {
		t.Errorf("Incorrect execution order: %v", executionOrder)
	}
	if !mockTableCreated {
		t.Error("Expected tracking table to be created")
	}
	if len(mockVersions) != 2 || mockVersions[0] != 20260612001 || mockVersions[1] != 20260612002 {
		t.Errorf("Versions not tracked correctly: %v", mockVersions)
	}
	if !mockTxCommitted {
		t.Error("Expected transaction to commit successfully")
	}
}

func TestMigratorSkipCompleted(t *testing.T) {
	resetMockState()
	mockVersions = []int64{20260612001}
	registry = nil
	var executed []int64

	Register(20260612001,
		func(s *Builder) error { executed = append(executed, 20260612001); return nil },
		func(s *Builder) error { return nil },
	)
	Register(20260612002,
		func(s *Builder) error { executed = append(executed, 20260612002); return nil },
		func(s *Builder) error { return nil },
	)

	migrator := NewMigrator(newMigrateTestAdapter(t))
	if err := migrator.Run(); err != nil {
		t.Fatalf("Migrator.Run failed: %v", err)
	}

	if len(executed) != 1 || executed[0] != 20260612002 {
		t.Errorf("Expected only version 20260612002 to execute, got: %v", executed)
	}
}

func TestMigratorRollbackOnFailure(t *testing.T) {
	resetMockState()
	registry = nil
	var executed []int64

	Register(20260612001,
		func(s *Builder) error {
			executed = append(executed, 20260612001)
			return fmt.Errorf("simulate migration failure")
		},
		func(s *Builder) error { return nil },
	)

	migrator := NewMigrator(newMigrateTestAdapter(t))
	err := migrator.Run()
	if err == nil {
		t.Fatal("Expected error from failing migration, got nil")
	}
	if !strings.Contains(err.Error(), "simulate migration failure") {
		t.Errorf("Expected migration failure error, got: %v", err)
	}
	if !mockTxRolledBack {
		t.Error("Expected transaction to rollback on execution failure")
	}
	if len(mockVersions) != 0 {
		t.Errorf("Expected no versions to be registered, got: %v", mockVersions)
	}
}

func TestMigratorRollbackLatest(t *testing.T) {
	resetMockState()
	mockVersions = []int64{20260612001, 20260612002}
	registry = nil
	var rolledBack []int64

	Register(20260612001,
		func(s *Builder) error { return nil },
		func(s *Builder) error { rolledBack = append(rolledBack, 20260612001); return nil },
	)
	Register(20260612002,
		func(s *Builder) error { return nil },
		func(s *Builder) error { rolledBack = append(rolledBack, 20260612002); return nil },
	)

	migrator := NewMigrator(newMigrateTestAdapter(t))
	if err := migrator.Rollback(); err != nil {
		t.Fatalf("Migrator.Rollback failed: %v", err)
	}

	if len(rolledBack) != 1 || rolledBack[0] != 20260612002 {
		t.Errorf("Expected only 20260612002 to rollback, got: %v", rolledBack)
	}
	if len(mockVersions) != 1 || mockVersions[0] != 20260612001 {
		t.Errorf("Expected only version 20260612001 to remain, got: %v", mockVersions)
	}
	if !mockTxCommitted {
		t.Error("Expected rollback transaction to commit successfully")
	}
}
