/*
Purpose:
This file implements the Grapher Schema Builder and Table DDL compiler for the GoStack framework.
It provides a fluent, driver-aware API for defining and compiling CREATE TABLE and DROP TABLE statements.

Philosophy:
Developers should never write raw DDL SQL. All schema definitions are expressed through
type-safe Go method calls, allowing the framework to emit the correct SQL for each driver.

Architecture:
Part of the database package. The Builder wraps a contract.Tx and dispatches DDL
operations. The Table and Column types implement the fluent DSL for schema definition.

Choice:
Consolidated schema/builder.go and schema/table.go into a single database/schema.go file
to reduce package fragmentation and simplify imports.

Implementation:
- ColumnKind: enumeration of supported column types.
- Column: struct representing a single column definition with modifier chaining.
- Table: struct holding an ordered list of Column definitions for a single table.
- Builder: wraps contract.Tx to execute CREATE/DROP TABLE DDL atomically.
*/
package database

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"strings"
)

// ColumnKind identifies the semantic data category of a column definition.
type ColumnKind int

const (
	KindID        ColumnKind = iota // Auto-increment primary key
	KindString                      // VARCHAR(255)
	KindInteger                     // INT
	KindBoolean                     // TINYINT(1) / BOOLEAN
	KindText                        // TEXT
	KindTimestamp                   // DATETIME / TIMESTAMPTZ
)

// Column represents a single database column definition.
type Column struct {
	name       string
	kind       ColumnKind
	isNullable bool
	isUnique   bool
	hasDefault bool
	defaultVal any
}

// Nullable marks this column as allowing NULL values.
func (c *Column) Nullable() *Column { c.isNullable = true; return c }

// Unique adds a UNIQUE constraint to this column.
func (c *Column) Unique() *Column { c.isUnique = true; return c }

// Default sets a default value for this column.
func (c *Column) Default(val any) *Column { c.hasDefault = true; c.defaultVal = val; return c }

// Table holds the full ordered list of column definitions for a CREATE TABLE statement.
type Table struct {
	columns []*Column
	driver  string
}

// NewTable constructs a fresh Table definition bound to a specific driver.
func NewTable(driver string) *Table { return &Table{driver: driver} }

// ID appends a driver-aware auto-incrementing primary key column.
func (t *Table) ID() *Column {
	col := &Column{name: "id", kind: KindID}
	t.columns = append(t.columns, col)
	return col
}

// String appends a VARCHAR(255) column.
func (t *Table) String(name string) *Column {
	col := &Column{name: name, kind: KindString}
	t.columns = append(t.columns, col)
	return col
}

// Integer appends an INTEGER column.
func (t *Table) Integer(name string) *Column {
	col := &Column{name: name, kind: KindInteger}
	t.columns = append(t.columns, col)
	return col
}

// Boolean appends a BOOLEAN column.
func (t *Table) Boolean(name string) *Column {
	col := &Column{name: name, kind: KindBoolean}
	t.columns = append(t.columns, col)
	return col
}

// Text appends a TEXT column.
func (t *Table) Text(name string) *Column {
	col := &Column{name: name, kind: KindText}
	t.columns = append(t.columns, col)
	return col
}

// Timestamp appends a TIMESTAMP / DATETIME column.
func (t *Table) Timestamp(name string) *Column {
	col := &Column{name: name, kind: KindTimestamp}
	t.columns = append(t.columns, col)
	return col
}


// Timestamps appends the standard created_at and updated_at columns.
func (t *Table) Timestamps() {
	t.columns = append(t.columns,
		&Column{name: "created_at", kind: KindTimestamp},
		&Column{name: "updated_at", kind: KindTimestamp, isNullable: true},
	)
}

// Compile translates the full column list into a driver-aware CREATE TABLE SQL string.
func (t *Table) Compile(tableName string) string {
	var cols []string
	for _, col := range t.columns {
		cols = append(cols, t.compileColumn(col))
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n\t%s\n)", tableName, strings.Join(cols, ",\n\t"))
}

func (t *Table) compileColumn(col *Column) string {
	var sb strings.Builder
	sb.WriteString(col.name)
	sb.WriteString(" ")

	switch col.kind {
	case KindID:
		if t.driver == "postgres" || t.driver == "cockroach" || t.driver == "cockroachdb" {
			sb.WriteString("BIGSERIAL PRIMARY KEY")
		} else if t.driver == "sqlite" || t.driver == "sqlite3" {
			sb.WriteString("INTEGER PRIMARY KEY AUTOINCREMENT")
		} else {
			sb.WriteString("BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY")
		}
		return sb.String()
	case KindString:
		sb.WriteString("VARCHAR(255)")
	case KindInteger:
		sb.WriteString("INT")
	case KindBoolean:
		if t.driver == "postgres" || t.driver == "cockroach" || t.driver == "cockroachdb" {
			sb.WriteString("BOOLEAN")
		} else {
			sb.WriteString("TINYINT(1)")
		}
	case KindText:
		sb.WriteString("TEXT")
	case KindTimestamp:
		if t.driver == "postgres" || t.driver == "cockroach" || t.driver == "cockroachdb" {
			sb.WriteString("TIMESTAMPTZ")
		} else {
			sb.WriteString("DATETIME")
		}
	}

	if !col.isNullable {
		sb.WriteString(" NOT NULL")
	} else {
		sb.WriteString(" NULL")
	}

	if col.hasDefault {
		sb.WriteString(fmt.Sprintf(" DEFAULT '%v'", col.defaultVal))
	}

	if col.isUnique {
		sb.WriteString(" UNIQUE")
	}

	return sb.String()
}

// Builder wraps a contract.Tx to execute DDL operations atomically within a database transaction.
type Builder struct {
	tx     contract.Tx
	driver string
}

// NewBuilder constructs a Schema Builder bound to an active transaction and a driver name.
func NewBuilder(tx contract.Tx, driver string) *Builder {
	return &Builder{tx: tx, driver: driver}
}

// Create compiles and executes a CREATE TABLE statement.
func (b *Builder) Create(tableName string, fn func(*Table)) error {
	tbl := NewTable(b.driver)
	fn(tbl)
	ddl := tbl.Compile(tableName)
	if err := b.tx.Exec(ddl); err != nil {
		return fmt.Errorf("[Grapher] Create table '%s' failed: %w", tableName, err)
	}
	return nil
}

// Drop executes a DROP TABLE IF EXISTS statement for the given table name.
func (b *Builder) Drop(tableName string) error {
	ddl := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
	if err := b.tx.Exec(ddl); err != nil {
		return fmt.Errorf("[Grapher] Drop table '%s' failed: %w", tableName, err)
	}
	return nil
}
