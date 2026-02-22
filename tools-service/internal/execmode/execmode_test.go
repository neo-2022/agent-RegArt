package execmode

import (
	"os"
	"testing"
)

func TestInit_Normal(t *testing.T) {
	os.Unsetenv("ADMIN_TRUSTED_MODE")
	os.Unsetenv("SAFE_MODE")
	m := Init()
	if m != ModeNormal {
		t.Errorf("ожидали ModeNormal, получили %v", m)
	}
	if IsTrusted() {
		t.Error("не должен быть trusted")
	}
	if IsSafe() {
		t.Error("не должен быть safe")
	}
	if String() != "normal" {
		t.Errorf("String() = %q, ожидали normal", String())
	}
}

func TestInit_Trusted(t *testing.T) {
	os.Setenv("ADMIN_TRUSTED_MODE", "true")
	os.Unsetenv("SAFE_MODE")
	defer os.Unsetenv("ADMIN_TRUSTED_MODE")

	m := Init()
	if m != ModeTrusted {
		t.Errorf("ожидали ModeTrusted, получили %v", m)
	}
	if !IsTrusted() {
		t.Error("должен быть trusted")
	}
	if String() != "trusted" {
		t.Errorf("String() = %q, ожидали trusted", String())
	}
}

func TestInit_Safe(t *testing.T) {
	os.Unsetenv("ADMIN_TRUSTED_MODE")
	os.Setenv("SAFE_MODE", "true")
	defer os.Unsetenv("SAFE_MODE")

	m := Init()
	if m != ModeSafe {
		t.Errorf("ожидали ModeSafe, получили %v", m)
	}
	if !IsSafe() {
		t.Error("должен быть safe")
	}
	if String() != "safe" {
		t.Errorf("String() = %q, ожидали safe", String())
	}
}

func TestInit_SafeOverridesTrusted(t *testing.T) {
	os.Setenv("ADMIN_TRUSTED_MODE", "true")
	os.Setenv("SAFE_MODE", "true")
	defer os.Unsetenv("ADMIN_TRUSTED_MODE")
	defer os.Unsetenv("SAFE_MODE")

	m := Init()
	if m != ModeSafe {
		t.Errorf("SAFE_MODE должен иметь приоритет, получили %v", m)
	}
}
