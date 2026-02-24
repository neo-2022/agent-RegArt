# Agent Core NG — AI-ассистент системного администратора

## Описание проекта

**Agent Core NG** — система на микросервисной архитектуре с единственным AI-агентом **Admin**, выполняющим роль полноценного системного администратора ПК. Проект предназначен для локального запуска на Linux и предоставляет мощного ассистента с возможностью выполнения системных команд, работы с файлами, мониторинга, использования долговременной памяти (RAG), интеграции с облачными хранилищами и подключения LLM-моделей.

Система поддерживает локальные модели через **Ollama** (Qwen, Llama, Nemotron, Mistral и др.) и российские облачные провайдеры: **YandexGPT** и **GigaChat**.

---

## Основные возможности

### Единый агент Admin — системный администратор ПК

Агент Admin обладает полным доступом к ПК пользователя:

- **Управление файлами:** чтение, запись, редактирование, удаление, листинг директорий
- **Выполнение команд:** запуск любых bash-команд на ПК
- **Мониторинг системы:** CPU, RAM, диски, температура, загрузка, логи
- **Администрирование:** сервисы, cron-задачи, автозагрузка, установка пакетов
- **Работа с кодом:** чтение, редактирование, отладка, запуск скриптов
- **Поиск в интернете:** DuckDuckGo, SearXNG (без API-ключей, работает в РФ)
- **Работа с Яндекс.Диском:** файлы, папки, загрузка, скачивание
- **RAG (база знаний):** семантический поиск и хранение информации

### Адаптация под мощность модели

Система автоматически определяет мощность модели:

- **Сильные модели (7B+):** получают полный набор базовых инструментов + расширенные инструменты администрирования. Сами строят цепочку действий.
- **Слабые модели (3B и меньше):** получают составные LEGO-скилы — один вызов выполняет цепочку действий (например, `full_system_report` вместо 5 отдельных команд).

### LLM-провайдеры

- **Ollama** — локальные модели (Qwen 3, Llama 3.1, Nemotron, Mistral и др.)
- **YandexGPT** — российский облачный провайдер от Яндекса
- **GigaChat** — российский облачный провайдер от Сбера

### Долговременная память (RAG)

- Векторная база данных ChromaDB для хранения фактов
- Семантический поиск по смыслу (не по ключевым словам)
- Индексация файлов из локальной файловой системы
- Автоматическое разбиение больших файлов на чанки
- Подключение контекста из RAG к запросам агента

### Система обучения агента

- Модель накапливает свою базу знаний
- После успешных взаимодействий извлекаются ключевые факты и паттерны
- Перед каждым запросом подгружаются релевантные знания из базы
- Знания хранятся в ChromaDB с привязкой к конкретной модели
- Статистика обучения доступна через API

### Облачное хранилище — Яндекс.Диск

- Просмотр файлов и папок
- Загрузка и скачивание файлов
- Создание и удаление папок
- Перемещение и копирование
- Поиск файлов по типу медиа

### Браузерные инструменты (browser-service)

- Навигация: открытие URL, получение DOM, текста, заголовка
- Скриншоты и PDF через headless Chrome
- Ввод: клавиатура, мышь, скроллинг через xdotool
- Управление окнами через wmctrl
- HTTP-запросы: GET/POST/PUT/DELETE/PATCH/HEAD/OPTIONS
- Поиск в интернете: DuckDuckGo, SearXNG (бесплатно, без API-ключей)
- Маскировка под ботов: Googlebot, YandexBot, Bingbot

### Система навыков (Skills)

- YAML-описания навыков в директории `skills/`
- 5 встроенных навыков: поиск, скриншот, краулер, доступность, PDF

### Рабочие пространства (Workspaces)

- Отдельная история чатов для каждого пространства
- Отдельная конфигурация агента (модель, промпт)
- Привязка к рабочей директории на ПК

### Веб-интерфейс

- Чат с агентом Admin
- Панель выбора моделей (локальные + облачные)
- Прикрепление файлов и голосовой ввод через Web Speech API
- RAG-поиск с визуализацией результатов
- Блоки кода с подсветкой синтаксиса и кнопкой копирования

---

## Архитектура

