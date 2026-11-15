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

// TestCompiler_TransAndDirectivesCombined is the golden test for the UI compiler's
// Category 1 fixes shipped in v1.0.16. It verifies in a single template that:
//
//  1. trans("key")        → emits io.WriteString(w, trans("key"))  — no ui.Escape wrapper
//  2. transRaw("key")     → emits io.WriteString(w, transRaw("key"))
//  3. transChoice("k", 3) → emits io.WriteString(w, transChoice("k", 3))
//  4. <!-- @include("x") --> inside an HTML comment is stripped before scanning,
//     so it never reaches the @include resolver and never causes a false
//     "circular or excessively deep @include" error.
//  5. @if / @endif and @foreach / @endforeach directives still compile correctly
//     alongside the new trans calls.
func TestCompiler_TransAndDirectivesCombined(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	componentsDir := filepath.Join(dir, "components")
	outputDir := filepath.Join(dir, "out")

	// Create the component directory and HTML template.
	compDir := filepath.Join(componentsDir, "greeting")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Template exercises all five requirements above.
	// @foreach uses "list as item" syntax: list comes first, then the loop variable.
	template := `<!-- @include("partials/commented") -->
@if(.Active)
<p>{{ trans("welcome") }}</p>
<p>{{ transRaw("html.content") }}</p>
<p>{{ transChoice("items", 3) }}</p>
@endif
@foreach(.Items as item)
<li>{{ .Name }}</li>
@endforeach`

	htmlPath := filepath.Join(compDir, "greeting.html")
	if err := os.WriteFile(htmlPath, []byte(template), 0644); err != nil {
		t.Fatalf("write html: %v", err)
	}

	compiler := NewAssetCompiler(componentsDir, outputDir)
	if err := compiler.Run(); err != nil {
		t.Fatalf("compiler.Run() unexpected error: %v", err)
	}

	genPath := filepath.Join(outputDir, "gostack_components_gen.go")
	src, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	output := string(src)

	// --- Requirement 1: trans emitted without ui.Escape ---
	if !strings.Contains(output, `io.WriteString(w, trans("welcome"))`) {
		t.Errorf("expected io.WriteString(w, trans(\"welcome\")) in output; got:\n%s", output)
	}
	// Must NOT be wrapped in ui.Escape.
	if strings.Contains(output, `ui.Escape(trans(`) {
		t.Errorf("trans() must NOT be wrapped in ui.Escape; got:\n%s", output)
	}

	// --- Requirement 2: transRaw emitted without ui.Escape ---
	if !strings.Contains(output, `io.WriteString(w, transRaw("html.content"))`) {
		t.Errorf("expected io.WriteString(w, transRaw(\"html.content\")) in output; got:\n%s", output)
	}

	// --- Requirement 3: transChoice with numeric second arg ---
	if !strings.Contains(output, `io.WriteString(w, transChoice("items", 3))`) {
		t.Errorf("expected io.WriteString(w, transChoice(\"items\", 3)) in output; got:\n%s", output)
	}

	// --- Requirement 4: commented @include must NOT appear in generated output ---
	if strings.Contains(output, `"partials/commented"`) {
		t.Errorf("commented @include must be stripped; found 'partials/commented' in output:\n%s", output)
	}

	// --- Requirement 5: @if/@foreach directives still compile ---
	if !strings.Contains(output, `ui.EvaluateBool(data, "Active")`) {
		t.Errorf("expected @if(.Active) to compile to EvaluateBool; got:\n%s", output)
	}
	if !strings.Contains(output, `ui.EvaluateSlice(data, "Items")`) {
		t.Errorf("expected @foreach(item in .Items) to compile to EvaluateSlice; got:\n%s", output)
	}
}

// TestIsTransCall_Validation ensures the isTransCall security boundary holds:
// valid calls are accepted, injection attempts are rejected.
func TestIsTransCall_Validation(t *testing.T) {
	t.Parallel()

	accept := []string{
		`trans("welcome")`,
		`trans("auth.login.title")`,
		`transRaw("html.content")`,
		`transChoice("items", 3)`,
		`transChoice("count.items", 100)`,
	}
	for _, expr := range accept {
		if !isTransCall(expr) {
			t.Errorf("isTransCall(%q) = false; want true", expr)
		}
	}

	reject := []string{
		`trans()`,                       // empty key
		`trans(variable)`,               // non-string-literal key — injection risk
		`trans("key", os.Exit(1))`,      // arbitrary Go in second arg
		`trans("key" + "x")`,            // concatenation — not a plain literal
		`fmt.Sprintf("x")`,              // completely different function
		`transChoice("k", variable)`,    // non-literal second arg
		`trans("key") + doSomething()`,  // trailing arbitrary Go
		`transRaw`,                      // no call parens
		``,                              // empty string
	}
	for _, expr := range reject {
		if isTransCall(expr) {
			t.Errorf("isTransCall(%q) = true; want false (should be rejected)", expr)
		}
	}
}

