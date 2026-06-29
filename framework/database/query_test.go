/*
Purpose:
This file contains unit tests for QueryBuilder SQL compilation and eager loading in the database package.
It covers MySQL/Postgres placeholder generation, WhereIn clauses, HasMany and BelongsTo eager loading.

Architecture:
- mockDB: implements contract.Database to verify SQL query generation without a real database.
- relationsMockDriver: full sql.Driver implementation for eager loading integration tests.
- relationsTestDB: wraps sql.DB to satisfy contract.Database.

Implementation:
- TestQueryBuilderMySQLPlaceholders: verifies MySQL '?' parameterization.
- TestQueryBuilderPostgresPlaceholders: verifies PostgreSQL '$N' parameterization.
- TestWhereInMySQL: verifies WHERE IN clause generation for MySQL.
- TestWhereInPostgres: verifies WHERE IN clause generation for PostgreSQL.
- TestHydratorWithMockDriver: verifies the reflective hydration engine.
- TestEagerLoadHasMany: verifies HasMany relationship loading without N+1.
- TestEagerLoadBelongsTo: verifies BelongsTo relationship loading without N+1.
*/
package database

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"io"
	"strings"
	"sync"
	"testing"
)

// ─── Simple mockDB (no real SQL) ───────────────────────────────────────────────

type mockDB struct{ driver string }

func (m *mockDB) Connect() error                              { return nil }
func (m *mockDB) Query(sql string, args ...any) (any, error)  { return nil, nil }
func (m *mockDB) Exec(sql string, args ...any) error          { return nil }
func (m *mockDB) BeginTx() (contract.Tx, error)              { return nil, nil }
func (m *mockDB) Driver() string                              { return m.driver }
func (m *mockDB) Close() error                                { return nil }

// ─── QueryBuilder placeholder tests ───────────────────────────────────────────

func TestQueryBuilderMySQLPlaceholders(t *testing.T) {
	db := &mockDB{driver: "mysql"}
	qb := New(db, "users")
	qb.Where("id", "=", 10).Where("status", "=", "active")

	got := qb.ToSQL()
	want := "SELECT * FROM users WHERE id = ? AND status = ?"
	if got != want {
		t.Errorf("Expected SQL: %s, got: %s", want, got)
	}
	if len(qb.bindings) != 2 {
		t.Fatalf("Expected 2 bindings, got: %d", len(qb.bindings))
	}
}

func TestQueryBuilderPostgresPlaceholders(t *testing.T) {
	db := &mockDB{driver: "postgres"}
	qb := New(db, "users")
	qb.Where("id", "=", 10).Where("status", "=", "active")

	got := qb.ToSQL()
	want := "SELECT * FROM users WHERE id = $1 AND status = $2"
	if got != want {
		t.Errorf("Expected SQL: %s, got: %s", want, got)
	}
}

func TestWhereInMySQL(t *testing.T) {
	db := &mockDB{driver: "mysql"}
	qb := New(db, "users")
	qb.WhereIn("id", []any{1, 2, 3})

	got := qb.ToSQL()
	want := "SELECT * FROM users WHERE id IN (?, ?, ?)"
	if got != want {
		t.Errorf("Expected SQL: %s, got: %s", want, got)
	}
}

func TestWhereInPostgres(t *testing.T) {
	db := &mockDB{driver: "postgres"}
	qb := New(db, "users")
	qb.WhereIn("id", []any{1, 2, 3})

	got := qb.ToSQL()
	want := "SELECT * FROM users WHERE id IN ($1, $2, $3)"
	if got != want {
		t.Errorf("Expected SQL: %s, got: %s", want, got)
	}
}

// ─── Hydrator mock driver ──────────────────────────────────────────────────────

type hydratorMockRows struct {
	cols   []string
	data   [][]driver.Value
	cursor int
}

func (m *hydratorMockRows) Columns() []string { return m.cols }
func (m *hydratorMockRows) Close() error      { return nil }
func (m *hydratorMockRows) Next(dest []driver.Value) error {
	if m.cursor >= len(m.data) {
		return io.EOF
	}
	for i, v := range m.data[m.cursor] {
		dest[i] = v
	}
	m.cursor++
	return nil
}

type hydratorMockDriver struct{}

