/*
Purpose:
This file implements the Citadel pluggable database driver registry.

Philosophy:
GoStack follows the principle of "modular self-containment" — the core framework
binary should not carry compile-time dependencies on any specific database driver
(MySQL, PostgreSQL, SQLite, etc.) unless that driver is explicitly imported by
the application developer's own code.

This registry achieves that by reversing the dependency direction:
Instead of GoStack importing drivers, drivers register themselves into GoStack
via their own init() functions, following the same pattern as Go's standard
"database/sql" package driver registration model.

LIFECYCLE:
1. The developer imports their chosen driver package (e.g. a GoStack MySQL adapter).
2. That package's init() calls foundation.Register("mysql", constructorFunc).
3. During application boot, GoStack calls foundation.GetDriver("mysql") to retrieve
   the constructor and build the live database connection.

This guarantees that only actively-imported drivers are compiled into the binary,
keeping the executable lean and free of unused transitive dependencies.
*/
package foundation

import (
	"errors"
	"sync"
)

// DriverFunc defines the universal constructor signature for all database driver adapters.
//
// DESIGN RATIONALE:
// By standardizing on a single function signature (DSN string in, interface + error out),
// the registry can invoke any driver's constructor without knowing its concrete type.
// The returned interface{} is subsequently type-asserted to contract.Database by the
// framework bootstrap layer.
//
// Parameters:
//   - dsn: The Data Source Name string containing all connection credentials and parameters.
//
// Returns:
//   - An initialized database connection object satisfying contract.Database, or an error.
type DriverFunc func(dsn string) (any, error)

var (
	// mu guards concurrent read/write access to the drivers map across goroutines.
	mu sync.RWMutex

	// drivers is the central registry mapping driver name strings to their constructor functions.
	// It is populated exclusively at application init time via Register().
	drivers = make(map[string]DriverFunc)
)

// Register adds a named driver constructor to the central registry.
//
// PANIC BEHAVIOR (INTENTIONAL):
// This function panics rather than returning an error for two specific invalid states:
//   - Registering a nil constructor: A nil DriverFunc would cause a silent crash at
//     connection time. Panicking at registration time makes the error immediately visible.
//   - Registering the same driver name twice: Double-registration indicates a package
//     import conflict or accidental duplicate init() calls. Panicking here prevents
//     silent overwrite of a valid driver with an incorrect one.
//
// This mirrors the design of Go's standard "database/sql".Register() for the same reasons.
//
// Parameters:
//   - name: The unique driver identifier string (e.g. "mysql", "postgres").
//   - f: The DriverFunc constructor to associate with this driver name.
func Register(name string, f DriverFunc) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		panic("foundation: Register driver is nil")
	}
	if _, dup := drivers[name]; dup {
		panic("foundation: Register called twice for driver " + name)
	}
	drivers[name] = f
}

// GetDriver retrieves a registered driver constructor by its name identifier.
//
// Parameters:
//   - name: The driver name string to look up (e.g. "mysql", "postgres").
//
// Returns:
//   - The registered DriverFunc constructor, or an error if no driver with that name exists.
//   - An error if the driver has not been registered (most likely the driver's package was not imported).
func GetDriver(name string) (DriverFunc, error) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, errors.New("foundation: driver " + name + " not found")
	}
	return f, nil
}
