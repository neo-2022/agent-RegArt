package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type httpMetrics struct {
	mu             sync.RWMutex
	totalRequests  uint64
	totalErrors    uint64
	statusCounts   map[int]*uint64
	latencySum     uint64
	latencyCount   uint64
	activeRequests int64
}

var metrics = &httpMetrics{
	statusCounts: make(map[int]*uint64),
}

type statusCapture struct {
	http.ResponseWriter
	code int
}

func (sc *statusCapture) WriteHeader(code int) {
	sc.code = code
	sc.ResponseWriter.WriteHeader(code)
}

func MetricsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&metrics.activeRequests, 1)
		defer atomic.AddInt64(&metrics.activeRequests, -1)

		atomic.AddUint64(&metrics.totalRequests, 1)
		start := time.Now()

		sc := &statusCapture{ResponseWriter: w, code: 200}
		next(sc, r)

		dur := uint64(time.Since(start).Milliseconds())
		atomic.AddUint64(&metrics.latencySum, dur)
		atomic.AddUint64(&metrics.latencyCount, 1)

		if sc.code >= 400 {
			atomic.AddUint64(&metrics.totalErrors, 1)
		}

		metrics.mu.Lock()
		cnt, ok := metrics.statusCounts[sc.code]
		if !ok {
			var v uint64
			cnt = &v
			metrics.statusCounts[sc.code] = cnt
		}
		atomic.AddUint64(cnt, 1)
		metrics.mu.Unlock()
	}
}

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	total := atomic.LoadUint64(&metrics.totalRequests)
	errors := atomic.LoadUint64(&metrics.totalErrors)
	active := atomic.LoadInt64(&metrics.activeRequests)
	latSum := atomic.LoadUint64(&metrics.latencySum)
	latCnt := atomic.LoadUint64(&metrics.latencyCount)

	fmt.Fprintf(w, "# HELP http_requests_total Общее количество HTTP-запросов\n")
	fmt.Fprintf(w, "# TYPE http_requests_total counter\n")
	fmt.Fprintf(w, "http_requests_total %d\n\n", total)

	fmt.Fprintf(w, "# HELP http_errors_total Общее количество HTTP-ошибок (4xx/5xx)\n")
	fmt.Fprintf(w, "# TYPE http_errors_total counter\n")
	fmt.Fprintf(w, "http_errors_total %d\n\n", errors)

	fmt.Fprintf(w, "# HELP http_active_requests Текущие активные запросы\n")
	fmt.Fprintf(w, "# TYPE http_active_requests gauge\n")
	fmt.Fprintf(w, "http_active_requests %d\n\n", active)

	var avgMs float64
	if latCnt > 0 {
		avgMs = float64(latSum) / float64(latCnt)
	}
	fmt.Fprintf(w, "# HELP http_request_duration_ms_avg Средняя задержка запроса (мс)\n")
	fmt.Fprintf(w, "# TYPE http_request_duration_ms_avg gauge\n")
	fmt.Fprintf(w, "http_request_duration_ms_avg %.2f\n\n", avgMs)

	fmt.Fprintf(w, "# HELP http_requests_by_status Количество запросов по HTTP-статусу\n")
	fmt.Fprintf(w, "# TYPE http_requests_by_status counter\n")
	metrics.mu.RLock()
	for code, cnt := range metrics.statusCounts {
		fmt.Fprintf(w, "http_requests_by_status{code=\"%d\"} %d\n", code, atomic.LoadUint64(cnt))
	}
	metrics.mu.RUnlock()
}
