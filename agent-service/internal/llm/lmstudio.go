// Файл lmstudio.go — провайдер LM Studio для запуска LLM-моделей локально.
//
// LM Studio (https://lmstudio.ai) — бесплатное десктопное приложение для запуска
// LLM-моделей на локальном компьютере пользователя. Поддерживает форматы GGUF, MLX
// и работает на CPU/GPU без облачных сервисов.
//
// Ключевые преимущества:
//   - Полностью бесплатный, без лимитов запросов и ограничений
//   - Работает офлайн, данные не покидают компьютер пользователя
//   - Поддерживает сотни моделей: Llama, Mistral, Qwen, Gemma, DeepSeek и др.
//   - OpenAI-совместимый API (тот же формат, что OpenRouter/Routeway)
//   - Встроенная поддержка tool calling для совместимых моделей
//   - Автоматическая загрузка/выгрузка моделей по запросу
//
// Авторизация: опциональный API-токен (по умолчанию не требуется).
// Базовый URL: http://localhost:1234/v1 (порт настраивается в LM Studio).
// Формат API: полностью совместим с OpenAI Chat Completions API.
//
// Отличие от Ollama:
//   - LM Studio имеет графический интерфейс для управления моделями
//   - Использует OpenAI-совместимый API (а не собственный формат Ollama)
//   - Поддерживает Anthropic-совместимый API дополнительно
//   - Может работать в headless-режиме как системный сервис
package llm

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// LMStudioProvider — провайдер для запуска моделей через LM Studio.
// Встраивает OpenRouterProvider, так как LM Studio полностью совместим
// с OpenAI Chat Completions API. Переопределяет Name() и ListModels()
// для корректной работы с локальным сервером.
//
// Особенности:
//   - API-ключ опционален (если не задан, используется заглушка "lm-studio")
//   - Модели именуются по формату LM Studio (например, "meta-llama-3.1-8b-instruct")
//   - ListModels() возвращает ВСЕ модели без фильтрации (в отличие от OpenRouter)
//   - Не требует интернета — всё работает локально
type LMStudioProvider struct {
	*OpenRouterProvider
}

// NewLMStudioProvider — создаёт новый экземпляр LMStudioProvider.
// Если baseURL пустой, используется стандартный адрес LM Studio: http://localhost:1234/v1
// Если apiKey пустой, используется заглушка "lm-studio" (LM Studio не требует ключ по умолчанию).
//
// Параметры:
//   - apiKey: API-токен LM Studio (пустая строка = авторизация не требуется)
//   - baseURL: базовый URL сервера LM Studio (пустая строка = http://localhost:1234/v1)
//
// Возвращает: настроенный экземпляр LMStudioProvider
func NewLMStudioProvider(apiKey, baseURL string) *LMStudioProvider {
	if baseURL == "" {
		baseURL = "http://localhost:1234/v1"
	}
	if apiKey == "" {
		apiKey = "lm-studio"
	}
	return &LMStudioProvider{
		OpenRouterProvider: &OpenRouterProvider{
			APIKey:  apiKey,
			BaseURL: baseURL,
			HTTP:    &http.Client{Timeout: 120 * time.Second},
			AppName: "AgentCore-NG",
		},
	}
}

// Name — возвращает имя провайдера ("lmstudio").
// Переопределяет OpenRouterProvider.Name() для корректной идентификации в реестре.
func (p *LMStudioProvider) Name() string { return "lmstudio" }

// ListModels — получает список всех загруженных и доступных моделей из LM Studio.
// В отличие от OpenRouter, возвращает ВСЕ модели без фильтрации по популярности,
// так как пользователь сам выбирает какие модели скачать в LM Studio.
//
// Формат ответа LM Studio совместим с OpenAI GET /v1/models:
//
//	{"data": [{"id": "meta-llama-3.1-8b-instruct", ...}, ...]}
//
// При ошибке подключения (LM Studio не запущен) возвращает пустой список
// без ошибки, чтобы не блокировать работу остальных провайдеров в UI.
func (p *LMStudioProvider) ListModels() ([]string, error) {
	url := p.BaseURL + "/models"
	fmt.Printf("[LMStudio] ListModels: requesting %s (BaseURL=%s)\n", url, p.BaseURL)
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("[LMStudio] ListModels: error creating request: %v", err)
		return nil, err
	}
	if p.APIKey != "" && p.APIKey != "lm-studio" {
		httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)
	}

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		log.Printf("[LMStudio] ListModels: error doing request: %v", err)
		return []string{}, nil
	}
	defer resp.Body.Close()

	log.Printf("[LMStudio] ListModels: response status %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		return []string{}, nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("[LMStudio] ListModels: error decoding response: %v", err)
		return []string{}, nil
	}

	var models []string = []string{}
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	log.Printf("[LMStudio] ListModels: found %d models", len(models))
	return models, nil
}

// ListModelsDetailed — возвращает детальную информацию о моделях LM Studio.
// Все локальные модели бесплатны и всегда доступны (пока LM Studio запущен).
// Не требует активации или оплаты — это ключевое преимущество перед облачными провайдерами.
func (p *LMStudioProvider) ListModelsDetailed() ([]ModelDetail, error) {
	models, err := p.ListModels()
	if err != nil {
		return nil, err
	}

	var details []ModelDetail
	for _, m := range models {
		details = append(details, ModelDetail{
			ID:             m,
			IsAvailable:    true,
			PricingInfo:    fmt.Sprintf("Бесплатно (локальная модель на %s)", p.BaseURL),
			ActivationHint: "",
		})
	}
	return details, nil
}