func (d *hydratorMockDriver) Open(name string) (driver.Conn, error) {
	return &hydratorMockConn{}, nil
}

type hydratorMockConn struct{}

func (c *hydratorMockConn) Prepare(query string) (driver.Stmt, error) {
	return &hydratorMockStmt{}, nil
}
func (c *hydratorMockConn) Close() error               { return nil }
func (c *hydratorMockConn) Begin() (driver.Tx, error) { return nil, nil }

type hydratorMockStmt struct{}

func (s *hydratorMockStmt) NumInput() int                                     { return -1 }
func (s *hydratorMockStmt) Close() error                                      { return nil }
func (s *hydratorMockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s *hydratorMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &hydratorMockRows{
		cols: []string{"id", "email"},
		data: [][]driver.Value{
			{int64(1), "first@gostack.io"},
			{int64(2), "second@gostack.io"},
		},
	}, nil
}

func init() {
	sql.Register("gostack_hydrator_mock", &hydratorMockDriver{})
}

type UserTestModel struct {
	ID    int64  `db:"id"`
	Email string `db:"email"`
}

func TestHydratorWithMockDriver(t *testing.T) {
	db, err := sql.Open("gostack_hydrator_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock connection: %v", err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, email FROM users")
	if err != nil {
		t.Fatalf("Failed to execute mock query: %v", err)
	}
	defer rows.Close()

	var users []UserTestModel
	if err := Hydrate(rows, &users); err != nil {
		t.Fatalf("Hydrate crashed: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("Expected 2 records, got: %d", len(users))
	}
	if users[0].ID != 1 || users[0].Email != "first@gostack.io" {
		t.Errorf("Record 0 mismatch: %+v", users[0])
	}
	if users[1].ID != 2 || users[1].Email != "second@gostack.io" {
		t.Errorf("Record 1 mismatch: %+v", users[1])
	}
}

// ─── Relations mock driver ────────────────────────────────────────────────────

var (
	relExecutedQueries []string
	relQueryMutex      sync.Mutex
)

func relTrackQuery(q string) {
	relQueryMutex.Lock()
	defer relQueryMutex.Unlock()
	relExecutedQueries = append(relExecutedQueries, q)
}

func relGetQueries() []string {
	relQueryMutex.Lock()
	defer relQueryMutex.Unlock()
	return append([]string{}, relExecutedQueries...)
}

func relClearQueries() {
	relQueryMutex.Lock()
	defer relQueryMutex.Unlock()
	relExecutedQueries = nil
}

type relationsMockDriver struct{}

func (d *relationsMockDriver) Open(name string) (driver.Conn, error) {
	return &relationsMockConn{}, nil
}

type relationsMockConn struct{}

func (c *relationsMockConn) Prepare(query string) (driver.Stmt, error) {
	return &relationsMockStmt{query: query}, nil
}
func (c *relationsMockConn) Close() error               { return nil }
func (c *relationsMockConn) Begin() (driver.Tx, error) { return nil, nil }

type relationsMockStmt struct{ query string }

func (s *relationsMockStmt) NumInput() int                                     { return -1 }
func (s *relationsMockStmt) Close() error                                      { return nil }
func (s *relationsMockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s *relationsMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	relTrackQuery(s.query)
	if strings.Contains(s.query, "COUNT(*)") {
		return &hydratorMockRows{
			cols: []string{"count"},
			data: [][]driver.Value{
				{int64(5)},
			},
		}, nil
	}
	if strings.Contains(s.query, "FROM users") {
		return &hydratorMockRows{
			cols: []string{"id", "name"},
			data: [][]driver.Value{
				{int64(1), "Alice"},
				{int64(2), "Bob"},
			},
		}, nil
	}
	if strings.Contains(s.query, "FROM posts") {
		return &hydratorMockRows{
			cols: []string{"id", "title", "user_id"},
			data: [][]driver.Value{
				{int64(10), "Post A", int64(1)},
				{int64(20), "Post B", int64(1)},
				{int64(30), "Post C", int64(2)},
			},
		}, nil
	}
	return nil, fmt.Errorf("[Mock] Unexpected query: %s", s.query)
}

func init() {
	sql.Register("gostack_relations_mock", &relationsMockDriver{})
}

type relationsTestDB struct {
	db     *sql.DB
	driver string
}

func (r *relationsTestDB) Connect() error                              { return nil }
func (r *relationsTestDB) Query(q string, args ...any) (any, error)  { return r.db.Query(q, args...) }
func (r *relationsTestDB) Exec(q string, args ...any) error          { _, err := r.db.Exec(q, args...); return err }
func (r *relationsTestDB) BeginTx() (contract.Tx, error)              { return nil, nil }
func (r *relationsTestDB) Driver() string                              { return r.driver }
func (r *relationsTestDB) Close() error                                { return nil }

type UserRelModel struct {
	ID    int64          `db:"id"`
	Name  string         `db:"name"`
	Posts []PostRelModel `rel:"has_many" fk:"user_id" table:"posts"`
}

type PostRelModel struct {
	ID     int64         `db:"id"`
	Title  string        `db:"title"`
	UserID int64         `db:"user_id"`
	User   *UserRelModel `rel:"belongs_to" fk:"user_id" table:"users"`
}

func TestEagerLoadHasMany(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var users []UserRelModel
	if err := New(testDB, "users").With("Posts").Get(&users); err != nil {
		t.Fatalf("Get with eager load failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}
	if len(users[0].Posts) != 2 {
		t.Fatalf("Expected Alice to have 2 posts, got %d", len(users[0].Posts))
	}
	if users[0].Posts[0].Title != "Post A" || users[0].Posts[1].Title != "Post B" {
		t.Errorf("Alice's posts mismatch: %+v", users[0].Posts)
	}
	if len(users[1].Posts) != 1 || users[1].Posts[0].Title != "Post C" {
		t.Errorf("Bob's posts mismatch: %+v", users[1].Posts)
	}

	queries := relGetQueries()
	if len(queries) != 2 {
		t.Fatalf("Expected exactly 2 SQL queries (no N+1), got %d: %v", len(queries), queries)
	}
}

func TestEagerLoadBelongsTo(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "postgres"}
	var posts []PostRelModel
	if err := New(testDB, "posts").With("User").Get(&posts); err != nil {
		t.Fatalf("Get with eager load failed: %v", err)
	}

	if len(posts) != 3 {
		t.Fatalf("Expected 3 posts, got %d", len(posts))
	}
	if posts[0].User == nil || posts[0].User.Name != "Alice" {
		t.Errorf("Post A owner mismatch: %+v", posts[0])
	}
	if posts[2].User == nil || posts[2].User.Name != "Bob" {
		t.Errorf("Post C owner mismatch: %+v", posts[2])
	}

	queries := relGetQueries()
	if len(queries) != 2 {
		t.Fatalf("Expected exactly 2 SQL queries (no N+1), got %d: %v", len(queries), queries)
	}
}

func TestQueryBuilder_Paginate(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var posts []PostRelModel
	meta, err := New(testDB, "posts").Paginate(&posts, 1, 2)
	if err != nil {
		t.Fatalf("Paginate failed: %v", err)
	}

	if meta == nil {
		t.Fatal("Expected metadata, got nil")
	}

	if meta.Total != 5 {
		t.Errorf("Expected total 5, got %d", meta.Total)
	}
	if meta.Page != 1 {
		t.Errorf("Expected page 1, got %d", meta.Page)
	}
	if meta.PerPage != 2 {
		t.Errorf("Expected per_page 2, got %d", meta.PerPage)
	}
	if meta.LastPage != 3 {
		t.Errorf("Expected last_page 3, got %d", meta.LastPage)
	}
	if !meta.HasNext {
		t.Errorf("Expected HasNext to be true")
	}
	if meta.HasPrev {
		t.Errorf("Expected HasPrev to be false")
	}

	if len(posts) != 3 {
		t.Errorf("Expected 3 posts, got %d", len(posts))
	}

	queries := relGetQueries()
	if len(queries) != 2 {
		t.Fatalf("Expected exactly 2 SQL queries (COUNT + SELECT), got %d: %v", len(queries), queries)
	}
	if !strings.Contains(queries[0], "SELECT COUNT(*)") {
		t.Errorf("Expected first query to be COUNT, got: %s", queries[0])
	}
	if !strings.Contains(queries[1], "LIMIT 2 OFFSET 0") {
		t.Errorf("Expected second query to have LIMIT 2 OFFSET 0, got: %s", queries[1])
	}
}
