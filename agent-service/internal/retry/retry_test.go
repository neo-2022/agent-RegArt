package retry

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDo_SuccessFirstAttempt(t *testing.T) {
	calls := 0
	err := Do(DefaultConfig, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("ожидался nil, получена ошибка: %v", err)
	}
	if calls != 1 {
		t.Fatalf("ожидался 1 вызов, получено %d", calls)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	calls := 0
	err := Do(Config{MaxRetries: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5}, func() error {
		calls++
		if calls < 3 {
			return errors.New("connection refused")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ожидался nil, получена ошибка: %v", err)
	}
	if calls != 3 {
		t.Fatalf("ожидалось 3 вызова, получено %d", calls)
	}
}

func TestDo_AllRetriesExhausted(t *testing.T) {
	calls := 0
	err := Do(Config{MaxRetries: 2, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5}, func() error {
		calls++
		return errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
	if calls != 3 { // 1 начальный + 2 retry
		t.Fatalf("ожидалось 3 вызова, получено %d", calls)
	}
	if !strings.Contains(err.Error(), "попыток исчерпаны") {
		t.Fatalf("ожидалось сообщение об исчерпании попыток, получено: %v", err)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	calls := 0
	err := Do(Config{MaxRetries: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5}, func() error {
		calls++
		return errors.New("invalid JSON format")
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
	if calls != 1 {
		t.Fatalf("ожидался 1 вызов (нетранзиентная ошибка), получено %d", calls)
	}
}

func TestDoWithContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // отменяем сразу

	calls := 0
	err := DoWithContext(ctx, Config{MaxRetries: 3, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5}, func() error {
		calls++
		return errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("ожидалась ошибка, получен nil")
	}
	if !strings.Contains(err.Error(), "retry отменён") {
		t.Fatalf("ожидалась ошибка отмены, получено: %v", err)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil ошибка", nil, false},
		{"connection refused", errors.New("connection refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"timeout", errors.New("i/o timeout"), true},
		{"HTTP 502", errors.New("HTTP 502 Bad Gateway"), true},
		{"HTTP 503", errors.New("ошибка 503 Service Unavailable"), true},
		{"HTTP 504", errors.New("504 Gateway Timeout"), true},
		{"HTTP 429", errors.New("429 Too Many Requests"), true},
		{"EOF", errors.New("unexpected EOF"), true},
		{"invalid JSON", errors.New("invalid JSON"), false},
		{"not found", errors.New("404 not found"), false},
		{"auth error", errors.New("401 unauthorized"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, ожидалось %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestDoWithResult_Success(t *testing.T) {
	calls := 0
	result, err := DoWithResult(DefaultConfig, func() (string, error) {
		calls++
		if calls < 2 {
			return "", errors.New("connection refused")
		}
		return "результат", nil
	})
	if err != nil {
		t.Fatalf("ожидался nil, получена ошибка: %v", err)
	}
	if result != "результат" {
		t.Fatalf("ожидался 'результат', получено '%s'", result)
	}
}

func TestDoWithResult_AllFailed(t *testing.T) {
	cfg := Config{MaxRetries: 1, InitialDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, Multiplier: 1.5}
	result, err := DoWithResult(cfg, func() (int, error) {
		return 0, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if result != 0 {
		t.Fatalf("ожидалось нулевое значение, получено %d", result)
	}
}

func TestExponentialBackoff(t *testing.T) {
	start := time.Now()
	calls := 0
	cfg := Config{MaxRetries: 2, InitialDelay: 50 * time.Millisecond, MaxDelay: 200 * time.Millisecond, Multiplier: 2.0}
	_ = Do(cfg, func() error {
		calls++
		return errors.New("connection refused")
	})
	elapsed := time.Since(start)
	// 1-я задержка: 50ms, 2-я задержка: 100ms → суммарно ~150ms минимум
	if elapsed < 100*time.Millisecond {
		t.Fatalf("backoff слишком быстрый: %v (ожидалось >= 100ms)", elapsed)
	}
}

func TestMaxDelayRespected(t *testing.T) {
	start := time.Now()
	cfg := Config{MaxRetries: 3, InitialDelay: 50 * time.Millisecond, MaxDelay: 60 * time.Millisecond, Multiplier: 10.0}
	_ = Do(cfg, func() error {
		return errors.New("connection refused")
	})
	elapsed := time.Since(start)
	// maxDelay=60ms → задержки: 50, 60, 60 = 170ms максимум + время выполнения
	if elapsed > 500*time.Millisecond {
		t.Fatalf("maxDelay не работает: %v (ожидалось < 500ms)", elapsed)
	}
}
