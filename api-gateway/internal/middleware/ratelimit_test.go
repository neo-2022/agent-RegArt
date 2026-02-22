package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)

	for i := 0; i < 3; i++ {
		if !rl.Allow("client1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	if rl.Allow("client1") {
		t.Error("4th request should be rate-limited")
	}

	if !rl.Allow("client2") {
		t.Error("different client should be allowed")
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 100*time.Millisecond)

	rl.Allow("c1")
	rl.Allow("c1")
	if rl.Allow("c1") {
		t.Error("should be rate-limited")
	}

	time.Sleep(150 * time.Millisecond)

	if !rl.Allow("c1") {
		t.Error("should be allowed after window expiry")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Second)
	mw := RateLimitMiddleware(rl)

	handler := mw(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}
