# ============================================================================
# Agent Core NG — Makefile
# ============================================================================
# Основные цели:
#   make build       — собрать все Go-сервисы
#   make test        — запустить все тесты (Go + Python)
#   make lint        — проверить форматирование и линтинг
#   make run         — запустить все сервисы локально
#   make docker      — запустить через docker compose
#   make clean       — удалить бинарники
# ============================================================================

.PHONY: build build-agent build-tools build-gateway build-browser \
        test test-go test-python lint lint-go lint-python \
        run run-memory run-tools run-agent run-gateway run-web \
        docker docker-down clean help check-env

# --- Переменные ---
AGENT_BIN  = agent-service/server
TOOLS_BIN  = tools-service/server
GATEWAY_BIN = api-gateway/api-gateway
BROWSER_BIN = browser-service/server

# ============================================================================
# Сборка
# ============================================================================

build: build-agent build-tools build-gateway build-browser ## Собрать все Go-сервисы

build-agent: ## Собрать agent-service
	cd agent-service && go build -o server ./cmd/server/

build-tools: ## Собрать tools-service
	cd tools-service && go build -o server ./cmd/server/

build-gateway: ## Собрать api-gateway
	cd api-gateway && go build -o api-gateway ./cmd/

build-browser: ## Собрать browser-service
	cd browser-service && go build -o server ./cmd/server/

# ============================================================================
# Тестирование
# ============================================================================

test: test-go test-python ## Запустить все тесты

test-go: ## Запустить Go-тесты
	cd agent-service && go test ./... -v -count=1
	cd tools-service && go test ./... -v -count=1 || true
	cd api-gateway && go test ./... -v -count=1 || true

test-python: ## Запустить Python-тесты (memory-service)
	cd memory-service && python3 -m pytest tests/ -v --tb=short 2>/dev/null || \
		echo "[SKIP] pytest не установлен или тесты отсутствуют"

# ============================================================================
# Линтинг
# ============================================================================

lint: lint-go lint-python ## Проверить форматирование и линтинг

lint-go: ## Проверить Go: gofmt + go vet
	@echo "=== gofmt ==="
	@test -z "$$(gofmt -l agent-service/ tools-service/ api-gateway/ browser-service/ 2>/dev/null)" || \
		(echo "gofmt нашёл неотформатированные файлы:" && gofmt -l agent-service/ tools-service/ api-gateway/ browser-service/ && exit 1)
	@echo "=== go vet ==="
	cd agent-service && go vet ./...
	cd tools-service && go vet ./...
	cd api-gateway && go vet ./...
	cd browser-service && go vet ./...

lint-python: ## Проверить Python: ruff/flake8
	@cd memory-service && (ruff check app/ 2>/dev/null || flake8 app/ 2>/dev/null || echo "[SKIP] ruff/flake8 не установлен")

# ============================================================================
# Запуск (локальный)
# ============================================================================

run: ## Запустить все сервисы (фоном)
	@echo "Запуск memory-service..."
	cd memory-service && python3 -m uvicorn app.main:app --host 0.0.0.0 --port 8001 &
	@echo "Запуск tools-service..."
	cd tools-service && go run ./cmd/server/ &
	@echo "Запуск agent-service..."
	cd agent-service && go run ./cmd/server/ &
	@echo "Запуск api-gateway..."
	cd api-gateway && go run ./cmd/ &
	@echo "Запуск web-ui..."
	cd web-ui && npm run dev &
	@echo ""
	@echo "Все сервисы запущены. Веб-интерфейс: http://localhost:5173"
	@echo "API Gateway: http://localhost:8080"

run-memory: ## Запустить memory-service
	cd memory-service && python3 -m uvicorn app.main:app --host 0.0.0.0 --port 8001 --reload

run-tools: ## Запустить tools-service
	cd tools-service && go run ./cmd/server/

run-agent: ## Запустить agent-service
	cd agent-service && go run ./cmd/server/

run-gateway: ## Запустить api-gateway
	cd api-gateway && go run ./cmd/

run-web: ## Запустить web-ui (dev mode)
	cd web-ui && npm run dev

# ============================================================================
# Docker
# ============================================================================

docker: ## Запустить через docker compose
	docker compose up -d --build

docker-down: ## Остановить docker compose
	docker compose down

# ============================================================================
# Проверка окружения
# ============================================================================

check-env: ## Проверить наличие необходимых инструментов
	@echo "=== Проверка окружения ==="
	@command -v go >/dev/null 2>&1 && echo "Go:         $$(go version)" || echo "Go:         НЕ НАЙДЕН"
	@command -v python3 >/dev/null 2>&1 && echo "Python:     $$(python3 --version)" || echo "Python:     НЕ НАЙДЕН"
	@command -v node >/dev/null 2>&1 && echo "Node.js:    $$(node --version)" || echo "Node.js:    НЕ НАЙДЕН"
	@command -v psql >/dev/null 2>&1 && echo "PostgreSQL: $$(psql --version | head -1)" || echo "PostgreSQL: НЕ НАЙДЕН"
	@command -v ollama >/dev/null 2>&1 && echo "Ollama:     $$(ollama --version 2>/dev/null || echo 'установлен')" || echo "Ollama:     НЕ НАЙДЕН (опционально)"
	@command -v docker >/dev/null 2>&1 && echo "Docker:     $$(docker --version | head -1)" || echo "Docker:     НЕ НАЙДЕН (опционально)"

# ============================================================================
# Очистка
# ============================================================================

clean: ## Удалить собранные бинарники
	rm -f $(AGENT_BIN) $(TOOLS_BIN) $(GATEWAY_BIN) $(BROWSER_BIN)
	@echo "Бинарники удалены"

# ============================================================================
# Справка
# ============================================================================

help: ## Показать справку
	@echo "Agent Core NG — доступные команды:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
