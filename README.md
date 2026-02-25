# Agent Core NG — AI-ассистент системного администратора

## Описание

**Agent Core NG** — микросервисная система с единственным AI-агентом **Admin**, выполняющим роль полноценного системного администратора ПК. Проект предназначен для локального запуска на Linux и предоставляет мощного ассистента с выполнением системных команд, работой с файлами, мониторингом, долговременной памятью (Eternal RAG), интеграцией с облачными хранилищами и подключением LLM-моделей.

Поддерживаемые LLM-провайдеры:
- **Ollama** — локальные модели (Qwen, Llama, Nemotron, Mistral и др.)
- **YandexGPT** — облачный провайдер от Яндекса
- **GigaChat** — облачный провайдер от Сбера

---

## Навигация по документации

| Документ | Описание |
|----------|----------|
| `README.md` | Обзор проекта, архитектура, установка, API |
| `DEPLOYMENT_GUIDE.md` | Руководство по развёртыванию |
| `ROADMAP.md` | Дорожная карта версий |
| `CHANGELOG.md` | История изменений |
| `Eternal_RAG_Architecture_Spec.txt` | Спецификация архитектуры Eternal RAG (15 этапов) |
| `UI_UX_Design_Spec.txt` | Спецификация UI/UX дизайна |
| `docs/rag-module.md` | Описание RAG-модуля и API |
| `docs/vector-store-module.md` | Описание слоя хранения |
| `docs/openapi/` | OpenAPI-спецификации всех сервисов |
| `tasks/admin_test_tasks.md` | Тестовые задачи для агента Admin |

---

## Архитектура

```
+----------------------------------------------------------------------------+
|                     Клиент (браузер, localhost:5173)                         |
|                          web-ui (React/Vite)                                |
+--------------------------------------+-------------------------------------+
                                       | HTTP
                                       v
+----------------------------------------------------------------------------+
|                    API Gateway (Go, :8080)                                  |
|  /memory/*  -> memory-service     /agents/* -> agent-service               |
|  /tools/*   -> tools-service      /skills/* -> agent-service               |
|  /chat, /models, /providers       -> agent-service                         |
|  /graph/*, /embeddings/*          -> agent-service                         |
+--------+-------------------+-------------------+---------------------------+
         |                   |                   |
         v                   v                   v
+-----------------+ +-----------------+ +---------------------------+
| memory-service  | | tools-service   | |    agent-service           |
| Python/FastAPI  | |     Go          | |        Go                  |
| :8001           | | :8082           | | :8083                      |
|                 | |                 | |                            |
| Qdrant          | | Команды         | | Агент Admin                |
| Skill Engine    | | Файлы           | | LLM: Ollama, YandexGPT,   |
| Graph Engine    | | Система         | |      GigaChat              |
| Эмбеддинги      | | Яндекс.Диск    | | PostgreSQL                 |
| Ранжирование    | | Приложения      | | Proxy: skills/graph/embed  |
+-----------------+ +-----------------+ +---------------------------+
```

### Инфраструктура (docker-compose)

| Сервис | Порт | Назначение |
|--------|------|------------|
| **PostgreSQL 16** | 5432 | Хранение чатов, агентов, провайдеров |
| **Qdrant 1.12** | 6333 | Векторное хранилище для RAG |
| **MinIO** | 9000/9001 | S3-совместимое объектное хранилище |
| **Neo4j 5.15** | 7474/7687 | Граф знаний (связи между фактами) |
| **Redis 7** | 6379 | Кэширование, очереди, pub/sub |
| **api-gateway** | 8080 | Единая точка входа, маршрутизация, CORS |
| **agent-service** | 8083 | Ядро агента Admin, LLM-провайдеры |
| **tools-service** | 8082 | Системные команды, файлы, Яндекс.Диск |
| **memory-service** | 8001 | Eternal RAG, эмбеддинги, обучение |
| **web-ui** | 5173 | Веб-интерфейс (React/TypeScript/Vite) |

---

## Возможности агента Admin

### Системное администрирование
- Чтение, запись, редактирование, удаление файлов, листинг директорий
- Выполнение bash-команд, мониторинг CPU/RAM/дисков/температуры
- Управление сервисами, cron-задачами, автозагрузкой, пакетами
- Работа с кодом: чтение, редактирование, отладка, запуск скриптов

