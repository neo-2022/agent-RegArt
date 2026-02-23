package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeadersMiddleware_SetsAllHeaders(t *testing.T) {
	handler := SecurityHeadersMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	expected := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-Xss-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}

	for header, value := range expected {
		got := w.Header().Get(header)
		if got != value {
			t.Errorf("заголовок %s: ожидалось %q, получено %q", header, value, got)
		}
	}

	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("заголовок Content-Security-Policy отсутствует")
	}

	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("заголовок Strict-Transport-Security отсутствует")
	}
}

func TestSecurityHeadersMiddleware_PassesRequestThrough(t *testing.T) {
	called := false
	handler := SecurityHeadersMiddleware(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if !called {
		t.Error("следующий обработчик не был вызван")
	}
	if w.Code != http.StatusOK {
		t.Errorf("ожидался код 200, получен %d", w.Code)
	}
}