```
+---------------------------------------------------------------------------+
|                    Клиент (браузер, localhost:5173)                        |
|                         web-ui (React/Vite)                               |
+-------------------------------------+-------------------------------------+
                                      | HTTP
                                      v
+---------------------------------------------------------------------------+
|                   API Gateway (Go, :8080)                                 |
|  Маршрутизация:                                                           |
|    /memory/*  -> memory-service    /agents/* -> agent-service             |
|    /tools/*   -> tools-service     /ydisk/*  -> tools-service             |
|    /chat, /models, /providers      -> agent-service                       |
|    /workspaces, /learning-stats    -> agent-service                       |
|  CORS: настраиваемый белый список доменов                                 |
+---------+--------------------+--------------------+-----------------------+
          |                    |                    |
          v                    v                    v
+-----------------+  +-----------------+  +-------------------------+
| memory-service  |  | tools-service   |  |    agent-service        |
| Python/FastAPI  |  |     Go          |  |        Go               |
| :8001           |  | :8082           |  | :8083                   |
|                 |  |                 |  |                         |
| ChromaDB        |  | Команды         |  | Агент Admin             |
| Sentence-       |  | Файлы           |  | LLM провайдеры:         |
|  transformers   |  | Система         |  |   Ollama, YandexGPT,    |
| RAG-поиск       |  | Яндекс.Диск    |  |   GigaChat              |
| Обучение        |  | Приложения      |  | PostgreSQL              |
+-----------------+  +-----------------+  +-------------------------+
                                                    |
                                           +--------+--------+
                                           v                  v
                                    +--------------+  +--------------+
                                    |browser-service|  |   skills/    |
                                    |    Go :8084   |  |  YAML-файлы  |
                                    |               |  |  навыков     |
                                    | Chrome headless| |              |
                                    | xdotool/wmctrl|  |              |
                                    | DuckDuckGo    |  |              |
                                    +--------------+  +--------------+
```

### Микросервисы

#### memory-service (Python/FastAPI, порт 8001)

Векторная память и обучение агента. ChromaDB + sentence-transformers (`all-MiniLM-L6-v2`).

| Эндпоинт | Описание |
|-----------|----------|
| `POST /facts` | Добавление факта в базу знаний |
| `GET /search?q=...` | Семантический поиск |
| `POST /files` | Индексация файла (чанки) |
| `POST /learn` | Сохранение знания модели |
| `GET /recall?model=...&query=...` | Извлечение знаний модели |
| `GET /learning-stats?model=...` | Статистика обучения |
| `GET /health` | Проверка здоровья |

#### tools-service (Go, порт 8082)

Системные команды, файлы, мониторинг, Яндекс.Диск.

| Эндпоинт | Описание |
|-----------|----------|
| `POST /execute` | Выполнение команды |
| `POST /read` | Чтение файла |
| `POST /write` | Запись файла |
| `POST /list` | Список файлов |
| `POST /delete` | Удаление файла |
| `GET /sysinfo` | Информация о системе |
| `GET /sysload` | Загрузка CPU/RAM/диски |
| `GET /cputemp` | Температура CPU |
| `POST /findapp` | Поиск приложения |
| `POST /launchapp` | Запуск приложения |
| `POST /addautostart` | Добавить в автозагрузку |
| `GET /ydisk/info` | Информация о Яндекс.Диске |
| `GET /ydisk/list?path=/` | Содержимое папки |
| `GET /ydisk/download?path=...` | Скачивание файла |
| `POST /ydisk/upload` | Загрузка файла |
| `POST /ydisk/mkdir` | Создание папки |
| `POST /ydisk/delete` | Удаление файла/папки |
| `POST /ydisk/move` | Перемещение |
| `GET /ydisk/search?media_type=...` | Поиск файлов |

#### agent-service (Go, порт 8083)

Ядро системы — управление агентом Admin, подключение к LLM, обучение, рабочие пространства.

| Эндпоинт | Описание |
|-----------|----------|
| `GET /agents` | Информация об агенте |
| `POST /chat` | Отправка сообщения агенту |
| `GET /models` | Список доступных моделей |
| `POST /update-model` | Обновление модели агента |
| `GET /providers` | Список провайдеров |
| `POST /providers` | Регистрация провайдера |
| `GET /cloud-models` | Модели облачных провайдеров |
| `GET /workspaces` | Список рабочих пространств |
| `POST /workspaces` | Создание/обновление пространства |
| `GET /learning-stats` | Статистика обучения |
| `POST /avatar` | Загрузка аватара |
| `GET /prompts` | Список файлов промптов |
| `POST /rag/add` | Добавление документа в RAG |
| `POST /rag/add-folder` | Индексация папки в RAG |
| `POST /rag/search` | Поиск по RAG |
| `GET /rag/files` | Список файлов в RAG |
| `GET /rag/stats` | Статистика RAG |
| `DELETE /rag/delete` | Удаление из RAG |

