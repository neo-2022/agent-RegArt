# Security-аудит Agent Core NG (локальный trusted-режим Admin-агента)

Дата оценки: 2026-02-22

## Контекст и допущения

Проект целевой: **локальный**, для **Admin-агента с максимальными правами** на ПК пользователя.
Это снижает требования к многоарендной изоляции, но **не отменяет необходимость жёстких предохранителей** от:
- случайно опасных действий модели,
- промпт-инъекций,
- компрометации UI/API на локальной машине,
- lateral movement при пробросе портов/туннелей.

## Обновлённая оценка

**8.6/10 для локального trusted-режима** после внедрённых патчей auth/RBAC/policy в `tools-service`.

## Что сделано в этом PR (конкретные security-патчи)

### 1) AuthN для tools-service (Bearer tokens)

В `tools-service` добавлен обязательный токен-контроль (кроме `TOOLS_AUTH_TOKENS` пуст — тогда fallback в legacy-режим с warning):
- новая переменная окружения: `TOOLS_AUTH_TOKENS=token1:viewer,token2:operator,token3:admin`
- middleware `withAuth(requiredRole, tokenRoles, next)` проверяет:
  - наличие `Authorization: Bearer <token>`
  - валидность токена
  - достаточность роли

### 2) RBAC по endpoint-ам

Введены роли:
- `viewer` — чтение/инфо
- `operator` — операционные изменения
- `admin` — высокорисковые действия (включая `/execute`)

Текущее разграничение:
- `viewer`: `/read`, `/list`, `/findapp`, `/ydisk/info`, `/ydisk/list`, `/ydisk/download`, `/ydisk/search`
- `operator`: `/write`, `/delete`, `/launchapp`, `/ydisk/upload`, `/ydisk/mkdir`, `/ydisk/delete`, `/ydisk/move`
- `admin`: `/execute`, `/addautostart`

### 3) Policy для команд в tools-service

Кроме глобального allowlist и blocked-patterns добавлена role-aware политика исполнения:
- `ExecuteCommandForRole(role, command)`
- `RoleCommandPolicy` для `viewer`/`operator`
- `admin` остаётся с полным allowlist (trusted-режим), но всё ещё защищён dangerous/pattern checks

Результат: даже при валидном токене `viewer` не сможет выполнить mutate/опасные команды через `/execute`.

## Остаточные риски (для локального режима)

1. Если `TOOLS_AUTH_TOKENS` не задан, сервис работает в legacy-режиме (осознанный fallback для обратной совместимости).
2. RBAC token-only (без mTLS/JWT rotation) — нормально для single-host, но не для сетевой экспозиции.
3. Командный движок всё ещё на `bash -c`; это ожидаемо для Admin-агента, но требует дисциплины policy.

## Практические рекомендации по запуску (hardening)

### P0
- Всегда задавать `TOOLS_AUTH_TOKENS` в окружении (не запускать в legacy fallback).
- Не публиковать `tools-service` наружу; доступ только через localhost или закрытую docker-сеть.
- Отдельный admin-token хранить вне UI (например, `.env` + права 600).

### P1
- Добавить TTL/rotation токенов (минимум ручная ротация по расписанию).
- Вынести policy в конфиг (`yaml/json`) для удобной ревизии.
- Добавить audit-log по схеме: `request_id`, `token_hash`, `role`, `endpoint`, `decision`.

### P2
- Опциональный mTLS между gateway и tools-service.
- Макс. лимит времени/ресурсов для `/execute` (CPU, memory, timeout per role).
- Выделенный профиль policy для "maintenance window" (временное расширение команд оператору).

## Краткий вывод

Для заявленного сценария «локальный Admin-агент с максимальными правами» архитектура стала существенно безопаснее за счёт **реально применённых технических барьеров**: AuthN, RBAC, role-based command policy. Это переводит систему из «доверенной, но хрупкой» в «доверенную с контролируемым риском».
