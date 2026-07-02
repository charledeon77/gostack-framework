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

type mockSchemaTx struct {
	queries []string
	args    [][]any
}

func (m *mockSchemaTx) Exec(sql string, args ...any) error {
	m.queries = append(m.queries, sql)
	m.args = append(m.args, args)
	return nil
}

func (m *mockSchemaTx) Query(sql string, args ...any) (any, error) { return nil, nil }
func (m *mockSchemaTx) Commit() error                              { return nil }
func (m *mockSchemaTx) Rollback() error                            { return nil }

func TestBuilderExecAndRaw(t *testing.T) {
	tx := &mockSchemaTx{}
	builder := NewBuilder(tx, "sqlite")

	err := builder.Exec("ALTER TABLE users ADD COLUMN age INT", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = builder.Raw("CREATE INDEX idx_users_email ON users(email)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tx.queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(tx.queries))
	}

	if tx.queries[0] != "ALTER TABLE users ADD COLUMN age INT" {
		t.Errorf("expected first query to be ALTER TABLE, got %q", tx.queries[0])
	}
	if len(tx.args[0]) != 1 || tx.args[0][0] != 42 {
		t.Errorf("expected argument 42, got %v", tx.args[0])
	}

	if tx.queries[1] != "CREATE INDEX idx_users_email ON users(email)" {
		t.Errorf("expected second query to be CREATE INDEX, got %q", tx.queries[1])
	}
}
