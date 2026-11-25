/*
Tempose — GoStack Ahead-of-Time (AOT) UI Component Compiler Subsystem.

Naming Metaphor: Why Tempose?
The GoStack UI Component Compiler was named Tempose as an elegant blend of "Tempo" (rhythm and speed)
and "Compose" (building cohesive UI components). Tempose composes HTML markup, scoped CSS stylesheets,
and client scripts into pure, lightning-fast Go byte streams at compile-time, delivering high-tempo
rendering performance without runtime template parsing overhead.

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
	"context"
	"crypto/sha256"
	"fmt"
	"go/format"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AssetCompiler orchestrates the full component compilation pipeline.
// It scans the components source directory, processes all assets (HTML, CSS, JS),
// bundles and fingerprints public assets, and writes a fully compiled, Go-formatted
// registration file to the output path.
//
// Fields:
//   - ComponentsPath: Absolute or relative path to the components source directory.
//   - OutputPath: Absolute or relative path where the generated Go file will be written.
//   - PublicAssetsPath: Destination directory for compiled CSS/JS bundles (defaults to "public/assets").
// CompileWarning describes a non-fatal compile-time diagnostic (such as an undeclared
// filter in WarnUnknownFilters mode) with its exact component name and line number (D53).
type CompileWarning struct {
	Component string
	Line      int
	Filter    string
	Expr      string
}

type AssetCompiler struct {
	ComponentsPath   string
	OutputPath       string
	PublicAssetsPath string
	// KnownFilters lists app-registered filter names accepted during compile-time
	// validation when compiling outside the app's runtime process (D52).
	KnownFilters map[string]bool
	// WarnUnknownFilters logs a warning instead of failing when an unknown filter
	// is encountered; intended for out-of-process CLI compilers (gost dev/make) (D52).
	WarnUnknownFilters bool
	// Warnings holds deduplicated, sorted warnings emitted by the most recent Run() (D53).
	Warnings []CompileWarning
}

// WithFilterNames declares additional app-registered filter names on the compiler
// and returns the receiver for chaining (D52).
func (c *AssetCompiler) WithFilterNames(names ...string) *AssetCompiler {
	for _, n := range names {
		c.RegisterFilter(n)
	}
	return c
}

// RegisterFilter declares a single app-registered filter name on the compiler (D52).
func (c *AssetCompiler) RegisterFilter(name string) {
	if c.KnownFilters == nil {
		c.KnownFilters = make(map[string]bool)
	}
	if name != "" {
		c.KnownFilters[name] = true
	}
}

func (c *AssetCompiler) isKnownFilter(name string) bool {
	return HasFilter(name) || (c.KnownFilters != nil && c.KnownFilters[name])
}

// LoadFilterManifest reads a newline-delimited filter manifest file (e.g. "gostack.filters"),
// ignoring blank lines and '#' comments, and registers each filter name on the compiler (D53).
// Returns os.ErrNotExist if the file does not exist.
func (c *AssetCompiler) LoadFilterManifest(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		c.RegisterFilter(line)
	}
	return nil
}

// NewAssetCompiler returns a pointer to an initialized AssetCompiler.
// An optional publicAssetsPath can be provided; if omitted, it defaults to "public/assets".
func NewAssetCompiler(componentsPath, outputPath string, publicAssetsPath ...string) *AssetCompiler {
	c := &AssetCompiler{
		ComponentsPath:   componentsPath,
		OutputPath:       outputPath,
		PublicAssetsPath: "public/assets",
	}
	if len(publicAssetsPath) > 0 && publicAssetsPath[0] != "" {
		c.PublicAssetsPath = publicAssetsPath[0]
	}
	return c
}

func (c *AssetCompiler) resolvePublicAssetsPath() string {
	if c.PublicAssetsPath != "" {
		return c.PublicAssetsPath
	}
	if c.OutputPath != "" {
		parent := filepath.Dir(c.OutputPath)
		if filepath.Base(parent) == "cmd" || filepath.Base(c.OutputPath) == "app" {
			projectRoot := filepath.Dir(parent)
			return filepath.Join(projectRoot, "public", "assets")
		}
		return filepath.Join(parent, "public", "assets")
	}
	return "public/assets"
}

func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:4]) // 8-character hex fingerprint
}

// Run executes the full compilation sequence.
func (c *AssetCompiler) Run() error {
	c.Warnings = nil
	entries, err := os.ReadDir(c.ComponentsPath)
	if err != nil {
		return fmt.Errorf("cannot scan components directory: %w", err)
	}

	var registrations strings.Builder
	needsFmt := false
	needsUI := false

	var cssBundle strings.Builder
	cssBundle.WriteString(GoStackCoreCSS)
	cssBundle.WriteString("\n\n")

	var jsBundle strings.Builder
	jsBundle.WriteString(GlideJS)
	jsBundle.WriteString("\n\n")

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isValidComponentName(name) {
			return fmt.Errorf("invalid component directory name %q: must match ^[a-z0-9][a-z0-9_-]*$", name)
		}
		componentDir := filepath.Join(c.ComponentsPath, name)

		// 1. Process Scoped CSS
		cssPath := filepath.Join(componentDir, name+".css")
		if _, err := os.Stat(cssPath); err == nil {
			rawCSS, err := os.ReadFile(cssPath)
			if err != nil {
				return err
			}
			prefixed, err := c.scopeCSS(name, string(rawCSS))
			if err != nil {
				return fmt.Errorf("component %q CSS invalid: %w", name, err)
			}
			cssBundle.WriteString(fmt.Sprintf("/* Component: %s */\n%s\n\n", name, prefixed))
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
			jsBundle.WriteString(fmt.Sprintf("/* Component: %s */\n%s\n\n", name, string(rawJS)))
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
			compiledHTML, hasFmt, err := c.compileHTML(name, rawHTMLStr)
			if err != nil {
				return err
			}
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

	// 4. Write compiled bundles to PublicAssetsPath and compute version fingerprints
	publicAssets := c.resolvePublicAssetsPath()
	cssDir := filepath.Join(publicAssets, "css")
	jsDir := filepath.Join(publicAssets, "js")
	if err := os.MkdirAll(cssDir, 0755); err != nil {
		return fmt.Errorf("failed to create css asset directory: %w", err)
	}
	if err := os.MkdirAll(jsDir, 0755); err != nil {
		return fmt.Errorf("failed to create js asset directory: %w", err)
	}

	cssBytes := []byte(cssBundle.String())
	cssHash := hashBytes(cssBytes)
	if err := os.WriteFile(filepath.Join(cssDir, "gostack.css"), cssBytes, 0644); err != nil {
		log.Printf("[GoStack Compiler Warning] Failed to write %s: %v", filepath.Join(cssDir, "gostack.css"), err)
	}

	jsBytes := []byte(jsBundle.String())
	jsHash := hashBytes(jsBytes)
	if err := os.WriteFile(filepath.Join(jsDir, "gostack.js"), jsBytes, 0644); err != nil {
		log.Printf("[GoStack Compiler Warning] Failed to write %s: %v", filepath.Join(jsDir, "gostack.js"), err)
	}

	// Always write dev hot-reload client script to assets/js
	if err := os.WriteFile(filepath.Join(jsDir, "gostack-hot-reload.js"), []byte(HotReloadClientJS), 0644); err != nil {
		log.Printf("[GoStack Compiler Warning] Failed to write %s: %v", filepath.Join(jsDir, "gostack-hot-reload.js"), err)
	}

	// Register versioned URLs in the generated code
	registrations.WriteString(fmt.Sprintf("\tui.RegisterAssetURLs(\"/assets/css/gostack.css?v=%s\", \"/assets/js/gostack.js?v=%s\")\n\n", cssHash, jsHash))
	needsUI = true

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

	// Pass generated source through gofmt so the output file is always clean.
	formatted, fmtErr := format.Source([]byte(goSource))
	if fmtErr != nil {
		return fmt.Errorf("failed to format generated Go source: %w\nGenerated code:\n%s", fmtErr, goSource)
	}
	if err := os.MkdirAll(c.OutputPath, 0755); err != nil {
		return fmt.Errorf("cannot create output directory %q: %w", c.OutputPath, err)
	}
	if err := os.WriteFile(filepath.Join(c.OutputPath, "gostack_components_gen.go"), formatted, 0644); err != nil {
		return err
	}
	c.flushWarnings()
	return nil
}

