package http

import (
	"fmt"
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

// TestThrottle_SEC10_AcceptCriteria tests all 4 accept criteria from SEC-10.
func TestThrottle_SEC10_AcceptCriteria(t *testing.T) {
	ResetTrustedProxies()
	defer ResetTrustedProxies()

	limiter := NewRateLimiter(5, 2*time.Second)
	mw := limiter.Middleware()
	next := func(ctx *Context) error {
		ctx.Writer.WriteHeader(http.StatusOK)
		return nil
	}

	// 1. Default config ignores X-Forwarded-For; 6 requests from 1 RemoteAddr with 6 different spoofed headers -> 6th is limited
	var lastStatus int
	var retryAfter string
	for i := 1; i <= 6; i++ {
		req := httptest.NewRequest("GET", "/api", nil)
		req.RemoteAddr = "198.51.100.5:1234"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.0.%d", i))
		w := httptest.NewRecorder()
		_ = mw(&Context{Writer: w, Request: req}, next)
		lastStatus = w.Code
		if i == 6 {
			retryAfter = w.Header().Get("Retry-After")
		}
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Fatalf("expected 6th spoofed request to be blocked (429), got: %d", lastStatus)
	}
	if retryAfter == "" {
		t.Fatal("expected Retry-After header on 429 response")
	}

	// 2. With RemoteAddr in trusted list, the header IS honoured
	_ = SetTrustedProxies([]string{"198.51.100.0/24"})
	trustedLimiter := NewRateLimiter(2, 2*time.Second)
	mwTrusted := trustedLimiter.Middleware()

	// 2 requests with client IP 203.0.113.199 through trusted proxy
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api", nil)
		req.RemoteAddr = "198.51.100.5:1234"
		req.Header.Set("X-Forwarded-For", "203.0.113.199")
		w := httptest.NewRecorder()
		_ = mwTrusted(&Context{Writer: w, Request: req}, next)
		if w.Code != http.StatusOK {
			t.Fatalf("expected request %d to be allowed, got: %d", i+1, w.Code)
		}
	}
	// 3rd request with same forwarded IP through trusted proxy gets blocked
	reqBlocked := httptest.NewRequest("GET", "/api", nil)
	reqBlocked.RemoteAddr = "198.51.100.5:1234"
	reqBlocked.Header.Set("X-Forwarded-For", "203.0.113.199")
	wBlocked := httptest.NewRecorder()
	_ = mwTrusted(&Context{Writer: wBlocked, Request: reqBlocked}, next)
	if wBlocked.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 3rd request from same forwarded IP to be blocked, got: %d", wBlocked.Code)
	}

	// 3. Different client IP through trusted proxy is allowed
	reqDifferent := httptest.NewRequest("GET", "/api", nil)
	reqDifferent.RemoteAddr = "198.51.100.5:1234"
	reqDifferent.Header.Set("X-Forwarded-For", "203.0.113.200")
	wDifferent := httptest.NewRecorder()
	_ = mwTrusted(&Context{Writer: wDifferent, Request: reqDifferent}, next)
	if wDifferent.Code != http.StatusOK {
		t.Fatalf("expected different forwarded IP to be allowed, got: %d", wDifferent.Code)
	}

	// 4. After window of inactivity, bucket count returns to zero
	quickLimiter := NewRateLimiter(5, 50*time.Millisecond)
	mwQuick := quickLimiter.Middleware()
	reqQ := httptest.NewRequest("GET", "/api", nil)
	reqQ.RemoteAddr = "127.0.0.1:5555"
	wQ := httptest.NewRecorder()
	_ = mwQuick(&Context{Writer: wQ, Request: reqQ}, next)

	if quickLimiter.ActiveBucketCount() != 1 {
		t.Fatalf("expected 1 active bucket, got %d", quickLimiter.ActiveBucketCount())
	}

	time.Sleep(100 * time.Millisecond)
	quickLimiter.Sweep()

	if quickLimiter.ActiveBucketCount() != 0 {
		t.Fatalf("expected 0 active buckets after sweep, got %d", quickLimiter.ActiveBucketCount())
	}
}
