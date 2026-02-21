package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider — провайдер для облачных моделей OpenAI.
// Поддерживает модели семейства GPT-4, GPT-4o, GPT-3.5-turbo, а также o1 и o3.
// Реализует полную поддержку tool calling (вызов инструментов).
// Авторизация через API-ключ в заголовке Authorization: Bearer.
// Базовый URL можно изменить для совместимости с прокси-сервисами.
type OpenAIProvider struct {
	APIKey  string       // API-ключ OpenAI (формат: sk-...)
	BaseURL string       // Базовый URL API (по умолчанию https://api.openai.com/v1)
	HTTP    *http.Client // HTTP-клиент для выполнения запросов
}

// NewOpenAIProvider — создаёт новый экземпляр OpenAIProvider.
// Если baseURL пустой, используется официальный API OpenAI.
// Можно указать альтернативный URL для прокси или OpenAI-совместимых сервисов.
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIProvider{
		APIKey:  apiKey,
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// Name — возвращает имя провайдера ("openai").
func (p *OpenAIProvider) Name() string { return "openai" }

// openaiRequest — структура запроса к OpenAI Chat Completions API.
// Формат соответствует документации: https://platform.openai.com/docs/api-reference/chat
type openaiRequest struct {
	Model    string          `json:"model"`           // Имя модели (gpt-4o, gpt-4-turbo и т.д.)
	Messages []openaiMessage `json:"messages"`        // Массив сообщений диалога
	Tools    []openaiTool    `json:"tools,omitempty"` // Доступные инструменты для вызова
	Stream   bool            `json:"stream"`          // Режим стриминга (пока не используется)
}

// openaiMessage — сообщение в формате OpenAI API.
// Поддерживает роли: system, user, assistant, tool.
type openaiMessage struct {
	Role       string           `json:"role"`                   // Роль отправителя сообщения
	Content    string           `json:"content"`                // Текст сообщения
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`   // Вызовы инструментов (для роли assistant)
	ToolCallID string           `json:"tool_call_id,omitempty"` // ID вызова инструмента (для роли tool)
}

// openaiTool — описание инструмента в формате OpenAI.
// OpenAI поддерживает только тип "function".
type openaiTool struct {
	Type     string             `json:"type"`     // Тип инструмента (всегда "function")
	Function openaiToolFunction `json:"function"` // Описание функции
}

// openaiToolFunction — описание функции инструмента.
// Содержит имя, описание и JSON Schema параметров.
type openaiToolFunction struct {
	Name        string `json:"name"`        // Имя функции
	Description string `json:"description"` // Описание функции для модели
	Parameters  any    `json:"parameters"`  // JSON Schema параметров функции
}

// openaiToolCall — вызов инструмента, запрошенный моделью.
// Содержит уникальный ID вызова и информацию о функции.
type openaiToolCall struct {
	ID       string                 `json:"id"`       // Уникальный ID вызова инструмента
	Type     string                 `json:"type"`     // Тип вызова (всегда "function")
	Function openaiToolCallFunction `json:"function"` // Имя функции и аргументы
}

// openaiToolCallFunction — имя и аргументы вызванной функции.
// Arguments содержит JSON-строку с аргументами.
type openaiToolCallFunction struct {
	Name      string `json:"name"`      // Имя вызванной функции
	Arguments string `json:"arguments"` // JSON-строка с аргументами вызова
}

// openaiResponse — структура ответа от OpenAI Chat Completions API.
// Содержит массив вариантов ответа (choices), где каждый вариант
// включает сообщение и причину остановки генерации.
type openaiResponse struct {
	ID      string `json:"id"` // Уникальный ID ответа
	Choices []struct {
		Message struct {
			Role      string           `json:"role"`                 // Роль (всегда "assistant")
			Content   string           `json:"content"`              // Текст ответа
			ToolCalls []openaiToolCall `json:"tool_calls,omitempty"` // Вызовы инструментов
		} `json:"message"`
		FinishReason string `json:"finish_reason"` // Причина остановки: stop, tool_calls, length
	} `json:"choices"`
	Model string `json:"model"` // Имя использованной модели
	Error *struct {
		Message string `json:"message"` // Текст ошибки (если есть)
	} `json:"error,omitempty"`
}

// Chat — отправляет запрос к OpenAI Chat Completions API.
// Конвертирует универсальный ChatRequest в формат OpenAI, отправляет запрос,
// парсит ответ и конвертирует обратно в ChatResponse.
// Поддерживает tool calling — вызовы инструментов конвертируются
// из формата OpenAI в универсальный формат ToolCall.
func (p *OpenAIProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ OpenAI не настроен")
	}

	// Конвертируем сообщения из универсального формата в формат OpenAI
	msgs := make([]openaiMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
	}

	// Конвертируем инструменты из универсального формата в формат OpenAI
	var oaiTools []openaiTool
	for _, t := range req.Tools {
		oaiTools = append(oaiTools, openaiTool{
			Type: t.Type,
			Function: openaiToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	// Формируем запрос к API
	oaiReq := openaiRequest{
		Model:    req.Model,
		Messages: msgs,
		Tools:    oaiTools,
		Stream:   false,
	}

	data, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	// Создаём HTTP-запрос с авторизацией через Bearer-токен
	httpReq, err := http.NewRequest("POST", p.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	// Отправляем запрос
	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к OpenAI: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	// Парсим ответ от OpenAI
	var oaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if oaiResp.Error != nil {
		return nil, fmt.Errorf("ошибка OpenAI: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI вернул пустой ответ")
	}

	// Конвертируем вызовы инструментов из формата OpenAI в универсальный
	choice := oaiResp.Choices[0]
	var toolCalls []ToolCall
	for _, tc := range choice.Message.ToolCalls {
		toolCalls = append(toolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: json.RawMessage(tc.Function.Arguments),
			},
		})
	}

	return &ChatResponse{
		Content:   choice.Message.Content,
		ToolCalls: toolCalls,
		Model:     oaiResp.Model,
	}, nil
}

// ListModels — получает список доступных моделей OpenAI через GET /models.
// Фильтрует результат, оставляя только GPT-модели (gpt-4*, gpt-3.5*, o1*, o3*),
// так как OpenAI API возвращает все модели, включая embedding и whisper.
func (p *OpenAIProvider) ListModels() ([]string, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ OpenAI не настроен")
	}

	httpReq, err := http.NewRequest("GET", p.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка моделей: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Фильтруем только GPT и reasoning модели
	var models []string
	for _, m := range result.Data {
		if isGPTModel(m.ID) {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// isGPTModel — проверяет, является ли модель GPT или reasoning моделью.
// Возвращает true для моделей с префиксами: gpt-4, gpt-3.5, o1, o3.
// Используется для фильтрации списка моделей OpenAI,
// чтобы не показывать embedding, whisper и другие служебные модели.
func isGPTModel(id string) bool {
	prefixes := []string{"gpt-4", "gpt-3.5", "o1", "o3"}
	for _, p := range prefixes {
		if len(id) >= len(p) && id[:len(p)] == p {
			return true
		}
	}
	return false
}

// ListModelsDetailed — возвращает детальную информацию о моделях OpenAI с ценами.
// Получает реальный список моделей через GET /models API, затем обогащает
// информацией о ценах из документации OpenAI: https://openai.com/api/pricing
// Все модели OpenAI платные, но GPT-3.5-turbo дешевле GPT-4.
func (p *OpenAIProvider) ListModelsDetailed() ([]ModelDetail, error) {
	names, err := p.ListModels()
	if err != nil {
		return nil, err
	}
	var details []ModelDetail
	for _, name := range names {
		d := ModelDetail{
			ID:             name,
			IsAvailable:    false,
			PricingInfo:    "Платная",
			ActivationHint: "Пополните баланс на platform.openai.com/settings/organization/billing",
		}
		switch {
		case len(name) >= 7 && name[:7] == "gpt-3.5":
			d.PricingInfo = "$0.50/1M токенов (вход), $1.50/1M (выход)"
		case len(name) >= 8 && name[:8] == "gpt-4o-m":
			d.PricingInfo = "$0.15/1M токенов (вход), $0.60/1M (выход)"
			d.IsAvailable = false
		case len(name) >= 6 && name[:6] == "gpt-4o":
			d.PricingInfo = "$2.50/1M токенов (вход), $10/1M (выход)"
		case len(name) >= 5 && name[:5] == "gpt-4":
			d.PricingInfo = "$30/1M токенов (вход), $60/1M (выход)"
		case len(name) >= 2 && name[:2] == "o1":
			d.PricingInfo = "$15/1M токенов (вход), $60/1M (выход)"
		case len(name) >= 2 && name[:2] == "o3":
			d.PricingInfo = "$10/1M токенов (вход), $40/1M (выход)"
		}
		details = append(details, d)
	}
	return details, nil
}
