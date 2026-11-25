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

func TestRouter_UseGlobalMiddleware(t *testing.T) {
	router := NewRouter()
	var order []string

	// Register global middleware via router.Use
	router.Use(func(ctx *Context, next NextHandler) error {
		order = append(order, "global-1")
		return next(ctx)
	})
	router.Use(func(ctx *Context, next NextHandler) error {
		order = append(order, "global-2")
		return next(ctx)
	})

	// Route-specific middleware
	routeMiddleware := func(ctx *Context, next NextHandler) error {
		order = append(order, "route-specific")
		return next(ctx)
	}

	router.Get("/test", func(ctx *Context) {
		order = append(order, "handler")
		ctx.Writer.Write([]byte("ok"))
	}, routeMiddleware)

	engine := NewEngine(router, nil)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != netHTTP.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	expectedOrder := []string{"global-1", "global-2", "route-specific", "handler"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("expected %d execution steps, got %d (%v)", len(expectedOrder), len(order), order)
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("step %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestEngine_D46_BodyGuardLayerAnd413(t *testing.T) {
	os.Setenv("MAX_REQUEST_BODY_SIZE", "256")
	defer os.Unsetenv("MAX_REQUEST_BODY_SIZE")

	router := NewRouter()
	router.Post("/submit", func(ctx *Context) {
		ctx.Writer.WriteHeader(netHTTP.StatusOK)
	})
	engine := NewEngine(router, nil)

	// 1. Oversized multipart POST returns 413 (not 400) before reading body
	bigMultipart := "--bound\r\nContent-Disposition: form-data; name=\"file\"; filename=\"a.txt\"\r\n\r\n" +
		strings.Repeat("X", 512) + "\r\n--bound--\r\n"
	req1 := httptest.NewRequest("POST", "/submit", strings.NewReader(bigMultipart))
	req1.Header.Set("Content-Type", "multipart/form-data; boundary=bound")
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)
	if w1.Code != netHTTP.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized multipart POST to return 413, got %d", w1.Code)
	}

	// 2. Oversized urlencoded POST returns 413 (not 400) even during method-override check
	bigForm := "k=" + strings.Repeat("a", 512)
	req2 := httptest.NewRequest("POST", "/submit", strings.NewReader(bigForm))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != netHTTP.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized urlencoded POST to return 413, got %d", w2.Code)
	}

	// 3. Malformed multipart Content-Type header (missing boundary) returns 400
	req3 := httptest.NewRequest("POST", "/submit", strings.NewReader("body"))
	req3.Header.Set("Content-Type", "multipart/form-data")
	w3 := httptest.NewRecorder()
	engine.ServeHTTP(w3, req3)
	if w3.Code != netHTTP.StatusBadRequest {
		t.Fatalf("expected malformed multipart Content-Type header to return 400, got %d", w3.Code)
	}

	// 4. PUT / PATCH / DELETE urlencoded bodies are validated (C26 / D46): malformed -> 400, oversized -> 413
	for _, method := range []string{"PUT", "PATCH", "DELETE"} {
		router.Handle(method, "/item", func(ctx *Context) error {
			ctx.Writer.WriteHeader(netHTTP.StatusOK)
			return nil
		})

		reqBad := httptest.NewRequest(method, "/item", strings.NewReader("bad=%ZZ"))
		reqBad.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		wBad := httptest.NewRecorder()
		engine.ServeHTTP(wBad, reqBad)
		if wBad.Code != netHTTP.StatusBadRequest {
			t.Fatalf("expected malformed urlencoded %s to return 400, got %d", method, wBad.Code)
		}

		reqBig := httptest.NewRequest(method, "/item", strings.NewReader("k="+strings.Repeat("a", 512)))
		reqBig.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		wBig := httptest.NewRecorder()
		engine.ServeHTTP(wBig, reqBig)
		if wBig.Code != netHTTP.StatusRequestEntityTooLarge {
			t.Fatalf("expected oversized urlencoded %s to return 413, got %d", method, wBig.Code)
		}
	}

	// 5. Multipart POST with _method=DELETE routes to DELETE handler and preserves body stream
	os.Setenv("MAX_REQUEST_BODY_SIZE", "4096")
	var gotField string
	router.Delete("/upload-item", func(ctx *Context) {
		val, err := ctx.PostErr("title")
		if err != nil {
			t.Errorf("PostErr failed: %v", err)
		}
		gotField = val
		ctx.Writer.WriteHeader(netHTTP.StatusOK)
	})
	mpBody := "--b1\r\nContent-Disposition: form-data; name=\"_method\"\r\n\r\nDELETE\r\n" +
		"--b1\r\nContent-Disposition: form-data; name=\"title\"\r\n\r\nhello\r\n--b1--\r\n"
	reqMP := httptest.NewRequest("POST", "/upload-item", strings.NewReader(mpBody))
	reqMP.Header.Set("Content-Type", "multipart/form-data; boundary=b1")
	wMP := httptest.NewRecorder()
	engine.ServeHTTP(wMP, reqMP)
	if wMP.Code != netHTTP.StatusOK || gotField != "hello" {
		t.Fatalf("expected multipart _method=DELETE to run DELETE handler and read title='hello', got code=%d title=%q", wMP.Code, gotField)
	}

	// 6. Context.PostErr surfaces *MaxBytesError on chunked/unknown-length oversized multipart body
	os.Setenv("MAX_REQUEST_BODY_SIZE", "128")
	router.Handle("POST", "/posterr", func(ctx *Context) error {
		_, err := ctx.PostErr("title")
		return err
	})
	bigMP2 := "--b2\r\nContent-Disposition: form-data; name=\"title\"\r\n\r\n" + strings.Repeat("Z", 512) + "\r\n--b2--\r\n"
	reqErr := httptest.NewRequest("POST", "/posterr", strings.NewReader(bigMP2))
	reqErr.ContentLength = -1 // simulate chunked so MaxBytesReader triggers inside PostErr
	reqErr.Header.Set("Content-Type", "multipart/form-data; boundary=b2")
	wErr := httptest.NewRecorder()
	engine.ServeHTTP(wErr, reqErr)
	if wErr.Code != netHTTP.StatusRequestEntityTooLarge {
		t.Fatalf("expected PostErr on oversized chunked multipart to return 413, got %d", wErr.Code)
	}
}

