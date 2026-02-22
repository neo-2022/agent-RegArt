// Пакет main — точка входа API Gateway.
// API Gateway — центральный маршрутизатор, который принимает все HTTP-запросы
// от web-ui (фронтенда) и перенаправляет их на соответствующие микросервисы:
//   - /memory/*  → memory-service (Python, порт 8001) — хранилище знаний, RAG-поиск
//   - /tools/*   → tools-service (Go, порт 8082) — выполнение команд, работа с файлами
//   - /agents/*  → agent-service (Go, порт 8083) — управление агентами, чат, LLM
//   - /chat, /models, /providers, /workspaces и др. → agent-service
//
// Функции:
//   - Reverse proxy для всех микросервисов
//   - CORS-защита с настраиваемым белым списком доменов
//   - Фильтрация HTTP-методов для каждого маршрута
//   - Два режима проксирования: с удалением префикса (Strip) и без
//
// Конфигурация через переменные окружения:
//   - MEMORY_SERVICE_URL  — URL memory-service (по умолчанию http://localhost:8001)
//   - TOOLS_SERVICE_URL   — URL tools-service (по умолчанию http://localhost:8082)
//   - AGENT_SERVICE_URL   — URL agent-service (по умолчанию http://localhost:8083)
//   - GATEWAY_PORT        — порт API Gateway (по умолчанию 8080)
//   - CORS_ALLOWED_ORIGINS — белый список доменов для CORS (через запятую)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/neo-2022/openclaw-memory/api-gateway/gates"
	"github.com/neo-2022/openclaw-memory/api-gateway/internal/apierror"
	"github.com/neo-2022/openclaw-memory/api-gateway/internal/logger"
	"github.com/neo-2022/openclaw-memory/api-gateway/internal/middleware"
)

// parseAllowedOrigins — разбирает переменную окружения CORS_ALLOWED_ORIGINS
// и возвращает множество (map) разрешённых доменов.
// По умолчанию разрешены: http://localhost:3000 (React dev), http://localhost:5173 (Vite dev).
// Пустые строки в списке игнорируются. Пробелы вокруг доменов удаляются.
func parseAllowedOrigins() map[string]struct{} {
	origins := strings.Split(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173"), ",")
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		allowed[o] = struct{}{}
	}
	return allowed
}

// corsMiddleware — middleware для обработки CORS (Cross-Origin Resource Sharing).
// Проверяет заголовок Origin запроса по белому списку разрешённых доменов.
// Если Origin присутствует в белом списке — устанавливает заголовки:
//   - Access-Control-Allow-Origin: <origin>
//   - Access-Control-Allow-Methods: <список методов>
//   - Access-Control-Allow-Headers: Content-Type, Authorization
//   - Vary: Origin (для корректного кэширования)
//
// Для preflight-запросов (OPTIONS) возвращает 204 No Content без дальнейшей обработки.
//
// Параметры:
//   - next: следующий обработчик в цепочке
//   - allowedMethods: разрешённые HTTP-методы для данного маршрута
//   - allowedOrigins: белый список доменов
var requestCounter uint64

func generateRequestID() string {
	requestCounter++
	return fmt.Sprintf("%d-%d", time.Now().UnixNano(), requestCounter)
}

func requestIDMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		r.Header.Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r)
	}
}

func panicRecoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				cid := r.Header.Get("X-Request-ID")
				ctx := logger.WithCorrelationID(r.Context(), cid)
				logger.С(ctx).Error("ПАНИКА в обработчике", slog.Any("ошибка", err), slog.String("путь", r.URL.Path))
				apierror.InternalError(w, cid, "внутренняя ошибка сервера")
			}
		}()
		next.ServeHTTP(w, r)
	}
}

func timeoutMiddleware(next http.HandlerFunc, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		if duration > timeout {
			cid := r.Header.Get("X-Request-ID")
			ctx := logger.WithCorrelationID(r.Context(), cid)
			logger.С(ctx).Warn("Медленный запрос", slog.String("метод", r.Method), slog.String("путь", r.URL.Path), slog.Duration("длительность", duration), slog.Duration("лимит", timeout))
		}
	}
}

func corsMiddleware(next http.HandlerFunc, allowedMethods []string, allowedOrigins map[string]struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if _, ok := allowedOrigins[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	}
}

