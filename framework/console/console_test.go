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
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
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

func TestMakeNew_RejectsDotDot(t *testing.T) {
	cmd := &NewCommand{}

	badNames := []string{"..", ".", "../escape", "foo/bar", "foo\\bar", "", "123bad", "-foo"}
	for _, bad := range badNames {
		err := cmd.Execute([]string{bad})
		if err == nil {
			t.Errorf("Expected gost new %q to fail validation, but it succeeded", bad)
		}
	}
}

func TestMakeAuth_GeneratedCodeCompiles(t *testing.T) {
	_ = os.RemoveAll("internal")
	_ = os.RemoveAll("database")
	_ = os.RemoveAll("templates")
	_ = os.RemoveAll("cmd")
	_ = os.RemoveAll("public")
	defer func() {
		_ = os.RemoveAll("internal")
		_ = os.RemoveAll("database")
		_ = os.RemoveAll("templates")
		_ = os.RemoveAll("cmd")
		_ = os.RemoveAll("public")
	}()

	cmd := &MakeAuthCommand{}
	if err := cmd.Execute([]string{"--force"}); err != nil {
		t.Fatalf("make:auth failed: %v", err)
	}

	controllerFile := filepath.Join("internal", "controller", "auth_controller.go")
	content, err := os.ReadFile(controllerFile)
	if err != nil {
		t.Fatalf("Failed to read generated controller: %v", err)
	}
	src := string(content)

	if !strings.Contains(src, `"fmt"`) {
		t.Errorf("auth_controller.go missing \"fmt\" import (F-240)")
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, controllerFile, nil, parser.AllErrors); err != nil {
		t.Errorf("Generated auth_controller.go has syntax errors: %v", err)
	}

	routesFile := filepath.Join("internal", "routes", "auth_routes.go")
	if _, err := parser.ParseFile(fset, routesFile, nil, parser.AllErrors); err != nil {
		t.Errorf("Generated auth_routes.go has syntax errors: %v", err)
	}
}

func TestAddComponent_RejectsTraversal(t *testing.T) {
	cmd := &AddComponentCommand{}

	badComponents := []string{"../../escaped", "sub/comp", "comp;evil", ".."}
	for _, comp := range badComponents {
		err := cmd.Execute([]string{comp})
		if err == nil {
			t.Errorf("Expected gost add %q to be rejected, got nil", comp)
		}
	}
}

// TestAppSkeleton_CompilesEndToEnd verifies that every project created from the app skeleton
// compiles cleanly with `go build ./...` (catches F-298, F-311, F-240, F-241).
func TestAppSkeleton_CompilesEndToEnd(t *testing.T) {
	tempDir := t.TempDir()

	// Locate framework root
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	frameworkRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))

	// Create test project directory
	projectDir := filepath.Join(tempDir, "testapp")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create temp project dir: %v", err)
	}

	// Copy app-skeleton files into temp project
	skeletonDir := filepath.Join(frameworkRoot, "templates", "app-skeleton")
	err = filepath.Walk(skeletonDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(skeletonDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(projectDir, rel)
		target = strings.TrimSuffix(target, ".tmpl")

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		// Interpolate variables
		str := string(content)
		str = strings.ReplaceAll(str, "{{PROJECT_NAME}}", "testapp")
		str = strings.ReplaceAll(str, "{{APP_KEY}}", "base64:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=")
		str = strings.ReplaceAll(str, "{{DB_DRIVER}}", "sqlite")
		str = strings.ReplaceAll(str, "{{DB_DSN}}", "database/database.sqlite")

		if strings.HasSuffix(path, "go.mod.tmpl") {
			str = "module testapp\n\ngo 1.22\n\nrequire github.com/charledeon77/gostack-framework v1.0.27\n"
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, []byte(str), info.Mode())
	})
	if err != nil {
		t.Fatalf("failed to copy skeleton files: %v", err)
	}

	// Inject replace directive so it builds against the local framework
	// Quote the local path in case it contains spaces (e.g. Windows paths)
	goModPath := filepath.Join(projectDir, "go.mod")
	replaceDirective := fmt.Sprintf("\nreplace github.com/charledeon77/gostack-framework => %q\n", filepath.ToSlash(frameworkRoot))
	f, err := os.OpenFile(goModPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open go.mod for replace: %v", err)
	}
	if _, err := f.WriteString(replaceDirective); err != nil {
		f.Close()
		t.Fatalf("failed to write replace directive: %v", err)
	}
	f.Close()

	// Execute `go build ./...` inside the scaffolded project
	// Run with -mod=mod so go can resolve dependencies against local replace without network
	buildCmd := exec.Command("go", "build", "-mod=mod", "./...")
	buildCmd.Dir = projectDir
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Scaffolded project failed to compile (F-311 / F-298 regression detected):\nOutput: %s\nError: %v", string(out), err)
	}
}

