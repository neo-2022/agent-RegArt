package llm

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// GigaChatProvider — провайдер для облачных моделей GigaChat от Сбера.
// Поддерживает модели: GigaChat, GigaChat-Plus, GigaChat-Pro, GigaChat-Max.
// Использует двухэтапную авторизацию: сначала получает OAuth-токен через
// отдельный сервер авторизации (ngw.devices.sberbank.ru), затем использует
// его для запросов к API.
//
// Особенности:
//   - OAuth2 авторизация с кэшированием токена (автоматическое обновление)
//   - TLS с InsecureSkipVerify=true (требование Сбера — их сертификат не в стандартных CA)
//   - Scope определяет уровень доступа: GIGACHAT_API_PERS (персональный),
//     GIGACHAT_API_B2B (бизнес), GIGACHAT_API_CORP (корпоративный)
//   - Формат API совместим с OpenAI (chat/completions), но без tool calling
type GigaChatProvider struct {
	ClientID     string       // ID клиента для OAuth (может быть пустым)
	ClientSecret string       // Секрет клиента / авторизационные данные
	Scope        string       // Область доступа: GIGACHAT_API_PERS, GIGACHAT_API_B2B, GIGACHAT_API_CORP
	BaseURL      string       // Базовый URL API (по умолчанию https://gigachat.devices.sberbank.ru/api/v1)
	AuthURL      string       // URL сервера авторизации OAuth
	HTTP         *http.Client // HTTP-клиент с отключённой проверкой TLS-сертификата

	mu          sync.Mutex // Мьютекс для потокобезопасного доступа к токену
	accessToken string     // Кэшированный OAuth access token
	tokenExpiry time.Time  // Время истечения токена (с запасом 30 секунд)
}

