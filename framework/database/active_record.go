package database

import (
	"github.com/charledeon77/gostack-framework/framework/contract"
	"reflect"
	"strings"
	"time"
)

// Model defines an optional interface for custom database table name mapping.
type Model interface {
	TableName() string
}

// Find retrieves a single record from the database by primary key id.
func Find[T any](db contract.Database, id any) (*T, error) {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	if hasField(reflect.TypeOf(model), "DeletedAt") {
		qb.WhereNull("deleted_at")
	}
	err := qb.Where("id", "=", id).First(&model)
	if err != nil {
		return nil, err
	}
	return &model, nil
}

// All retrieves all records for the model type from the database.
func All[T any](db contract.Database) ([]T, error) {
	var models []T
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	if hasField(reflect.TypeOf(model), "DeletedAt") {
		qb.WhereNull("deleted_at")
	}
	err := qb.Get(&models)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// Create inserts a new record into the database and returns the populated struct.
func Create[T any](db contract.Database, fields map[string]any) (*T, error) {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	
	// Inject auto-timestamps if they exist and are not already set
	t := reflect.TypeOf(model)
	now := time.Now()
	if hasField(t, "CreatedAt") {
		if _, ok := fields["created_at"]; !ok {
			fields["created_at"] = now
		}
	}
	if hasField(t, "UpdatedAt") {
		if _, ok := fields["updated_at"]; !ok {
			fields["updated_at"] = now
		}
	}

	err := qb.InsertModel(&model, fields)
	if err != nil {
		return nil, err
	}

	// Reflectively apply the fields to the model object to return a hydrated struct.
	val := reflect.ValueOf(&model).Elem()
	for k, v := range fields {
		fName, err := findFieldByDBTag(reflect.TypeOf(model), k)
		if err == nil {
			f := val.FieldByName(fName)
			if f.CanSet() {
				// Convert types if needed (e.g. float64 from JSON to int/uint)
				vVal := reflect.ValueOf(v)
				if f.Type() != vVal.Type() && vVal.Type().ConvertibleTo(f.Type()) {
					f.Set(vVal.Convert(f.Type()))
				} else {
					f.Set(vVal)
				}
			}
		}
	}
	return &model, nil
}

// Update updates records matching the primary key id.
func Update[T any](db contract.Database, id any, fields map[string]any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)

	// Inject auto-timestamp update
	t := reflect.TypeOf(model)
	if hasField(t, "UpdatedAt") {
		if _, ok := fields["updated_at"]; !ok {
			fields["updated_at"] = time.Now()
		}
	}

	if hasField(t, "DeletedAt") {
		qb.WhereNull("deleted_at")
	}

	return qb.Where("id", "=", id).Update(fields)
}

// DeleteRecord deletes records matching the primary key id.
// If the model supports soft deletes, it updates deleted_at instead of deleting.
func DeleteRecord[T any](db contract.Database, id any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	t := reflect.TypeOf(model)

	if hasField(t, "DeletedAt") {
		return qb.Where("id", "=", id).Update(map[string]any{
			"deleted_at": time.Now(),
		})
	}

	return qb.Where("id", "=", id).Delete()
}

// getTableName extracts table name dynamically or falls back to pluralized type.
func getTableName(val any) string {
	if m, ok := val.(Model); ok {
		return m.TableName()
	}
	t := reflect.TypeOf(val)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return strings.ToLower(t.Name()) + "s"
}

// hasField checks if a reflection Type contains a struct field named name (or matching its lowercase/db tags).
func hasField(t reflect.Type, name string) bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Name == name {
			return true
		}
		dbTag := field.Tag.Get("db")
		if dbTag == strings.ToLower(name) || dbTag == name {
			return true
		}
	}
	return false
}
