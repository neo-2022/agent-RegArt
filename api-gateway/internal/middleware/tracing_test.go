package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTracingMiddleware_GeneratesTraceID(t *testing.T) {
	traceMW := TracingMiddleware("test-service")
	handler := traceMW(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	traceID := w.Header().Get("X-Trace-ID")
	if traceID == "" {
		t.Error("X-Trace-ID не установлен в ответе")
	}
	if !strings.HasPrefix(traceID, "trace-") {
		t.Errorf("X-Trace-ID должен начинаться с 'trace-', получено %q", traceID)
	}
}

func TestTracingMiddleware_PreservesExistingTraceID(t *testing.T) {
	traceMW := TracingMiddleware("test-service")
	handler := traceMW(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Trace-ID", "existing-trace-123")
	w := httptest.NewRecorder()
	handler(w, req)

	traceID := w.Header().Get("X-Trace-ID")
	if traceID != "existing-trace-123" {
		t.Errorf("X-Trace-ID должен быть 'existing-trace-123', получено %q", traceID)
	}
}

func TestTracingMiddleware_GeneratesSpanID(t *testing.T) {
	traceMW := TracingMiddleware("test-service")
	handler := traceMW(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	spanID := w.Header().Get("X-Span-ID")
	if spanID == "" {
		t.Error("X-Span-ID не установлен в ответе")
	}
	if !strings.HasPrefix(spanID, "span-") {
		t.Errorf("X-Span-ID должен начинаться с 'span-', получено %q", spanID)
	}
}

func TestGenerateTraceID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateTraceID()
		if ids[id] {
			t.Errorf("дубликат Trace ID: %s", id)
		}
		ids[id] = true
	}
}

func TestGenerateSpanID_Unique(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateSpanID()
		if ids[id] {
			t.Errorf("дубликат Span ID: %s", id)
		}
		ids[id] = true
	}
}
