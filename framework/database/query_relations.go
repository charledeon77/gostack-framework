/*
Purpose:
This file implements the dynamic reflection-based ORM Relationship Management system
for the GoStack framework. It currently supports eager loading for HasMany and BelongsTo relations.

Philosophy:
We believe relationship loading should be convenient, transparent, and highly performant.
Rather than inducing N+1 database querying loops, we batch load related entities using single sub-queries.
We favor runtime structural tag analysis (db, rel, fk, table) to remain zero-dependency.

Architecture:
Post-hydration, the query builder routes structural destination targets to the eager-loader.
The loader inspects struct field definitions, queries children/owners using WhereIn sub-queries,
and reflective maps hydrated relation structures back onto parent models using normalized key comparisons.

Choice:
We chose runtime reflection over code generation to maximize developer ergonomics
and avoid introducing CLI build steps into simple application templates.

Implementation:
- eagerLoadRelations: root entry point that routes structs and slices to the slice-loader.
- eagerLoadRelationForSlice: extracts field tags, defaults tables/keys, and triggers loading.
- loadHasMany: eager loads HasMany relationships by mapping parent PKs to child FKs.
- loadBelongsTo: eager loads BelongsTo relationships by mapping parent FKs to owner PKs.
- findPrimaryKeyField: traverses fields to identify the struct's primary key name.
- findFieldByDBTag: matches database column strings with struct field targets.
- normalizeKey: coerces numeric and string kinds into comparable types for map lookup.
*/
package database

import (
	"fmt"
	"reflect"
	"strings"
)

func (qb *QueryBuilder) eagerLoadRelations(dest any) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.IsNil() {
		return fmt.Errorf("[GoStack Relation Engine] Eager load destination must be a non-nil pointer")
	}

	destIndirect := reflect.Indirect(destVal)
	if destIndirect.Kind() == reflect.Slice {
		if destIndirect.Len() == 0 {
			return nil
		}
		for _, relationName := range qb.relations {
			if err := qb.eagerLoadRelationForSlice(destIndirect, relationName); err != nil {
				return err
			}
		}
	} else if destIndirect.Kind() == reflect.Struct {
		sliceVal := reflect.New(reflect.SliceOf(destIndirect.Type())).Elem()
		sliceVal = reflect.Append(sliceVal, destIndirect)
		for _, relationName := range qb.relations {
			if err := qb.eagerLoadRelationForSlice(sliceVal, relationName); err != nil {
				return err
			}
		}
		destIndirect.Set(sliceVal.Index(0))
	} else {
		return fmt.Errorf("[GoStack Relation Engine] Unsupported eager load target kind: %s. Target must be struct or slice", destIndirect.Kind())
	}

	return nil
}

func (qb *QueryBuilder) eagerLoadRelationForSlice(sliceVal reflect.Value, relationName string) error {
	firstElem := sliceVal.Index(0)
	parentType := firstElem.Type()
	if parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}

	relField, exists := parentType.FieldByName(relationName)
	if !exists {
		return fmt.Errorf("[GoStack Relation Engine] Field %s does not exist on struct %s", relationName, parentType.Name())
	}

	relTag := relField.Tag.Get("rel")
	if relTag == "" {
		return fmt.Errorf("[GoStack Relation Engine] Field %s on %s is missing the 'rel' tag", relationName, parentType.Name())
	}

	fkTag := relField.Tag.Get("fk")
	if fkTag == "" {
		fkTag = strings.ToLower(parentType.Name()) + "_id"
	}

	tableTag := relField.Tag.Get("table")
	if tableTag == "" {
		childType := relField.Type
		if childType.Kind() == reflect.Slice {
			childType = childType.Elem()
		}
		if childType.Kind() == reflect.Ptr {
			childType = childType.Elem()
		}
		tableTag = strings.ToLower(childType.Name()) + "s"
	}

	switch relTag {
	case "has_many":
		return qb.loadHasMany(sliceVal, relationName, fkTag, tableTag, relField)
	case "belongs_to":
		return qb.loadBelongsTo(sliceVal, relationName, fkTag, tableTag, relField)
	default:
		return fmt.Errorf("[GoStack Relation Engine] Unsupported relationship type '%s' on field %s", relTag, relationName)
	}
}

