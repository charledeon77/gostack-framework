package http

import (
	netHTTP "net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMethodOverrideMiddleware(t *testing.T) {
	router := NewRouter()

	var matchedMethod string
	router.Delete("/posts/1", func(ctx *Context) {
		matchedMethod = "DELETE"
		ctx.Writer.Write([]byte("deleted"))
	}, MethodOverrideMiddleware())

	router.Put("/posts/1", func(ctx *Context) {
		matchedMethod = "PUT"
		ctx.Writer.Write([]byte("updated"))
	}, MethodOverrideMiddleware())

	engine := NewEngine(router, nil)

	// 1. Override via query string _method=DELETE
	req := httptest.NewRequest(netHTTP.MethodPost, "/posts/1?_method=DELETE", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if matchedMethod != "DELETE" {
		t.Errorf("expected DELETE route matched, got %s (code=%d)", matchedMethod, w.Code)
	}

	// 2. Override via X-HTTP-Method-Override header
	matchedMethod = ""
	req2 := httptest.NewRequest(netHTTP.MethodPost, "/posts/1", nil)
	req2.Header.Set("X-HTTP-Method-Override", "PUT")
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)

	if matchedMethod != "PUT" {
		t.Errorf("expected PUT route matched, got %s (code=%d)", matchedMethod, w2.Code)
	}

	// 3. Override via form body
	matchedMethod = ""
	form := url.Values{}
	form.Set("_method", "DELETE")
	req3 := httptest.NewRequest(netHTTP.MethodPost, "/posts/1", strings.NewReader(form.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, req3)

	if matchedMethod != "DELETE" {
		t.Errorf("expected DELETE via form body, got %s (code=%d)", matchedMethod, w3.Code)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	router := NewRouter()

	var capturedCtxID string
	router.Get("/health", func(ctx *Context) {
		if id, ok := ctx.Get("request_id").(string); ok {
			capturedCtxID = id
		}
		ctx.Writer.Write([]byte("OK"))
	}, RequestIDMiddleware())

	engine := NewEngine(router, nil)

	// Case 1: Fresh request generates ID
	req := httptest.NewRequest(netHTTP.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	headerID := w.Header().Get("X-Request-ID")
	if headerID == "" {
		t.Fatal("expected X-Request-ID header to be set")
	}
	if capturedCtxID != headerID {
		t.Errorf("context ID (%s) != header ID (%s)", capturedCtxID, headerID)
	}

	// Case 2: Existing X-Request-ID is preserved
	req2 := httptest.NewRequest(netHTTP.MethodGet, "/health", nil)
	req2.Header.Set("X-Request-ID", "custom-gateway-id-12345")
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)

	if got := w2.Header().Get("X-Request-ID"); got != "custom-gateway-id-12345" {
		t.Errorf("expected custom ID preserved, got %s", got)
	}
}
