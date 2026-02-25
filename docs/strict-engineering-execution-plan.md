# Strict Engineering Execution Plan: UI/UX + Eternal RAG

## Цель

Этот документ фиксирует **первый обязательный этап отработки** следующих спецификаций:

1. `UI_UX_Design_Spec.txt`
2. `Eternal_RAG_Architecture_Spec.txt`
3. `Полный документ проверки дизайна и UX.txt`
4. `Полный документ проверок UI + Memory + RAG.txt`

Цель этапа — перевести высокоуровневые требования в проверяемый engineering backlog с явными гейтами качества.

---

## Область ответственности этапа

Текущий этап включает:

- инвентаризацию требований по UI/UX и RAG;
- привязку требований к текущим модулям репозитория;
- формирование приоритизированного плана внедрения;
- определение критериев приёмки и тестовой стратегии для каждого потока работ.

Текущий этап **не заявляет** завершение всей архитектуры Eternal RAG; он формирует контролируемый baseline, после которого изменения выполняются сериями с обязательным прохождением тестов и проверок.

---

## Карта соответствия требований и подсистем

| Блок требований | Источник | Подсистема | Текущий статус | Следующий шаг |
|---|---|---|---|---|
| Премиальный 3-панельный layout без overlay | UI/UX spec + UX checks | `web-ui/src/App.tsx`, `web-ui/src/styles/App.css` | Частично реализовано, требуется формализация инвариантов поведения панелей | Вынести layout-параметры в централизованный конфиг + unit-тесты на state transitions |
| Адаптивность и относительные размеры | UI/UX spec + UX checks | `web-ui/src/styles/App.css` | Частично, есть mix fixed/relative | Ввести токены размеров/брейкпоинтов, покрыть snapshot/DOM-тестами |
| Premium model selector (поиск, карточки, hover/motion) | UI/UX spec | `web-ui/src/App.tsx` | Не подтверждено тестами | Добавить компонентизацию селектора и unit-тесты интерактивности |
| RAG-панель как file explorer и состояния Empty/Processing/Ready/Error/Outdated/Conflict | UI/UX spec + combined checks | `web-ui/src/App.tsx`, `memory-service` API | Частично | Ввести state machine UI и контрактные тесты API-статусов |
| Ingestion pipeline (object/meta/chunks/embeddings/graph links) | Eternal RAG spec + combined checks | `memory-service`, `agent-service` | Фрагментарно | Зафиксировать API-контракты ingestion и integration tests с моками backend storage |
| Versioning, soft delete, conflict handling | Eternal RAG spec + combined checks | `memory-service`, data layer | Частично | Добавить version model + тесты конфликтов и восстановлений |
| Retrieval composition (semantic + keyword + graph + skill + ranking factors) | Eternal RAG spec + combined checks | `agent-service/internal/rag`, `memory-service` | Частично | Ввести явные стратегии ранжирования и покрыть unit/integration |
| Безопасность (workspace isolation, ACL, audit) | Combined checks | `api-gateway`, `memory-service`, `tools-service` | Не полностью проверено | Поднять security test-suite и зафиксировать mandatory policies |
| Backup/recovery и долговременная устойчивость | Eternal RAG spec + combined checks | `docker-compose`, ops scripts | Не подтверждено автотестами | Добавить smoke-скрипты backup/restore и регламент проверок |

---

## Приоритетный backlog (итерации)

### Итерация 1 — UI foundation и отсутствие хардкода в layout

- Централизация UI design-токенов (размеры панелей, отступы, transition, z-index policy).
- Запрет жёстких значений в компонентах через единый конфиг/константы.
- Тесты:
  - unit-тесты для panel toggle state;
  - компонентные тесты адаптивных классов/режимов;
  - проверка отсутствия overlay-перекрытий в layout-контейнере.

### Итерация 2 — RAG panel state model

- Явная модель состояний RAG UI: `empty | processing | ready | error | outdated | conflict`.
- Отображение user-facing статусов и причин ошибок.
- Тесты:
  - unit-тесты state reducer;
  - e2e smoke на переключение состояний.

### Итерация 3 — Memory ingestion/versioning baseline

- Контракты API ingestion с version metadata.
- Soft delete вместо hard delete на критичных сущностях памяти.
- Тесты:
  - integration: загрузка/обновление/получение старой версии;
  - regression: конфликт версий и корректная сигнализация.

### Итерация 4 — Retrieval quality and ranking

- Явные коэффициенты ранжирования (relevance/importance/recency/reliability/frequency) через конфиг.
- Тесты:
  - unit ранжирования;
  - integration retrieval pipeline;
  - perf smoke на SLA.

---

## Quality gates (обязательные)

Для каждой итерации:

1. `lint` — зелёный.
2. `type-check` — зелёный.
3. `unit/integration` — зелёные.
4. `build` — зелёный.
5. Документация обновлена синхронно.
6. Нет TODO/FIXME без обоснования.

Если хотя бы один гейт красный — итерация не считается завершённой.

---

## Риски и смягчение

- **Риск:** большой объём требований в одном релизе.
  - **Смягчение:** выпуск итерациями с жёсткими quality gates.
- **Риск:** регрессии из-за рефакторинга UI layout.
  - **Смягчение:** снапшот/DOM-тесты layout-инвариантов.
- **Риск:** несовместимость старых memory данных с versioning.
  - **Смягчение:** миграционный слой и обратная совместимость API на переходный период.

---

## Критерии готовности всей программы работ

Программа считается завершённой, когда:

- все пункты чек-листов UX и UI+Memory+RAG закрыты;
- все ключевые потоки покрыты автоматическими тестами;
- документация и операционные инструкции актуализированы;
- подтверждены backup/restore и наблюдаемость (метрики/логи).
