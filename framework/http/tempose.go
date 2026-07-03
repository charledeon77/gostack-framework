// Package http (Navigator, Bridge, Tempose, and Glide) houses the core HTTP request-response lifecycle management.
// It provides the primitives for routing, context propagation, and view rendering
// for the GoStack framework.
package http

import (
	"bytes"
	"fmt"
	"github.com/charledeon77/gostack-framework/framework/ui"
	"io"
	"strings"
)

// ViewFunc defines the functional signature for a compiled view.
// In GoStack, we treat HTML templates as executable Go functions rather than 
// interpreted strings. This provides two massive advantages:
// 1. Compile-time safety: Any syntax error in logic is caught during the build.
// 2. Performance: Avoiding reflection and parsing at runtime ensures O(1) 
//    dispatching, reaching parity with the world's fastest web frameworks.
type ViewFunc func(w io.Writer, data any) error

// Tempose acts as a centralized registry for all application views.
//
// Architectural Rationale:
// Unlike traditional frameworks that parse disk files on every request, 
// Tempose maintains an in-memory map of compiled functions. This registry 
// is initialized at application boot-time, ensuring that the framework 
// fails fast if a template is missing.
type Tempose struct {
	// views stores the registry of template functions, indexed by their unique path.
	views map[string]ViewFunc
}

// NewTempose initializes the view registry. 
// This should be called by the application's bootstrapper during the
// dependency injection phase.
func NewTempose() *Tempose {
	return &Tempose{
		views: make(map[string]ViewFunc),
	}
}

// Register explicitly maps a template name to a compiled ViewFunc.
//
// Usage: This is typically invoked by generated code during the 'gostack build' 
// process to wire up the application's view layer.
func (t *Tempose) Register(name string, fn ViewFunc) {
	t.views[name] = fn
}

// Render executes the requested view and writes the output to the provided io.Writer.
//
// Architectural Note: This method is now purely focused on rendering. 
// It does not touch HTTP status codes, keeping it decoupled and 
// usable in any I/O context (e.g., CLI, Email, HTTP).
//
// If a closing </head> tag is detected in the rendered view, Tempose automatically
// injects the Master Asset Block (CSS & GlideJS runtime) right before </head> to hydrate the page.
//
// Returns an error if the requested view name has not been registered.
func (t *Tempose) Render(w io.Writer, name string, data any) error {
	view, ok := t.views[name]
	if !ok {
		return fmt.Errorf("tempose: view '%s' not registered in the system", name)
	}
	
	var buf bytes.Buffer
	if err := view(&buf, data); err != nil {
		return err
	}
	
	html := buf.String()
	if idx := strings.Index(strings.ToLower(html), "</head>"); idx != -1 {
		// Write everything up to </head>
		if _, err := io.WriteString(w, html[:idx]); err != nil {
			return err
		}
		// Write the master asset block (base styles + components CSS + GlideJS + component scripts)
		ui.WriteMasterAssetBlock(w)
		// Write the rest of the HTML (including the closing </head> tag)
		if _, err := io.WriteString(w, html[idx:]); err != nil {
			return err
		}
	} else {
		// Fragment rendering (e.g. HTMX swaps or partials) — just stream straight to the writer
		if _, err := io.WriteString(w, html); err != nil {
			return err
		}
	}
	
	return nil
}