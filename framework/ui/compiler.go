/*
Purpose:
This file implements the Tempose Ahead-of-Time (AOT) component compilation engine.

Philosophy:
GoStack treats frontend components (HTML, CSS, JS) as first-class Go citizens.
Rather than parsing template files at runtime (which introduces disk I/O latency
and defers syntax errors to production), the compiler transforms all component
assets into static Go source code during the build step.

This means the generated file (gostack_components_gen.go) is a standard Go file
that is compiled directly into the application binary. The result is:
  - Zero disk I/O on every HTTP request for view rendering.
  - Compile-time guarantees that all registered components exist.
  - A single, portable binary with all assets embedded.

COMPONENT STRUCTURE:
Each component lives in its own subdirectory under the components path:
	components/
	  counter/
	    counter.html   ← markup template (supports {{ .Field }} bindings)
	    counter.css    ← component-scoped styles
	    counter.js     ← component-specific client scripts

The compiler scans this directory structure, processes each asset type,
and emits a single Go registration file containing all compiled outputs.
*/
package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AssetCompiler orchestrates the full component compilation pipeline.
// It scans the components source directory, processes all assets (HTML, CSS, JS),
// and writes a fully compiled, Go-formatted registration file to the output path.
//
// Fields:
//   - ComponentsPath: Absolute or relative path to the components source directory.
//   - OutputPath: Absolute or relative path where the generated Go file will be written.
type AssetCompiler struct {
	ComponentsPath string
	OutputPath     string
}

// NewAssetCompiler returns a pointer to an initialized AssetCompiler.
func NewAssetCompiler(componentsPath, outputPath string) *AssetCompiler {
	return &AssetCompiler{
		ComponentsPath: componentsPath,
		OutputPath:     outputPath,
	}
}

// Run executes the full compilation sequence.
func (c *AssetCompiler) Run() error {
	entries, err := os.ReadDir(c.ComponentsPath)
	if err != nil {
		return fmt.Errorf("cannot scan components directory: %w", err)
	}

	var registrations strings.Builder
	needsFmt := false
	needsUI := false

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		componentDir := filepath.Join(c.ComponentsPath, name)

		// 1. Process Scoped CSS
		cssPath := filepath.Join(componentDir, name+".css")
		if _, err := os.Stat(cssPath); err == nil {
			rawCSS, err := os.ReadFile(cssPath)
			if err != nil {
				return err
			}
			prefixed := c.scopeCSS(name, string(rawCSS))
			safeCSS := strings.ReplaceAll(prefixed, "`", "` + \"`\" + `")
			registrations.WriteString(fmt.Sprintf("\tui.RegisterComponentStyle(\"%s\", `%s`)\n\n", name, safeCSS))
			needsUI = true
		}

		// 2. Process JS Scripts
		jsPath := filepath.Join(componentDir, name+".js")
		if _, err := os.Stat(jsPath); err == nil {
			rawJS, err := os.ReadFile(jsPath)
			if err != nil {
				return err
			}
			safeJS := strings.ReplaceAll(string(rawJS), "`", "` + \"`\" + `")
			registrations.WriteString(fmt.Sprintf("\tui.RegisterComponentScript(\"%s\", `%s`)\n\n", name, safeJS))
			needsUI = true
		}

		// 3. Process HTML Templates
		htmlPath := filepath.Join(componentDir, name+".html")
		if _, err := os.Stat(htmlPath); err == nil {
			rawHTML, err := os.ReadFile(htmlPath)
			if err != nil {
				return err
			}
			rawHTMLStr := string(rawHTML)
			if strings.Contains(rawHTMLStr, "{{") || strings.Contains(rawHTMLStr, "@") {
				needsUI = true
			}
			compiledHTML, hasFmt := c.compileHTML(name, rawHTMLStr)
			if hasFmt {
				needsFmt = true
			}
			registrations.WriteString(fmt.Sprintf(
				"\tt.Register(\"%s\", func(w io.Writer, data any, t http.ViewTranslator) error {\n"+
				"\t\ttrans := func(key string, replace ...map[string]string) string {\n"+
				"\t\t\tif t == nil { return ui.Escape(key) }\n"+
				"\t\t\treturn ui.Escape(t.Trans(key, replace...))\n"+
				"\t\t}\n"+
				"\t\ttransRaw := func(key string, replace ...map[string]string) string {\n"+
				"\t\t\tif t == nil { return key }\n"+
				"\t\t\treturn t.Trans(key, replace...)\n"+
				"\t\t}\n"+
				"\t\ttransChoice := func(key string, count int, replace ...map[string]string) string {\n"+
				"\t\t\tif t == nil { return ui.Escape(key) }\n"+
				"\t\t\treturn ui.Escape(t.TransChoice(key, count, replace...))\n"+
				"\t\t}\n"+
				"\t\t_ = trans\n"+
				"\t\t_ = transRaw\n"+
				"\t\t_ = transChoice\n"+
				"%s\t\treturn nil\n\t})\n\n", name, compiledHTML))
		}
	}

	// Build dynamic imports slice
	var imports []string
	if needsFmt {
		imports = append(imports, `"fmt"`)
	}
	imports = append(imports, `"github.com/charledeon77/gostack-framework/framework/http"`)
	if needsUI {
		imports = append(imports, `"github.com/charledeon77/gostack-framework/framework/ui"`)
	}
	imports = append(imports, `"io"`)

	goSource := fmt.Sprintf(`// Code generated by GoStack. DO NOT EDIT.
// This file is compiled from component directories during local development builds.

package main

import (
	%s
)

// RegisterComponents binds all compiled component views, styles, and scripts.
func RegisterComponents(t *http.Tempose) {
%s}`, strings.Join(imports, "\n\t"), registrations.String())

	return os.WriteFile(filepath.Join(c.OutputPath, "gostack_components_gen.go"), []byte(goSource), 0644)
}

