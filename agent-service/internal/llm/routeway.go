// Файл routeway.go — провайдер Routeway для доступа к 70+ облачным LLM-моделям.
//
// Routeway (https://routeway.ai) — агрегатор LLM-моделей с единым OpenAI-совместимым API.
// Поддерживает модели от Meta (Llama), DeepSeek, Mistral, NVIDIA, Qwen и других.
//
// Ключевые преимущества по сравнению с OpenRouter:
//   - 200 бесплатных запросов в день (против 50 у OpenRouter)
//   - Бесплатная модель llama-3.1-8b-instruct:free с поддержкой tool calling
//   - Нет ограничений по rate limit для платных моделей
//   - OpenAI-совместимый формат (тот же код, что и для OpenRouter)
//
// Авторизация: Bearer-токен в заголовке Authorization.
// Базовый URL: https://api.routeway.ai/v1
// Формат API: полностью совместим с OpenAI Chat Completions API.
//
// Реализация переиспользует код OpenRouterProvider, переопределяя только
// имя провайдера (Name() → "routeway") и базовый URL по умолчанию.
package llm

import (
	"net/http"
	"time"
)

// RoutewayProvider — провайдер для доступа к моделям через Routeway.
// Встраивает OpenRouterProvider, так как API полностью совместимы.
// Переопределяет только Name() для корректной идентификации в реестре.
//
// Бесплатные модели Routeway (суффикс :free):
//   - llama-3.1-8b-instruct:free (8B, tool calling)
//   - llama-3.3-70b-instruct:free (70B, tool calling)
//   - deepseek-r1:free (reasoning)
//   - nemotron-nano-9b-v2:free (9B, tool calling)
//   - и другие (всего 16+ бесплатных моделей)
type RoutewayProvider struct {
	*OpenRouterProvider
}

// NewRoutewayProvider — создаёт новый экземпляр RoutewayProvider.
// Если baseURL пустой, используется официальный API Routeway: https://api.routeway.ai/v1
//
// Параметры:
//   - apiKey: API-ключ Routeway (получить на https://routeway.ai/dashboard)
//   - baseURL: базовый URL (пустая строка = https://api.routeway.ai/v1)
//
// Возвращает: настроенный экземпляр RoutewayProvider
func NewRoutewayProvider(apiKey, baseURL string) *RoutewayProvider {
	if baseURL == "" {
		baseURL = "https://api.routeway.ai/v1"
	}
	return &RoutewayProvider{
		OpenRouterProvider: &OpenRouterProvider{
			APIKey:  apiKey,
			BaseURL: baseURL,
			HTTP:    &http.Client{Timeout: 120 * time.Second},
			AppName: "AgentCore-NG",
		},
	}
}

// Name — возвращает имя провайдера ("routeway").
// Переопределяет OpenRouterProvider.Name() для корректной идентификации в реестре.
func (p *RoutewayProvider) Name() string { return "routeway" }
