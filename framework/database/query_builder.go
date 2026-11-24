/*
Crafter — GoStack Relational Query Builder & Active Record Subsystem.

Naming Metaphor: Why Crafter?
The GoStack Relational Query Builder was named Crafter because an artisan "crafter" meticulously
shapes raw materials into refined, purposeful objects. Crafter takes raw clauses, criteria, and joins,
and meticulously sculpts them into safe, parameterized, dialect-optimized SQL queries without
exposing the developer to boilerplate string concatenation or injection hazards.

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
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

var pgPlaceholderRegex = regexp.MustCompile(`\$(\d+)`)

// ErrNotFound is returned by First() when no matching row exists in the database.
var ErrNotFound = errors.New("record not found")

func shiftPgPlaceholders(sql string, offset int) string {
	if offset == 0 {
		return sql
	}
	return pgPlaceholderRegex.ReplaceAllStringFunc(sql, func(m string) string {
		numStr := m[1:]
		var n int
		fmt.Sscanf(numStr, "%d", &n)
		return fmt.Sprintf("$%d", n+offset)
	})
}

var (
	identRegex  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	tableRegex  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	columnRegex = regexp.MustCompile(`^(?:(?:[A-Za-z_][A-Za-z0-9_]*|\*|(?:[A-Za-z_][A-Za-z0-9_]*)\.(?:[A-Za-z_][A-Za-z0-9_]*|\*))|(?:COUNT|SUM|AVG|MIN|MAX)\((?:[A-Za-z_][A-Za-z0-9_]*|\*|(?:[A-Za-z_][A-Za-z0-9_]*)\.[A-Za-z_][A-Za-z0-9_]*)\))(?:\s+(?i:as)\s+[A-Za-z_][A-Za-z0-9_]*)?$`)
)

func isValidOperator(op string) bool {
	switch strings.ToUpper(strings.TrimSpace(op)) {
	case "=", "!=", "<>", ">", ">=", "<", "<=", "LIKE", "NOT LIKE", "ILIKE", "NOT ILIKE", "IS", "IS NOT", "IN", "NOT IN":
		return true
	default:
		return false
	}
}

func isValidDirection(dir string) bool {
	switch strings.ToUpper(strings.TrimSpace(dir)) {
	case "ASC", "DESC", "ASC NULLS FIRST", "DESC NULLS LAST":
		return true
	default:
		return false
	}
}

// QueryBuilder serves as the core state manager for SQL construction.
type QueryBuilder struct {
	table           string
	columns         []string
	where           []string
	joins           []string
	bindings        []any
	db              contract.Database
	tx              contract.Tx
	relations       []string
	orderBy         string // e.g. "created_at DESC"
	limitVal        int    // 0 = no limit
	offsetVal       int    // 0 = no offset
	withTrashed     bool
	allowMassMutate bool   // PAR-12: opt-in for mass UPDATE/DELETE without WHERE
	err             error
}

// AllowMassMutation opts in to allowing UPDATE or DELETE without a WHERE clause (PAR-12).
// This is an explicit, dangerous override — by default GoStack rejects mass mutations.
//
//	gostack.Table("staging").AllowMassMutation().Delete()
func (qb *QueryBuilder) AllowMassMutation() *QueryBuilder {
	qb.allowMassMutate = true
	return qb
}

func (qb *QueryBuilder) setErr(err error) {
	if qb.err == nil && err != nil {
		qb.err = err
	}
}

// Err returns the first validation or execution error encountered by the query builder.
func (qb *QueryBuilder) Err() error {
	return qb.err
}

type dbOrTx interface {
	Query(sql string, args ...any) (any, error)
	Exec(sql string, args ...any) error
}

func (qb *QueryBuilder) executor() dbOrTx {
	if qb.tx != nil {
		return qb.tx
	}
	return qb.db
}

var savepointCounter int64

type savepointTx struct {
	parent contract.Tx
	name   string
}

func (s *savepointTx) Exec(sql string, args ...any) error {
	return s.parent.Exec(sql, args...)
}

func (s *savepointTx) ExecContext(ctx context.Context, sql string, args ...any) error {
	return s.parent.ExecContext(ctx, sql, args...)
}

func (s *savepointTx) Query(sql string, args ...any) (any, error) {
	return s.parent.Query(sql, args...)
}

func (s *savepointTx) QueryContext(ctx context.Context, sql string, args ...any) (any, error) {
	return s.parent.QueryContext(ctx, sql, args...)
}

func (s *savepointTx) Commit() error {
	// Committing a savepoint is a no-op at the outer transaction level;
	// the RELEASE SAVEPOINT command is handled in the Transaction wrapper closure.
	return nil
}

func (s *savepointTx) Rollback() error {
	return s.parent.Exec(fmt.Sprintf("ROLLBACK TO SAVEPOINT %s", s.name))
}

// Transaction runs a closure within a database transaction context, automatically
// handling Commit, Rollback, and panic recovery.
//
// It natively supports nested transactions. If the first argument is a contract.Tx, it
// uses database SAVEPOINTs to run a nested sub-transaction. If it is a
// contract.Database, it begins a standard database transaction.
func Transaction(conn any, fn func(tx contract.Tx) error) error {
	if db, ok := conn.(contract.Database); ok {
		tx, err := db.BeginTx()
		if err != nil {
			return err
		}
		defer func() {
			if p := recover(); p != nil {
				_ = tx.Rollback()
				panic(p)
			}
		}()
		if err := fn(tx); err != nil {
			// F-193: capture and surface rollback error if rollback fails
			if rbErr := tx.Rollback(); rbErr != nil {
				return fmt.Errorf("transaction callback failed: %w (rollback failed: %v)", err, rbErr)
			}
			return err
		}
		return tx.Commit()
	}

	if tx, ok := conn.(contract.Tx); ok {
		spName := fmt.Sprintf("sp_%d", atomic.AddInt64(&savepointCounter, 1))
		if err := tx.Exec(fmt.Sprintf("SAVEPOINT %s", spName)); err != nil {
			return err
		}

		sTx := &savepointTx{parent: tx, name: spName}

		defer func() {
			if p := recover(); p != nil {
				_ = sTx.Rollback()
				panic(p)
			}
		}()

		if err := fn(sTx); err != nil {
			// F-193: capture and surface savepoint rollback error
			if rbErr := sTx.Rollback(); rbErr != nil {
				return fmt.Errorf("savepoint callback failed: %w (rollback failed: %v)", err, rbErr)
			}
			return err
		}

		return tx.Exec(fmt.Sprintf("RELEASE SAVEPOINT %s", spName))
	}

	return fmt.Errorf("transaction: unsupported connection type, expected contract.Database or contract.Tx")
}

// New constructs a new QueryBuilder instance.
func New(db contract.Database, table string) *QueryBuilder {
	qb := &QueryBuilder{
		db:    db,
		table: table,
	}
	if !tableRegex.MatchString(table) {
		qb.setErr(fmt.Errorf("[Crafter] invalid table identifier: %q", table))
	}
	return qb
}

// WithTx configures the query builder to run operations inside a transaction.
func (qb *QueryBuilder) WithTx(tx contract.Tx) *QueryBuilder {
	qb.tx = tx
	return qb
}

// Select specifies the columns to be retrieved.
func (qb *QueryBuilder) Select(cols ...string) *QueryBuilder {
	for _, col := range cols {
		if !columnRegex.MatchString(col) {
			qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in Select: %q", col))
			return qb
		}
	}
	qb.columns = cols
	return qb
}

// Where adds a filtering condition to the internal state.
func (qb *QueryBuilder) Where(col, op string, val any) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in Where: %q", col))
		return qb
	}
	if !isValidOperator(op) {
		qb.setErr(fmt.Errorf("[Crafter] invalid operator in Where: %q", op))
		return qb
	}

	upperOp := strings.ToUpper(strings.TrimSpace(op))

	// F-194: handle IS NULL and IS NOT NULL without emitting invalid placeholders
	if (upperOp == "IS" || upperOp == "IS NOT") && val == nil {
		qb.where = append(qb.where, fmt.Sprintf("%s %s NULL", col, upperOp))
		return qb
	}
	if val == nil && upperOp == "=" {
		qb.where = append(qb.where, fmt.Sprintf("%s IS NULL", col))
		return qb
	}
	if val == nil && (upperOp == "!=" || upperOp == "<>") {
		qb.where = append(qb.where, fmt.Sprintf("%s IS NOT NULL", col))
		return qb
	}

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}

	// F-194: expand slice values passed to IN or NOT IN operators
	if upperOp == "IN" || upperOp == "NOT IN" {
		vVal := reflect.ValueOf(val)
		if vVal.Kind() == reflect.Slice {
			sliceLen := vVal.Len()
			if sliceLen == 0 {
				if upperOp == "IN" {
					qb.where = append(qb.where, "1 = 0")
				} else {
					qb.where = append(qb.where, "1 = 1")
				}
				return qb
			}
			var placeholders []string
			for i := 0; i < sliceLen; i++ {
				var ph string
				if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
					ph = fmt.Sprintf("$%d", len(qb.bindings)+1)
				} else {
					ph = "?"
				}
				placeholders = append(placeholders, ph)
				qb.bindings = append(qb.bindings, vVal.Index(i).Interface())
			}
			qb.where = append(qb.where, fmt.Sprintf("%s %s (%s)", col, upperOp, strings.Join(placeholders, ", ")))
			return qb
		}
	}

	var placeholder string
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		placeholder = fmt.Sprintf("$%d", len(qb.bindings)+1)
	} else {
		placeholder = "?"
	}

	qb.where = append(qb.where, fmt.Sprintf("%s %s %s", col, op, placeholder))
	qb.bindings = append(qb.bindings, val)
	return qb
}

// OrWhere adds an OR filtering condition to the query.
func (qb *QueryBuilder) OrWhere(col, op string, val any) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in OrWhere: %q", col))
		return qb
	}
	if !isValidOperator(op) {
		qb.setErr(fmt.Errorf("[Crafter] invalid operator in OrWhere: %q", op))
		return qb
	}

	upperOp := strings.ToUpper(strings.TrimSpace(op))

	// F-194: handle IS NULL and IS NOT NULL for OrWhere
	if (upperOp == "IS" || upperOp == "IS NOT") && val == nil {
		clause := fmt.Sprintf("%s %s NULL", col, upperOp)
		if len(qb.where) > 0 {
			qb.where = append(qb.where, "OR "+clause)
		} else {
			qb.where = append(qb.where, clause)
		}
		return qb
	}
	if val == nil && upperOp == "=" {
		clause := fmt.Sprintf("%s IS NULL", col)
		if len(qb.where) > 0 {
			qb.where = append(qb.where, "OR "+clause)
		} else {
			qb.where = append(qb.where, clause)
		}
		return qb
	}
	if val == nil && (upperOp == "!=" || upperOp == "<>") {
		clause := fmt.Sprintf("%s IS NOT NULL", col)
		if len(qb.where) > 0 {
			qb.where = append(qb.where, "OR "+clause)
		} else {
			qb.where = append(qb.where, clause)
		}
		return qb
	}

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}

	// F-194: expand slice values passed to IN or NOT IN operators in OrWhere
	if upperOp == "IN" || upperOp == "NOT IN" {
		vVal := reflect.ValueOf(val)
		if vVal.Kind() == reflect.Slice {
			sliceLen := vVal.Len()
			if sliceLen == 0 {
				var clause string
				if upperOp == "IN" {
					clause = "1 = 0"
				} else {
					clause = "1 = 1"
				}
				if len(qb.where) > 0 {
					qb.where = append(qb.where, "OR "+clause)
				} else {
					qb.where = append(qb.where, clause)
				}
				return qb
			}
			var placeholders []string
			for i := 0; i < sliceLen; i++ {
				var ph string
				if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
					ph = fmt.Sprintf("$%d", len(qb.bindings)+1)
				} else {
					ph = "?"
				}
				placeholders = append(placeholders, ph)
				qb.bindings = append(qb.bindings, vVal.Index(i).Interface())
			}
			clause := fmt.Sprintf("%s %s (%s)", col, upperOp, strings.Join(placeholders, ", "))
			if len(qb.where) > 0 {
				qb.where = append(qb.where, "OR "+clause)
			} else {
				qb.where = append(qb.where, clause)
			}
			return qb
		}
	}

	var placeholder string
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		placeholder = fmt.Sprintf("$%d", len(qb.bindings)+1)
	} else {
		placeholder = "?"
	}

	clause := fmt.Sprintf("%s %s %s", col, op, placeholder)
	if len(qb.where) > 0 {
		qb.where = append(qb.where, "OR "+clause)
	} else {
		qb.where = append(qb.where, clause)
	}
	qb.bindings = append(qb.bindings, val)
	return qb
}

// WhereGroup evaluates a nested WHERE clause wrapped in parentheses: AND (condition1 OR condition2)
func (qb *QueryBuilder) WhereGroup(fn func(q *QueryBuilder)) *QueryBuilder {
	sub := &QueryBuilder{db: qb.db, tx: qb.tx, table: qb.table}
	fn(sub)
	if sub.err != nil {
		qb.setErr(sub.err)
		return qb
	}
	if len(sub.where) == 0 {
		return qb
	}

	subWhere := sub.whereSQL()
	subWhere = strings.TrimPrefix(subWhere, " WHERE ")

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		offset := len(qb.bindings)
		if offset > 0 {
			subWhere = shiftPgPlaceholders(subWhere, offset)
		}
	}

	clause := fmt.Sprintf("(%s)", subWhere)
	if len(qb.where) > 0 {
		qb.where = append(qb.where, "AND "+clause)
	} else {
		qb.where = append(qb.where, clause)
	}
	qb.bindings = append(qb.bindings, sub.bindings...)
	return qb
}

// OrWhereGroup evaluates a nested WHERE clause wrapped in parentheses preceded by OR: OR (condition1 AND condition2)
func (qb *QueryBuilder) OrWhereGroup(fn func(q *QueryBuilder)) *QueryBuilder {
	sub := &QueryBuilder{db: qb.db, tx: qb.tx, table: qb.table}
	fn(sub)
	if sub.err != nil {
		qb.setErr(sub.err)
		return qb
	}
	if len(sub.where) == 0 {
		return qb
	}

	subWhere := sub.whereSQL()
	subWhere = strings.TrimPrefix(subWhere, " WHERE ")

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		offset := len(qb.bindings)
		if offset > 0 {
			subWhere = shiftPgPlaceholders(subWhere, offset)
		}
	}

	clause := fmt.Sprintf("(%s)", subWhere)
	if len(qb.where) > 0 {
		qb.where = append(qb.where, "OR "+clause)
	} else {
		qb.where = append(qb.where, clause)
	}
	qb.bindings = append(qb.bindings, sub.bindings...)
	return qb
}

// WhereNull adds a condition requiring a column to be NULL.
func (qb *QueryBuilder) WhereNull(col string) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in WhereNull: %q", col))
		return qb
	}
	qb.where = append(qb.where, fmt.Sprintf("%s IS NULL", col))
	return qb
}

// WhereNotNull adds a condition requiring a column to be NOT NULL.
func (qb *QueryBuilder) WhereNotNull(col string) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in WhereNotNull: %q", col))
		return qb
	}
	qb.where = append(qb.where, fmt.Sprintf("%s IS NOT NULL", col))
	return qb
}

// WhereBetween adds a condition requiring a column value to fall between min and max.
func (qb *QueryBuilder) WhereBetween(col string, min, max any) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in WhereBetween: %q", col))
		return qb
	}
	var p1, p2 string
	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		p1 = fmt.Sprintf("$%d", len(qb.bindings)+1)
		p2 = fmt.Sprintf("$%d", len(qb.bindings)+2)
	} else {
		p1 = "?"
		p2 = "?"
	}

	qb.where = append(qb.where, fmt.Sprintf("%s BETWEEN %s AND %s", col, p1, p2))
	qb.bindings = append(qb.bindings, min, max)
	return qb
}

// WhereLike adds a condition requiring a column value to match a pattern.
func (qb *QueryBuilder) WhereLike(col, pattern string) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in WhereLike: %q", col))
		return qb
	}
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

	qb.where = append(qb.where, fmt.Sprintf("%s LIKE %s", col, placeholder))
	qb.bindings = append(qb.bindings, pattern)
	return qb
}

// Join adds an INNER JOIN clause to the query.
func (qb *QueryBuilder) Join(table, first, op, second string) *QueryBuilder {
	if !tableRegex.MatchString(table) {
		qb.setErr(fmt.Errorf("[Crafter] invalid table identifier in Join: %q", table))
		return qb
	}
	if !columnRegex.MatchString(first) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in Join (first): %q", first))
		return qb
	}
	if !columnRegex.MatchString(second) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in Join (second): %q", second))
		return qb
	}
	if !isValidOperator(op) {
		qb.setErr(fmt.Errorf("[Crafter] invalid operator in Join: %q", op))
		return qb
	}
	qb.joins = append(qb.joins, fmt.Sprintf("INNER JOIN %s ON %s %s %s", table, first, op, second))
	return qb
}

// LeftJoin adds a LEFT JOIN clause to the query.
func (qb *QueryBuilder) LeftJoin(table, first, op, second string) *QueryBuilder {
	if !tableRegex.MatchString(table) {
		qb.setErr(fmt.Errorf("[Crafter] invalid table identifier in LeftJoin: %q", table))
		return qb
	}
	if !columnRegex.MatchString(first) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in LeftJoin (first): %q", first))
		return qb
	}
	if !columnRegex.MatchString(second) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in LeftJoin (second): %q", second))
		return qb
	}
	if !isValidOperator(op) {
		qb.setErr(fmt.Errorf("[Crafter] invalid operator in LeftJoin: %q", op))
		return qb
	}
	qb.joins = append(qb.joins, fmt.Sprintf("LEFT JOIN %s ON %s %s %s", table, first, op, second))
	return qb
}

// RightJoin adds a RIGHT JOIN clause to the query.
func (qb *QueryBuilder) RightJoin(table, first, op, second string) *QueryBuilder {
	if !tableRegex.MatchString(table) {
		qb.setErr(fmt.Errorf("[Crafter] invalid table identifier in RightJoin: %q", table))
		return qb
	}
	if !columnRegex.MatchString(first) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in RightJoin (first): %q", first))
		return qb
	}
	if !columnRegex.MatchString(second) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in RightJoin (second): %q", second))
		return qb
	}
	if !isValidOperator(op) {
		qb.setErr(fmt.Errorf("[Crafter] invalid operator in RightJoin: %q", op))
		return qb
	}
	qb.joins = append(qb.joins, fmt.Sprintf("RIGHT JOIN %s ON %s %s %s", table, first, op, second))
	return qb
}

// WithTrashed includes soft-deleted records in the query constraints.
func (qb *QueryBuilder) WithTrashed() *QueryBuilder {
	qb.withTrashed = true
	return qb
}

// whereSQL compiles internal filter states into a standard WHERE clause string.
func (qb *QueryBuilder) whereSQL() string {
	if len(qb.where) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(" WHERE ")
	for i, w := range qb.where {
		if i > 0 {
			if !strings.HasPrefix(w, "OR ") && !strings.HasPrefix(w, "AND ") {
				sb.WriteString(" AND ")
			} else {
				sb.WriteString(" ")
			}
		}

		if i == 0 {
			cleaned := w
			if strings.HasPrefix(w, "OR ") {
				cleaned = strings.TrimPrefix(w, "OR ")
			} else if strings.HasPrefix(w, "AND ") {
				cleaned = strings.TrimPrefix(w, "AND ")
			}
			sb.WriteString(cleaned)
		} else {
			sb.WriteString(w)
		}
	}
	return sb.String()
}

// ToSQL serializes the internal state into a valid SQL query string.
func (qb *QueryBuilder) ToSQL() string {
	if qb.err != nil {
		return ""
	}
	cols := "*"
	if len(qb.columns) > 0 {
		cols = strings.Join(qb.columns, ", ")
	}
	sqlStr := fmt.Sprintf("SELECT %s FROM %s", cols, qb.table)

	if len(qb.joins) > 0 {
		sqlStr += " " + strings.Join(qb.joins, " ")
	}

	sqlStr += qb.whereSQL()

	if qb.orderBy != "" {
		sqlStr += " ORDER BY " + qb.orderBy
	}
	if qb.limitVal > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", qb.limitVal)
	}
	// F-195: Limit(-1) explicitly means "no limit" — omit the LIMIT clause.
	// A limitVal of 0 (default) also means no limit was set.
	if qb.offsetVal > 0 {
		sqlStr += fmt.Sprintf(" OFFSET %d", qb.offsetVal)
	}
	return sqlStr
}

// OrderBy appends an ORDER BY clause to the query.
// dir should be "ASC", "DESC", "ASC NULLS FIRST", or "DESC NULLS LAST".
//
// Example:
//
//	gostack.Table("posts").OrderBy("created_at", "DESC").Get(&posts)
func (qb *QueryBuilder) OrderBy(col, dir string) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in OrderBy: %q", col))
		return qb
	}
	d := strings.ToUpper(strings.TrimSpace(dir))
	if !isValidDirection(d) {
		qb.setErr(fmt.Errorf("[Crafter] invalid direction in OrderBy: %q", dir))
		return qb
	}
	qb.orderBy = col + " " + d
	return qb
}

// Limit caps the number of rows returned. Limit(n) requires n >= 0.
// If n < 0, an error is recorded on the builder to prevent pagination arithmetic errors from full-table scanning (C2).
// Use NoLimit() to explicitly remove any limit constraint.
func (qb *QueryBuilder) Limit(n int) *QueryBuilder {
	if n < 0 {
		qb.setErr(fmt.Errorf("database: Limit requires n >= 0, got %d (use NoLimit() to explicitly clear limit)", n))
		return qb
	}
	qb.limitVal = n
	return qb
}

// NoLimit explicitly removes any LIMIT constraint from the query.
func (qb *QueryBuilder) NoLimit() *QueryBuilder {
	qb.limitVal = -1
	return qb
}

// Offset skips the first n rows before returning results. Pass 0 or a negative number to omit offset.
func (qb *QueryBuilder) Offset(n int) *QueryBuilder {
	if n < 0 {
		qb.offsetVal = 0
	} else {
		qb.offsetVal = n
	}
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
	if qb.err != nil {
		return qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}

	cols := "*"
	if len(qb.columns) > 0 {
		cols = strings.Join(qb.columns, ", ")
	}
	base := fmt.Sprintf("SELECT %s FROM %s", cols, qb.table)
	if len(qb.joins) > 0 {
		base += " " + strings.Join(qb.joins, " ")
	}
	base += qb.whereSQL()

	if qb.orderBy != "" {
		base += " ORDER BY " + qb.orderBy
	}
	base += " LIMIT 1"

	result, err := qb.executor().Query(base, qb.bindings...)
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
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("[Crafter] %w", ErrNotFound)
		}
		return fmt.Errorf("[Crafter] First() hydration failed: %w", err)
	}

	// F-190: eager-load relations when requested (Get() already handles this; First() was missing it).
	if len(qb.relations) > 0 {
		if err := qb.eagerLoadRelations(dest); err != nil {
			return fmt.Errorf("[Crafter] First() eager load failed: %w", err)
		}
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
	if qb.err != nil {
		return qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	if len(data) == 0 {
		return nil
	}

	// PAR-12: Reject mass UPDATE without a WHERE clause unless explicitly opted in.
	if len(qb.where) == 0 && !qb.allowMassMutate {
		return fmt.Errorf("database: dangerous UPDATE without WHERE clause rejected; call AllowMassMutation() to force a full-table update")
	}

	// F-188: Fire BeforeSave hook before building the SQL so hook mutations are reflected.
	if model != nil {
		if bs, ok := model.(BeforeSaver); ok {
			if err := bs.BeforeSave(); err != nil {
				return fmt.Errorf("[Crafter] BeforeSave hook failed: %w", err)
			}
			// Re-extract mutated fields from the model into the data map
			modelVal := reflect.ValueOf(model)
			if modelVal.Kind() == reflect.Ptr {
				modelVal = modelVal.Elem()
			}
			if modelVal.Kind() == reflect.Struct {
				modelType := modelVal.Type()
				for col := range data {
					for i := 0; i < modelType.NumField(); i++ {
						field := modelType.Field(i)
						dbTag := field.Tag.Get("db")
						if dbTag == "" {
							dbTag = strings.ToLower(field.Name)
						}
						if dbTag == col {
							fVal := modelVal.Field(i)
							if fVal.CanInterface() {
								data[col] = fVal.Interface()
							}
							break
						}
					}
				}
			}
		}
	}

	for col := range data {
		if !identRegex.MatchString(col) {
			return fmt.Errorf("[Crafter] invalid column identifier in Update: %q", col)
		}
	}

	keys := make([]string, 0, len(data))
	for col := range data {
		keys = append(keys, col)
	}
	sort.Strings(keys)

	var setClauses []string
	var bindings []any

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	for _, col := range keys {
		val := data[col]
		var placeholder string
		if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
			placeholder = fmt.Sprintf("$%d", len(bindings)+1)
		} else {
			placeholder = "?"
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", col, placeholder))
		bindings = append(bindings, val)
	}

	whereClause := qb.whereSQL()
	if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
		whereClause = shiftPgPlaceholders(whereClause, len(bindings))
	}

	sqlStr := fmt.Sprintf("UPDATE %s SET %s%s", qb.table, strings.Join(setClauses, ", "), whereClause)
	
	if len(qb.where) > 0 {
		bindings = append(bindings, qb.bindings...)
	}

	return qb.executor().Exec(sqlStr, bindings...)
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
	if qb.err != nil {
		return qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}

	// PAR-12: Reject mass DELETE without a WHERE clause unless explicitly opted in.
	if len(qb.where) == 0 && !qb.allowMassMutate {
		return fmt.Errorf("database: dangerous DELETE without WHERE clause rejected; call AllowMassMutation() to force a full-table delete")
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
	sqlStr += qb.whereSQL()

	// C3: When a model implements a post-hook, statement and hook share one transactional boundary.
	if model != nil {
		if hook, ok := model.(AfterDeleter); ok {
			if qb.tx == nil && qb.db != nil {
				tx, err := qb.db.BeginTx()
				if err != nil {
					return fmt.Errorf("[Crafter] failed to begin transaction for AfterDelete hook: %w", err)
				}
				if err := tx.Exec(sqlStr, qb.bindings...); err != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Printf("[Crafter CRITICAL] Failed to rollback transaction after DeleteModel Exec error: %v (original: %v)", rbErr, err)
					}
					return err
				}
				if err := hook.AfterDelete(); err != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Printf("[Crafter CRITICAL] Failed to rollback transaction after AfterDelete hook error: %v (original: %v)", rbErr, err)
					}
					return fmt.Errorf("[Crafter] AfterDelete hook failed: %w", err)
				}
				return tx.Commit()
			}
			if err := qb.executor().Exec(sqlStr, qb.bindings...); err != nil {
				return err
			}
			if err := hook.AfterDelete(); err != nil {
				return fmt.Errorf("[Crafter] AfterDelete hook failed: %w", err)
			}
			return nil
		}
	}

	if err := qb.executor().Exec(sqlStr, qb.bindings...); err != nil {
		return err
	}
	return nil
}

// Execute triggers the execution of the built SQL query via the 
// injected database connection. 
func (qb *QueryBuilder) Execute() (any, error) {
	if qb.err != nil {
		return nil, qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return nil, fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	
	return qb.executor().Query(qb.ToSQL(), qb.bindings...)
}

// Get executes the compiled query and automatically hydates the database results 
// directly into the provided destination pointer (struct or slice of structs).
func (qb *QueryBuilder) Get(dest any) error {
	if qb.err != nil {
		return qb.err
	}
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
// Values are automatically partitioned into chunks of 500 (combined with OR)
// to prevent exceeding database driver parameter placeholder limits.
func (qb *QueryBuilder) WhereIn(col string, vals []any) *QueryBuilder {
	if !columnRegex.MatchString(col) {
		qb.setErr(fmt.Errorf("[Crafter] invalid column identifier in WhereIn: %q", col))
		return qb
	}
	if len(vals) == 0 {
		qb.where = append(qb.where, "1 = 0")
		return qb
	}

	wdrv := ""
	if qb.db != nil {
		wdrv = qb.db.Driver()
	}

	const chunkSize = 500
	var clauses []string

	for start := 0; start < len(vals); start += chunkSize {
		end := start + chunkSize
		if end > len(vals) {
			end = len(vals)
		}
		chunk := vals[start:end]

		var placeholders []string
		for _, val := range chunk {
			var placeholder string
			if wdrv == "postgres" || wdrv == "cockroach" || wdrv == "cockroachdb" {
				placeholder = fmt.Sprintf("$%d", len(qb.bindings)+1)
			} else {
				placeholder = "?"
			}
			placeholders = append(placeholders, placeholder)
			qb.bindings = append(qb.bindings, val)
		}
		clauses = append(clauses, fmt.Sprintf("%s IN (%s)", col, strings.Join(placeholders, ", ")))
	}

	if len(clauses) == 1 {
		qb.where = append(qb.where, clauses[0])
	} else {
		qb.where = append(qb.where, "("+strings.Join(clauses, " OR ")+")")
	}

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
	if qb.err != nil {
		return qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return fmt.Errorf("database connection is nil; ensure the QueryBuilder was initialized with a valid database adapter")
	}
	if len(data) == 0 {
		return nil
	}

	for col := range data {
		if !identRegex.MatchString(col) {
			return fmt.Errorf("[Crafter] invalid column identifier in Insert: %q", col)
		}
	}

	// Fire BeforeSave hook if model implements it.
	// F-187: After the hook fires, re-extract mutated fields from the struct so that
	// any mutations the hook made (e.g. hashing passwords, setting UpdatedAt) are
	// included in the INSERT rather than the pre-hook values in `data`.
	if model != nil {
		if hook, ok := model.(BeforeSaver); ok {
			if err := hook.BeforeSave(); err != nil {
				return fmt.Errorf("[SparkORM] BeforeSave hook failed: %w", err)
			}
			// Re-extract column values from the mutated model into data.
			modelVal := reflect.ValueOf(model)
			if modelVal.Kind() == reflect.Ptr {
				modelVal = modelVal.Elem()
			}
			if modelVal.Kind() == reflect.Struct {
				modelType := modelVal.Type()
				for col := range data {
					for i := 0; i < modelType.NumField(); i++ {
						field := modelType.Field(i)
						dbTag := field.Tag.Get("db")
						if dbTag == "" {
							dbTag = strings.ToLower(field.Name)
						}
						if dbTag == col {
							fVal := modelVal.Field(i)
							if fVal.CanInterface() {
								data[col] = fVal.Interface()
							}
							break
						}
					}
				}
			}
		}
	}

	var columns []string
	var placeholders []string
	var bindings []any

	drv := ""
	if qb.db != nil {
		drv = qb.db.Driver()
	}
	for col, val := range data {
		columns = append(columns, col)
		var placeholder string
		if drv == "postgres" || drv == "cockroach" || drv == "cockroachdb" {
			placeholder = fmt.Sprintf("$%d", len(bindings)+1)
		} else {
			placeholder = "?"
		}
		placeholders = append(placeholders, placeholder)
		bindings = append(bindings, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", qb.table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	// C3: When a model implements a post-hook, statement and hook share one transactional boundary.
	if model != nil {
		if hook, ok := model.(AfterCreator); ok {
			if qb.tx == nil && qb.db != nil {
				tx, err := qb.db.BeginTx()
				if err != nil {
					return fmt.Errorf("[SparkORM] failed to begin transaction for AfterCreate hook: %w", err)
				}
				if err := tx.Exec(query, bindings...); err != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Printf("[SparkORM CRITICAL] Failed to rollback transaction after InsertModel Exec error: %v (original: %v)", rbErr, err)
					}
					return err
				}
				if err := hook.AfterCreate(); err != nil {
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Printf("[SparkORM CRITICAL] Failed to rollback transaction after AfterCreate hook error: %v (original: %v)", rbErr, err)
					}
					return fmt.Errorf("[SparkORM] AfterCreate hook failed: %w", err)
				}
				return tx.Commit()
			}
			// Already within an enclosing transaction or non-standard executor
			if err := qb.executor().Exec(query, bindings...); err != nil {
				return err
			}
			if err := hook.AfterCreate(); err != nil {
				return fmt.Errorf("[SparkORM] AfterCreate hook failed: %w", err)
			}
			return nil
		}
	}

	if err := qb.executor().Exec(query, bindings...); err != nil {
		return err
	}
	return nil
}

// InsertGetId compiles and executes an INSERT statement and returns the generated primary key (PAR-13).
// On PostgreSQL/CockroachDB it appends RETURNING id and reads from the query result.
// On MySQL and SQLite it uses sql.Result.LastInsertId().
func (qb *QueryBuilder) InsertGetId(data map[string]any) (int64, error) {
	if qb.err != nil {
		return 0, qb.err
	}
	if qb.db == nil {
		return 0, fmt.Errorf("database connection is nil; InsertGetId requires a *SQLAdapter connection")
	}
	if len(data) == 0 {
		return 0, fmt.Errorf("InsertGetId called with empty data map")
	}

	for col := range data {
		if !identRegex.MatchString(col) {
			return 0, fmt.Errorf("[Crafter] invalid column identifier in InsertGetId: %q", col)
		}
	}

	var columns []string
	var placeholders []string
	var bindings []any

	drv := qb.db.Driver()
	isPostgres := drv == "postgres" || drv == "cockroach" || drv == "cockroachdb"

	for col, val := range data {
		columns = append(columns, col)
		if isPostgres {
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(bindings)+1))
		} else {
			placeholders = append(placeholders, "?")
		}
		bindings = append(bindings, val)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qb.table, strings.Join(columns, ", "), strings.Join(placeholders, ", "))

	if isPostgres {
		// Postgres: use RETURNING id and Query to retrieve the new ID.
		query += " RETURNING id"
		rows, err := qb.db.Query(query, bindings...)
		if err != nil {
			return 0, fmt.Errorf("InsertGetId: query failed: %w", err)
		}
		defer func() {
			if rc, ok := rows.(interface{ Close() error }); ok {
				_ = rc.Close()
			}
		}()
		if sr, ok := rows.(interface {
			Next() bool
			Scan(dest ...any) error
		}); ok && sr.Next() {
			var id int64
			if err := sr.Scan(&id); err != nil {
				return 0, fmt.Errorf("InsertGetId: scan RETURNING id failed: %w", err)
			}
			return id, nil
		}
		return 0, fmt.Errorf("InsertGetId: no row returned from RETURNING id")
	}

	// MySQL / SQLite: use ExecResult to call LastInsertId.
	adapter, ok := qb.db.(*SQLAdapter)
	if !ok {
		return 0, fmt.Errorf("InsertGetId: LastInsertId requires a *SQLAdapter (got %T)", qb.db)
	}
	result, err := adapter.ExecResult(query, bindings...)
	if err != nil {
		return 0, fmt.Errorf("InsertGetId: exec failed: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("InsertGetId: LastInsertId failed: %w", err)
	}
	return id, nil
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
	if qb.err != nil {
		return 0, qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return 0, fmt.Errorf("database connection is nil")
	}
	sqlStr := qb.countSQL()
	result, err := qb.executor().Query(sqlStr, qb.bindings...)
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
	if len(qb.joins) > 0 {
		sqlStr += " " + strings.Join(qb.joins, " ")
	}
	sqlStr += qb.whereSQL()
	return sqlStr
}

// DefaultMaxPaginationPerPage defines the maximum rows per page returned by Paginate
// to prevent memory exhaustion / denial of service attacks via unbounded pagination requests.
const DefaultMaxPaginationPerPage = 100

// Paginate counts total records matching current query conditions, retrieves the requested
// chunk of rows for the target page, and hydrates them into dest.
func (qb *QueryBuilder) Paginate(dest any, page, perPage int) (*PageMeta, error) {
	if qb.err != nil {
		return nil, qb.err
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 15
	} else if perPage > DefaultMaxPaginationPerPage {
		perPage = DefaultMaxPaginationPerPage
	}

	total, err := qb.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count records for pagination: %w", err)
	}

	offset := (page - 1) * perPage

	cols := "*"
	if len(qb.columns) > 0 {
		cols = strings.Join(qb.columns, ", ")
	}
	base := fmt.Sprintf("SELECT %s FROM %s", cols, qb.table)
	if len(qb.joins) > 0 {
		base += " " + strings.Join(qb.joins, " ")
	}
	base += qb.whereSQL()

	if qb.orderBy != "" {
		base += " ORDER BY " + qb.orderBy
	}
	sqlStr := base + fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	result, err := qb.executor().Query(sqlStr, qb.bindings...)
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

// executeAggregate executes a single value aggregate query and returns the scanned result.
func (qb *QueryBuilder) executeAggregate(expr string, dest any) error {
	if qb.err != nil {
		return qb.err
	}
	if qb.db == nil && qb.tx == nil {
		return fmt.Errorf("database connection is nil")
	}
	sqlStr := fmt.Sprintf("SELECT %s FROM %s", expr, qb.table)
	if len(qb.joins) > 0 {
		sqlStr += " " + strings.Join(qb.joins, " ")
	}
	sqlStr += qb.whereSQL()

	result, err := qb.executor().Query(sqlStr, qb.bindings...)
	if err != nil {
		return err
	}
	rows, ok := result.(*sql.Rows)
	if !ok {
		return fmt.Errorf("expected *sql.Rows, got %T", result)
	}
	defer rows.Close()

	if !rows.Next() {
		return sql.ErrNoRows
	}
	return rows.Scan(dest)
}

// Sum calculates the sum of a column.
func (qb *QueryBuilder) Sum(col string) (float64, error) {
	if !identRegex.MatchString(col) && !columnRegex.MatchString(col) {
		return 0, fmt.Errorf("[Crafter] invalid column identifier in Sum: %q", col)
	}
	var val sql.NullFloat64
	err := qb.executeAggregate(fmt.Sprintf("SUM(%s)", col), &val)
	if err != nil {
		return 0, err
	}
	return val.Float64, nil
}

// Avg calculates the average of a column.
func (qb *QueryBuilder) Avg(col string) (float64, error) {
	if !identRegex.MatchString(col) && !columnRegex.MatchString(col) {
		return 0, fmt.Errorf("[Crafter] invalid column identifier in Avg: %q", col)
	}
	var val sql.NullFloat64
	err := qb.executeAggregate(fmt.Sprintf("AVG(%s)", col), &val)
	if err != nil {
		return 0, err
	}
	return val.Float64, nil
}

// Min calculates the minimum of a column.
func (qb *QueryBuilder) Min(col string) (any, error) {
	if !identRegex.MatchString(col) && !columnRegex.MatchString(col) {
		return nil, fmt.Errorf("[Crafter] invalid column identifier in Min: %q", col)
	}
	var raw []byte
	err := qb.executeAggregate(fmt.Sprintf("MIN(%s)", col), &raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	// Attempt numeric coercion; fall back to string for non-numeric types.
	var f sql.NullFloat64
	if err2 := qb.executeAggregate(fmt.Sprintf("MIN(%s)", col), &f); err2 == nil && f.Valid {
		return f.Float64, nil
	}
	return string(raw), nil
}

// Max calculates the maximum of a column.
func (qb *QueryBuilder) Max(col string) (any, error) {
	if !identRegex.MatchString(col) && !columnRegex.MatchString(col) {
		return nil, fmt.Errorf("[Crafter] invalid column identifier in Max: %q", col)
	}
	var raw []byte
	err := qb.executeAggregate(fmt.Sprintf("MAX(%s)", col), &raw)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	// Attempt numeric coercion; fall back to string for non-numeric types.
	var f sql.NullFloat64
	if err2 := qb.executeAggregate(fmt.Sprintf("MAX(%s)", col), &f); err2 == nil && f.Valid {
		return f.Float64, nil
	}
	return string(raw), nil
}