// Package-level compiled regexes (RE2-compatible, no backreferences).
var directiveRegex    = regexp.MustCompile(`(@if\([^)]+\)|@elseif\([^)]+\)|@else|@endif|@foreach\([^)]+\)|@endforeach)`)
var variableNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)
var includeRe         = regexp.MustCompile(`@include\(["']([^"'\n]+)["']\)`)
var extendsRe         = regexp.MustCompile(`@extends\(["']([^"'\n]+)["']\)`)
var sectionRe         = regexp.MustCompile(`(?s)@section\(["']([^"'\n]+)["']\)(.*?)@endsection`)
var yieldRe           = regexp.MustCompile(`@yield\(["']([^"'\n]+)["']\)`)

func isVariableName(expr string) bool {
	return variableNameRegex.MatchString(expr)
}

// compileHTML transforms a raw HTML template string into a sequence of Go statements.
// Returns the compiled Go source and whether fmt.Sprint is used (for import tracking).
func (c *AssetCompiler) compileHTML(componentName, rawHTML string) (string, bool) {
	// Resolve layout inheritance and partial includes at compile time.
	if preprocessed, err := c.preprocessLayouts(rawHTML); err == nil {
		rawHTML = preprocessed
	}

	wrappedHTML := fmt.Sprintf("<div gs-component=\"%s\">\n%s\n</div>", componentName, strings.TrimSpace(rawHTML))

	var sb strings.Builder
	needsFmt := false

	matches := directiveRegex.FindAllStringIndex(wrappedHTML, -1)
	if len(matches) == 0 {
		c.compileTextSegment(wrappedHTML, &sb, &needsFmt)
		return sb.String(), needsFmt
	}

	lastIdx := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if start > lastIdx {
			c.compileTextSegment(wrappedHTML[lastIdx:start], &sb, &needsFmt)
		}
		c.compileDirective(wrappedHTML[start:end], &sb, &needsFmt)
		lastIdx = end
	}
	if lastIdx < len(wrappedHTML) {
		c.compileTextSegment(wrappedHTML[lastIdx:], &sb, &needsFmt)
	}

	return sb.String(), needsFmt
}

