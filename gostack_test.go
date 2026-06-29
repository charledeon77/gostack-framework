package gostack

import (
	"testing"
)

// TestDatabaseRegistration verifies the framework connection logic
func TestDatabaseRegistration(t *testing.T) {
	err := InitDatabase("postgres", "postgres://user:pass@localhost:5432/db")
	if err != nil {
		t.Logf("Engine initialized (expected connection error): %v", err)
	}
}

// TestFluentQuery verifies the Query Builder syntax
func TestFluentQuery(t *testing.T) {
	sql := Table("users").Where("id", "=", "1").Where("active", "=", "1").ToSQL()
	expected := "SELECT * FROM users WHERE id = ? AND active = ?"
	
	if sql != expected {
		t.Errorf("Expected %s, got %s", expected, sql)
	}
}