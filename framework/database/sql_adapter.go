/*
Purpose:
This file implements standard SQL-based database operations, ensuring that the framework
can communicate with relational databases like MySQL, PostgreSQL, or SQLite.

Philosophy:
We believe relational database interactions should be clean, driver-agnostic, and secure.
By wrapping standard library sql.DB pools, we provide a unified execution surface.

Architecture:
Part of the database package, providing the driver implementation conforming to contract.Database.

Choice:
Consolidated adapter, query, schema, and migrate into the database package to simplify database
operations and remove circular dependencies.

Implementation:
- SQLAdapter: struct wrapping sql.DB and tracking the driver string name.
- NewSQLAdapter: constructor establishing standard database connection pools.
- SQLTx: struct wrapping standard sql.Tx transactions.
*/
package database

import (
	"database/sql"
	"fmt"
	"github.com/charledeon77/gostack/framework/contract"
)

// SQLAdapter is the concrete implementation of the contract.Database interface.
type SQLAdapter struct {
	db     *sql.DB
	driver string
}

// NewSQLAdapter acts as a constructor for the SQLAdapter.
func NewSQLAdapter(driver, dsn string) (*SQLAdapter, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}
	return &SQLAdapter{db: db, driver: driver}, nil
}

// Connect verifies that the database connection is alive by performing a Ping.
func (a *SQLAdapter) Connect() error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database connection is uninitialized")
	}
	if err := a.db.Ping(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	return nil
}

// Query executes the provided SQL query with parameters and returns the result set.
func (a *SQLAdapter) Query(sqlString string, args ...any) (any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database connection is uninitialized")
	}
	rows, err := a.db.Query(sqlString, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	return rows, nil
}

// Exec executes a SQL statement that returns no rows (DDL, INSERT, UPDATE, DELETE).
func (a *SQLAdapter) Exec(sqlString string, args ...any) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database connection is uninitialized")
	}
	_, err := a.db.Exec(sqlString, args...)
	if err != nil {
		return fmt.Errorf("exec statement failed: %w", err)
	}
	return nil
}

// BeginTx opens a new database transaction and returns it wrapped in the contract.Tx interface.
func (a *SQLAdapter) BeginTx() (contract.Tx, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database connection is uninitialized")
	}
	tx, err := a.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	return &SQLTx{tx: tx}, nil
}

// Driver returns the database driver identifier name.
func (a *SQLAdapter) Driver() string {
	if a == nil {
		return ""
	}
	return a.driver
}

// Close releases the underlying SQL database connection pool.
func (a *SQLAdapter) Close() error {
	if a == nil || a.db == nil {
		return nil
	}
	return a.db.Close()
}


// SQLTx wraps a standard *sql.Tx to implement the contract.Tx interface.
type SQLTx struct {
	tx *sql.Tx
}

// Exec executes a parameterized SQL statement inside the transaction.
func (t *SQLTx) Exec(sqlString string, args ...any) error {
	_, err := t.tx.Exec(sqlString, args...)
	if err != nil {
		return fmt.Errorf("transaction exec failed: %w", err)
	}
	return nil
}

// Query executes a parameterized SQL query inside the transaction.
func (t *SQLTx) Query(sqlString string, args ...any) (any, error) {
	rows, err := t.tx.Query(sqlString, args...)
	if err != nil {
		return nil, fmt.Errorf("transaction query failed: %w", err)
	}
	return rows, nil
}

// Commit finalizes the transaction.
func (t *SQLTx) Commit() error {
	return t.tx.Commit()
}

// Rollback aborts the transaction.
func (t *SQLTx) Rollback() error {
	return t.tx.Rollback()
}
