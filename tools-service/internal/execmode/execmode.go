// Package execmode — режимы выполнения для tools-service.
//
// Определяет два режима:
//   - ADMIN_TRUSTED_MODE=true: Admin-агент получает полный доступ без ограничений,
//     опасные действия только логируются (WARN), но НЕ блокируются.
//   - SAFE_MODE=true: все роли ограничены до viewer-уровня (только чтение).
//     Используется для тестов и демо.
//
// При конфликте (оба true) приоритет у SAFE_MODE.
package execmode

import (
	"log/slog"
	"os"
	"strings"
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeTrusted
	ModeSafe
)

var current Mode

func Init() Mode {
	trusted := strings.EqualFold(os.Getenv("ADMIN_TRUSTED_MODE"), "true")
	safe := strings.EqualFold(os.Getenv("SAFE_MODE"), "true")

	switch {
	case safe:
		current = ModeSafe
		slog.Warn("SAFE_MODE включён — все роли ограничены до viewer")
	case trusted:
		current = ModeTrusted
		slog.Info("ADMIN_TRUSTED_MODE включён — Admin без ограничений, риски логируются")
	default:
		current = ModeNormal
		slog.Info("Стандартный режим — whitelist + RBAC")
	}
	return current
}

func Current() Mode   { return current }
func IsTrusted() bool { return current == ModeTrusted }
func IsSafe() bool    { return current == ModeSafe }
func String() string {
	switch current {
	case ModeTrusted:
		return "trusted"
	case ModeSafe:
		return "safe"
	default:
		return "normal"
	}
}
