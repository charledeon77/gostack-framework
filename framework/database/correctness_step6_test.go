package database

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

// PointerTableNameModel defines TableName on pointer receiver.
type PointerTableNameModel struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
}

func (p *PointerTableNameModel) TableName() string {
	return "custom_pointer_table"
}

// F-183: Test that ActiveRecord uses TableName() defined on pointer receivers.
func TestActiveRecord_UsesPointerTableName(t *testing.T) {
	tableName := getTableName(new(PointerTableNameModel))
	if tableName != "custom_pointer_table" {
		t.Fatalf("expected tableName to be 'custom_pointer_table', got %q", tableName)
	}
}

// F-184: Test that nil values or mismatched types in Create return error instead of panicking.
func TestCreate_NullValueReturnsError(t *testing.T) {
	m := new(PointerTableNameModel)
	val := reflect.ValueOf(m).Elem()
	field := val.FieldByName("Name")

	// Attempting to set an invalid type or nil to a non-nullable field
	err := applyField(field, "Name", nil)
	if err == nil {
		t.Fatal("expected error when setting nil to non-nullable string field, got nil")
	}

	err = applyField(field, "Name", []int{1, 2, 3})
	if err == nil {
		t.Fatal("expected error when setting slice into string field, got nil")
	}
}

// F-185: Test that Postgres boolean values 't' and 'f' are correctly hydrated.
func TestHydrate_PostgresBooleanTrue(t *testing.T) {
	type BoolModel struct {
		ID       int64 `db:"id"`
		IsActive bool  `db:"is_active"`
	}

	var m BoolModel
	cols := []string{"id", "is_active"}
	rawValues := []sql.RawBytes{sql.RawBytes("1"), sql.RawBytes("t")}

	err := mapRowToStruct(cols, rawValues, reflect.ValueOf(&m).Elem())
	if err != nil {
		t.Fatalf("unexpected hydration error: %v", err)
	}
	if !m.IsActive {
		t.Fatalf("expected IsActive to be true for 't', got false")
	}

	rawValues[1] = sql.RawBytes("f")
	err = mapRowToStruct(cols, rawValues, reflect.ValueOf(&m).Elem())
	if err != nil {
		t.Fatalf("unexpected hydration error: %v", err)
	}
	if m.IsActive {
		t.Fatalf("expected IsActive to be false for 'f', got true")
	}
}

// F-186: Test hydration of pointer fields and slice/byte fields.
func TestHydrate_PointerAndBytesFields(t *testing.T) {
	type ComplexModel struct {
		ID      int64    `db:"id"`
		Bio     *string  `db:"bio"`
		Score   *int     `db:"score"`
		Payload []byte   `db:"payload"`
		Tags    []string `db:"tags"`
	}

	var m ComplexModel
	cols := []string{"id", "bio", "score", "payload", "tags"}
	rawValues := []sql.RawBytes{
		sql.RawBytes("10"),
		sql.RawBytes("Developer"),
		sql.RawBytes("99"),
		sql.RawBytes("raw_data"),
		sql.RawBytes(`["go", "framework"]`),
	}

	err := mapRowToStruct(cols, rawValues, reflect.ValueOf(&m).Elem())
	if err != nil {
		t.Fatalf("unexpected hydration error: %v", err)
	}

	if m.Bio == nil || *m.Bio != "Developer" {
		t.Fatalf("expected Bio to be 'Developer', got %v", m.Bio)
	}
	if m.Score == nil || *m.Score != 99 {
		t.Fatalf("expected Score to be 99, got %v", m.Score)
	}
	if string(m.Payload) != "raw_data" {
		t.Fatalf("expected Payload to be 'raw_data', got %s", string(m.Payload))
	}
	if len(m.Tags) != 2 || m.Tags[0] != "go" || m.Tags[1] != "framework" {
		t.Fatalf("expected Tags to be ['go', 'framework'], got %v", m.Tags)
	}
}

// BeforeSaveMutationModel mutates its password in BeforeSave.
type BeforeSaveMutationModel struct {
	ID       int64  `db:"id"`
	Password string `db:"password"`
}

func (b *BeforeSaveMutationModel) BeforeSave() error {
	b.Password = "hashed_" + b.Password
	return nil
}

// F-187: Test that mutations inside BeforeSave hook are persisted in INSERT.
func TestBeforeSave_MutationIsPersisted(t *testing.T) {
	db := &execRecordingMockDB{driver: "mysql"}
	qb := New(db, "users")

	model := &BeforeSaveMutationModel{
		Password: "plain_password",
	}

	data := map[string]any{
		"password": model.Password,
	}

	err := qb.InsertModel(model, data)
	if err != nil {
		t.Fatalf("unexpected error in InsertModel: %v", err)
	}

	// Verify that the data passed to SQL contains the hashed password
	foundHashed := false
	for _, arg := range db.lastArgs {
		if arg == "hashed_plain_password" {
			foundHashed = true
			break
		}
	}
	if !foundHashed {
		t.Fatalf("expected BeforeSave mutation 'hashed_plain_password' to be persisted, args were: %v", db.lastArgs)
	}
}

// F-197: Test that hostile pivot table tags in many-to-many relations are rejected.
func TestRelation_HostilePivotTagRejected(t *testing.T) {
	type HostilePivotModel struct {
		ID       int64 `db:"id"`
		Children []struct {
			ID int64 `db:"id"`
		} `rel:"many_to_many" pivot:"users; DROP TABLE users;--" fk:"user_id" related_fk:"child_id"`
	}

	db := &mockDB{driver: "mysql"}
	qb := New(db, "users").With("Children")

	items := []HostilePivotModel{{ID: 1}}
	err := qb.eagerLoadRelations(&items)
	if err == nil {
		t.Fatal("expected error for hostile pivot table name, got nil")
	}
	if !strings.Contains(err.Error(), "contains invalid characters") {
		t.Fatalf("expected error mentioning 'contains invalid characters', got: %v", err)
	}
}

// F-207, F-208: Test that Builder.Create and Builder.Drop reject invalid table names.
func TestSchema_RejectsInvalidTableName(t *testing.T) {
	mockTx := &mockSchemaTx{}
	builder := NewBuilder(mockTx, "postgres")

	// Hostile table name with SQL injection
	err := builder.Create("users; DROP TABLE secrets;--", func(table *Table) {
		table.ID()
	})
	if err == nil {
		t.Fatal("expected Create to reject invalid table name, got nil")
	}

	err = builder.Drop("users; DROP TABLE secrets;--")
	if err == nil {
		t.Fatal("expected Drop to reject invalid table name, got nil")
	}
}

// badTypeDB returns a string instead of *sql.Rows from Query.
type badTypeDB struct {
	mockDB
}

func (b *badTypeDB) Query(sql string, args ...any) (any, error) {
	return "not_sql_rows", nil
}

// F-213: Test that Migrator returns an error if Query returns an unexpected type.
func TestMigrator_TypeAssertionFailReturnsError(t *testing.T) {
	m := NewMigrator(&badTypeDB{})
	_, err := m.completedVersions()
	if err == nil {
		t.Fatal("expected error on failed *sql.Rows type assertion, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected result type") {
		t.Fatalf("expected 'unexpected result type' error, got %v", err)
	}
}
