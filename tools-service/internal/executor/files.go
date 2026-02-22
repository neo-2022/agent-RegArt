// Package executor — работа с файловой системой через API.
//
// Предоставляет безопасные функции для чтения, записи, просмотра
// и удаления файлов с многоуровневой защитой:
//   - Запрещённые системные директории (ForbiddenPaths)
//   - Разрешённые системные файлы (AllowedSystemFiles)
//   - Защита от path traversal (проход через ..)
//   - Ограничение максимального размера файла (MaxFileSize = 10 МБ)
//   - Поддержка ~ как домашней директории пользователя
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ForbiddenPaths — системные директории, доступ к которым запрещён через API.
// Защищает критические файлы системы от случайного или намеренного повреждения.
var ForbiddenPaths = []string{
	"/etc/shadow", "/etc/passwd", "/etc/sudoers",
	"/proc", "/sys", "/dev",
	"/boot", "/sbin", "/usr/sbin",
}

// AllowedSystemFiles — исключения из запрещённых путей.
// Эти файлы из /proc разрешены для чтения (информация о CPU и памяти).
var AllowedSystemFiles = map[string]struct{}{
	"/proc/cpuinfo": {},
	"/proc/meminfo": {},
}

// MaxFileSize — максимальный размер файла для чтения/записи (10 МБ).
const MaxFileSize = 10 * 1024 * 1024

// validatePath — проверяет путь на безопасность.
// Выполняет:
//  1. Разрешение ~ в домашнюю директорию
//  2. Нормализацию пути (filepath.Clean)
//  3. Проверку на path traversal (..)
//  4. Проверку по белому списку системных файлов
//  5. Проверку по чёрному списку запрещённых директорий
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

// ReadFile — безопасное чтение файла по указанному пути.
// Проверяет путь на безопасность и ограничивает размер файла до MaxFileSize.
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

// WriteFile — безопасная запись файла по указанному пути.
// Проверяет путь и размер содержимого. Автоматически создаёт родительские директории.
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

// ListDirectory — получение списка файлов и папок в указанной директории.
// Возвращает только имена (без полных путей).
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

// DeleteFile — безопасное удаление файла по указанному пути.
// Перед удалением проверяет путь на безопасность.
func DeleteFile(path string) error {
	cleanPath, err := validatePath(path)
	if err != nil {
		return err
	}
	return os.Remove(cleanPath)
}

// resolveHomePath — заменяет ~ на домашнюю директорию текущего пользователя.
// Если путь пустой или равен "~" — возвращает домашнюю директорию.
// Если начинается с "~/" — заменяет префикс на домашнюю директорию.
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