func (c *AssetCompiler) flushWarnings() {
	if len(c.Warnings) == 0 {
		return
	}
	type warnKey struct {
		component string
		filter    string
	}
	seen := make(map[warnKey]CompileWarning, len(c.Warnings))
	for _, w := range c.Warnings {
		k := warnKey{component: w.Component, filter: w.Filter}
		if existing, ok := seen[k]; !ok || w.Line < existing.Line {
			seen[k] = w
		}
	}
	deduped := make([]CompileWarning, 0, len(seen))
	for _, w := range seen {
		deduped = append(deduped, w)
	}
	sort.Slice(deduped, func(i, j int) bool {
		if deduped[i].Component != deduped[j].Component {
			return deduped[i].Component < deduped[j].Component
		}
		if deduped[i].Line != deduped[j].Line {
			return deduped[i].Line < deduped[j].Line
		}
		return deduped[i].Filter < deduped[j].Filter
	})
	c.Warnings = deduped
	for _, w := range deduped {
		log.Printf("[GoStack Compiler Warning] %s:%d: unknown filter %q in expression %q (declare in gostack.filters or on AssetCompiler)", w.Component, w.Line, w.Filter, w.Expr)
	}
}

// Package-level compiled regexes (RE2-compatible, no backreferences).
var directiveRegex     = regexp.MustCompile(`(@if\([^)]+\)|@elseif\([^)]+\)|@else|@endif|@foreach\([^)]+\)|@endforeach)`)
var componentNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
var variableNameRegex  = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_.]*$`)
var identifierRegex    = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var numberLiteralRegex = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?$`)
var includeRe          = regexp.MustCompile(`@include\(["']([^"'\n]+)["']\)`)
var extendsRe          = regexp.MustCompile(`@extends\(["']([^"'\n]+)["']\)`)
var sectionRe          = regexp.MustCompile(`(?s)@section\(["']([^"'\n]+)["']\)(.*?)@endsection`)
var yieldRe            = regexp.MustCompile(`@yield\(["']([^"'\n]+)["']\)`)
// htmlCommentRe matches HTML comments including multiline ones (dotall via (?s)).
// Used in preprocessLayouts to strip comments before scanning for directives,
// preventing false positives such as <!-- @include("x") --> from being processed.
var htmlCommentRe      = regexp.MustCompile(`(?s)<!--.*?-->`)
var bodyTagRe          = regexp.MustCompile(`(?i)<body(\s+[^>]*)?>`)
var rootDocTagsRe      = regexp.MustCompile(`(?i)<(!doctype|html|head|body)(\s+[^>]*)?>`)

