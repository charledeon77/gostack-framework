package lang

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslator_EmbedFallback(t *testing.T) {
	translator := NewTranslator("")

	enMsg := translator.Trans("en", "auth.failed", nil)
	if enMsg != "These credentials do not match our records." {
		t.Errorf("Unexpected English translation: %q", enMsg)
	}

	esMsg := translator.Trans("es", "auth.failed", nil)
	if esMsg != "Estas credenciales no coinciden con nuestros registros." {
		t.Errorf("Unexpected Spanish translation: %q", esMsg)
	}

	missingMsg := translator.Trans("xx", "auth.failed", nil)
	if missingMsg != "These credentials do not match our records." {
		t.Errorf("Expected fallback to English for missing locale, got: %q", missingMsg)
	}
}

func TestTranslator_CustomOverrides(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gostack_lang")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	customEn := `{"auth.failed": "Oops, bad login info!", "custom.key": "Hello {{name}}"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "en.json"), []byte(customEn), 0644); err != nil {
		t.Fatalf("Failed to write custom en.json: %v", err)
	}

	translator := NewTranslator(tmpDir)

	enMsg := translator.Trans("en", "auth.failed", nil)
	if enMsg != "Oops, bad login info!" {
		t.Errorf("Expected custom English override, got: %q", enMsg)
	}

	interpolated := translator.Trans("en", "custom.key", map[string]string{"name": "Alice"})
	if interpolated != "Hello Alice" {
		t.Errorf("Expected interpolated value, got: %q", interpolated)
	}
}