### Долговременная память (Eternal RAG)
- Хранение фактов, файлов, знаний модели в Qdrant
- Семантический поиск по смыслу (all-MiniLM-L6-v2, dim=384)
- Версионирование знаний (`learning_key` + `version`)
- Soft delete (удаление без потери данных)
- Семантическое обнаружение противоречий (порог 0.85)
- Композитное ранжирование (relevance, importance, reliability, recency, frequency)
- Изоляция по workspace
- TTL и политика переиндексации
- Аудит событий памяти и метрики retrieval

### Skill Engine (движок навыков)
- CRUD навыков через API (`/skills/*`)
- Автоматический поиск релевантных навыков перед каждым LLM-запросом
- Автосоздание навыков из диалога (category="skill")
- Семантический поиск по навыкам

### Graph Engine (граф знаний)
- Автоматическое создание связей `relates_to` между похожими знаниями (порог 0.7)
- API для управления узлами и связями (`/graph/*`)
- Поиск соседей и связанных знаний

### Адаптация под мощность модели
- **Сильные модели (7B+):** полный набор инструментов, самостоятельное построение цепочки действий
- **Слабые модели (3B-):** составные LEGO-скилы (один вызов = цепочка действий)

### Облачное хранилище — Яндекс.Диск
- Просмотр, загрузка, скачивание, создание/удаление папок, перемещение, поиск

### Браузерные инструменты (browser-service)
- Headless Chrome, скриншоты, PDF, DOM
- Клавиатура, мышь, управление окнами (xdotool/wmctrl)
- HTTP-запросы, поиск (DuckDuckGo, SearXNG)

---

## Веб-интерфейс

Трёхзонный интерфейс без overlay-перекрытий:
- **Левая панель** — чаты и рабочие пространства (collapse/expand)
- **Центральная область** — диалог с агентом
- **Правая панель** — RAG / Логи / Настройки / Навыки

### Premium UI-компоненты
- **ModelPopover** — premium popover с поиском, карточками моделей, метаданными
- **PromptPanel** — inline-панель управления промптами агента
- **Soft Depth CSS** — 4-уровневая система глубины (тени, градиенты, свечение)
- **RAG File Explorer** — файловый менеджер с папками, drag&drop, поиск, сортировка
  - Pin/unpin файлов, soft delete с корзиной, восстановление
  - Move между папками, inline rename, multi-select, batch delete
  - Content/semantic search с % релевантности
  - Отображение противоречий знаний
  - Индикатор статуса эмбеддингов
- **Skills Panel** — панель навыков (создание, поиск, просмотр)

Подробнее: `web-ui/README.md`

---

## API-эндпоинты

### memory-service (:8001)

| Эндпоинт | Метод | Описание |
|----------|-------|----------|
| `/health` | GET | Проверка здоровья |
| `/search` | POST | Семантический поиск (workspace_id, min_priority) |
| `/facts` | POST | Добавление факта |
| `/files` | POST | Индексация файла (чанки) |
| `/files/rename` | PATCH | Переименование файла |
| `/learnings` | POST | Сохранение знания модели |
| `/learnings/search` | POST | Поиск знаний модели |
| `/learnings/{model}` | DELETE | Soft delete знаний |
| `/learnings/versions/{model}` | GET | История версий знаний |
| `/skills` | GET/POST | Список / создание навыков |
| `/skills/{id}` | GET/PUT/DELETE | CRUD навыка |
| `/skills/search` | POST | Семантический поиск навыков |
| `/graph/nodes` | GET/POST | Узлы графа знаний |
| `/graph/edges` | GET/POST | Связи графа знаний |
| `/graph/neighbors/{id}` | GET | Соседи узла |
| `/embeddings/status` | GET | Статус модели эмбеддингов |
| `/audit/logs` | GET | Журнал событий памяти |
| `/metrics/retrieval` | GET | Метрики поиска |
| `/backup/checks` | GET | Проверка readiness backup |

### agent-service (:8083)

