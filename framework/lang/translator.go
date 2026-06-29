package lang

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
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
