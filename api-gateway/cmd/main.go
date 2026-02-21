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
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/neo-2022/openclaw-memory/api-gateway/gates"
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
func corsMiddleware(next http.HandlerFunc, allowedMethods []string, allowedOrigins map[string]struct{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Проверяем, разрешён ли домен запроса
			if _, ok := allowedOrigins[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Preflight-запрос (OPTIONS) — отвечаем 204 без дальнейшей обработки
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
	// Загружаем URL микросервисов из переменных окружения
	memoryURL := getEnv("MEMORY_SERVICE_URL", "http://localhost:8001")
	toolsURL := getEnv("TOOLS_SERVICE_URL", "http://localhost:8082")
	agentURL := getEnv("AGENT_SERVICE_URL", "http://localhost:8083")
	port := getEnv("GATEWAY_PORT", "8080")

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
		handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			log.Printf("[GATEWAY] %s %s → %s (target: %s)", req.Method, req.URL.Path, r.Path, r.Target.Host)
			for _, m := range r.Methods {
				if m == req.Method {
					proxy.ServeHTTP(w, req)
					return
				}
			}
			log.Printf("[GATEWAY] ОТКАЗ: метод %s не разрешён для %s", req.Method, req.URL.Path)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}), r.Methods, allowedOrigins)

		http.Handle(r.Path, handler)
	}

	log.Printf("API Gateway запускается на порту %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

// getEnv — вспомогательная функция для чтения переменной окружения.
// Если переменная не задана или пуста, возвращает значение по умолчанию.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
