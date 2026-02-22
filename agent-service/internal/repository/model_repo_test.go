package repository

import (
	"testing"
)

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
				t.Errorf("parseParamSize(%q) = %f, want %f", tc.input, result, tc.expected)
			}
		})
	}
}

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
				t.Errorf("isCodeModel(%q, %q) = %v, want %v", tc.modelName, tc.family, result, tc.expected)
			}
		})
	}
}

func TestClassifyModelRoles_WithTools(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "llama",
		ParameterSize: "8B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("llama3.1:8b", true, details)

	hasAdmin := false
	hasNovice := false
	for _, r := range info.SuitableRoles {
		if r == "admin" {
			hasAdmin = true
		}
		if r == "novice" {
			hasNovice = true
		}
	}

	if !hasAdmin {
		t.Error("8B model with tools should be suitable for admin")
	}
	if !hasNovice {
		t.Error("any model should be suitable for novice")
	}
}

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

	if hasAdmin {
		t.Error("model without tools should NOT be suitable for admin")
	}

	if _, ok := info.RoleNotes["admin"]; !ok {
		t.Error("should have a note explaining why admin is not suitable")
	}
}

func TestClassifyModelRoles_CodeModel(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "qwen2",
		ParameterSize: "7B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("qwen2.5-coder:7b", true, details)

	hasCoder := false
	for _, r := range info.SuitableRoles {
		if r == "coder" {
			hasCoder = true
		}
	}

	if !hasCoder {
		t.Error("code model with tools should be suitable for coder")
	}
}

func TestClassifyModelRoles_LargeModel(t *testing.T) {
	details := OllamaModelDetails{
		Family:        "llama",
		ParameterSize: "70B",
		Quantization:  "Q4_0",
	}

	info := ClassifyModelRoles("llama3.1:70b", true, details)

	if note, ok := info.RoleNotes["admin"]; !ok || note == "" {
		t.Error("large model should have admin role note")
	}
}

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
		t.Error("unknown size (0) with tools should still be suitable for admin")
	}
}
