/*
Purpose:
This file provides a lightweight, interface-driven validation layer for incoming HTTP request data,
extended with a declarative rule composition engine for expressive, Laravel-style field validation.

Philosophy:
We believe request validation should be performant and integrated directly with the routing pipeline.
By compiling structural validators into middleware, we filter bad payloads before executing controller
logic. Rules should read like a policy document, not imperative if-chains — making validation intent
obvious at a glance.

Architecture:
Two complementary layers:
  1. ValidateRequest middleware — decodes the JSON body, invokes the Validator interface, and short-
     circuits with a 422 response on failure. Unchanged from the original design.
  2. Rules engine — a reflect-based composer that iterates a RuleSet map, extracts string
     representations of struct field values, and applies each Rule function in order, collecting
     all failures into a single error map.

Choice:
We chose a function-value approach for Rule (func(field, value string) (bool, string)) over string
tags (e.g. `validate:"required|email"`) to preserve compile-time safety, allow closures for
parameterised rules (MinLength, MaxLength), and avoid a secondary tag parser.

Implementation:
- Validator: interface all request structs must implement to expose their error map.
- ValidateRequest: middleware factory that decodes JSON and dispatches to Validate().
- typeCache: thread-safe sync.Map caching reflection types to amortise repeated parsing cost.
- Rule: a validation function signature — receives field name + string value, returns ok + message.
- RuleSet: a map of struct field names to ordered slices of Rule functions.
- Rules(src, set): reflects over src, coerces each field to string, and runs its rules.
- Built-in rules: Required, IsEmail, IsNumeric, MinLength(n), MaxLength(n), Matches(pattern).
*/
package http

import (
	"database/sql"
	"encoding/json"
	"fmt"
	netHTTP "net/http"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// ─── Core Validator Interface ──────────────────────────────────────────────────

// Validator defines the interface all request payload structs must implement
// to participate in the ValidateRequest middleware pipeline.
type Validator interface {
	// Validate returns a map of field name → human-readable error message.
	// An empty map (or nil) signals that all fields passed validation.
	Validate() map[string]string
}

// ValidateRequest returns a Middleware that decodes the JSON request body into req's
// type, runs its Validate() method, and short-circuits with HTTP 422 on failure.
//
// Parameters:
//   - req: A zero-value instance of the struct representing the expected JSON payload.
//     The struct must implement Validator.
//
// On success, the decoded and validated value is stored in the request context under
// the key "validated_data" for downstream handler retrieval.
//
// Implementation note:
// We capture the reflect.Type at registration time and close over it in the returned
// middleware function. This is both safe and efficient — reflect.TypeOf is O(1) and
// avoids the unchecked type assertion from a sync.Map that could panic on a cache miss.
func ValidateRequest(req any) Middleware {
	// Capture the concrete type once at middleware construction time.
	t := reflect.TypeOf(req)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return func(ctx *Context, next NextHandler) error {
		// Allocate a fresh zero-value pointer to the request struct type on each call.
		input := reflect.New(t).Interface()

		if err := json.NewDecoder(ctx.Request.Body).Decode(input); err != nil {
			_ = ctx.JSON(netHTTP.StatusBadRequest, map[string]string{"error": "Invalid JSON format"})
			return nil
		}

		if v, ok := input.(Validator); ok {
			if errors := v.Validate(); len(errors) > 0 {
				_ = ctx.JSON(netHTTP.StatusUnprocessableEntity, map[string]any{"errors": errors})
				return nil
			}
		}

		ctx.Set("validated_data", input)
		return next(ctx)
	}
}

// ─── Declarative Rule Engine ───────────────────────────────────────────────────

// Rule is a single validation function. It receives the field name and its string-
// coerced value, and returns whether the value is valid and an error message if not.
//
// Example custom rule:
//
//	noSpaces := func(field, value string) (bool, string) {
//	    if strings.Contains(value, " ") {
//	        return false, field + " must not contain spaces"
//	    }
//	    return true, ""
//	}
type Rule func(field, value string) (ok bool, message string)

// RuleSet maps struct field names (as they appear in the Go struct, e.g. "Email")
// to an ordered slice of Rule functions to apply against that field's value.
//
// Example:
//
//	http.RuleSet{
//	    "Email":    {http.Required, http.IsEmail},
//	    "Password": {http.Required, http.MinLength(8)},
//	}
type RuleSet map[string][]Rule

// Rules reflects over src, extracts the string representation of each field named
// in set, and runs each Rule in the declared order. The first failing rule for each
// field contributes its message to the returned error map. Fields with no failures
// are omitted from the result.
//
// Parameters:
//   - src: A pointer to the struct being validated (typically a request object).
//   - set: The RuleSet declaring which fields to validate and which rules to apply.
//
// Returns a map[string]string of field → error message. An empty map means all rules passed.
//
// Example usage inside a Validator implementation:
//
//	func (r *LoginRequest) Validate() map[string]string {
//	    return http.Rules(r, http.RuleSet{
//	        "Email":    {http.Required, http.IsEmail},
//	        "Password": {http.Required, http.MinLength(8)},
//	    })
//	}
func Rules(src any, set RuleSet) map[string]string {
	errors := make(map[string]string)

	v := reflect.ValueOf(src)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors
	}

	for fieldName, rules := range set {
		fieldVal := v.FieldByName(fieldName)
		if !fieldVal.IsValid() {
			errors[fieldName] = fieldName + " is an unrecognised field"
			continue
		}

		// Coerce the field value to string for uniform rule processing.
		strVal := fmt.Sprintf("%v", fieldVal.Interface())
		// Treat zero values of numeric types as empty strings for Required checks.
		if fieldVal.IsZero() && fieldVal.Kind() != reflect.String {
			strVal = ""
		}

		for _, rule := range rules {
			if ok, msg := rule(fieldName, strVal); !ok {
				errors[fieldName] = msg
				break // Only the first failing rule is reported per field.
			}
		}
	}

	return errors
}

