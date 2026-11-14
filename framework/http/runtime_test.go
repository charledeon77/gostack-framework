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

