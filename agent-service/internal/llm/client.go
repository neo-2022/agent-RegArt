package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaRequest представляет запрос к Ollama API.
// Поле Options позволяет передать параметры генерации (num_ctx, temperature и др.).
type OllamaRequest struct {
	Model    string                 `json:"model"`
	Messages []Message              `json:"messages"`
	Stream   bool                   `json:"stream"`
	Tools    []Tool                 `json:"tools,omitempty"`   // описание инструментов для модели
	Options  map[string]interface{} `json:"options,omitempty"` // параметры генерации (num_ctx, temperature и др.)
}

// Message представляет одно сообщение в диалоге
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall представляет вызов инструмента от модели
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall используется для вызова инструмента (содержит аргументы)
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"` // может быть строкой или объектом
}

// Tool описывает доступный инструмент (для передачи модели)
type Tool struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition используется для описания инструмента
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"` // JSON Schema
}

// OllamaResponse представляет ответ от Ollama
type OllamaResponse struct {
	Model     string  `json:"model"`
	CreatedAt string  `json:"created_at"`
	Message   Message `json:"message"`
	Done      bool    `json:"done"`
}

// translateProviderError — переводит ошибки от облачных LLM-провайдеров на русский язык.
// Парсит JSON-тело ответа и извлекает сообщение об ошибке.
// Для известных HTTP-кодов добавляет понятное описание на русском.
func translateProviderError(statusCode int, body string) string {
	if strings.Contains(body, "<html") || strings.Contains(body, "<!DOCTYPE") {
		switch statusCode {
		case 403:
			return "Доступ заблокирован (Cloudflare/WAF). Возможно, ваш IP заблокирован провайдером. Попробуйте VPN или другой IP."
		case 503:
			return "Сервис провайдера временно недоступен. Попробуйте позже."
		default:
			return fmt.Sprintf("Провайдер вернул HTML-ответ (HTTP %d). Возможно, сервис недоступен или IP заблокирован.", statusCode)
		}
	}

	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	msg := body
	if json.Unmarshal([]byte(body), &parsed) == nil && parsed.Error.Message != "" {
		msg = parsed.Error.Message
	}

	lower := strings.ToLower(msg)
	switch statusCode {
	case 400:
		if strings.Contains(lower, "specified folder id") && strings.Contains(lower, "does not match") {
			return "Ошибка YandexGPT: Folder ID не соответствует папке сервисного аккаунта. Проверьте Folder ID в настройках провайдера. " + msg
		}
		return "Некорректный запрос (400). Проверьте параметры провайдера. " + msg
	case 401:
		return "Неверный API-ключ. Проверьте настройки провайдера. " + msg
	case 402:
		return "Недостаточно средств на балансе. Пополните баланс или используйте бесплатную модель. " + msg
	case 403:
		return "Доступ запрещён. Проверьте разрешения API-ключа. " + msg
	case 429:
		return "Превышен лимит запросов (rate limit). Подождите и попробуйте снова. " + msg
	case 500:
		return "Внутренняя ошибка сервера провайдера. Попробуйте позже. " + msg
	case 502, 503:
		return "Сервер провайдера временно недоступен. Попробуйте позже. " + msg
	case 504:
		return "Таймаут сервера провайдера. Попробуйте более лёгкую модель. " + msg
	}
	return msg
}

// TranslateLLMError — публичная обёртка для перевода ошибок LLM на русский.
// Анализирует текст ошибки и заменяет типичные английские сообщения на русские.
func TranslateLLMError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "specified folder id") && strings.Contains(lower, "does not match"):
		return "Ошибка YandexGPT: Folder ID не соответствует папке сервисного аккаунта. Проверьте Folder ID в настройках провайдера. " + errText
	case strings.Contains(lower, "rate limit") || strings.Contains(lower, "429"):
		return "Превышен лимит запросов. Подождите и попробуйте снова. " + errText
	case strings.Contains(lower, "insufficient") || strings.Contains(lower, "402") || strings.Contains(lower, "payment"):
		return "Недостаточно средств на балансе провайдера. " + errText
	case strings.Contains(lower, "unauthorized") || strings.Contains(lower, "401") || strings.Contains(lower, "invalid api key"):
		return "Неверный API-ключ. Проверьте настройки провайдера. " + errText
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "Таймаут запроса к провайдеру. Модель не ответила вовремя. Попробуйте более лёгкую модель."
	case strings.Contains(lower, "connection refused"):
		return "Не удалось подключиться к провайдеру. Проверьте, что сервис запущен."
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "dns"):
		return "Не удалось найти сервер провайдера. Проверьте URL в настройках."
	}
	return errText
}

// Client представляет клиент для работы с Ollama
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient создаёт нового клиента с указанным URL (по умолчанию http://localhost:11434)
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{},
	}
}

// Chat отправляет запрос к Ollama и возвращает ответ
func (c *Client) Chat(req *OllamaRequest) (*OllamaResponse, error) {
	url := c.BaseURL + "/api/chat"
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTP.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	if req.Stream {
		dec := json.NewDecoder(resp.Body)
		var combined OllamaResponse
		var content strings.Builder

		for {
			var chunk OllamaResponse
			if err := dec.Decode(&chunk); err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("failed to decode stream chunk: %w", err)
			}

			if chunk.Model != "" {
				combined.Model = chunk.Model
			}
			if chunk.CreatedAt != "" {
				combined.CreatedAt = chunk.CreatedAt
			}
			if chunk.Message.Role != "" {
				combined.Message.Role = chunk.Message.Role
			}
			if chunk.Message.Content != "" {
				content.WriteString(chunk.Message.Content)
			}
			if len(chunk.Message.ToolCalls) > 0 {
				combined.Message.ToolCalls = chunk.Message.ToolCalls
			}
			combined.Done = chunk.Done
			if chunk.Done {
				break
			}
		}

		combined.Message.Content = content.String()
		return &combined, nil
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &ollamaResp, nil
}