func isHeadSlot(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "title", "meta", "head", "styles", "scripts", "headers", "header_scripts":
		return true
	default:
		return false
	}
}

func isValidComponentName(name string) bool {
	return componentNameRegex.MatchString(name)
}

func isVariableName(expr string) bool {
	return variableNameRegex.MatchString(expr)
}

func isValidIdentifier(name string) bool {
	return identifierRegex.MatchString(name)
}

func isLiteral(expr string) bool {
	if (strings.HasPrefix(expr, `"`) && strings.HasSuffix(expr, `"`)) ||
		(strings.HasPrefix(expr, `'`) && strings.HasSuffix(expr, `'`)) {
		return true
	}
	if numberLiteralRegex.MatchString(expr) {
		return true
	}
	if expr == "true" || expr == "false" {
		return true
	}
	return false
}

// isTransCall validates that expr is a safe call to one of the three built-in
// translation helpers: trans, transRaw, or transChoice.
//
// Security contract: the first argument must be a plain quoted string literal
// (the translation key). No nested function calls or arbitrary Go expressions
// are permitted inside the call. This prevents template injection while still
// allowing the full optional-argument signatures:
//
//	trans("key")
//	trans("key", map[string]string{"Name": .FirstName})   ← map args: rejected (non-string literal)
//	transChoice("key", 3)                                 ← second int arg: allowed as number literal
//
// In practice, map replacement args come from controller data, not from the
// template itself, so the restriction is not limiting.
func isTransCall(expr string) bool {
	var prefix string
	switch {
	case strings.HasPrefix(expr, "trans("):
		prefix = "trans("
	case strings.HasPrefix(expr, "transRaw("):
		prefix = "transRaw("
	case strings.HasPrefix(expr, "transChoice("):
		prefix = "transChoice("
	default:
		return false
	}
	if !strings.HasSuffix(expr, ")") {
		return false
	}
	// Extract everything between the outer parens.
	inner := expr[len(prefix) : len(expr)-1]
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return false
	}
	// The first argument must be a double-quoted string literal (the key).
	// We allow an optional second argument that is a number literal (for transChoice count).
	// Anything else is rejected.
	if !strings.HasPrefix(inner, `"`) {
		return false
	}
	// Find the closing quote of the first string arg.
	end := strings.Index(inner[1:], `"`)
	if end == -1 {
		return false
	}
	afterKey := strings.TrimSpace(inner[end+2:]) // skip opening quote + content + closing quote
	if afterKey == "" {
		return true // trans("key") — valid
	}
	if !strings.HasPrefix(afterKey, ",") {
		return false
	}
	secondArg := strings.TrimSpace(afterKey[1:])
	// Only allow a plain number literal as an optional second argument (transChoice count).
	return numberLiteralRegex.MatchString(secondArg)
}

func calculateLine(fullText, snippet string) int {
	idx := strings.Index(fullText, snippet)
	if idx == -1 {
		return 1
	}
	line := strings.Count(fullText[:idx], "\n")
	if line < 1 {
		line = 1
	}
	return line
}

type exprError struct {
	expr string
	err  error
}

