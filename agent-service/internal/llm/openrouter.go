// Файл openrouter.go — провайдер OpenRouter для доступа к сотням облачных LLM-моделей.
//
// OpenRouter (https://openrouter.ai) — это агрегатор LLM-моделей, предоставляющий
// единый OpenAI-совместимый API для доступа к моделям от разных провайдеров:
//   - OpenAI (GPT-4, GPT-4o, o1, o3)
//   - Anthropic (Claude Sonnet, Haiku, Opus)
//   - Google (Gemini Pro, Gemini Ultra)
//   - Meta (Llama 3.1, Llama 3.2)
//   - Mistral (Mistral Large, Mixtral)
//   - DeepSeek (DeepSeek-V3, DeepSeek-R1)
//   - И множество других (100+ моделей)
//
// Преимущества OpenRouter:
//   - Один API-ключ для всех моделей
//   - Автоматический fallback при недоступности модели
//   - Поддержка tool calling (для моделей, которые его поддерживают)
//   - Единый формат запросов/ответов (OpenAI-совместимый)
//
// Авторизация: Bearer-токен в заголовке Authorization.
// Базовый URL: https://openrouter.ai/api/v1
// Формат API: полностью совместим с OpenAI Chat Completions API.
package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenRouterProvider — провайдер для доступа к моделям через OpenRouter.
// Реализует интерфейс ChatProvider. Использует OpenAI-совместимый формат API,
// что позволяет переиспользовать те же структуры запросов/ответов.
//
// Поля:
//   - APIKey: API-ключ OpenRouter (формат: sk-or-v1-...)
//   - BaseURL: базовый URL API (по умолчанию https://openrouter.ai/api/v1)
//   - HTTP: HTTP-клиент для выполнения запросов
//   - AppName: название приложения (отправляется в заголовке X-Title для аналитики OpenRouter)
type OpenRouterProvider struct {
	APIKey  string       // API-ключ OpenRouter
	BaseURL string       // Базовый URL API
	HTTP    *http.Client // HTTP-клиент
	AppName string       // Название приложения для аналитики OpenRouter
}

// NewOpenRouterProvider — создаёт новый экземпляр OpenRouterProvider.
// Если baseURL пустой, используется официальный API OpenRouter.
// AppName по умолчанию "AgentCore-NG" — отображается в дашборде OpenRouter.
//
// Параметры:
//   - apiKey: API-ключ OpenRouter
//   - baseURL: базовый URL (пустая строка = https://openrouter.ai/api/v1)
//
// Возвращает: настроенный экземпляр OpenRouterProvider
func NewOpenRouterProvider(apiKey, baseURL string) *OpenRouterProvider {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterProvider{
		APIKey:  apiKey,
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
		AppName: "AgentCore-NG",
	}
}

// Name — возвращает имя провайдера ("openrouter").
// Используется как ключ в реестре провайдеров (Registry).
func (p *OpenRouterProvider) Name() string { return "openrouter" }

// openrouterRequest — структура запроса к OpenRouter Chat Completions API.
// Полностью совместима с форматом OpenAI, с дополнительными полями
// для маршрутизации и fallback-логики OpenRouter.
type openrouterRequest struct {
	Model    string          `json:"model"`           // Имя модели (например, "openai/gpt-4o", "anthropic/claude-sonnet-4-20250514")
	Messages []openaiMessage `json:"messages"`        // Массив сообщений диалога (формат OpenAI)
	Tools    []openaiTool    `json:"tools,omitempty"` // Доступные инструменты для tool calling
	Stream   bool            `json:"stream"`          // Режим стриминга
}

// openrouterResponse — структура ответа от OpenRouter API.
// Совместима с форматом OpenAI, с дополнительной информацией
// об использовании токенов и маршрутизации.
type openrouterResponse struct {
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
		Code    int    `json:"code"`    // Код ошибки
	} `json:"error,omitempty"`
}

