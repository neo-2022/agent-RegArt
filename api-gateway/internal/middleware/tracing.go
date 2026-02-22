package middleware

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

func TracingMiddleware(serviceName string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			traceID := r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = generateTraceID()
			}
			spanID := generateSpanID()
			parentSpanID := r.Header.Get("X-Span-ID")

			r.Header.Set("X-Trace-ID", traceID)
			r.Header.Set("X-Span-ID", spanID)
			if parentSpanID != "" {
				r.Header.Set("X-Parent-Span-ID", parentSpanID)
			}

			w.Header().Set("X-Trace-ID", traceID)
			w.Header().Set("X-Span-ID", spanID)

			start := time.Now()
			next.ServeHTTP(w, r)
			duration := time.Since(start)

			log.Printf("[TRACE] service=%s trace_id=%s span_id=%s parent=%s method=%s path=%s duration=%v",
				serviceName, traceID, spanID, parentSpanID, r.Method, r.URL.Path, duration)
		}
	}
}

var traceCounter uint64

func generateTraceID() string {
	n := atomic.AddUint64(&traceCounter, 1)
	return fmt.Sprintf("trace-%d-%d", time.Now().UnixNano(), n)
}

func generateSpanID() string {
	n := atomic.AddUint64(&traceCounter, 1)
	return fmt.Sprintf("span-%d-%d", time.Now().UnixNano(), n)
}