// compileHTML transforms a raw HTML template string into a sequence of Go statements.
// Returns the compiled Go source, whether fmt.Sprint is used (for import tracking), and any validation error.
func (c *AssetCompiler) compileHTML(componentName, rawHTML string) (string, bool, error) {
	// Resolve layout inheritance and partial includes at compile time.
	// didExtend is only true if a layout was actually extended and applied after comment stripping.
	preprocessed, didExtend, err := c.preprocessLayouts(componentName, rawHTML)
	if err != nil {
		return "", false, fmt.Errorf("component %q: %w", componentName, err)
	}

	// Strict Framework Contract: Standalone components must not author root document tags
	// directly, smuggled through @include partials, or masked by commented-out @extends directives.
	if !didExtend {
		cleanHTML := htmlCommentRe.ReplaceAllString(preprocessed, "")
		if rootDocTagsRe.MatchString(cleanHTML) {
			return "", false, fmt.Errorf("component %q authors root document tags (<!DOCTYPE>, <html>, <head>, or <body>) without @extends. Use @extends(\"layouts/app\") for pages, or author as a component fragment.", componentName)
		}
	}

	rawHTML = preprocessed

	trimmedHTML := strings.TrimSpace(rawHTML)
	lower := strings.ToLower(trimmedHTML)
	isFullDoc := strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html")

	var wrappedHTML string
	if isFullDoc {
		// Document generated via layout inheritance (@extends) already has the component badge
		// injected into the layout's primary @yield slot (<div gs-component="...">).
		wrappedHTML = trimmedHTML
	} else if strings.HasPrefix(lower, fmt.Sprintf("<div gs-component=\"%s\"", strings.ToLower(componentName))) {
		// D14 / D45: Template already contains outer component wrapper; avoid nested duplicate wrappers (case-insensitive).
		wrappedHTML = trimmedHTML
	} else {
		wrappedHTML = fmt.Sprintf("<div gs-component=\"%s\">\n%s\n</div>", componentName, trimmedHTML)
	}

	var sb strings.Builder
	needsFmt := false
	recordWarnings := func(warns []CompileWarning) {
		for _, w := range warns {
			w.Component = componentName
			w.Line = calculateLine(wrappedHTML, w.Expr)
			c.Warnings = append(c.Warnings, w)
		}
	}

	matches := directiveRegex.FindAllStringIndex(wrappedHTML, -1)
	if len(matches) == 0 {
		warns, errExp := c.compileTextSegment(wrappedHTML, &sb, &needsFmt)
		recordWarnings(warns)
		if errExp != nil {
			line := calculateLine(wrappedHTML, errExp.expr)
			return "", false, fmt.Errorf("%s:%d: %w", componentName, line, errExp.err)
		}
		return sb.String(), needsFmt, nil
	}

	var dirStack []struct {
		name string
		line int
	}

	lastIdx := 0
	for _, match := range matches {
		start, end := match[0], match[1]
		if start > lastIdx {
			warns, errExp := c.compileTextSegment(wrappedHTML[lastIdx:start], &sb, &needsFmt)
			recordWarnings(warns)
			if errExp != nil {
				line := calculateLine(wrappedHTML, errExp.expr)
				return "", false, fmt.Errorf("%s:%d: %w", componentName, line, errExp.err)
			}
		}

		dirSnippet := wrappedHTML[start:end]
		line := calculateLine(wrappedHTML, dirSnippet)

		switch {
		case strings.HasPrefix(dirSnippet, "@if("):
			dirStack = append(dirStack, struct {
				name string
				line int
			}{name: "if", line: line})
		case strings.HasPrefix(dirSnippet, "@elseif("):
			if len(dirStack) == 0 || dirStack[len(dirStack)-1].name != "if" {
				return "", false, fmt.Errorf("%s:%d: @elseif without matching @if", componentName, line)
			}
		case dirSnippet == "@else":
			if len(dirStack) == 0 || dirStack[len(dirStack)-1].name != "if" {
				return "", false, fmt.Errorf("%s:%d: @else without matching @if", componentName, line)
			}
		case dirSnippet == "@endif":
			if len(dirStack) == 0 || dirStack[len(dirStack)-1].name != "if" {
				return "", false, fmt.Errorf("%s:%d: @endif without matching @if", componentName, line)
			}
			dirStack = dirStack[:len(dirStack)-1]
		case strings.HasPrefix(dirSnippet, "@foreach("):
			dirStack = append(dirStack, struct {
				name string
				line int
			}{name: "foreach", line: line})
		case dirSnippet == "@endforeach":
			if len(dirStack) == 0 || dirStack[len(dirStack)-1].name != "foreach" {
				return "", false, fmt.Errorf("%s:%d: @endforeach without matching @foreach", componentName, line)
			}
			dirStack = dirStack[:len(dirStack)-1]
		}

		if err := c.compileDirective(dirSnippet, &sb, &needsFmt); err != nil {
			return "", false, fmt.Errorf("%s:%d: %w", componentName, line, err)
		}
		lastIdx = end
	}
	if lastIdx < len(wrappedHTML) {
		warns, errExp := c.compileTextSegment(wrappedHTML[lastIdx:], &sb, &needsFmt)
		recordWarnings(warns)
		if errExp != nil {
			line := calculateLine(wrappedHTML, errExp.expr)
			return "", false, fmt.Errorf("%s:%d: %w", componentName, line, errExp.err)
		}
	}

	if len(dirStack) > 0 {
		top := dirStack[len(dirStack)-1]
		return "", false, fmt.Errorf("%s:%d: unclosed @%s directive (expected @end%s)", componentName, top.line, top.name, top.name)
	}

	return sb.String(), needsFmt, nil
}

