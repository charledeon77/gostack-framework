package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAssetCompilerRun(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}

	rootPath := filepath.Join(cwd, "..", "..")
	componentsPath := filepath.Join(rootPath, "templates", "components")
	outputPath := filepath.Join(rootPath, "cmd", "app")

	// Verify or create directories if they don't exist
	if err := os.MkdirAll(componentsPath, 0755); err != nil {
		t.Fatalf("failed to create components path: %v", err)
	}
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		t.Fatalf("failed to create output path: %v", err)
	}

	compiler := NewAssetCompiler(componentsPath, outputPath)
	err = compiler.Run()
	if err != nil {
		t.Fatalf("Compiler.Run failed: %v", err)
	}

	genFile := filepath.Join(outputPath, "gostack_components_gen.go")
	if _, err := os.Stat(genFile); os.IsNotExist(err) {
		t.Fatalf("Expected generated file %s does not exist", genFile)
	}
}
