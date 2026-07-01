/*
Purpose:
This file implements the dynamic reflection-based ORM Relationship Management system
for the GoStack framework. It supports eager loading for HasOne, HasMany, BelongsTo,
ManyToMany, and HasManyThrough relations with recursive nested eager loading.

Philosophy:
We believe relationship loading should be convenient, transparent, and highly performant.
Rather than inducing N+1 database querying loops, we batch load related entities using single sub-queries.
We favor runtime structural tag analysis (db, rel, fk, table) to remain zero-dependency.

Architecture:
Post-hydration, the query builder routes structural destination targets to the eager-loader.
The loader inspects struct field definitions, queries children/owners using WhereIn sub-queries,
and reflective maps hydrated relation structures back onto parent models using normalized key comparisons.
*/
package database

import (
	"database/sql"
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
	if destIndirect.Len() == 0 {
		return nil
	}

	// Group and parse nested dot-notation relations (e.g., "User.Profile", "User.Posts")
	relationsMap := make(map[string][]string)
	for _, rel := range qb.relations {
		parts := strings.SplitN(rel, ".", 2)
		top := parts[0]
		if len(parts) > 1 {
			relationsMap[top] = append(relationsMap[top], parts[1])
		} else {
			if _, exists := relationsMap[top]; !exists {
				relationsMap[top] = nil
			}
		}
	}

	var sliceVal reflect.Value
	if destIndirect.Kind() == reflect.Slice {
		sliceVal = destIndirect
	} else if destIndirect.Kind() == reflect.Struct {
		sliceVal = reflect.New(reflect.SliceOf(destIndirect.Type())).Elem()
		sliceVal = reflect.Append(sliceVal, destIndirect)
	} else {
		return fmt.Errorf("[GoStack Relation Engine] Unsupported eager load target kind: %s. Target must be struct or slice", destIndirect.Kind())
	}

	for topLevelName, nestedPaths := range relationsMap {
		if err := qb.eagerLoadRelationForSlice(sliceVal, topLevelName, nestedPaths); err != nil {
			return err
		}
	}

	if destIndirect.Kind() == reflect.Struct {
		destIndirect.Set(sliceVal.Index(0))
	}

	return nil
}

func (qb *QueryBuilder) eagerLoadRelationForSlice(sliceVal reflect.Value, relationName string, nestedPaths []string) error {
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
	case "has_one":
		return qb.loadHasOne(sliceVal, relationName, fkTag, tableTag, relField, nestedPaths)
	case "has_many":
		return qb.loadHasMany(sliceVal, relationName, fkTag, tableTag, relField, nestedPaths)
	case "belongs_to":
		return qb.loadBelongsTo(sliceVal, relationName, fkTag, tableTag, relField, nestedPaths)
	case "many_to_many":
		return qb.loadManyToMany(sliceVal, relationName, fkTag, tableTag, relField, nestedPaths)
	case "has_many_through":
		return qb.loadHasManyThrough(sliceVal, relationName, fkTag, tableTag, relField, nestedPaths)
	default:
		return fmt.Errorf("[GoStack Relation Engine] Unsupported relationship type '%s' on field %s", relTag, relationName)
	}
}

func (qb *QueryBuilder) recursiveEagerLoad(childSliceVal reflect.Value, nestedPaths []string) error {
	if len(nestedPaths) == 0 || childSliceVal.Len() == 0 {
		return nil
	}
	childQb := New(qb.db, "")
	if qb.tx != nil {
		childQb.WithTx(qb.tx)
	}
	childQb.relations = nestedPaths

	slicePtr := reflect.New(childSliceVal.Type())
	slicePtr.Elem().Set(childSliceVal)

	if err := childQb.eagerLoadRelations(slicePtr.Interface()); err != nil {
		return err
	}
	return nil
}