func (c *AssetCompiler) compileDirective(dirStr string, sb *strings.Builder, needsFmt *bool) error {
	switch {
	case strings.HasPrefix(dirStr, "@if("):
		expr := strings.TrimSuffix(strings.TrimPrefix(dirStr, "@if("), ")")
		expr = strings.TrimSpace(expr)
		if strings.HasPrefix(expr, ".") {
			field := strings.TrimPrefix(expr, ".")
			if !isVariableName(field) {
				return fmt.Errorf("invalid field in @if: %q", expr)
			}
			sb.WriteString(fmt.Sprintf("\t\tif ui.EvaluateBool(data, \"%s\") {\n", field))
		} else if strings.HasPrefix(expr, "!.") {
			field := strings.TrimPrefix(expr, "!.")
			if !isVariableName(field) {
				return fmt.Errorf("invalid field in @if: %q", expr)
			}
			sb.WriteString(fmt.Sprintf("\t\tif !ui.EvaluateBool(data, \"%s\") {\n", field))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\tif ui.EvaluateBool(data, \"%s\") {\n", expr))
		} else if strings.HasPrefix(expr, "!") && isVariableName(strings.TrimPrefix(expr, "!")) {
			sb.WriteString(fmt.Sprintf("\t\tif !ui.EvaluateBool(data, \"%s\") {\n", strings.TrimPrefix(expr, "!")))
		} else if expr == "true" {
			sb.WriteString("\t\tif true {\n")
		} else if expr == "false" {
			sb.WriteString("\t\tif false {\n")
		} else {
			return fmt.Errorf("invalid @if expression %q: arbitrary Go expressions are not allowed", expr)
		}
	case strings.HasPrefix(dirStr, "@elseif("):
		expr := strings.TrimSuffix(strings.TrimPrefix(dirStr, "@elseif("), ")")
		expr = strings.TrimSpace(expr)
		if strings.HasPrefix(expr, ".") {
			field := strings.TrimPrefix(expr, ".")
			if !isVariableName(field) {
				return fmt.Errorf("invalid field in @elseif: %q", expr)
			}
			sb.WriteString(fmt.Sprintf("\t\t} else if ui.EvaluateBool(data, \"%s\") {\n", field))
		} else if strings.HasPrefix(expr, "!.") {
			field := strings.TrimPrefix(expr, "!.")
			if !isVariableName(field) {
				return fmt.Errorf("invalid field in @elseif: %q", expr)
			}
			sb.WriteString(fmt.Sprintf("\t\t} else if !ui.EvaluateBool(data, \"%s\") {\n", field))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\t} else if ui.EvaluateBool(data, \"%s\") {\n", expr))
		} else if strings.HasPrefix(expr, "!") && isVariableName(strings.TrimPrefix(expr, "!")) {
			sb.WriteString(fmt.Sprintf("\t\t} else if !ui.EvaluateBool(data, \"%s\") {\n", strings.TrimPrefix(expr, "!")))
		} else if expr == "true" {
			sb.WriteString("\t\t} else if true {\n")
		} else if expr == "false" {
			sb.WriteString("\t\t} else if false {\n")
		} else {
			return fmt.Errorf("invalid @elseif expression %q: arbitrary Go expressions are not allowed", expr)
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
			if !isVariableName(listName) || !isValidIdentifier(varName) {
				return fmt.Errorf("invalid @foreach syntax %q: list and item must be valid identifiers", expr)
			}
			sb.WriteString(fmt.Sprintf("\t\tfor _, %sVal := range ui.EvaluateSlice(data, \"%s\") {\n", varName, listName))
			sb.WriteString(fmt.Sprintf("\t\t\tdata := map[string]any{\"$parent\": data, \"%s\": %sVal}\n", varName, varName))
		} else {
			return fmt.Errorf("invalid @foreach syntax %q: expected 'list as item' or 'list in item'", expr)
		}
	case dirStr == "@endforeach":
		sb.WriteString("\t\t}\n")
	}
	return nil
}

func (c *AssetCompiler) compileTextSegment(segment string, sb *strings.Builder, needsFmt *bool) ([]CompileWarning, *exprError) {
	if segment == "" {
		return nil, nil
	}
	var warns []CompileWarning
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
			// Filter pipe chain (including `| required` and compile-time filter validation) (D51/D53)
			compiledExpr, reqField, chainWarns, err := c.compileFilterChain(expr)
			warns = append(warns, chainWarns...)
			if err != nil {
				return warns, &exprError{expr: expr, err: err}
			}
			if reqField != "" {
				sb.WriteString(fmt.Sprintf("\t\t{ reqVal, reqErr := ui.EvaluateRequired(data, %q); if reqErr != nil { return reqErr }; if _, err := io.WriteString(w, ui.Escape(%s)); err != nil { return err } }\n", reqField, compiledExpr))
			} else {
				sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(%s)); err != nil { return err }\n", compiledExpr))
			}
		} else if strings.HasPrefix(expr, ".") {
			fieldName := strings.TrimPrefix(expr, ".")
			if !isVariableName(fieldName) {
				return warns, &exprError{expr: expr, err: fmt.Errorf("invalid field identifier %q", expr)}
			}
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(ui.Evaluate(data, \"%s\"))); err != nil { return err }\n", fieldName))
		} else if isVariableName(expr) {
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(ui.Evaluate(data, \"%s\"))); err != nil { return err }\n", expr))
		} else if isLiteral(expr) {
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, ui.Escape(%s)); err != nil { return err }\n", expr))
		} else if isTransCall(expr) {
			// trans/transRaw/transChoice are defined as local closures in every generated
			// component function (see Run above). They already handle HTML escaping
			// internally, so we emit a direct write — no additional ui.Escape wrapper.
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, %s); err != nil { return err }\n", expr))
		} else {
			return warns, &exprError{expr: expr, err: fmt.Errorf(
				"invalid template expression {{ %s }}: only field references (.Field), "+
					"simple variables, string/number literals, filter pipes (expr | filter), "+
					"and translation calls (trans/transRaw/transChoice) are accepted", expr)}
		}

		restSafe := strings.ReplaceAll(rest, "`", "` + \"`\" + `")
		if restSafe != "" {
			sb.WriteString(fmt.Sprintf("\t\tif _, err := io.WriteString(w, `%s`); err != nil { return err }\n", restSafe))
		}
	}
	return warns, nil
}

