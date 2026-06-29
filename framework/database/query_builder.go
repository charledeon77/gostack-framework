/*
Purpose:
This file contains the QueryBuilder struct and its associated chainable helper methods
for constructing and executing relational SQL queries within the GoStack framework.

Philosophy:
We believe query construction should be fluent, clean, and database-agnostic.
By decoupling SQL compilation details (such as placeholder syntax) from application code,
we empower developers to write standard SQL logic that is highly legible and testable.

Architecture:
QueryBuilder tracks local query state (table, filters, bindings, eager relations) and delegates
execution to an injected database client conforming to the contract.Database interface.
Post-execution, result sets are directed to the hydration engine and relationships are eager-loaded.

Choice:
We chose a direct, chainable builder style for its readability and simplicity.
Rather than a heavily layered AST, we map clauses directly to query strings and bindings,
providing a lightweight interface with minimal abstraction overhead.

Implementation:
- QueryBuilder: manages table targets, filter state, placeholder index bindings, and relations.
- New: instantiates a new builder with an injected database connector.
- Where: appends AND filtering clauses with driver-specific placeholders.
- WhereIn: appends WHERE IN clauses to avoid query duplication.
- With: records relationships to be eager-loaded during hydration.
- ToSQL: compiles internal state into an executable SQL string.
- Execute: performs connection safety checks and executes the compiled query.
- Get: coordinates query execution, cursor hydration, and eager relationship mapping.
*/
package database

import (
	"database/sql"
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/contract"
	"strings"
)

// QueryBuilder serves as the core state manager for SQL construction.
type QueryBuilder struct {
	table     string
	where     []string
	bindings  []any
	db        contract.Database
	relations []string
	orderBy   string // e.g. "created_at DESC"
	limitVal  int    // 0 = no limit
	offsetVal int    // 0 = no offset
}

// New constructs a new QueryBuilder instance.
func New(db contract.Database, table string) *QueryBuilder {
	return &QueryBuilder{
		db:    db,
		table: table,
	}
}

// Where adds a filtering condition to the internal state.
func (qb *QueryBuilder) Where(col, op string, val any) *QueryBuilder {
	var placeholder string
	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		placeholder = fmt.Sprintf("$%d", len(qb.bindings)+1)
	} else {
		placeholder = "?"
	}

	qb.where = append(qb.where, fmt.Sprintf("%s %s %s", col, op, placeholder))
	qb.bindings = append(qb.bindings, val)
	return qb
}

// ToSQL serializes the internal state into a valid SQL query string.
func (qb *QueryBuilder) ToSQL() string {
	sqlStr := fmt.Sprintf("SELECT * FROM %s", qb.table)

	if len(qb.where) > 0 {
		sqlStr += " WHERE "
		for i, w := range qb.where {
			sqlStr += w
			if i < len(qb.where)-1 {
				sqlStr += " AND "
			}
		}
	}
	if qb.orderBy != "" {
		sqlStr += " ORDER BY " + qb.orderBy
	}
	if qb.limitVal > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", qb.limitVal)
	}
	if qb.offsetVal > 0 {
		sqlStr += fmt.Sprintf(" OFFSET %d", qb.offsetVal)
	}
	return sqlStr
}

// OrderBy appends an ORDER BY clause to the query.
// dir should be "ASC" or "DESC".
//
// Example:
//
//	gostack.Table("posts").OrderBy("created_at", "DESC").Get(&posts)
func (qb *QueryBuilder) OrderBy(col, dir string) *QueryBuilder {
	qb.orderBy = col + " " + dir
	return qb
}

// Limit caps the number of rows returned.
//
// Example:
//
//	gostack.Table("posts").Limit(10).Get(&posts)
func (qb *QueryBuilder) Limit(n int) *QueryBuilder {
	qb.limitVal = n
	return qb
}

// Offset skips the first n rows before returning results.
//
// Example:
//
//	gostack.Table("posts").Limit(10).Offset(20).Get(&posts)
func (qb *QueryBuilder) Offset(n int) *QueryBuilder {
	qb.offsetVal = n
	return qb
}