// TestCompiler_BundledStaticAssetsAndRegistration verifies that:
// 1. Component CSS and JS are bundled into physical files under public/assets/css and public/assets/js.
// 2. Short content hashes are computed and registered in gostack_components_gen.go.
// 3. WriteMasterAssetBlock emits external <link> and <script defer> tags instead of inline blocks.
func TestCompiler_BundledStaticAssetsAndRegistration(t *testing.T) {
	dir := t.TempDir()
	componentsDir := filepath.Join(dir, "components")
	outputDir := filepath.Join(dir, "out")
	publicAssetsDir := filepath.Join(dir, "public", "assets")

	compDir := filepath.Join(componentsDir, "button")
	if err := os.MkdirAll(compDir, 0755); err != nil {
		t.Fatalf("mkdir comp: %v", err)
	}

	// Button HTML, CSS, JS
	if err := os.WriteFile(filepath.Join(compDir, "button.html"), []byte("<button>{{ .Label }}</button>"), 0644); err != nil {
		t.Fatalf("write html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "button.css"), []byte(".btn { background: blue; }"), 0644); err != nil {
		t.Fatalf("write css: %v", err)
	}
	if err := os.WriteFile(filepath.Join(compDir, "button.js"), []byte("console.log('button');"), 0644); err != nil {
		t.Fatalf("write js: %v", err)
	}

	compiler := NewAssetCompiler(componentsDir, outputDir, publicAssetsDir)
	if err := compiler.Run(); err != nil {
		t.Fatalf("compiler.Run() failed: %v", err)
	}

	// 1. Verify CSS bundle exists and contains scoped CSS + GoStackCoreCSS
	cssPath := filepath.Join(publicAssetsDir, "css", "gostack.css")
	cssData, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("expected bundled css at %s: %v", cssPath, err)
	}
	if !strings.Contains(string(cssData), "background: blue;") {
		t.Errorf("expected button css in bundle, got:\n%s", string(cssData))
	}
	if !strings.Contains(string(cssData), "[gs-cloak]") {
		t.Errorf("expected [gs-cloak] rule in css bundle")
	}
	if strings.Contains(string(cssData), "@font-face") || strings.Contains(string(cssData), "data:font") {
		t.Errorf("framework core css must NOT embed raw fonts; fonts belong to applications")
	}

	// 2. Verify JS bundle exists and contains GlideJS + component script
	jsPath := filepath.Join(publicAssetsDir, "js", "gostack.js")
	jsData, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("expected bundled js at %s: %v", jsPath, err)
	}
	if !strings.Contains(string(jsData), "console.log('button');") {
		t.Errorf("expected button js in bundle, got:\n%s", string(jsData))
	}
	if strings.Contains(string(jsData), "createElement('style')") {
		t.Errorf("Glide runtime must NOT inject runtime style elements; got:\n%s", string(jsData))
	}

	// 3. Verify hot reload script was written
	hotReloadPath := filepath.Join(publicAssetsDir, "js", "gostack-hot-reload.js")
	if _, err := os.Stat(hotReloadPath); err != nil {
		t.Fatalf("expected hot reload script at %s: %v", hotReloadPath, err)
	}

	// 4. Verify generated Go source registers versioned asset URLs
	genGoPath := filepath.Join(outputDir, "gostack_components_gen.go")
	genSource, err := os.ReadFile(genGoPath)
	if err != nil {
		t.Fatalf("read generated go source: %v", err)
	}
	genStr := string(genSource)
	if !strings.Contains(genStr, `ui.RegisterAssetURLs("/assets/css/gostack.css?v=`) {
		t.Errorf("expected RegisterAssetURLs with version hash in generated code; got:\n%s", genStr)
	}

	// 5. Verify WriteMasterAssetBlock emits external links
	RegisterAssetURLs("/assets/css/gostack.css?v=test1234", "/assets/js/gostack.js?v=test5678")
	var buf strings.Builder
	WriteMasterAssetBlock(&buf)
	assetBlock := buf.String()

	if !strings.Contains(assetBlock, `<link rel="stylesheet" href="/assets/css/gostack.css?v=test1234">`) {
		t.Errorf("expected external stylesheet link in master block; got:\n%s", assetBlock)
	}
	if !strings.Contains(assetBlock, `<script defer src="/assets/js/gostack.js?v=test5678"></script>`) {
		t.Errorf("expected external script link in master block; got:\n%s", assetBlock)
	}
	if strings.Contains(assetBlock, "<style id=\"gostack-core-styles\">") {
		t.Errorf("master asset block must NOT contain inline style tag; got:\n%s", assetBlock)
	}
}
