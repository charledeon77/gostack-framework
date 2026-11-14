package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	compiled, _, err := c.compileHTML("test", html)
	if err != nil {
		t.Fatalf("Unexpected compile error: %v", err)
	}
	
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
	compiledForeach, _, err := c.compileHTML("test", foreachHTML)
	if err != nil {
		t.Fatalf("Unexpected compile error in foreach: %v", err)
	}

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

// ─── SEC-26 & SEC-27 Tests ──────────────────────────────────────────────────

func TestCompiler_SEC26_HostileComponentName(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_sec26_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "components")
	outDir := filepath.Join(tmpDir, "output")
	_ = os.MkdirAll(compRoot, 0755)
	_ = os.MkdirAll(outDir, 0755)

	hostileName := "bad_name; os.Exit(0);"
	hostileDir := filepath.Join(compRoot, hostileName)
	if err := os.MkdirAll(hostileDir, 0755); err != nil {
		t.Skip("OS does not permit hostile dirname creation")
	}

	c := NewAssetCompiler(compRoot, outDir)
	err = c.Run()
	if err == nil {
		t.Fatal("Expected compiler.Run to abort on hostile component name, got nil")
	}
	if !strings.Contains(err.Error(), hostileName) {
		t.Errorf("Expected error to name the offending component %q, got: %v", hostileName, err)
	}
}

func TestCompiler_SEC26_HostileExpression(t *testing.T) {
	c := NewAssetCompiler("", "")

	// 1. Hostile {{ os.RemoveAll("/") }} must abort with line number
	html := "<div>\n  <p>Hello</p>\n  {{ os.RemoveAll(\"/\") }}\n</div>"
	_, _, err := c.compileHTML("my-comp", html)
	if err == nil {
		t.Fatal("Expected hostile expression {{ os.RemoveAll(\"/\") }} to fail compilation, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "my-comp:3:") {
		t.Errorf("Expected error to report line 3 for hostile expression, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "os.RemoveAll") {
		t.Errorf("Expected error to mention the hostile expression, got: %s", errMsg)
	}

	// 2. Hostile @if(os.Exit(0)) must abort
	ifHTML := "<div>\n@if(os.Exit(0))\n  <p>Dead</p>\n@endif\n</div>"
	_, _, err = c.compileHTML("my-comp", ifHTML)
	if err == nil {
		t.Fatal("Expected hostile @if(os.Exit(0)) to fail compilation, got nil")
	}
	if !strings.Contains(err.Error(), "my-comp:2:") {
		t.Errorf("Expected error to report line 2 for hostile @if, got: %s", err.Error())
	}
}

func TestCompiler_SEC26_LegitimateExpressions(t *testing.T) {
	c := NewAssetCompiler("", "")

	html := `<div>
  <h1>{{ .Title }}</h1>
  <p>{{ .Items | truncate(20) }}</p>
  @if(.Active)
    <span>Active</span>
  @endif
</div>`

	compiled, _, err := c.compileHTML("blog-post", html)
	if err != nil {
		t.Fatalf("Expected legitimate expressions to compile cleanly, got: %v", err)
	}
	if !strings.Contains(compiled, `ui.Evaluate(data, "Title")`) {
		t.Errorf("Expected compiled output to evaluate Title, got:\n%s", compiled)
	}
	if !strings.Contains(compiled, `ui.ApplyFilter`) || !strings.Contains(compiled, `"truncate"`) {
		t.Errorf("Expected compiled output to apply truncate filter, got:\n%s", compiled)
	}
	if !strings.Contains(compiled, `if ui.EvaluateBool(data, "Active") {`) {
		t.Errorf("Expected compiled output to evaluate Active bool, got:\n%s", compiled)
	}
}

func TestCompiler_SEC27_TraversalAndLayouts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_sec27_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	outDir := filepath.Join(tmpDir, "output")
	_ = os.MkdirAll(compRoot, 0755)
	_ = os.MkdirAll(outDir, 0755)

	c := NewAssetCompiler(compRoot, outDir)

	// 1. Path traversal in @include must abort
	traversalHTML := `<div>@include("../../../../etc/passwd")</div>`
	_, _, err = c.compileHTML("traversal-comp", traversalHTML)
	if err == nil {
		t.Fatal("Expected traversal @include to abort compilation, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") && !strings.Contains(err.Error(), "traversal") {
		t.Errorf("Expected traversal security violation error, got: %v", err)
	}

	// 2. Missing layout in @extends must abort with the missing path named
	missingLayoutHTML := `@extends("layouts/nonexistent_layout")
@section('content')
  <p>Test</p>
@endsection`
	_, _, err = c.compileHTML("missing-layout-comp", missingLayoutHTML)
	if err == nil {
		t.Fatal("Expected missing layout in @extends to abort compilation, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent_layout") {
		t.Errorf("Expected error to mention missing layout path 'nonexistent_layout', got: %v", err)
	}

	// 3. Nested includes still resolve correctly
	partialsDir := filepath.Join(compRoot, "partials")
	_ = os.MkdirAll(partialsDir, 0755)
	headerPath := filepath.Join(partialsDir, "header.html")
	navPath := filepath.Join(partialsDir, "nav.html")

	_ = os.WriteFile(navPath, []byte(`<nav>Navigation</nav>`), 0644)
	_ = os.WriteFile(headerPath, []byte(`<header>@include("partials/nav")</header>`), 0644)

	nestedHTML := `<div>@include("partials/header")</div>`
	compiled, _, err := c.compileHTML("nested-comp", nestedHTML)
	if err != nil {
		t.Fatalf("Expected nested includes to resolve, got error: %v", err)
	}
	if !strings.Contains(compiled, `<header><nav>Navigation</nav></header>`) {
		t.Errorf("Expected nested includes to be inlined, got compiled:\n%s", compiled)
	}
}

func TestHotReloadWatcher_ServeSSE(t *testing.T) {
	watcher := NewHotReloadWatcher(nil, 500*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/__gostack_reload", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		watcher.ServeSSE(rec, req)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// Broadcast reload event
	watcher.broadcast()
	time.Sleep(50 * time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ServeSSE did not terminate after context cancellation")
	}

	body := rec.Body.String()
	if !strings.Contains(body, ": connected\n\n") {
		t.Errorf("expected ': connected' in SSE stream, got %q", body)
	}
	if !strings.Contains(body, "data: reload\n\n") {
		t.Errorf("expected 'data: reload' in SSE stream, got %q", body)
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", rec.Header().Get("Content-Type"))
	}
}