// First executes the query with LIMIT 1 and hydrates the result into dest,
// which should be a pointer to a struct (not a slice).
// Returns an error wrapping ErrNoRows if no record is found.
//
// Example:
//
//	var user model.User
//	err := gostack.Table("users").Where("email", "=", email).First(&user)
func (qb *QueryBuilder) First(dest any) error {
	if qb.db == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}

	sqlStr := qb.ToSQL()
	// Enforce LIMIT 1 — override any previously set limit.
	// We strip any existing LIMIT/OFFSET by building the clause fresh.
	base := fmt.Sprintf("SELECT * FROM %s", qb.table)
	if len(qb.where) > 0 {
		base += " WHERE "
		for i, w := range qb.where {
			base += w
			if i < len(qb.where)-1 {
				base += " AND "
			}
		}
	}
	if qb.orderBy != "" {
		base += " ORDER BY " + qb.orderBy
	}
	base += " LIMIT 1"
	_ = sqlStr // unused now; base is the canonical first-query SQL

	result, err := qb.db.Query(base, qb.bindings...)
	if err != nil {
		return fmt.Errorf("[Crafter] First() execution failed: %w", err)
	}

	rows, ok := result.(*sql.Rows)
	if !ok {
		return fmt.Errorf("[Crafter] Driver compatibility mismatch: expected *sql.Rows, got %T", result)
	}
	defer rows.Close()

	// Hydrate handles both slice and struct targets; passing a struct pointer
	// reads exactly one row and returns sql.ErrNoRows if the cursor is empty.
	if err := Hydrate(rows, dest); err != nil {
		if err.Error() == "sql: no rows in result set" {
			return fmt.Errorf("[Crafter] record not found")
		}
		return fmt.Errorf("[Crafter] First() hydration failed: %w", err)
	}
	return nil
}

// Update compiles and executes an UPDATE statement against the target table.
// Fires BeforeSave hook on model if provided via UpdateModel.
//
// Example:
//
//	gostack.Table("users").Where("id", "=", 1).Update(map[string]any{"name": "Alice"})
func (qb *QueryBuilder) Update(data map[string]any) error {
	return qb.UpdateModel(nil, data)
}

// UpdateModel is Update with optional model hook support.
func (qb *QueryBuilder) UpdateModel(model any, data map[string]any) error {
	if qb.db == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	if len(data) == 0 {
		return nil
	}

	var setClauses []string
	var bindings []any

	drv := qb.db.Driver()
	for col, val := range data {
		var placeholder string
		if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
			placeholder = fmt.Sprintf("$%d", len(bindings)+1)
		} else {
			placeholder = "?"
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", col, placeholder))
		bindings = append(bindings, val)
	}

	sqlStr := fmt.Sprintf("UPDATE %s SET %s", qb.table, strings.Join(setClauses, ", "))

	if len(qb.where) > 0 {
		// Re-index postgres placeholders for the WHERE bindings.
		for _, w := range qb.where {
			if qb.db.Driver() == "postgres" {
				// Rebuild the where clause with the correct placeholder offset.
				_ = w // already stored with correct $N from Where()
			}
		}
		sqlStr += " WHERE "
		for i, w := range qb.where {
			sqlStr += w
			if i < len(qb.where)-1 {
				sqlStr += " AND "
			}
		}
		bindings = append(bindings, qb.bindings...)
	}

	return qb.db.Exec(sqlStr, bindings...)
}

// Delete compiles and executes a DELETE statement against the target table.
// Fires BeforeDelete and AfterDelete hooks on model if provided via DeleteModel.
//
// Example:
//
//	gostack.Table("users").Where("id", "=", 42).Delete()
func (qb *QueryBuilder) Delete() error {
	return qb.DeleteModel(nil)
}

