package database

import (
	"github.com/charledeon77/gostack/framework/contract"
	"reflect"
	"strings"
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
	return qb.Where("id", "=", id).Update(fields)
}

// DeleteRecord deletes records matching the primary key id.
func DeleteRecord[T any](db contract.Database, id any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
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