#### api-gateway (Go, порт 8080)

Единая точка входа. Маршрутизация запросов + CORS.

#### web-ui (React/TypeScript/Vite, порт 5173)

Веб-интерфейс для взаимодействия с агентом Admin.

---

## Инструменты агента Admin

### Базовые инструменты (для сильных моделей 7B+)

| Инструмент | Описание |
|------------|----------|
| `execute(command)` | Выполнить bash-команду |
| `read(path)` | Прочитать файл |
| `write(path, content)` | Записать файл |
| `list(path?)` | Содержимое директории |
| `delete(path)` | Удалить файл |
| `sysinfo()` | ОС, архитектура, хост, пользователь |
| `sysload()` | Загрузка CPU, память, диски |
| `cputemp()` | Температура процессора |
| `findapp(name)` | Найти .desktop файл приложения |
| `launchapp(desktop_file)` | Запустить приложение |
| `addautostart(app_name)` | Добавить в автозагрузку |
| `web_search(query)` | Поиск в интернете |
| `web_get(url)` | Получить содержимое страницы |
| `rag_search(query)` | Поиск по базе знаний |
| `rag_add(content, metadata)` | Добавить в базу знаний |

### Расширенные инструменты Admin

| Инструмент | Описание |
|------------|----------|
| `debug_code(file_path, args?)` | Запустить скрипт и вернуть stdout/stderr |
| `edit_file(file_path, old_text, new_text)` | Заменить текст в файле |
| `configure_agent(agent_name, model?, provider?, prompt?)` | Настроить агента |
| `get_agent_info(agent_name)` | Информация об агенте |
| `list_models_for_role(role)` | Список моделей с рекомендациями |
| `view_logs(level?, service?, limit?)` | Системные логи |

### Составные LEGO-скилы (для слабых моделей 3B и меньше)

| Скил | Описание |
|------|----------|
| `full_system_report()` | Полный отчёт о системе за один вызов |
| `check_stack(tools)` | Проверка установленных инструментов |
| `diagnose_service(service_name)` | Диагностика сервиса |
| `web_research(query)` | Поиск + получение контента |
| `check_resources_batch(urls)` | Проверка доступности нескольких URL |
| `generate_report(path, title, content)` | Создание отчёта с верификацией |
| `create_script(path, content, executable?)` | Создание скрипта |
| `run_commands(commands)` | Выполнение нескольких команд |
| `setup_cron_job(schedule, command)` | Настройка cron-задачи |
| `setup_git_automation(repo_path, ...)` | Автоматизация Git |
| `project_init(name, type, path?)` | Инициализация проекта |
| `install_packages(packages, manager?)` | Установка пакетов |

---

## Установка

### Системные требования

- **ОС:** Ubuntu 22.04+ (или другой Linux с systemd)
- **CPU:** 4+ ядер (рекомендуется AMD Ryzen 7+ / Intel Core i7+)
- **RAM:** 16 GB минимум (32+ GB рекомендуется для больших моделей)
- **GPU:** NVIDIA с 8+ GB VRAM для локальных моделей (опционально)
- **Диск:** 50+ GB свободного места (SSD рекомендуется)

### Предварительные требования

- Git
- Go 1.22+
- Python 3.10+ и pip
- PostgreSQL 14+
- Node.js 20+ и npm
- Ollama (для локальных LLM-моделей)

### Быстрый старт через Docker Compose

```bash
git clone https://github.com/neo-2022/agent-RegArt.git
cd agent-RegArt
cp .env.example .env   # отредактируйте при необходимости
docker compose up -d   # PostgreSQL + ChromaDB + memory-service
```

Docker Compose поднимает:
- **PostgreSQL** (порт 5432) — хранение чатов, агентов, провайдеров
- **ChromaDB** (порт 8000) — векторное хранилище для RAG
- **memory-service** (порт 8001) — API памяти и обучения

Остальные сервисы (agent-service, tools-service, api-gateway, web-ui) запускаются локально — см. раздел «Сборка и запуск» ниже.

### Быстрая установка (install.sh)

```bash
git clone https://github.com/neo-2022/agent-RegArt.git
cd agent-RegArt
chmod +x install.sh
sudo ./install.sh
```