// compileFilterChain compiles a pipe-separated filter expression into a Go ui.ApplyFilter call chain.
// It also handles `| required` as a parsed pipe segment and validates all filter names at compile time (D51/D53).
func (c *AssetCompiler) compileFilterChain(expr string) (string, string, []CompileWarning, error) {
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
		return `""`, "", nil, nil
	}

	// Scan pipe segments for `default` and `required`
	base := pipes[0]
	hasDefault := false
	isRequired := false
	for _, pipe := range pipes[1:] {
		p := strings.TrimSpace(pipe)
		if strings.HasPrefix(p, "default(") || strings.HasPrefix(p, "default:") || p == "default" {
			hasDefault = true
		}
		if p == "required" {
			isRequired = true
		}
	}
	if isRequired && hasDefault {
		return "", "", nil, fmt.Errorf("expression %q cannot combine | required with | default", expr)
	}

	var goExpr string
	var reqField string
	var warns []CompileWarning
	if strings.HasPrefix(base, ".") {
		fieldName := strings.TrimPrefix(base, ".")
		if !isVariableName(fieldName) {
			return "", "", nil, fmt.Errorf("invalid field in filter chain: %q", base)
		}
		if isRequired {
			reqField = fieldName
			goExpr = "reqVal"
		} else if hasDefault {
			goExpr = fmt.Sprintf(`ui.EvaluateOptional(data, "%s")`, fieldName)
		} else {
			goExpr = fmt.Sprintf(`ui.Evaluate(data, "%s")`, fieldName)
		}
	} else if isVariableName(base) {
		if isRequired {
			reqField = base
			goExpr = "reqVal"
		} else if hasDefault {
			goExpr = fmt.Sprintf(`ui.EvaluateOptional(data, "%s")`, base)
		} else {
			goExpr = fmt.Sprintf(`ui.Evaluate(data, "%s")`, base)
		}
	} else if isLiteral(base) {
		if isRequired {
			return "", "", nil, fmt.Errorf("| required cannot be applied to literal %q", base)
		}
		goExpr = base
	} else {
		return "", "", nil, fmt.Errorf("invalid base expression in filter chain: %q", base)
	}

	// Apply each filter in the chain
	for _, pipe := range pipes[1:] {
		pipe = strings.TrimSpace(pipe)
		if pipe == "required" {
			continue
		}
		if strings.HasPrefix(pipe, "default:") {
			val := strings.TrimSpace(strings.TrimPrefix(pipe, "default:"))
			val = strings.Trim(val, `"'`)
			goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "default", %s)`, goExpr, strconv.Quote(val))
			continue
		}
		// Parse filter name and optional args: filterName(arg1, arg2)
		if parenIdx := strings.Index(pipe, "("); parenIdx != -1 {
			filterName := strings.TrimSpace(pipe[:parenIdx])
			if !isValidIdentifier(filterName) {
				return "", "", warns, fmt.Errorf("invalid filter name: %q", filterName)
			}
			if !c.isKnownFilter(filterName) {
				if c.WarnUnknownFilters {
					warns = append(warns, CompileWarning{Filter: filterName, Expr: expr})
				} else {
					return "", "", warns, fmt.Errorf("unknown filter %q in expression %q", filterName, expr)
				}
			}
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
						if strings.Contains(cleaned, `\`) || strings.Contains(cleaned, `"`) || strings.Contains(cleaned, `'`) {
							return "", "", warns, fmt.Errorf("filter argument contains invalid characters (quotes/backslashes not allowed): %s", cleaned)
						}
						goArgs = append(goArgs, strconv.Quote(cleaned))
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
				if strings.Contains(cleaned, `\`) || strings.Contains(cleaned, `"`) || strings.Contains(cleaned, `'`) {
					return "", "", warns, fmt.Errorf("filter argument contains invalid characters (quotes/backslashes not allowed): %s", cleaned)
				}
				goArgs = append(goArgs, strconv.Quote(cleaned))
			}

			if len(goArgs) > 0 {
				goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s", %s)`, goExpr, filterName, strings.Join(goArgs, ", "))
			} else {
				goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s")`, goExpr, filterName)
			}
		} else {
			// No args: filterName only
			filterName := pipe
			if !isValidIdentifier(filterName) {
				return "", "", warns, fmt.Errorf("invalid filter name: %q", filterName)
			}
			if !c.isKnownFilter(filterName) {
				if c.WarnUnknownFilters {
					warns = append(warns, CompileWarning{Filter: filterName, Expr: expr})
				} else {
					return "", "", warns, fmt.Errorf("unknown filter %q in expression %q", filterName, expr)
				}
			}
			goExpr = fmt.Sprintf(`ui.ApplyFilter(%s, "%s")`, goExpr, filterName)
		}
	}

	return goExpr, reqField, warns, nil
}

