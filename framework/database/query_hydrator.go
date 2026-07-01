/*
Purpose:
This file manages dynamic data translation, structural inspection,
and relational row hydration for the GoStack framework ecosystem.

Philosophy:
By leveraging Go's runtime reflection engine ('reflect'), this utility parses
custom 'db' struct tags to reconcile data positioning. This allows the framework
to populate data layouts without forcing developers to write manual, error-prone
'rows.Scan' loops for every entity model.

Architecture:
It acts as the hydration helper functions within the database package.

Choice:
Consolidated into the database package to avoid circular references and keep all mapping
operations in a single package context.

Implementation:
- Hydrate: inspects raw relational database rows and dynamically maps their column values.
- mapRowToStruct: matches database columns with structural fields to assign values.
- findFieldByTagName: matches database columns with struct tags or structural exact field name matches.
*/
package database

import (
	"database/sql"
	"fmt"
	"reflect"
	"time"
)

// Hydrate inspects raw relational database rows and dynamically maps their 
// column values directly into a provided Go struct or slice pointer target.
func Hydrate(rows *sql.Rows, dest any) error {
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.IsNil() {
		return fmt.Errorf("[GoStack Hydrator] Hydration target must be a valid, non-nil pointer address")
	}

	destIndirect := reflect.Indirect(destValue)
	destType := destIndirect.Type()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("[GoStack Hydrator] Failed to read database column signatures from cursor stream: %w", err)
	}

	scanArgs := make([]any, len(columns))
	values := make([]sql.RawBytes, len(columns))
	for i := range values {
		scanArgs[i] = &values[i]
	}

	switch destType.Kind() {
	case reflect.Slice:
		sliceElementType := destType.Elem()
		isPointerSlice := sliceElementType.Kind() == reflect.Ptr
		
		structType := sliceElementType
		if isPointerSlice {
			structType = sliceElementType.Elem()
		}

		if structType.Kind() != reflect.Struct {
			return fmt.Errorf("[GoStack Hydrator] Target collection elements must be composed of explicit structural types")
		}

		for rows.Next() {
			if err := rows.Scan(scanArgs...); err != nil {
				return fmt.Errorf("[GoStack Hydrator] Row binary scan extraction layer crash: %w", err)
			}

			newStructValue := reflect.New(structType).Elem()
			
			if err := mapRowToStruct(columns, values, newStructValue); err != nil {
				return err
			}

			if isPointerSlice {
				destIndirect.Set(reflect.Append(destIndirect, newStructValue.Addr()))
			} else {
				destIndirect.Set(reflect.Append(destIndirect, newStructValue))
			}
		}

	case reflect.Struct:
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return fmt.Errorf("[GoStack Hydrator] Cursor failure prior to row parsing pass: %w", err)
			}
			return sql.ErrNoRows
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("[GoStack Hydrator] Single row scanner translation layer crash: %w", err)
		}

		if err := mapRowToStruct(columns, values, destIndirect); err != nil {
			return err
		}

	default:
		return fmt.Errorf("[GoStack Hydrator] Unsupported runtime target kind: %s. Target must be a struct or structural slice array", destType.Kind())
	}

	return rows.Err()
}

func mapRowToStruct(columns []string, values []sql.RawBytes, structVal reflect.Value) error {
	for i, colName := range columns {
		rawValue := values[i]
		if rawValue == nil {
			continue
		}

		targetField := findFieldByTagName(structVal, colName)
		if !targetField.IsValid() || !targetField.CanSet() {
			continue
		}

		switch targetField.Kind() {
		case reflect.String:
			targetField.SetString(string(rawValue))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			var val int64
			if _, err := fmt.Sscanf(string(rawValue), "%d", &val); err == nil {
				targetField.SetInt(val)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			var val uint64
			if _, err := fmt.Sscanf(string(rawValue), "%d", &val); err == nil {
				targetField.SetUint(val)
			}
		case reflect.Float32, reflect.Float64:
			var val float64
			if _, err := fmt.Sscanf(string(rawValue), "%f", &val); err == nil {
				targetField.SetFloat(val)
			}
		case reflect.Bool:
			targetField.SetBool(string(rawValue) == "1" || string(rawValue) == "true")
		case reflect.Struct:
			typeName := targetField.Type().String()
			if typeName == "time.Time" {
				str := string(rawValue)
				var t time.Time
				var err error
				formats := []string{
					"2006-01-02 15:04:05",
					time.RFC3339,
					"2006-01-02",
				}
				for _, fmtStr := range formats {
					t, err = time.Parse(fmtStr, str)
					if err == nil {
						break
					}
				}
				if err == nil {
					targetField.Set(reflect.ValueOf(t))
				}
			} else if typeName == "sql.NullString" {
				targetField.FieldByName("String").SetString(string(rawValue))
				targetField.FieldByName("Valid").SetBool(true)
			} else if typeName == "sql.NullInt64" {
				var val int64
				if _, err := fmt.Sscanf(string(rawValue), "%d", &val); err == nil {
					targetField.FieldByName("Int64").SetInt(val)
					targetField.FieldByName("Valid").SetBool(true)
				}
			} else if typeName == "sql.NullFloat64" {
				var val float64
				if _, err := fmt.Sscanf(string(rawValue), "%f", &val); err == nil {
					targetField.FieldByName("Float64").SetFloat(val)
					targetField.FieldByName("Valid").SetBool(true)
				}
			} else if typeName == "sql.NullBool" {
				b := string(rawValue) == "1" || string(rawValue) == "true"
				targetField.FieldByName("Bool").SetBool(b)
				targetField.FieldByName("Valid").SetBool(true)
			} else if typeName == "sql.NullTime" {
				str := string(rawValue)
				var t time.Time
				var err error
				formats := []string{
					"2006-01-02 15:04:05",
					time.RFC3339,
					"2006-01-02",
				}
				for _, fmtStr := range formats {
					t, err = time.Parse(fmtStr, str)
					if err == nil {
						break
					}
				}
				if err == nil {
					targetField.FieldByName("Time").Set(reflect.ValueOf(t))
					targetField.FieldByName("Valid").SetBool(true)
				}
			}
		}
	}
	return nil
}

func findFieldByTagName(structVal reflect.Value, columnName string) reflect.Value {
	t := structVal.Type()
	for i := 0; i < structVal.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("db")
		if tag == columnName {
			return structVal.Field(i)
		}
	}
	return structVal.FieldByName(columnName)
}
