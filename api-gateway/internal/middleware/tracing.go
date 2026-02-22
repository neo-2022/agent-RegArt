package middleware

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

// TracingMiddleware — HTTP-мидлварь для распределённой трассировки запросов.
//
// Для каждого входящего запроса:
// 1. Проверяет наличие заголовка X-Trace-ID (для продолжения существующей трассировки).
// 2. Если заголовка нет — генерирует новый Trace ID.
// 3. Генерирует уникальный Span ID для текущего участка обработки.
// 4. Сохраняет Parent Span ID, если запрос пришёл с уже установленным X-Span-ID.
// 5. Устанавливает заголовки X-Trace-ID и X-Span-ID в ответ.
// 6. Логирует время обработки запроса.
//
// Совместим со стилем OpenTelemetry, но не требует полного SDK.
func TracingMiddleware(serviceName string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Получить или сгенерировать Trace ID
			traceID := r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = generateTraceID()
			}
			// Сгенерировать Span ID для текущего участка
			spanID := generateSpanID()
			// Сохранить родительский Span ID (если есть)
			parentSpanID := r.Header.Get("X-Span-ID")

			// Установить заголовки для дальнейшей передачи по цепочке сервисов
			r.Header.Set("X-Trace-ID", traceID)
			r.Header.Set("X-Span-ID", spanID)
			if parentSpanID != "" {
				r.Header.Set("X-Parent-Span-ID", parentSpanID)
			}

			// Установить заголовки в ответ клиенту
			w.Header().Set("X-Trace-ID", traceID)
			w.Header().Set("X-Span-ID", spanID)

			// Замерить время обработки запроса
			start := time.Now()
			next.ServeHTTP(w, r)
			duration := time.Since(start)

			log.Printf("[ТРАССИРОВКА] сервис=%s trace_id=%s span_id=%s parent=%s метод=%s путь=%s длительность=%v",
				serviceName, traceID, spanID, parentSpanID, r.Method, r.URL.Path, duration)
		}
	}
}

// traceCounter — атомарный счётчик для генерации уникальных ID трассировки.
var traceCounter uint64

// generateTraceID — генерирует уникальный идентификатор трассировки.
// Формат: trace-{unix_nano}-{порядковый_номер}.
func generateTraceID() string {
	n := atomic.AddUint64(&traceCounter, 1)
	return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), n)
}

// generateSpanID — генерирует уникальный идентификатор участка (span).
// Формат: span-{unix_nano}-{порядковый_номер}.
func generateSpanID() string {
	n := atomic.AddUint64(&traceCounter, 1)
	return fmt.Sprintf("span-%d-%d", time.Now().UnixNano(), n)
}
