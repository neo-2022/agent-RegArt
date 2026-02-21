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

type CerebrasProvider struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

func NewCerebrasProvider(apiKey, baseURL string) *CerebrasProvider {
	if baseURL == "" {
		baseURL = "https://api.cerebras.ai/v1"
	}
	return &CerebrasProvider{
		APIKey:  apiKey,
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *CerebrasProvider) Name() string { return "cerebras" }

func (p *CerebrasProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ Cerebras не настроен")
	}

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

	oaiReq := openaiRequest{
		Model:    req.Model,
		Messages: msgs,
		Tools:    oaiTools,
		Stream:   false,
	}

	data, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса Cerebras: %w", err)
	}

	httpReq, err := http.NewRequest("POST", p.BaseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса Cerebras: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к Cerebras: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Cerebras HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	var oaiResp openaiResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaiResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа Cerebras: %w", err)
	}

	if oaiResp.Error != nil {
		return nil, fmt.Errorf("ошибка Cerebras: %s", oaiResp.Error.Message)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("Cerebras вернул пустой ответ")
	}

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

func isCerebrasModel(id string) bool {
	known := []string{
		"llama3.1-8b", "llama-3.3-70b",
		"qwen-3-32b", "qwen-3-235b",
		"gpt-oss-120b", "zai-glm-4.7",
	}
	lower := strings.ToLower(id)
	for _, k := range known {
		if strings.HasPrefix(lower, k) {
			return true
		}
	}
	return false
}

func (p *CerebrasProvider) ListModels() ([]string, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ Cerebras не настроен")
	}

	httpReq, err := http.NewRequest("GET", p.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения списка моделей Cerebras: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Cerebras /models HTTP %d: %s", resp.StatusCode, string(body))
	}

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
		if isCerebrasModel(m.ID) {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

func (p *CerebrasProvider) ListModelsDetailed() ([]ModelDetail, error) {
	names, err := p.ListModels()
	if err != nil {
		return nil, err
	}

	var details []ModelDetail
	for _, name := range names {
		d := ModelDetail{
			ID:          name,
			IsAvailable: true,
			PricingInfo: "Бесплатно (Free tier: 1M токенов/день)",
		}
		switch {
		case strings.HasPrefix(name, "llama3.1-8b"):
			d.PricingInfo = "Бесплатно | PayGo: $0.10/1M токенов"
		case strings.HasPrefix(name, "llama-3.3-70b"):
			d.PricingInfo = "Бесплатно | PayGo: $0.60/1M токенов"
		case strings.HasPrefix(name, "qwen-3-32b"):
			d.PricingInfo = "Бесплатно | PayGo: $0.30/1M токенов"
		case strings.HasPrefix(name, "qwen-3-235b"):
			d.PricingInfo = "Бесплатно | PayGo: $0.90/1M токенов"
		case strings.HasPrefix(name, "gpt-oss-120b"):
			d.PricingInfo = "Бесплатно | PayGo: $0.60/1M токенов"
		case strings.HasPrefix(name, "zai-glm-4.7"):
			d.PricingInfo = "Бесплатно (Preview) | PayGo: $0.60/1M токенов"
		}
		details = append(details, d)
	}
	return details, nil
}