Установщик автоматически:
- Установит все зависимости (Go, Node.js, Python, PostgreSQL)
- Соберёт все Go-сервисы
- Установит Python-зависимости для memory-service
- Соберёт web-ui
- Создаст systemd-сервисы для автозапуска
- Настроит PostgreSQL

### Ручная установка

#### 1. Клонирование репозитория

```bash
git clone https://github.com/neo-2022/agent-RegArt.git
cd agent-RegArt
```

#### 2. Переменные окружения

Создайте файл `.env` в корне проекта или экспортируйте переменные:

```bash
# === PostgreSQL (для agent-service) ===
export DATABASE_URL="postgres://agent_user:password@localhost:5432/agentcore?sslmode=disable"

# === API Gateway (CORS) ===
export CORS_ALLOWED_ORIGINS="http://localhost:3000,http://localhost:5173"

# === Сервисные URL (значения по умолчанию) ===
export TOOLS_SERVICE_URL="http://localhost:8082"
export BROWSER_SERVICE_URL="http://localhost:8084"
export MEMORY_SERVICE_URL="http://localhost:8001"
export AGENT_SERVICE_URL="http://localhost:8083"
export GATEWAY_URL="http://localhost:8080"

# === Ollama (локальные модели) ===
export OLLAMA_URL="http://localhost:11434"

# === Облачные LLM-провайдеры (опционально) ===
# YandexGPT
export YANDEXGPT_API_KEY="..."
export YANDEXGPT_FOLDER_ID="..."

# GigaChat (Сбер)
export GIGACHAT_CLIENT_SECRET="..."
export GIGACHAT_CLIENT_ID="..."
export GIGACHAT_SCOPE="GIGACHAT_API_PERS"

# === Облачное хранилище ===
export YANDEX_DISK_TOKEN="y0_..."

# === Web-UI ===
export VITE_API_URL="http://localhost:8080"
```

#### 3. Настройка PostgreSQL

```bash
sudo -u postgres psql -c "CREATE USER agent_user WITH PASSWORD 'your_password';"
sudo -u postgres psql -c "CREATE DATABASE agentcore OWNER agent_user;"
```

#### 4. Установка Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3.1:8b
```

#### 5. Сборка и запуск

```bash
# memory-service
cd memory-service && pip install -r requirements.txt
python -m uvicorn app.main:app --host 0.0.0.0 --port 8001 &

# tools-service
cd ../tools-service && go build -o tools-service ./cmd/server/ && ./tools-service &

# agent-service
cd ../agent-service && go build -o agent-service ./cmd/server/ && ./agent-service &

# api-gateway
cd ../api-gateway && go build -o api-gateway ./cmd/ && ./api-gateway &

