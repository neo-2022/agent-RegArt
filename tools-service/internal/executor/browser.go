// Пакет executor — модуль взаимодействия с браузером.
// Предоставляет функции для открытия URL в браузере пользователя,
// получения содержимого веб-страниц и взаимодействия с AI-чатами.
// Используется Агентом-Админом для работы с интернет-ресурсами.
package executor

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// OpenURL — открывает URL в браузере пользователя через xdg-open.
// Работает на Linux (использует xdg-open для определения браузера по умолчанию).
// Параметры:
//   - url: полный URL для открытия (например, "https://chat.openai.com")
//
// Возвращает ошибку, если xdg-open не найден или URL некорректен.
func OpenURL(url string) error {
	if url == "" {
		return fmt.Errorf("URL не может быть пустым")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	cmd := exec.Command("xdg-open", url)
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
func FetchURL(url string) (int, string, error) {
	if url == "" {
		return 0, "", fmt.Errorf("URL не может быть пустым")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка создания запроса: %w", err)
	}
	req.Header.Set("User-Agent", "OpenClaw-Memory/1.0 AdminAgent")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	// Ограничиваем чтение 100 КБ
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
func SendToAIChat(url, payload, contentType string) (int, string, error) {
	if url == "" {
		return 0, "", fmt.Errorf("URL не может быть пустым")
	}
	if contentType == "" {
		contentType = "application/json"
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
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
