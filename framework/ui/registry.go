// Package ui (Tempose + Glide) coordinates AOT component compilation, scoped asset collection,
// and the Glide client-side reactive directive engine runtime injection.
package ui

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode"
)


// SafeHTML represents a string value that has already been sanitized and can be safely rendered as raw HTML.
type SafeHTML string

// Escape sanitizes template output to prevent XSS, unless the value is marked as SafeHTML or template.HTML.
func Escape(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case SafeHTML:
		return string(v)
	case template.HTML:
		return string(v)
	default:
		return html.EscapeString(fmt.Sprint(val))
	}
}

// ── Filter Registry ──────────────────────────────────────────────────────────

var (
	filtersMu sync.RWMutex
	filters   = map[string]any{
		"date":     FilterDate,
		"truncate": FilterTruncate,
		"slugify":  FilterSlugify,
		"plural":   FilterPlural,
		"upper":    FilterUpper,
		"lower":    FilterLower,
	}
)

// RegisterFilter adds or overrides a named template filter.
// The fn value must be a function with signature func(val any, args ...string) string.
func RegisterFilter(name string, fn any) {
	filtersMu.Lock()
	defer filtersMu.Unlock()
	filters[name] = fn
}

// ApplyFilter invokes a named filter on the given value with optional string arguments.
// It returns the filtered string, or the original fmt.Sprint value if the filter is not found.
func ApplyFilter(val any, filterName string, args ...string) string {
	filtersMu.RLock()
	fn, ok := filters[filterName]
	filtersMu.RUnlock()
	if !ok {
		return fmt.Sprint(val)
	}
	switch f := fn.(type) {
	case func(any, ...string) string:
		return f(val, args...)
	case func(any) string:
		return f(val)
	default:
		// Invoke via reflection for custom filter signatures
		fv := reflect.ValueOf(fn)
		ft := fv.Type()
		if ft.Kind() != reflect.Func {
			return fmt.Sprint(val)
		}
		inArgs := []reflect.Value{reflect.ValueOf(val)}
		for _, a := range args {
			if ft.NumIn() > len(inArgs) {
				inArgs = append(inArgs, reflect.ValueOf(a))
			}
		}
		// Pad missing args with zero values
		for len(inArgs) < ft.NumIn() {
			inArgs = append(inArgs, reflect.Zero(ft.In(len(inArgs))))
		}
		out := fv.Call(inArgs)
		if len(out) > 0 {
			return fmt.Sprint(out[0].Interface())
		}
		return fmt.Sprint(val)
	}
}

// FilterDate formats a time.Time or parseable string using Go reference time layout.
// Format defaults to "2006-01-02" if empty.
func FilterDate(val any, args ...string) string {
	layout := "2006-01-02"
	if len(args) > 0 && args[0] != "" {
		layout = args[0]
	}
	switch v := val.(type) {
	case time.Time:
		return v.Format(layout)
	case *time.Time:
		if v == nil {
			return ""
		}
		return v.Format(layout)
	case string:
		for _, f := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if t, err := time.Parse(f, v); err == nil {
				return t.Format(layout)
			}
		}
		return v
	default:
		return fmt.Sprint(val)
	}
}

// FilterTruncate shortens a string to the given character length, appending "…" if cut.
func FilterTruncate(val any, args ...string) string {
	s := fmt.Sprint(val)
	length := 80
	if len(args) > 0 {
		if _, err := fmt.Sscanf(args[0], "%d", &length); err != nil {
			length = 80
		}
	}
	runes := []rune(s)
	if len(runes) <= length {
		return s
	}
	return string(runes[:length]) + "…"
}

// FilterSlugify converts a string to a URL-safe, dash-separated slug.
func FilterSlugify(val any, _ ...string) string {
	s := fmt.Sprint(val)
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteRune('-')
			prevDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// FilterPlural returns the singular or plural form based on the "count" string arg.
// Usage in template: {{ count | plural("apple", "apples") }}
func FilterPlural(val any, args ...string) string {
	count := 0
	_, _ = fmt.Sscanf(fmt.Sprint(val), "%d", &count)
	if len(args) == 0 {
		return fmt.Sprint(val)
	}
	if len(args) == 1 {
		if count == 1 {
			return fmt.Sprintf("1 %s", args[0])
		}
		return fmt.Sprintf("%d %ss", count, args[0])
	}
	if count == 1 {
		return fmt.Sprintf("1 %s", args[0])
	}
	return fmt.Sprintf("%d %s", count, args[1])
}

// FilterUpper converts the value to uppercase.
func FilterUpper(val any, _ ...string) string {
	return strings.ToUpper(fmt.Sprint(val))
}

// FilterLower converts the value to lowercase.
func FilterLower(val any, _ ...string) string {
	return strings.ToLower(fmt.Sprint(val))
}



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
	_, _ = io.WriteString(w, GoStackCoreCSS)
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

	// ── Development Hot Reload Client script ──
	if os.Getenv("APP_ENV") != "production" {
		_, _ = io.WriteString(w, `<script id="gostack-hot-reload">
  if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
    let disconnected = false;
    function connectSSE() {
      const es = new EventSource('/__gostack_reload');
      es.onmessage = (e) => {
        if (e.data === 'reload') {
          location.reload();
        }
      };
      es.onopen = () => {
        if (disconnected) {
          location.reload();
        }
      };
      es.onerror = () => {
        disconnected = true;
        es.close();
        setTimeout(connectSSE, 1000);
      };
    }
    connectSSE();
  }
</script>
`)
	}
}

// Evaluate performs a safe runtime reflection lookup of a field name on a given data object.
// It supports nested dot-notation paths (e.g. "User.Profile.Name").
func Evaluate(data any, field string) any {
	if data == nil {
		return ""
	}
	parts := strings.Split(field, ".")
	current := data
	for _, part := range parts {
		if part == "" {
			continue
		}
		current = evaluateSingle(current, part)
		if current == nil {
			return ""
		}
	}
	return current
}

func evaluateSingle(data any, field string) any {
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
	return nil
}

// EvaluateBool inspects the resolved value of a field path and returns its truthiness.
func EvaluateBool(data any, field string) bool {
	res := Evaluate(data, field)
	if res == nil {
		return false
	}
	switch v := res.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false" && v != "0"
	case int:
		return v != 0
	case int8:
		return v != 0
	case int16:
		return v != 0
	case int32:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	case uint8:
		return v != 0
	case uint16:
		return v != 0
	case uint32:
		return v != 0
	case uint64:
		return v != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	}
	// Check if slice/map/array has elements
	rv := reflect.ValueOf(res)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Map || rv.Kind() == reflect.Array {
		return rv.Len() > 0
	}
	return true
}

// EvaluateSlice inspects the field path and returns a slice of interfaces.
func EvaluateSlice(data any, field string) []any {
	res := Evaluate(data, field)
	if res == nil {
		return nil
	}
	val := reflect.ValueOf(res)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return nil
	}
	out := make([]any, val.Len())
	for i := 0; i < val.Len(); i++ {
		out[i] = val.Index(i).Interface()
	}
	return out
}