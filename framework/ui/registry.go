// Package ui (Tempose + Glide) coordinates AOT component compilation, scoped asset collection,
// and the Glide client-side reactive directive engine runtime injection.
package ui

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"strconv"
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
		inArgs := []reflect.Value{}
		if ft.NumIn() > 0 {
			firstParamType := ft.In(0)
			if val == nil {
				inArgs = append(inArgs, reflect.Zero(firstParamType))
			} else {
				vVal := reflect.ValueOf(val)
				if vVal.Type().AssignableTo(firstParamType) {
					inArgs = append(inArgs, vVal)
				} else if vVal.Type().ConvertibleTo(firstParamType) {
					inArgs = append(inArgs, vVal.Convert(firstParamType))
				} else {
					inArgs = append(inArgs, reflect.ValueOf(val))
				}
			}
		}
		for _, a := range args {
			idx := len(inArgs)
			if ft.NumIn() > idx {
				targetType := ft.In(idx)
				switch targetType.Kind() {
				case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
					if n, err := strconv.ParseInt(a, 10, 64); err == nil {
						inArgs = append(inArgs, reflect.ValueOf(n).Convert(targetType))
					} else {
						inArgs = append(inArgs, reflect.Zero(targetType))
					}
				case reflect.Float64, reflect.Float32:
					if f, err := strconv.ParseFloat(a, 64); err == nil {
						inArgs = append(inArgs, reflect.ValueOf(f).Convert(targetType))
					} else {
						inArgs = append(inArgs, reflect.Zero(targetType))
					}
				case reflect.Bool:
					b := a == "true" || a == "1"
					inArgs = append(inArgs, reflect.ValueOf(b))
				default:
					inArgs = append(inArgs, reflect.ValueOf(a))
				}
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
	var count int
	raw := fmt.Sprint(val)
	if _, err := fmt.Sscanf(raw, "%d", &count); err != nil {
		// D12: non-integer input is returned verbatim — a formatting helper is a no-op on input it does not understand.
		if len(args) == 0 {
			return raw
		}
		return raw
	}
	if len(args) == 0 {
		return raw
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

// HotReloadClientJS provides the browser client SSE connection for development hot reloading.
const HotReloadClientJS = `if (location.hostname === 'localhost' || location.hostname === '127.0.0.1') {
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
}`

var (
	// styleRegistry maintains isolated string mappings of component-scoped styles.
	styleRegistry = make(map[string]string)

	// scriptRegistry maintains component-scoped scripts.
	scriptRegistry = make(map[string]string)

	// registeredStylesheetURL holds the versioned CSS bundle URL (e.g. /assets/css/gostack.css?v=1a2b3c4d).
	registeredStylesheetURL = "/assets/css/gostack.css"

	// registeredScriptURL holds the versioned JS bundle URL (e.g. /assets/js/gostack.js?v=5e6f7g8h).
	registeredScriptURL = "/assets/js/gostack.js"

	// registeredHotReloadURL holds the hot reload client JS URL.
	registeredHotReloadURL = "/assets/js/gostack-hot-reload.js"

	// registryMu guards read/write concurrency across concurrent HTTP threads accessing components.
	registryMu sync.RWMutex
)

// RegisterAssetURLs configures the versioned asset URLs emitted by WriteMasterAssetBlock.
func RegisterAssetURLs(cssURL, jsURL string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if cssURL != "" {
		registeredStylesheetURL = cssURL
	}
	if jsURL != "" {
		registeredScriptURL = jsURL
	}
}

// GetRegisteredAssetURLs returns the registered stylesheet and script URLs.
func GetRegisteredAssetURLs() (cssURL, jsURL string) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registeredStylesheetURL, registeredScriptURL
}

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

// WriteMasterAssetBlock streams the linked stylesheet and script tags into the HTTP response writer.
// It is called once per page render, typically just before the closing </head> tag.
// It emits external <link> and <script defer> tags satisfying strict Content Security Policies (CSP)
// without requiring 'unsafe-inline'. Returns an error if writing to w fails (D30).
func WriteMasterAssetBlock(w io.Writer) error {
	registryMu.RLock()
	cssURL := registeredStylesheetURL
	jsURL := registeredScriptURL
	hotURL := registeredHotReloadURL
	registryMu.RUnlock()

	// 1. External Stylesheet link
	if _, err := io.WriteString(w, fmt.Sprintf("<link rel=\"stylesheet\" href=\"%s\">\n", cssURL)); err != nil {
		return err
	}

	// 2. External Reactive script link
	if _, err := io.WriteString(w, fmt.Sprintf("<script defer src=\"%s\"></script>\n", jsURL)); err != nil {
		return err
	}

	// 3. Development Hot Reload Client script — D13: inject only for explicit dev environments, not "not production"
	appEnv := strings.ToLower(os.Getenv("APP_ENV"))
	if appEnv == "local" || appEnv == "development" {
		if _, err := io.WriteString(w, fmt.Sprintf("<script defer src=\"%s\"></script>\n", hotURL)); err != nil {
			return err
		}
	}
	return nil
}

// EvaluateErr performs a safe runtime reflection lookup of a field name on a given data object.
// Returns an error if an intermediate field or path component fails to resolve (D33).
func EvaluateErr(data any, field string) (any, error) {
	if data == nil {
		return "", fmt.Errorf("evaluator: nil data object for field %q", field)
	}
	parts := strings.Split(field, ".")
	current := data
	for _, part := range parts {
		if part == "" {
			continue
		}
		res, err := evaluateSingleErr(current, part)
		if err != nil {
			return "", err
		}
		current = res
	}
	return current, nil
}

// Evaluate performs a safe runtime reflection lookup of a field name on a given data object.
// It supports nested dot-notation paths (e.g. "User.Profile.Name").
// D33: Structural reflection failures are logged loudly in ALL environments (including production).
func Evaluate(data any, field string) any {
	res, err := EvaluateErr(data, field)
	if err != nil {
		log.Printf("[Tempose Error] Reflection lookup failed for %q: %v", field, err)
		return ""
	}
	return res
}

// EvaluateOptional performs a reflection lookup but returns empty string without logging if the field is absent.
func EvaluateOptional(data any, field string) any {
	res, err := EvaluateErr(data, field)
	if err != nil {
		return ""
	}
	return res
}

func evaluateSingleErr(data any, field string) (any, error) {
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		kv := reflect.ValueOf(field)
		res := val.MapIndex(kv)
		if res.IsValid() {
			return res.Interface(), nil
		}
		return nil, fmt.Errorf("map key %q not found", field)
	case reflect.Struct:
		f := val.FieldByName(field)
		if f.IsValid() {
			if f.CanInterface() {
				return f.Interface(), nil
			}
			return nil, fmt.Errorf("struct field %q is unexported", field)
		}
		// Try method lookup
		m := val.MethodByName(field)
		if m.IsValid() && m.Type().NumIn() == 0 && m.Type().NumOut() == 1 {
			res := m.Call(nil)
			return res[0].Interface(), nil
		}
		return nil, fmt.Errorf("struct field or method %q not found", field)
	default:
		return nil, fmt.Errorf("cannot lookup field %q on type %T", field, data)
	}
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
		lower := strings.ToLower(strings.TrimSpace(v))
		return lower != "" && lower != "false" && lower != "0" && lower != "no" && lower != "off"
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
	if val.Kind() == reflect.Map {
		keys := val.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface())
		})
		out := make([]any, len(keys))
		for i, k := range keys {
			out[i] = val.MapIndex(k).Interface()
		}
		return out
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
