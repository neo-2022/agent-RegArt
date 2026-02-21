package executor

import (
	"os"
	"path/filepath"
	"strings"
)

func ReadFile(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteFile(path, content string) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(cleanPath, []byte(content), 0644)
}

func ListDirectory(path string) ([]string, error) {
	path = resolveHomePath(path)
	cleanPath := filepath.Clean(path)
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func DeleteFile(path string) error {
	cleanPath := filepath.Clean(path)
	return os.Remove(cleanPath)
}

func resolveHomePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" || path == "~/" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return "."
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
