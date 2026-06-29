package auth

import (
	"context"
	"github.com/charledeon77/gostack/framework/contract"
	"github.com/charledeon77/gostack/framework/http"
	netHTTP "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// ─── MOCKS ───────────────────────────────────────────────────────────────────

type mockUser struct {
	id       any
	email    string
	password string
}

func (u *mockUser) GetID() any       { return u.id }
func (u *mockUser) GetEmail() string { return u.email }
func (u *mockUser) GetPassword() string { return u.password }

type mockUserProvider struct {
	users         map[any]*mockUser
	retrieveCalls int
}

func newMockUserProvider() *mockUserProvider {
	return &mockUserProvider{
		users: make(map[any]*mockUser),
	}
}

func (p *mockUserProvider) RetrieveByID(id any) (contract.Authenticatable, error) {
	p.retrieveCalls++
	user, exists := p.users[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (p *mockUserProvider) RetrieveByCredentials(credentials map[string]any) (contract.Authenticatable, error) {
	p.retrieveCalls++
	if token, ok := credentials["token"].(string); ok {
		if token == "valid-token" {
			return &mockUser{id: int64(42), email: "api@example.com"}, nil
		}
		return nil, ErrInvalidToken
	}

	email, ok := credentials["email"].(string)
	if !ok {
		return nil, ErrUserNotFound
	}

	for _, user := range p.users {
		if user.email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (p *mockUserProvider) ValidateCredentials(user contract.Authenticatable, credentials map[string]any) bool {
	plain, ok := credentials["password"].(string)
	if !ok {
		return false
	}
	return plain == user.GetPassword() // Simple plain-text comparison for testing mocks
}

type mockSession struct {
	id   string
	data map[string]any
}

func newMockSession(id string) *mockSession {
	return &mockSession{
		id:   id,
		data: make(map[string]any),
	}
}

func (s *mockSession) ID() string           { return s.id }
func (s *mockSession) Get(key string) any   { return s.data[key] }
func (s *mockSession) Set(key string, v any) { s.data[key] = v }
func (s *mockSession) Delete(key string)    { delete(s.data, key) }
func (s *mockSession) Clear()               { s.data = make(map[string]any) }

// ─── TESTS ───────────────────────────────────────────────────────────────────

func TestBcryptHasher(t *testing.T) {
	hasher := NewBcryptHasher(4) // Use fast cost factor for testing

	hashed, err := hasher.Hash("secret")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	if !hasher.Verify("secret", hashed) {
		t.Error("Verify failed: password should match hash")
	}

	if hasher.Verify("wrong", hashed) {
		t.Error("Verify failed: wrong password should not match hash")
	}
}

func TestSessionGuard(t *testing.T) {
	provider := newMockUserProvider()
	user := &mockUser{id: int64(1), email: "test@example.com", password: "hash"}
	provider.users[int64(1)] = user

	guard := NewSessionGuard("web", provider)

	// 1. Uninitialized session context should fail safely
	req := httptest.NewRequest("GET", "/", nil)
	if guard.Check(req) {
		t.Error("Check should return false when session context is missing")
	}

	// 2. Initialized empty session should fail check
	sess := newMockSession("sess-1")
	ctx := context.WithValue(req.Context(), contract.SessionContextKey, sess)
	req = req.WithContext(ctx)

	if guard.Check(req) {
		t.Error("Check should return false for unauthenticated session")
	}

	// 3. Setup caching container and login
	cache := &contract.AuthCache{}
	ctx = context.WithValue(req.Context(), contract.UserContextKey, cache)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	if err := guard.Login(w, req, user); err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if sess.Get(guard.sessionKey) != int64(1) {
		t.Errorf("Expected session key %s to be 1, got %v", guard.sessionKey, sess.Get(guard.sessionKey))
	}

	// 4. Retrieve user and verify cache works
	provider.retrieveCalls = 0
	retrieved, ok := guard.User(req)
	if !ok || retrieved.GetID() != int64(1) {
		t.Errorf("User() retrieval failed: ok=%v", ok)
	}

	// Retrieve again, retrieveCalls count should remain 0 (serving from context cache)
	_, _ = guard.User(req)
	if provider.retrieveCalls > 0 {
		t.Errorf("Expected user retrieval to be cached, but provider was called %d times", provider.retrieveCalls)
	}

	// 5. Logout should clear session and cache
	if err := guard.Logout(w, req); err != nil {
		t.Fatalf("Logout failed: %v", err)
	}

	if sess.Get(guard.sessionKey) != nil {
		t.Error("Session key should be deleted after logout")
	}

	if guard.Check(req) {
		t.Error("Check should return false after logout")
	}
}

func TestTokenGuard(t *testing.T) {
	provider := newMockUserProvider() // retrieves ID 42 for token "valid-token"
	guard := NewTokenGuard(provider)

	// 1. Missing authorization header should fail check
	req := httptest.NewRequest("GET", "/", nil)
	cache1 := &contract.AuthCache{}
	req = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache1))

	if guard.Check(req) {
		t.Error("Check should fail when token is missing")
	}

	// 2. Invalid bearer token should fail check
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	cache2 := &contract.AuthCache{}
	req = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache2))

	if guard.Check(req) {
		t.Error("Check should fail for invalid bearer token")
	}

	// 3. Valid bearer token should pass check and populate user
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	cache3 := &contract.AuthCache{}
	req = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache3))

	retrieved, ok := guard.User(req)
	if !ok || retrieved.GetID() != int64(42) {
		t.Errorf("Expected token login user ID 42, got %v (ok=%v)", retrieved.GetID(), ok)
	}

	// 4. Query param api_token validation
	req = httptest.NewRequest("GET", "/?api_token=valid-token", nil)
	cache4 := &contract.AuthCache{}
	req = req.WithContext(context.WithValue(req.Context(), contract.UserContextKey, cache4))

	retrieved, ok = guard.User(req)
	if !ok || retrieved.GetID() != int64(42) {
		t.Errorf("Expected query param login user ID 42, got %v (ok=%v)", retrieved.GetID(), ok)
	}
}