func (c *AssetCompiler) compileDirective(dirStr string, sb *strings.Builder, needsFmt *bool) {
	switch {
	case strings.HasPrefix(dirStr, "@if("):
		expr := strings.TrimSuffix(strings.TrimPrefix(dirStr, "@if("), ")")
		if strings.HasPrefix(expr, ".") {
			sb.WriteString(fmt.Sprintf("\t\tif ui.EvaluateBool(data, \"%s\") {\n", strings.TrimPrefix(expr, ".")))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\tif ui.EvaluateBool(data, \"%s\") {\n", expr))
		} else {
			sb.WriteString(fmt.Sprintf("\t\tif %s {\n", expr))
		}
	case strings.HasPrefix(dirStr, "@elseif("):
		expr := strings.TrimSuffix(strings.TrimPrefix(dirStr, "@elseif("), ")")
		if strings.HasPrefix(expr, ".") {
			sb.WriteString(fmt.Sprintf("\t\t} else if ui.EvaluateBool(data, \"%s\") {\n", strings.TrimPrefix(expr, ".")))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\t} else if ui.EvaluateBool(data, \"%s\") {\n", expr))
		} else {
			sb.WriteString(fmt.Sprintf("\t\t} else if %s {\n", expr))
		}
	case dirStr == "@else":
		sb.WriteString("\t\t} else {\n")
	case dirStr == "@endif":
		sb.WriteString("\t\t}\n")
	case strings.HasPrefix(dirStr, "@foreach("):
		expr := strings.TrimSuffix(strings.TrimPrefix(dirStr, "@foreach("), ")")
		parts := strings.Split(expr, " as ")
		if len(parts) != 2 {
			parts = strings.Split(expr, " in ")
		}
		if len(parts) == 2 {
			listName := strings.TrimPrefix(strings.TrimSpace(parts[0]), ".")
			varName := strings.TrimSpace(parts[1])
			sb.WriteString(fmt.Sprintf("\t\tfor _, %sVal := range ui.EvaluateSlice(data, \"%s\") {\n", varName, listName))
			sb.WriteString(fmt.Sprintf("\t\t\tdata := map[string]any{\"$parent\": data, \"%s\": %sVal}\n", varName, varName))
		}
	case dirStr == "@endforeach":
		sb.WriteString("\t\t}\n")
	}
}

func (c *AssetCompiler) compileTextSegment(segment string, sb *strings.Builder, needsFmt *bool) {
	if segment == "" {
		return
	}
	parts := strings.Split(segment, "{{")

	firstSafe := strings.ReplaceAll(parts[0], "`", "` + \"`\" + `")
	if firstSafe != "" {
		sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, `%s`); err != nil { return err }\n", firstSafe))
	}

	for i := 1; i < len(parts); i++ {
		subparts := strings.Split(parts[i], "}}")
		if len(subparts) < 2 {
			continue
		}
		expr := strings.TrimSpace(subparts[0])
		rest := subparts[1]

		if strings.Contains(expr, "|") {
			// Filter pipe chain: e.g. ".Title | slugify" or ".CreatedAt | date(\"2006-01-02\")"
			compiledExpr := c.compileFilterChain(expr)
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(%s)); err != nil { return err }\n", compiledExpr))
		} else if strings.HasPrefix(expr, ".") {
			fieldName := strings.TrimPrefix(expr, ".")
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(ui.Evaluate(data, \"%s\"))); err != nil { return err }\n", fieldName))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(ui.Evaluate(data, \"%s\"))); err != nil { return err }\n", expr))
		} else {
			*needsFmt = true
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, fmt.Sprint(%s)); err != nil { return err }\n", expr))
		}

		restSafe := strings.ReplaceAll(rest, "`", "` + \"`\" + `")
		if restSafe != "" {
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, `%s`); err != nil { return err }\n", restSafe))
		}
	}
}