# web-ui
cd ../web-ui && npm install && npm run dev &
```

#### 6. Открытие в браузере

Перейдите на http://localhost:5173

---

## Конфигурация облачных провайдеров

### YandexGPT

1. Создайте каталог и сервисный аккаунт в [Yandex Cloud Console](https://console.yandex.cloud)
2. Создайте API-ключ: IAM -> Сервисные аккаунты -> Ключи API
3. Скопируйте Folder ID с главной страницы каталога
4. Настройте через UI (раздел "Провайдеры") или переменные окружения:
   ```bash
   export YANDEXGPT_API_KEY="..."
   export YANDEXGPT_FOLDER_ID="..."
   ```

### GigaChat (Сбер)

1. Зарегистрируйтесь на [developers.sber.ru](https://developers.sber.ru/portal/products/gigachat)
2. Создайте проект и получите Authorization Key
3. Настройте через UI или переменные окружения:
   ```bash
   export GIGACHAT_CLIENT_SECRET="..."
   export GIGACHAT_CLIENT_ID="..."
   export GIGACHAT_SCOPE="GIGACHAT_API_PERS"
   ```

### Яндекс.Диск

1. Получите OAuth-токен: https://oauth.yandex.ru
2. `export YANDEX_DISK_TOKEN="y0_..."`

---

## Структура проекта

```
agent-RegArt/
+-- agent-service/           # Сервис агента Admin (Go)
|   +-- cmd/server/          # Точка входа (main.go)
|   +-- internal/
|       +-- llm/             # LLM-провайдеры (ollama, yandexgpt, gigachat)
|       +-- models/          # Модели данных
|       +-- repository/      # Работа с PostgreSQL
|       +-- db/              # Подключение к БД
|       +-- tools/           # Определения инструментов
|       +-- metrics/         # Prometheus метрики
+-- tools-service/           # Сервис инструментов (Go)
|   +-- cmd/server/          # HTTP-обработчики (вкл. Яндекс.Диск)
|   +-- internal/executor/   # Выполнение команд, файлы, система
+-- memory-service/          # Сервис памяти (Python/FastAPI)
|   +-- app/                 # FastAPI, ChromaDB, обучение
+-- api-gateway/             # API Gateway (Go)
|   +-- cmd/main.go          # Маршрутизация + CORS
+-- browser-service/         # Браузерный микросервис (Go)
|   +-- cmd/server/          # HTTP-сервер (порт 8084)
|   +-- internal/
|       +-- browser/         # Навигация, скриншоты, PDF, DOM
|       +-- input/           # Клавиатура, мышь, окна, буфер обмена
|       +-- search/          # DuckDuckGo, SearXNG
|       +-- crawler/         # HTTP-запросы с маскировкой
|       +-- access/          # Проверка доступности URL
+-- skills/                  # YAML-описания навыков
+-- web-ui/                  # Веб-интерфейс (React/Vite)
|   +-- src/                 # Компоненты, стили
+-- .github/workflows/       # CI/CD (GitHub Actions)
+-- Makefile                 # Сборка, тесты, линтинг, запуск
+-- .env.example             # Шаблон переменных окружения
+-- ROADMAP.md               # Дорожная карта проекта
+-- CHANGELOG.md             # История изменений
+-- install.sh               # Установщик для Linux
+-- README.md                # Этот файл
```

---

## Сборка и тестирование через Makefile

Проект включает `Makefile` с основными целями:

```bash
make build        # собрать все Go-сервисы
make test         # запустить все тесты (Go + Python)
make test-go      # только Go-тесты
make test-python  # только Python-тесты (memory-service)
make lint         # проверка форматирования и линтинг
make run          # запустить все сервисы локально
make docker       # запустить через docker compose
make check-env    # проверить переменные окружения
make clean        # удалить бинарники
make help         # справка по всем целям
```

### Тесты

**Go-тесты** (agent-service):
```bash
cd agent-service && go test ./... -v
```

**Python-тесты** (memory-service):
```bash
cd memory-service && python -m pytest tests/ -v
```

---

## CI/CD

GitHub Actions автоматически проверяет при каждом push и PR:
1. **agent-service** -- `go build`, `go test`, `gofmt`, `go vet`
2. **tools-service** -- `go build`, `gofmt`, `go vet`
3. **api-gateway** -- `go build`, `gofmt`, `go vet`
4. **web-ui** -- `tsc --noEmit`, `vite build`
5. **memory-service** -- `pytest`

---

## Принципы кода

- **Хардкод запрещён:** все URL и пути настраиваются через переменные окружения
- **Подробное логирование:** каждый обработчик логирует вход/выход/ошибки
- **Динамический резолв путей:** `~`, пустые пути автоматически разрешаются через `os.UserHomeDir()`
- **Единый агент:** вся логика сосредоточена в одном агенте Admin для простоты и надёжности

---

## Hardening (P0)

### Execution Profiles

- `ADMIN_TRUSTED_MODE=true` — Admin получает полный доступ без role-ограничений. Риск помечается в логах, операции не блокируются.
- `SAFE_MODE=true` — режим только для тестов/демо. Блокируются деструктивные команды (rm -rf, shutdown и т.п.).

### Единый формат ошибок

Все сервисы (agent-service, tools-service, api-gateway) возвращают ошибки в едином формате:

```json
{
  "code": "BAD_REQUEST",
  "message": "Описание ошибки",
  "hint": "Подсказка для исправления",
  "request_id": "1234567890-1",
  "retryable": false
}
```

### Correlation-ID

Заголовок `X-Request-ID` генерируется на входе в каждый сервис (если не передан клиентом) и пробрасывается через всю цепочку вызовов. Позволяет трассировать запрос через все микросервисы.

### Tool-call chain logging

Каждый вызов инструмента логируется с полями: инструмент, параметры (без секретов), длительность, outcome (success/error). Ошибки LLM и ошибки инструментов разделены явно (`[LLM-ERROR]` vs `[TOOL-CALL]`).

---

## Лицензия

MIT License

## Авторы

- **Neo** -- архитектор и владелец проекта

## Модели
Система автоматически загружает модели из `agent-service/models`. Подробнее в [документации](docs/ru/models-setup.md).
