#!/usr/bin/env bash
# ============================================================================
# Agent Core NG — Скрипт установки для Linux
# ============================================================================
# Устанавливает все компоненты системы:
#   - memory-service (Python/FastAPI + ChromaDB)
#   - tools-service (Go)
#   - agent-service (Go)
#   - api-gateway (Go)
#   - web-ui (React/Vite)
#
# Использование:
#   chmod +x install.sh
#   sudo ./install.sh
#
# После установки сервисы запускаются автоматически через systemd.
# Веб-интерфейс доступен по адресу http://localhost:3000
# API Gateway доступен по адресу http://localhost:8080
# ============================================================================

set -euo pipefail

# Цвета для вывода в терминал
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # Без цвета

# Директории установки
INSTALL_DIR="/opt/agent-core"
CONFIG_DIR="/etc/agent-core"
DATA_DIR="/var/lib/agent-core"
LOG_DIR="/var/log/agent-core"
BIN_DIR="/usr/local/bin"

# Пользователь для запуска сервисов
SERVICE_USER="agent-core"

# Порты сервисов
PORT_GATEWAY=8080
PORT_MEMORY=8001
PORT_TOOLS=8082
PORT_AGENT=8083
PORT_WEB=3000

# ============================================================================
# Вспомогательные функции
# ============================================================================

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[OK]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "Этот скрипт должен быть запущен от root (sudo ./install.sh)"
    fi
}

# ============================================================================
# Шаг 1: Проверка системных зависимостей
# ============================================================================

