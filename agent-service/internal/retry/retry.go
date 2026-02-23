// Package retry — универсальный механизм повторных попыток с exponential backoff.
//
// Используется для устойчивости к транзиентным сбоям при:
//   - HTTP-запросах к ChromaDB
//   - Вызовах инструментов через tools-service
//   - Запросах к LLM-провайдерам
//
// Стратегия: exponential backoff с коэффициентом 1.5x и ограничением максимальной задержки.
package retry

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"strings"
	"time"
)

// Config — конфигурация retry-логики.
type Config struct {
	MaxRetries   int           // Максимальное количество повторных попыток (по умолчанию 3)
	InitialDelay time.Duration // Начальная задержка перед первой повторной попыткой (по умолчанию 500ms)
	MaxDelay     time.Duration // Максимальная задержка между попытками (по умолчанию 10s)
	Multiplier   float64       // Коэффициент увеличения задержки (по умолчанию 1.5)
}

// DefaultConfig — конфигурация по умолчанию для retry.
var DefaultConfig = Config{
	MaxRetries:   3,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     10 * time.Second,
	Multiplier:   1.5,
}

// ToolCallConfig — конфигурация для вызовов инструментов (tools-service).
var ToolCallConfig = Config{
	MaxRetries:   3,
	InitialDelay: 1 * time.Second,
	MaxDelay:     8 * time.Second,
	Multiplier:   2.0,
}

// ChromaDBConfig — конфигурация для запросов к ChromaDB.
var ChromaDBConfig = Config{
	MaxRetries:   3,
	InitialDelay: 500 * time.Millisecond,
	MaxDelay:     5 * time.Second,
	Multiplier:   1.5,
}

// LLMConfig — конфигурация для запросов к LLM-провайдерам.
var LLMConfig = Config{
	MaxRetries:   3,
	InitialDelay: 2 * time.Second,
	MaxDelay:     15 * time.Second,
	Multiplier:   2.0,
}

// IsRetryable — проверяет, является ли ошибка транзиентной (стоит повторить).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Сетевые ошибки (таймаут, connection refused, reset)
	var netErr net.Error
	if ok := isNetError(err, &netErr); ok {
		return true
	}

	// HTTP ошибки сервера (502, 503, 504, 429)
	for _, code := range []string{"502", "503", "504", "429"} {
		if strings.Contains(errStr, code) {
			return true
		}
	}

	// Ошибки соединения
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"eof",
		"timeout",
		"temporary failure",
		"no such host",
		"i/o timeout",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}

	return false
}

// isNetError — проверяет, является ли ошибка сетевой.
func isNetError(err error, target *net.Error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset")
}

// Do — выполняет функцию fn с повторными попытками при транзиентных ошибках.
// Использует exponential backoff между попытками.
func Do(cfg Config, fn func() error) error {
	return DoWithContext(context.Background(), cfg, fn)
}

// DoWithContext — выполняет функцию с retry и поддержкой контекста для отмены.
func DoWithContext(ctx context.Context, cfg Config, fn func() error) error {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultConfig.MaxRetries
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = DefaultConfig.InitialDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultConfig.MaxDelay
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = DefaultConfig.Multiplier
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("[RETRY] повторная попытка",
				slog.Int("попытка", attempt),
				slog.Int("макс", cfg.MaxRetries),
				slog.Duration("задержка", delay),
				slog.String("ошибка", lastErr.Error()),
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf("retry отменён: %w (последняя ошибка: %v)", ctx.Err(), lastErr)
			case <-time.After(delay):
			}

			delay = time.Duration(float64(delay) * cfg.Multiplier)
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
		}

		err := fn()
		if err == nil {
			if attempt > 0 {
				slog.Info("[RETRY] успех после повтора", slog.Int("попытка", attempt))
			}
			return nil
		}

		lastErr = err

		if !IsRetryable(err) {
			slog.Debug("[RETRY] ошибка не является транзиентной, retry пропущен",
				slog.String("ошибка", err.Error()),
			)
			return err
		}
	}

	return fmt.Errorf("все %d попыток исчерпаны: %w", cfg.MaxRetries+1, lastErr)
}

// DoWithResult — выполняет функцию, возвращающую результат и ошибку, с retry.
func DoWithResult[T any](cfg Config, fn func() (T, error)) (T, error) {
	return DoWithResultContext[T](context.Background(), cfg, fn)
}

// DoWithResultContext — выполняет функцию с результатом, retry и контекстом.
func DoWithResultContext[T any](ctx context.Context, cfg Config, fn func() (T, error)) (T, error) {
	var zero T
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = DefaultConfig.MaxRetries
	}
	if cfg.InitialDelay <= 0 {
		cfg.InitialDelay = DefaultConfig.InitialDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultConfig.MaxDelay
	}
	if cfg.Multiplier <= 0 {
		cfg.Multiplier = DefaultConfig.Multiplier
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			slog.Warn("[RETRY] повторная попытка",
				slog.Int("попытка", attempt),
				slog.Int("макс", cfg.MaxRetries),
				slog.Duration("задержка", delay),
				slog.String("ошибка", lastErr.Error()),
			)

			select {
			case <-ctx.Done():
				return zero, fmt.Errorf("retry отменён: %w (последняя ошибка: %v)", ctx.Err(), lastErr)
			case <-time.After(delay):
			}

			delay = time.Duration(math.Min(float64(delay)*cfg.Multiplier, float64(cfg.MaxDelay)))
		}

		result, err := fn()
		if err == nil {
			if attempt > 0 {
				slog.Info("[RETRY] успех после повтора", slog.Int("попытка", attempt))
			}
			return result, nil
		}

		lastErr = err

		if !IsRetryable(err) {
			return zero, err
		}
	}

	return zero, fmt.Errorf("все %d попыток исчерпаны: %w", cfg.MaxRetries+1, lastErr)
}
