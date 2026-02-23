package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMetricsMiddleware_IncrementsTotal(t *testing.T) {
	before := atomic.LoadUint64(&metrics.totalRequests)

	handler := MetricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	after := atomic.LoadUint64(&metrics.totalRequests)
	if after <= before {
		t.Error("totalRequests не увеличился после запроса")
	}
}

func TestMetricsMiddleware_TracksErrors(t *testing.T) {
	before := atomic.LoadUint64(&metrics.totalErrors)

	handler := MetricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	req := httptest.NewRequest("GET", "/fail", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	after := atomic.LoadUint64(&metrics.totalErrors)
	if after <= before {
		t.Error("totalErrors не увеличился после ошибки 500")
	}
}

func TestMetricsMiddleware_TracksLatency(t *testing.T) {
	before := atomic.LoadUint64(&metrics.latencyCount)

	handler := MetricsMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/latency", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	after := atomic.LoadUint64(&metrics.latencyCount)
	if after <= before {
		t.Error("latencyCount не увеличился")
	}
}

func TestMetricsHandler_ReturnsPrometheusFormat(t *testing.T) {
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	MetricsHandler(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "http_requests_total") {
		t.Error("метрика http_requests_total отсутствует")
	}
	if !strings.Contains(body, "http_errors_total") {
		t.Error("метрика http_errors_total отсутствует")
	}
	if !strings.Contains(body, "http_active_requests") {
		t.Error("метрика http_active_requests отсутствует")
	}
	if !strings.Contains(body, "http_request_duration_ms_avg") {
		t.Error("метрика http_request_duration_ms_avg отсутствует")
	}

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type должен быть text/plain, получено %q", ct)
	}
}
