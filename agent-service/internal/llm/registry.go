package llm

import (
	"fmt"
	"os"
	"sync"
)

// Registry — потокобезопасный реестр LLM-провайдеров.
// Хранит все зарегистрированные провайдеры (локальные и облачные)
// и предоставляет методы для их регистрации, получения и перечисления.
// Используется глобально через переменную GlobalRegistry.
// Потокобезопасность обеспечивается через sync.RWMutex.
type Registry struct {
	mu        sync.RWMutex            // Мьютекс для потокобезопасного доступа к карте провайдеров
	providers map[string]ChatProvider // Карта: имя провайдера → реализация ChatProvider
}

// GlobalRegistry — глобальный экземпляр реестра провайдеров.
// Инициализируется при старте приложения через InitProviders().
// Дополнительные провайдеры могут быть зарегистрированы динамически
// через RegisterProvider() (например, при сохранении настроек в UI).
var GlobalRegistry = &Registry{
	providers: make(map[string]ChatProvider),
}

// Register — регистрирует нового провайдера в реестре.
// Если провайдер с таким именем уже существует, он будет заменён.
// Это позволяет обновлять настройки провайдера (например, API-ключ)
// без перезапуска сервиса.
func (r *Registry) Register(provider ChatProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.Name()] = provider
}

// Get — возвращает провайдера по имени.
// Возвращает ошибку, если провайдер не найден в реестре.
// Используется в chatHandler для маршрутизации запросов
// к правильному провайдеру на основе настроек агента.
func (r *Registry) Get(name string) (ChatProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("провайдер %q не зарегистрирован", name)
	}
	return p, nil
}

// List — возвращает список имён всех зарегистрированных провайдеров.
// Используется для отображения доступных провайдеров в UI.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// ListAll — возвращает полную информацию о всех зарегистрированных провайдерах,
// включая список доступных моделей каждого провайдера.
// Используется в эндпоинте GET /cloud-models для отображения в UI.
func (r *Registry) ListAll() []ProviderInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var infos []ProviderInfo
	for _, p := range r.providers {
		models, _ := p.ListModels()
		infos = append(infos, ProviderInfo{
			Name:   p.Name(),
			Models: models,
		})
	}
	return infos
}

