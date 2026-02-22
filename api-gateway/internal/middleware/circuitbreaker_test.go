package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedState(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	if cb.State() != StateClosed {
		t.Error("initial state should be Closed")
	}

	cb.RecordSuccess()
	if cb.State() != StateClosed {
		t.Error("state should remain Closed after success")
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Error("should be Closed after 2 failures")
	}

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Error("should be Open after 3 failures")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Error("should be Open")
	}

	time.Sleep(150 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Error("should be HalfOpen after reset timeout")
	}
}

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
		t.Errorf("should be Closed after half-open successes, got %d", cb.State())
	}
}

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
		t.Errorf("expected 503, got %d", w.Code)
	}
}
