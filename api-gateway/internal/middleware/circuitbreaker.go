package middleware

import (
	"log"
	"net/http"
	"sync"
	"time"
)

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu              sync.RWMutex
	state           CircuitState
	failures        int
	successes       int
	maxFailures     int
	halfOpenMax     int
	resetTimeout    time.Duration
	lastFailureTime time.Time
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        StateClosed,
		maxFailures:  maxFailures,
		halfOpenMax:  2,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		return StateHalfOpen
	}
	return cb.state
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			log.Printf("[CIRCUIT-BREAKER] state -> CLOSED")
		}
	case StateClosed:
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.maxFailures {
			cb.state = StateOpen
			log.Printf("[CIRCUIT-BREAKER] state -> OPEN (failures=%d)", cb.failures)
		}
	case StateHalfOpen:
		cb.state = StateOpen
		log.Printf("[CIRCUIT-BREAKER] state -> OPEN (half-open failure)")
	}
}

type circuitResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *circuitResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func CircuitBreakerMiddleware(cb *CircuitBreaker, serviceName string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			state := cb.State()

			if state == StateOpen {
				log.Printf("[CIRCUIT-BREAKER] %s: circuit OPEN, rejecting request", serviceName)
				http.Error(w, `{"error":"service unavailable","reason":"circuit breaker open"}`, http.StatusServiceUnavailable)
				return
			}

			if state == StateHalfOpen {
				log.Printf("[CIRCUIT-BREAKER] %s: circuit HALF-OPEN, allowing probe", serviceName)
			}

			crw := &circuitResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(crw, r)

			if crw.statusCode >= 500 {
				cb.RecordFailure()
			} else {
				cb.RecordSuccess()
			}
		}
	}
}
