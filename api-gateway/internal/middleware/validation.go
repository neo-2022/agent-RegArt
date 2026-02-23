package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/neo-2022/openclaw-memory/api-gateway/internal/apierror"
)

const (
	MaxPayloadSize = 10 * 1024 * 1024 // 10 МБ
	MaxURILength   = 2048             // Максимальная длина URI
	MaxHeaderSize  = 8192             // Максимальный размер одного заголовка (8 КБ)
	MaxQueryLength = 4096             // Максимальная длина query string
)

// ValidationMiddleware — HTTP-мидлварь для валидации входящих запросов.
//
// Проверяет:
//   - Content-Length не превышает MaxPayloadSize (10 МБ)
//   - Длина URI не превышает MaxURILength (2048)
//   - Длина query string не превышает MaxQueryLength (4096)
//   - Content-Type корректен для POST/PUT/PATCH запросов
//   - Отсутствие подозрительных паттернов в заголовках
func ValidationMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cid := r.Header.Get("X-Request-ID")

		if r.ContentLength > MaxPayloadSize {
			slog.Warn("[ВАЛИДАЦИЯ] превышен размер payload",
				slog.Int64("размер", r.ContentLength),
				slog.Int64("лимит", MaxPayloadSize),
				slog.String("путь", r.URL.Path),
			)
			apierror.PayloadTooLarge(w, cid, "размер запроса превышает лимит 10 МБ")
			return
		}

		if len(r.URL.RequestURI()) > MaxURILength {
			slog.Warn("[ВАЛИДАЦИЯ] превышена длина URI",
				slog.Int("длина", len(r.URL.RequestURI())),
				slog.Int("лимит", MaxURILength),
			)
			apierror.BadRequest(w, cid, "URI слишком длинный")
			return
		}

		if len(r.URL.RawQuery) > MaxQueryLength {
			slog.Warn("[ВАЛИДАЦИЯ] превышена длина query string",
				slog.Int("длина", len(r.URL.RawQuery)),
				slog.Int("лимит", MaxQueryLength),
			)
			apierror.BadRequest(w, cid, "Query string слишком длинный")
			return
		}

		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			ct := r.Header.Get("Content-Type")
			if ct != "" && !isAllowedContentType(ct) {
				slog.Warn("[ВАЛИДАЦИЯ] недопустимый Content-Type",
					slog.String("content-type", ct),
					slog.String("путь", r.URL.Path),
				)
				apierror.BadRequest(w, cid, "недопустимый Content-Type")
				return
			}
		}

		if hasSuspiciousHeaders(r) {
			slog.Warn("[ВАЛИДАЦИЯ] подозрительные заголовки",
				slog.String("путь", r.URL.Path),
				slog.String("ip", r.RemoteAddr),
			)
			apierror.BadRequest(w, cid, "подозрительные заголовки запроса")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, MaxPayloadSize)

		next.ServeHTTP(w, r)
	}
}

// isAllowedContentType — проверяет, является ли Content-Type допустимым.
func isAllowedContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	allowed := []string{
		"application/json",
		"application/x-www-form-urlencoded",
		"multipart/form-data",
		"text/plain",
	}
	for _, a := range allowed {
		if strings.HasPrefix(ct, a) {
			return true
		}
	}
	return false
}

// hasSuspiciousHeaders — проверяет наличие подозрительных паттернов в заголовках.
func hasSuspiciousHeaders(r *http.Request) bool {
	suspicious := []string{
		"<script",
		"javascript:",
		"onerror=",
		"onload=",
		"eval(",
		"document.cookie",
	}
	for _, values := range r.Header {
		for _, v := range values {
			lower := strings.ToLower(v)
			for _, pattern := range suspicious {
				if strings.Contains(lower, pattern) {
					return true
				}
			}
			if len(v) > MaxHeaderSize {
				return true
			}
		}
	}
	return false
}
