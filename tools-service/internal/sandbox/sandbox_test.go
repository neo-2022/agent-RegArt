package sandbox

import (
	"testing"
	"time"
)

// TestDefaultConfig — проверяет значения конфигурации по умолчанию.
// Ожидаемые значения: образ ubuntu:22.04, лимит памяти 256m,
// лимит CPU 0.5, таймаут 30 секунд.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Image != "ubuntu:22.04" {
		t.Errorf("ожидался образ ubuntu:22.04, получен %s", cfg.Image)
	}
	if cfg.MemoryLimit != "256m" {
		t.Errorf("ожидался лимит памяти 256m, получен %s", cfg.MemoryLimit)
	}
	if cfg.CPULimit != "0.5" {
		t.Errorf("ожидался лимит CPU 0.5, получен %s", cfg.CPULimit)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("ожидался таймаут 30s, получен %v", cfg.Timeout)
	}
}

// TestExecute_DisabledSandbox — проверяет поведение при выключенной песочнице.
// Ожидаемое поведение: если Enabled=false, команда не выполняется,
// возвращается ошибка и Sandboxed=false.
func TestExecute_DisabledSandbox(t *testing.T) {
	cfg := Config{Enabled: false}
	result := Execute(cfg, "echo hello")

	if result.Sandboxed {
		t.Error("команда не должна быть помечена как выполненная в песочнице")
	}
	if result.Error == "" {
		t.Error("должна быть возвращена ошибка при выключенной песочнице")
	}
}

// TestIsAvailable — проверяет доступность Docker на текущей машине.
// Это информационный тест — результат зависит от окружения.
func TestIsAvailable(t *testing.T) {
	available := IsAvailable()
	t.Logf("Docker доступен: %v", available)
}
