package sandbox

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Image != "ubuntu:22.04" {
		t.Errorf("expected default image ubuntu:22.04, got %s", cfg.Image)
	}
	if cfg.MemoryLimit != "256m" {
		t.Errorf("expected memory limit 256m, got %s", cfg.MemoryLimit)
	}
	if cfg.CPULimit != "0.5" {
		t.Errorf("expected CPU limit 0.5, got %s", cfg.CPULimit)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
}

func TestExecute_DisabledSandbox(t *testing.T) {
	cfg := Config{Enabled: false}
	result := Execute(cfg, "echo hello")

	if result.Sandboxed {
		t.Error("should not be sandboxed when disabled")
	}
	if result.Error == "" {
		t.Error("should return error when sandbox is disabled")
	}
}

func TestIsAvailable(t *testing.T) {
	available := IsAvailable()
	t.Logf("Docker available: %v", available)
}