func TestCSRFMiddleware(t *testing.T) {
	mw := CSRF("XSRF-TOKEN")

	// Helper to bootstrap a test request context with session
	bootstrapCtx := func(method string, form url.Values) (*http.Context, *mockSession, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		var req *netHTTP.Request
		if form != nil {
			req = httptest.NewRequest(method, "/", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, "/", nil)
		}
		
		sess := newMockSession("sess-1")
		ctx := &http.Context{
			Writer:  w,
			Request: req,
		}
		ctx.Set("session", sess)
		
		// Map session to raw context so CSRF helper can find it
		reqCtx := context.WithValue(req.Context(), contract.SessionContextKey, sess)
		ctx.Request = req.WithContext(reqCtx)
		
		return ctx, sess, w
	}

	// 1. GET requests should bypass verification, but set the token in session and cookie
	ctx, sess, w := bootstrapCtx("GET", nil)
	nextCalled := false
	next := func(c *http.Context) error { nextCalled = true; return nil }

	err := mw(ctx, next)
	if err != nil || !nextCalled {
		t.Fatalf("GET request failed to pass middleware: %v", err)
	}

	tokenVal := sess.Get("csrf_token")
	if tokenVal == nil || tokenVal.(string) == "" {
		t.Fatal("Expected csrf_token to be set in session")
	}
	token := tokenVal.(string)

	cookies := w.Result().Cookies()
	var xsrfCookie *netHTTP.Cookie
	for _, c := range cookies {
		if c.Name == "XSRF-TOKEN" {
			xsrfCookie = c
			break
		}
	}
	if xsrfCookie == nil || xsrfCookie.Value != token {
		t.Errorf("Expected XSRF-TOKEN cookie to match session token: %v", xsrfCookie)
	}

	// 2. POST request without token should be blocked with 403
	ctx, _, w = bootstrapCtx("POST", nil)
	nextCalled = false
	_ = mw(ctx, next)
	if nextCalled || w.Code != netHTTP.StatusForbidden {
		t.Errorf("Expected POST to be blocked (403), got status %d", w.Code)
	}

	// 3. POST request with correct token in header should pass
	ctx, sess, w = bootstrapCtx("POST", nil)
	sess.Set("csrf_token", token) // Preset token
	ctx.Request.Header.Set("X-CSRF-Token", token)
	nextCalled = false

	_ = mw(ctx, next)
	if !nextCalled {
		t.Error("Expected POST with header token to pass")
	}

	// 4. POST request with correct token in form post parameter should pass
	form := url.Values{}
	form.Set("_token", token)
	ctx, sess, w = bootstrapCtx("POST", form)
	sess.Set("csrf_token", token) // Preset token
	nextCalled = false

	_ = mw(ctx, next)
	if !nextCalled {
		t.Error("Expected POST with form parameter token to pass")
	}
}