func (c *AssetCompiler) resolveTemplateFile(relPath string) (string, error) {
	roots := []string{c.ComponentsPath}
	if parent := filepath.Dir(c.ComponentsPath); parent != "" && parent != "." && parent != c.ComponentsPath {
		roots = append(roots, parent)
	}

	cleanRel := filepath.Clean(relPath)
	if filepath.IsAbs(cleanRel) {
		return "", fmt.Errorf("security violation: absolute path traversal attempt on %q", relPath)
	}

	var lastErr error
	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			lastErr = err
			continue
		}
		target := filepath.Join(absRoot, cleanRel)
		absTarget, err := filepath.Abs(target)
		if err != nil {
			lastErr = err
			continue
		}

		rel, err := filepath.Rel(absRoot, absTarget)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			lastErr = fmt.Errorf("security violation: path traversal attempt on %q", relPath)
			continue
		}

		if evalPath, err := filepath.EvalSymlinks(absTarget); err == nil {
			if evalRoot, errRoot := filepath.EvalSymlinks(absRoot); errRoot == nil {
				relEval, err := filepath.Rel(evalRoot, evalPath)
				if err != nil || relEval == ".." || strings.HasPrefix(relEval, ".."+string(filepath.Separator)) {
					return "", fmt.Errorf("security violation: symlink points outside root on %q", relPath)
				}
			}
		}

		if _, err := os.Stat(absTarget); err == nil {
			return absTarget, nil
		}
	}

	if lastErr != nil && strings.Contains(lastErr.Error(), "security violation") {
		return "", lastErr
	}
	return "", fmt.Errorf("file %q not found", relPath)
}

// preprocessLayouts resolves template inheritance at compile time.
// Supports @extends, @section/@endsection, @yield, @include, and <slot> composition.
// Returns the preprocessed HTML, whether a layout was actually extended, and any error.
// All regexes use RE2 syntax (no backreferences) for Go compatibility.
func (c *AssetCompiler) preprocessLayouts(componentName, rawHTML string) (string, bool, error) {
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

	// 1. Strip HTML comments before scanning for any directives.
	// This prevents commented-out directives (e.g. <!-- @include("x") -->) from
	// being treated as real instructions and triggering false circular-include errors.
	rawHTML = htmlCommentRe.ReplaceAllString(rawHTML, "")

	// 2. Inline @include partial files (supporting recursive nested includes safely).
	maxDepth := 10
	for depth := 0; depth < maxDepth; depth++ {
		if !strings.Contains(rawHTML, "@include(") {
			break
		}
		var incErr error
		rawHTML = includeRe.ReplaceAllStringFunc(rawHTML, func(incStr string) string {
			if incErr != nil {
				return ""
			}
			m := includeRe.FindStringSubmatch(incStr)
			if len(m) < 2 {
				return ""
			}
			incPath := m[1]
			incFile, err := c.resolveTemplateFile(incPath + ".html")
			if err != nil {
				incErr = fmt.Errorf("invalid @include(%q): %w", incPath, err)
				return ""
			}
			b, err := os.ReadFile(incFile)
			if err != nil {
				incErr = fmt.Errorf("included file %q not readable: %w", incPath, err)
				return ""
			}
			return string(b)
		})
		if incErr != nil {
			return "", false, incErr
		}
	}

	if strings.Contains(rawHTML, "@include(") {
		return "", false, fmt.Errorf("circular or excessively deep @include detected")
	}

	if !strings.Contains(rawHTML, "@extends") {
		return rawHTML, false, nil
	}

	// 2. Extract layout path from @extends("layouts/app").
	m := extendsRe.FindStringSubmatch(rawHTML)
	if len(m) < 2 {
		return rawHTML, false, nil
	}
	layoutPath := m[1]

	layoutFile, err := c.resolveTemplateFile(layoutPath + ".html")
	if err != nil {
		return "", false, fmt.Errorf("layout file %q not found: %w", layoutPath, err)
	}
	layoutBytes, err := os.ReadFile(layoutFile)
	if err != nil {
		return "", false, fmt.Errorf("layout file %q not readable: %w", layoutPath, err)
	}
	layoutHTML := string(layoutBytes)

	// 3. Extract all @section blocks from the child template and validate against document tags.
	sections := sectionRe.FindAllStringSubmatch(rawHTML, -1)
	sectionMap := make(map[string]string)
	for _, sec := range sections {
		if len(sec) >= 3 {
			secContent := sec[2]
			if rootDocTagsRe.MatchString(secContent) {
				return "", false, fmt.Errorf("child template extending layout %q must not author <!DOCTYPE>, <html>, <head>, or <body> tags (author page content inside @section)", layoutPath)
			}
			sectionMap[sec[1]] = secContent
		}
	}

	// If no explicit @section tags were provided, treat the un-extended remainder as the default 'content' section.
	if len(sections) == 0 {
		remainder := extendsRe.ReplaceAllString(rawHTML, "")
		trimmedRemainder := strings.TrimSpace(remainder)
		if trimmedRemainder != "" {
			if rootDocTagsRe.MatchString(trimmedRemainder) {
				return "", false, fmt.Errorf("child template extending layout %q must not author <!DOCTYPE>, <html>, <head>, or <body> tags (author page content inside @section)", layoutPath)
			}
			sectionMap["content"] = trimmedRemainder
		}
	}

	// 4. Replace @yield placeholders in the layout with section content.
	// Primary content slots are automatically wrapped in a component scope badge (<div gs-component="...">)
	// so that component-scoped CSS rules match with surgical precision while preserving outer <!DOCTYPE> Standards Mode.
	merged := yieldRe.ReplaceAllStringFunc(layoutHTML, func(yieldStr string) string {
		ym := yieldRe.FindStringSubmatch(yieldStr)
		if len(ym) < 2 {
			return ""
		}
		slotName := ym[1]
		content, ok := sectionMap[slotName]
		if !ok {
			return ""
		}
		if isHeadSlot(slotName) {
			return content
		}
		return fmt.Sprintf("<div gs-component=\"%s\">\n%s\n</div>", componentName, strings.TrimSpace(content))
	})

	return merged, true, nil
}

