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
	"context"
	"database/sql"
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"time"
)

// SQLAdapter is the concrete implementation of the contract.Database interface.
type SQLAdapter struct {
	db     *sql.DB
	driver string
}

// SetMaxOpenConns sets the maximum number of open connections to the database.
func (a *SQLAdapter) SetMaxOpenConns(n int) {
	if a != nil && a.db != nil {
		a.db.SetMaxOpenConns(n)
	}
}

// SetMaxIdleConns sets the maximum number of connections in the idle connection pool.
func (a *SQLAdapter) SetMaxIdleConns(n int) {
	if a != nil && a.db != nil {
		a.db.SetMaxIdleConns(n)
	}
}

// SetConnMaxLifetime sets the maximum amount of time a connection may be reused.
func (a *SQLAdapter) SetConnMaxLifetime(d time.Duration) {
	if a != nil && a.db != nil {
		a.db.SetConnMaxLifetime(d)
	}
}

// SetConnMaxIdleTime sets the maximum amount of time a connection may be idle.
func (a *SQLAdapter) SetConnMaxIdleTime(d time.Duration) {
	if a != nil && a.db != nil {
		a.db.SetConnMaxIdleTime(d)
	}
}

// NewSQLAdapter acts as a constructor for the SQLAdapter, configuring production-grade
// connection pool safeguards (pool size, idle timeouts, and lifetime recycling).
func NewSQLAdapter(driver, dsn string) (*SQLAdapter, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Production connection pool defaults matching cloud environments (PostgreSQL/MySQL/SQLite)
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	return &SQLAdapter{db: db, driver: driver}, nil
}

// Connect verifies that the database connection is alive by performing a Ping.
// F-220: use PingContext with a 10 s timeout so a dead server doesn't block forever.
func (a *SQLAdapter) Connect() error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database connection is uninitialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	return nil
}

// QueryContext executes the provided SQL query with a caller-supplied context.
// F-219: prefer this method when a request context is available.
func (a *SQLAdapter) QueryContext(ctx context.Context, sqlString string, args ...any) (any, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database connection is uninitialized")
	}
	rows, err := a.db.QueryContext(ctx, sqlString, args...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	return rows, nil
}

// Query executes the provided SQL query with a default 30-second timeout context.
// F-219: bounds unbounded queries while preserving rows reading capability.
func (a *SQLAdapter) Query(sqlString string, args ...any) (any, error) {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	return a.QueryContext(ctx, sqlString, args...)
}

// ExecContext executes a SQL statement that returns no rows with a caller-supplied context.
// F-219: prefer this method when a request context is available.
func (a *SQLAdapter) ExecContext(ctx context.Context, sqlString string, args ...any) error {
	if a == nil || a.db == nil {
		return fmt.Errorf("database connection is uninitialized")
	}
	_, err := a.db.ExecContext(ctx, sqlString, args...)
	if err != nil {
		return fmt.Errorf("exec statement failed: %w", err)
	}
	return nil
}

// Exec executes a SQL statement that returns no rows (DDL, INSERT, UPDATE, DELETE).
// F-219: wraps ExecContext so callers that have no request context still get a timeout.
func (a *SQLAdapter) Exec(sqlString string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return a.ExecContext(ctx, sqlString, args...)
}

// ExecResult executes a SQL statement and returns the sql.Result for retrieving LastInsertId (PAR-13).
// This is used internally by InsertGetId and is not part of the contract.Database interface.
func (a *SQLAdapter) ExecResult(sqlString string, args ...any) (sql.Result, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("database connection is uninitialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := a.db.ExecContext(ctx, sqlString, args...)
	if err != nil {
		return nil, fmt.Errorf("exec statement failed: %w", err)
	}
	return result, nil
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

// ExecContext executes a parameterized SQL statement inside the transaction with a context deadline.
func (t *SQLTx) ExecContext(ctx context.Context, sqlString string, args ...any) error {
	_, err := t.tx.ExecContext(ctx, sqlString, args...)
	if err != nil {
		return fmt.Errorf("transaction exec failed: %w", err)
	}
	return nil
}

// Exec executes a parameterized SQL statement inside the transaction with a default 30-second timeout.
func (t *SQLTx) Exec(sqlString string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return t.ExecContext(ctx, sqlString, args...)
}

// QueryContext executes a parameterized SQL query inside the transaction with a context deadline.
func (t *SQLTx) QueryContext(ctx context.Context, sqlString string, args ...any) (any, error) {
	rows, err := t.tx.QueryContext(ctx, sqlString, args...)
	if err != nil {
		return nil, fmt.Errorf("transaction query failed: %w", err)
	}
	return rows, nil
}

// Query executes a parameterized SQL query inside the transaction with a default 30-second timeout.
func (t *SQLTx) Query(sqlString string, args ...any) (any, error) {
	ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
	return t.QueryContext(ctx, sqlString, args...)
}

// Commit finalizes the transaction.
func (t *SQLTx) Commit() error {
	return t.tx.Commit()
}

// Rollback aborts the transaction.
func (t *SQLTx) Rollback() error {
	return t.tx.Rollback()
}