// compileFilterChain compiles a pipe-separated filter expression into a Go ui.ApplyFilter call chain.
// Examples:
//   ".Title | slugify"              → ui.ApplyFilter(ui.Evaluate(data, "Title"), "slugify")
//   ".CreatedAt | date(\"2006\")"   → ui.ApplyFilter(ui.Evaluate(data, "CreatedAt"), "date", "2006")
//   ".Name | upper | truncate(20)" → ui.ApplyFilter(ui.ApplyFilter(...), "truncate", "20")
func (c *AssetCompiler) compileFilterChain(expr string) string {
	// Parse the expression into parts by splitting on '|', but respecting quotes/parens.
	var pipes []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	parenDepth := 0

	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch ch {
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
			current.WriteByte(ch)
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
			current.WriteByte(ch)
		case '(':
			if !inSingleQuote && !inDoubleQuote {
				parenDepth++
			}
			current.WriteByte(ch)
		case ')':
			if !inSingleQuote && !inDoubleQuote {
				parenDepth--
			}
			current.WriteByte(ch)
		case '|':
			if !inSingleQuote && !inDoubleQuote && parenDepth == 0 {
				pipes = append(pipes, strings.TrimSpace(current.String()))
				current.Reset()
			} else {
				current.WriteByte(ch)
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		pipes = append(pipes, strings.TrimSpace(current.String()))
	}

	if len(pipes) == 0 {
		return `""`
	}

	// Compile the base value expression
	base := pipes[0]
	var goExpr string
	if strings.HasPrefix(base, ".") {
		goExpr = fmt.Sprintf(`ui.Evaluate(data, "%s")`, strings.TrimPrefix(base, "."))
	} else if isVariableName(base) {
		goExpr = fmt.Sprintf(`ui.Evaluate(data, "%s")`, base)
	} else {
		goExpr = base
	}

	// Apply each filter in the chain
	for _, pipe := range pipes[1:] {
		pipe = strings.TrimSpace(pipe)
		// Parse filter name and optional args: filterName(arg1, arg2)
		if parenIdx := strings.Index(pipe, "("); parenIdx != -1 {
			filterName := strings.TrimSpace(pipe[:parenIdx])
			argsStr := strings.TrimSuffix(strings.TrimSpace(pipe[parenIdx+1:]), ")")

			// Parse filter args, respecting quotes/parens.
			var goArgs []string
			var argBuilder strings.Builder
			argSingleQuote := false
			argDoubleQuote := false
			argParenDepth := 0

			for i := 0; i < len(argsStr); i++ {
				ch := argsStr[i]
				switch ch {
				case '\'':
					if !argDoubleQuote {
						argSingleQuote = !argSingleQuote
					}
					argBuilder.WriteByte(ch)
				case '"':
					if !argSingleQuote {
						argDoubleQuote = !argDoubleQuote
					}
					argBuilder.WriteByte(ch)
				case '(':
					if !argSingleQuote && !argDoubleQuote {
						argParenDepth++
					}
					argBuilder.WriteByte(ch)
				case ')':
					if !argSingleQuote && !argDoubleQuote {
						argParenDepth--
					}
					argBuilder.WriteByte(ch)
				case ',':
					if !argSingleQuote && !argDoubleQuote && argParenDepth == 0 {
						cleaned := strings.TrimSpace(argBuilder.String())
						cleaned = strings.Trim(cleaned, `"'`)
						goArgs = append(goArgs, fmt.Sprintf(`"%s"`, cleaned))
						argBuilder.Reset()
					} else {
						argBuilder.WriteByte(ch)
					}
				default:
					argBuilder.WriteByte(ch)
				}
			}
			if argBuilder.Len() > 0 {
				cleaned := strings.TrimSpace(argBuilder.String())
				cleaned = strings.Trim(cleaned, `"'`)
				goArgs = append(goArgs, fmt.Sprintf(`"%s"`, cleaned))
			}

			if len(goArgs) > 0 {
				goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s", %s)`, goExpr, filterName, strings.Join(goArgs, ", "))
			} else {
				goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s")`, goExpr, filterName)
			}
		} else {
			// No args: filterName only
			goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s")`, goExpr, pipe)
		}
	}

	return goExpr
}

// preprocessLayouts resolves template inheritance at compile time.
// Supports @extends, @section/@endsection, @yield, @include, and <slot> composition.
// All regexes use RE2 syntax (no backreferences) for Go compatibility.
func (c *AssetCompiler) preprocessLayouts(rawHTML string) (string, error) {
	// 0. Convert <slot name="..."> HTML syntax to @yield, and slot="..." attributes to @section blocks.
	// This allows Svelte-style slot composition alongside Blade-style directives.
	rawHTML = regexp.MustCompile(`(?i)<slot name=["']([^"']+)["'][^>]*></slot>`).ReplaceAllStringFunc(rawHTML, func(s string) string {
		m := regexp.MustCompile(`(?i)name=["']([^"']+)["']`).FindStringSubmatch(s)
		if len(m) < 2 {
			return s
		}
		return fmt.Sprintf(`@yield('%s')`, m[1])
	})
	// Self-closing default slot → @yield('content')
	rawHTML = regexp.MustCompile(`(?i)<slot></slot>`).ReplaceAllString(rawHTML, `@yield('content')`)
	rawHTML = regexp.MustCompile(`(?i)<slot/>`).ReplaceAllString(rawHTML, `@yield('content')`)

	// Convert <div slot="name">...</div> blocks to @section/@endsection.
	rawHTML = regexp.MustCompile(`(?is)<([a-z][a-z0-9]*)[^>]*\bslot=["']([^"']+)["'][^>]*>(.*?)</[a-z][a-z0-9]*>`).ReplaceAllStringFunc(rawHTML, func(s string) string {
		parts := regexp.MustCompile(`(?is)<([a-z][a-z0-9]*)[^>]*\bslot=["']([^"']+)["'][^>]*>(.*?)</[a-z][a-z0-9]*>`).FindStringSubmatch(s)
		if len(parts) < 4 {
			return s
		}
		slotName, content := parts[2], parts[3]
		return fmt.Sprintf("@section('%s')%s@endsection", slotName, content)
	})

	// 1. Inline @include partial files.
	rawHTML = includeRe.ReplaceAllStringFunc(rawHTML, func(incStr string) string {
		m := includeRe.FindStringSubmatch(incStr)
		if len(m) < 2 {
			return ""
		}
		incFile := filepath.Join(c.ComponentsPath, m[1]+".html")
		if _, err := os.Stat(incFile); err != nil {
			incFile = filepath.Join(filepath.Dir(c.ComponentsPath), m[1]+".html")
		}
		b, err := os.ReadFile(incFile)
		if err != nil {
			return fmt.Sprintf("<!-- Include error: %s not found -->", m[1])
		}
		return string(b)
	})

	if !strings.Contains(rawHTML, "@extends") {
		return rawHTML, nil
	}

	// 2. Extract layout path from @extends("layouts/app").
	m := extendsRe.FindStringSubmatch(rawHTML)
	if len(m) < 2 {
		return rawHTML, nil
	}
	layoutPath := m[1]

	layoutFile := filepath.Join(c.ComponentsPath, layoutPath+".html")
	if _, err := os.Stat(layoutFile); err != nil {
		layoutFile = filepath.Join(filepath.Dir(c.ComponentsPath), layoutPath+".html")
	}
	layoutBytes, err := os.ReadFile(layoutFile)
	if err != nil {
		return "", fmt.Errorf("layout file %s not found: %w", layoutPath, err)
	}
	layoutHTML := string(layoutBytes)

	// 3. Extract all @section blocks from the child template.
	sections := sectionRe.FindAllStringSubmatch(rawHTML, -1)
	sectionMap := make(map[string]string)
	for _, sec := range sections {
		if len(sec) >= 3 {
			sectionMap[sec[1]] = sec[2] // sec[1]=name, sec[2]=content
		}
	}

	// 4. Replace @yield placeholders in the layout with section content.
	merged := yieldRe.ReplaceAllStringFunc(layoutHTML, func(yieldStr string) string {
		ym := yieldRe.FindStringSubmatch(yieldStr)
		if len(ym) < 2 {
			return ""
		}
		if content, ok := sectionMap[ym[1]]; ok {
			return content
		}
		return ""
	})

	return merged, nil
}

// scopeCSS transforms raw CSS into component-scoped CSS by prepending a unique
// selector prefix to every rule block, preventing styles from leaking globally.
func (c *AssetCompiler) scopeCSS(componentName, rawCSS string) string {
	var scoped []string
	prefix := fmt.Sprintf("gostack-root [gs-component=\"%s\"]", componentName)

	lines := strings.Split(rawCSS, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "/*") {
			continue
		}
		if strings.Contains(line, "{") && !strings.HasPrefix(line, "@") {
			scoped = append(scoped, prefix+" "+line)
		} else {
			scoped = append(scoped, line)
		}
	}
	return strings.Join(scoped, "\n")
}

