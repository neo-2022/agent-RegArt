package metrics

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "endpoint", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agent_service_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	chatRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_chat_requests_total",
			Help: "Total number of chat requests",
		},
		[]string{"agent", "provider", "model"},
	)

	chatRequestsErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_chat_requests_errors_total",
			Help: "Total number of chat request errors",
		},
		[]string{"agent", "provider", "model", "error_type"},
	)

	ragSearchesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_rag_searches_total",
			Help: "Total number of RAG searches",
		},
		[]string{"status", "documents_found"},
	)

	ragSearchDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "agent_service_rag_search_duration_seconds",
			Help:    "RAG search duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	llmRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_llm_requests_total",
			Help: "Total number of LLM requests",
		},
		[]string{"provider", "model"},
	)

	llmTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_llm_tokens_total",
			Help: "Total number of LLM tokens",
		},
		[]string{"provider", "model", "token_type"},
	)

	llmRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agent_service_llm_request_duration_seconds",
			Help:    "LLM request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider", "model"},
	)

	toolCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "agent_service_tool_calls_total",
			Help: "Total number of tool calls",
		},
		[]string{"tool_name", "status"},
	)

	toolCallDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "agent_service_tool_call_duration_seconds",
			Help:    "Tool call duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tool_name"},
	)
)

var registered = false

func Init() {
	if !registered {
		registry := prometheus.NewRegistry()
		registry.MustRegister(
			httpRequestsTotal,
			httpRequestDuration,
			chatRequestsTotal,
			chatRequestsErrors,
			ragSearchesTotal,
			ragSearchDuration,
			llmRequestsTotal,
			llmTokensTotal,
			llmRequestDuration,
			toolCallsTotal,
			toolCallDuration,
		)
		registered = true
	}
	log.Printf("[METRICS] Prometheus метрики инициализированы")
}

var metricsRegistry *prometheus.Registry

func InitPrometheusHandler() http.Handler {
	if metricsRegistry == nil {
		metricsRegistry = prometheus.NewRegistry()
		metricsRegistry.MustRegister(
			httpRequestsTotal,
			httpRequestDuration,
			chatRequestsTotal,
			chatRequestsErrors,
			ragSearchesTotal,
			ragSearchDuration,
			llmRequestsTotal,
			llmTokensTotal,
			llmRequestDuration,
			toolCallsTotal,
			toolCallDuration,
		)
		log.Printf("[METRICS] Prometheus endpoint инициализирован")
	}
	return promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{})
}

func RecordHTTPRequest(method, endpoint string, status int, duration time.Duration) {
	httpRequestsTotal.WithLabelValues(method, endpoint, fmt.Sprintf("%d", status)).Inc()
	httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration.Seconds())
}

func RecordChatRequest(agent, provider, model string) {
	chatRequestsTotal.WithLabelValues(agent, provider, model).Inc()
}

func RecordChatError(agent, provider, model, errorType string) {
	chatRequestsErrors.WithLabelValues(agent, provider, model, errorType).Inc()
}

func RecordRAGSearch(status string, documentsFound int, duration time.Duration) {
	ragSearchesTotal.WithLabelValues(status, fmt.Sprintf("%d", documentsFound)).Inc()
	ragSearchDuration.Observe(duration.Seconds())
}

func RecordLLMRequest(provider, model string) {
	llmRequestsTotal.WithLabelValues(provider, model).Inc()
}

func RecordLLMToken(provider, model, tokenType string, tokens int) {
	llmTokensTotal.WithLabelValues(provider, model, tokenType).Add(float64(tokens))
}

func RecordLLMRequestDuration(provider, model string, duration time.Duration) {
	llmRequestDuration.WithLabelValues(provider, model).Observe(duration.Seconds())
}

func RecordToolCall(toolName, status string, duration time.Duration) {
	toolCallsTotal.WithLabelValues(toolName, status).Inc()
	toolCallDuration.WithLabelValues(toolName).Observe(duration.Seconds())
}
