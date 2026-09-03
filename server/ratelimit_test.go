package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRateLimiterBlocksExcess(t *testing.T) {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		window:  60e9, // 1m
		limit:   3,
		strict:  1,
		enabled: true,
	}

	for i := 0; i < 3; i++ {
		if _, allowed := rl.allow("ip:1.2.3.4", false); !allowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if _, allowed := rl.allow("ip:1.2.3.4", false); allowed {
		t.Error("4th request should be blocked under the normal cap")
	}

	// A different IP has its own budget.
	for i := 0; i < 3; i++ {
		if _, allowed := rl.allow("ip:5.6.7.8", false); !allowed {
			t.Fatalf("other-ip request %d should be allowed", i)
		}
	}
	// Strict endpoints are capped lower regardless of the normal limit.
	if _, allowed := rl.allow("ip:5.6.7.8", true); allowed {
		t.Error("strict request should be blocked under the strict cap")
	}
}

func TestRateLimiterDisabledAllowsAll(t *testing.T) {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		window:  60e9,
		limit:   1,
		strict:  1,
		enabled: false,
	}
	for i := 0; i < 100; i++ {
		if _, allowed := rl.allow("ip:9.9.9.9", false); !allowed {
			t.Fatalf("request %d blocked when limiter disabled", i)
		}
	}
}

func TestRateLimitMiddlewareReturns429(t *testing.T) {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		window:  60e9,
		limit:   1,
		strict:  1,
		enabled: true,
	}
	var hits int
	h := rl.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest("GET", "/api/me", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest("GET", "/api/me", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Errorf("second status = %d, want 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429")
	}
	if hits != 1 {
		t.Errorf("handler invoked %d times, want 1", hits)
	}
}
