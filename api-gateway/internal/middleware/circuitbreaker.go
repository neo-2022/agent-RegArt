// Package middleware — HTTP-мидлвари для api-gateway.
//
// Содержит реализации Circuit Breaker, Rate Limiter и Tracing,
// которые обеспечивают устойчивость, защиту от перегрузок и трассировку запросов.
package middleware

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/neo-2022/openclaw-memory/api-gateway/internal/apierror"
)

// CircuitState — состояние автоматического выключателя (Circuit Breaker).
type CircuitState int

const (
	// StateClosed — замкнут (нормальная работа, запросы проходят).
	StateClosed CircuitState = iota
	// StateOpen — разомкнут (сервис недоступен, запросы отклоняются).
	StateOpen
	// StateHalfOpen — полуоткрыт (пропускаем пробные запросы для проверки восстановления).
	StateHalfOpen
)

// CircuitBreaker — реализация паттерна Circuit Breaker.
//
// Отслеживает количество ошибок от бэкенд-сервиса. При достижении maxFailures
// переходит в состояние Open и отклоняет все запросы на время resetTimeout.
// После таймаута переходит в HalfOpen и пропускает пробные запросы.
// При успехе пробных запросов возвращается в Closed.
type CircuitBreaker struct {
	mu              sync.RWMutex  // Мьютекс для потокобезопасного доступа
	state           CircuitState  // Текущее состояние
	failures        int           // Количество последовательных ошибок
	successes       int           // Количество успехов в состоянии HalfOpen
	maxFailures     int           // Порог ошибок для перехода в Open
	halfOpenMax     int           // Количество успехов для возврата в Closed из HalfOpen
	resetTimeout    time.Duration // Таймаут перед переходом из Open в HalfOpen
	lastFailureTime time.Time     // Время последней ошибки
}

// NewCircuitBreaker — создаёт новый Circuit Breaker.
// maxFailures — сколько ошибок нужно для перехода в Open.
// resetTimeout — через сколько времени попробовать восстановить соединение.
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        StateClosed,
		maxFailures:  maxFailures,
		halfOpenMax:  2,
		resetTimeout: resetTimeout,
	}
}

// State — получить текущее состояние Circuit Breaker.
// Автоматически переходит из Open в HalfOpen, если прошло достаточно времени.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.state == StateOpen && time.Since(cb.lastFailureTime) > cb.resetTimeout {
		return StateHalfOpen
	}
	return cb.state
}

// RecordSuccess — зафиксировать успешный ответ от бэкенд-сервиса.
// В состоянии HalfOpen: при достижении halfOpenMax успехов — переход в Closed.
// В состоянии Closed: сбрасывает счётчик ошибок.
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
			log.Printf("[CIRCUIT-BREAKER] состояние -> CLOSED (замкнут)")
		}
	case StateClosed:
		cb.failures = 0
	}
}

// RecordFailure — зафиксировать ошибку от бэкенд-сервиса.
// В состоянии Closed: при достижении maxFailures — переход в Open.
// В состоянии HalfOpen: любая ошибка — переход обратно в Open.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.maxFailures {
			cb.state = StateOpen
			log.Printf("[CIRCUIT-BREAKER] состояние -> OPEN (разомкнут, ошибок=%d)", cb.failures)
		}
	case StateHalfOpen:
		cb.state = StateOpen
		log.Printf("[CIRCUIT-BREAKER] состояние -> OPEN (ошибка в полуоткрытом режиме)")
	}
}

// circuitResponseWriter — обёртка над http.ResponseWriter для перехвата статус-кода.
type circuitResponseWriter struct {
	http.ResponseWriter
	statusCode int // Перехваченный HTTP статус-код ответа
}

// WriteHeader — перехватывает статус-код ответа перед отправкой клиенту.
func (w *circuitResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// CircuitBreakerMiddleware — HTTP-мидлварь, оборачивающая обработчик в Circuit Breaker.
//
// Если Circuit Breaker в состоянии Open — сразу отклоняет запрос (503 Service Unavailable).
// Если в HalfOpen — пропускает запрос как пробный.
// После выполнения запроса фиксирует результат (успех при статусе <500, ошибка при >=500).
func CircuitBreakerMiddleware(cb *CircuitBreaker, serviceName string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			state := cb.State()

			if state == StateOpen {
				log.Printf("[CIRCUIT-BREAKER] %s: цепь разомкнута, запрос отклонён", serviceName)
				cid := r.Header.Get("X-Request-ID")
				apierror.ServiceUnavailable(w, cid, "сервис недоступен", "circuit breaker open")
				return
			}

			if state == StateHalfOpen {
				log.Printf("[CIRCUIT-BREAKER] %s: полуоткрытый режим, пропускаем пробный запрос", serviceName)
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
