# Web UI (React + TypeScript + Vite)

Фронтенд Agent Core NG на React/Vite.

## Запуск и проверка

- Установка зависимостей: `npm install`
- Dev-режим: `npm run dev`
- Тесты: `npm run test`
- Сборка: `npm run build`

## Базовая архитектура UI/UX

В текущей итерации реализован baseline трёхзонного интерфейса **без overlay-перекрытий**:

- левая панель (чаты и пространства),
- центральная область (диалог),
- правая системная панель (RAG / Логи / Настройки).

Ключевые модули:

- токены ширин/анимаций: `src/config/uiLayout.ts`;
- модель состояний RAG-панели: `src/config/ragPanelState.ts`;
- пользовательские UI-настройки: `src/config/uiPreferences.ts`.
- централизованные фильтры логов: `src/config/logFilters.ts`. (значения фильтра + русские подписи в UI).

## Premium UI-компоненты

### ModelPopover (`src/components/ModelPopover.tsx`)

Заменяет стандартный `<select>` на premium popover с поиском, карточками моделей и метаданными.
Поддерживает локальные (Ollama) и облачные модели с отображением семейства, размера, цен.

### PromptPanel (`src/components/PromptPanel.tsx`)

Inline-панель управления промптами агента. Встраивается в карточку агента вместо overlay-модала.
Поддерживает выбор файлов промптов, inline-редактирование и сохранение.

### Soft Depth CSS System

4-уровневая система глубины в `src/styles/App.css`:

- `--shadow-surface` / `--shadow-raised` / `--shadow-elevated` / `--shadow-floating`
- `--glow-accent` / `--glow-success` / `--glow-error`
- `--gradient-surface` / `--gradient-raised` / `--gradient-card`
- `--transition-fast` / `--transition-normal` / `--transition-slow`

### RAG File Explorer

Панель RAG с функциями файлового менеджера:
- Expand/collapse папок (chevron + анимация)
- Drag & drop загрузка файлов
- Поиск и сортировка
- UI-состояния (Empty/Loading/Error/Processing/Outdated/Conflict)

## Тестовое покрытие

### UI-компоненты
- `src/components/ModelPopover.test.ts` — интерфейсы, логика поиска, экспорт
- `src/components/PromptPanel.test.ts` — интерфейсы, синхронизация, статусы, экспорт

### UI-конфиги
- `src/config/uiLayout.test.ts`
- `src/config/ragPanelState.test.ts`
- `src/config/uiPreferences.test.ts`
- `src/config/logFilters.test.ts`

## Настройки интерфейса (persisted)

Сохраняются в localStorage:

- `compactSidebar`
- `reducedMotion`
- `inferenceProfile` (`economy | standard | deep`) — явный индикатор баланса скорость/качество/расход.

## Валидация API на границе клиента

Для исключения падений вида `*.map/find is not a function` добавлены нормализаторы ответов API:

- `src/config/workspaceApi.ts` — `GET /workspaces`
- `src/config/modelsApi.ts` — `GET /models`
- `src/config/providersApi.ts` — `GET /providers`
- `src/config/agentsApi.ts` — `GET /agents/`

Поддерживаются формы:

- «чистый» массив,
- объект-обёртка с массивом в одном из ключей (`workspaces|items|data` для workspaces, `models|items|data` для models, `providers|items|data` для providers, `agents|items|data` для agents).

Невалидные элементы отфильтровываются, а необязательные поля приводятся к безопасным дефолтам.

Тесты:

- `src/config/workspaceApi.test.ts`
- `src/config/modelsApi.test.ts`
- `src/config/providersApi.test.ts`
- `src/config/agentsApi.test.ts`