// NewGigaChatProvider — создаёт новый экземпляр GigaChatProvider.
// Настраивает HTTP-клиент с отключённой проверкой TLS-сертификата
// (требование Сбера — их API использует собственный CA).
// По умолчанию scope = GIGACHAT_API_PERS (персональный доступ).
func NewGigaChatProvider(clientID, clientSecret, scope, baseURL string) *GigaChatProvider {
	if baseURL == "" {
		baseURL = "https://gigachat.devices.sberbank.ru/api/v1"
	}
	if scope == "" {
		scope = "GIGACHAT_API_PERS"
	}
	return &GigaChatProvider{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scope:        scope,
		BaseURL:      baseURL,
		AuthURL:      "https://ngw.devices.sberbank.ru:9443/api/v2/oauth",
		HTTP: &http.Client{
			Timeout: 120 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Name — возвращает имя провайдера ("gigachat").
func (p *GigaChatProvider) Name() string { return "gigachat" }

// getToken — получает или возвращает кэшированный OAuth access token.
// Логика кэширования:
//  1. Если токен есть и не истёк — возвращаем из кэша (быстрый путь)
//  2. Если токен истёк или отсутствует — запрашиваем новый через OAuth
//  3. Новый токен кэшируется с запасом 30 секунд до истечения
//
// Потокобезопасность обеспечивается через sync.Mutex.
// Авторизация поддерживает два режима:
//   - Basic Auth (ClientID + ClientSecret) — если ClientID указан
//   - Прямая передача в заголовке Authorization — если ClientID пустой
func (p *GigaChatProvider) getToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Быстрый путь: возвращаем кэшированный токен, если он ещё действителен
	if p.accessToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.accessToken, nil
	}

	if p.ClientSecret == "" {
		return "", fmt.Errorf("учётные данные GigaChat не настроены")
	}

	// Формируем запрос на получение токена через OAuth endpoint
	data := url.Values{}
	data.Set("scope", p.Scope)

	req, err := http.NewRequest("POST", p.AuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса авторизации: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// RqUID — уникальный идентификатор запроса в формате UUID v4 (требование API Сбера)
	req.Header.Set("RqUID", generateUUIDv4())

	// Два режима авторизации в зависимости от наличия ClientID
	if p.ClientID != "" {
		// Стандартная Basic Auth с ClientID и ClientSecret
		req.SetBasicAuth(p.ClientID, p.ClientSecret)
	} else {
		// Прямая передача секрета (для упрощённого режима авторизации)
		req.Header.Set("Authorization", "Basic "+p.ClientSecret)
	}

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка получения токена GigaChat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("авторизация GigaChat вернула статус %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"` // OAuth access token
		ExpiresAt   int64  `json:"expires_at"`   // Время истечения (Unix timestamp в миллисекундах)
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("ошибка декодирования ответа авторизации: %w", err)
	}

	// Кэшируем токен с запасом 30 секунд до реального истечения
	p.accessToken = tokenResp.AccessToken
	p.tokenExpiry = time.UnixMilli(tokenResp.ExpiresAt).Add(-30 * time.Second)
	return p.accessToken, nil
}

// gigachatRequest — структура запроса к GigaChat Chat Completions API.
// Формат совместим с OpenAI API (model, messages, stream).
type gigachatRequest struct {
	Model    string            `json:"model"`    // Имя модели (GigaChat, GigaChat-Plus, GigaChat-Pro, GigaChat-Max)
	Messages []gigachatMessage `json:"messages"` // Массив сообщений диалога
	Stream   bool              `json:"stream"`   // Режим стриминга (пока не используется)
}

// gigachatMessage — сообщение в формате GigaChat API.
// Совместим с форматом OpenAI (role + content).
type gigachatMessage struct {
	Role    string `json:"role"`    // Роль: system, user, assistant
	Content string `json:"content"` // Текст сообщения
}

// gigachatResponse — структура ответа от GigaChat API.
// Формат совместим с OpenAI (choices → message → content).
type gigachatResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`    // Роль (всегда "assistant")
			Content string `json:"content"` // Текст ответа
		} `json:"message"`
		FinishReason string `json:"finish_reason"` // Причина остановки: stop, length
	} `json:"choices"`
	Model string `json:"model"` // Имя использованной модели
	Error *struct {
		Message string `json:"message"` // Текст ошибки
	} `json:"error,omitempty"`
}

// Chat — отправляет запрос к GigaChat Chat Completions API.
// Автоматически получает/обновляет OAuth-токен перед запросом.
// Конвертирует универсальный ChatRequest в формат GigaChat:
//   - Сообщения с ролью "tool" конвертируются в "assistant"
//     (GigaChat не поддерживает tool calling)
//   - Авторизация через Bearer-токен (полученный через OAuth)
//
// generateUUIDv4 — генерирует UUID версии 4 для заголовка RqUID.
// GigaChat API требует уникальный идентификатор запроса в формате UUID v4.
func generateUUIDv4() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // версия 4
	b[8] = (b[8] & 0x3f) | 0x80 // вариант RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (p *GigaChatProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	log.Printf("GigaChat: отправка запроса, модель=%s, сообщений=%d", req.Model, len(req.Messages))
	// Получаем или обновляем OAuth-токен
	token, err := p.getToken()
	if err != nil {
		log.Printf("GigaChat: ошибка получения токена: %v", err)
		return nil, err
	}

	// Конвертируем сообщения, заменяя "tool" на "assistant"
	var msgs []gigachatMessage
	for _, m := range req.Messages {
		role := m.Role
		if role == "tool" {
			// GigaChat не поддерживает роль "tool"
			role = "assistant"
		}
		msgs = append(msgs, gigachatMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	gReq := gigachatRequest{
		Model:    req.Model,
		Messages: msgs,
		Stream:   false,
	}

	data, err := json.Marshal(gReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	// Создаём HTTP-запрос с Bearer-токеном
	httpReq, err := http.NewRequest("POST", p.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к GigaChat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("GigaChat: ошибка HTTP %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("GigaChat HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	var gResp gigachatResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		log.Printf("GigaChat: ошибка декодирования ответа: %v", err)
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if gResp.Error != nil {
		log.Printf("GigaChat: ошибка API: %s", gResp.Error.Message)
		return nil, fmt.Errorf("ошибка GigaChat: %s", gResp.Error.Message)
	}

	var content string
	if len(gResp.Choices) > 0 {
		content = gResp.Choices[0].Message.Content
	}

	log.Printf("GigaChat: ответ получен, %d символов", len(content))
	return &ChatResponse{
		Content: content,
		Model:   req.Model,
	}, nil
}

// ListModels — получает список доступных моделей GigaChat.
// Сначала пытается получить актуальный список через GET /models API.
// Если авторизация не удалась (нет ключа или ошибка) — возвращает
// захардкоженный список основных моделей как fallback.
//
// Модели GigaChat:
//   - GigaChat — базовая модель (бесплатная для персонального использования)
//   - GigaChat-Plus — улучшенная модель с большим контекстом
//   - GigaChat-Pro — профессиональная модель (лучшее качество)
//   - GigaChat-Max — максимальная модель (самая мощная)
func (p *GigaChatProvider) ListModels() ([]string, error) {
	token, err := p.getToken()
	if err != nil {
		return []string{
			"GigaChat",
			"GigaChat-Plus",
			"GigaChat-Pro",
			"GigaChat-Max",
		}, nil
	}

	httpReq, err := http.NewRequest("GET", p.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка моделей GigaChat: %w", err)
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

	var models []string
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	if len(models) == 0 {
		return []string{"GigaChat", "GigaChat-Plus", "GigaChat-Pro", "GigaChat-Max"}, nil
	}
	return models, nil
}

// gigachatModelPricing — информация о ценах и бесплатности моделей GigaChat.
// Источник: https://developers.sber.ru/docs/ru/gigachat/models
// GigaChat (базовая) — бесплатна для персонального использования (GIGACHAT_API_PERS).
// GigaChat-Plus — бесплатна для персонального использования.
// GigaChat-Pro — платная, требует пополнения баланса или подписку B2B.
// GigaChat-Max — платная, самая мощная, требует пополнения баланса.
var gigachatModelPricing = map[string]ModelDetail{
	"GigaChat": {
		ID: "GigaChat", IsAvailable: true,
		PricingInfo:    "Бесплатно (персональный доступ GIGACHAT_API_PERS)",
		ActivationHint: "",
	},
	"GigaChat-Plus": {
		ID: "GigaChat-Plus", IsAvailable: true,
		PricingInfo:    "Бесплатно (персональный доступ GIGACHAT_API_PERS)",
		ActivationHint: "",
	},
	"GigaChat-Pro": {
		ID: "GigaChat-Pro", IsAvailable: false,
		PricingInfo:    "Платная: ~3.5 руб/1K токенов",
		ActivationHint: "Необходимо пополнить баланс на developers.sber.ru или подключить тариф B2B (GIGACHAT_API_B2B).",
	},
	"GigaChat-Max": {
		ID: "GigaChat-Max", IsAvailable: false,
		PricingInfo:    "Платная: ~7 руб/1K токенов",
		ActivationHint: "Необходимо пополнить баланс на developers.sber.ru или подключить тариф B2B (GIGACHAT_API_B2B).",
	},
}

// ListModelsDetailed — возвращает детальную информацию о моделях GigaChat с ценами.
// Сначала получает реальный список моделей через API, затем обогащает
// информацией о ценах из документации Сбера (gigachatModelPricing).
// Если модель не найдена в справочнике цен — по умолчанию помечается как платная.
func (p *GigaChatProvider) ListModelsDetailed() ([]ModelDetail, error) {
	names, err := p.ListModels()
	if err != nil {
		return nil, err
	}
	var details []ModelDetail
	for _, name := range names {
		if d, ok := gigachatModelPricing[name]; ok {
			details = append(details, d)
		} else {
			details = append(details, ModelDetail{
				ID:             name,
				IsAvailable:    false,
				PricingInfo:    "Платная (уточните на developers.sber.ru)",
				ActivationHint: "Пополните баланс на developers.sber.ru или подключите тариф B2B.",
			})
		}
	}
	return details, nil
}
