/*
Purpose:
This file contains unit tests to verify the correctness of the memory session implementation,
the in-memory session store, and the session injection middleware.

Philosophy:
We believe robust test suites are critical for framework stability. The session layer handles
sensitive user state; thus, concurrent access safety, persistence across requests, and proper
cookie header output must be fully covered by automated unit tests.
*/
package http

import (
	netHTTP "net/http"
	"net/http/httptest"
	"testing"
)

// TestMemorySession verifies basic get, set, delete, and clear capabilities of MemorySession.
func TestMemorySession(t *testing.T) {
	sess := NewMemorySession("test_id")
	if sess.ID() != "test_id" {
		t.Errorf("Expected ID to be test_id, got %s", sess.ID())
	}

	sess.Set("foo", "bar")
	if val := sess.Get("foo"); val != "bar" {
		t.Errorf("Expected Get('foo') to return 'bar', got %v", val)
	}

	sess.Delete("foo")
	if val := sess.Get("foo"); val != nil {
		t.Errorf("Expected Get('foo') to be nil after delete, got %v", val)
	}

	sess.Set("a", 1)
	sess.Set("b", 2)
	sess.Clear()
	if val := sess.Get("a"); val != nil {
		t.Errorf("Expected Get('a') to be nil after clear, got %v", val)
	}
}

// TestInMemoryStore verifies Load, Save, and Destroy interactions on the memory store.
func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()
	sess, err := store.Load("sess1")
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	if sess.ID() != "sess1" {
		t.Errorf("Expected session ID to be sess1, got %s", sess.ID())
	}

	sess.Set("name", "GoStack")
	if err := store.Save(sess); err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}

	// Load again and verify persistence
	sess2, err := store.Load("sess1")
	if err != nil {
		t.Fatalf("Failed to reload session: %v", err)
	}

	if val := sess2.Get("name"); val != "GoStack" {
		t.Errorf("Expected loaded session value to be GoStack, got %v", val)
	}

	// Destroy
	if err := store.Destroy("sess1"); err != nil {
		t.Fatalf("Failed to destroy session: %v", err)
	}

	// Load after destroy should return a fresh, empty session
	sess3, err := store.Load("sess1")
	if err != nil {
		t.Fatalf("Failed to load after destroy: %v", err)
	}
	if val := sess3.Get("name"); val != nil {
		t.Errorf("Expected session to be empty after destroy, got %v", val)
	}
}

// TestSessionMiddleware verifies that the session middleware correctly reads and writes cookies,
// injects the session into context, and commits modified session data back to the store.
func TestSessionMiddleware(t *testing.T) {
	store := NewInMemoryStore()
	mw := SessionMiddleware(store, "gostack_session")

	// Set up a mock pipeline/handler that modifies session data
	handler := func(ctx *Context) error {
		sess := GetSession(ctx)
		if sess == nil {
			t.Error("Expected session to be present in context")
			return nil
		}
		sess.Set("user", "antigravity")
		return ctx.JSON(netHTTP.StatusOK, map[string]string{"status": "ok"})
	}

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	ctx := &Context{
		Writer:  w,
		Request: req,
	}

	// Execute middleware
	err := mw(ctx, handler)
	if err != nil {
		t.Fatalf("Middleware execution failed: %v", err)
	}

	// Verify response cookie was set
	cookies := w.Result().Cookies()
	var sessionCookie *netHTTP.Cookie
	for _, c := range cookies {
		if c.Name == "gostack_session" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("Expected gostack_session cookie to be set in response headers")
	}

	if sessionCookie.Value == "" {
		t.Error("Expected session cookie value to be non-empty")
	}

	// Verify session data was saved to the store
	sess, err := store.Load(sessionCookie.Value)
	if err != nil {
		t.Fatalf("Failed to load session from store using cookie value: %v", err)
	}

	if val := sess.Get("user"); val != "antigravity" {
		t.Errorf("Expected session value 'user' to be saved as 'antigravity', got %v", val)
	}
}
