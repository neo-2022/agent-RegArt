package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidationMiddleware_PassesValidRequest(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/chat", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ожидался код 200, получен %d", w.Code)
	}
}

func TestValidationMiddleware_RejectsLargePayload(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("x"))
	req.ContentLength = MaxPayloadSize + 1
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("ожидался код 413, получен %d", w.Code)
	}
}

func TestValidationMiddleware_RejectsLongURI(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	longPath := "/" + strings.Repeat("a", MaxURILength+1)
	req := httptest.NewRequest("GET", longPath, nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался код 400, получен %d", w.Code)
	}
}

func TestValidationMiddleware_RejectsLongQuery(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	longQuery := "/api?" + strings.Repeat("a", MaxQueryLength+1)
	req := httptest.NewRequest("GET", longQuery, nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался код 400, получен %d", w.Code)
	}
}

func TestValidationMiddleware_RejectsBadContentType(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/xml")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался код 400, получен %d", w.Code)
	}
}

func TestValidationMiddleware_AllowsJSON(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("POST", "/api/chat", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ожидался код 200, получен %d", w.Code)
	}
}

func TestValidationMiddleware_RejectsSuspiciousHeaders(t *testing.T) {
	handler := ValidationMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/chat", nil)
	req.Header.Set("X-Custom", "<script>alert(1)</script>")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ожидался код 400, получен %d", w.Code)
	}
}

func TestIsAllowedContentType(t *testing.T) {
	tests := []struct {
		ct       string
		expected bool
	}{
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"multipart/form-data; boundary=---", true},
		{"text/plain", true},
		{"application/x-www-form-urlencoded", true},
		{"application/xml", false},
		{"text/html", false},
		{"image/png", false},
	}

	for _, tt := range tests {
		t.Run(tt.ct, func(t *testing.T) {
			result := isAllowedContentType(tt.ct)
			if result != tt.expected {
				t.Errorf("isAllowedContentType(%q) = %v, ожидалось %v", tt.ct, result, tt.expected)
			}
		})
	}
}

func TestHasSuspiciousHeaders(t *testing.T) {
	tests := []struct {
		name     string
		headers  map[string]string
		expected bool
	}{
		{"нормальные заголовки", map[string]string{"Content-Type": "application/json"}, false},
		{"XSS script", map[string]string{"X-Custom": "<script>alert(1)</script>"}, true},
		{"javascript:", map[string]string{"X-Redir": "javascript:void(0)"}, true},
		{"onerror", map[string]string{"X-Val": "onerror=alert(1)"}, true},
		{"eval()", map[string]string{"X-Code": "eval(document.cookie)"}, true},
		{"длинный заголовок", map[string]string{"X-Long": strings.Repeat("x", MaxHeaderSize+1)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			result := hasSuspiciousHeaders(req)
			if result != tt.expected {
				t.Errorf("hasSuspiciousHeaders() = %v, ожидалось %v", result, tt.expected)
			}
		})
	}
}