// main — точка входа API Gateway.
// Конфигурирует все маршруты, создаёт reverse proxy для каждого микросервиса,
// оборачивает обработчики в CORS middleware и запускает HTTP-сервер.
func main() {
	logger.Init("api-gateway")

	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")
	toolsURL := getEnv("TOOLS_SERVICE_URL", "http://localhost:8082")
	agentURL := getEnv("AGENT_SERVICE_URL", "http://localhost:8083")
	port := getEnv("GATEWAY_PORT", "8080")

	rlLimit, _ := strconv.Atoi(getEnv("RATE_LIMIT_RPS", "60"))
	rlWindow, _ := time.ParseDuration(getEnv("RATE_LIMIT_WINDOW", "1m"))
	rateLimiter := middleware.NewRateLimiter(rlLimit, rlWindow)
	rateLimitMW := middleware.RateLimitMiddleware(rateLimiter)
	slog.Info("Ограничитель частоты настроен", slog.Int("лимит", rlLimit), slog.Duration("окно", rlWindow))

	// Предохранители от отказов для каждого бэкенда
	cbMemory := middleware.NewCircuitBreaker(5, 30*time.Second)
	cbTools := middleware.NewCircuitBreaker(5, 30*time.Second)
	cbAgent := middleware.NewCircuitBreaker(10, 30*time.Second)

	// Мидлварь распределённой трассировки
	traceMW := middleware.TracingMiddleware("api-gateway")

	// Парсим URL для создания reverse proxy
	memoryTarget, _ := url.Parse(memoryURL)
	toolsTarget, _ := url.Parse(toolsURL)
	agentTarget, _ := url.Parse(agentURL)

	// Таблица маршрутов: путь → целевой сервис, методы, режим проксирования.
	// Strip=true: удаляет префикс пути (например, /memory/search → /search)
	// Strip=false: передаёт путь как есть (например, /chat → /chat)
	routes := []struct {
		Path    string   // Префикс URL-пути
		Target  *url.URL // Целевой микросервис
		Methods []string // Разрешённые HTTP-методы
		Strip   bool     // Удалять ли префикс пути при проксировании
	}{
		// Маршруты с удалением префикса — для сервисов с собственной маршрутизацией
		{Path: "/memory/", Target: memoryTarget, Methods: []string{"GET", "POST", "DELETE"}, Strip: true},
		{Path: "/tools/", Target: toolsTarget, Methods: []string{"GET", "POST", "DELETE"}, Strip: true},
		{Path: "/agents/", Target: agentTarget, Methods: []string{"GET", "POST", "DELETE"}, Strip: true},
		// Маршруты без удаления префикса — точные пути agent-service
		{Path: "/models", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		{Path: "/update-model", Target: agentTarget, Methods: []string{"POST"}, Strip: false},
		{Path: "/avatar", Target: agentTarget, Methods: []string{"POST"}, Strip: false},
		{Path: "/avatar-info", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		{Path: "/prompts/load", Target: agentTarget, Methods: []string{"POST"}, Strip: false},
		{Path: "/prompts", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		{Path: "/agent/prompt", Target: agentTarget, Methods: []string{"POST"}, Strip: false},
		{Path: "/chat", Target: agentTarget, Methods: []string{"POST"}, Strip: false},
		// Новые маршруты для облачных провайдеров и рабочих пространств
		{Path: "/providers", Target: agentTarget, Methods: []string{"GET", "POST"}, Strip: false},
		{Path: "/cloud-models", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		{Path: "/workspaces", Target: agentTarget, Methods: []string{"GET", "POST", "DELETE"}, Strip: false},
		// Маршрут для статистики обучения агентов (проксируется на agent-service)
		{Path: "/learning-stats", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		// Яндекс.Диск — облачное хранилище (проксируется на tools-service)
		// Все операции: просмотр, загрузка, скачивание, создание папок, удаление, перемещение, поиск
		{Path: "/ydisk/", Target: toolsTarget, Methods: []string{"GET", "POST", "DELETE"}, Strip: false},
		// Статика аватаров: без удаления префикса, чтобы /uploads/... шёл как есть
		{Path: "/uploads/", Target: agentTarget, Methods: []string{"GET"}, Strip: false},
		// Системные логи (проксируется на agent-service)
		{Path: "/logs", Target: agentTarget, Methods: []string{"GET", "POST", "PATCH"}, Strip: false},
		// Проверка здоровья через memory-service
		{Path: "/health", Target: memoryTarget, Methods: []string{"GET"}, Strip: false},
	}

	// Загружаем белый список доменов для CORS
	allowedOrigins := parseAllowedOrigins()

	// Регистрируем обработчики с CORS для каждого маршрута
	for _, r := range routes {
		var proxy http.Handler
		if r.Strip {
			// Режим с удалением префикса: /memory/search → /search
			proxy = gates.NewCustomProxy(r.Target, r.Path)
		} else {
			// Режим без удаления: /chat → /chat
			proxy = gates.NewProxyWithoutStrip(r.Target)
		}
		// Оборачиваем proxy в CORS middleware с проверкой допустимых HTTP-методов
		routeTimeout := 60 * time.Second
		if r.Path == "/chat" {
			routeTimeout = 300 * time.Second
		}

		// Выбираем предохранитель по целевому сервису
		var cb *middleware.CircuitBreaker
		var svcName string
		switch r.Target {
		case memoryTarget:
			cb = cbMemory
			svcName = "memory"
		case toolsTarget:
			cb = cbTools
			svcName = "tools"
		default:
			cb = cbAgent
			svcName = "agent"
		}
		cbMW := middleware.CircuitBreakerMiddleware(cb, svcName)

		handler := requestIDMiddleware(
			traceMW(
				rateLimitMW(
					panicRecoveryMiddleware(
						timeoutMiddleware(
							cbMW(
								corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
									cid := req.Header.Get("X-Request-ID")
									ctx := logger.WithCorrelationID(req.Context(), cid)
									logger.С(ctx).Info("Проксирование запроса", slog.String("метод", req.Method), slog.String("путь", req.URL.Path), slog.String("маршрут", r.Path), slog.String("цель", r.Target.Host))
									for _, m := range r.Methods {
										if m == req.Method {
											proxy.ServeHTTP(w, req)
											return
										}
									}
									logger.С(ctx).Warn("Метод не разрешён", slog.String("метод", req.Method), slog.String("путь", req.URL.Path))
									apierror.MethodNotAllowed(w, cid)
								}), r.Methods, allowedOrigins),
							),
							routeTimeout,
						),
					),
				),
			),
		)

		http.Handle(r.Path, handler)
	}

	http.HandleFunc("/metrics", middleware.MetricsHandler)

	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 320 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("API Gateway запускается", slog.String("порт", port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Ошибка сервера", slog.String("ошибка", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Получен сигнал завершения", slog.String("сигнал", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Ошибка при завершении сервера", slog.String("ошибка", err.Error()))
	}
	slog.Info("Сервер корректно остановлен")
}

// getEnv — вспомогательная функция для чтения переменной окружения.
// Если переменная не задана или пуста, возвращает значение по умолчанию.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
