package middleware

import (
	"net/http"
)

// SecurityHeadersMiddleware — мидлварь для установки заголовков безопасности.
//
// Устанавливает:
//   - X-Content-Type-Options: nosniff — запрет автоопределения MIME-типа
//   - X-Frame-Options: DENY — запрет встраивания в iframe
//   - X-XSS-Protection: 1; mode=block — защита от XSS в старых браузерах
//   - Content-Security-Policy: default-src 'self' — базовая CSP-политика
//   - Referrer-Policy: strict-origin-when-cross-origin — контроль Referer
//   - Permissions-Policy: camera=(), microphone=(), geolocation=() — ограничение API
func SecurityHeadersMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		next.ServeHTTP(w, r)
	}
}
