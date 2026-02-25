# RAG-модуль (актуальная концепция Eternal RAG)

## Обзор

Модуль RAG обеспечивает поиск контекста и долговременную память для агента на основе `memory-service`.
Документ синхронизирован с текущей концепцией: без привязки к конкретной legacy-векторной БД.

## Ключевые принципы

- **Единая точка доступа**: retrieval и learnings через API `memory-service`.
- **Версионирование знаний**: каждое знание имеет `learning_key` + `version`.
- **Soft-delete**: удаление знаний выполняется через статус `deleted`, без физического hard-delete.
- **Изоляция по workspace**: поиск и learnings фильтруются по `workspace_id`.
- **Наблюдаемость**: аудит событий памяти и метрики retrieval.

## Основные API-потоки

### Retrieval

- `POST /search`
  - принимает `query`, `top_k`, `workspace_id`;
  - возвращает объединённый список результатов (`facts/files/learnings`) с ранжированием.

### Learnings

- `POST /learnings` — добавить новое знание (с автоматическим increment `version` при обновлениях).
- `POST /learnings/search` — поиск знаний в контексте модели и workspace.
- `DELETE /learnings/{model_name}` — soft-delete знаний модели.
- `GET /learnings/versions/{model_name}` — история версий знаний по модели.

### Наблюдаемость и эксплуатация

- `GET /audit/logs` — журнал событий памяти.
- `GET /metrics/retrieval` — агрегированные метрики поиска.
- `GET /backup/checks` — проверка readiness backup/recovery окружения.

## Ranking factors (реализовано)

В retrieval реализовано композитное ранжирование по факторам:

- `relevance` — семантическая близость (из distance);
- `importance` — метаданные записи (0..1);
- `reliability` — метаданные записи (0..1);
- `recency` — свежесть `created_at` относительно окна `RECENCY_WINDOW_DAYS`;
- `frequency` — метаданные записи (0..1).

Все веса задаются через env:

- `RANK_WEIGHT_RELEVANCE`
- `RANK_WEIGHT_IMPORTANCE`
- `RANK_WEIGHT_RELIABILITY`
- `RANK_WEIGHT_RECENCY`
- `RANK_WEIGHT_FREQUENCY`

Реализация: `memory-service/app/ranking.py`.
Тесты: `memory-service/tests/test_ranking.py`.

## Примечание по совместимости документации

Старые упоминания legacy-векторной БД удалены из актуальных описаний архитектуры.
Если в исторических ветках/архивных файлах встречаются legacy-ссылки, ориентироваться нужно на текущий `README.md` и этот документ.
