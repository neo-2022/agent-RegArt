package middleware

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter — ограничитель частоты запросов (Rate Limiter).
//
// Использует алгоритм скользящего окна: для каждого клиента (по IP-адресу)
// хранит временные метки запросов и ограничивает количество запросов
// в пределах заданного окна (window).
type RateLimiter struct {
	mu       sync.Mutex             // Мьютекс для потокобезопасного доступа
	requests map[string][]time.Time // Временные метки запросов по ключу (IP-адрес)
	limit    int                    // Максимальное количество запросов в окне
	window   time.Duration          // Размер скользящего окна
}

// NewRateLimiter — создаёт новый Rate Limiter.
// limit — максимум запросов в окне, window — размер окна (например, 1 минута).
// Запускает фоновую горутину для периодической очистки устаревших записей.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
	go rl.cleanup()
	return rl
}

// Allow — проверяет, можно ли пропустить запрос от указанного клиента (key).
// Возвращает true, если лимит не превышен, и регистрирует новый запрос.
// Возвращает false, если клиент превысил лимит в текущем окне.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Отфильтровать запросы, попадающие в текущее окно
	times := rl.requests[key]
	var valid []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Проверить, не превышен ли лимит
	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

// cleanup — фоновая горутина для периодической очистки устаревших записей.
// Удаляет клиентов, у которых нет запросов в текущем окне,
// и обновляет списки запросов для остальных.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-rl.window)
		for key, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if t.After(cutoff) {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware — HTTP-мидлварь для ограничения частоты запросов.
//
// Определяет клиента по IP-адресу (или заголовку X-Forwarded-For для прокси).
// Если лимит превышен — возвращает 429 Too Many Requests.
func RateLimitMiddleware(limiter *RateLimiter) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				key = forwarded
			}
			if !limiter.Allow(key) {
				http.Error(w, `{"error":"превышен лимит запросов"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		}
	}
}