var (
	cssCommentRegex           = regexp.MustCompile(`(?s)/\*.*?\*/`)
	atImportRegex             = regexp.MustCompile(`(?i)@import\b`)
	atNamespaceRegex          = regexp.MustCompile(`(?i)@namespace\b`)
	forbiddenCSSSelectorRegex = regexp.MustCompile(`(?i)^(:root|:host|html|body)(\s|$|\[|:|,|\.)`)
)

// scopeCSS transforms raw CSS into component-scoped CSS by prepending a unique
// selector prefix to every rule block, preventing styles from leaking globally.
//
// Rules:
//  1. All comments (single-line, multi-line, inline) are stripped upfront.
//  2. Multi-selector rules (e.g. "h1, h2, .title") have each selector prefixed individually.
//  3. At-rules like "@keyframes" are handled specially: keyframe steps (from, to, 0%, 100%)
//     remain unscoped so CSS animations function properly.
//  4. Nested rules inside "@media" or "@supports" have their selectors scoped correctly.
//  5. Root/document selectors (:root, :host, html, body) and unscoped @import/@namespace directives
//     are strictly rejected at compile time.
func (c *AssetCompiler) scopeCSS(componentName, rawCSS string) (string, error) {
	cleaned := cssCommentRegex.ReplaceAllString(rawCSS, "")
	if atImportRegex.MatchString(cleaned) {
		return "", fmt.Errorf("component %q CSS contains forbidden @import directive: external stylesheet imports are not permitted in component styles", componentName)
	}
	if atNamespaceRegex.MatchString(cleaned) {
		return "", fmt.Errorf("component %q CSS contains forbidden @namespace directive: namespaces are not permitted in component styles", componentName)
	}

	prefix := fmt.Sprintf("[gs-component=\"%s\"]", componentName)

	var sb strings.Builder
	sb.Grow(len(cleaned) + len(cleaned)/4)

	var (
		tokenBuffer    strings.Builder
		braceDepth     int
		keyframesDepth int = -1
	)

	for i := 0; i < len(cleaned); i++ {
		ch := cleaned[i]

		switch ch {
		case '{':
			header := strings.TrimSpace(tokenBuffer.String())
			tokenBuffer.Reset()
			braceDepth++

			if strings.HasPrefix(header, "@") {
				lower := strings.ToLower(header)
				if strings.HasPrefix(lower, "@keyframes") || strings.HasPrefix(lower, "@-webkit-keyframes") {
					keyframesDepth = braceDepth
				}
				sb.WriteString(header)
				sb.WriteString(" {\n")
			} else if keyframesDepth != -1 && braceDepth > keyframesDepth {
				// Inside @keyframes block: keyframe stops (e.g. from, to, 50%) must remain unscoped
				sb.WriteString(header)
				sb.WriteString(" {\n")
			} else {
				// Regular selector: may contain multiple comma-separated targets
				scopedHeader, err := scopeSelectorList(componentName, prefix, header)
				if err != nil {
					return "", err
				}
				sb.WriteString(scopedHeader)
				sb.WriteString(" {\n")
			}

		case '}':
			content := strings.TrimSpace(tokenBuffer.String())
			tokenBuffer.Reset()

			if content != "" {
				lines := strings.Split(content, "\n")
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" {
						sb.WriteString("  ")
						sb.WriteString(trimmed)
						sb.WriteString("\n")
					}
				}
			}

			if keyframesDepth == braceDepth {
				keyframesDepth = -1
			}
			braceDepth--
			if braceDepth < 0 {
				braceDepth = 0
			}

			sb.WriteString("}\n")

		default:
			tokenBuffer.WriteByte(ch)
		}
	}

	tail := strings.TrimSpace(tokenBuffer.String())
	if tail != "" {
		sb.WriteString(tail)
		sb.WriteString("\n")
	}

	return strings.TrimSpace(sb.String()), nil
}

// scopeSelectorList splits comma-separated selectors and prefixes each one individually.
func scopeSelectorList(componentName, prefix, header string) (string, error) {
	parts := strings.Split(header, ",")
	var scopedParts []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if forbiddenCSSSelectorRegex.MatchString(trimmed) {
			return "", fmt.Errorf("component %q CSS contains forbidden root document selector %q: component styles must not target :root, :host, html, or body", componentName, trimmed)
		}
		scopedParts = append(scopedParts, prefix+" "+trimmed)
	}
	return strings.Join(scopedParts, ", "), nil
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

// Start begins the polling loop. It blocks until ctx is cancelled.
// Run this in a goroutine: go watcher.Start(ctx)
func (w *HotReloadWatcher) Start(ctx context.Context) {
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
func (w *HotReloadWatcher) ServeSSE(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("X-Accel-Buffering", "no")
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte(": connected\n\n"))
	if f, ok := rw.(http.Flusher); ok {
		f.Flush()
	}

	ch, unsub := w.subscribe()
	defer unsub()

	done := req.Context().Done()
	for {
		select {
		case <-ch:
			_, _ = rw.Write([]byte("data: reload\n\n"))
			if f, ok := rw.(http.Flusher); ok {
				f.Flush()
			}
		case <-done:
			return
		}
	}
}

