/*
Purpose:
This file implements the LocalStorage driver for the Vault storage subsystem of GoStack.
It provides a standard file-system storage backend complying with the Storage interface.

Philosophy:
Storage engines must be unified and secure. Writing raw paths directly in application
logic causes host platform locks. Additionally, un-sanitized user file path inputs
expose applications to directory traversal exploits. This driver prevents that by
resolving all operations strictly within a configured root directory.

Architecture:
A standalone framework package (`github.com/charledeon77/gostack/framework/storage`). Implements the
`contract.Storage` interface.

Choice:
We chose a local OS file-system backend using standard `os` and `filepath` primitives,
ensuring zero-dependency footprint. The path resolution uses strict prefix checking
against the root directory to guarantee sandbox safety.

Implementation:
- LocalStorage: implements storage operations.
  - NewLocalStorage(rootDir): initializes and establishes root directory.
  - Put(): writes file content, creating missing subdirectories automatically.
  - Get(): reads file content.
  - Delete(): removes file.
  - Exists(): verifies file presence.
  - resolvePath(): sanitizes and validates bounds constraints.
*/
package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LocalStorage handles file-system operations securely confined to a root path.
type LocalStorage struct {
	rootDir string
}

// NewLocalStorage instantiates a LocalStorage driver bound to the specified root directory.
func NewLocalStorage(rootDir string) (*LocalStorage, error) {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to resolve absolute root path: %w", err)
	}

	// Ensure the root directory exists
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return nil, fmt.Errorf("storage: failed to initialize root directory: %w", err)
	}

	return &LocalStorage{rootDir: absRoot}, nil
}

// Put writes contents to the relative target path, creating parent folders if missing.
func (s *LocalStorage) Put(path string, contents []byte) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return fmt.Errorf("storage: failed to create subdirectories: %w", err)
	}

	if err := os.WriteFile(fullPath, contents, 0644); err != nil {
		return fmt.Errorf("storage: failed to write file: %w", err)
	}

	return nil
}

// Get reads the file contents at the relative target path.
func (s *LocalStorage) Get(path string) ([]byte, error) {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("storage: failed to read file: %w", err)
	}

	return contents, nil
}

// Delete removes the file at the relative target path.
func (s *LocalStorage) Delete(path string) error {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted/non-existent is a success
		}
		return fmt.Errorf("storage: failed to delete file: %w", err)
	}

	return nil
}

// Exists asserts whether the relative target path matches an existing file on disk.
func (s *LocalStorage) Exists(path string) bool {
	fullPath, err := s.resolvePath(path)
	if err != nil {
		return false
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

// resolvePath sanitizes paths and prevents directory traversal attacks.
func (s *LocalStorage) resolvePath(path string) (string, error) {
	// Clean the path to eliminate relative paths (e.g. "../../")
	cleanPath := filepath.Clean(path)

	// Combine with root directory
	fullPath := filepath.Join(s.rootDir, cleanPath)

	// Ensure the resolved absolute path starts with the root directory path prefix
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("storage: failed to resolve absolute target path: %w", err)
	}

	if !strings.HasPrefix(absPath, s.rootDir) {
		return "", fmt.Errorf("storage: security violation: path traversal attempt detected on '%s'", path)
	}

	return absPath, nil
}
