package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_PreflightOptions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedOrigins = []string{"https://myapp.com"}
	cfg.AllowCredentials = true

	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/data", nil)
	req.Header.Set("Origin", "https://myapp.com")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for preflight, got %d", rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://myapp.com" {
		t.Errorf("expected Allow-Origin https://myapp.com, got %q", got)
	}

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials true, got %q", got)
	}
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowedOrigins = []string{"https://trusted.com"}

	handler := Middleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/data", nil)
	req.Header.Set("Origin", "https://evil.com")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected empty Allow-Origin for untrusted site, got %q", got)
	}
}