func TestEngine_D44_SecurityHeadersMiddlewareAndUnsafeInline(t *testing.T) {
	// 1. SecurityHeaders() middleware preserves the Engine's per-request CSP nonce
	router := NewRouter()
	router.Use(SecurityHeaders())
	var handlerNonce string
	router.Get("/styled", func(ctx *Context) {
		handlerNonce = ctx.CSPNonce()
		_, _ = ctx.Writer.Write([]byte(`<style nonce="` + handlerNonce + `">body{}</style>`))
	})
	engine := NewEngine(router, nil)

	req := httptest.NewRequest("GET", "/styled", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	csp := w.Header().Get("Content-Security-Policy")
	if handlerNonce == "" || !strings.Contains(csp, "'nonce-"+handlerNonce+"'") {
		t.Fatalf("expected SecurityHeaders() middleware to preserve Engine nonce %q in CSP, got %q", handlerNonce, csp)
	}

	// 2. Policy with style-src 'unsafe-inline' is NOT broken by nonce injection
	router2 := NewRouter()
	router2.Get("/legacy", func(ctx *Context) {
		ctx.Writer.WriteHeader(netHTTP.StatusOK)
	})
	engine2 := NewEngine(router2, nil)
	engine2.SecurityHeadersConfig.ContentSecurityPolicy = "default-src 'self'; style-src 'self' 'unsafe-inline';"

	req2 := httptest.NewRequest("GET", "/legacy", nil)
	w2 := httptest.NewRecorder()
	engine2.ServeHTTP(w2, req2)

	csp2 := w2.Header().Get("Content-Security-Policy")
	if strings.Contains(csp2, "'nonce-") {
		t.Fatalf("expected 'unsafe-inline' style-src policy to NOT have nonce injected, got %q", csp2)
	}
}

func TestEngine_D44_CSPNoncePipelineAndPanicPage(t *testing.T) {
	router := NewRouter()
	var capturedNonce string
	router.Get("/ok", func(ctx *Context) {
		capturedNonce = ctx.CSPNonce()
		ctx.Writer.WriteHeader(netHTTP.StatusOK)
	})
	router.Get("/boom", func(ctx *Context) {
		panic("kaboom")
	})
	engine := NewEngine(router, nil)

	// 1. Normal request exposes CSPNonce() and injects it into Content-Security-Policy
	req1 := httptest.NewRequest("GET", "/ok", nil)
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)
	if capturedNonce == "" {
		t.Fatal("expected non-empty ctx.CSPNonce()")
	}
	csp1 := w1.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp1, "'nonce-"+capturedNonce+"'") {
		t.Fatalf("expected CSP header to contain 'nonce-%s', got %q", capturedNonce, csp1)
	}

	// 2. Panic 500 page <style nonce="..."> matches Content-Security-Policy nonce
	req2 := httptest.NewRequest("GET", "/boom", nil)
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != netHTTP.StatusInternalServerError {
		t.Fatalf("expected 500 on panic, got %d", w2.Code)
	}
	csp2 := w2.Header().Get("Content-Security-Policy")
	body2 := w2.Body.String()
	if !strings.Contains(body2, `<style nonce="`) {
		t.Fatalf("expected 500 panic page to carry <style nonce=\"...\">, got %s", body2)
	}
	if !strings.Contains(csp2, "'nonce-") {
		t.Fatalf("expected 500 panic response CSP to carry nonce, got %q", csp2)
	}
}