func (qb *QueryBuilder) loadHasOne(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField, nestedPaths []string) error {
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
	seenValues := make(map[string]bool)
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

	childType := relField.Type
	structChildType := childType
	if childType.Kind() == reflect.Ptr {
		structChildType = childType.Elem()
	}

	childSliceType := reflect.SliceOf(childType)
	childSlicePtr := reflect.New(childSliceType)
	childQb := New(qb.db, tableTag)
	if qb.tx != nil {
		childQb.WithTx(qb.tx)
	}
	childQb.WhereIn(fkTag, pkValues)
	if err := childQb.Get(childSlicePtr.Interface()); err != nil {
		return err
	}
	childSlice := childSlicePtr.Elem()

	if err := qb.recursiveEagerLoad(childSlice, nestedPaths); err != nil {
		return err
	}

	childByParent := make(map[string]reflect.Value)
	for i := 0; i < childSlice.Len(); i++ {
		childVal := childSlice.Index(i)
		childStructVal := childVal
		if childVal.Kind() == reflect.Ptr {
			childStructVal = childVal.Elem()
		}
		fkVal := childStructVal.FieldByName(childStructVal.Type().Field(0).Name).Interface()
		if fName, err := findFieldByDBTag(structChildType, fkTag); err == nil {
			fkVal = childStructVal.FieldByName(fName).Interface()
		}
		normFKVal := normalizeKey(fkVal)
		if _, exists := childByParent[normFKVal]; !exists {
			childByParent[normFKVal] = childVal
		}
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
			if child, exists := childByParent[normPKVal]; exists {
				targetFieldVal.Set(child)
			}
		}
	}

	return nil
}

