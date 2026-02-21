package llm

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type YandexGPTProvider struct {
	APIKey             string
	FolderID           string
	BaseURL            string
	ServiceAccountJSON string
	HTTP               *http.Client

	mu               sync.Mutex
	iamToken         string
	iamTokenExpiry   time.Time
	resolvedFolderID string
	lastFolderErr    error
}

type yandexServiceAccountKey struct {
	ID               string `json:"id"`
	ServiceAccountID string `json:"service_account_id"`
	PrivateKey       string `json:"private_key"`
}

func NewYandexGPTProvider(apiKey, folderID, baseURL, saJSON string) *YandexGPTProvider {
	if baseURL == "" {
		baseURL = "https://llm.api.cloud.yandex.net"
	}
	return &YandexGPTProvider{
		APIKey:             apiKey,
		FolderID:           folderID,
		BaseURL:            baseURL,
		ServiceAccountJSON: saJSON,
		HTTP:               &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *YandexGPTProvider) Name() string { return "yandexgpt" }

func (p *YandexGPTProvider) useServiceAccount() bool {
	return p.ServiceAccountJSON != ""
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func (p *YandexGPTProvider) buildJWT(saKey yandexServiceAccountKey) (string, error) {
	block, _ := pem.Decode([]byte(saKey.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("не удалось декодировать PEM приватного ключа")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("ошибка парсинга приватного ключа: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("ключ не является RSA")
	}

	now := time.Now()
	headerJSON, _ := json.Marshal(map[string]string{
		"typ": "JWT",
		"alg": "PS256",
		"kid": saKey.ID,
	})
	payloadJSON, _ := json.Marshal(map[string]interface{}{
		"iss": saKey.ServiceAccountID,
		"aud": "https://iam.api.cloud.yandex.net/iam/v1/tokens",
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	})

	signingInput := base64URLEncode(headerJSON) + "." + base64URLEncode(payloadJSON)
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPSS(rand.Reader, rsaKey, crypto.SHA256, hashed[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	if err != nil {
		return "", fmt.Errorf("ошибка подписи JWT: %w", err)
	}

	return signingInput + "." + base64URLEncode(sig), nil
}

func (p *YandexGPTProvider) getIAMToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.iamToken != "" && time.Now().Before(p.iamTokenExpiry) {
		return p.iamToken, nil
	}

	var saKey yandexServiceAccountKey
	if err := json.Unmarshal([]byte(p.ServiceAccountJSON), &saKey); err != nil {
		return "", fmt.Errorf("ошибка парсинга JSON сервисного аккаунта: %w", err)
	}

	jwt, err := p.buildJWT(saKey)
	if err != nil {
		return "", err
	}

	reqBody, _ := json.Marshal(map[string]string{"jwt": jwt})
	resp, err := p.HTTP.Post("https://iam.api.cloud.yandex.net/iam/v1/tokens", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("ошибка запроса IAM-токена: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IAM API HTTP %d: %s", resp.StatusCode, string(body))
	}

	var iamResp struct {
		IAMToken  string `json:"iamToken"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.Unmarshal(body, &iamResp); err != nil {
		return "", fmt.Errorf("ошибка декодирования IAM-ответа: %w", err)
	}

	p.iamToken = iamResp.IAMToken
	p.iamTokenExpiry = time.Now().Add(11 * time.Hour)

	if p.resolvedFolderID == "" && p.FolderID == "" {
		fid, err := p.resolveServiceAccountFolder(saKey.ServiceAccountID, p.iamToken)
		if err != nil {
			p.lastFolderErr = err
		} else if fid != "" {
			p.resolvedFolderID = fid
		}
	}

	return p.iamToken, nil
}

func (p *YandexGPTProvider) resolveServiceAccountFolder(saID, iamToken string) (string, error) {
	url := "https://iam.api.cloud.yandex.net/iam/v1/serviceAccounts/" + saID
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+iamToken)
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IAM serviceAccounts HTTP %d: %s", resp.StatusCode, string(body))
	}
	var sa struct {
		FolderID string `json:"folderId"`
	}
	if err := json.Unmarshal(body, &sa); err != nil {
		return "", err
	}
	return sa.FolderID, nil
}

func (p *YandexGPTProvider) effectiveFolderID() string {
	if p.resolvedFolderID != "" {
		return p.resolvedFolderID
	}
	return p.FolderID
}

func (p *YandexGPTProvider) GetResolvedFolderID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resolvedFolderID
}

func (p *YandexGPTProvider) setAuthHeaders(req *http.Request) error {
	if p.useServiceAccount() {
		token, err := p.getIAMToken()
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("Authorization", "Api-Key "+p.APIKey)
	}
	fid := p.effectiveFolderID()
	if fid != "" {
		req.Header.Set("x-folder-id", fid)
	}
	return nil
}

type yandexRequest struct {
	ModelURI          string               `json:"modelUri"`
	CompletionOptions yandexCompletionOpts `json:"completionOptions"`
	Messages          []yandexMessage      `json:"messages"`
}

type yandexCompletionOpts struct {
	Stream      bool    `json:"stream"`
	Temperature float64 `json:"temperature"`
	MaxTokens   string  `json:"maxTokens"`
}

type yandexMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type yandexResponse struct {
	Result struct {
		Alternatives []struct {
			Message struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"message"`
			Status string `json:"status"`
		} `json:"alternatives"`
		Usage struct {
			InputTextTokens  string `json:"inputTextTokens"`
			CompletionTokens string `json:"completionTokens"`
			TotalTokens      string `json:"totalTokens"`
		} `json:"usage"`
		ModelVersion string `json:"modelVersion"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (p *YandexGPTProvider) Chat(req *ChatRequest) (*ChatResponse, error) {
	if !p.useServiceAccount() && p.APIKey == "" {
		return nil, fmt.Errorf("API-ключ или JSON сервисного аккаунта YandexGPT не настроены")
	}
	if p.useServiceAccount() {
		if _, err := p.getIAMToken(); err != nil {
			return nil, err
		}
	}
	fid := p.effectiveFolderID()
	if fid == "" {
		return nil, fmt.Errorf("Folder ID YandexGPT не настроен. Укажите Folder ID вручную")
	}

	modelURI := fmt.Sprintf("gpt://%s/%s/latest", fid, req.Model)

	var msgs []yandexMessage
	for _, m := range req.Messages {
		role := m.Role
		if role == "tool" {
			role = "assistant"
		}
		msgs = append(msgs, yandexMessage{
			Role: role,
			Text: m.Content,
		})
	}

	yReq := yandexRequest{
		ModelURI: modelURI,
		CompletionOptions: yandexCompletionOpts{
			Stream:      false,
			Temperature: 0.6,
			MaxTokens:   "2000",
		},
		Messages: msgs,
	}

	data, err := json.Marshal(yReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	httpReq, err := http.NewRequest("POST", p.BaseURL+"/foundationModels/v1/completion", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ошибка создания запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := p.setAuthHeaders(httpReq); err != nil {
		return nil, err
	}

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ошибка отправки запроса к YandexGPT: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("YandexGPT HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	var yResp yandexResponse
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return nil, fmt.Errorf("ошибка декодирования ответа: %w", err)
	}

	if yResp.Error != nil {
		return nil, fmt.Errorf("ошибка YandexGPT: %s", yResp.Error.Message)
	}

	var content string
	if len(yResp.Result.Alternatives) > 0 {
		content = yResp.Result.Alternatives[0].Message.Text
	}

	return &ChatResponse{
		Content: content,
		Model:   req.Model,
	}, nil
}

func (p *YandexGPTProvider) Validate() error {
	if !p.useServiceAccount() && p.APIKey == "" {
		return fmt.Errorf("API-ключ или JSON сервисного аккаунта YandexGPT не настроены")
	}
	if p.useServiceAccount() {
		if _, err := p.getIAMToken(); err != nil {
			return err
		}
	}
	fid := p.effectiveFolderID()
	if fid == "" {
		if p.lastFolderErr != nil {
			return fmt.Errorf("Не удалось определить Folder ID автоматически (%v). Укажите Folder ID вручную", p.lastFolderErr)
		}
		return fmt.Errorf("Folder ID YandexGPT не настроен. Укажите Folder ID вручную (находится в Yandex Cloud Console → Каталог)")
	}

	yReq := yandexRequest{
		ModelURI: fmt.Sprintf("gpt://%s/%s/latest", fid, "yandexgpt-lite"),
		CompletionOptions: yandexCompletionOpts{
			Stream:      false,
			Temperature: 0.0,
			MaxTokens:   "1",
		},
		Messages: []yandexMessage{{Role: "user", Text: "ping"}},
	}
	data, err := json.Marshal(yReq)
	if err != nil {
		return fmt.Errorf("ошибка маршалинга запроса: %w", err)
	}

	httpReq, err := http.NewRequest("POST", p.BaseURL+"/foundationModels/v1/completion", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if err := p.setAuthHeaders(httpReq); err != nil {
		return err
	}

	resp, err := p.HTTP.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ошибка отправки запроса к YandexGPT: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("YandexGPT HTTP %d: %s", resp.StatusCode, translateProviderError(resp.StatusCode, string(body)))
	}

	var yResp yandexResponse
	if err := json.NewDecoder(resp.Body).Decode(&yResp); err != nil {
		return fmt.Errorf("ошибка декодирования ответа: %w", err)
	}
	if yResp.Error != nil {
		return fmt.Errorf("ошибка YandexGPT: %s", yResp.Error.Message)
	}
	return nil
}

func (p *YandexGPTProvider) ListModels() ([]string, error) {
	return []string{
		"yandexgpt",
		"yandexgpt-lite",
		"yandexgpt-32k",
		"summarization",
	}, nil
}

func (p *YandexGPTProvider) ListModelsDetailed() ([]ModelDetail, error) {
	return []ModelDetail{
		{
			ID:             "yandexgpt-lite",
			IsAvailable:    true,
			PricingInfo:    "Бесплатный грант 4000 руб. при регистрации. Далее: 0.20 руб/1K токенов",
			ActivationHint: "",
		},
		{
			ID:             "yandexgpt",
			IsAvailable:    false,
			PricingInfo:    "1.20 руб/1K токенов (генерация), 0.30 руб/1K токенов (промпт)",
			ActivationHint: "Пополните баланс Yandex Cloud или используйте грант на 4000 руб. при регистрации: https://cloud.yandex.ru/docs/billing/",
		},
		{
			ID:             "yandexgpt-32k",
			IsAvailable:    false,
			PricingInfo:    "1.20 руб/1K токенов (генерация), 0.30 руб/1K токенов (промпт), контекст 32K",
			ActivationHint: "Пополните баланс Yandex Cloud. Модель с расширенным контекстом 32K токенов.",
		},
		{
			ID:             "summarization",
			IsAvailable:    false,
			PricingInfo:    "1.20 руб/1K токенов",
			ActivationHint: "Пополните баланс Yandex Cloud. Специализированная модель для суммаризации текстов.",
		},
	}, nil
}
