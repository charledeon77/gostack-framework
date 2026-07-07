package lang

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)


//go:embed core_translations/*.json
var coreFs embed.FS

// FileTranslator loads translation dictionaries from core embed files
// and merges custom user translations from a specified local directory.
type FileTranslator struct {
	mu        sync.RWMutex
	dict      map[string]map[string]string // locale -> key -> value
	customDir string
}

// NewTranslator constructs and loads a FileTranslator instance.
func NewTranslator(customDir string) *FileTranslator {
	t := &FileTranslator{
		dict:      make(map[string]map[string]string),
		customDir: customDir,
	}
	t.loadCore()
	if customDir != "" {
		t.loadCustom()
	}
	return t
}

// loadCore parses the core embedded translation dictionaries.
func (t *FileTranslator) loadCore() {
	files, err := coreFs.ReadDir("core_translations")
	if err != nil {
		return
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(f.Name(), ".json")
		data, err := coreFs.ReadFile("core_translations/" + f.Name())
		if err != nil {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		t.dict[locale] = m
	}
}

// loadCustom reads and merges custom local JSON translations overriding the core translations.
func (t *FileTranslator) loadCustom() {
	files, err := os.ReadDir(t.customDir)
	if err != nil {
		return // Ignore missing or unreadable custom directories
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		locale := strings.TrimSuffix(f.Name(), ".json")
		data, err := os.ReadFile(filepath.Join(t.customDir, f.Name()))
		if err != nil {
			continue
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		t.mu.Lock()
		if _, exists := t.dict[locale]; !exists {
			t.dict[locale] = make(map[string]string)
		}
		for k, v := range m {
			t.dict[locale][k] = v
		}
		t.mu.Unlock()
	}
}

// Trans resolves a localized string key in a given target locale, interpolating placeholders.
func (t *FileTranslator) Trans(locale string, key string, replace map[string]string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	localeMap, exists := t.dict[locale]
	if !exists {
		localeMap, exists = t.dict["en"]
		if !exists {
			return key
		}
	}

	val, exists := localeMap[key]
	if !exists {
		if locale != "en" {
			if enMap, ok := t.dict["en"]; ok {
				if enVal, ok := enMap[key]; ok {
					val = enVal
					exists = true
				}
			}
		}
		if !exists {
			if os.Getenv("APP_ENV") != "production" {
				fmt.Printf("[Transios Warning] Missing translation key: %q for locale: %q\n", key, locale)
			}
			return key
		}
	}

	for k, v := range replace {
		val = strings.ReplaceAll(val, ":"+k, v)
		val = strings.ReplaceAll(val, "{{"+k+"}}", v)
		val = strings.ReplaceAll(val, "{{ "+k+" }}", v)
	}

	return val
}

// TransChoice resolves a pluralized translation key based on a numeric count.
//
// Translation files support two pluralization syntaxes:
//
// 1. Simple pipe — two forms separated by |:
//
//	"apple|apples"  (singular|plural)
//
// 2. Extended pipe — explicit count-range guards separated by |:
//
//	"{0}no apples|{1}one apple|[2,*]many apples"
//	"{0}no items|[1,5]:count items|[6,*]many items"
//
// Guards:
//   - {n}   — matches the exact count n
//   - [a,b] — matches the inclusive range a..b; use * for open-ended upper bound
//
// The :count placeholder in the chosen form is replaced with the actual count.
// Any additional replacements from the replace map are also applied.
func (t *FileTranslator) TransChoice(locale string, key string, count int, replace map[string]string) string {
	raw := t.Trans(locale, key, nil) // resolve key without replacements first
	if raw == key {
		return key // key not found — return as-is
	}

	chosen := choosePlural(raw, count)

	// Merge :count into the replacements map
	merged := map[string]string{"count": strings.ReplaceAll(fmt.Sprint(count), "%!v(MISSING)", "")}
	for k, v := range replace {
		merged[k] = v
	}
	for k, v := range merged {
		chosen = strings.ReplaceAll(chosen, ":"+k, v)
		chosen = strings.ReplaceAll(chosen, "{{"+k+"}}", v)
		chosen = strings.ReplaceAll(chosen, "{{ "+k+" }}", v)
	}
	return chosen
}

// choosePlural selects the correct plural form from a pipe-separated string.
func choosePlural(raw string, count int) string {
	parts := strings.Split(raw, "|")
	if len(parts) == 1 {
		// No pipe — return as-is
		return raw
	}

	// Check for extended syntax ({n}... / [a,b]...) in any segment
	hasGuards := false
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			hasGuards = true
			break
		}
	}

	if !hasGuards {
		// Simple binary: singular | plural
		if count == 1 {
			return strings.TrimSpace(parts[0])
		}
		return strings.TrimSpace(parts[len(parts)-1])
	}

	// Extended: find first matching guard
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if strings.HasPrefix(trimmed, "{") {
			end := strings.Index(trimmed, "}")
			if end == -1 {
				continue
			}
			guardStr := trimmed[1:end]
			n, err := strconv.Atoi(guardStr)
			if err == nil && n == count {
				return strings.TrimSpace(trimmed[end+1:])
			}
		} else if strings.HasPrefix(trimmed, "[") {
			end := strings.Index(trimmed, "]")
			if end == -1 {
				continue
			}
			rangeParts := strings.Split(trimmed[1:end], ",")
			if len(rangeParts) == 2 {
				low, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				highStr := strings.TrimSpace(rangeParts[1])
				if err1 != nil {
					continue
				}
				if highStr == "*" {
					if count >= low {
						return strings.TrimSpace(trimmed[end+1:])
					}
				} else {
					high, err2 := strconv.Atoi(highStr)
					if err2 == nil && count >= low && count <= high {
						return strings.TrimSpace(trimmed[end+1:])
					}
				}
			}
		}
	}

	// Fallback: last segment
	return strings.TrimSpace(parts[len(parts)-1])
}

