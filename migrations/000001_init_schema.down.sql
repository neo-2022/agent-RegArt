-- 000001_init_schema.down.sql
-- Откат начальной миграции: удаление всех таблиц в обратном порядке.
-- Порядок: таблицы с внешними ключами → независимые таблицы → расширения.

DROP TABLE IF EXISTS rag_documents CASCADE;
DROP TABLE IF EXISTS system_logs CASCADE;
DROP TABLE IF EXISTS provider_configs CASCADE;
DROP TABLE IF EXISTS model_tool_supports CASCADE;
DROP TABLE IF EXISTS prompt_files CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS agents CASCADE;
DROP TABLE IF EXISTS chats CASCADE;
DROP TABLE IF EXISTS workspaces CASCADE;
