# Модуль слоя хранения для RAG (актуальная версия)

## Обзор

Документ описывает **логический слой хранения** в текущей концепции Eternal RAG.
Слой не зашивается в одну конкретную БД и не требует упоминания legacy-реализаций.

## Роль слоя хранения

- хранение фактов (`facts`);
- хранение чанков файлов (`files`);
- хранение обучающих знаний (`learnings`) с версионированием;
- фильтрация по `workspace_id`;
- поддержка аудита и метрик retrieval.

## Контрактные требования к storage-слою

1. **Стабильные ID и метаданные** для каждого объекта памяти.
2. **Версионность** (increment `version` + связь `superseded_by/previous_version_id`).
3. **Soft-delete**, а не hard-delete для критичных сущностей памяти.
4. **Изоляция workspace** на уровне выборок и операций изменения.
5. **Поддержка ranking metadata** (`importance`, `reliability`, `frequency`, `created_at`).

## Точки интеграции в проекте

- API и бизнес-логика: `memory-service/app/main.py`, `memory-service/app/memory.py`.
- Конфиг ранжирования и backup-checks: `memory-service/app/config.py`.
- Модели запросов/ответов: `memory-service/app/models.py`.

## Проверки качества

- Unit: `memory-service/tests/test_ranking.py`.
- API/integration: `memory-service/tests/test_main.py`.

## Эксплуатационные endpoint-ы

- `GET /audit/logs`
- `GET /metrics/retrieval`
- `GET /backup/checks`

## Статус документа

Этот файл заменяет прежние описания, завязанные на конкретную legacy-векторную БД.
