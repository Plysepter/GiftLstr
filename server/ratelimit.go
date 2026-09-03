package main

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// rateLimiter is a fixed-window counter keyed by client IP.
//
// Configuration (all via the environment):
//
//   - GIFT_RATE_LIMIT_ENABLED: "1" enables rate limiting (default disabled).
//   - GIFT_RATE_LIMIT: cap on requests per IP per window (default 120).
//   - GIFT_RATE_LIMIT_STRICT: cap applied to the most abuse-prone endpoints
//     (metadata fetch, share-code accept) (default 30).
//   - GIFT_RATE_LIMIT_WINDOW: cap reset window (default "1m").
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket

	window  time.Duration
	limit   int
	strict  int
	enabled bool
}

type bucket struct {
	count    int
	windowAt time.Time
}

func newRateLimiter() *rateLimiter {
	window := time.Minute
	if w := os.Getenv("GIFT_RATE_LIMIT_WINDOW"); w != "" {
		if d, err := time.ParseDuration(w); err == nil && d > 0 {
			window = d
		}
	}
	return &rateLimiter{
		buckets: make(map[string]*bucket),
		window:  window,
		limit:   envInt("GIFT_RATE_LIMIT", 120),
		strict:  envInt("GIFT_RATE_LIMIT_STRICT", 30),
		enabled: os.Getenv("GIFT_RATE_LIMIT_ENABLED") == "1",
	}
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = xff[:i]
		}
		if ip := net.ParseIP(strings.TrimSpace(xff)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// allow records a request for an IP and reports whether it is permitted, plus
// how many requests remain in the window. If strict is set the lower cap
// applies. With rate limiting disabled every request is allowed.
func (rl *rateLimiter) allow(key string, strict bool) (remaining int, allowed bool) {
	if !rl.enabled {
		return -1, true
	}
	limit := rl.limit
	if strict {
		limit = rl.strict
	}
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok || now.Sub(b.windowAt) >= rl.window {
		b = &bucket{windowAt: now}
		rl.buckets[key] = b
	}
	if b.count >= limit {
		return 0, false
	}
	b.count++
	return limit - b.count, true
}

// rateLimit wraps a handler, rejecting requests that exceed the cap with 429
// and a Retry-After header.
func (rl *rateLimiter) rateLimit(next http.HandlerFunc) http.HandlerFunc {
	return rl.wrap(false, next)
}

// rateLimitStrict wraps a handler with the lower strict cap, intended for
// abuse-prone endpoints.
func (rl *rateLimiter) rateLimitStrict(next http.HandlerFunc) http.HandlerFunc {
	return rl.wrap(true, next)
}

func (rl *rateLimiter) wrap(strict bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := "ip:" + clientIP(r)
		remaining, allowed := rl.allow(key, strict)
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		if rl.enabled && remaining >= 0 {
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		}
		next(w, r)
	}
}
