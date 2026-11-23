package http

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// F-068: Test that WebSocket Hub broadcast does not deadlock when client send channel is full.
func TestWebSocket_BroadcastFullChannelNoDeadlock(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Register a client with a buffer of 1
	c := &Connection{
		hub:  hub,
		send: make(chan []byte, 1),
	}
	hub.register <- c
	time.Sleep(20 * time.Millisecond)

	// Fill the buffer
	c.send <- []byte("first")

	// Broadcast another message — should not block or deadlock the hub
	done := make(chan struct{})
	go func() {
		hub.Broadcast([]byte("second"))
		close(done)
	}()

	select {
	case <-done:
		// Broadcast returned without deadlocking
	case <-time.After(2 * time.Second):
		t.Fatal("Hub.Broadcast deadlocked on stalled connection!")
	}
}

// F-073: Test that DBSessionMiddleware rejects unvalidated/malicious session cookies.
func TestDBSessionMiddleware_RejectsUnvalidatedCookie(t *testing.T) {
	store := NewDBSessionStore(nil, "sessions", 24*time.Hour, "secret-key-that-is-at-least-32-bytes-long!")
	mw := DBSessionMiddleware(store, "test_session")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	// Add an invalid session cookie (e.g. SQL injection attempt or directory traversal)
	req.AddCookie(&http.Cookie{
		Name:  "test_session",
		Value: "invalid_untrusted_cookie_with_no_hmac_signature",
	})

	ctx := &Context{
		Request: req,
		Writer:  rec,
	}

	err := mw(ctx, func(c *Context) error {
		sessVal := c.Get("session")
		if sessVal == nil {
			return errors.New("no session in context")
		}
		sess := sessVal.(contract.Session)
		// The session ID must NOT be the malicious cookie value!
		if sess.ID() == "invalid_untrusted_cookie_with_no_hmac_signature" {
			t.Fatal("DBSessionMiddleware accepted unvalidated session ID!")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected middleware error: %v", err)
	}

	// Verify that the cookie sent to client was rotated to a safe, valid session ID
	respCookie := rec.Result().Cookies()
	if len(respCookie) == 0 {
		t.Fatal("expected session cookie to be set in response")
	}
	if respCookie[0].Value == "invalid_untrusted_cookie_with_no_hmac_signature" {
		t.Fatal("DBSessionMiddleware echoed back the invalid session ID!")
	}
}

// badScanDB returns a mock result that fails during Scan.
type badScanDB struct {
	driver string
}

func (b *badScanDB) Connect() error                             { return nil }
func (b *badScanDB) Query(query string, args ...any) (any, error) { return &sql.Rows{}, nil }
func (b *badScanDB) Exec(query string, args ...any) error         { return nil }
func (b *badScanDB) BeginTx() (any, error)                      { return nil, nil }
func (b *badScanDB) Driver() string                             { return "postgres" }
func (b *badScanDB) Close() error                               { return nil }

// F-090: Test that IsUnique fails closed when DB is nil or query fails.
func TestIsUnique_FailsClosed(t *testing.T) {
	// Rule with nil DB must fail closed and return error message without panicking
	rule := IsUnique(nil, "users", "email")
	ok, msg := rule("email", "test@example.com")
	if ok {
		t.Fatal("expected IsUnique with nil DB to fail closed (return false), got true")
	}
	if !strings.Contains(msg, "uniqueness check unavailable") {
		t.Fatalf("expected message mentioning 'uniqueness check unavailable', got: %s", msg)
	}
}