// Chat — отправляет запрос к OpenRouter Chat Completions API.
// Конвертирует универсальный ChatRequest в формат OpenRouter (OpenAI-совместимый),
// отправляет запрос и конвертирует ответ обратно в ChatResponse.
//
// Особенности OpenRouter:
//   - Модели указываются в формате "провайдер/модель" (например, "openai/gpt-4o")
//   - Поддерживает tool calling для совместимых моделей
//   - Добавляет заголовки X-Title и HTTP-Referer для аналитики
//
// Параметры:
//   - req: универсальный запрос ChatRequest
//
// Возвращает:
//   - *ChatResponse: ответ от модели (текст + вызовы инструментов)
//   - error: ошибка запроса, авторизации или декодирования
func (p *OpenRouterProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ OpenRouter не настроен")
	}

	// Конвертируем сообщения из универсального формата в формат OpenAI
	msgs := make([]openaiMessage, len(req.Messages))
	for i, m := range req.Messages {
		msg := openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			msg.ToolCalls = append(msg.ToolCalls, openaiToolCall{
				ID:   tc.ID,
				Type: tc.Type,
				Function: openaiToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: string(tc.Function.Arguments),
				},
			})
		}
		msgs[i] = msg
	}

	// Конвертируем инструменты из универсального формата
	var orTools []openaiTool
	for _, t := range req.Tools {
		orTools = append(orTools, openaiTool{
			Type: t.Type,
			Function: openaiToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}

	// Формируем запрос к API
	orReq := openrouterRequest{
		Model:    req.Model,
		Messages: msgs,
		Tools:    orTools,
		Stream:   false,
	}

	data, err := json.Marshal(orReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса OpenRouter: %w", err)
	}

	// Создаём HTTP-запрос с авторизацией и метаданными OpenRouter
	httpReq, err := http.NewRequest("POST", p.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса OpenRouter: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	// Заголовки для аналитики OpenRouter (отображаются в дашборде)
	httpReq.Header.Set("X-Title", p.AppName)
	httpReq.Header.Set("HTTP-Referer", "https://github.com/neo-2022/openclaw-memory")

	// Отправляем запрос
	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	// Парсим ответ
	var orResp openrouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&orResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа OpenRouter: %w", err)
	}

	if orResp.Error != nil {
		return nil, fmt.Errorf("ошибка OpenRouter: %s (код %d)", orResp.Error.Message, orResp.Error.Code)
	}

	if len(orResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenRouter вернул пустой ответ")
	}

	// Конвертируем вызовы инструментов из формата OpenAI в универсальный
	choice := orResp.Choices[0]
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
		Model:     orResp.Model,
	}, nil
}

// openrouterModelData — структура модели из ответа OpenRouter GET /models.
// Содержит ID, имя и информацию о ценах (стоимость промпта и ответа).
type openrouterModelData struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pricing struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

// fetchModelsRaw — получает сырые данные о моделях из OpenRouter API.
// Возвращает массив openrouterModelData с ID, именами и ценами.
// Используется как ListModels(), так и ListModelsDetailed().
func (p *OpenRouterProvider) fetchModelsRaw() ([]openrouterModelData, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ OpenRouter не настроен")
	}

	httpReq, err := http.NewRequest("GET", p.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка моделей OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter /models вернул статус %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ошибка чтения ответа списка моделей OpenRouter: %w", err)
	}

	var result struct {
		Data []openrouterModelData `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("ошибка декодирования списка моделей OpenRouter: %w", err)
	}

	return result.Data, nil
}

// isPopularModel — проверяет, относится ли модель к популярным провайдерам.
// Фильтрует модели OpenRouter, оставляя только от известных провайдеров
// (OpenAI, Anthropic, Google, Meta, Mistral, DeepSeek, Qwen, Cohere).
func isPopularModel(id string) bool {
	popularPrefixes := []string{
		"openai/", "anthropic/", "google/", "meta-llama/",
		"mistralai/", "deepseek/", "qwen/", "cohere/",
	}
	for _, prefix := range popularPrefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

// ListModels — получает список доступных моделей OpenRouter через GET /models.
// Фильтрует по популярным провайдерам, ограничивает до 100 для UI.
func (p *OpenRouterProvider) ListModels() ([]string, error) {
	rawModels, err := p.fetchModelsRaw()
	if err != nil {
		return nil, err
	}

	var models []string
	for _, m := range rawModels {
		models = append(models, m.ID)
	}

	return models, nil
}

// ListModelsDetailed — получает детальную информацию о моделях OpenRouter с ценами.
// Запрашивает GET /models и парсит поле pricing для каждой модели.
// Модель считается бесплатной, если pricing.prompt == "0" и pricing.completion == "0".
// Для платных моделей формируется информация о стоимости и подсказка по активации.
func (p *OpenRouterProvider) ListModelsDetailed() ([]ModelDetail, error) {
	rawModels, err := p.fetchModelsRaw()
	if err != nil {
		return nil, err
	}

	var details []ModelDetail
	for _, m := range rawModels {
		isFree := m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"
		pricingInfo := "Бесплатно"
		activationHint := ""

		if !isFree {
			pricingInfo = fmt.Sprintf("Промпт: %s$/токен, Ответ: %s$/токен", m.Pricing.Prompt, m.Pricing.Completion)
			activationHint = "Пополните баланс на openrouter.ai/credits. Бесплатные модели отмечены ценой $0."
		}

		details = append(details, ModelDetail{
			ID:             m.ID,
			IsAvailable:    isFree,
			PricingInfo:    pricingInfo,
			ActivationHint: activationHint,
		})
	}

	return details, nil
}
