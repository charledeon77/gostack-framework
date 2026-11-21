package http

import (
	"bytes"
	"errors"
	"io"
	netHTTP "net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestEngine_SEC05_ErrorAndRecovery(t *testing.T) {
	router := NewRouter()

	// 1. Handler returning an error produces non-2xx and reaches configured error handler
	var capturedErr error
	router.SetErrorHandler(func(ctx *Context, err error) {
		capturedErr = err
		ctx.Writer.WriteHeader(netHTTP.StatusUnprocessableEntity)
		_, _ = ctx.Writer.Write([]byte("custom error handled: " + err.Error()))
	})

	router.Handle("POST", "/submit", func(ctx *Context) error {
		return errors.New("validation failed for email")
	})

	router.Get("/crash", func(ctx *Context) {
		var nilPtr *string
		_ = *nilPtr // force nil pointer panic
	})

	router.Get("/hello", func(ctx *Context) {
		ctx.Writer.Header().Set("X-Custom-Greeting", "Antigravity")
		_, _ = ctx.Writer.Write([]byte("Hello World"))
	})

	engine := NewEngine(router, nil)

	// Test 1: Error returns 422 and error reaches configured handler
	req1 := httptest.NewRequest("POST", "/submit", nil)
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)
	if w1.Code != netHTTP.StatusUnprocessableEntity {
		t.Fatalf("expected error handler to produce 422, got %d", w1.Code)
	}
	if capturedErr == nil || !strings.Contains(capturedErr.Error(), "validation failed") {
		t.Fatalf("expected error to reach handler, got %v", capturedErr)
	}

	// Test 2: Handler that panics returns 500 and process keeps serving next requests
	req2 := httptest.NewRequest("GET", "/crash", nil)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != netHTTP.StatusInternalServerError {
		t.Fatalf("expected panic to be safely recovered with 500, got %d", w2.Code)
	}

	// Follow-up request succeeds immediately after panic
	req3 := httptest.NewRequest("GET", "/hello", nil)
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, req3)
	if w3.Code != netHTTP.StatusOK || w3.Body.String() != "Hello World" {
		t.Fatalf("expected server to continue serving healthy responses after panic recovery, got %d %s", w3.Code, w3.Body.String())
	}

	// Test 3: HEAD /hello returns same headers as GET with an empty body
	reqHead := httptest.NewRequest("HEAD", "/hello", nil)
	wHead := httptest.NewRecorder()
	engine.ServeHTTP(wHead, reqHead)
	if wHead.Code != netHTTP.StatusOK {
		t.Fatalf("expected HEAD /hello to return 200, got %d", wHead.Code)
	}
	if wHead.Header().Get("X-Custom-Greeting") != "Antigravity" {
		t.Fatalf("expected custom header on HEAD request, got %q", wHead.Header().Get("X-Custom-Greeting"))
	}
	if wHead.Body.Len() != 0 {
		t.Fatalf("expected HEAD response body to be empty, got %d bytes: %q", wHead.Body.Len(), wHead.Body.String())
	}

	// Test 4: OPTIONS /hello returns Allow listing registered methods
	reqOptions := httptest.NewRequest("OPTIONS", "/hello", nil)
	wOptions := httptest.NewRecorder()
	engine.ServeHTTP(wOptions, reqOptions)
	allow := wOptions.Header().Get("Allow")
	if !strings.Contains(allow, "GET") {
		t.Fatalf("expected Allow header on OPTIONS to list GET, got: %q", allow)
	}

	// Test 5: POST to GET-only static route returns 405 with Allow header
	req405 := httptest.NewRequest("POST", "/hello", nil)
	w405 := httptest.NewRecorder()
	engine.ServeHTTP(w405, req405)
	if w405.Code != netHTTP.StatusMethodNotAllowed {
		t.Fatalf("expected POST to GET-only route to return 405, got %d", w405.Code)
	}
	if !strings.Contains(w405.Header().Get("Allow"), "GET") {
		t.Fatalf("expected Allow header on 405 to list GET, got: %q", w405.Header().Get("Allow"))
	}
}

func TestEngine_SEC09_RequestBodyLimit(t *testing.T) {
	// Set small 1KB body limit for testing
	os.Setenv("MAX_REQUEST_BODY_SIZE", "1024")
	defer os.Unsetenv("MAX_REQUEST_BODY_SIZE")

	router := NewRouter()
	router.Handle("POST", "/upload", func(ctx *Context) error {
		_, err := io.ReadAll(ctx.Request.Body)
		return err
	})
	engine := NewEngine(router, nil)

	// 1. Body exceeding 1KB returns 413 Request Entity Too Large
	bigBody := bytes.Repeat([]byte("A"), 2048)
	reqBig := httptest.NewRequest("POST", "/upload", bytes.NewReader(bigBody))
	wBig := httptest.NewRecorder()
	engine.ServeHTTP(wBig, reqBig)

	if wBig.Code != netHTTP.StatusRequestEntityTooLarge {
		t.Fatalf("expected body exceeding limit to return 413, got %d", wBig.Code)
	}

	// 2. Body under 1KB succeeds
	smallBody := []byte("Hello under limit")
	reqSmall := httptest.NewRequest("POST", "/upload", bytes.NewReader(smallBody))
	wSmall := httptest.NewRecorder()
	engine.ServeHTTP(wSmall, reqSmall)

	if wSmall.Code != netHTTP.StatusOK {
		t.Fatalf("expected small body under limit to succeed, got %d", wSmall.Code)
	}
}

