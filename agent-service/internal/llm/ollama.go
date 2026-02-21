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

// OllamaProvider — провайдер для локальных моделей через Ollama.
// Ollama запускается на ПК пользователя и предоставляет REST API
// для взаимодействия с локально установленными моделями (LLaMA, Qwen, Mistral и др.).
// Поддерживает стриминг ответов и вызов инструментов (tool calling).
// По умолчанию подключается к http://localhost:11434.
type OllamaProvider struct {
	BaseURL string       // Базовый URL Ollama API (по умолчанию http://localhost:11434)
	HTTP    *http.Client // HTTP-клиент для выполнения запросов
}

// NewOllamaProvider — создаёт новый экземпляр OllamaProvider.
// Если baseURL пустой, используется адрес по умолчанию http://localhost:11434.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaProvider{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// Name — возвращает имя провайдера ("ollama").
// Используется для идентификации провайдера в реестре.
func (p *OllamaProvider) Name() string { return "ollama" }

// Chat — отправляет запрос к Ollama API (/api/chat) и возвращает ответ.
// Конвертирует универсальный ChatRequest в формат запроса Ollama,
// отправляет его и парсит ответ обратно в ChatResponse.
// Если включён стриминг (req.Stream = true), чтение происходит
// через readStream — чанки JSON читаются последовательно до флага done=true.
func (p *OllamaProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	// Формируем запрос в формате Ollama API
	ollamaReq := &OllamaRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Stream:   req.Stream,
		Tools:    req.Tools,
		Options: map[string]interface{}{
			"num_ctx": 8192,
		},
	}

	url := p.BaseURL + "/api/chat"
	data, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	// Отправляем POST-запрос к Ollama
	resp, err := p.HTTP.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	// Если включён стриминг — читаем ответ по частям
	if req.Stream {
		return p.readStream(resp.Body)
	}

	// Обычный (не стриминговый) режим — парсим весь ответ целиком
	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	return &ChatResponse{
		Content:   ollamaResp.Message.Content,
		ToolCalls: ollamaResp.Message.ToolCalls,
		Model:     ollamaResp.Model,
	}, nil
}

// readStream — читает потоковый ответ от Ollama.
// Ollama возвращает ответ в виде последовательности JSON-объектов (чанков),
// каждый из которых содержит часть текста. Последний чанк имеет done=true.
// Все части текста собираются в единый ответ через strings.Builder.
func (p *OllamaProvider) readStream(body io.Reader) (*ChatResponse, error) {
	dec := json.NewDecoder(body)
	var content strings.Builder
	var toolCalls []ToolCall
	var model string

	for {
		var chunk OllamaResponse
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("ошибка декодирования чанка стрима: %w", err)
		}

		if chunk.Model != "" {
			model = chunk.Model
		}
		// Собираем текст из каждого чанка
		if chunk.Message.Content != "" {
			content.WriteString(chunk.Message.Content)
		}
		// Вызовы инструментов приходят обычно в одном чанке
		if len(chunk.Message.ToolCalls) > 0 {
			toolCalls = chunk.Message.ToolCalls
		}
		// Флаг done=true означает конец стрима
		if chunk.Done {
			break
		}
	}

	return &ChatResponse{
		Content:   content.String(),
		ToolCalls: toolCalls,
		Model:     model,
	}, nil
}

// ListModels — получает список установленных локальных моделей из Ollama.
// Обращается к эндпоинту GET /api/tags и возвращает список имён моделей.
// Эти модели отображаются в UI в режиме "Локальная".
func (p *OllamaProvider) ListModels() ([]string, error) {
	resp, err := p.HTTP.Get(p.BaseURL + "/api/tags")
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к Ollama: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ошибка парсинга ответа Ollama: %w", err)
	}

	var models []string
	for _, m := range result.Models {
		models = append(models, m.Name)
	}
	return models, nil
}

// ListModelsDetailed — возвращает детальную информацию о локальных моделях Ollama.
// Все модели Ollama бесплатны, так как запускаются локально на ПК пользователя.
// Не требуют API-ключей, подписок или пополнения баланса.
func (p *OllamaProvider) ListModelsDetailed() ([]ModelDetail, error) {
	names, err := p.ListModels()
	if err != nil {
		return nil, err
	}
	var details []ModelDetail
	for _, name := range names {
		details = append(details, ModelDetail{
			ID:             name,
			IsAvailable:    true,
			PricingInfo:    "Бесплатно (локальная модель)",
			ActivationHint: "",
		})
	}
	return details, nil
}
