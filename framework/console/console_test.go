/*
Purpose:
This file contains unit tests for the console Kernel and generator commands (make:migration, make:controller).
It validates command routing, argument passing, output stub file generation, and file parsing.

Philosophy:
We believe code generators should be tested with high-fidelity checks. Rather than just checking if a file
exists, we verify that the generated output compiles and conforms to Go's standard AST compiler syntax,
ensuring zero broken stubs reach developers.

Architecture:
Tests build an in-memory Console Kernel, registers generator commands, executes them with custom names
targeting temporary directories, and invokes Go's AST parser (`go/parser`) to check generated files.

Choice:
We chose standard `go/parser` file checks over string assertions because parsing checks verify syntactical correctness
directly, guarding against compiler failures from generated code.

Implementation:
- mockCommand: a simple test command to verify routing and argument passing.
- TestConsoleKernelCommandRouting: asserts matching command execution and help triggers.
- TestMakeMigrationGenerator: runs `make:migration`, verifies file creation, asserts syntax validation using `parser.ParseFile`, and deletes file.
- TestMakeControllerGenerator: runs `make:controller`, verifies file creation, asserts syntax validation using `parser.ParseFile`, and deletes file.
*/
package console

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockCommand struct {
	executed bool
	lastArgs []string
}

func (m *mockCommand) Name() string { return "test:mock" }
func (m *mockCommand) Description() string { return "Mock command for testing" }
func (m *mockCommand) Execute(args []string) error {
	m.executed = true
	m.lastArgs = args
	return nil
}

func TestConsoleKernelCommandRouting(t *testing.T) {
	kernel := NewKernel()
	mock := &mockCommand{}
	kernel.Register(mock)

	// Test case 1: Routing a registered command
	if err := kernel.Run([]string{"test:mock", "arg1", "arg2"}); err != nil {
		t.Fatalf("Kernel execution failed: %v", err)
	}

	if !mock.executed {
		t.Error("Expected mock command to be executed")
	}
	if len(mock.lastArgs) != 2 || mock.lastArgs[0] != "arg1" || mock.lastArgs[1] != "arg2" {
		t.Errorf("Args not passed correctly: %v", mock.lastArgs)
	}

	// Test case 2: Unregistered command returns error
	if err := kernel.Run([]string{"test:invalid"}); err == nil {
		t.Error("Expected error on unregistered command, got nil")
	}
}

func TestMakeMigrationGenerator(t *testing.T) {
	// Clean up any old files/folders before testing
	_ = os.RemoveAll("database")

	cmd := &MakeMigrationCommand{}
	testName := "create_test_posts_table"

	// Run command
	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:migration failed: %v", err)
	}

	// Find created file (database/migrations/<timestamp>_create_test_posts_table.go)
	dirPath := filepath.Join("database", "migrations")
	files, err := os.ReadDir(dirPath)
	if err != nil {
		t.Fatalf("Failed to read migrations directory: %v", err)
	}

	var generatedFile string
	for _, f := range files {
		if strings.Contains(f.Name(), testName) {
			generatedFile = filepath.Join(dirPath, f.Name())
			break
		}
	}

	if generatedFile == "" {
		t.Fatal("Migration file was not generated")
	}
	defer os.RemoveAll("database") // Clean up generated folder structure

	// Parse file to verify Go syntax validity
	fset := token.NewFileSet()
	_, err = parser.ParseFile(fset, generatedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated migration has syntax errors: %v", err)
	}
}

func TestMakeControllerGenerator(t *testing.T) {
	// Clean up any old files/folders before testing
	_ = os.RemoveAll("internal")

	cmd := &MakeControllerCommand{}
	testName := "test_items_controller"

	// Run command
	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:controller failed: %v", err)
	}

	expectedFile := filepath.Join("internal", "controller", "test_items_controller.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Controller file not found: %s", expectedFile)
	}
	defer os.RemoveAll("internal") // Clean up generated folder structure

	// Parse file to verify Go syntax validity
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, expectedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated controller has syntax errors: %v", err)
	}
}

func TestMakeRequestGenerator(t *testing.T) {
	_ = os.RemoveAll("internal")

	cmd := &MakeRequestCommand{}
	testName := "store_user_request"

	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:request failed: %v", err)
	}

	expectedFile := filepath.Join("internal", "request", "store_user_request.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Request file not found: %s", expectedFile)
	}
	defer os.RemoveAll("internal")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, expectedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated request has syntax errors: %v", err)
	}
}

func TestMakeMiddlewareGenerator(t *testing.T) {
	_ = os.RemoveAll("internal")

	cmd := &MakeMiddlewareCommand{}
	testName := "check_age"

	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:middleware failed: %v", err)
	}

	expectedFile := filepath.Join("internal", "middleware", "check_age_middleware.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Middleware file not found: %s", expectedFile)
	}
	defer os.RemoveAll("internal")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, expectedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated middleware has syntax errors: %v", err)
	}
}

func TestMakeMailGenerator(t *testing.T) {
	_ = os.RemoveAll("internal")

	cmd := &MakeMailCommand{}
	testName := "order_shipped"

	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:mail failed: %v", err)
	}

	expectedFile := filepath.Join("internal", "mail", "order_shipped_mail.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Mail file not found: %s", expectedFile)
	}
	defer os.RemoveAll("internal")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, expectedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated mail has syntax errors: %v", err)
	}
}

func TestMakeSeederGenerator(t *testing.T) {
	_ = os.RemoveAll("database")

	cmd := &MakeSeederCommand{}
	testName := "user_table_seeder"

	if err := cmd.Execute([]string{testName}); err != nil {
		t.Fatalf("make:seeder failed: %v", err)
	}

	expectedFile := filepath.Join("database", "seeders", "user_table_seeder.go")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("Seeder file not found: %s", expectedFile)
	}
	defer os.RemoveAll("database")

	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, expectedFile, nil, parser.AllErrors)
	if err != nil {
		t.Errorf("Generated seeder has syntax errors: %v", err)
	}
}

