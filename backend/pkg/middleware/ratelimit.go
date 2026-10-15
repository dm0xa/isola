// Package middleware provides HTTP middleware shared by the isola server
// binaries.
package middleware

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimiter is a fixed-window, per-client-IP request limiter. Without it the
// unauthenticated API (session creation spins up containers) is trivially
// abusable.
type RateLimiter struct {
	mu       sync.Mutex
	windows  map[string]*rateWindow
	limit    int
	interval time.Duration
}

type rateWindow struct {
	count int
	start time.Time
}

// NewRateLimiter returns a limiter allowing at most limit requests per interval
// from a single client IP.
func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		windows:  make(map[string]*rateWindow),
		limit:    limit,
		interval: interval,
	}
	go rl.reap()
	return rl
}

// Middleware rejects requests that exceed the configured rate with 429.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.interval.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MiddlewareMutating applies the limiter only to state-changing requests
// (POST/PUT/PATCH/DELETE). Read-only polling is exempt: the viewer polls the
// session endpoint every 200 ms while a container is provisioning (≈300
// req/min from one browser), which would otherwise trip a 120 req/min limit.
// The expensive, abusable operations are the mutating ones — session creation
// launches a container — so those stay throttled.
func (rl *RateLimiter) MiddlewareMutating(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !rl.allow(clientIP(r)) {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.interval.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	w, ok := rl.windows[key]
	if !ok || now.Sub(w.start) >= rl.interval {
		rl.windows[key] = &rateWindow{count: 1, start: now}
		return true
	}
	if w.count >= rl.limit {
		return false
	}
	w.count++
	return true
}

func (rl *RateLimiter) reap() {
	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		rl.mu.Lock()
		for key, w := range rl.windows {
			if now.Sub(w.start) >= rl.interval {
				delete(rl.windows, key)
			}
		}
		rl.mu.Unlock()
	}
}

// clientIP extracts the client IP from RemoteAddr. When the server sits behind
// a reverse proxy, chi's RealIP middleware has already rewritten RemoteAddr
// from the (proxy-controlled) forwarding headers.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
