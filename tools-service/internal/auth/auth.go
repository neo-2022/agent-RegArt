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
	"log/slog"
	"net/http"
	"os"
	"strings"
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
		if len(tokenRoles) == 0 {
			ctx := context.WithValue(r.Context(), roleContextKey, RoleAdmin)
			next(w, r.WithContext(ctx))
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error":"отсутствует заголовок Authorization"}`, http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"формат: Authorization: Bearer <token>"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		role, ok := tokenRoles[token]
		if !ok {
			slog.Warn("Невалидный токен", slog.String("endpoint", r.URL.Path))
			http.Error(w, `{"error":"невалидный токен"}`, http.StatusUnauthorized)
			return
		}

		if !HasAccess(role, requiredRole) {
			slog.Warn("Недостаточно прав",
				slog.String("роль", string(role)),
				slog.String("требуется", string(requiredRole)),
				slog.String("endpoint", r.URL.Path),
			)
			http.Error(w, `{"error":"недостаточно прав (требуется `+string(requiredRole)+`)"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), roleContextKey, role)
		next(w, r.WithContext(ctx))
	}
}

// RoleFromContext — извлекает роль из контекста запроса.
func RoleFromContext(ctx context.Context) Role {
	if role, ok := ctx.Value(roleContextKey).(Role); ok {
		return role
	}
	return RoleViewer
}
