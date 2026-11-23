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
	"io"
	"os"
	"path/filepath"
	"strings"
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
	exists, err := store.Exists(testPath)
	if err != nil {
		t.Fatalf("Exists returned unexpected error: %v", err)
	}
	if exists {
		t.Fatal("expected file to not exist initially")
	}

	// 2. Put file
	if err := store.Put(testPath, testData); err != nil {
		t.Fatalf("failed to put file: %v", err)
	}

	// 3. Assert file exists now
	exists, err = store.Exists(testPath)
	if err != nil {
		t.Fatalf("Exists returned unexpected error after write: %v", err)
	}
	if !exists {
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
	exists, err = store.Exists(testPath)
	if err != nil {
		t.Fatalf("Exists returned unexpected error after delete: %v", err)
	}
	if exists {
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

	// 4. Exists should return false (not an error — traversal is a path error, not a missing-file error)
	if exists, _ := store.Exists(traversalPath); exists {
		t.Error("expected Exists with directory traversal path to return false, but it returned true")
	}
}

func TestLocalStorage_Streaming(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostack-storage-stream-test-*")
	if err != nil {
		t.Fatalf("failed to create temp test directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store, err := NewLocalStorage(tempDir)
	if err != nil {
		t.Fatalf("failed to create LocalStorage driver: %v", err)
	}

	testPath := "streams/large_attachment.dat"
	content := "This is a stream of bytes representing a large email attachment."
	reader := strings.NewReader(content)

	// 1. PutStream
	if err := store.PutStream(testPath, reader); err != nil {
		t.Fatalf("PutStream failed: %v", err)
	}

	// 2. Exists
	exists, err := store.Exists(testPath)
	if err != nil {
		t.Fatalf("Exists after PutStream returned unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("expected streamed file to exist")
	}

	// 3. GetStream
	rc, err := store.GetStream(testPath)
	if err != nil {
		t.Fatalf("GetStream failed: %v", err)
	}
	defer rc.Close()

	streamedBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read from stream: %v", err)
	}

	if string(streamedBytes) != content {
		t.Errorf("expected stream content %q, got %q", content, string(streamedBytes))
	}

	// 4. Traversal protection on streaming
	traversalPath := filepath.Join("..", "escaped_stream.txt")
	if err := store.PutStream(traversalPath, strings.NewReader("bad")); err == nil {
		t.Error("expected PutStream with directory traversal path to fail, but it succeeded")
	}
	if _, err := store.GetStream(traversalPath); err == nil {
		t.Error("expected GetStream with directory traversal path to fail, but it succeeded")
	}
}
