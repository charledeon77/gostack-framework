/*
Purpose:
Package health provides standardized Kubernetes and container liveness/readiness probes (DX-06).
It registers /healthz and /ready HTTP handlers that check the overall process and verify database/cache connectivity.
*/
package health

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/charledeon77/gostack-framework/framework/contract"
)

// Status represents the health response structure.
type Status struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// LivenessHandler returns a 200 OK as long as the process is alive.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Status{
			Status:    "up",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ReadinessHandler checks database and cache connectivity before reporting ready.
func ReadinessHandler(db contract.Database, cache contract.CacheStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		checks := make(map[string]string)
		allHealthy := true

		if db != nil {
			if err := db.Connect(); err != nil {
				checks["database"] = "down: " + err.Error()
				allHealthy = false
			} else {
				checks["database"] = "up"
			}
		}

		if cache != nil {
			// Write and read temporary test key
			testKey := "health_probe_ping"
			_ = cache.Put(testKey, "pong", 5*time.Second)
			_, err := cache.Get(testKey)
			if err != nil && err != contract.ErrCacheMiss {
				checks["cache"] = "down: " + err.Error()
				allHealthy = false
			} else {
				checks["cache"] = "up"
			}
		}

		status := "ready"
		code := http.StatusOK
		if !allHealthy {
			status = "degraded"
			code = http.StatusServiceUnavailable
		}

		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(Status{
			Status:    status,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Checks:    checks,
		})
	}
}

