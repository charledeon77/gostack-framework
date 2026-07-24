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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
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

type embeddedMockRows struct {
	cols   []string
	data   [][]driver.Value
	cursor int
}

func (m *embeddedMockRows) Columns() []string { return m.cols }
func (m *embeddedMockRows) Close() error      { return nil }
func (m *embeddedMockRows) Next(dest []driver.Value) error {
	if m.cursor >= len(m.data) {
		return io.EOF
	}
	for i, v := range m.data[m.cursor] {
		dest[i] = v
	}
	m.cursor++
	return nil
}

type embeddedMockDriver struct{}

func (d *embeddedMockDriver) Open(name string) (driver.Conn, error) {
	return &embeddedMockConn{}, nil
}

type embeddedMockConn struct{}

func (c *embeddedMockConn) Prepare(query string) (driver.Stmt, error) {
	return &embeddedMockStmt{}, nil
}
func (c *embeddedMockConn) Close() error               { return nil }
func (c *embeddedMockConn) Begin() (driver.Tx, error) { return nil, nil }

type embeddedMockStmt struct{}

func (s *embeddedMockStmt) NumInput() int                                     { return -1 }
func (s *embeddedMockStmt) Close() error                                      { return nil }
func (s *embeddedMockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s *embeddedMockStmt) Query(args []driver.Value) (driver.Rows, error) {
	return &embeddedMockRows{
		cols: []string{"id", "email", "password", "failed_attempts"},
		data: [][]driver.Value{
			{int64(42), "embedded@gostack.io", "secret_pass", int64(3)},
		},
	}, nil
}

func init() {
	sql.Register("gostack_embedded_hydrator_mock", &embeddedMockDriver{})
}

type TestEmbeddedInner struct {
	Password string `db:"password"`
}

type TestEmbeddedOuter struct {
	ID             int64  `db:"id"`
	Email          string `db:"email"`
	TestEmbeddedInner
	FailedAttempts int    `db:"failed_attempts"`
}

type TestEmbeddedOuterPtr struct {
	ID             int64  `db:"id"`
	Email          string `db:"email"`
	*TestEmbeddedInner
	FailedAttempts int    `db:"failed_attempts"`
}

