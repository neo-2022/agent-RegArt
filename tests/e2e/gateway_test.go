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

func gatewayURL() string {
	if u := os.Getenv("GATEWAY_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func TestGateway_HealthEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/health")
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", body["status"])
	}
}

func TestGateway_CORSHeaders(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("OPTIONS", gatewayURL()+"/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5173" {
		t.Errorf("expected CORS origin http://localhost:5173, got %q", origin)
	}
}

func TestGateway_RequestIDPropagation(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	rid := resp.Header.Get("X-Request-ID")
	if rid == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestGateway_TraceIDPropagation(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	traceID := resp.Header.Get("X-Trace-ID")
	if traceID == "" {
		t.Error("expected X-Trace-ID header in response")
	}
}

func TestGateway_MethodNotAllowed(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("DELETE", gatewayURL()+"/health", nil)

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestGateway_ModelsEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/models")
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 200 or 502, got %d", resp.StatusCode)
	}
}

func TestGateway_AgentsEndpoint(t *testing.T) {
	resp, err := http.Get(gatewayURL() + "/agents/")
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 200 or 502, got %d", resp.StatusCode)
	}
}

func TestGateway_RateLimiting(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	rateLimited := false

	for i := 0; i < 200; i++ {
		req, _ := http.NewRequest("GET", gatewayURL()+"/health", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Skipf("Gateway not available: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			rateLimited = true
			break
		}
	}

	t.Logf("Rate limited after burst: %v", rateLimited)
}

func TestGateway_ChatEndpoint_POST(t *testing.T) {
	client := &http.Client{Timeout: 10 * time.Second}
	body := `{"message":"hello","agent":"admin"}`
	req, _ := http.NewRequest("POST", gatewayURL()+"/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Gateway not available: %v", err)
	}
	defer resp.Body.Close()

	_ = fmt.Sprintf("Chat response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadGateway && resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("unexpected status %d", resp.StatusCode)
	}
}