// ── Dev Hot-Reload Watcher ───────────────────────────────────────────────────

// HotReloadWatcher polls a components directory for file changes and re-runs the
// AssetCompiler automatically. On each rebuild it broadcasts a reload signal to
// all connected SSE clients so the browser refreshes instantly — with zero
// external dependencies (no inotify, no fsnotify, no CGO).
//
// Usage in your dev server entrypoint:
//
//	watcher := ui.NewHotReloadWatcher(compiler, 500*time.Millisecond)
//	go watcher.Start(ctx)
//	// Register the SSE endpoint with your router:
//	router.Get("/__gostack_reload", func(ctx *http.Context) {
//	    watcher.ServeSSE(ctx.Writer, ctx.Request)
//	})
//
// In your base layout HTML add:
//
//	<script>
//	  if (location.hostname === 'localhost') {
//	    const es = new EventSource('/__gostack_reload');
//	    es.onmessage = () => location.reload();
//	  }
//	</script>
type HotReloadWatcher struct {
	compiler *AssetCompiler
	interval time.Duration

	mu      sync.RWMutex
	clients map[chan struct{}]struct{}

	// lastSnapshot stores mod-time fingerprints for all watched files.
	lastSnapshot map[string]time.Time
}

// NewHotReloadWatcher creates a watcher that polls the compiler's ComponentsPath.
// interval controls how often the directory is scanned for changes.
func NewHotReloadWatcher(compiler *AssetCompiler, interval time.Duration) *HotReloadWatcher {
	return &HotReloadWatcher{
		compiler:     compiler,
		interval:     interval,
		clients:      make(map[chan struct{}]struct{}),
		lastSnapshot: make(map[string]time.Time),
	}
}