func (qb *QueryBuilder) loadHasMany(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField, nestedPaths []string) error {
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
	seenValues := make(map[string]bool)
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
	if qb.tx != nil {
		childQb.WithTx(qb.tx)
	}
	childQb.WhereIn(fkTag, pkValues)
	if err := childQb.Get(childSlicePtr.Interface()); err != nil {
		return err
	}
	childSlice := childSlicePtr.Elem()

	if err := qb.recursiveEagerLoad(childSlice, nestedPaths); err != nil {
		return err
	}

	childrenByParent := make(map[string]reflect.Value)
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

func (qb *QueryBuilder) loadBelongsTo(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField, nestedPaths []string) error {
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
	seenValues := make(map[string]bool)
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
	if qb.tx != nil {
		ownerQb.WithTx(qb.tx)
	}
	ownerQb.WhereIn(ownerPKColumn, fkValues)
	if err := ownerQb.Get(ownerSlicePtr.Interface()); err != nil {
		return err
	}
	ownerSlice := ownerSlicePtr.Elem()

	if err := qb.recursiveEagerLoad(ownerSlice, nestedPaths); err != nil {
		return err
	}

	ownersByPK := make(map[string]reflect.Value)
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

func (qb *QueryBuilder) loadManyToMany(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField, nestedPaths []string) error {
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
	seenValues := make(map[string]bool)
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

	pivotTable := relField.Tag.Get("pivot")
	if pivotTable == "" {
		return fmt.Errorf("[GoStack Relation Engine] many_to_many relation field %s is missing the 'pivot' tag", relationName)
	}
	parentFK := relField.Tag.Get("fk")
	if parentFK == "" {
		parentFK = strings.ToLower(parentType.Name()) + "_id"
	}
	relatedFK := relField.Tag.Get("related_fk")
	if relatedFK == "" {
		relatedFK = relField.Tag.Get("pivot_fk")
	}
	if relatedFK == "" {
		childType := relField.Type
		if childType.Kind() == reflect.Slice {
			childType = childType.Elem()
		}
		if childType.Kind() == reflect.Ptr {
			childType = childType.Elem()
		}
		relatedFK = strings.ToLower(childType.Name()) + "_id"
	}

	var placeholders []string
	var bindings []any
	drv := qb.db.Driver()
	for _, v := range pkValues {
		if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(bindings)+1))
		} else {
			placeholders = append(placeholders, "?")
		}
		bindings = append(bindings, v)
	}
	query := fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s IN (%s)", parentFK, relatedFK, pivotTable, parentFK, strings.Join(placeholders, ", "))

	result, err := qb.executor().Query(query, bindings...)
	if err != nil {
		return err
	}
	rows, ok := result.(*sql.Rows)
	if !ok {
		return fmt.Errorf("invalid rows type from pivot query")
	}
	defer rows.Close()

	type pivotPair struct {
		parentKey  string
		relatedKey string
	}
	var pairs []pivotPair
	var relatedKeys []any
	seenRelated := make(map[string]bool)

	for rows.Next() {
		var pVal, rVal any
		if err := rows.Scan(&pVal, &rVal); err != nil {
			return err
		}
		pStr := normalizeKey(pVal)
		rStr := normalizeKey(rVal)
		pairs = append(pairs, pivotPair{parentKey: pStr, relatedKey: rStr})
		if !seenRelated[rStr] {
			seenRelated[rStr] = true
			relatedKeys = append(relatedKeys, rVal)
		}
	}

	if len(relatedKeys) == 0 {
		return nil
	}

	childSliceType := relField.Type
	if childSliceType.Kind() != reflect.Slice {
		return fmt.Errorf("[GoStack Relation Engine] Field %s for 'many_to_many' relationship must be a slice", relationName)
	}
	childElemType := childSliceType.Elem()
	structChildType := childElemType
	if childElemType.Kind() == reflect.Ptr {
		structChildType = childElemType.Elem()
	}

	childPKField, err := findPrimaryKeyField(structChildType)
	if err != nil {
		return err
	}

	childPKStructField, _ := structChildType.FieldByName(childPKField)
	childPKColumn := childPKStructField.Tag.Get("db")
	if childPKColumn == "" {
		childPKColumn = "id"
	}

	childSlicePtr := reflect.New(childSliceType)
	childQb := New(qb.db, tableTag)
	if qb.tx != nil {
		childQb.WithTx(qb.tx)
	}
	childQb.WhereIn(childPKColumn, relatedKeys)
	if err := childQb.Get(childSlicePtr.Interface()); err != nil {
		return err
	}
	childSlice := childSlicePtr.Elem()

	if err := qb.recursiveEagerLoad(childSlice, nestedPaths); err != nil {
		return err
	}

	childrenByPK := make(map[string]reflect.Value)
	for i := 0; i < childSlice.Len(); i++ {
		childVal := childSlice.Index(i)
		childStructVal := childVal
		if childVal.Kind() == reflect.Ptr {
			childStructVal = childVal.Elem()
		}
		pkVal := childStructVal.FieldByName(childPKField).Interface()
		normPKVal := normalizeKey(pkVal)
		childrenByPK[normPKVal] = childVal
	}

	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		parentStructVal := parentVal
		if parentVal.Kind() == reflect.Ptr {
			parentStructVal = parentVal.Elem()
		}
		pkVal := parentStructVal.FieldByName(pkField).Interface()
		normPKVal := normalizeKey(pkVal)

		parentSlice := reflect.MakeSlice(childSliceType, 0, 0)
		for _, pair := range pairs {
			if pair.parentKey == normPKVal {
				if childVal, exists := childrenByPK[pair.relatedKey]; exists {
					parentSlice = reflect.Append(parentSlice, childVal)
				}
			}
		}

		targetFieldVal := parentStructVal.FieldByName(relationName)
		if targetFieldVal.CanSet() {
			targetFieldVal.Set(parentSlice)
		}
	}

	return nil
}

