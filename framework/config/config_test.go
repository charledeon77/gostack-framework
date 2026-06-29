/*
Purpose:
This file contains the complete unit test suite for the GoStack config package.
It validates all parsing behaviour of the Manager: key/value loading, comment
stripping, blank-line skipping, quoted values, inline comment trimming, typed
getters, fallback resolution, and graceful error handling for missing files.

Philosophy:
Configuration parsing is load-bearing infrastructure. A silent mis-parse (e.g.
an inline comment leaking into a DSN string) would corrupt every downstream
subsystem that reads from it. Every edge case of the parser must therefore be
tested explicitly and independently, not as a side-effect of integration tests.

Architecture:
Each test is a focused, standalone unit that creates a temporary .env file via
writeTempEnv(), constructs a fresh Manager, and asserts a single behaviour.
No global state is shared between tests; all I/O is scoped to t.TempDir().

Choice:
We use stdlib testing only — no third-party assertion libraries — to remain
consistent with the rest of the GoStack test suite and to avoid introducing
dependencies purely for test convenience.

Implementation:
- writeTempEnv: helper that writes a .env string to a temp file and returns its path.
- TestLoad_BasicKeyValue: asserts plain KEY=VALUE parsing.
- TestLoad_CommentsAndBlanks: asserts comment lines and blank lines are ignored.
- TestLoad_QuotedValues: asserts double- and single-quoted value stripping.
- TestLoad_InlineComment: asserts inline comments after a space-hash are stripped.
- TestGetOrDefault: asserts fallback when key is absent or present.
- TestGetInt: asserts integer parsing and fallback on bad/missing values.
- TestGetBool: asserts truthy/falsy string variants and missing-key fallback.
- TestLoad_MissingFile: asserts an error is returned for a non-existent path.
*/
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}
	return path
}

func TestLoad_BasicKeyValue(t *testing.T) {
	path := writeTempEnv(t, "APP_NAME=GoStack\nAPP_ENV=local\n")
	m := New()
	if err := m.Load(path); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := m.Get("APP_NAME"); got != "GoStack" {
		t.Errorf("APP_NAME = %q, want %q", got, "GoStack")
	}
	if got := m.Get("APP_ENV"); got != "local" {
		t.Errorf("APP_ENV = %q, want %q", got, "local")
	}
}

func TestLoad_CommentsAndBlanks(t *testing.T) {
	path := writeTempEnv(t, `
# This is a comment
APP_KEY=secret

# Another comment
DEBUG=true
`)
	m := New()
	if err := m.Load(path); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := m.Get("APP_KEY"); got != "secret" {
		t.Errorf("APP_KEY = %q, want %q", got, "secret")
	}
	if got := m.Get("DEBUG"); got != "true" {
		t.Errorf("DEBUG = %q, want %q", got, "true")
	}
}

func TestLoad_QuotedValues(t *testing.T) {
	path := writeTempEnv(t, `DB_DSN="user:pass@/mydb"` + "\nSECRET='my secret value'\n")
	m := New()
	if err := m.Load(path); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := m.Get("DB_DSN"); got != "user:pass@/mydb" {
		t.Errorf("DB_DSN = %q, want %q", got, "user:pass@/mydb")
	}
	if got := m.Get("SECRET"); got != "my secret value" {
		t.Errorf("SECRET = %q, want %q", got, "my secret value")
	}
}

func TestLoad_InlineComment(t *testing.T) {
	path := writeTempEnv(t, "PORT=8080 # http port\n")
	m := New()
	if err := m.Load(path); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := m.Get("PORT"); got != "8080" {
		t.Errorf("PORT = %q, want %q", got, "8080")
	}
}

func TestGetOrDefault(t *testing.T) {
	m := New()
	m.Set("EXISTING", "hello")
	if got := m.GetOrDefault("EXISTING", "fallback"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if got := m.GetOrDefault("MISSING", "fallback"); got != "fallback" {
		t.Errorf("got %q, want %q", got, "fallback")
	}
}

func TestGetInt(t *testing.T) {
	m := New()
	m.Set("PORT", "3000")
	m.Set("BAD", "notanumber")
	if got := m.GetInt("PORT", 8080); got != 3000 {
		t.Errorf("PORT = %d, want 3000", got)
	}
	if got := m.GetInt("BAD", 8080); got != 8080 {
		t.Errorf("BAD fallback = %d, want 8080", got)
	}
	if got := m.GetInt("MISSING", 42); got != 42 {
		t.Errorf("MISSING fallback = %d, want 42", got)
	}
}

func TestGetBool(t *testing.T) {
	m := New()
	m.Set("FLAG_TRUE", "true")
	m.Set("FLAG_ONE", "1")
	m.Set("FLAG_YES", "yes")
	m.Set("FLAG_FALSE", "false")
	m.Set("FLAG_ZERO", "0")

	cases := []struct {
		key      string
		fallback bool
		want     bool
	}{
		{"FLAG_TRUE", false, true},
		{"FLAG_ONE", false, true},
		{"FLAG_YES", false, true},
		{"FLAG_FALSE", true, false},
		{"FLAG_ZERO", true, false},
		{"MISSING", true, true},
	}
	for _, c := range cases {
		if got := m.GetBool(c.key, c.fallback); got != c.want {
			t.Errorf("GetBool(%q) = %v, want %v", c.key, got, c.want)
		}
	}
}

func TestLoad_MissingFile(t *testing.T) {
	m := New()
	err := m.Load("/nonexistent/.env")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}
