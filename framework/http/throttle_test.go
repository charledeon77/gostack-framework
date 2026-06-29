package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestThrottle_AllowsUnderLimit(t *testing.T) {
	middleware := Throttle(3, 500*time.Millisecond)

	nextCalled := 0
	next := func(ctx *Context) error {
		nextCalled++
		ctx.Writer.WriteHeader(http.StatusOK)
		return nil
	}

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		ctx := &Context{Writer: w, Request: req}

		err := middleware(ctx, next)
		if err != nil {
			t.Fatalf("Unexpected error from middleware: %v", err)
		}

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %v on request %d", w.Code, i+1)
		}
	}

	if nextCalled != 3 {
		t.Errorf("Expected next handler to be called 3 times, was %d", nextCalled)
	}
}

func TestThrottle_BlocksOverLimit(t *testing.T) {
	middleware := Throttle(2, 1*time.Second)

	next := func(ctx *Context) error {
		ctx.Writer.WriteHeader(http.StatusOK)
		return nil
	}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		ctx := &Context{Writer: w, Request: req}

		_ = middleware(ctx, next)
		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK on initial requests, got %v", w.Code)
		}
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	ctx := &Context{Writer: w, Request: req}

	_ = middleware(ctx, next)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429 (Too Many Requests), got %v", w.Code)
	}

	reqDiff := httptest.NewRequest("GET", "/test", nil)
	reqDiff.RemoteAddr = "192.168.1.1:12345"
	wDiff := httptest.NewRecorder()
	ctxDiff := &Context{Writer: wDiff, Request: reqDiff}

	_ = middleware(ctxDiff, next)
	if wDiff.Code != http.StatusOK {
		t.Errorf("Expected status OK for different IP, got %v", wDiff.Code)
	}
}