func TestRouteMiddleware(t *testing.T) {
	provider := newMockUserProvider()
	manager := NewAuthManager("web")
	sessionGuard := NewSessionGuard("web", provider)
	manager.Register("web", sessionGuard)

	requireAuthMw := RequireAuth(manager, "web")
	guestMw := Guest(manager, "web")

	// Helper to bootstrap request
	bootstrapReq := func(isAuthenticated bool) *http.Context {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		sess := newMockSession("sess-1")
		if isAuthenticated {
			sess.Set(sessionGuard.sessionKey, int64(1))
			provider.users[int64(1)] = &mockUser{id: int64(1), email: "test@example.com"}
		}
		
		ctx := &http.Context{
			Writer:  w,
			Request: req,
		}
		ctx.Set("session", sess)
		
		reqCtx := context.WithValue(req.Context(), contract.SessionContextKey, sess)
		ctx.Request = req.WithContext(reqCtx)
		
		return ctx
	}

	// Test RequireAuth - Authenticated (should pass)
	ctx := bootstrapReq(true)
	nextCalled := false
	err := requireAuthMw(ctx, func(c *http.Context) error { nextCalled = true; return nil })
	if err != nil || !nextCalled {
		t.Error("Expected authenticated request to pass RequireAuth")
	}

	// Test RequireAuth - Unauthenticated (should redirect to /login)
	ctx = bootstrapReq(false)
	nextCalled = false
	_ = requireAuthMw(ctx, func(c *http.Context) error { nextCalled = true; return nil })
	if nextCalled {
		t.Error("Expected unauthenticated request to be blocked by RequireAuth")
	}
	if ctx.Writer.(*httptest.ResponseRecorder).Code != netHTTP.StatusFound || 
		ctx.Writer.Header().Get("Location") != "/login" {
		t.Error("Expected redirection to /login")
	}

	// Test RequireAuth - API Unauthenticated (should return 401 JSON)
	ctx = bootstrapReq(false)
	ctx.Request.Header.Set("Accept", "application/json")
	nextCalled = false
	_ = requireAuthMw(ctx, func(c *http.Context) error { nextCalled = true; return nil })
	if nextCalled {
		t.Error("Expected unauthenticated request to be blocked by RequireAuth")
	}
	if ctx.Writer.(*httptest.ResponseRecorder).Code != netHTTP.StatusUnauthorized {
		t.Errorf("Expected 401 Unauthorized for API, got %d", ctx.Writer.(*httptest.ResponseRecorder).Code)
	}

	// Test Guest - Authenticated (should redirect to /home)
	ctx = bootstrapReq(true)
	nextCalled = false
	_ = guestMw(ctx, func(c *http.Context) error { nextCalled = true; return nil })
	if nextCalled {
		t.Error("Expected authenticated user to be blocked by Guest middleware")
	}
	if ctx.Writer.(*httptest.ResponseRecorder).Code != netHTTP.StatusFound || 
		ctx.Writer.Header().Get("Location") != "/home" {
		t.Error("Expected redirection to /home")
	}

	// Test Guest - Unauthenticated (should pass)
	ctx = bootstrapReq(false)
	nextCalled = false
	err = guestMw(ctx, func(c *http.Context) error { nextCalled = true; return nil })
	if err != nil || !nextCalled {
		t.Error("Expected unauthenticated guest request to pass Guest middleware")
	}
}
