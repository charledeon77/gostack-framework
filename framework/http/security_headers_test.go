package http

import (
	"crypto/tls"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSecurityHeaders_SEC12_AcceptCriteria(t *testing.T) {
	// 1. Bare Engine returns security headers by default (without manual middleware)
	router := NewRouter()
	router.Get("/ping", func(ctx *Context) {
		_, _ = ctx.Writer.Write([]byte("pong"))
	})
	engine := NewEngine(router, nil)

	req := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	headers := w.Header()
	if headers.Get("X-Frame-Options") != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY by default, got %q", headers.Get("X-Frame-Options"))
	}
	if headers.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("expected X-Content-Type-Options nosniff by default, got %q", headers.Get("X-Content-Type-Options"))
	}
	if headers.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Fatalf("expected Referrer-Policy by default, got %q", headers.Get("Referrer-Policy"))
	}

	// 2. Default CSP contains neither 'unsafe-inline' nor a bare https:
	csp := headers.Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("expected Content-Security-Policy header by default")
	}
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("expected CSP to NOT contain 'unsafe-inline', got %q", csp)
	}
	if strings.Contains(csp, " https:") || strings.HasSuffix(csp, " https:") {
		t.Fatalf("expected CSP to NOT contain bare 'https:', got %q", csp)
	}

	// 3. Plain-HTTP response in local carries NO Strict-Transport-Security
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	reqHTTP := httptest.NewRequest("GET", "/ping", nil)
	wHTTP := httptest.NewRecorder()
	engine.ServeHTTP(wHTTP, reqHTTP)
	if hsts := wHTTP.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Fatalf("expected plain HTTP in local to omit HSTS, got %q", hsts)
	}

	// HTTPS response DOES carry Strict-Transport-Security with max-age >= 31536000
	reqHTTPS := httptest.NewRequest("GET", "/ping", nil)
	reqHTTPS.TLS = &tls.ConnectionState{}
	wHTTPS := httptest.NewRecorder()
	engine.ServeHTTP(wHTTPS, reqHTTPS)
	hstsHTTPS := wHTTPS.Header().Get("Strict-Transport-Security")
	if hstsHTTPS == "" || !strings.Contains(hstsHTTPS, "max-age=31536000") {
		t.Fatalf("expected HSTS with max-age >= 31536000 on HTTPS, got %q", hstsHTTPS)
	}

	// 4. X-XSS-Protection is absent
	if xxss := headers.Get("X-XSS-Protection"); xxss != "" {
		t.Fatalf("expected X-XSS-Protection to be absent (deprecated/harmful), got %q", xxss)
	}
}
