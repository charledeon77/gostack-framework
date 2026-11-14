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
	"crypto/tls"
	netHTTP "net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
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

// TestSessionCookie_SecurityAttributes verifies SEC-02 requirements.
func TestSessionCookie_SecurityAttributes(t *testing.T) {
	store := NewInMemoryStore()
	mw := SessionMiddleware(store, "gostack_session")

	// 1. Simulated TLS request contains Secure and SameSite=Lax
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	reqTLS := httptest.NewRequest("GET", "/", nil)
	reqTLS.TLS = &tls.ConnectionState{}
	wTLS := httptest.NewRecorder()
	ctxTLS := &Context{Writer: wTLS, Request: reqTLS}
	_ = mw(ctxTLS, func(c *Context) error { return nil })

	var tlsCookie *netHTTP.Cookie
	for _, c := range wTLS.Result().Cookies() {
		if c.Name == "gostack_session" {
			tlsCookie = c
			break
		}
	}
	if tlsCookie == nil || !tlsCookie.Secure || tlsCookie.SameSite != netHTTP.SameSiteLaxMode {
		t.Fatalf("expected TLS cookie with Secure=true and SameSite=Lax, got: %+v", tlsCookie)
	}

	// 2. Plain HTTP in APP_ENV=local omits Secure and still has SameSite=Lax
	reqLocal := httptest.NewRequest("GET", "/", nil)
	wLocal := httptest.NewRecorder()
	ctxLocal := &Context{Writer: wLocal, Request: reqLocal}
	_ = mw(ctxLocal, func(c *Context) error { return nil })

	var localCookie *netHTTP.Cookie
	for _, c := range wLocal.Result().Cookies() {
		if c.Name == "gostack_session" {
			localCookie = c
			break
		}
	}
	if localCookie == nil || localCookie.Secure || localCookie.SameSite != netHTTP.SameSiteLaxMode {
		t.Fatalf("expected local HTTP cookie with Secure=false and SameSite=Lax, got: %+v", localCookie)
	}

	// 3. Max-Age equals configured lifetime
	os.Setenv("SESSION_LIFETIME", "60")
	defer os.Unsetenv("SESSION_LIFETIME")
	wTTL := httptest.NewRecorder()
	ctxTTL := &Context{Writer: wTTL, Request: reqLocal}
	_ = mw(ctxTTL, func(c *Context) error { return nil })
	for _, c := range wTTL.Result().Cookies() {
		if c.Name == "gostack_session" {
			if c.MaxAge != 3600 {
				t.Fatalf("expected MaxAge=3600 from SESSION_LIFETIME=60, got %d", c.MaxAge)
			}
		}
	}
}

// TestSession_SEC16_AdoptionProtection verifies SEC-16 requirements.
func TestSession_SEC16_AdoptionProtection(t *testing.T) {
	store := NewInMemoryStore()
	mw := SessionMiddleware(store, "gostack_session")

	// 1. Request with malformed ID (deadbeef) -> gets a newly minted valid ID
	reqMalformed := httptest.NewRequest("GET", "/", nil)
	reqMalformed.AddCookie(&netHTTP.Cookie{Name: "gostack_session", Value: "deadbeef"})
	w1 := httptest.NewRecorder()
	_ = mw(&Context{Writer: w1, Request: reqMalformed}, func(c *Context) error { return nil })

	var cookie1 *netHTTP.Cookie
	for _, c := range w1.Result().Cookies() {
		if c.Name == "gostack_session" {
			cookie1 = c
			break
		}
	}
	if cookie1 == nil || cookie1.Value == "deadbeef" || len(cookie1.Value) != 64 {
		t.Fatalf("expected fresh 64-character session ID, got: %+v", cookie1)
	}

	// 2. Correctly formatted 64-char hex but unknown in store -> gets new ID
	unknownID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	reqUnknown := httptest.NewRequest("GET", "/", nil)
	reqUnknown.AddCookie(&netHTTP.Cookie{Name: "gostack_session", Value: unknownID})
	w2 := httptest.NewRecorder()
	_ = mw(&Context{Writer: w2, Request: reqUnknown}, func(c *Context) error { return nil })

	var cookie2 *netHTTP.Cookie
	for _, c := range w2.Result().Cookies() {
		if c.Name == "gostack_session" {
			cookie2 = c
			break
		}
	}
	if cookie2 == nil || cookie2.Value == unknownID {
		t.Fatalf("expected unknown ID to be rejected and replaced, got %v", cookie2)
	}

	// 3. Request with a live, existing ID -> same ID reused, no new cookie emitted
	liveID := cookie2.Value
	reqLive := httptest.NewRequest("GET", "/", nil)
	reqLive.AddCookie(&netHTTP.Cookie{Name: "gostack_session", Value: liveID})
	w3 := httptest.NewRecorder()
	_ = mw(&Context{Writer: w3, Request: reqLive}, func(c *Context) error { return nil })

	if len(w3.Result().Cookies()) != 0 {
		t.Fatalf("expected no new Set-Cookie for live session, got %d cookies", len(w3.Result().Cookies()))
	}
}

// TestSessionStore_SEC04_RotateAcrossStores tests Rotate and Exists on SessionStore implementations.
func TestSessionStore_SEC04_RotateAcrossStores(t *testing.T) {
	stores := []struct {
		name  string
		store contract.SessionStore
	}{
		{"InMemoryStore", NewInMemoryStore()},
	}

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			s, err := tc.store.Load(generateSessionID())
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			s.Set("role", "admin")
			_ = tc.store.Save(s)

			oldID := s.ID()
			if !tc.store.Exists(oldID) {
				t.Fatalf("expected old ID %s to exist in store", oldID)
			}

			newSess, err := tc.store.Rotate(oldID)
			if err != nil {
				t.Fatalf("Rotate failed: %v", err)
			}

			if newSess.ID() == oldID {
				t.Fatal("expected new ID to differ from old ID")
			}
			if tc.store.Exists(oldID) {
				t.Fatal("expected old ID to be deleted after rotate")
			}
			if !tc.store.Exists(newSess.ID()) {
				t.Fatal("expected new ID to exist in store")
			}
			if newSess.Get("role") != "admin" {
				t.Fatalf("expected data 'role'='admin' to be migrated, got %v", newSess.Get("role"))
			}
		})
	}
}

// TestSessionStore_SEC14_SweeperEviction tests background sweeping of expired sessions.
func TestSessionStore_SEC14_SweeperEviction(t *testing.T) {
	store := NewInMemoryStore(50 * time.Millisecond)
	defer store.Close()

	s, _ := store.Load(generateSessionID())
	s.Set("foo", "bar")
	_ = store.Save(s)

	if store.ActiveCount() != 1 {
		t.Fatalf("expected 1 active session, got %d", store.ActiveCount())
	}

	time.Sleep(120 * time.Millisecond)
	store.Sweep()

	if store.ActiveCount() != 0 {
		t.Fatalf("expected 0 active sessions after sweep, got %d", store.ActiveCount())
	}
}