check_dependencies() {
    info "Проверка системных зависимостей..."

    local missing=()

    # Проверка Go
    if ! command -v go &> /dev/null; then
        missing+=("golang-go")
    else
        local go_version
        go_version=$(go version | grep -oP '\d+\.\d+' | head -1)
        info "Go найден: версия $go_version"
    fi

    # Проверка Python
    if ! command -v python3 &> /dev/null; then
        missing+=("python3")
    else
        local py_version
        py_version=$(python3 --version 2>&1 | grep -oP '\d+\.\d+')
        info "Python найден: версия $py_version"
    fi

    # Проверка pip
    if ! command -v pip3 &> /dev/null; then
        missing+=("python3-pip")
    fi

    # Проверка Node.js
    if ! command -v node &> /dev/null; then
        missing+=("nodejs")
    else
        local node_version
        node_version=$(node --version)
        info "Node.js найден: версия $node_version"
    fi

    # Проверка npm
    if ! command -v npm &> /dev/null; then
        missing+=("npm")
    fi

    # Проверка PostgreSQL
    if ! command -v psql &> /dev/null; then
        missing+=("postgresql")
    else
        info "PostgreSQL найден"
    fi

    # Проверка git
    if ! command -v git &> /dev/null; then
        missing+=("git")
    fi

    # Установка недостающих зависимостей
    if [[ ${#missing[@]} -gt 0 ]]; then
        info "Установка недостающих пакетов: ${missing[*]}"
        apt-get update -qq
        apt-get install -y -qq "${missing[@]}"
        success "Зависимости установлены"
    else
        success "Все системные зависимости найдены"
    fi

    # Проверка наличия python3-venv
    if ! dpkg -l python3-venv &> /dev/null; then
        info "Установка python3-venv..."
        apt-get install -y -qq python3-venv
    fi
}

# ============================================================================
# Шаг 2: Создание пользователя и директорий
# ============================================================================

setup_directories() {
    info "Создание директорий и пользователя..."

    # Создание системного пользователя (если не существует)
    if ! id "$SERVICE_USER" &> /dev/null; then
        useradd --system --no-create-home --shell /usr/sbin/nologin "$SERVICE_USER"
        success "Пользователь $SERVICE_USER создан"
    else
        info "Пользователь $SERVICE_USER уже существует"
    fi

    # Создание директорий
    mkdir -p "$INSTALL_DIR"
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"/{chroma,temp,uploads,prompts}
    mkdir -p "$LOG_DIR"

    # Установка прав
    chown -R "$SERVICE_USER":"$SERVICE_USER" "$DATA_DIR"
    chown -R "$SERVICE_USER":"$SERVICE_USER" "$LOG_DIR"

    success "Директории созданы"
}

# ============================================================================
# Шаг 3: Настройка PostgreSQL
# ============================================================================

setup_postgresql() {
    info "Настройка PostgreSQL..."

    # Запуск PostgreSQL если не запущен
    if ! systemctl is-active --quiet postgresql; then
        systemctl start postgresql
        systemctl enable postgresql
    fi

    # Создание пользователя и базы данных
    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='agentcore'" | grep -q 1; then
        info "Пользователь PostgreSQL 'agentcore' уже существует"
    else
        sudo -u postgres psql -c "CREATE USER agentcore WITH PASSWORD 'agentcore';"
        success "Пользователь PostgreSQL создан"
    fi

    if sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='agentcore'" | grep -q 1; then
        info "База данных 'agentcore' уже существует"
    else
        sudo -u postgres psql -c "CREATE DATABASE agentcore OWNER agentcore;"
        success "База данных создана"
    fi
}

# ============================================================================
# Шаг 4: Копирование исходного кода
# ============================================================================

copy_source() {
    info "Копирование исходного кода..."

    # Определяем откуда копировать (текущая директория скрипта)
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

    # Копируем все компоненты
    for component in agent-service tools-service api-gateway memory-service web-ui; do
        if [[ -d "$script_dir/$component" ]]; then
            cp -r "$script_dir/$component" "$INSTALL_DIR/"
            info "  Скопирован: $component"
        else
            warn "  Не найден: $component (пропущен)"
        fi
    done

    # Копируем конфигурационные файлы
    if [[ -d "$script_dir/prompts" ]]; then
        cp -r "$script_dir/prompts" "$DATA_DIR/"
    fi

    # Копируем docker-compose.yml (для опционального Docker-режима)
    if [[ -f "$script_dir/docker-compose.yml" ]]; then
        cp "$script_dir/docker-compose.yml" "$INSTALL_DIR/"
    fi

    success "Исходный код скопирован в $INSTALL_DIR"
}

# ============================================================================
# Шаг 5: Сборка Go-сервисов
# ============================================================================

build_go_services() {
    info "Сборка Go-сервисов..."

    for service in agent-service tools-service api-gateway; do
        if [[ -d "$INSTALL_DIR/$service" ]]; then
            info "  Сборка $service..."
            (cd "$INSTALL_DIR/$service" && go build -o "$BIN_DIR/agent-${service}" ./cmd/server 2>/dev/null || \
             cd "$INSTALL_DIR/$service" && go build -o "$BIN_DIR/agent-${service}" ./cmd/main.go 2>/dev/null || \
             cd "$INSTALL_DIR/$service" && go build -o "$BIN_DIR/agent-${service}" ./cmd/...)
            success "  $service собран -> $BIN_DIR/agent-${service}"
        fi
    done
}

# ============================================================================
# Шаг 6: Настройка Python-окружения для memory-service
# ============================================================================

setup_memory_service() {
    info "Настройка memory-service (Python)..."

    local venv_dir="$INSTALL_DIR/memory-service/venv"

    # Создание виртуального окружения
    python3 -m venv "$venv_dir"

    # Установка зависимостей
    "$venv_dir/bin/pip" install --upgrade pip -q
    "$venv_dir/bin/pip" install -r "$INSTALL_DIR/memory-service/requirements.txt" -q

    # Создание директорий для данных
    mkdir -p "$DATA_DIR/chroma" "$DATA_DIR/temp"

    success "memory-service настроен (виртуальное окружение: $venv_dir)"
}

# ============================================================================
# Шаг 7: Сборка веб-интерфейса
# ============================================================================

build_web_ui() {
    info "Сборка веб-интерфейса..."

    if [[ -d "$INSTALL_DIR/web-ui" ]]; then
        (cd "$INSTALL_DIR/web-ui" && npm install --silent 2>/dev/null && npm run build 2>/dev/null)

        # Установка serve для раздачи статики
        if ! command -v serve &> /dev/null; then
            npm install -g serve --silent
        fi

        success "Веб-интерфейс собран"
    else
        warn "web-ui не найден, пропускаем"
    fi
}

# ============================================================================
# Шаг 8: Создание конфигурационного файла
# ============================================================================

create_config() {
    info "Создание конфигурации..."

    cat > "$CONFIG_DIR/agent-core.env" <<EOF
# ============================================================================
# Agent Core NG — Конфигурация
# ============================================================================
# Этот файл содержит все переменные окружения для работы системы.
# Отредактируйте его для настройки облачных провайдеров и параметров.
# После изменения перезапустите сервисы:
#   sudo systemctl restart agent-*.service
# ============================================================================

# --- PostgreSQL ---
DATABASE_URL=postgres://agentcore:agentcore@localhost:5432/agentcore?sslmode=disable

# --- Порты сервисов ---
AGENT_SERVICE_PORT=${PORT_AGENT}
TOOLS_SERVICE_PORT=${PORT_TOOLS}
MEMORY_SERVICE_PORT=${PORT_MEMORY}
GATEWAY_PORT=${PORT_GATEWAY}

# --- URL сервисов (для API Gateway) ---
AGENT_SERVICE_URL=http://localhost:${PORT_AGENT}
TOOLS_SERVICE_URL=http://localhost:${PORT_TOOLS}
MEMORY_SERVICE_URL=http://localhost:${PORT_MEMORY}

# --- CORS ---
CORS_ALLOWED_ORIGINS=http://localhost:${PORT_WEB},http://localhost:5173

# --- ChromaDB (memory-service) ---
CHROMA_DIR=${DATA_DIR}/chroma
TEMP_DIR=${DATA_DIR}/temp
EMBEDDING_MODEL=all-MiniLM-L6-v2

# --- Ollama (локальные модели) ---
OLLAMA_URL=http://localhost:11434

# --- OpenRouter (сотни облачных моделей через один API-ключ) ---
# Получите ключ на https://openrouter.ai/keys
# OPENROUTER_API_KEY=sk-or-...

# --- OpenAI ---
# OPENAI_API_KEY=sk-...

# --- Anthropic (Claude) ---
# ANTHROPIC_API_KEY=sk-ant-...

# --- YandexGPT ---
# YANDEX_API_KEY=...
# YANDEX_FOLDER_ID=...

# --- GigaChat (Сбер) ---
# GIGACHAT_CLIENT_ID=...
# GIGACHAT_CLIENT_SECRET=...

# --- Яндекс.Диск (облачное хранилище) ---
# Получите OAuth-токен на https://oauth.yandex.ru/
# YANDEX_DISK_TOKEN=...

# --- Рабочая директория (для tools-service) ---
WORK_DIR=/home
EOF

    chmod 600 "$CONFIG_DIR/agent-core.env"
    chown root:root "$CONFIG_DIR/agent-core.env"

    success "Конфигурация создана: $CONFIG_DIR/agent-core.env"
}

# ============================================================================
# Шаг 9: Создание systemd-юнитов
# ============================================================================

create_systemd_units() {
    info "Создание systemd-юнитов..."

    # --- memory-service ---
    cat > /etc/systemd/system/agent-memory.service <<EOF
[Unit]
Description=Agent Core NG — Memory Service (ChromaDB + RAG + обучение)
After=network.target
Wants=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
EnvironmentFile=${CONFIG_DIR}/agent-core.env
WorkingDirectory=${INSTALL_DIR}/memory-service
ExecStart=${INSTALL_DIR}/memory-service/venv/bin/uvicorn app.main:app --host 0.0.0.0 --port ${PORT_MEMORY}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    # --- tools-service ---
    cat > /etc/systemd/system/agent-tools.service <<EOF
[Unit]
Description=Agent Core NG — Tools Service (команды, файлы, Яндекс.Диск)
After=network.target agent-memory.service
Wants=network.target

[Service]
Type=simple
User=root
EnvironmentFile=${CONFIG_DIR}/agent-core.env
ExecStart=${BIN_DIR}/agent-tools-service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    # --- agent-service ---
    cat > /etc/systemd/system/agent-agent.service <<EOF
[Unit]
Description=Agent Core NG — Agent Service (агенты, LLM, оркестрация)
After=network.target postgresql.service agent-memory.service
Wants=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
EnvironmentFile=${CONFIG_DIR}/agent-core.env
ExecStart=${BIN_DIR}/agent-agent-service
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    # --- api-gateway ---
    cat > /etc/systemd/system/agent-gateway.service <<EOF
[Unit]
Description=Agent Core NG — API Gateway (маршрутизация, CORS)
After=network.target agent-agent.service agent-tools.service agent-memory.service
Wants=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
EnvironmentFile=${CONFIG_DIR}/agent-core.env
ExecStart=${BIN_DIR}/agent-api-gateway
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    # --- web-ui ---
    cat > /etc/systemd/system/agent-web.service <<EOF
[Unit]
Description=Agent Core NG — Web UI (React)
After=network.target agent-gateway.service
Wants=network.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_USER}
Environment=VITE_API_GATEWAY_URL=http://localhost:${PORT_GATEWAY}
WorkingDirectory=${INSTALL_DIR}/web-ui
ExecStart=/usr/bin/npx serve -s dist -l ${PORT_WEB}
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    # Перезагрузка systemd
    systemctl daemon-reload

    success "systemd-юниты созданы"
}

# ============================================================================
# Шаг 10: Запуск сервисов
# ============================================================================

start_services() {
    info "Запуск сервисов..."

    local services=(agent-memory agent-tools agent-agent agent-gateway agent-web)

    for svc in "${services[@]}"; do
        systemctl enable "$svc.service" 2>/dev/null
        systemctl start "$svc.service" 2>/dev/null || true
        if systemctl is-active --quiet "$svc.service"; then
            success "  $svc запущен"
        else
            warn "  $svc не удалось запустить (проверьте: journalctl -u $svc -n 20)"
        fi
    done
}

# ============================================================================
# Шаг 11: Вывод итоговой информации
# ============================================================================

print_summary() {
    echo ""
    echo -e "${GREEN}============================================================================${NC}"
    echo -e "${GREEN}  Agent Core NG — Установка завершена!${NC}"
    echo -e "${GREEN}============================================================================${NC}"
    echo ""
    echo -e "  Веб-интерфейс:  ${BLUE}http://localhost:${PORT_WEB}${NC}"
    echo -e "  API Gateway:    ${BLUE}http://localhost:${PORT_GATEWAY}${NC}"
    echo -e "  Memory Service: ${BLUE}http://localhost:${PORT_MEMORY}${NC}"
    echo -e "  Tools Service:  ${BLUE}http://localhost:${PORT_TOOLS}${NC}"
    echo -e "  Agent Service:  ${BLUE}http://localhost:${PORT_AGENT}${NC}"
    echo ""
    echo -e "  Конфигурация:   ${YELLOW}${CONFIG_DIR}/agent-core.env${NC}"
    echo -e "  Данные:         ${YELLOW}${DATA_DIR}${NC}"
    echo -e "  Логи:           ${YELLOW}${LOG_DIR}${NC}"
    echo ""
    echo -e "  ${YELLOW}Для настройки облачных провайдеров отредактируйте:${NC}"
    echo -e "    sudo nano ${CONFIG_DIR}/agent-core.env"
    echo -e "    sudo systemctl restart agent-agent agent-tools agent-gateway"
    echo ""
    echo -e "  ${YELLOW}Управление сервисами:${NC}"
    echo -e "    sudo systemctl status agent-*        # Статус всех сервисов"
    echo -e "    sudo systemctl restart agent-*       # Перезапуск всех сервисов"
    echo -e "    sudo systemctl stop agent-*          # Остановка всех сервисов"
    echo -e "    journalctl -u agent-agent -f         # Логи agent-service в реальном времени"
    echo ""
    echo -e "  ${YELLOW}Для удаления:${NC}"
    echo -e "    sudo ./install.sh --uninstall"
    echo ""
}

# ============================================================================
# Удаление (--uninstall)
# ============================================================================

uninstall() {
    info "Удаление Agent Core NG..."

    # Остановка и удаление сервисов
    local services=(agent-memory agent-tools agent-agent agent-gateway agent-web)
    for svc in "${services[@]}"; do
        systemctl stop "$svc.service" 2>/dev/null || true
        systemctl disable "$svc.service" 2>/dev/null || true
        rm -f "/etc/systemd/system/$svc.service"
    done
    systemctl daemon-reload

    # Удаление бинарников
    rm -f "$BIN_DIR/agent-tools-service"
    rm -f "$BIN_DIR/agent-agent-service"
    rm -f "$BIN_DIR/agent-api-gateway"

    # Удаление директорий
    rm -rf "$INSTALL_DIR"
    rm -rf "$CONFIG_DIR"
    rm -rf "$LOG_DIR"

    # Данные не удаляем по умолчанию
    warn "Данные в $DATA_DIR НЕ удалены (удалите вручную при необходимости)"
    warn "База PostgreSQL НЕ удалена (удалите вручную при необходимости)"

    success "Agent Core NG удалён"
}

# ============================================================================
# Главная функция
# ============================================================================

main() {
    echo ""
    echo -e "${BLUE}============================================================================${NC}"
    echo -e "${BLUE}  Agent Core NG — Установщик для Linux${NC}"
    echo -e "${BLUE}============================================================================${NC}"
    echo ""

    # Обработка аргументов
    if [[ "${1:-}" == "--uninstall" ]]; then
        check_root
        uninstall
        exit 0
    fi

    check_root

    # Выполнение шагов установки
    check_dependencies
    setup_directories
    setup_postgresql
    copy_source
    build_go_services
    setup_memory_service
    build_web_ui
    create_config
    create_systemd_units
    start_services
    print_summary
}

# Запуск
main "$@"
