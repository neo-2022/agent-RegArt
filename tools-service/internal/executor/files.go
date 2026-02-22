package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ForbiddenPaths — системные директории, доступ к которым запрещён через API.
var ForbiddenPaths = []string{
	"/etc/shadow", "/etc/passwd", "/etc/sudoers",
	"/proc", "/sys", "/dev",
	"/boot", "/sbin", "/usr/sbin",
}

var AllowedSystemFiles = map[string]struct{}{
	"/proc/cpuinfo": {},
	"/proc/meminfo": {},
}

// MaxFileSize — максимальный размер файла для чтения/записи (10 МБ).
const MaxFileSize = 10 * 1024 * 1024

// validatePath проверяет путь на path traversal и запрещённые директории.
func validatePath(path string) (string, error) {
	path = resolveHomePath(path)
	cleanPath := filepath.Clean(path)

	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("path traversal запрещён: %s", path)
	}

	if _, ok := AllowedSystemFiles[cleanPath]; ok {
		return cleanPath, nil
	}

	for _, forbidden := range ForbiddenPaths {
		if strings.HasPrefix(cleanPath, forbidden) {
			return "", fmt.Errorf("доступ к %s запрещён", forbidden)
		}
	}

	return cleanPath, nil
}

func ReadFile(path string) (string, error) {
	cleanPath, err := validatePath(path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", err
	}
	if info.Size() > MaxFileSize {
		return "", fmt.Errorf("файл слишком большой: %d байт (макс %d)", info.Size(), MaxFileSize)
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func WriteFile(path, content string) error {
	cleanPath, err := validatePath(path)
	if err != nil {
		return err
	}

	if len(content) > MaxFileSize {
		return fmt.Errorf("содержимое слишком большое: %d байт (макс %d)", len(content), MaxFileSize)
	}

	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(cleanPath, []byte(content), 0644)
}

func ListDirectory(path string) ([]string, error) {
	cleanPath, err := validatePath(path)
	if err != nil {
		return nil, err
	}

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
	cleanPath, err := validatePath(path)
	if err != nil {
		return err
	}
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