func (qb *QueryBuilder) loadHasManyThrough(sliceVal reflect.Value, relationName, fkTag, tableTag string, relField reflect.StructField, nestedPaths []string) error {
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
	seenValues := make(map[string]bool)
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

	throughTable := relField.Tag.Get("through")
	if throughTable == "" {
		return fmt.Errorf("[GoStack Relation Engine] has_many_through relation field %s is missing the 'through' tag", relationName)
	}
	parentFK := relField.Tag.Get("fk")
	if parentFK == "" {
		parentFK = strings.ToLower(parentType.Name()) + "_id"
	}
	farFK := relField.Tag.Get("through_fk")
	if farFK == "" {
		farFK = relField.Tag.Get("far_fk")
	}
	if farFK == "" {
		singularThrough := strings.TrimSuffix(throughTable, "s")
		farFK = singularThrough + "_id"
	}

	var placeholders []string
	var bindings []any
	drv := qb.db.Driver()
	for _, v := range pkValues {
		if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(bindings)+1))
		} else {
			placeholders = append(placeholders, "?")
		}
		bindings = append(bindings, v)
	}
	query := fmt.Sprintf("SELECT id, %s FROM %s WHERE %s IN (%s)", parentFK, throughTable, parentFK, strings.Join(placeholders, ", "))

	result, err := qb.executor().Query(query, bindings...)
	if err != nil {
		return err
	}
	rows, ok := result.(*sql.Rows)
	if !ok {
		return fmt.Errorf("invalid rows type from through query")
	}
	defer rows.Close()

	type throughPair struct {
		throughID string
		parentKey string
	}
	var pairs []throughPair
	var throughIDs []any
	seenThrough := make(map[string]bool)

	for rows.Next() {
		var tVal, pVal any
		if err := rows.Scan(&tVal, &pVal); err != nil {
			return err
		}
		tStr := normalizeKey(tVal)
		pStr := normalizeKey(pVal)
		pairs = append(pairs, throughPair{throughID: tStr, parentKey: pStr})
		if !seenThrough[tStr] {
			seenThrough[tStr] = true
			throughIDs = append(throughIDs, tVal)
		}
	}

	if len(throughIDs) == 0 {
		return nil
	}

	childSliceType := relField.Type
	if childSliceType.Kind() != reflect.Slice {
		return fmt.Errorf("[GoStack Relation Engine] Field %s for 'has_many_through' relationship must be a slice", relationName)
	}

	childSlicePtr := reflect.New(childSliceType)
	childQb := New(qb.db, tableTag)
	if qb.tx != nil {
		childQb.WithTx(qb.tx)
	}
	childQb.WhereIn(farFK, throughIDs)
	if err := childQb.Get(childSlicePtr.Interface()); err != nil {
		return err
	}
	childSlice := childSlicePtr.Elem()

	if err := qb.recursiveEagerLoad(childSlice, nestedPaths); err != nil {
		return err
	}

	farByThroughID := make(map[string][]reflect.Value)
	for i := 0; i < childSlice.Len(); i++ {
		childVal := childSlice.Index(i)
		childStructVal := childVal
		if childVal.Kind() == reflect.Ptr {
			childStructVal = childVal.Elem()
		}
		var farFKVal any
		structChildType := childStructVal.Type()
		if fName, err := findFieldByDBTag(structChildType, farFK); err == nil {
			farFKVal = childStructVal.FieldByName(fName).Interface()
		} else {
			farFKVal = childStructVal.FieldByName(structChildType.Field(0).Name).Interface()
		}
		farFKStr := normalizeKey(farFKVal)
		farByThroughID[farFKStr] = append(farByThroughID[farFKStr], childVal)
	}

	for i := 0; i < sliceVal.Len(); i++ {
		parentVal := sliceVal.Index(i)
		parentStructVal := parentVal
		if parentVal.Kind() == reflect.Ptr {
			parentStructVal = parentVal.Elem()
		}
		pkVal := parentStructVal.FieldByName(pkField).Interface()
		normPKVal := normalizeKey(pkVal)

		parentSlice := reflect.MakeSlice(childSliceType, 0, 0)
		for _, pair := range pairs {
			if pair.parentKey == normPKVal {
				if childVals, exists := farByThroughID[pair.throughID]; exists {
					for _, cv := range childVals {
						parentSlice = reflect.Append(parentSlice, cv)
					}
				}
			}
		}

		targetFieldVal := parentStructVal.FieldByName(relationName)
		if targetFieldVal.CanSet() {
			targetFieldVal.Set(parentSlice)
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

func normalizeKey(val any) string {
	if val == nil {
		return ""
	}
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	return fmt.Sprintf("%v", v.Interface())
}