func TestHydratorWithEmbeddedStructs(t *testing.T) {
	db, err := sql.Open("gostack_embedded_hydrator_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock connection: %v", err)
	}
	defer db.Close()

	// 1. Test embedding by value
	rows, err := db.Query("SELECT id, email, password, failed_attempts")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	
	var valueModels []TestEmbeddedOuter
	if err := Hydrate(rows, &valueModels); err != nil {
		t.Fatalf("Hydrate by value failed: %v", err)
	}
	rows.Close()

	if len(valueModels) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(valueModels))
	}
	if valueModels[0].ID != 42 || valueModels[0].Email != "embedded@gostack.io" || valueModels[0].Password != "secret_pass" || valueModels[0].FailedAttempts != 3 {
		t.Errorf("Value model hydration mismatch: %+v", valueModels[0])
	}

	// 2. Test embedding by pointer (with nil-pointer allocation)
	rows, err = db.Query("SELECT id, email, password, failed_attempts")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var ptrModels []TestEmbeddedOuterPtr
	if err := Hydrate(rows, &ptrModels); err != nil {
		t.Fatalf("Hydrate by pointer failed: %v", err)
	}

	if len(ptrModels) != 1 {
		t.Fatalf("Expected 1 record, got %d", len(ptrModels))
	}
	if ptrModels[0].ID != 42 || ptrModels[0].Email != "embedded@gostack.io" || ptrModels[0].TestEmbeddedInner == nil || ptrModels[0].Password != "secret_pass" || ptrModels[0].FailedAttempts != 3 {
		t.Errorf("Pointer model hydration mismatch: %+v", ptrModels[0])
		if ptrModels[0].TestEmbeddedInner != nil {
			t.Errorf("Embedded pointer value: %+v", ptrModels[0].TestEmbeddedInner)
		}
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
	if strings.Contains(s.query, "FROM profiles") {
		return &hydratorMockRows{
			cols: []string{"id", "user_id", "bio"},
			data: [][]driver.Value{
				{int64(100), int64(1), "Bio of Alice"},
				{int64(200), int64(2), "Bio of Bob"},
			},
		}, nil
	}
	if strings.Contains(s.query, "FROM role_user") {
		return &hydratorMockRows{
			cols: []string{"user_id", "role_id"},
			data: [][]driver.Value{
				{int64(1), int64(10)},
				{int64(1), int64(20)},
				{int64(2), int64(20)},
			},
		}, nil
	}
	if strings.Contains(s.query, "FROM roles") {
		return &hydratorMockRows{
			cols: []string{"id", "name"},
			data: [][]driver.Value{
				{int64(10), "Admin"},
				{int64(20), "Editor"},
			},
		}, nil
	}
	if strings.Contains(s.query, "FROM users") {
		// Mock hasManyThrough query targeting intermediate users table
		if strings.Contains(s.query, "country_id") {
			return &hydratorMockRows{
				cols: []string{"id", "country_id"},
				data: [][]driver.Value{
					{int64(1), int64(50)},
					{int64(2), int64(60)},
				},
			}, nil
		}
		// Standard query
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
	if strings.Contains(s.query, "FROM countries") {
		return &hydratorMockRows{
			cols: []string{"id", "name"},
			data: [][]driver.Value{
				{int64(50), "USA"},
				{int64(60), "Canada"},
			},
		}, nil
	}
	return nil, fmt.Errorf("[Mock] Unexpected query: %s", s.query)
}

func init() {
	sql.Register("gostack_relations_mock", &relationsMockDriver{})
}

type mockTx struct {
	db *relationsTestDB
}

func (m *mockTx) Exec(q string, args ...any) error {
	relTrackQuery("[TX] " + q)
	return m.db.Exec(q, args...)
}

func (m *mockTx) Query(q string, args ...any) (any, error) {
	relTrackQuery("[TX] " + q)
	return m.db.Query(q, args...)
}

func (m *mockTx) Commit() error {
	relTrackQuery("[TX] COMMIT")
	return nil
}

func (m *mockTx) Rollback() error {
	relTrackQuery("[TX] ROLLBACK")
	return nil
}

type relationsTestDB struct {
	db     *sql.DB
	driver string
}

func (r *relationsTestDB) Connect() error                              { return nil }
func (r *relationsTestDB) Query(q string, args ...any) (any, error)  { return r.db.Query(q, args...) }
func (r *relationsTestDB) Exec(q string, args ...any) error          { _, err := r.db.Exec(q, args...); return err }
func (r *relationsTestDB) BeginTx() (contract.Tx, error)              { return &mockTx{db: r}, nil }
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

func TestQueryBuilder_SelectColumns(t *testing.T) {
	qb := New(nil, "users").Select("id", "email", "name")
	sqlStr := qb.ToSQL()
	expected := "SELECT id, email, name FROM users"
	if sqlStr != expected {
		t.Errorf("Expected SQL %q, got %q", expected, sqlStr)
	}
}

func TestQueryBuilder_LogicalFilters(t *testing.T) {
	qb := New(nil, "users").
		Where("status", "=", "active").
		OrWhere("role", "=", "admin").
		WhereNull("deleted_at").
		WhereNotNull("verified_at").
		WhereBetween("age", 18, 65).
		WhereLike("name", "John%")

	sqlStr := qb.ToSQL()
	expected := "SELECT * FROM users WHERE status = ? OR role = ? AND deleted_at IS NULL AND verified_at IS NOT NULL AND age BETWEEN ? AND ? AND name LIKE ?"
	if sqlStr != expected {
		t.Errorf("Expected SQL %q, got %q", expected, sqlStr)
	}
}

func TestQueryBuilder_Joins(t *testing.T) {
	qb := New(nil, "users").
		Join("profiles", "users.id", "=", "profiles.user_id").
		LeftJoin("posts", "users.id", "=", "posts.user_id").
		RightJoin("roles", "users.role_id", "=", "roles.id")

	sqlStr := qb.ToSQL()
	expected := "SELECT * FROM users INNER JOIN profiles ON users.id = profiles.user_id LEFT JOIN posts ON users.id = posts.user_id RIGHT JOIN roles ON users.role_id = roles.id"
	if sqlStr != expected {
		t.Errorf("Expected SQL %q, got %q", expected, sqlStr)
	}
}

type SoftDeleteModel struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	DeletedAt *time.Time `db:"deleted_at"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
}

func TestActiveRecord_LifecycleHelpers(t *testing.T) {
	var model SoftDeleteModel
	typ := reflect.TypeOf(model)
	if !hasField(typ, "DeletedAt") {
		t.Error("Expected SoftDeleteModel to have DeletedAt field")
	}
	if !hasField(typ, "CreatedAt") {
		t.Error("Expected SoftDeleteModel to have CreatedAt field")
	}
	if !hasField(typ, "UpdatedAt") {
		t.Error("Expected SoftDeleteModel to have UpdatedAt field")
	}
}

type ProfileRelModel struct {
	ID     int64  `db:"id"`
	UserID int64  `db:"user_id"`
	Bio    string `db:"bio"`
}

type RoleRelModel struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

type UserWithRelations struct {
	ID      int64             `db:"id"`
	Name    string            `db:"name"`
	Profile *ProfileRelModel  `rel:"has_one" fk:"user_id" table:"profiles"`
	Roles   []RoleRelModel    `rel:"many_to_many" pivot:"role_user" fk:"user_id" related_fk:"role_id" table:"roles"`
	Posts   []PostRelModel    `rel:"has_many" fk:"user_id" table:"posts"`
}

type CountryRelModel struct {
	ID    int64          `db:"id"`
	Name  string         `db:"name"`
	Posts []PostRelModel `rel:"has_many_through" through:"users" fk:"country_id" through_fk:"user_id" table:"posts"`
}

func TestEagerLoadHasOne(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var users []UserWithRelations
	err = New(testDB, "users").With("Profile").Get(&users)
	if err != nil {
		t.Fatalf("Eager load HasOne failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}

	if users[0].Profile == nil || users[0].Profile.Bio != "Bio of Alice" {
		t.Errorf("Alice profile mismatch: %+v", users[0].Profile)
	}
	if users[1].Profile == nil || users[1].Profile.Bio != "Bio of Bob" {
		t.Errorf("Bob profile mismatch: %+v", users[1].Profile)
	}

	queries := relGetQueries()
	if len(queries) != 2 {
		t.Fatalf("Expected exactly 2 SQL queries, got %d: %v", len(queries), queries)
	}
}

func TestEagerLoadManyToMany(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var users []UserWithRelations
	err = New(testDB, "users").With("Roles").Get(&users)
	if err != nil {
		t.Fatalf("Eager load ManyToMany failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}

	if len(users[0].Roles) != 2 || users[0].Roles[0].Name != "Admin" || users[0].Roles[1].Name != "Editor" {
		t.Errorf("Alice roles mismatch: %+v", users[0].Roles)
	}
	if len(users[1].Roles) != 1 || users[1].Roles[0].Name != "Editor" {
		t.Errorf("Bob roles mismatch: %+v", users[1].Roles)
	}

	queries := relGetQueries()
	if len(queries) != 3 { // SELECT users, SELECT pivot pairs, SELECT roles
		t.Fatalf("Expected exactly 3 SQL queries, got %d: %v", len(queries), queries)
	}
}

func TestEagerLoadHasManyThrough(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var countries []CountryRelModel
	err = New(testDB, "countries").With("Posts").Get(&countries)
	if err != nil {
		t.Fatalf("Eager load HasManyThrough failed: %v", err)
	}

	if len(countries) != 2 {
		t.Fatalf("Expected 2 countries, got %d", len(countries))
	}

	// USA (id: 50) -> through user (id: 1) -> posts (user_id: 1) -> Post A (10), Post B (20)
	if len(countries[0].Posts) != 2 || countries[0].Posts[0].Title != "Post A" || countries[0].Posts[1].Title != "Post B" {
		t.Errorf("USA posts mismatch: %+v", countries[0].Posts)
	}

	// Canada (id: 60) -> through user (id: 2) -> posts (user_id: 2) -> Post C (30)
	if len(countries[1].Posts) != 1 || countries[1].Posts[0].Title != "Post C" {
		t.Errorf("Canada posts mismatch: %+v", countries[1].Posts)
	}

	queries := relGetQueries()
	if len(queries) != 3 { // SELECT countries, SELECT through users, SELECT far posts
		t.Fatalf("Expected exactly 3 SQL queries, got %d: %v", len(queries), queries)
	}
}

func TestNestedEagerLoading(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}
	var users []UserWithRelations
	err = New(testDB, "users").With("Posts.User").Get(&users)
	if err != nil {
		t.Fatalf("Nested eager load failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(users))
	}
	if len(users[0].Posts) != 2 {
		t.Fatalf("Expected 2 posts for user 0, got %d", len(users[0].Posts))
	}
	// Verify nested loading loaded User back on Post
	if users[0].Posts[0].User == nil || users[0].Posts[0].User.Name != "Alice" {
		t.Errorf("Nested user not eager-loaded correctly on Post A: %+v", users[0].Posts[0].User)
	}

	queries := relGetQueries()
	if len(queries) != 3 { // SELECT users, SELECT posts, SELECT nested users
		t.Fatalf("Expected exactly 3 SQL queries, got %d: %v", len(queries), queries)
	}
}

func TestDatabaseTransactions(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}

	err = Transaction(testDB, func(tx contract.Tx) error {
		var users []UserWithRelations
		err := New(testDB, "users").WithTx(tx).Get(&users)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Transaction closure failed: %v", err)
	}

	queries := relGetQueries()
	// Should see "[TX]" prefix on queries and a COMMIT
	foundTXQuery := false
	foundCommit := false
	for _, q := range queries {
		if strings.HasPrefix(q, "[TX] SELECT") {
			foundTXQuery = true
		}
		if q == "[TX] COMMIT" {
			foundCommit = true
		}
	}
	if !foundTXQuery {
		t.Error("Expected queries inside transaction to carry [TX] execution context marker")
	}
	if !foundCommit {
		t.Error("Expected transaction COMMIT callback to run successfully")
	}
}

func TestNestedTransactions(t *testing.T) {
	relClearQueries()
	dbConn, err := sql.Open("gostack_relations_mock", "")
	if err != nil {
		t.Fatalf("Failed to open mock db: %v", err)
	}
	defer dbConn.Close()

	testDB := &relationsTestDB{db: dbConn, driver: "mysql"}

	// Outer transaction
	err = Transaction(testDB, func(tx contract.Tx) error {
		// Nested transaction
		err := Transaction(tx, func(subTx contract.Tx) error {
			var users []UserWithRelations
			return New(testDB, "users").WithTx(subTx).Get(&users)
		})
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Nested Transaction failed: %v", err)
	}

	queries := relGetQueries()
	
	foundSavepoint := false
	foundRelease := false
	for _, q := range queries {
		if strings.Contains(q, "SAVEPOINT sp_") {
			foundSavepoint = true
		}
		if strings.Contains(q, "RELEASE SAVEPOINT sp_") {
			foundRelease = true
		}
	}

	if !foundSavepoint {
		t.Error("Expected SAVEPOINT to be registered for nested transaction")
	}
	if !foundRelease {
		t.Error("Expected RELEASE SAVEPOINT to be registered for completed nested transaction")
	}
}

