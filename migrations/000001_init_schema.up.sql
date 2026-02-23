-- 000001_init_schema.up.sql
-- Начальная миграция: создание всех таблиц Agent Core NG.
-- Порядок: расширения → независимые таблицы → таблицы с внешними ключами.

-- ============================================================================
-- 1. Расширения PostgreSQL
-- ============================================================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================================
-- 2. Workspaces — рабочие пространства (независимая таблица)
-- ============================================================================
CREATE TABLE IF NOT EXISTS workspaces (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    name          TEXT NOT NULL,
    path          TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_workspaces_deleted_at ON workspaces (deleted_at);

-- ============================================================================
-- 3. Chats — чаты (ссылается на workspaces)
-- ============================================================================
CREATE TABLE IF NOT EXISTS chats (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name          TEXT NOT NULL DEFAULT '',
    user_id       TEXT NOT NULL DEFAULT '',
    workspace_id  BIGINT REFERENCES workspaces(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_chats_deleted_at ON chats (deleted_at);

-- ============================================================================
-- 4. Agents — агенты (ссылается на workspaces)
-- ============================================================================
CREATE TABLE IF NOT EXISTS agents (
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    name                TEXT NOT NULL,
    prompt              TEXT NOT NULL DEFAULT '',
    llm_model           TEXT NOT NULL DEFAULT '',
    provider            TEXT NOT NULL DEFAULT 'ollama',
    supports_tools      BOOLEAN NOT NULL DEFAULT FALSE,
    avatar              TEXT NOT NULL DEFAULT '',
    current_prompt_file TEXT NOT NULL DEFAULT '',
    workspace_id        BIGINT REFERENCES workspaces(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_name ON agents (name);
CREATE INDEX IF NOT EXISTS idx_agents_deleted_at ON agents (deleted_at);

-- ============================================================================
-- 5. Messages — сообщения (ссылается на agents и chats)
-- ============================================================================
CREATE TABLE IF NOT EXISTS messages (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    role          TEXT NOT NULL DEFAULT '',
    content       TEXT NOT NULL DEFAULT '',
    tool_call_id  TEXT NOT NULL DEFAULT '',
    agent_id      BIGINT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    chat_id       UUID REFERENCES chats(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_messages_deleted_at ON messages (deleted_at);

-- ============================================================================
-- 6. PromptFiles — файлы промптов (независимая таблица)
-- ============================================================================
CREATE TABLE IF NOT EXISTS prompt_files (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    agent_name    TEXT NOT NULL DEFAULT '',
    filename      TEXT NOT NULL DEFAULT '',
    content       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_prompt_files_deleted_at ON prompt_files (deleted_at);

-- ============================================================================
-- 7. ModelToolSupport — кэш поддержки инструментов (независимая таблица)
-- ============================================================================
CREATE TABLE IF NOT EXISTS model_tool_supports (
    model_name      TEXT PRIMARY KEY,
    supports_tools  BOOLEAN NOT NULL DEFAULT FALSE,
    family          TEXT NOT NULL DEFAULT '',
    parameter_size  TEXT NOT NULL DEFAULT '',
    is_code_model   BOOLEAN NOT NULL DEFAULT FALSE,
    suitable_roles  TEXT NOT NULL DEFAULT '[]',
    role_notes      TEXT NOT NULL DEFAULT '{}',
    checked_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- 8. ProviderConfigs — настройки облачных LLM-провайдеров (независимая таблица)
-- ============================================================================
CREATE TABLE IF NOT EXISTS provider_configs (
    id                    BIGSERIAL PRIMARY KEY,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at            TIMESTAMPTZ,
    provider_name         TEXT NOT NULL,
    api_key               TEXT NOT NULL DEFAULT '',
    base_url              TEXT NOT NULL DEFAULT '',
    folder_id             TEXT NOT NULL DEFAULT '',
    scope                 TEXT NOT NULL DEFAULT '',
    service_account_json  TEXT NOT NULL DEFAULT '',
    enabled               BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_configs_provider_name ON provider_configs (provider_name);
CREATE INDEX IF NOT EXISTS idx_provider_configs_deleted_at ON provider_configs (deleted_at);

-- ============================================================================
-- 9. SystemLogs — централизованные логи (независимая таблица)
-- ============================================================================
CREATE TABLE IF NOT EXISTS system_logs (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    level         TEXT NOT NULL,
    service       TEXT NOT NULL,
    message       TEXT NOT NULL,
    details       TEXT NOT NULL DEFAULT '',
    resolved      BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_system_logs_deleted_at ON system_logs (deleted_at);
CREATE INDEX IF NOT EXISTS idx_system_logs_level ON system_logs (level);
CREATE INDEX IF NOT EXISTS idx_system_logs_service ON system_logs (service);

-- ============================================================================
-- 10. RagDocuments — документы базы знаний RAG (ссылается на workspaces)
-- ============================================================================
CREATE TABLE IF NOT EXISTS rag_documents (
    id            BIGSERIAL PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at    TIMESTAMPTZ,
    title         TEXT NOT NULL,
    content       TEXT NOT NULL DEFAULT '',
    source        TEXT NOT NULL DEFAULT '',
    chunk_index   INT NOT NULL DEFAULT 0,
    total_chunks  INT NOT NULL DEFAULT 0,
    workspace_id  BIGINT REFERENCES workspaces(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_rag_documents_deleted_at ON rag_documents (deleted_at);
