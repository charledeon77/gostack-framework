package http

import (
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersDefault(t *testing.T) {
	mw := SecurityHeaders()

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	ctx := &Context{
		Writer:  w,
		Request: req,
	}

	executed := false
	err := mw(ctx, func(c *Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Fatalf("next handler was not executed")
	}

	headers := w.Header()
	if got := headers.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := headers.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("expected X-Content-Type-Options nosniff, got %q", got)
	}
	if got := headers.Get("Strict-Transport-Security"); got == "" {
		t.Errorf("expected Strict-Transport-Security to be present")
	}
	if got := headers.Get("Content-Security-Policy"); got == "" {
		t.Errorf("expected Content-Security-Policy to be present")
	}
}