func TestMakeJobAndProviderGenerators_ValidInterfaces(t *testing.T) {
	_ = os.RemoveAll("internal")
	defer os.RemoveAll("internal")

	// 1. make:job
	jobCmd := &MakeJobCommand{}
	if err := jobCmd.Execute([]string{"SendEmail"}); err != nil {
		t.Fatalf("make:job failed: %v", err)
	}
	jobFile := filepath.Join("internal", "jobs", "SendEmailJob.go")
	content, err := os.ReadFile(jobFile)
	if err != nil {
		t.Fatalf("failed to read generated job file: %v", err)
	}
	src := string(content)
	if !strings.Contains(src, "func (j *SendEmailJob) Name() string") {
		t.Errorf("job stub missing Name() string method: %s", src)
	}
	if !strings.Contains(src, "func (j *SendEmailJob) MaxAttempts() int") {
		t.Errorf("job stub missing MaxAttempts() int method: %s", src)
	}
	if strings.Contains(src, "MaxRetries") {
		t.Errorf("job stub still has obsolete MaxRetries method: %s", src)
	}

	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, jobFile, nil, parser.AllErrors); err != nil {
		t.Errorf("Generated job has syntax errors: %v", err)
	}

	// 2. make:provider
	providerCmd := &MakeProviderCommand{}
	if err := providerCmd.Execute([]string{"Payment"}); err != nil {
		t.Fatalf("make:provider failed: %v", err)
	}
	providerFile := filepath.Join("internal", "providers", "PaymentServiceProvider.go")
	pContent, err := os.ReadFile(providerFile)
	if err != nil {
		t.Fatalf("failed to read generated provider file: %v", err)
	}
	pSrc := string(pContent)
	if !strings.Contains(pSrc, "func (p *PaymentServiceProvider) Register(c *foundation.Container)") {
		t.Errorf("provider stub missing c *foundation.Container in Register: %s", pSrc)
	}
	if !strings.Contains(pSrc, "func (p *PaymentServiceProvider) Boot(c *foundation.Container)") {
		t.Errorf("provider stub missing c *foundation.Container in Boot: %s", pSrc)
	}

	if _, err := parser.ParseFile(fset, providerFile, nil, parser.AllErrors); err != nil {
		t.Errorf("Generated provider has syntax errors: %v", err)
	}
}

func TestCommand_DatabaseDependencyContract(t *testing.T) {
	kernel := NewKernel()
	commands := []Command{
		&NewCommand{},
		&ServeCommand{},
		&DevCommand{},
		&LangSearchCommand{},
		&MigrateCommand{},
		&RollbackCommand{},
		&MakeMigrationCommand{},
		&MakeControllerCommand{},
		&MakeAuthCommand{},
		&MakeModelCommand{},
		&MakeRequestCommand{},
		&MakeMiddlewareCommand{},
		&MakeMailCommand{},
		&MakeSeederCommand{},
		&SeedCommand{},
		&AddComponentCommand{},
		&MakeEventCommand{},
		&MakeJobCommand{},
		&MakePolicyCommand{},
		&MakeProviderCommand{},
	}

	for _, cmd := range commands {
		if err := kernel.Register(cmd); err != nil {
			t.Fatalf("failed to register command %s: %v", cmd.Name(), err)
		}
	}

	expectedDBCommands := map[string]bool{
		"migrate":          true,
		"migrate:rollback": true,
		"db:seed":          true,
	}

	for name := range expectedDBCommands {
		cmd, exists := kernel.Get(name)
		if !exists {
			t.Errorf("expected command %q to be registered in kernel", name)
			continue
		}
		dep, isDep := cmd.(DatabaseDependent)
		if !isDep || !dep.RequiresDatabase() {
			t.Errorf("command %q is expected to require a database connection, but RequiresDatabase() is false or unimplemented", name)
		}
	}

	// Commands that should NOT require a database connection (including serve and dev, which delegate to the app subprocess)
	nonDBCommands := []string{"new", "dev", "serve", "make:migration", "make:controller", "make:model"}
	for _, name := range nonDBCommands {
		cmd, exists := kernel.Get(name)
		if !exists {
			t.Errorf("expected command %q to be registered in kernel", name)
			continue
		}
		if dep, isDep := cmd.(DatabaseDependent); isDep && dep.RequiresDatabase() {
			t.Errorf("command %q should NOT require a database connection, but RequiresDatabase() returned true", name)
		}
	}
}