| Эндпоинт | Метод | Описание |
|----------|-------|----------|
| `/health` | GET | Проверка здоровья |
| `/agents` | GET | Информация об агенте |
| `/chat` | POST | Отправка сообщения агенту |
| `/models` | GET | Список моделей |
| `/update-model` | POST | Обновление модели агента |
| `/providers` | GET/POST | Список / регистрация провайдеров |
| `/workspaces` | GET/POST | Рабочие пространства |
| `/learning-stats` | GET | Статистика обучения |
| `/rag/add` | POST | Добавление документа в RAG |
| `/rag/search` | POST | Поиск по RAG |
| `/rag/files` | GET | Файлы в RAG |
| `/rag/stats` | GET | Статистика RAG |
| `/skills/*` | * | Proxy к memory-service Skills API |
| `/graph/*` | * | Proxy к memory-service Graph API |
| `/embeddings/status` | GET | Proxy к memory-service |

### tools-service (:8082)

| Эндпоинт | Метод | Описание |
|----------|-------|----------|
| `/execute` | POST | Выполнение bash-команды |
| `/read` | POST | Чтение файла |
| `/write` | POST | Запись файла |
| `/list` | POST | Список файлов |
| `/delete` | POST | Удаление файла |
| `/sysinfo` | GET | Информация о системе |
| `/sysload` | GET | Загрузка CPU/RAM/диски |
| `/cputemp` | GET | Температура CPU |
| `/ydisk/*` | * | Операции с Яндекс.Диском |

### api-gateway (:8080)

Единая точка входа. Маршрутизирует запросы к сервисам. CORS настраивается через `CORS_ALLOWED_ORIGINS`.

---

## Установка

### Системные требования

- **ОС:** Ubuntu 22.04+ (или другой Linux с systemd)
- **CPU:** 4+ ядер
- **RAM:** 16 GB минимум (32+ GB для больших моделей)
- **Диск:** 50+ GB SSD
- **GPU:** NVIDIA 8+ GB VRAM (опционально, для локальных моделей)

### Предварительные требования

- Git, Go 1.24+, Python 3.10+, Node.js 20+, Docker и Docker Compose
- Ollama (для локальных LLM-моделей)

### Быстрый старт (Docker Compose)

```bash
git clone https://github.com/neo-2022/agent-RegArt.git
cd agent-RegArt
cp .env.example .env   # отредактируйте при необходимости
docker compose up -d
```

Поднимает полный стек: PostgreSQL, Qdrant, MinIO, Neo4j, Redis + все микросервисы + web-ui.

Откройте http://localhost:5173

### Установка через install.sh

```bash
git clone https://github.com/neo-2022/agent-RegArt.git
cd agent-RegArt
chmod +x install.sh && sudo ./install.sh
```

Установщик автоматически поставит зависимости, соберёт сервисы, создаст systemd-юниты и настроит PostgreSQL.

### Ручная сборка и запуск

```bash
# memory-service
cd memory-service && pip install -r requirements.txt
python -m uvicorn app.main:app --host 0.0.0.0 --port 8001 &

# Go-сервисы
cd ../tools-service && go build -o tools-service ./cmd/server/ && ./tools-service &
cd ../agent-service && go build -o agent-service ./cmd/server/ && ./agent-service &
cd ../api-gateway && go build -o api-gateway ./cmd/ && ./api-gateway &

# web-ui
cd ../web-ui && npm install && npm run dev &
```

---

## Конфигурация

### Переменные окружения

```bash
# PostgreSQL
DATABASE_URL="postgres://agent_user:password@localhost:5432/agentcore?sslmode=disable"

# Сервисные URL
TOOLS_SERVICE_URL="http://localhost:8082"
MEMORY_SERVICE_URL="http://localhost:8001"
AGENT_SERVICE_URL="http://localhost:8083"
GATEWAY_URL="http://localhost:8080"

# Ollama
OLLAMA_URL="http://localhost:11434"

# CORS
CORS_ALLOWED_ORIGINS="http://localhost:3000,http://localhost:5173"

# Memory-service
VECTOR_BACKEND=qdrant
QDRANT_URL="http://localhost:6333"
EMBEDDING_MODEL=all-MiniLM-L6-v2
CONTRADICTION_THRESHOLD=0.85

# Ранжирование (веса)
RANK_WEIGHT_RELEVANCE=0.4
RANK_WEIGHT_IMPORTANCE=0.2
RANK_WEIGHT_RELIABILITY=0.15
RANK_WEIGHT_RECENCY=0.15
RANK_WEIGHT_FREQUENCY=0.1

# Облачные LLM (опционально)
YANDEXGPT_API_KEY="..."
YANDEXGPT_FOLDER_ID="..."
GIGACHAT_CLIENT_SECRET="..."
GIGACHAT_CLIENT_ID="..."
YANDEX_DISK_TOKEN="y0_..."
```

