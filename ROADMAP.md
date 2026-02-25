# Дорожная карта Agent Core NG

## v0.2.0 — Стабильность и тесты (ближайшее)

- [x] Единый агент Admin (удалены coder/novice)
- [x] CI/CD: GitHub Actions (go build, gofmt, go vet, tsc, vite build)
- [x] Документация: README, CHANGELOG, PLAN.md
- [x] Валидация переменных окружения при старте
- [x] `.env.example` с документацией всех переменных
- [x] Makefile для сборки, тестов, линтинга
- [x] Docker Compose для полного стека (PostgreSQL + Qdrant + сервисы)
- [ ] Unit-тесты для ключевых пакетов (repository, models, llm)
- [ ] Интеграционные тесты memory-service (FastAPI TestClient)

## v0.2.1 — Безопасность и надёжность (текущее)

- [x] tools-service: DangerousCommands + расширенные BlockedPatterns
- [x] tools-service: path traversal защита (ForbiddenPaths, AllowedSystemFiles)
- [x] tools-service: SSRF-защита в browser.go (валидация URL, блокировка приватных адресов)
- [x] tools-service: удалены опасные команды из AllowedCommands (rm, chmod, chown, sudo, bash, sh)
- [x] tools-service: лимит размера файлов (MaxFileSize 10MB)
- [x] memory-service: size limits на входные данные (MAX_TEXT_LENGTH, MAX_QUERY_LENGTH)
- [x] memory-service: embedding_model_version в метаданных коллекций Qdrant
- [x] memory-service: предупреждение при смене модели эмбеддингов
- [x] memory-service: Skill Engine (CRUD навыков, семантический поиск)
- [x] memory-service: Graph Engine (узлы, связи, авто-связи relates_to)
- [x] memory-service: семантическое обнаружение противоречий
- [x] memory-service: TTL и политика переиндексации
- [x] web-ui: Premium UI (ModelPopover, PromptPanel, Soft Depth CSS)
- [x] web-ui: RAG File Explorer (pin, soft-delete, move, rename, content search)
- [x] web-ui: Skills Panel (создание, поиск, просмотр навыков)
- [x] docker-compose: MinIO, Neo4j, Redis добавлены в стек
- [x] api-gateway: request-id middleware (X-Request-ID)
- [x] api-gateway: panic recovery middleware
- [x] api-gateway: timeout middleware (60s / 300s для /chat)
- [x] AppError — единая модель ошибок для Go-сервисов (agent-service/internal/apperror)
- [x] RAG token limiting — TruncateChunk + LimitContext (MaxChunkLen, MaxContextLen)
- [x] web-ui: ErrorBoundary компонент (перехват ошибок рендеринга)

## v0.3.0 — Контракты и архитектура

- [ ] OpenAPI/proto контракты для всех сервисов
- [ ] Версионирование API (v1/v2)
- [ ] Рефакторинг на domain/usecase/infrastructure (Clean Architecture)
- [ ] Circuit breaker для межсервисных вызовов
- [ ] Rate limiting на api-gateway
- [ ] Context propagation (OpenTelemetry tracing)

## v0.4.0 — Расширение возможностей

- [ ] WebSocket для real-time обновлений чата (вместо HTTP polling)
- [ ] Индексация файлов с Яндекс.Диска в RAG
- [ ] RAG TTL и политика переиндексации
- [ ] Расширение browser-service: автоматизация взаимодействия с AI-чатами
- [ ] Chrome AI интеграция (Gemini Nano, Prompt API, Summarizer API)
- [ ] Улучшенная система обучения: автоматический сброс устаревших знаний
- [ ] Sandbox для tools-service (docker-in-docker)

## v0.5.0 — Масштабирование

- [ ] Многопользовательский режим (аутентификация, авторизация)
- [ ] Изоляция данных между пользователями
- [ ] Prometheus-метрики + Grafana-дашборды
- [ ] Тестирование под нагрузкой
- [ ] Упаковка в deb-пакет

## v1.0.0 — Долгосрочные цели

- Плагинная архитектура для новых LLM-провайдеров
- Мобильное приложение (React Native)
- Распределённый режим (несколько экземпляров agent-service)
- Голосовой ввод и синтез речи
- Автономный режим работы (offline) через локальные модели
- E2E тесты, coverage отчёты, security scan (SAST)
- Полный Docker Compose: все Go-сервисы собираются и запускаются в контейнерах

## Известные ограничения

- Однопользовательский режим (нет аутентификации)
- HTTP polling вместо WebSocket
- Модели без tool calling помечаются, но tool calling для них недоступен
- browser-service требует X11/Wayland для xdotool и wmctrl
- tools-service выполняет команды без sandbox (запланировано docker-in-docker)
