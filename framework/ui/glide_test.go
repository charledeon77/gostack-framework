package ui

import (
	"strings"
	"testing"
)

func TestGlide_SEC40_NoNewFunctionOrWith(t *testing.T) {
	// 1. Shipped GlideJS must have ZERO occurrences of new Function and with(
	if strings.Contains(GlideJS, "new Function") {
		t.Errorf("Security violation: GlideJS contains 'new Function'!")
	}
	if strings.Contains(GlideJS, "with(scope)") || strings.Contains(GlideJS, "with (scope)") {
		t.Errorf("Security violation: GlideJS contains 'with(scope)'!")
	}

	// 2. FORBIDDEN_IDENTIFIERS must block global scope access
	forbidden := []string{"'window'", "'document'", "'cookie'", "'global'", "'eval'", "'Function'"}
	for _, id := range forbidden {
		if !strings.Contains(GlideJS, id) {
			t.Errorf("Expected FORBIDDEN_IDENTIFIERS to contain %s", id)
		}
	}
}

func TestGlide_SEC41_SinkGuards(t *testing.T) {
	// 1. Attribute safety check must block on* event handlers and javascript: URLs
	if !strings.Contains(GlideJS, "isSafeAttribute") {
		t.Fatal("Expected GlideJS to contain isSafeAttribute helper")
	}
	if !strings.Contains(GlideJS, "javascript:") {
		t.Errorf("Expected isSafeAttribute to check for 'javascript:' pseudo-protocol")
	}
	if !strings.Contains(GlideJS, "startsWith('on')") {
		t.Errorf("Expected isSafeAttribute to block inline on* event handler attributes")
	}

	// 2. gs-raw must be supported and gs-html must produce a console warning
	if !strings.Contains(GlideJS, "gs-raw") {
		t.Errorf("Expected GlideJS to support gs-raw explicit safe sink")
	}
	if !strings.Contains(GlideJS, `console.warn('[Glide Security] "gs-html" writes raw unescaped HTML`) {
		t.Errorf("Expected GlideJS to issue a security warning on gs-html")
	}
}

func TestGlide_SEC42_PersistOptIn(t *testing.T) {
	// 1. LocalStorage must only be populated when opt-in gs-persist attribute is present
	if !strings.Contains(GlideJS, "canPersist") || !strings.Contains(GlideJS, "gs-persist") {
		t.Errorf("Expected GlideJS to check for gs-persist opt-in attribute")
	}

	// 2. State keys must be namespaced with gs_persist_
	if !strings.Contains(GlideJS, "gs_persist_") {
		t.Errorf("Expected GlideJS to namespace persisted keys with gs_persist_")
	}

	// 3. Glide must expose clearPersistedState for session logout clearing
	if !strings.Contains(GlideJS, "clearPersistedState") {
		t.Errorf("Expected Glide to export clearPersistedState method")
	}
}

func TestGlide_SEC43_RouterOKAndOriginGuard(t *testing.T) {
	// 1. Same-origin URL validation before fetching
	if !strings.Contains(GlideJS, "targetUrl.origin !== window.location.origin") {
		t.Errorf("Expected Glide router navigate() to verify same-origin")
	}

	// 2. Check res.ok before morphing
	if !strings.Contains(GlideJS, "!res.ok") {
		t.Errorf("Expected Glide router navigate() to check res.ok")
	}

	// 3. Handle 404 explicitly and shield 500 from morphing
	if !strings.Contains(GlideJS, "res.status === 404") {
		t.Errorf("Expected Glide router navigate() to handle 404 status")
	}
	if !strings.Contains(GlideJS, "Navigation failed with HTTP") {
		t.Errorf("Expected Glide router navigate() to log HTTP failure on non-OK")
	}
}

