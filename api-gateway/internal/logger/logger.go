// Пакет logger — структурированное JSON-логирование для api-gateway.
// Использует стандартный пакет log/slog для вывода логов в формате JSON.
// Каждая запись содержит: время, уровень, сообщение, сервис, correlation-id.
package logger

import (
	"context"
	"log/slog"
	"os"
)

// ctxKey — тип ключа для хранения значений в контексте.
type ctxKey string

// CorrelationIDKey — ключ для хранения идентификатора корреляции в контексте запроса.
const CorrelationIDKey ctxKey = "correlation_id"

// Инициализация — вызывается один раз при старте сервиса.
// Устанавливает глобальный логгер с JSON-форматом и именем сервиса.
func Init(serviceName string) {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	// Оборачиваем обработчик для автоматического добавления имени сервиса
	logger := slog.New(handler).With(slog.String("сервис", serviceName))
	slog.SetDefault(logger)
}

// С возвращает логгер с привязанным идентификатором корреляции из контекста.
// Если в контексте нет correlation-id — возвращает логгер без него.
func С(ctx context.Context) *slog.Logger {
	if cid, ok := ctx.Value(CorrelationIDKey).(string); ok && cid != "" {
		return slog.Default().With(slog.String("correlation_id", cid))
	}
	return slog.Default()
}

// WithCorrelationID — добавляет идентификатор корреляции в контекст.
func WithCorrelationID(ctx context.Context, cid string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, cid)
}