Полный список переменных: `.env.example`

---

## Сборка и тестирование

### Makefile

```bash
make build        # собрать все Go-сервисы
make test         # все тесты (Go + Python)
make test-go      # только Go-тесты
make test-python  # только Python-тесты
make lint         # проверка форматирования
make run          # запустить все сервисы
make docker       # запустить через docker compose
make clean        # удалить бинарники
```

### Интеграционные тесты

```bash
./integration_tests.sh   # полный стек через docker-compose
```

---

## CI/CD

GitHub Actions — 6 проверок при каждом push/PR:

1. **agent-service** — `go build`, `go test`, `gofmt`, `go vet`
2. **tools-service** — `go build`, `go test`, `gofmt`, `go vet`
3. **api-gateway** — `go build`, `gofmt`, `go vet`
4. **web-ui** — `tsc --noEmit`, `vite build`
5. **memory-service** — `pytest`
6. **безопасность** — проверка секретов

---

## Безопасность

- Path traversal защита (ForbiddenPaths, AllowedSystemFiles)
- SSRF-защита (валидация URL, блокировка приватных адресов)
- DangerousCommands + BlockedPatterns
- Лимит размера файлов (MaxFileSize 10MB)
- Size limits на входные данные memory-service
- Единый формат ошибок (AppError) с `X-Request-ID`
- Workspace isolation для данных памяти
- `ADMIN_TRUSTED_MODE` / `SAFE_MODE` для профилей безопасности

---

## Структура проекта

```
agent-RegArt/
+-- agent-service/           # Ядро агента Admin (Go)
|   +-- cmd/server/          # Точка входа, HTTP-обработчики
|   +-- internal/
|       +-- llm/             # LLM-провайдеры (ollama, yandexgpt, gigachat)
|       +-- models/          # Модели данных
|       +-- repository/      # PostgreSQL
|       +-- tools/           # Определения инструментов
+-- tools-service/           # Системные инструменты (Go)
|   +-- cmd/server/          # HTTP-сервер
|   +-- internal/executor/   # Команды, файлы, система, Яндекс.Диск
+-- memory-service/          # Eternal RAG (Python/FastAPI)
|   +-- app/
|       +-- main.py          # FastAPI endpoints
|       +-- memory.py        # Логика памяти
|       +-- skill_engine.py  # Skill Engine
|       +-- graph_engine.py  # Graph Engine
|       +-- ranking.py       # Композитное ранжирование
|       +-- ttl.py           # TTL и переиндексация
|       +-- config.py        # Конфигурация
|       +-- qdrant_store.py  # Qdrant adapter
|   +-- tests/               # pytest тесты
+-- api-gateway/             # API Gateway (Go)
|   +-- cmd/main.go          # Маршрутизация + CORS
+-- browser-service/         # Браузерный микросервис (Go)
+-- web-ui/                  # Веб-интерфейс (React/TypeScript/Vite)
|   +-- src/
|       +-- components/      # ModelPopover, PromptPanel
|       +-- config/          # uiLayout, ragPanelState, uiPreferences
|       +-- styles/          # Soft Depth CSS
+-- skills/                  # YAML-описания навыков
+-- tray-app/                # Системный трей (Python)
+-- docs/                    # Дополнительная документация
|   +-- openapi/             # OpenAPI-спецификации сервисов
|   +-- rag-module.md        # Описание RAG-модуля
|   +-- vector-store-module.md
+-- tasks/                   # Тестовые задачи для агента
+-- .github/workflows/       # CI/CD (GitHub Actions)
+-- docker-compose.yml       # Полный стек (10 сервисов)
+-- Makefile                 # Сборка, тесты, линтинг
+-- .env.example             # Шаблон переменных окружения
+-- install.sh               # Установщик для Linux
+-- deploy.sh                # Скрипт развёртывания
+-- integration_tests.sh     # Интеграционные тесты
```

---

## Лицензия

MIT License

## Автор

**Neo** — архитектор и владелец проекта
