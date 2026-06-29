package migrate

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
)

// Purpose: To provide automatic struct-to-database schema diffing for Traveller (Migration Engine).
// Philosophy: Django's `makemigrations` is one of its most beloved productivity features.
// Rather than requiring developers to manually write migration files every time they add a
// struct field, the Differ inspects Go struct tags and the live database schema, computes
// what columns are missing, and outputs ready-to-run ALTER TABLE SQL statements.
// Architecture:
// The Differ accepts a `database/sql.DB` handle and a slice of model instances (as `any`).
// It queries `INFORMATION_SCHEMA.COLUMNS` for each model's table, then reflects the struct's
// `db` tags to build a diff of missing columns. It outputs `ALTER TABLE` statements as strings
// so the developer can review them before running — or they can call `ApplyDiff` to run them.
// Choice:
// We output ALTER TABLE strings rather than auto-running them silently. This matches the
// philosophy of Django's `makemigrations` (generate, review) vs `migrate` (run). Developers
// retain full control. The table name is derived from the struct name (lowercase + "s") by
// default, but can be overridden via the `table` struct tag.
// Implementation:
// - Diff(db, models...): Reflects each model and queries INFORMATION_SCHEMA for missing columns.
// - ApplyDiff(db, stmts): Executes the generated ALTER TABLE statements.
// - goTypeToSQL(kind): Maps Go reflect.Kind to a sensible SQL column type.

// DiffResult holds the generated SQL for one model.
type DiffResult struct {
	Table      string
	Statements []string
}

// Diff compares Go struct models against the live database schema and returns ALTER TABLE statements
// for any columns that are present in the struct but missing from the database table.
// The driver parameter must match the active database driver (e.g. "mysql", "postgres", "sqlite", "cockroachdb").
func Diff(db *sql.DB, driver string, models ...any) ([]DiffResult, error) {
	var results []DiffResult

	for _, model := range models {
		t := reflect.TypeOf(model)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return nil, fmt.Errorf("differ: expected struct, got %s", t.Kind())
		}

		// Determine table name
		tableName := strings.ToLower(t.Name()) + "s"
		// Check if there is a TableName() method for custom table naming
		if m, ok := model.(interface{ TableName() string }); ok {
			tableName = m.TableName()
		}

		// Fetch existing columns from the database
		existingCols, err := fetchColumns(db, driver, tableName)
		if err != nil {
			return nil, fmt.Errorf("differ: failed to fetch columns for table '%s': %w", tableName, err)
		}

		var stmts []string
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			dbTag := field.Tag.Get("db")
			if dbTag == "" || dbTag == "-" {
				continue
			}
			// Skip relation fields
			if field.Tag.Get("rel") != "" {
				continue
			}

			if _, exists := existingCols[dbTag]; !exists {
				sqlType := goTypeToSQL(field.Type.Kind())
				stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", tableName, dbTag, sqlType)
				stmts = append(stmts, stmt)
			}
		}

		if len(stmts) > 0 {
			results = append(results, DiffResult{
				Table:      tableName,
				Statements: stmts,
			})
		}
	}

	return results, nil
}

// ApplyDiff executes the generated ALTER TABLE statements against the database.
func ApplyDiff(db *sql.DB, results []DiffResult) error {
	for _, result := range results {
		for _, stmt := range result.Statements {
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("differ: failed to apply migration '%s': %w", stmt, err)
			}
		}
	}
	return nil
}

// fetchColumns queries the database schema for the column names of a table,
// using the appropriate query for each database driver.
func fetchColumns(db *sql.DB, driver string, table string) (map[string]struct{}, error) {
	cols := make(map[string]struct{})
	var rows *sql.Rows
	var err error

	switch driver {
	case "sqlite", "sqlite3":
		// SQLite uses PRAGMA table_info() — INFORMATION_SCHEMA is not supported.
		rows, err = db.Query("SELECT name FROM pragma_table_info(?)", table)
	case "postgres", "cockroach", "cockroachdb":
		rows, err = db.Query(
			`SELECT column_name FROM information_schema.columns WHERE table_name = $1`, table,
		)
	default:
		// MySQL and other INFORMATION_SCHEMA-compatible databases.
		rows, err = db.Query(
			`SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ?`, table,
		)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = struct{}{}
	}
	return cols, rows.Err()
}

// goTypeToSQL maps a Go reflect.Kind to a sensible default SQL column type.
func goTypeToSQL(k reflect.Kind) string {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "BIGINT"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "BIGINT UNSIGNED"
	case reflect.Float32, reflect.Float64:
		return "DOUBLE"
	case reflect.Bool:
		return "TINYINT(1)"
	default:
		return "VARCHAR(255)"
	}
}
