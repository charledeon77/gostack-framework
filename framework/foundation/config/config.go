/*
Purpose:
This file implements the GoCon environment configuration system. It parses a `.env`
file and exposes typed getters used throughout the application.

Philosophy:
Configuration should be environment-driven and never hardcoded. By reading from a `.env`
file at startup, developers can safely switch between local, staging, and production
environments without touching source code. We use zero external dependencies — pure stdlib.

Architecture:
A single `Manager` struct holds a parsed key→value map loaded once at boot via `Load()`.
The global `gostack.Config` facade exposes it to the entire application.

Choice:
We chose a simple line-by-line `.env` parser over external libraries (like godotenv) to
maintain GoStack's zero-dependency philosophy. The parser handles comments, blank lines,
quoted values, and inline comments.

Implementation:
- Manager: holds the parsed env map.
- Load(path): reads and parses the .env file into the manager.
- Get(key): returns raw string value.
- GetOrDefault(key, fallback): returns value or fallback if missing.
- GetInt(key, fallback): returns int-parsed value or fallback.
- GetBool(key, fallback): returns bool-parsed value or fallback.
*/
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Manager holds the parsed environment configuration map.
type Manager struct {
	values map[string]string
}

// New builds a fresh, empty Manager instance.
func New() *Manager {
	return &Manager{values: make(map[string]string)}
}

// Load reads and parses a .env file into the Manager.
// Lines starting with '#' are treated as comments and skipped.
// Blank lines are skipped. Values may optionally be quoted with " or '.
//
// Example .env:
//
//	APP_NAME=GoStack
//	DB_DSN="user:pass@/mydb"   # inline comment
func (m *Manager) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip blanks and comment-only lines.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip inline comments (anything after an unquoted #).
		if idx := strings.Index(line, " #"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		// Strip surrounding quotes.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if key != "" {
			m.values[key] = val
		}
	}

	return scanner.Err()
}

// Set manually assigns a key/value pair. Useful for testing or runtime overrides.
func (m *Manager) Set(key, value string) {
	m.values[key] = value
}

// Get returns the raw string value for key, or an empty string if not found.
func (m *Manager) Get(key string) string {
	return m.values[key]
}

// GetOrDefault returns the value for key, falling back to the provided default
// if the key is absent or empty.
func (m *Manager) GetOrDefault(key, fallback string) string {
	if v, ok := m.values[key]; ok && v != "" {
		return v
	}
	return fallback
}

// GetInt returns the integer value for key, or fallback if the key is absent
// or cannot be parsed as an integer.
func (m *Manager) GetInt(key string, fallback int) int {
	v, ok := m.values[key]
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// GetBool returns the boolean value for key. Accepts "true", "1", "yes" (case-insensitive)
// as truthy. Returns fallback if the key is absent or unrecognized.
func (m *Manager) GetBool(key string, fallback bool) bool {
	v, ok := m.values[key]
	if !ok || v == "" {
		return fallback
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}
