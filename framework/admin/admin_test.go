package admin

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestAdminMount_ProductionWithoutGuardFails(t *testing.T) {
	os.Setenv("APP_ENV", "production")
	defer os.Unsetenv("APP_ENV")

	oldGuard := AuthGuard
	AuthGuard = nil
	defer func() { AuthGuard = oldGuard }()

	mux := http.NewServeMux()
	err := Mount(mux, nil, "/admin")
	if err == nil {
		t.Fatal("expected Mount in production without AuthGuard to return error, got nil")
	}
}

func TestAdminMount_LocalWithoutGuardSucceeds(t *testing.T) {
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	oldGuard := AuthGuard
	AuthGuard = nil
	defer func() { AuthGuard = oldGuard }()

	mux := http.NewServeMux()
	err := Mount(mux, nil, "/admin")
	if err != nil {
		t.Fatalf("expected Mount in local to succeed, got %v", err)
	}

	// 1. Request from 127.0.0.1 succeeds
	req := httptest.NewRequest("GET", "/admin/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		t.Fatalf("expected 127.0.0.1 to be allowed, got 403")
	}

	// 2. Request from remote with spoofed X-Forwarded-For is rejected with 403
	reqRemote := httptest.NewRequest("GET", "/admin/", nil)
	reqRemote.RemoteAddr = "203.0.113.7:1234"
	reqRemote.Header.Set("X-Forwarded-For", "127.0.0.1")
	wRemote := httptest.NewRecorder()
	mux.ServeHTTP(wRemote, reqRemote)
	if wRemote.Code != http.StatusForbidden {
		t.Fatalf("expected remote IP with spoofed header to receive 403, got %d", wRemote.Code)
	}
}

func TestAdmin_DestructionRequiresPOST(t *testing.T) {
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	mux := http.NewServeMux()
	_ = Mount(mux, nil, "/admin")

	req := httptest.NewRequest("GET", "/admin/users/1/delete", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET on delete endpoint to return 405, got %d", w.Code)
	}
	if w.Header().Get("Allow") != "POST" {
		t.Fatalf("expected Allow: POST header, got %q", w.Header().Get("Allow"))
	}
}

func TestAdmin_SequenceRequiresPOST(t *testing.T) {
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	mux := http.NewServeMux()
	_ = Mount(mux, nil, "/admin")

	reqRetry := httptest.NewRequest("GET", "/admin/sequence/retry?id=123", nil)
	reqRetry.RemoteAddr = "127.0.0.1:1234"
	wRetry := httptest.NewRecorder()
	mux.ServeHTTP(wRetry, reqRetry)

	if wRetry.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET on sequence retry to return 405, got %d", wRetry.Code)
	}

	reqDelete := httptest.NewRequest("GET", "/admin/sequence/delete?id=123", nil)
	reqDelete.RemoteAddr = "127.0.0.1:1234"
	wDelete := httptest.NewRecorder()
	mux.ServeHTTP(wDelete, reqDelete)

	if wDelete.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected GET on sequence delete to return 405, got %d", wDelete.Code)
	}
}

