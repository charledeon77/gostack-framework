package migrate

import (
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

type TestUserModel struct {
	ID       int64  `db:"id"`
	Username string `db:"username"`
	Email    string `db:"email"`
}

type TestPostModel struct {
	ID      int64  `db:"id"`
	UserID  int64  `db:"user_id"`
	Title   string `db:"title"`
	Content string `db:"content"` // newly added field in struct
}

func TestSQLiteDiff(t *testing.T) {
	dbFile := "test_differ.db"
	defer os.Remove(dbFile)

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	defer db.Close()

	// 1. Create tables with fewer columns than the struct models
	_, err = db.Exec(`CREATE TABLE testusermodels (
		id INTEGER PRIMARY KEY,
		username TEXT
	)`)
	if err != nil {
		t.Fatalf("failed to create users table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE testpostmodels (
		id INTEGER PRIMARY KEY,
		user_id INTEGER,
		title TEXT
	)`)
	if err != nil {
		t.Fatalf("failed to create posts table: %v", err)
	}

	// 2. Perform diff check using the differ
	diffs, err := Diff(db, "sqlite", TestUserModel{}, TestPostModel{})
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	// 3. Verify results
	if len(diffs) != 2 {
		t.Fatalf("expected 2 diff results, got %d", len(diffs))
	}

	for _, d := range diffs {
		if d.Table == "testusermodels" {
			if len(d.Statements) != 1 {
				t.Fatalf("expected 1 statement for users, got %d", len(d.Statements))
			}
			expected := "ALTER TABLE testusermodels ADD COLUMN email VARCHAR(255);"
			if d.Statements[0] != expected {
				t.Errorf("expected: %s, got: %s", expected, d.Statements[0])
			}
		} else if d.Table == "testpostmodels" {
			if len(d.Statements) != 1 {
				t.Fatalf("expected 1 statement for posts, got %d", len(d.Statements))
			}
			expected := "ALTER TABLE testpostmodels ADD COLUMN content VARCHAR(255);"
			if d.Statements[0] != expected {
				t.Errorf("expected: %s, got: %s", expected, d.Statements[0])
			}
		} else {
			t.Errorf("unexpected table name: %s", d.Table)
		}
	}
}

type ComplexFieldModel struct {
	Active     bool       `db:"active"`
	Score      float64    `db:"score"`
	Count      uint64     `db:"count"`
	CreatedAt  time.Time  `db:"created_at"`
	Payload    []byte     `db:"payload"`
	NullableID *int64     `db:"nullable_id"`
}

func TestGoTypeToSQL_Dialects(t *testing.T) {
	m := reflect.TypeOf(ComplexFieldModel{})

	// 1. Postgres checks
	activeField, _ := m.FieldByName("Active")
	if sqlType := goTypeToSQL(activeField.Type, "postgres"); sqlType != "BOOLEAN" {
		t.Errorf("Postgres bool: expected BOOLEAN, got %s", sqlType)
	}

	scoreField, _ := m.FieldByName("Score")
	if sqlType := goTypeToSQL(scoreField.Type, "postgres"); sqlType != "DOUBLE PRECISION" {
		t.Errorf("Postgres float64: expected DOUBLE PRECISION, got %s", sqlType)
	}

	countField, _ := m.FieldByName("Count")
	if sqlType := goTypeToSQL(countField.Type, "postgres"); sqlType != "BIGINT" {
		t.Errorf("Postgres uint64: expected BIGINT, got %s", sqlType)
	}

	createdField, _ := m.FieldByName("CreatedAt")
	if sqlType := goTypeToSQL(createdField.Type, "postgres"); sqlType != "TIMESTAMPTZ" {
		t.Errorf("Postgres time.Time: expected TIMESTAMPTZ, got %s", sqlType)
	}

	payloadField, _ := m.FieldByName("Payload")
	if sqlType := goTypeToSQL(payloadField.Type, "postgres"); sqlType != "BYTEA" {
		t.Errorf("Postgres []byte: expected BYTEA, got %s", sqlType)
	}

	ptrField, _ := m.FieldByName("NullableID")
	if sqlType := goTypeToSQL(ptrField.Type, "postgres"); sqlType != "BIGINT" {
		t.Errorf("Postgres *int64: expected BIGINT, got %s", sqlType)
	}

	// 2. MySQL checks
	if sqlType := goTypeToSQL(activeField.Type, "mysql"); sqlType != "TINYINT(1)" {
		t.Errorf("MySQL bool: expected TINYINT(1), got %s", sqlType)
	}
	if sqlType := goTypeToSQL(countField.Type, "mysql"); sqlType != "BIGINT UNSIGNED" {
		t.Errorf("MySQL uint64: expected BIGINT UNSIGNED, got %s", sqlType)
	}

	// 3. SQLite checks
	if sqlType := goTypeToSQL(activeField.Type, "sqlite"); sqlType != "INTEGER" {
		t.Errorf("SQLite bool: expected INTEGER, got %s", sqlType)
	}
	if sqlType := goTypeToSQL(payloadField.Type, "sqlite"); sqlType != "BLOB" {
		t.Errorf("SQLite []byte: expected BLOB, got %s", sqlType)
	}
}

func TestApplyDiff_TransactionalRollback(t *testing.T) {
	dbFile := "test_differ_rollback.db"
	defer os.Remove(dbFile)

	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		t.Fatalf("failed to open sqlite DB: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE rollback_test (id INTEGER PRIMARY KEY, name TEXT)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Batch where the FIRST statement is invalid SQL syntax.
	// Invalid SQL must come first so that ApplyDiff returns an error
	// before any DDL is executed. SQLite auto-commits DDL inside
	// transactions, so a DDL executed before the failing statement
	// cannot be rolled back — this ordering is intentional.
	badBatch := []DiffResult{
		{
			Table: "rollback_test",
			Statements: []string{
				"INVALID SQL STATEMENT THAT MUST FAIL;",
				"ALTER TABLE rollback_test ADD COLUMN valid_col VARCHAR(255);",
			},
		},
	}

	err = ApplyDiff(db, badBatch)
	if err == nil {
		t.Fatal("expected ApplyDiff to fail on invalid statement, got nil")
	}

	// Verify rollback: valid_col must NOT exist in rollback_test!
	var colCount int
	row := db.QueryRow("SELECT count(*) FROM pragma_table_info('rollback_test') WHERE name = 'valid_col'")
	if err := row.Scan(&colCount); err != nil {
		t.Fatalf("failed to query pragma_table_info: %v", err)
	}
	if colCount != 0 {
		t.Errorf("expected valid_col to be rolled back, but it exists in table!")
	}
}
