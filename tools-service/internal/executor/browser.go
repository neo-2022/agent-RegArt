// Пакет executor — модуль взаимодействия с браузером.
// Предоставляет функции для открытия URL в браузере пользователя,
// получения содержимого веб-страниц и взаимодействия с AI-чатами.
// Используется Агентом-Админом для работы с интернет-ресурсами.
package executor

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxURLLength = 2048

func allowPrivateURLs() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("BROWSER_ALLOW_PRIVATE_URLS"))) == "true"
}

func normalizeURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("URL не может быть пустым")
	}
	if len(raw) > maxURLLength {
		return nil, fmt.Errorf("URL слишком длинный")
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("некорректный URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("поддерживаются только http/https URL")
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("некорректный URL: отсутствует host")
	}
	return u, nil
}

func isPrivateHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" {
		return true
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	if ip.String() == "169.254.169.254" {
		return true
	}
	return false
}

func validateExternalURL(raw string) (string, error) {
	u, err := normalizeURL(raw)
	if err != nil {
		return "", err
	}

	if !allowPrivateURLs() {
		if isPrivateHost(u.Hostname()) {
			return "", fmt.Errorf("доступ к локальным/приватным адресам запрещён")
		}
	}

	return u.String(), nil
}

// OpenURL — открывает URL в браузере пользователя через xdg-open.
// Работает на Linux (использует xdg-open для определения браузера по умолчанию).
// Параметры:
//   - url: полный URL для открытия (например, "https://chat.openai.com")
//
// Возвращает ошибку, если xdg-open не найден или URL некорректен.
func OpenURL(urlStr string) error {
	validated, err := validateExternalURL(urlStr)
	if err != nil {
		return err
	}
	cmd := exec.Command("xdg-open", validated)
	return cmd.Start()
}

// FetchURL — получает текстовое содержимое веб-страницы по URL.
// Делает HTTP GET-запрос и возвращает тело ответа как строку.
// Таймаут запроса — 30 секунд. Максимальный размер ответа — 100 КБ.
// Параметры:
//   - url: полный URL страницы
//
// Возвращает:
//   - statusCode: HTTP-код ответа (200, 404, 500 и т.д.)
//   - body: текстовое содержимое ответа (обрезанное до 100 КБ)
//   - error: ошибка, если запрос не удался
func FetchURL(urlStr string) (int, string, error) {
	validated, err := validateExternalURL(urlStr)
	if err != nil {
		return 0, "", err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", validated, nil)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("User-Agent", "OpenClaw-Memory/1.0 AdminAgent")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 100*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	return resp.StatusCode, string(body), nil
}

// SendToAIChat — отправляет POST-запрос к AI-чату по указанному URL.
// Используется Админом для взаимодействия с внешними AI-сервисами
// (например, ChatGPT, GigaChat веб-интерфейс и др.).
// Параметры:
//   - url: URL API эндпоинта AI-чата
//   - payload: тело запроса (обычно JSON)
//   - contentType: тип содержимого (по умолчанию application/json)
//
// Возвращает:
//   - statusCode: HTTP-код ответа
//   - body: текстовое содержимое ответа
//   - error: ошибка, если запрос не удался
func SendToAIChat(urlStr, payload, contentType string) (int, string, error) {
	validated, err := validateExternalURL(urlStr)
	if err != nil {
		return 0, "", err
	}
	if contentType == "" {
		contentType = "application/json"
	}

	client := &http.Client{Timeout: 60 * time.Second}

	req, err := http.NewRequest("POST", validated, strings.NewReader(payload))
	if err != nil {
		return 0, "", fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "OpenClaw-Memory/1.0 AdminAgent")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 100*1024)
	body, err := io.ReadAll(limited)
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	return resp.StatusCode, string(body), nil
}
