// Package auth — аутентификация и авторизация для tools-service.
//
// Реализует:
//   - Парсинг токенов из переменной окружения TOOLS_AUTH_TOKENS
//   - Иерархию ролей: viewer < operator < admin
//   - HTTP-middleware для проверки Bearer-токенов и ролей
//   - Legacy-режим (без токенов) с предупреждением
package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/neo-2022/openclaw-memory/tools-service/internal/execmode"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

type contextKey string

const roleContextKey contextKey = "auth_role"

var roleLevel = map[Role]int{
	RoleViewer:   1,
	RoleOperator: 2,
	RoleAdmin:    3,
}

// HasAccess — проверяет, достаточно ли роли для требуемого уровня доступа.
func HasAccess(actual, required Role) bool {
	return roleLevel[actual] >= roleLevel[required]
}

// ParseTokens — парсит TOOLS_AUTH_TOKENS="token1:viewer,token2:operator,token3:admin"
// Возвращает карту токен→роль.
func ParseTokens(raw string) map[string]Role {
	result := make(map[string]Role)
	if raw == "" {
		return result
	}
	pairs := strings.Split(raw, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			slog.Warn("Пропущен невалидный токен (формат token:role)", slog.String("пара", pair))
			continue
		}
		token := strings.TrimSpace(parts[0])
		role := Role(strings.TrimSpace(parts[1]))
		if _, ok := roleLevel[role]; !ok {
			slog.Warn("Неизвестная роль, пропущена", slog.String("роль", string(role)))
			continue
		}
		result[token] = role
	}
	return result
}

// LoadTokensFromEnv — загружает токены из переменной окружения TOOLS_AUTH_TOKENS.
func LoadTokensFromEnv() map[string]Role {
	raw := os.Getenv("TOOLS_AUTH_TOKENS")
	tokens := ParseTokens(raw)
	if len(tokens) == 0 {
		slog.Warn("TOOLS_AUTH_TOKENS не задан или пуст — работа в legacy-режиме без аутентификации")
	} else {
		slog.Info("Загружены токены аутентификации", slog.Int("количество", len(tokens)))
	}
	return tokens
}

// WithAuth — middleware для проверки Bearer-токена и минимальной роли.
// Если tokenRoles пуст (legacy-режим), пропускает все запросы с предупреждением.
func WithAuth(requiredRole Role, tokenRoles map[string]Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")

		if len(tokenRoles) == 0 {
			role := RoleAdmin
			if execmode.IsSafe() {
				role = RoleViewer
			}
			ctx := context.WithValue(r.Context(), roleContextKey, role)
			next(w, r.WithContext(ctx))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "отсутствует заголовок Authorization", "Добавьте Authorization: Bearer <token>", requestID)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "формат: Authorization: Bearer <token>", "Используйте Bearer-токен", requestID)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		role, ok := tokenRoles[token]
		if !ok {
			slog.Warn("Невалидный токен", slog.String("endpoint", r.URL.Path))
			writeAuthError(w, http.StatusUnauthorized, "UNAUTHORIZED", "невалидный токен", "Проверьте TOOLS_AUTH_TOKENS", requestID)
			return
		}

		if execmode.IsSafe() {
			role = RoleViewer
		}

		if !HasAccess(role, requiredRole) {
			slog.Warn("Недостаточно прав",
				slog.String("роль", string(role)),
				slog.String("требуется", string(requiredRole)),
				slog.String("endpoint", r.URL.Path),
			)
			writeAuthError(w, http.StatusForbidden, "FORBIDDEN", "недостаточно прав (требуется "+string(requiredRole)+")", "Используйте токен с ролью "+string(requiredRole)+" или выше", requestID)
			return
		}

		ctx := context.WithValue(r.Context(), roleContextKey, role)
		next(w, r.WithContext(ctx))
	}
}

type authErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Hint      string `json:"hint,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Retryable bool   `json:"retryable"`
}

func writeAuthError(w http.ResponseWriter, status int, code, message, hint, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(authErrorResponse{
		Code:      code,
		Message:   message,
		Hint:      hint,
		RequestID: requestID,
		Retryable: false,
	})
}

// RoleFromContext — извлекает роль из контекста запроса.
func RoleFromContext(ctx context.Context) Role {
	if role, ok := ctx.Value(roleContextKey).(Role); ok {
		return role
	}
	return RoleViewer
}