// Start begins the polling loop. It blocks until ctx.Done() is closed.
// Run this in a goroutine: go watcher.Start(ctx)
func (w *HotReloadWatcher) Start(ctx interface{ Done() <-chan struct{} }) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Take initial snapshot so first tick doesn't fire spuriously.
	_ = w.snapshot()

	for {
		select {
		case <-ticker.C:
			changed, err := w.hasChanged()
			if err != nil {
				log.Printf("[GoStack HotReload] Error scanning components: %v", err)
				continue
			}
			if !changed {
				continue
			}
			log.Println("[GoStack HotReload] Change detected — recompiling components…")
			if err := w.compiler.Run(); err != nil {
				log.Printf("[GoStack HotReload] Compile error: %v", err)
			} else {
				log.Println("[GoStack HotReload] Recompile complete. Notifying browsers.")
				w.broadcast()
			}
		case <-ctx.Done():
			return
		}
	}
}

// snapshot records the current mod-time of every file under ComponentsPath.
func (w *HotReloadWatcher) snapshot() map[string]time.Time {
	snap := make(map[string]time.Time)
	_ = filepath.Walk(w.compiler.ComponentsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		snap[path] = info.ModTime()
		return nil
	})
	w.mu.Lock()
	w.lastSnapshot = snap
	w.mu.Unlock()
	return snap
}

// hasChanged compares the current filesystem state against the last snapshot.
func (w *HotReloadWatcher) hasChanged() (bool, error) {
	w.mu.RLock()
	prev := w.lastSnapshot
	w.mu.RUnlock()

	current := make(map[string]time.Time)
	if err := filepath.Walk(w.compiler.ComponentsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		current[path] = info.ModTime()
		return nil
	}); err != nil {
		return false, err
	}

	if len(current) != len(prev) {
		w.mu.Lock()
		w.lastSnapshot = current
		w.mu.Unlock()
		return true, nil
	}
	for path, modTime := range current {
		if prev[path] != modTime {
			w.mu.Lock()
			w.lastSnapshot = current
			w.mu.Unlock()
			return true, nil
		}
	}
	return false, nil
}

// broadcast sends a reload signal to all connected SSE clients.
func (w *HotReloadWatcher) broadcast() {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for ch := range w.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// subscribe registers a new SSE client channel, returning it and an unsubscribe func.
func (w *HotReloadWatcher) subscribe() (chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	w.clients[ch] = struct{}{}
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		delete(w.clients, ch)
		w.mu.Unlock()
	}
}

// ServeSSE writes an HTTP/SSE stream that sends a "reload" event whenever the
// compiler detects a component change. Connect a browser EventSource to this
// endpoint to get automatic page refreshes during development.
func (w *HotReloadWatcher) ServeSSE(writer interface {
	Header() interface{ Set(string, string) }
	WriteHeader(int)
	Write([]byte) (int, error)
	Flush()
}, req interface{ Context() interface{ Done() <-chan struct{} } }) {
	type flusher interface {
		Flush()
	}

	// We use net/http types directly — the interface approach keeps this file
	// free of an import cycle with framework/http.
	type headerSetter interface {
		Header() interface{ Set(string, string) }
	}

	// In practice callers pass *http.ResponseWriter — safe to cast.
	type realWriter interface {
		Header() interface {
			Set(string, string)
		}
		WriteHeader(int)
		Write([]byte) (int, error)
		Flush()
	}

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(200)
	writer.Write([]byte(": connected\n\n")) //nolint:errcheck
	writer.Flush()

	ch, unsub := w.subscribe()
	defer unsub()

	done := req.Context().Done()
	for {
		select {
		case <-ch:
			writer.Write([]byte("data: reload\n\n")) //nolint:errcheck
			writer.Flush()
		case <-done:
			return
		}
	}
}

