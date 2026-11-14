package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/charledeon77/gostack-framework/framework/cache"
)

func TestHealth_Liveness(t *testing.T) {
	handler := LivenessHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status Status
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("json decode failed: %v", err)
	}
	if status.Status != "up" {
		t.Errorf("expected 'up', got %s", status.Status)
	}
}

func TestHealth_ReadinessWithMemoryCache(t *testing.T) {
	memStore := cache.NewMemoryStore()
	handler := ReadinessHandler(nil, memStore)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 ready, got %d", rec.Code)
	}

	var status Status
	_ = json.NewDecoder(rec.Body).Decode(&status)
	if status.Status != "ready" || status.Checks["cache"] != "up" {
		t.Errorf("expected ready with cache up, got: %+v", status)
	}
}

