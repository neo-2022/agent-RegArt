package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestRateLimiter_Allow — проверяет базовую работу Rate Limiter.
// Ожидаемое поведение: первые N запросов проходят, (N+1)-й — отклоняется.
// Разные клиенты имеют независимые счётчики.
func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, time.Second)

	for i := 0; i < 3; i++ {
		if !rl.Allow("client1") {
			t.Errorf("запрос %d должен быть разрешён", i+1)
		}
	}

	if rl.Allow("client1") {
		t.Error("4-й запрос должен быть отклонён (превышен лимит)")
	}

	if !rl.Allow("client2") {
		t.Error("запрос от другого клиента должен быть разрешён")
	}
}

// TestRateLimiter_WindowExpiry — проверяет сброс лимита после истечения окна.
// Ожидаемое поведение: после истечения окна клиент снова может отправлять запросы.
func TestRateLimiter_WindowExpiry(t *testing.T) {
	rl := NewRateLimiter(2, 100*time.Millisecond)

	rl.Allow("c1")
	rl.Allow("c1")
	if rl.Allow("c1") {
		t.Error("запрос должен быть отклонён (лимит превышен)")
	}

	time.Sleep(150 * time.Millisecond)

	if !rl.Allow("c1") {
		t.Error("после истечения окна запрос должен быть разрешён")
	}
}

// TestRateLimitMiddleware — проверяет HTTP-мидлварь Rate Limiter.
// Ожидаемое поведение: первые 2 запроса — 200 OK, 3-й — 429 Too Many Requests.
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
			t.Errorf("запрос %d: ожидался код 200, получен %d", i+1, w.Code)
		}
	}

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("ожидался код 429, получен %d", w.Code)
	}
}
