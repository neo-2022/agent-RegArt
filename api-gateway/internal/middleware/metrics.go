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
	routeCounts    map[string]*uint64
	routeLatency   map[string]*uint64
}

var metrics = &httpMetrics{
	statusCounts: make(map[int]*uint64),
	routeCounts:  make(map[string]*uint64),
	routeLatency: make(map[string]*uint64),
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

		route := normalizeRoute(r.URL.Path)

		metrics.mu.Lock()
		cnt, ok := metrics.statusCounts[sc.code]
		if !ok {
			var v uint64
			cnt = &v
			metrics.statusCounts[sc.code] = cnt
		}
		atomic.AddUint64(cnt, 1)

		rc, ok := metrics.routeCounts[route]
		if !ok {
			var v uint64
			rc = &v
			metrics.routeCounts[route] = rc
		}
		atomic.AddUint64(rc, 1)

		rl, ok := metrics.routeLatency[route]
		if !ok {
			var v uint64
			rl = &v
			metrics.routeLatency[route] = rl
		}
		atomic.AddUint64(rl, dur)
		metrics.mu.Unlock()
	}
}

func normalizeRoute(path string) string {
	prefixes := []string{"/memory/", "/tools/", "/agents/", "/ydisk/", "/rag/", "/autoskill/", "/uploads/"}
	for _, p := range prefixes {
		if len(path) >= len(p) && path[:len(p)] == p {
			return p + "*"
		}
	}
	return path
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

	fmt.Fprintf(w, "\n# HELP http_requests_by_route Количество запросов по маршруту\n")
	fmt.Fprintf(w, "# TYPE http_requests_by_route counter\n")
	for route, cnt := range metrics.routeCounts {
		fmt.Fprintf(w, "http_requests_by_route{route=\"%s\"} %d\n", route, atomic.LoadUint64(cnt))
	}

	fmt.Fprintf(w, "\n# HELP http_route_latency_ms_total Суммарная задержка по маршруту (мс)\n")
	fmt.Fprintf(w, "# TYPE http_route_latency_ms_total counter\n")
	for route, lat := range metrics.routeLatency {
		fmt.Fprintf(w, "http_route_latency_ms_total{route=\"%s\"} %d\n", route, atomic.LoadUint64(lat))
	}
	metrics.mu.RUnlock()
}
