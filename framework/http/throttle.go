package http

import (
	"fmt"
	"net"
	netHTTP "net/http"
	"strings"
	"sync"
	"time"
)

// clientIP resolves the target client IP address checking routing proxies before fallback.
func clientIP(r *netHTTP.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if comma := strings.Index(ip, ","); comma != -1 {
			return strings.TrimSpace(ip[:comma])
		}
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Throttle provisions a middleware layer implementing a thread-safe sliding window rate-limiting algorithm.
func Throttle(limit int, period time.Duration) Middleware {
	var mu sync.Mutex
	type clientLimit struct {
		requests []time.Time
	}
	clients := make(map[string]*clientLimit)

	return func(ctx *Context, next NextHandler) error {
		ip := clientIP(ctx.Request)

		mu.Lock()
		client, exists := clients[ip]
		if !exists {
			client = &clientLimit{requests: make([]time.Time, 0)}
			clients[ip] = client
		}

		now := time.Now()
		cutoff := now.Add(-period)

		var active []time.Time
		for _, t := range client.requests {
			if t.After(cutoff) {
				active = append(active, t)
			}
		}

		if len(active) >= limit {
			mu.Unlock()
			ctx.Writer.Header().Set("Retry-After", fmt.Sprintf("%d", int(period.Seconds())))
			ctx.Writer.WriteHeader(netHTTP.StatusTooManyRequests)
			_, _ = ctx.Writer.Write([]byte("429 Too Many Requests: Rate limit exceeded"))
			return nil
		}

		client.requests = append(active, now)
		mu.Unlock()

		return next(ctx)
	}
}
