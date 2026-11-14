package http

import (
	"fmt"
	"net"
	netHTTP "net/http"
	"strings"
	"sync"
	"time"
)

var (
	trustedProxiesMu sync.RWMutex
	trustedProxies   []*net.IPNet
)

// SetTrustedProxies configures trusted proxy IP addresses or CIDR subnets (SEC-10).
func SetTrustedProxies(cidrs []string) error {
	var parsed []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			ip := net.ParseIP(c)
			if ip == nil {
				return fmt.Errorf("throttle: invalid IP %q", c)
			}
			mask := net.CIDRMask(32, 32)
			if ip.To4() == nil {
				mask = net.CIDRMask(128, 128)
			}
			parsed = append(parsed, &net.IPNet{IP: ip, Mask: mask})
			continue
		}
		_, ipNet, err := net.ParseCIDR(c)
		if err != nil {
			return fmt.Errorf("throttle: invalid CIDR %q: %w", c, err)
		}
		parsed = append(parsed, ipNet)
	}
	trustedProxiesMu.Lock()
	trustedProxies = parsed
	trustedProxiesMu.Unlock()
	return nil
}

// ResetTrustedProxies clears the trusted proxy list (for testing).
func ResetTrustedProxies() {
	trustedProxiesMu.Lock()
	trustedProxies = nil
	trustedProxiesMu.Unlock()
}

func isTrustedProxy(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	trustedProxiesMu.RLock()
	defer trustedProxiesMu.RUnlock()
	for _, p := range trustedProxies {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP resolves the target client IP address.
// Forwarding headers (X-Forwarded-For, X-Real-IP) are ONLY trusted when RemoteAddr matches a trusted proxy (SEC-10).
func clientIP(r *netHTTP.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	if isTrustedProxy(r.RemoteAddr) {
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			if comma := strings.Index(ip, ","); comma != -1 {
				return strings.TrimSpace(ip[:comma])
			}
			return strings.TrimSpace(ip)
		}
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
	}

	return host
}

type clientLimit struct {
	requests []time.Time
	lastSeen time.Time
}

// RateLimiter tracks client rate limits with automatic periodic eviction (SEC-10).
type RateLimiter struct {
	mu          sync.Mutex
	limit       int
	period      time.Duration
	clients     map[string]*clientLimit
	lastCleanup time.Time
}

// NewRateLimiter instantiates an explicit rate limiter instance.
func NewRateLimiter(limit int, period time.Duration) *RateLimiter {
	return &RateLimiter{
		limit:       limit,
		period:      period,
		clients:     make(map[string]*clientLimit),
		lastCleanup: time.Now(),
	}
}

// ActiveBucketCount returns the current number of tracked client IP buckets (test hook for SEC-10).
func (rl *RateLimiter) ActiveBucketCount() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.clients)
}

// Sweep explicitly evicts client records whose requests have fully expired (test hook for SEC-10).
func (rl *RateLimiter) Sweep() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.period)
	for k, v := range rl.clients {
		if v.lastSeen.Before(cutoff) {
			delete(rl.clients, k)
		}
	}
	rl.lastCleanup = now
}

// Middleware returns the GoStack Middleware wrapping this rate limiter.
func (rl *RateLimiter) Middleware() Middleware {
	return func(ctx *Context, next NextHandler) error {
		ip := clientIP(ctx.Request)

		rl.mu.Lock()
		now := time.Now()

		// Periodic TTL eviction: clean up inactive client records every 2 * period
		if now.Sub(rl.lastCleanup) > 2*rl.period {
			cutoffCleanup := now.Add(-rl.period)
			for k, v := range rl.clients {
				if v.lastSeen.Before(cutoffCleanup) {
					delete(rl.clients, k)
				}
			}
			rl.lastCleanup = now
		}

		client, exists := rl.clients[ip]
		if !exists {
			client = &clientLimit{requests: make([]time.Time, 0), lastSeen: now}
			rl.clients[ip] = client
		}
		client.lastSeen = now

		cutoff := now.Add(-rl.period)
		var active []time.Time
		for _, t := range client.requests {
			if t.After(cutoff) {
				active = append(active, t)
			}
		}

		if len(active) >= rl.limit {
			// Calculate remaining time until oldest request exits the sliding window (SEC-10)
			remaining := active[0].Add(rl.period).Sub(now)
			retrySeconds := int(remaining.Seconds()) + 1
			if retrySeconds <= 0 {
				retrySeconds = 1
			}
			rl.mu.Unlock()
			ctx.Writer.Header().Set("Retry-After", fmt.Sprintf("%d", retrySeconds))
			ctx.Writer.WriteHeader(netHTTP.StatusTooManyRequests)
			_, _ = ctx.Writer.Write([]byte("429 Too Many Requests: Rate limit exceeded"))
			return nil
		}

		client.requests = append(active, now)
		rl.mu.Unlock()

		return next(ctx)
	}
}

// Throttle provisions a middleware layer implementing a thread-safe sliding window rate-limiting algorithm
// equipped with automatic periodic eviction to prevent memory accumulation over time.
func Throttle(limit int, period time.Duration) Middleware {
	return NewRateLimiter(limit, period).Middleware()
}
