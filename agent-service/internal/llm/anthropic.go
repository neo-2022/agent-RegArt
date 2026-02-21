package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// AnthropicProvider — провайдер для облачных моделей Anthropic (Claude).
// Поддерживает модели семейства Claude 3 и Claude 3.5 (Sonnet, Haiku, Opus).
// Реализует поддержку tool calling через блоки контента типа "tool_use".
// Авторизация через заголовок x-api-key.
//
// Особенности API Anthropic по сравнению с OpenAI:
//   - Системный промпт передаётся отдельным полем "system", а не сообщением
//   - Ответ содержит массив блоков контента (text + tool_use), а не одно сообщение
//   - Версия API указывается в заголовке anthropic-version
type AnthropicProvider struct {
	APIKey  string       // API-ключ Anthropic (формат: sk-ant-...)
	BaseURL string       // Базовый URL API (по умолчанию https://api.anthropic.com)
	HTTP    *http.Client // HTTP-клиент для выполнения запросов
}

// NewAnthropicProvider — создаёт новый экземпляр AnthropicProvider.
// Если baseURL пустой, используется официальный API Anthropic.
func NewAnthropicProvider(apiKey, baseURL string) *AnthropicProvider {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		APIKey:  apiKey,
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// Name — возвращает имя провайдера ("anthropic").
func (p *AnthropicProvider) Name() string { return "anthropic" }

// anthropicRequest — структура запроса к Anthropic Messages API.
// Формат соответствует документации: https://docs.anthropic.com/en/docs/messages
// Системный промпт передаётся отдельным полем, а не как сообщение с ролью "system".
type anthropicRequest struct {
	Model     string             `json:"model"`            // Имя модели (claude-sonnet-4-20250514 и т.д.)
	MaxTokens int                `json:"max_tokens"`       // Максимальное количество токенов в ответе
	System    string             `json:"system,omitempty"` // Системный промпт (отдельно от сообщений)
	Messages  []anthropicMessage `json:"messages"`         // Массив сообщений диалога
	Tools     []anthropicTool    `json:"tools,omitempty"`  // Доступные инструменты
}

// anthropicMessage — сообщение в формате Anthropic API.
// Поддерживает роли: user и assistant (system передаётся отдельно).
type anthropicMessage struct {
	Role    string `json:"role"`    // Роль: user или assistant
	Content string `json:"content"` // Текст сообщения
}

// anthropicTool — описание инструмента в формате Anthropic.
// В отличие от OpenAI, Anthropic использует input_schema вместо parameters.
type anthropicTool struct {
	Name        string `json:"name"`         // Имя инструмента
	Description string `json:"description"`  // Описание для модели
	InputSchema any    `json:"input_schema"` // JSON Schema входных параметров
}

// anthropicResponse — структура ответа от Anthropic Messages API.
// Ответ содержит массив блоков контента, каждый из которых может быть
// текстом ("text") или вызовом инструмента ("tool_use").
type anthropicResponse struct {
	ID      string `json:"id"`   // Уникальный ID ответа
	Type    string `json:"type"` // Тип ответа (всегда "message")
	Role    string `json:"role"` // Роль (всегда "assistant")
	Content []struct {
		Type  string          `json:"type"`            // Тип блока: "text" или "tool_use"
		Text  string          `json:"text,omitempty"`  // Текст ответа (для блока "text")
		ID    string          `json:"id,omitempty"`    // ID вызова инструмента (для "tool_use")
		Name  string          `json:"name,omitempty"`  // Имя инструмента (для "tool_use")
		Input json.RawMessage `json:"input,omitempty"` // Аргументы инструмента (для "tool_use")
	} `json:"content"`
	Model      string `json:"model"`       // Имя использованной модели
	StopReason string `json:"stop_reason"` // Причина остановки: end_turn, tool_use, max_tokens
	Error      *struct {
		Type    string `json:"type"`    // Тип ошибки
		Message string `json:"message"` // Текст ошибки
	} `json:"error,omitempty"`
}

// Chat — отправляет запрос к Anthropic Messages API.
// Конвертирует универсальный ChatRequest в формат Anthropic:
//   - Извлекает системный промпт из массива сообщений (роль "system")
//     и передаёт его отдельным полем
//   - Сообщения с ролью "tool" конвертируются в роль "user"
//     (Anthropic не поддерживает роль "tool" в сообщениях)
//   - Ответные блоки "tool_use" конвертируются в универсальный формат ToolCall
func (p *AnthropicProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ Anthropic не настроен")
	}

	// Извлекаем системный промпт из сообщений и конвертируем остальные
	var systemPrompt string
	var msgs []anthropicMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			// Anthropic требует системный промпт отдельным полем
			systemPrompt = m.Content
			continue
		}
		role := m.Role
		if role == "tool" {
			// Anthropic не поддерживает роль "tool", конвертируем в "user"
			role = "user"
		}
		msgs = append(msgs, anthropicMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	// Конвертируем инструменты в формат Anthropic (input_schema вместо parameters)
	var aTools []anthropicTool
	for _, t := range req.Tools {
		aTools = append(aTools, anthropicTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	// Формируем запрос с лимитом 4096 токенов на ответ
	aReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  msgs,
		Tools:     aTools,
	}

	data, err := json.Marshal(aReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	// Создаём HTTP-запрос с авторизацией через x-api-key
	// и указанием версии API через anthropic-version
	httpReq, err := http.NewRequest("POST", p.BaseURL+"/v1/messages", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к Anthropic: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	var aResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&aResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if aResp.Error != nil {
		return nil, fmt.Errorf("ошибка Anthropic: %s", aResp.Error.Message)
	}

	// Обрабатываем блоки контента ответа:
	// "text" — текстовый ответ, "tool_use" — вызов инструмента
	var content string
	var toolCalls []ToolCall
	for _, block := range aResp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			// Конвертируем вызов инструмента Anthropic в универсальный формат
			toolCalls = append(toolCalls, ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}

	return &ChatResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Model:     aResp.Model,
	}, nil
}

// ListModels — возвращает список доступных моделей Anthropic.
// Anthropic не предоставляет API для получения списка моделей,
// поэтому используется актуальный список из документации:
// https://docs.anthropic.com/en/docs/about-claude/models
func (p *AnthropicProvider) ListModels() ([]string, error) {
	return []string{
		"claude-sonnet-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
	}, nil
}

// ListModelsDetailed — возвращает детальную информацию о моделях Anthropic с ценами.
// Источник: https://docs.anthropic.com/en/docs/about-claude/pricing
// Все модели Claude платные. Haiku — самая доступная, Opus — самая дорогая.
// Claude Sonnet 4 — новейшая модель с оптимальным балансом цены и качества.
func (p *AnthropicProvider) ListModelsDetailed() ([]ModelDetail, error) {
	return []ModelDetail{
		{
			ID:             "claude-sonnet-4-20250514",
			IsAvailable:    false,
			PricingInfo:    "$3/1M токенов (вход), $15/1M (выход)",
			ActivationHint: "Пополните баланс на console.anthropic.com/settings/billing",
		},
		{
			ID:             "claude-3-5-sonnet-20241022",
			IsAvailable:    false,
			PricingInfo:    "$3/1M токенов (вход), $15/1M (выход)",
			ActivationHint: "Пополните баланс на console.anthropic.com/settings/billing",
		},
		{
			ID:             "claude-3-5-haiku-20241022",
			IsAvailable:    false,
			PricingInfo:    "$0.80/1M токенов (вход), $4/1M (выход)",
			ActivationHint: "Пополните баланс на console.anthropic.com/settings/billing. Haiku — самая доступная модель Claude.",
		},
		{
			ID:             "claude-3-opus-20240229",
			IsAvailable:    false,
			PricingInfo:    "$15/1M токенов (вход), $75/1M (выход)",
			ActivationHint: "Пополните баланс на console.anthropic.com/settings/billing. Opus — самая мощная модель Claude.",
		},
	}, nil
}