func (qb *QueryBuilder) loadHasMany(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField) error {
	firstElem := sliceVal.Index(0)
	parentType := firstElem.Type()
	if parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}

	pkField, err := findPrimaryKeyField(parentType)
	if err != nil {
		return err
	}

	var pkValues []any
	seenValues := make(map[any]bool)
	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		if parentVal.Kind() == reflect.Ptr {
			parentVal = parentVal.Elem()
		}
		pkVal := parentVal.FieldByName(pkField).Interface()
		normVal := normalizeKey(pkVal)
		if !seenValues[normVal] {
			seenValues[normVal] = true
			pkValues = append(pkValues, pkVal)
		}
	}

	if len(pkValues) == 0 {
		return nil
	}

	childSliceType := relField.Type
	if childSliceType.Kind() != reflect.Slice {
		return fmt.Errorf("[GoStack Relation Engine] Field %s for 'has_many' relationship must be a slice", relationName)
	}
	childElemType := childSliceType.Elem()
	structChildType := childElemType
	if childElemType.Kind() == reflect.Ptr {
		structChildType = childElemType.Elem()
	}

	childFKField, err := findFieldByDBTag(structChildType, fkTag)
	if err != nil {
		return fmt.Errorf("[GoStack Relation Engine] Could not map FK '%s' to struct field on %s: %w", fkTag, structChildType.Name(), err)
	}

	childSlicePtr := reflect.New(childSliceType)
	childQb := New(qb.db, tableTag)
	childQb.WhereIn(fkTag, pkValues)
	if err := childQb.Get(childSlicePtr.Interface()); err != nil {
		return err
	}
	childSlice := childSlicePtr.Elem()

	childrenByParent := make(map[any]reflect.Value)
	for i := 0; i < childSlice.Len(); i++ {
		childVal := childSlice.Index(i)
		childStructVal := childVal
		if childVal.Kind() == reflect.Ptr {
			childStructVal = childVal.Elem()
		}
		fkVal := childStructVal.FieldByName(childFKField).Interface()
		normFKVal := normalizeKey(fkVal)
		parentSlice, exists := childrenByParent[normFKVal]
		if !exists {
			parentSlice = reflect.MakeSlice(childSliceType, 0, 0)
		}
		parentSlice = reflect.Append(parentSlice, childVal)
		childrenByParent[normFKVal] = parentSlice
	}

	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		parentStructVal := parentVal
		if parentVal.Kind() == reflect.Ptr {
			parentStructVal = parentVal.Elem()
		}
		pkVal := parentStructVal.FieldByName(pkField).Interface()
		normPKVal := normalizeKey(pkVal)
		targetFieldVal := parentStructVal.FieldByName(relationName)
		if targetFieldVal.CanSet() {
			if children, exists := childrenByParent[normPKVal]; exists {
				targetFieldVal.Set(children)
			} else {
				targetFieldVal.Set(reflect.MakeSlice(childSliceType, 0, 0))
			}
		}
	}

	return nil
}

func (qb *QueryBuilder) loadBelongsTo(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField) error {
	firstElem := sliceVal.Index(0)
	parentType := firstElem.Type()
	if parentType.Kind() == reflect.Ptr {
		parentType = parentType.Elem()
	}

	parentFKField, err := findFieldByDBTag(parentType, fkTag)
	if err != nil {
		return fmt.Errorf("[GoStack Relation Engine] Could not map FK '%s' to struct field on parent %s: %w", fkTag, parentType.Name(), err)
	}

	var fkValues []any
	seenValues := make(map[any]bool)
	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		if parentVal.Kind() == reflect.Ptr {
			parentVal = parentVal.Elem()
		}
		fkVal := parentVal.FieldByName(parentFKField).Interface()
		if fkVal == nil || fkVal == 0 || fkVal == "" {
			continue
		}
		normVal := normalizeKey(fkVal)
		if !seenValues[normVal] {
			seenValues[normVal] = true
			fkValues = append(fkValues, fkVal)
		}
	}

	if len(fkValues) == 0 {
		return nil
	}

	ownerType := relField.Type
	structOwnerType := ownerType
	if ownerType.Kind() == reflect.Ptr {
		structOwnerType = ownerType.Elem()
	}

	ownerPKField, err := findPrimaryKeyField(structOwnerType)
	if err != nil {
		return err
	}

	ownerPKStructField, _ := structOwnerType.FieldByName(ownerPKField)
	ownerPKColumn := ownerPKStructField.Tag.Get("db")
	if ownerPKColumn == "" {
		ownerPKColumn = "id"
	}

	ownerSliceType := reflect.SliceOf(ownerType)
	ownerSlicePtr := reflect.New(ownerSliceType)
	ownerQb := New(qb.db, tableTag)
	ownerQb.WhereIn(ownerPKColumn, fkValues)
	if err := ownerQb.Get(ownerSlicePtr.Interface()); err != nil {
		return err
	}
	ownerSlice := ownerSlicePtr.Elem()

	ownersByPK := make(map[any]reflect.Value)
	for i := 0; i < ownerSlice.Len(); i++ {
		ownerVal := ownerSlice.Index(i)
		ownerStructVal := ownerVal
		if ownerVal.Kind() == reflect.Ptr {
			ownerStructVal = ownerVal.Elem()
		}
		pkVal := ownerStructVal.FieldByName(ownerPKField).Interface()
		normPKVal := normalizeKey(pkVal)
		ownersByPK[normPKVal] = ownerVal
	}

	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		parentStructVal := parentVal
		if parentVal.Kind() == reflect.Ptr {
			parentStructVal = parentVal.Elem()
		}
		fkVal := parentStructVal.FieldByName(parentFKField).Interface()
		normFKVal := normalizeKey(fkVal)
		targetFieldVal := parentStructVal.FieldByName(relationName)
		if targetFieldVal.CanSet() {
			if owner, exists := ownersByPK[normFKVal]; exists {
				targetFieldVal.Set(owner)
			}
		}
	}

	return nil
}

func findPrimaryKeyField(t reflect.Type) (string, error) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("db") == "id" {
			return field.Name, nil
		}
	}
	if _, exists := t.FieldByName("ID"); exists {
		return "ID", nil
	}
	return "", fmt.Errorf("[GoStack Relation Engine] Struct %s does not specify an ID field or db:\"id\" tag", t.Name())
}

func findFieldByDBTag(t reflect.Type, tag string) (string, error) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Tag.Get("db") == tag {
			return f.Name, nil
		}
	}
	normalizedTag := strings.ReplaceAll(strings.ToLower(tag), "_", "")
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		normalizedFieldName := strings.ReplaceAll(strings.ToLower(f.Name), "_", "")
		if normalizedFieldName == normalizedTag {
			return f.Name, nil
		}
	}
	return "", fmt.Errorf("[GoStack Relation Engine] Struct field mapping database column '%s' not found", tag)
}

func normalizeKey(val any) any {
	if val == nil {
		return nil
	}
	v := reflect.ValueOf(val)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint())
	case reflect.String:
		return v.String()
	default:
		return val
	}
}
