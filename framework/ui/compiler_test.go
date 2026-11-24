package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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

	// 1. Basic rules
	rawCSS := `.btn { color: red; }
#title { font-size: 20px; }`
	scoped, err := c.scopeCSS("test", rawCSS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(scoped, `[gs-component="test"] .btn {`) {
		t.Errorf("Expected scoped .btn, got:\n%s", scoped)
	}
	if !strings.Contains(scoped, `[gs-component="test"] #title {`) {
		t.Errorf("Expected scoped #title, got:\n%s", scoped)
	}

	// 2. Multi-line comments are stripped cleanly without corrupting the next selector
	commentCSS := `/* Sign-in card styles
   for floating inputs */
.floating-field {
  color: blue;
}
/* inline comment */ .subtext { font-size: 12px; }`
	scopedComments, err := c.scopeCSS("auth", commentCSS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(scopedComments, "/*") || strings.Contains(scopedComments, "*/") {
		t.Errorf("Expected all comments to be stripped, got:\n%s", scopedComments)
	}
	if !strings.Contains(scopedComments, `[gs-component="auth"] .floating-field {`) {
		t.Errorf("Expected [gs-component=\"auth\"] .floating-field, got:\n%s", scopedComments)
	}
	if !strings.Contains(scopedComments, `[gs-component="auth"] .subtext {`) {
		t.Errorf("Expected [gs-component=\"auth\"] .subtext, got:\n%s", scopedComments)
	}

	// 3. Comma-separated selectors: each selector must be individually prefixed
	commaCSS := `h1, h2, .custom-heading {
  margin: 0;
}`
	scopedComma, err := c.scopeCSS("headings", commaCSS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedComma := `[gs-component="headings"] h1, [gs-component="headings"] h2, [gs-component="headings"] .custom-heading {`
	if !strings.Contains(scopedComma, expectedComma) {
		t.Errorf("Expected comma-separated selectors to all be scoped, got:\n%s", scopedComma)
	}

	// 4. @keyframes: keyframe steps (from, to, 0%, 100%) must NOT be scoped
	keyframeCSS := `@keyframes slideIn {
  from {
    opacity: 0;
  }
  to {
    opacity: 1;
  }
}`
	scopedKeyframe, err := c.scopeCSS("anim", keyframeCSS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(scopedKeyframe, `[gs-component="anim"] from`) || strings.Contains(scopedKeyframe, `[gs-component="anim"] to`) {
		t.Errorf("Keyframe steps must NOT be prefixed with component selector, got:\n%s", scopedKeyframe)
	}
	if !strings.Contains(scopedKeyframe, "from {") || !strings.Contains(scopedKeyframe, "to {") {
		t.Errorf("Keyframe steps from/to must be preserved, got:\n%s", scopedKeyframe)
	}

	// 5. @media query: inner selectors must be scoped, but @media itself is not prefixed
	mediaCSS := `@media (min-width: 768px) {
  .responsive-box {
    width: 50%;
  }
}`
	scopedMedia, err := c.scopeCSS("layout", mediaCSS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(scopedMedia), `[gs-component="layout"] @media`) {
		t.Errorf("@media must not be prefixed itself, got:\n%s", scopedMedia)
	}
	if !strings.Contains(scopedMedia, `[gs-component="layout"] .responsive-box {`) {
		t.Errorf("Inner selector within @media must be scoped, got:\n%s", scopedMedia)
	}
}

func TestComponentCSS_RejectsRootSelector(t *testing.T) {
	c := NewAssetCompiler("", "")
	badSelectors := []string{
		":root { --primary: red; }",
		":host { display: block; }",
		"html { background: black; }",
		"body { margin: 0; }",
		"body.theme-dark { color: white; }",
		"h1, :root { display: none; }",
	}

	for _, css := range badSelectors {
		_, err := c.scopeCSS("card", css)
		if err == nil {
			t.Errorf("Expected compile error for root selector in CSS: %s", css)
		} else if !strings.Contains(err.Error(), "forbidden root document selector") {
			t.Errorf("Expected forbidden root document selector error, got: %v", err)
		}
	}
}

func TestComponentCSS_RejectsImport(t *testing.T) {
	c := NewAssetCompiler("", "")
	badDirectives := []string{
		`@import url("https://evil.example/x.css");`,
		`@import "global.css";`,
		`@namespace url(http://www.w3.org/1999/xhtml);`,
	}

	for _, css := range badDirectives {
		_, err := c.scopeCSS("badge", css)
		if err == nil {
			t.Errorf("Expected compile error for at-directive in CSS: %s", css)
		} else if !strings.Contains(err.Error(), "forbidden") {
			t.Errorf("Expected forbidden at-directive error, got: %v", err)
		}
	}
}

func TestFilterArg_CannotInjectGoSource(t *testing.T) {
	c := NewAssetCompiler("", "")
	hostileTemplate := `{{ .Name | truncate("a\"); os.RemoveAll(\"C:/\") ; //") }}`
	_, _, err := c.compileHTML("hostile-filter", hostileTemplate)
	if err == nil {
		t.Fatal("Expected compile error for filter argument with quotes and backslashes, got nil")
	}
	if !strings.Contains(err.Error(), "invalid characters") {
		t.Errorf("Expected invalid characters error, got: %v", err)
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
	if err := WriteMasterAssetBlock(&buf); err != nil {
		t.Fatalf("WriteMasterAssetBlock failed: %v", err)
	}
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

func TestCompiler_NoDivWrapperOnFullDocument(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_nodiv_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	layoutsDir := filepath.Join(compRoot, "layouts")
	_ = os.MkdirAll(layoutsDir, 0755)

	layoutContent := `<!DOCTYPE html>
<html lang="en">
<head><title>Test</title></head>
<body>@yield('content')</body>
</html>`
	if err := os.WriteFile(filepath.Join(layoutsDir, "app.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewAssetCompiler(compRoot, "")

	// 1. Document generated via @extends must retain <!DOCTYPE html> at byte 0 and NOT wrap outer doc in <div gs-component="...">
	childDoc := `@extends("layouts/app")
@section('content')
<h1>Hello</h1>
@endsection`
	compiledDoc, _, err := c.compileHTML("home", childDoc)
	if err != nil {
		t.Fatalf("compileHTML failed: %v", err)
	}
	if strings.Contains(compiledDoc, `<div gs-component="home"><!DOCTYPE`) {
		t.Errorf("Full document must not be wrapped in div gs-component at root; got:\n%s", compiledDoc)
	}
	if !strings.Contains(compiledDoc, "<!DOCTYPE html>") {
		t.Errorf("Full document must retain <!DOCTYPE html>; got:\n%s", compiledDoc)
	}
	if !strings.Contains(compiledDoc, `<div gs-component="home">`) {
		t.Errorf("Expected content slot to be wrapped in div gs-component=\"home\"; got:\n%s", compiledDoc)
	}

	// 2. Component fragment must still be wrapped
	fragment := `<div class="card"><h2>Hello</h2></div>`
	compiledFrag, _, err := c.compileHTML("card", fragment)
	if err != nil {
		t.Fatalf("compileHTML failed: %v", err)
	}
	if !strings.Contains(compiledFrag, `<div gs-component="card">`) {
		t.Errorf("Component fragment should be wrapped in div gs-component; got:\n%s", compiledFrag)
	}
}

func TestCompiler_DirectiveNestingValidation(t *testing.T) {
	c := NewAssetCompiler("", "")

	// Unclosed @if
	_, _, err := c.compileHTML("test-unclosed-if", `@if(.flag)
<p>Open</p>`)
	if err == nil {
		t.Fatal("Expected error for unclosed @if, got nil")
	}
	if !strings.Contains(err.Error(), "unclosed @if") {
		t.Errorf("Expected 'unclosed @if' error message, got: %v", err)
	}

	// Unmatched @endif
	_, _, err = c.compileHTML("test-unmatched-endif", `<p>Hello</p>
@endif`)
	if err == nil {
		t.Fatal("Expected error for unmatched @endif, got nil")
	}
	if !strings.Contains(err.Error(), "@endif without matching @if") {
		t.Errorf("Expected '@endif without matching @if' error message, got: %v", err)
	}

	// Unmatched @else
	_, _, err = c.compileHTML("test-unmatched-else", `@else
<p>Hello</p>`)
	if err == nil {
		t.Fatal("Expected error for unmatched @else, got nil")
	}
	if !strings.Contains(err.Error(), "@else without matching @if") {
		t.Errorf("Expected '@else without matching @if' error message, got: %v", err)
	}

	// Unmatched @endforeach
	_, _, err = c.compileHTML("test-unmatched-endforeach", `<p>Loop</p>
@endforeach`)
	if err == nil {
		t.Fatal("Expected error for unmatched @endforeach, got nil")
	}
	if !strings.Contains(err.Error(), "@endforeach without matching @foreach") {
		t.Errorf("Expected '@endforeach without matching @foreach' error message, got: %v", err)
	}

	// Unclosed @foreach
	_, _, err = c.compileHTML("test-unclosed-foreach", `@foreach(.items as item)
<p>Loop</p>`)
	if err == nil {
		t.Fatal("Expected error for unclosed @foreach, got nil")
	}
	if !strings.Contains(err.Error(), "unclosed @foreach") {
		t.Errorf("Expected 'unclosed @foreach' error message, got: %v", err)
	}
}

func TestRegistry_EvaluateSliceMaps(t *testing.T) {
	data := map[string]any{
		"users": map[string]string{
			"c": "Charlie",
			"a": "Alice",
			"b": "Bob",
		},
	}
	items := EvaluateSlice(data, "users")
	if len(items) != 3 {
		t.Fatalf("Expected 3 items from map, got %d", len(items))
	}
	// Because keys are sorted: "a"->"Alice", "b"->"Bob", "c"->"Charlie"
	if items[0] != "Alice" || items[1] != "Bob" || items[2] != "Charlie" {
		t.Errorf("Expected sorted slice [Alice Bob Charlie], got: %v", items)
	}
}

func TestRegistry_EvaluateBoolCaseInsensitive(t *testing.T) {
	cases := []struct {
		input any
		want  bool
	}{
		{"false", false},
		{"False", false},
		{"FALSE", false},
		{"0", false},
		{"no", false},
		{"No", false},
		{"off", false},
		{"", false},
		{"true", true},
		{"True", true},
		{"yes", true},
		{"1", true},
	}

	for _, tc := range cases {
		data := map[string]any{"v": tc.input}
		got := EvaluateBool(data, "v")
		if got != tc.want {
			t.Errorf("EvaluateBool(%q): expected %v, got %v", tc.input, tc.want, got)
		}
	}
}

type unexportedFieldsHolder struct {
	Exported   string
	unexported string
}

func TestRegistry_EvaluateUnexportedStructFieldSafe(t *testing.T) {
	obj := unexportedFieldsHolder{
		Exported:   "visible",
		unexported: "secret",
	}

	// Exported field should resolve
	exp := Evaluate(obj, "Exported")
	if exp != "visible" {
		t.Errorf("Expected 'visible', got %v", exp)
	}

	// Unexported field should safely return nil, NOT panic!
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Evaluate panicked on unexported struct field: %v", r)
		}
	}()

	unexp := Evaluate(obj, "unexported")
	if unexp != "" && unexp != nil {
		t.Errorf("Expected empty value for unexported field, got %v", unexp)
	}
}

func TestCompiler_LayoutYieldComponentBadge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_layout_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	layoutsDir := filepath.Join(compRoot, "layouts")
	_ = os.MkdirAll(layoutsDir, 0755)

	layoutContent := `<!DOCTYPE html>
<html lang="en">
<head>
  <title>@yield('title')</title>
</head>
<body>
  <header>Site Header</header>
  <main>
    @yield('content')
  </main>
</body>
</html>`
	if err := os.WriteFile(filepath.Join(layoutsDir, "app.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewAssetCompiler(compRoot, "")

	childTemplate := `@extends("layouts/app")
@section('title')
Login Page
@endsection
@section('content')
<form action="/login" method="POST">
  <input type="text" name="email" />
  <button type="submit">Sign In</button>
</form>
@endsection`

	compiled, _, err := c.compileHTML("login", childTemplate)
	if err != nil {
		t.Fatalf("compileHTML with layout failed: %v", err)
	}

	// 1. Must retain <!DOCTYPE html> at byte 0 (not wrapped in outer div)
	if !strings.HasPrefix(strings.TrimSpace(compiled), "<!DOCTYPE html>") && !strings.Contains(compiled, "<!DOCTYPE html>") {
		t.Errorf("Expected compiled document to retain <!DOCTYPE html>, got:\n%s", compiled)
	}
	if strings.HasPrefix(strings.TrimSpace(compiled), "<div gs-component=\"login\">") {
		t.Errorf("Document must NOT be wrapped on outer boundary in div gs-component; got:\n%s", compiled)
	}

	// 2. The @yield('title') slot in <head> must NOT be wrapped in a <div>
	if strings.Contains(compiled, "<title><div") || strings.Contains(compiled, "gs-component=\"login\">Login Page") {
		t.Errorf("Title slot in <head> must NOT be wrapped in div gs-component; got:\n%s", compiled)
	}

	// 3. The @yield('content') slot inside <main> MUST be wrapped in <div gs-component="login">
	expectedBadge := `<div gs-component="login">`
	if !strings.Contains(compiled, expectedBadge) {
		t.Errorf("Expected child content in @yield('content') to be badged with %s; got:\n%s", expectedBadge, compiled)
	}
	if !strings.Contains(compiled, `<form action="/login" method="POST">`) {
		t.Errorf("Expected child form to be inside compiled document; got:\n%s", compiled)
	}
}

func TestCompiler_ExtendsChildContainsHtmlBodyError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_err_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	layoutsDir := filepath.Join(compRoot, "layouts")
	_ = os.MkdirAll(layoutsDir, 0755)

	layoutContent := `<!DOCTYPE html><html><body>@yield('content')</body></html>`
	if err := os.WriteFile(filepath.Join(layoutsDir, "app.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewAssetCompiler(compRoot, "")

	// Child template mistakenly authoring <body> inside @section
	badChild := `@extends("layouts/app")
@section('content')
<body>
  <h1>Duplicate Body</h1>
</body>
@endsection`

	_, _, err = c.compileHTML("bad-page", badChild)
	if err == nil {
		t.Fatal("Expected compile error when extending child authors <body>, got nil")
	}
	if !strings.Contains(err.Error(), "must not author <!DOCTYPE>, <html>, <head>, or <body>") {
		t.Errorf("Expected descriptive error about forbidden document tags, got: %v", err)
	}
}

func TestCompiler_ExtendsImplicitContentSection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_implicit_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	layoutsDir := filepath.Join(compRoot, "layouts")
	_ = os.MkdirAll(layoutsDir, 0755)

	layoutContent := `<!DOCTYPE html><html><body>@yield('content')</body></html>`
	if err := os.WriteFile(filepath.Join(layoutsDir, "app.html"), []byte(layoutContent), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewAssetCompiler(compRoot, "")

	// Child template without explicit @section('content')
	implicitChild := `@extends("layouts/app")
<h2>Implicit Page Content</h2>`

	compiled, _, err := c.compileHTML("dashboard", implicitChild)
	if err != nil {
		t.Fatalf("compileHTML failed: %v", err)
	}

	expectedBadge := `<div gs-component="dashboard">`
	if !strings.Contains(compiled, expectedBadge) {
		t.Errorf("Expected implicit child content to be wrapped in %s, got:\n%s", expectedBadge, compiled)
	}
	if !strings.Contains(compiled, `<h2>Implicit Page Content</h2>`) {
		t.Errorf("Expected child content in compiled output, got:\n%s", compiled)
	}
}

func TestCompiler_StandaloneFullDocumentError(t *testing.T) {
	c := NewAssetCompiler("", "")

	// 1. Full document rejected
	fullDoc := `<!DOCTYPE html>
<html lang="en">
<head><title>Standalone</title></head>
<body class="page-body"><h1>Standalone Page</h1></body>
</html>`
	_, _, err := c.compileHTML("account", fullDoc)
	if err == nil {
		t.Fatal("Expected compile error when component authors full document without @extends, got nil")
	}
	if !strings.Contains(err.Error(), "authors root document tags") {
		t.Errorf("Expected descriptive error about root document tags, got: %v", err)
	}

	// 2. Standalone <body> directly rejected
	standaloneBody := `<body class="page-body"><h1>Bare Body</h1></body>`
	_, _, err = c.compileHTML("bare-body", standaloneBody)
	if err == nil {
		t.Fatal("Expected compile error when component authors bare <body> without @extends, got nil")
	}

	// 3. Standalone <head> directly rejected
	standaloneHead := `<head><title>Bare Head</title></head><div>Content</div>`
	_, _, err = c.compileHTML("bare-head", standaloneHead)
	if err == nil {
		t.Fatal("Expected compile error when component authors bare <head> without @extends, got nil")
	}

	// 4. Custom elements and attributes containing "body" must NOT be falsely rejected
	validCustomElement := `<div data-body="custom"><body-widget>Valid Content</body-widget></div>`
	compiled, _, err := c.compileHTML("custom-el", validCustomElement)
	if err != nil {
		t.Fatalf("Valid custom element should not be rejected: %v", err)
	}
	if !strings.Contains(compiled, `<div gs-component="custom-el">`) {
		t.Errorf("Expected custom element component to be wrapped in div gs-component; got:\n%s", compiled)
	}

	// 5. Commented-out document tags must NOT be falsely rejected
	commentedDoc := `<!-- <body>Legacy code</body> -->
<div class="card"><h1>Active Content</h1></div>`
	_, _, err = c.compileHTML("commented", commentedDoc)
	if err != nil {
		t.Fatalf("Commented-out tags should not trigger error: %v", err)
	}
}

func TestCompiler_SmuggledRootDocumentTagsViaInclude(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_comp_smuggle_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	compRoot := filepath.Join(tmpDir, "resources", "components")
	partialsDir := filepath.Join(compRoot, "partials")
	_ = os.MkdirAll(partialsDir, 0755)

	// Partial file smuggling <body>
	smuggledPartial := `<body class="smuggled"><h1>Smuggled</h1></body>`
	if err := os.WriteFile(filepath.Join(partialsDir, "smuggled.html"), []byte(smuggledPartial), 0644); err != nil {
		t.Fatal(err)
	}

	c := NewAssetCompiler(compRoot, "")

	// Component attempts to smuggle <body> via @include without @extends
	trojanComponent := `<div class="wrapper">
@include("partials/smuggled")
</div>`

	_, _, err = c.compileHTML("trojan", trojanComponent)
	if err == nil {
		t.Fatal("Expected compile error when component smuggles <body> via @include without @extends, got nil")
	}
	if !strings.Contains(err.Error(), "authors root document tags") {
		t.Errorf("Expected root document tags error, got: %v", err)
	}
}

func TestCompiler_CommentedOutExtendsDoesNotBypassRootTagCheck(t *testing.T) {
	c := NewAssetCompiler("", "")
	commentedExtends := `<!-- @extends("layouts/app") -->
<!DOCTYPE html>
<html lang="en">
<head><title>Fake</title></head>
<body><h1>Fake Extends Page</h1></body>
</html>`

	_, _, err := c.compileHTML("fake-extends", commentedExtends)
	if err == nil {
		t.Fatal("Expected compile error when @extends is commented out and template authors root document tags, got nil")
	}
	if !strings.Contains(err.Error(), "authors root document tags") {
		t.Errorf("Expected root document tags error, got: %v", err)
	}
}

// TestShippedTemplates_CSPCompliance verifies that shipped component templates
// strictly obey CSP policies: no inline onclick, no inline style=, no orphaned data-controller,
// and all GoStack.<method> calls map to actual functions on GoStack in glide.go (D34, D42).
func TestShippedTemplates_CSPCompliance(t *testing.T) {
	_, callerFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed to determine source location")
	}
	rootDir := filepath.Clean(filepath.Join(filepath.Dir(callerFile), "..", ".."))
	templatesDir := filepath.Join(rootDir, "templates", "components")
	if _, err := os.Stat(templatesDir); os.IsNotExist(err) {
		t.Fatalf("templates/components directory not found: %s", templatesDir)
	}

	validMethods := GoStackMethodNames()
	if len(validMethods) == 0 {
		t.Fatalf("GoStackMethodNames() returned empty list from glide.go")
	}
	validGoStackMethods := make(map[string]bool)
	for _, m := range validMethods {
		validGoStackMethods[m] = true
	}

	err := filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		str := string(content)

		if strings.Contains(str, "onclick=") {
			t.Errorf("template %s contains prohibited inline onclick= handler (CSP violation)", path)
		}
		if strings.Contains(str, "style=") {
			t.Errorf("template %s contains prohibited inline style= attribute (CSP violation)", path)
		}
		if strings.Contains(str, `data-controller="`) {
			t.Errorf("template %s contains unsupported data-controller attribute", path)
		}

		// Verify any GoStack.method($el) calls
		re := regexp.MustCompile(`GoStack\.([a-zA-Z0-9_]+)\(`)
		matches := re.FindAllStringSubmatch(str, -1)
		for _, m := range matches {
			methodName := m[1]
			if !validGoStackMethods[methodName] {
				t.Errorf("template %s calls GoStack.%s which does not exist in glide.go", path, methodName)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking templates: %v", err)
	}

	// D42: Also scan generated component files to assert CSP compliance
	genFiles := []string{
		filepath.Join(rootDir, "cmd", "app", "gostack_components_gen.go"),
		filepath.Join(rootDir, "_examples", "app", "gostack_components_gen.go"),
	}
	for _, genFile := range genFiles {
		content, err := os.ReadFile(genFile)
		if err != nil {
			t.Fatalf("failed to read generated component file %s: %v", genFile, err)
		}
		genStr := string(content)
		if strings.Contains(genStr, "onclick=") {
			t.Errorf("generated file %s contains prohibited inline onclick= handler (CSP violation)", genFile)
		}
		if strings.Contains(genStr, `data-controller="`) {
			t.Errorf("generated file %s contains unsupported data-controller attribute", genFile)
		}
	}
}
