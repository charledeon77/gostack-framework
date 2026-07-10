package ui

import (
	"os"
	"path/filepath"
	"strings"
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

func TestAssetCompiler_CompileHTMLDirectives(t *testing.T) {
	c := NewAssetCompiler("", "")
	
	// Test @if compilation
	html := `@if(.User.IsAdmin)
  <p>Admin</p>
@elseif(.User.IsGuest)
  <p>Guest</p>
@else
  <p>Standard</p>
@endif`
	compiled, _ := c.compileHTML("test", html)
	
	if !strings.Contains(compiled, `if ui.EvaluateBool(data, "User.IsAdmin") {`) {
		t.Errorf("Expected compiled output to contain if statement, got:\n%s", compiled)
	}
	if !strings.Contains(compiled, `} else if ui.EvaluateBool(data, "User.IsGuest") {`) {
		t.Errorf("Expected compiled output to contain elseif statement, got:\n%s", compiled)
	}
	if !strings.Contains(compiled, `} else {`) {
		t.Errorf("Expected compiled output to contain else block, got:\n%s", compiled)
	}

	// Test @foreach compilation
	foreachHTML := `@foreach(.posts as post)
  <p>{{ post.Title }}</p>
@endforeach`
	compiledForeach, _ := c.compileHTML("test", foreachHTML)

	if !strings.Contains(compiledForeach, `for _, postVal := range ui.EvaluateSlice(data, "posts") {`) {
		t.Errorf("Expected compiled output to contain loop statement, got:\n%s", compiledForeach)
	}
	if !strings.Contains(compiledForeach, `data := map[string]any{"$parent": data, "post": postVal}`) {
		t.Errorf("Expected compiled output to contain local scope variables injection, got:\n%s", compiledForeach)
	}
}

func TestAssetCompiler_ScopeCSS(t *testing.T) {
	c := NewAssetCompiler("", "")
	rawCSS := `.btn { color: red; }
#title { font-size: 20px; }`
	expected := `[gs-component="test"] .btn { color: red; }
[gs-component="test"] #title { font-size: 20px; }`

	scoped := c.scopeCSS("test", rawCSS)
	if strings.TrimSpace(scoped) != strings.TrimSpace(expected) {
		t.Errorf("Expected scoped CSS to be:\n%s\n\nGot:\n%s", expected, scoped)
	}
}
