/*
Purpose:
This file contains unit tests for the schema Table DDL compiler in the database package.
It validates that fluent column definitions are correctly translated into database-specific SQL.

Philosophy:
Fast, independent unit tests that verify compilation logic at the string level,
without spinning up live database instances.

Architecture:
Instantiates Table structs configured with different drivers, chains fluent methods,
and asserts the exact string match of compiled DDL against expected driver dialects.

Implementation:
- TestTableCompileMySQL: Verifies compiled SQL for MySQL columns.
- TestTableCompilePostgres: Verifies compiled SQL for PostgreSQL columns.
*/
package database

import (
	"strings"
	"testing"
)

func TestTableCompileMySQL(t *testing.T) {
	tbl := NewTable("mysql")
	tbl.ID()
	tbl.String("email").Unique()
	tbl.Integer("age").Nullable()
	tbl.Boolean("is_active").Default(1)
	tbl.Text("bio")
	tbl.Timestamps()

	ddl := tbl.Compile("users")

	expectedClauses := []string{
		"id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY",
		"email VARCHAR(255) NOT NULL UNIQUE",
		"age INT NULL",
		"is_active TINYINT(1) NOT NULL DEFAULT '1'",
		"bio TEXT NOT NULL",
		"created_at DATETIME NOT NULL",
		"updated_at DATETIME NULL",
	}

	for _, clause := range expectedClauses {
		if !strings.Contains(ddl, clause) {
			t.Errorf("Expected compiled DDL to contain: %q\nGot:\n%s", clause, ddl)
		}
	}
}

func TestTableCompilePostgres(t *testing.T) {
	tbl := NewTable("postgres")
	tbl.ID()
	tbl.String("email").Unique()
	tbl.Integer("age").Nullable()
	tbl.Boolean("is_active").Default(true)
	tbl.Text("bio")
	tbl.Timestamps()

	ddl := tbl.Compile("users")

	expectedClauses := []string{
		"id BIGSERIAL PRIMARY KEY",
		"email VARCHAR(255) NOT NULL UNIQUE",
		"age INT NULL",
		"is_active BOOLEAN NOT NULL DEFAULT 'true'",
		"bio TEXT NOT NULL",
		"created_at TIMESTAMPTZ NOT NULL",
		"updated_at TIMESTAMPTZ NULL",
	}

	for _, clause := range expectedClauses {
		if !strings.Contains(ddl, clause) {
			t.Errorf("Expected compiled DDL to contain: %q\nGot:\n%s", clause, ddl)
		}
	}
}