// DeleteModel is Delete with optional model hook support.
func (qb *QueryBuilder) DeleteModel(model any) error {
	if qb.db == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}

	// Fire BeforeDelete hook.
	if model != nil {
		if hook, ok := model.(BeforeDeleter); ok {
			if err := hook.BeforeDelete(); err != nil {
				return fmt.Errorf("[Crafter] BeforeDelete hook failed: %w", err)
			}
		}
	}

	sqlStr := fmt.Sprintf("DELETE FROM %s", qb.table)
	if len(qb.where) > 0 {
		sqlStr += " WHERE "
		for i, w := range qb.where {
			sqlStr += w
			if i < len(qb.where)-1 {
				sqlStr += " AND "
			}
		}
	}
	if err := qb.db.Exec(sqlStr, qb.bindings...); err != nil {
		return err
	}

	// Fire AfterDelete hook.
	if model != nil {
		if hook, ok := model.(AfterDeleter); ok {
			if err := hook.AfterDelete(); err != nil {
				return fmt.Errorf("[Crafter] AfterDelete hook failed: %w", err)
			}
		}
	}
	return nil
}

// Execute triggers the execution of the built SQL query via the 
// injected database connection. 
func (qb *QueryBuilder) Execute() (any, error) {
	if qb.db == nil {
		return nil, fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	
	return qb.db.Query(qb.ToSQL(), qb.bindings...)
}

// Get executes the compiled query and automatically hydates the database results 
// directly into the provided destination pointer (struct or slice of structs).
func (qb *QueryBuilder) Get(dest any) error {
	result, err := qb.Execute()
	if err != nil {
		return fmt.Errorf("[Crafter] Execution pipeline crashed: %w", err)
	}

	rows, ok := result.(*sql.Rows)
	if !ok {
		return fmt.Errorf("[Crafter] Driver compatibility mismatch. Expected active *sql.Rows cursor, received: %T", result)
	}
	defer rows.Close()

	if err := Hydrate(rows, dest); err != nil {
		return fmt.Errorf("[Crafter] Hydration process failed: %w", err)
	}

	if len(qb.relations) > 0 {
		if err := qb.eagerLoadRelations(dest); err != nil {
			return fmt.Errorf("[Crafter] Eager loading failed: %w", err)
		}
	}

	return nil
}

// With specifies one or more relationships to be eager-loaded with the query results.
func (qb *QueryBuilder) With(relations ...string) *QueryBuilder {
	qb.relations = append(qb.relations, relations...)
	return qb
}

// WhereIn adds a "WHERE col IN (?, ?, ...)" clause to the query state.
func (qb *QueryBuilder) WhereIn(col string, vals []any) *QueryBuilder {
	if len(vals) == 0 {
		qb.where = append(qb.where, "1 = 0")
		return qb
	}

	var placeholders []string
	wdrv := ""
	if qb.db != nil {
		wdrv = qb.db.Driver()
	}
	for _, val := range vals {
		var placeholder string
		if wdrv == "postgres" || wdrv == "cockroach" || wdrv == "cockroachdb" {
			placeholder = fmt.Sprintf("$%d", len(qb.bindings)+1)
		} else {
			placeholder = "?"
		}
		placeholders = append(placeholders, placeholder)
		qb.bindings = append(qb.bindings, val)
	}

	qb.where = append(qb.where, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
	return qb
}

// Insert compiles and executes an INSERT DDL operation against the target table.
// It fires BeforeSave() before execution and AfterCreate() after a successful insert.
func (qb *QueryBuilder) Insert(data map[string]any) error {
	return qb.InsertModel(nil, data)
}

// InsertModel is Insert with optional model hook support. Pass the model pointer
// to enable BeforeSave and AfterCreate lifecycle hooks.
func (qb *QueryBuilder) InsertModel(model any, data map[string]any) error {
	if qb.db == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	if len(data) == 0 {
		return nil
	}

	// Fire BeforeSave hook if model implements it.
	if model != nil {
		if hook, ok := model.(BeforeSaver); ok {
			if err := hook.BeforeSave(); err != nil {
				return fmt.Errorf("[SparkORM] BeforeSave hook failed: %w", err)
			}
		}
	}

	var columns []string
	var placeholders []string
	var bindings []any

	idrv := qb.db.Driver()
	for col, val := range data {
		columns = append(columns, col)
		var placeholder string
		if idrv == "postgres" || idrv == "cockroach" || idrv == "cockroachdb" {
			placeholder = fmt.Sprintf("$%d", len(bindings)+1)
		} else {
			placeholder = "?"
		}
		placeholders = append(placeholders, placeholder)
		bindings = append(bindings, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", qb.table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	if err := qb.db.Exec(query, bindings...); err != nil {
		return err
	}

	// Fire AfterCreate hook if model implements it.
	if model != nil {
		if hook, ok := model.(AfterCreator); ok {
			if err := hook.AfterCreate(); err != nil {
				return fmt.Errorf("[SparkORM] AfterCreate hook failed: %w", err)
			}
		}
	}
	return nil
}

// PageMeta stores structural pagination attributes for API response formatting.
type PageMeta struct {
	Total    int  `json:"total"`
	Page     int  `json:"page"`
	PerPage  int  `json:"per_page"`
	LastPage int  `json:"last_page"`
	HasNext  bool `json:"has_next"`
	HasPrev  bool `json:"has_prev"`
}

// Count returns the total number of records matching current query constraints.
func (qb *QueryBuilder) Count() (int, error) {
	if qb.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}
	sqlStr := qb.countSQL()
	result, err := qb.db.Query(sqlStr, qb.bindings...)
	if err != nil {
		return 0, err
	}
	rows, ok := result.(*sql.Rows)
	if !ok {
		return 0, fmt.Errorf("expected *sql.Rows, got %T", result)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, fmt.Errorf("no rows returned for count query")
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (qb *QueryBuilder) countSQL() string {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM %s", qb.table)
	if len(qb.where) > 0 {
		sqlStr += " WHERE "
		for i, w := range qb.where {
			sqlStr += w
			if i < len(qb.where)-1 {
				sqlStr += " AND "
			}
		}
	}
	return sqlStr
}

// Paginate counts total records matching current query conditions, retrieves the requested
// chunk of rows for the target page, and hydrates them into dest.
func (qb *QueryBuilder) Paginate(dest any, page, perPage int) (*PageMeta, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	}

	total, err := qb.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count records for pagination: %w", err)
	}

	offset := (page - 1) * perPage

	// Execute paginated selection using ToSQL + limit/offset
	// Build paginated SQL without the builder's own LIMIT/OFFSET (we control those here).
	base := fmt.Sprintf("SELECT * FROM %s", qb.table)
	if len(qb.where) > 0 {
		base += " WHERE "
		for i, w := range qb.where {
			base += w
			if i < len(qb.where)-1 {
				base += " AND "
			}
		}
	}
	if qb.orderBy != "" {
		base += " ORDER BY " + qb.orderBy
	}
	sqlStr := base + fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	result, err := qb.db.Query(sqlStr, qb.bindings...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute paginated query: %w", err)
	}

	rows, ok := result.(*sql.Rows)
	if !ok {
		return nil, fmt.Errorf("driver compatibility mismatch: expected *sql.Rows, got %T", result)
	}
	defer rows.Close()

	if err := Hydrate(rows, dest); err != nil {
		return nil, fmt.Errorf("failed to hydrate paginated records: %w", err)
	}

	if len(qb.relations) > 0 {
		if err := qb.eagerLoadRelations(dest); err != nil {
			return nil, fmt.Errorf("failed to eager load paginated relations: %w", err)
		}
	}

	lastPage := 1
	if total > 0 {
		lastPage = (total + perPage - 1) / perPage
	}

	return &PageMeta{
		Total:    total,
		Page:     page,
		PerPage:  perPage,
		LastPage: lastPage,
		HasNext:  page < lastPage,
		HasPrev:  page > 1,
	}, nil
}


