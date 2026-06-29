/*
Purpose:
This file implements unit tests for the GoStack LocalStorage driver.
It validates lifecycle file operations and directory traversal protection mechanisms.

Philosophy:
A storage driver must be secure. We explicitly test directory traversal attacks
to guarantee that security filters actively block illegal reads or writes outside the sandbox.
*/
package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorage_FileLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostack-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp test directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create LocalStorage driver: %v", err)
	}

	testPath := "uploads/documents/report.txt"
	testData := []byte("Hello, GoStack Storage Subsystem!")

	// 1. Assert file does not exist initially
	if store.Exists(testPath) {
		t.Fatal("expected file to not exist initially")
	}

	// 2. Put file
	if err := store.Put(testPath, testData); err != nil {
		t.Fatalf("failed to put file: %v", err)
	}

	// 3. Assert file exists now
	if !store.Exists(testPath) {
		t.Fatal("expected file to exist after write")
	}

	// 4. Get file contents and compare
	readData, err := store.Get(testPath)
	if err != nil {
		t.Fatalf("failed to get file content: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Errorf("read data mismatch. expected '%s', got '%s'", string(testData), string(readData))
	}

	// 5. Delete file
	if err := store.Delete(testPath); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// 6. Assert file is gone
	if store.Exists(testPath) {
		t.Fatal("expected file to not exist after deletion")
	}
}

func TestLocalStorage_DirectoryTraversalDefense(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostack-storage-test-traversal-*")
	if err != nil {
		t.Fatalf("failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to build LocalStorage: %v", err)
	}

	// Traversal attempt to escape root directory
	traversalPath := filepath.Join("..", "escaped.txt")
	testData := []byte("secret")

	// 1. Put should fail
	err = store.Put(traversalPath, testData)
	if err == nil {
		t.Error("expected Put with directory traversal path to fail, but it succeeded")
	}

	// 2. Get should fail
	_, err = store.Get(traversalPath)
	if err == nil {
		t.Error("expected Get with directory traversal path to fail, but it succeeded")
	}

	// 3. Delete should fail
	err = store.Delete(traversalPath)
	if err == nil {
		t.Error("expected Delete with directory traversal path to fail, but it succeeded")
	}

	// 4. Exists should return false
	if store.Exists(traversalPath) {
		t.Error("expected Exists with directory traversal path to return false, but it returned true")
	}
}
