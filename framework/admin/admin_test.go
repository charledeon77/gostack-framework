package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
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

func TestAdmin_UnsetAppEnvRequiresGuard(t *testing.T) {
	os.Unsetenv("APP_ENV")
	defer os.Unsetenv("APP_ENV")

	oldGuard := AuthGuard
	AuthGuard = nil
	defer func() { AuthGuard = oldGuard }()

	mux := http.NewServeMux()
	err := Mount(mux, nil, "/admin")
	if err == nil {
		t.Fatal("expected Mount with unset APP_ENV and nil AuthGuard to fail closed, got nil")
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

type AdminTestUser struct {
	ID           int    `db:"id"`
	Email        string `db:"email"`
	PasswordHash string `db:"password_hash"`
}

func (u *AdminTestUser) TableName() string {
	return "admin_test_users"
}

func TestAdmin_EditForm_PreservesBlankSensitiveColumns(t *testing.T) {
	os.Setenv("APP_ENV", "local")
	defer os.Unsetenv("APP_ENV")

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE admin_test_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL,
		password_hash TEXT NOT NULL
	);`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	_, err = db.Exec(`INSERT INTO admin_test_users (id, email, password_hash) VALUES (1, 'initial@example.com', '$2a$12$originalhash');`)
	if err != nil {
		t.Fatalf("failed to insert initial record: %v", err)
	}

	Register(&AdminTestUser{})

	mux := http.NewServeMux()
	if err := Mount(mux, db, "/admin"); err != nil {
		t.Fatalf("Mount failed: %v", err)
	}

	// Submit POST to edit endpoint with updated email and empty password_hash
	csrfToken := "valid-test-csrf-token"
	formData := url.Values{}
	formData.Set("_csrf", csrfToken)
	formData.Set("email", "updated@example.com")
	formData.Set("password_hash", "") // Sensitive column left empty in UI

	req := httptest.NewRequest("POST", "/admin/admintestuser/1", strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: adminCSRFCookieName, Value: csrfToken})
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected redirect (302) on successful edit, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify database record: email was updated, but password_hash remains intact!
	var email, passwordHash string
	err = db.QueryRow("SELECT email, password_hash FROM admin_test_users WHERE id = 1").Scan(&email, &passwordHash)
	if err != nil {
		t.Fatalf("failed to query updated record: %v", err)
	}

	if email != "updated@example.com" {
		t.Errorf("expected email to be 'updated@example.com', got %q", email)
	}
	if passwordHash != "$2a$12$originalhash" {
		t.Errorf("F-321 regression: expected password_hash to be preserved as '$2a$12$originalhash', but got %q", passwordHash)
	}
}