// InitProviders — инициализирует провайдеров из переменных окружения.
// Вызывается один раз при старте сервиса.
//
// Ollama регистрируется всегда (по умолчанию http://localhost:11434).
// Облачные провайдеры регистрируются только при наличии API-ключей:
//   - OPENAI_API_KEY, OPENAI_BASE_URL — для OpenAI (GPT-4, GPT-4o и др.)
//   - ANTHROPIC_API_KEY, ANTHROPIC_BASE_URL — для Anthropic (Claude)
//   - YANDEXGPT_API_KEY, YANDEXGPT_FOLDER_ID, YANDEXGPT_BASE_URL — для YandexGPT
//   - GIGACHAT_CLIENT_SECRET, GIGACHAT_CLIENT_ID, GIGACHAT_SCOPE, GIGACHAT_BASE_URL — для GigaChat
func InitProviders() {
	// Ollama — локальный провайдер, регистрируется всегда
	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "http://localhost:11434"
	}
	GlobalRegistry.Register(NewOllamaProvider(ollamaURL))

	// OpenAI — облачный провайдер (GPT-4, GPT-4o, GPT-3.5-turbo и др.)
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		baseURL := os.Getenv("OPENAI_BASE_URL")
		GlobalRegistry.Register(NewOpenAIProvider(key, baseURL))
	}

	// Anthropic — облачный провайдер (Claude Sonnet, Haiku, Opus)
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		baseURL := os.Getenv("ANTHROPIC_BASE_URL")
		GlobalRegistry.Register(NewAnthropicProvider(key, baseURL))
	}

	// YandexGPT — российский облачный провайдер от Яндекса
	// Требует API-ключ и ID каталога (folder_id) из Yandex Cloud
	if key := os.Getenv("YANDEXGPT_API_KEY"); key != "" {
		folderID := os.Getenv("YANDEXGPT_FOLDER_ID")
		baseURL := os.Getenv("YANDEXGPT_BASE_URL")
		saJSON := os.Getenv("YANDEXGPT_SA_JSON")
		GlobalRegistry.Register(NewYandexGPTProvider(key, folderID, baseURL, saJSON))
	}

	// GigaChat — российский облачный провайдер от Сбера
	// Использует OAuth2 авторизацию через client credentials
	if secret := os.Getenv("GIGACHAT_CLIENT_SECRET"); secret != "" {
		clientID := os.Getenv("GIGACHAT_CLIENT_ID")
		scope := os.Getenv("GIGACHAT_SCOPE")
		baseURL := os.Getenv("GIGACHAT_BASE_URL")
		GlobalRegistry.Register(NewGigaChatProvider(clientID, secret, scope, baseURL))
	}

	// OpenRouter — агрегатор LLM-моделей (сотни моделей через один API-ключ)
	// Поддерживает модели от OpenAI, Anthropic, Google, Meta, Mistral, DeepSeek и др.
	// API полностью совместим с OpenAI Chat Completions API
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		baseURL := os.Getenv("OPENROUTER_BASE_URL")
		GlobalRegistry.Register(NewOpenRouterProvider(key, baseURL))
	}

	// Routeway — альтернативный агрегатор LLM-моделей (70+ моделей)
	// 200 бесплатных запросов в день (в 4 раза больше OpenRouter)
	// Бесплатные модели: llama-3.1-8b-instruct:free, deepseek-r1:free и др.
	// API полностью совместим с OpenAI Chat Completions API
	if key := os.Getenv("ROUTEWAY_API_KEY"); key != "" {
		baseURL := os.Getenv("ROUTEWAY_BASE_URL")
		GlobalRegistry.Register(NewRoutewayProvider(key, baseURL))
	}

	// Cerebras — сверхбыстрый облачный инференс (до 2500 токенов/сек)
	// Free tier: 1M токенов/день, 30 RPM, без карты
	// Модели: llama3.1-8b, gpt-oss-120b, qwen-3-32b, qwen-3-235b, zai-glm-4.7
	// API полностью совместим с OpenAI Chat Completions API
	if key := os.Getenv("CEREBRAS_API_KEY"); key != "" {
		baseURL := os.Getenv("CEREBRAS_BASE_URL")
		GlobalRegistry.Register(NewCerebrasProvider(key, baseURL))
	}

	// LM Studio — бесплатное десктопное приложение для запуска LLM локально.
	// Без лимитов запросов, работает офлайн, OpenAI-совместимый API.
	// По умолчанию сервер на http://localhost:1234/v1
	// Регистрируется ВСЕГДА (как и Ollama) — если LM Studio не запущен,
	// ListModels() вернёт пустой список без ошибки.
	lmsURL := os.Getenv("LMSTUDIO_BASE_URL")
	lmsKey := os.Getenv("LMSTUDIO_API_KEY")
	GlobalRegistry.Register(NewLMStudioProvider(lmsKey, lmsURL))
}

// RegisterProvider — динамически регистрирует провайдера по имени и параметрам.
// Вызывается при сохранении настроек провайдера через UI (POST /providers).
// Параметр extra содержит дополнительные данные в зависимости от провайдера:
//   - Для yandexgpt: folder_id (ID каталога в Yandex Cloud)
//   - Для gigachat: scope (область доступа, например "GIGACHAT_API_PERS")
//   - Для остальных: не используется
//
// Если провайдер с таким именем уже зарегистрирован, он будет заменён
// новым экземпляром с обновлёнными настройками.
func RegisterProvider(name, apiKey, baseURL, extra, saJSON string) error {
	switch name {
	case "openai":
		GlobalRegistry.Register(NewOpenAIProvider(apiKey, baseURL))
	case "anthropic":
		GlobalRegistry.Register(NewAnthropicProvider(apiKey, baseURL))
	case "yandexgpt":
		GlobalRegistry.Register(NewYandexGPTProvider(apiKey, extra, baseURL, saJSON))
	case "gigachat":
		// extra = scope для GigaChat, apiKey = client_secret
		GlobalRegistry.Register(NewGigaChatProvider("", apiKey, extra, baseURL))
	case "openrouter":
		// OpenRouter — агрегатор LLM-моделей, один API-ключ для всех
		GlobalRegistry.Register(NewOpenRouterProvider(apiKey, baseURL))
	case "routeway":
		// Routeway — альтернативный агрегатор (200 бесплатных запросов/день)
		GlobalRegistry.Register(NewRoutewayProvider(apiKey, baseURL))
	case "cerebras":
		GlobalRegistry.Register(NewCerebrasProvider(apiKey, baseURL))
	case "lmstudio":
		// LM Studio — бесплатный локальный провайдер (без лимитов)
		GlobalRegistry.Register(NewLMStudioProvider(apiKey, baseURL))
	case "ollama":
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		GlobalRegistry.Register(NewOllamaProvider(baseURL))
	default:
		return fmt.Errorf("неизвестный провайдер: %s", name)
	}
	return nil
}