// ─── Built-in Rules ───────────────────────────────────────────────────────────

// Required rejects empty string values.
//
// Example: http.Required
var Required Rule = func(field, value string) (bool, string) {
	if strings.TrimSpace(value) == "" {
		return false, field + " is required"
	}
	return true, ""
}

// IsEmail rejects values that do not match a standard email address pattern.
//
// Example: http.IsEmail
var IsEmail Rule = func(field, value string) (bool, string) {
	// RFC 5322 simplified pattern — sufficient for server-side sanity checks.
	pattern := `^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, value)
	if !matched {
		return false, field + " must be a valid email address"
	}
	return true, ""
}

// IsNumeric rejects values that contain non-digit characters.
//
// Example: http.IsNumeric
var IsNumeric Rule = func(field, value string) (bool, string) {
	matched, _ := regexp.MatchString(`^\d+$`, value)
	if !matched {
		return false, field + " must be a numeric value"
	}
	return true, ""
}

// MinLength returns a Rule that rejects values shorter than n Unicode characters.
//
// Example: http.MinLength(8)
func MinLength(n int) Rule {
	return func(field, value string) (bool, string) {
		if utf8.RuneCountInString(value) < n {
			return false, fmt.Sprintf("%s must be at least %d characters", field, n)
		}
		return true, ""
	}
}

// MaxLength returns a Rule that rejects values longer than n Unicode characters.
//
// Example: http.MaxLength(255)
func MaxLength(n int) Rule {
	return func(field, value string) (bool, string) {
		if utf8.RuneCountInString(value) > n {
			return false, fmt.Sprintf("%s must not exceed %d characters", field, n)
		}
		return true, ""
	}
}

// Matches returns a Rule that rejects values not matching the given regular expression.
//
// Example: http.Matches(`^[a-zA-Z]+$`)
func Matches(pattern string) Rule {
	compiled := regexp.MustCompile(pattern)
	return func(field, value string) (bool, string) {
		if !compiled.MatchString(value) {
			return false, fmt.Sprintf("%s format is invalid", field)
		}
		return true, ""
	}
}

// IsUnique returns a Rule that queries a database to ensure the field value is unique in the specified table and column.
//
// Security: table and column names are validated against a strict alphanumeric+underscore allowlist
// before being interpolated into the SQL query, preventing SQL injection attacks.
func IsUnique(db contract.Database, table, column string) Rule {
	return func(field, value string) (bool, string) {
		// Guard against SQL injection: only allow safe identifier characters.
		if !isSafeIdentifier(table) {
			return false, fmt.Sprintf("%s uniqueness check failed: invalid table name", field)
		}
		if !isSafeIdentifier(column) {
			return false, fmt.Sprintf("%s uniqueness check failed: invalid column name", field)
		}

		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, column)
		result, err := db.Query(query, value)
		if err != nil {
			return false, fmt.Sprintf("%s uniqueness check failed: %v", field, err)
		}

		rows, ok := result.(*sql.Rows)
		if !ok {
			// Driver returned a non-standard result — treat as "cannot confirm uniqueness".
			return false, fmt.Sprintf("%s uniqueness check failed: unsupported driver result type", field)
		}
		defer rows.Close()

		var count int
		if rows.Next() {
			if err := rows.Scan(&count); err == nil && count > 0 {
				return false, fmt.Sprintf("%s has already been taken", field)
			}
		}
		return true, ""
	}
}

// isSafeIdentifier checks that a database identifier (table/column name) contains
// only alphanumeric characters and underscores, preventing SQL injection.
var safeIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isSafeIdentifier(name string) bool {
	return safeIdentifierPattern.MatchString(name)
}
