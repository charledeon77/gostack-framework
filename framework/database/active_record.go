package database

import (
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"reflect"
	"strings"
	"time"
)

// Model defines an optional interface for custom database table name mapping.
type Model interface {
	TableName() string
}

// FillableModel is implemented by any model struct that declares an explicit allowlist (PAR-06).
// Only keys present in Fillable() will be included in Create/Update — all others are silently stripped.
//
//	func (u *User) Fillable() []string { return []string{"name", "email"} }
type FillableModel interface {
	Fillable() []string
}

// GuardedModel is implemented by any model struct that declares an explicit blocklist (PAR-06).
// Keys present in Guarded() are stripped from Create/Update even if they appear in the input map.
// A model with no Fillable and no Guarded is guarded-by-default (all fields are blocked), requiring
// an explicit Fillable() declaration or the use of ForceCreate/ForceUpdate.
//
//	func (u *User) Guarded() []string { return []string{"is_admin", "role"} }
type GuardedModel interface {
	Guarded() []string
}

// filterFields enforces mass-assignment protection for a given model (PAR-06).
// - If the model implements FillableModel: only keys in Fillable() are allowed.
// - Else if the model implements GuardedModel: keys in Guarded() are stripped.
// - Else (no declaration): all fields are blocked (guarded-by-default).
// Use ForceCreate/ForceUpdate for internal trusted writes that bypass this check.
func filterFields(model any, fields map[string]any) (map[string]any, error) {
	if model == nil {
		return fields, nil // no model declaration → caller is using raw QB, no enforcement
	}

	if fm, ok := model.(FillableModel); ok {
		allowed := make(map[string]bool, len(fm.Fillable()))
		for _, f := range fm.Fillable() {
			allowed[f] = true
		}
		filtered := make(map[string]any, len(fields))
		for k, v := range fields {
			if allowed[k] {
				filtered[k] = v
			}
		}
		return filtered, nil
	}

	if gm, ok := model.(GuardedModel); ok {
		blocked := make(map[string]bool, len(gm.Guarded()))
		for _, f := range gm.Guarded() {
			blocked[f] = true
		}
		filtered := make(map[string]any, len(fields))
		for k, v := range fields {
			if !blocked[k] {
				filtered[k] = v
			}
		}
		return filtered, nil
	}

	// Guarded-by-default: neither interface implemented.
	return nil, fmt.Errorf("mass-assignment protection: model %T has no Fillable() or Guarded() declaration; declare Fillable() or use ForceCreate/ForceUpdate for trusted writes", model)
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
// Mass-assignment protection (PAR-06) is enforced: if model T implements FillableModel
// or GuardedModel, fields are filtered accordingly. Use ForceCreate for trusted internal writes.
func Create[T any](db contract.Database, fields map[string]any) (*T, error) {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)

	// PAR-06: Enforce mass-assignment protection.
	filtered, err := filterFields(&model, fields)
	if err != nil {
		return nil, err
	}
	fields = filtered

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

	if err := qb.InsertModel(&model, fields); err != nil {
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

// ForceCreate bypasses mass-assignment protection for trusted internal writes (PAR-06).
func ForceCreate[T any](db contract.Database, fields map[string]any) (*T, error) {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)

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

	if err := qb.InsertModel(&model, fields); err != nil {
		return nil, err
	}

	val := reflect.ValueOf(&model).Elem()
	for k, v := range fields {
		fName, err := findFieldByDBTag(reflect.TypeOf(model), k)
		if err == nil {
			f := val.FieldByName(fName)
			if f.CanSet() {
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
// Mass-assignment protection (PAR-06) is enforced. Use ForceUpdate for trusted internal writes.
func Update[T any](db contract.Database, id any, fields map[string]any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)

	// PAR-06: Enforce mass-assignment protection.
	filtered, err := filterFields(&model, fields)
	if err != nil {
		return err
	}
	fields = filtered

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

// ForceUpdate bypasses mass-assignment protection for trusted internal writes (PAR-06).
func ForceUpdate[T any](db contract.Database, id any, fields map[string]any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)

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

// Restore restores a soft-deleted record by clearing its deleted_at timestamp (DX-01).
func Restore[T any](db contract.Database, id any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	return qb.Where("id", "=", id).Update(map[string]any{
		"deleted_at": nil,
	})
}

// ForceDelete permanently deletes a record from the database even if soft deletes are enabled (DX-01).
func ForceDelete[T any](db contract.Database, id any) error {
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	return qb.Where("id", "=", id).Delete()
}

// WithTrashed retrieves all records for the model type including soft-deleted ones (DX-01).
func WithTrashed[T any](db contract.Database) ([]T, error) {
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

// OnlyTrashed retrieves only soft-deleted records for the model type (DX-01).
func OnlyTrashed[T any](db contract.Database) ([]T, error) {
	var models []T
	var model T
	tableName := getTableName(model)
	qb := New(db, tableName)
	err := qb.WhereNotNull("deleted_at").Get(&models)
	if err != nil {
		return nil, err
	}
	return models, nil
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
