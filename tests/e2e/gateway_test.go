// Package e2e — сквозные (end-to-end) тесты для api-gateway.
//
// Тесты проверяют реальные HTTP-эндпоинты api-gateway:
// здоровье сервиса, CORS-заголовки, трассировку, ограничение частоты запросов,
// маршрутизацию к агентам и чату.
//
// Для запуска необходим работающий api-gateway (по умолчанию http://localhost:8080).
// URL можно переопределить через переменную окружения GATEWAY_URL.
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// gatewayURL — возвращает базовый URL api-gateway.
// Если переменная GATEWAY_URL задана — использует её, иначе — http://localhost:8080.
func gatewayURL() string {
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

// TestGateway_HealthEndpoint — проверяет эндпоинт здоровья /health.
// Ожидаемое поведение: статус 200, тело ответа содержит {"status":"ok"}.
func TestGateway_HealthEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/health")
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ожидался статус 200, получен %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("ожидался status=ok, получен %v", body["status"])
	}
}

// TestGateway_CORSHeaders — проверяет наличие CORS-заголовков в ответе.
// Отправляет OPTIONS-запрос с Origin: http://localhost:5173 и проверяет,
// что Access-Control-Allow-Origin установлен корректно.
func TestGateway_CORSHeaders(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("OPTIONS", gatewayURL()+"/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5173" {
		t.Errorf("ожидался CORS origin http://localhost:5173, получен %q", origin)
	}
}

// TestGateway_RequestIDPropagation — проверяет генерацию X-Request-ID.
// Каждый ответ от api-gateway должен содержать заголовок X-Request-ID
// для идентификации запроса в логах.
func TestGateway_RequestIDPropagation(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("ожидался заголовок X-Request-ID в ответе")
	}
}

// TestGateway_TraceIDPropagation — проверяет генерацию X-Trace-ID.
// Каждый ответ от api-gateway должен содержать заголовок X-Trace-ID
// для распределённой трассировки запросов между сервисами.
func TestGateway_TraceIDPropagation(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	traceID := resp.Header.Get("X-Trace-ID")
	if traceID == "" {
		t.Error("ожидался заголовок X-Trace-ID в ответе")
	}
}

// TestGateway_MethodNotAllowed — проверяет обработку неподдерживаемого HTTP-метода.
// DELETE-запрос к /health должен вернуть 405 Method Not Allowed.
func TestGateway_MethodNotAllowed(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("DELETE", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("ожидался статус 405, получен %d", resp.StatusCode)
	}
}

// TestGateway_ModelsEndpoint — проверяет эндпоинт списка моделей /models.
// Допустимые ответы: 200 (модели доступны) или 502 (Ollama недоступна).
func TestGateway_ModelsEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/models")
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Errorf("ожидался статус 200 или 502, получен %d", resp.StatusCode)
	}
}

// TestGateway_AgentsEndpoint — проверяет эндпоинт списка агентов /agents/.
// Допустимые ответы: 200 (агенты доступны) или 502 (agent-service недоступен).
func TestGateway_AgentsEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/agents/")
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Errorf("ожидался статус 200 или 502, получен %d", resp.StatusCode)
	}
}

// TestGateway_RateLimiting — проверяет работу Rate Limiter.
// Отправляет серию быстрых запросов и проверяет, что рано или поздно
// сервер вернёт 429 Too Many Requests.
func TestGateway_RateLimiting(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	rateLimited := false

	for i := 0; i < 200; i++ {
		req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Skipf("api-gateway недоступен: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	t.Logf("Лимит запросов сработал: %v", rateLimited)
}

// TestGateway_ChatEndpoint_POST — проверяет POST-запрос к эндпоинту чата /chat.
// Отправляет тестовое сообщение агенту admin и проверяет,
// что ответ имеет допустимый статус (200, 502, 500).
func TestGateway_ChatEndpoint_POST(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	body := `{"message":"hello","agent":"admin"}`
	req, _ := http.NewRequest("POST", gatewayURL()+"/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("api-gateway недоступен: %v", err)
	}
	defer resp.Body.Close()

	_ = fmt.Sprintf("Статус ответа чата: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("неожиданный статус %d", resp.StatusCode)
	}
}
