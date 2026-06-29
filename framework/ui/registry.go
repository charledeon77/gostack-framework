// Package ui (Tempose + Glide) coordinates AOT component compilation, scoped asset collection,
// and the Glide client-side reactive directive engine runtime injection.
package ui

import (
	"io"
	"reflect"
	"sync"
)

var (
	// styleRegistry maintains isolated string mappings of component-scoped styles.
	styleRegistry = make(map[string]string)

	// scriptRegistry maintains component-scoped scripts.
	scriptRegistry = make(map[string]string)
	
	// registryMu guards read/write concurrency across concurrent HTTP threads accessing components.
	registryMu sync.RWMutex
)

// RegisterComponentStyle saves isolated, prefixed component CSS configurations during system boot passes.
func RegisterComponentStyle(name, prefixedCSS string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	styleRegistry[name] = prefixedCSS
}

// RegisterComponentScript registers component-scoped JS scripts during system boot passes.
func RegisterComponentScript(name, js string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	scriptRegistry[name] = js
}

// WriteMasterAssetBlock streams the GoStack core styles, the Glide reactive runtime,
// and all registered component-scoped styles and scripts into the HTTP response writer.
// It is called once per page render, typically just before the closing </head> tag.
// No arguments are required — the Glide engine is embedded directly from GlideJS.
func WriteMasterAssetBlock(w io.Writer) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	// ── Styles: core base CSS + all component-scoped styles ──
	_, _ = io.WriteString(w, "<style id=\"gostack-core-styles\">\n")
	_, _ = io.WriteString(w, CoreBaseCSS)
	_, _ = io.WriteString(w, "\n")
	for _, css := range styleRegistry {
		_, _ = io.WriteString(w, css)
		_, _ = io.WriteString(w, "\n")
	}
	_, _ = io.WriteString(w, "</style>\n")

	// ── Scripts: Glide runtime + all component-scoped scripts ──
	_, _ = io.WriteString(w, "<script id=\"gostack-glide-runtime\">\n")
	_, _ = io.WriteString(w, GlideJS)
	_, _ = io.WriteString(w, "\n")
	for _, js := range scriptRegistry {
		_, _ = io.WriteString(w, js)
		_, _ = io.WriteString(w, "\n")
	}
	_, _ = io.WriteString(w, "</script>\n")
}

// Evaluate performs a safe runtime reflection lookup of a field name on a given data object.
func Evaluate(data any, field string) any {
	if data == nil {
		return ""
	}
	
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	
	switch val.Kind() {
	case reflect.Map:
		kv := reflect.ValueOf(field)
		res := val.MapIndex(kv)
		if res.IsValid() {
			return res.Interface()
		}
	case reflect.Struct:
		f := val.FieldByName(field)
		if f.IsValid() {
			return f.Interface()
		}
		// Try method lookup
		m := val.MethodByName(field)
		if m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
			res := m.Call(nil)
			return res[0].Interface()
		}
	}
	return ""
}