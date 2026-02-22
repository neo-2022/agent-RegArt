package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCircuitBreaker_ClosedState — проверяет начальное состояние Circuit Breaker.
// Ожидаемое поведение: при создании состояние — Closed (замкнут),
// после успешного запроса состояние остаётся Closed.
func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	if cb.State() != StateClosed {
		t.Error("начальное состояние должно быть Closed (замкнут)")
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Error("после успеха состояние должно остаться Closed")
	}
}

// TestCircuitBreaker_OpensAfterFailures — проверяет переход в Open после maxFailures ошибок.
// Ожидаемое поведение: после 2 ошибок — Closed, после 3-й — Open.
func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Error("после 2 ошибок состояние должно быть Closed")
	}

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Error("после 3 ошибок состояние должно быть Open (разомкнут)")
	}
}

// TestCircuitBreaker_HalfOpenAfterTimeout — проверяет переход из Open в HalfOpen после таймаута.
// Ожидаемое поведение: после истечения resetTimeout состояние переходит в HalfOpen.
func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Error("состояние должно быть Open")
	}

	time.Sleep(150 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Error("после таймаута состояние должно быть HalfOpen (полуоткрыт)")
	}
}

// TestCircuitBreaker_ClosesAfterHalfOpenSuccess — проверяет возврат в Closed из HalfOpen.
// Ожидаемое поведение: после halfOpenMax успешных запросов в HalfOpen — переход в Closed.
func TestCircuitBreaker_ClosesAfterHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(150 * time.Millisecond)

	cb.mu.Lock()
	cb.state = StateHalfOpen
	cb.mu.Unlock()

	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("после успехов в HalfOpen состояние должно быть Closed, получено %d", cb.State())
	}
}

// TestCircuitBreakerMiddleware_RejectsWhenOpen — проверяет отклонение запросов в состоянии Open.
// Ожидаемое поведение: при Open мидлварь возвращает 503 Service Unavailable.
func TestCircuitBreakerMiddleware_RejectsWhenOpen(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Minute)
	cb.RecordFailure()

	mw := CircuitBreakerMiddleware(cb, "test-svc")
	handler := mw(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("ожидался код 503, получен %d", w.Code)
	}
}
