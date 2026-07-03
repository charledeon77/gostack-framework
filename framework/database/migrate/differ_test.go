package migrate

import (
	"database/sql"
	"os"
	"testing"

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