func TestEngine_SEC15_StatusCaptureCapabilities(t *testing.T) {
	rec := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: rec, code: netHTTP.StatusOK}

	// Verify unwrapping
	if sc.Unwrap() != rec {
		t.Fatal("expected statusCapture.Unwrap() to return underlying ResponseWriter")
	}

	// Verify flusher pass-through
	sc.Flush()
	if !rec.Flushed {
		t.Fatal("expected Flush() to reach underlying recorder")
	}
}

func TestRouter_Static(t *testing.T) {
	tempDir := t.TempDir()

	// Create test static files in tempDir
	cssContent := []byte("body { background: #000; }")
	cssFile := tempDir + "/test.css"
	if err := os.WriteFile(cssFile, cssContent, 0644); err != nil {
		t.Fatalf("failed to write test css: %v", err)
	}

	router := NewRouter()
	router.Static("/assets", tempDir)
	engine := NewEngine(router, nil)

	// 1. Valid file request without version
	req1 := httptest.NewRequest("GET", "/assets/test.css", nil)
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)
	if w1.Code != netHTTP.StatusOK {
		t.Fatalf("expected 200 OK for /assets/test.css, got %d", w1.Code)
	}
	if !strings.Contains(w1.Body.String(), "background: #000") {
		t.Fatalf("expected css body, got %q", w1.Body.String())
	}
	if w1.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Fatalf("expected 3600 cache-control for unversioned request, got %q", w1.Header().Get("Cache-Control"))
	}

	// 2. Versioned file request with ?v=1234
	req2 := httptest.NewRequest("GET", "/assets/test.css?v=1234", nil)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != netHTTP.StatusOK {
		t.Fatalf("expected 200 OK for versioned request, got %d", w2.Code)
	}
	if w2.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable cache-control for versioned request, got %q", w2.Header().Get("Cache-Control"))
	}

	// 3. Path traversal attempt should 404
	req3 := httptest.NewRequest("GET", "/assets/../../etc/passwd", nil)
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, req3)
	if w3.Code != netHTTP.StatusNotFound {
		t.Fatalf("expected 404 for traversal attempt, got %d", w3.Code)
	}

	// 4. Directory request should 404
	req4 := httptest.NewRequest("GET", "/assets/", nil)
	w4 := httptest.NewRecorder()
	engine.ServeHTTP(w4, req4)
	if w4.Code != netHTTP.StatusNotFound {
		t.Fatalf("expected 404 for directory request, got %d", w4.Code)
	}

	// 5. Non-existent file should 404
	req5 := httptest.NewRequest("GET", "/assets/non-existent.js", nil)
	w5 := httptest.NewRecorder()
	engine.ServeHTTP(w5, req5)
	if w5.Code != netHTTP.StatusNotFound {
		t.Fatalf("expected 404 for non-existent file, got %d", w5.Code)
	}
}

func TestContext_RenderBuffersBeforeStatus(t *testing.T) {
	tempose := NewTempose()
	tempose.Register("failing-view", func(w io.Writer, data any, trans ViewTranslator) error {
		return errors.New("template syntax or evaluation failed")
	})
	tempose.Register("successful-view", func(w io.Writer, data any, trans ViewTranslator) error {
		_, err := w.Write([]byte("<h1>Success</h1>"))
		return err
	})

	// Test 1: Failing render does NOT commit 200 OK header
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("GET", "/fail", nil)
	ctx1 := &Context{
		Writer:  w1,
		Request: req1,
		Tempose: tempose,
	}

	err := ctx1.Render("failing-view", nil)
	if err == nil {
		t.Fatal("expected Render to return error on failed view, got nil")
	}
	if w1.Flushed {
		t.Errorf("expected response not to be committed when render fails")
	}
	if w1.Body.Len() > 0 {
		t.Errorf("expected empty body when render fails, got %q", w1.Body.String())
	}

	// Test 2: Successful render commits 200 OK and body
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/success", nil)
	ctx2 := &Context{
		Writer:  w2,
		Request: req2,
		Tempose: tempose,
	}

	err = ctx2.Render("successful-view", nil)
	if err != nil {
		t.Fatalf("expected successful render, got %v", err)
	}
	if w2.Code != netHTTP.StatusOK {
		t.Errorf("expected 200 OK on success, got %d", w2.Code)
	}
	if w2.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("expected text/html charset=utf-8 Content-Type, got %q", w2.Header().Get("Content-Type"))
	}
	if w2.Body.String() != "<h1>Success</h1>" {
		t.Errorf("expected '<h1>Success</h1>', got %q", w2.Body.String())
	}
}

