// Package repository — тесты классификации моделей для единого агента Admin.
//
// Проверяют логику определения подходящих ролей для LLM-моделей
// на основе размера параметров, поддержки инструментов
// и семейства модели.
package repository

import (
	"strings"
	"testing"
)

// TestParseParamSize — проверяет парсинг строки размера параметров модели.
// Ожидаемое поведение:
//   - "8B" → 8.0 (миллиарды параметров)
//   - "500M" → 0.5 (миллионы → миллиарды)
//   - "" → 0 (пустая строка)
//   - "3.5B" → 3.5 (дробное значение)
func TestParseParamSize(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"8B", 8.0},
		{"13B", 13.0},
		{"70B", 70.0},
		{"7b", 7.0},
		{"500M", 0.5},
		{"1500M", 1.5},
		{"", 0},
		{"  ", 0},
		{"3.5B", 3.5},
		{"0.5B", 0.5},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := parseParamSize(tc.input)
			if result != tc.expected {
				t.Errorf("parseParamSize(%q) = %f, ожидалось %f", tc.input, result, tc.expected)
			}
		})
	}
}

// TestIsCodeModel — проверяет определение модели для кодирования.
// Ожидаемое поведение:
//   - Модели с "coder", "codellama", "starcoder", "codegemma" в имени → true
//   - Модели с семейством "coder", "codellama" → true
//   - Обычные модели (llama, mistral, phi) → false
func TestIsCodeModel(t *testing.T) {
	tests := []struct {
		modelName string
		family    string
		expected  bool
	}{
		{"qwen2.5-coder:7b", "", true},
		{"deepseek-coder:6.7b", "", true},
		{"codellama:13b", "", true},
		{"starcoder2:3b", "", true},
		{"codegemma:7b", "", true},
		{"llama3.1:8b", "", false},
		{"mistral:7b", "", false},
		{"phi3:mini", "", false},
		{"some-model", "coder", true},
		{"some-model", "codellama", true},
		{"some-model", "llama", false},
	}

	for _, tc := range tests {
		t.Run(tc.modelName, func(t *testing.T) {
			result := isCodeModel(tc.modelName, tc.family)
			if result != tc.expected {
				t.Errorf("isCodeModel(%q, %q) = %v, ожидалось %v", tc.modelName, tc.family, result, tc.expected)
			}
		})
	}
}

// TestClassifyModelRoles_WithTools — проверяет классификацию модели с поддержкой инструментов.
// Ожидаемое поведение: модель 8B с tools подходит для admin.
func TestClassifyModelRoles_WithTools(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "llama",
		ParameterSize: "8B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("llama3.1:8b", true, details)

	hasAdmin := false
	for _, r := range info.SuitableRoles {
		if r == "admin" {
			hasAdmin = true
		}
	}

	if !hasAdmin {
		t.Error("модель 8B с tools должна подходить для admin")
	}
	if len(info.SuitableRoles) != 1 || info.SuitableRoles[0] != "admin" {
		t.Error("единственная роль должна быть admin")
	}
}

// TestClassifyModelRoles_WithoutTools — проверяет классификацию модели без поддержки инструментов.
// Ожидаемое поведение: модель без tools всё равно получает роль admin,
// но с примечанием об ограничениях.
func TestClassifyModelRoles_WithoutTools(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "phi",
		ParameterSize: "3B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("phi3:mini", false, details)

	hasAdmin := false
	for _, r := range info.SuitableRoles {
		if r == "admin" {
			hasAdmin = true
		}
	}

	if !hasAdmin {
		t.Error("модель без tools всё равно должна получить роль admin")
	}

	note, ok := info.RoleNotes["admin"]
	if !ok {
		t.Error("должно быть примечание для роли admin")
	}
	if ok && !strings.Contains(note, "Ограниченно") {
		t.Error("примечание должно содержать информацию об ограничениях")
	}
}

// TestClassifyModelRoles_CodeModel — проверяет классификацию модели для кодирования.
// Ожидаемое поведение: модель с "coder" в имени получает роль admin с пометкой о специализации на коде.
func TestClassifyModelRoles_CodeModel(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "qwen2",
		ParameterSize: "7B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("qwen2.5-coder:7b", true, details)

	hasAdmin := false
	for _, r := range info.SuitableRoles {
		if r == "admin" {
			hasAdmin = true
		}
	}

	if !hasAdmin {
		t.Error("модель с coder в имени должна получить роль admin")
	}

	note := info.RoleNotes["admin"]
	if !strings.Contains(note, "код") {
		t.Error("примечание должно указывать на специализацию на коде")
	}
}

// TestClassifyModelRoles_LargeModel — проверяет классификацию большой модели (70B).
// Ожидаемое поведение: большая модель должна иметь примечание для роли admin.
func TestClassifyModelRoles_LargeModel(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "llama",
		ParameterSize: "70B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("llama3.1:70b", true, details)

	if note, ok := info.RoleNotes["admin"]; !ok || note == "" {
		t.Error("большая модель должна иметь примечание для роли admin")
	}
}

// TestClassifyModelRoles_UnknownSize — проверяет классификацию модели с неизвестным размером.
// Ожидаемое поведение: модель с неизвестным размером (0) и tools
// всё равно должна подходить для admin.
func TestClassifyModelRoles_UnknownSize(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "unknown",
		ParameterSize: "",
		Quantization:  "",
	}

	info := ClassifyModelRoles("custom-model", true, details)

	hasAdmin := false
	for _, r := range info.SuitableRoles {
		if r == "admin" {
			hasAdmin = true
		}
	}

	if !hasAdmin {
		t.Error("модель с неизвестным размером (0) и tools должна подходить для admin")
	}
}
