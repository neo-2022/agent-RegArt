// Пакет models — определяет все модели данных (ORM-сущности) для agent-service.
// Используется библиотека GORM для маппинга структур Go на таблицы PostgreSQL.
// Каждая структура с gorm.Model получает автоматические поля: ID, CreatedAt, UpdatedAt, DeletedAt.
//
// Иерархия моделей:
//
//	Workspace → Chat → Message
//	Workspace → Agent
//	Agent → Message
//	Agent → ProviderConfig (через поле Provider)
//	ModelToolSupport — независимая таблица-кэш
//	PromptFile — файлы промптов
package models

import (
	"time"

	"gorm.io/gorm"
)

// Agent — модель агента системы (admin, coder, novice).
// Каждый агент имеет своё имя, модель LLM, провайдера, системный промпт и аватар.
// Агент может быть привязан к рабочему пространству (WorkspaceID).
//
// Поля:
//   - Name: уникальное имя агента (admin, coder, novice). Индексируется для быстрого поиска.
//   - Prompt: текущий системный промпт агента (может быть загружен из файла или введён вручную).
//   - LLMModel: имя модели LLM, используемой агентом (например, "llama3.1:8b", "gpt-4o").
//   - Provider: имя провайдера LLM (ollama, openai, anthropic, yandexgpt, gigachat).
//     По умолчанию "ollama" для локальных моделей.
//   - SupportsTools: поддерживает ли текущая модель вызов инструментов (tool calling).
//     Определяется автоматически при первом использовании модели.
//   - Avatar: имя файла аватара агента в директории uploads/avatars/.
//   - CurrentPromptFile: имя файла, из которого загружен текущий промпт.
//     Пустая строка, если промпт введён вручную.
//   - Messages: связь один-ко-многим с сообщениями агента.
//   - WorkspaceID: внешний ключ на рабочее пространство (может быть NULL).
type Agent struct {
	gorm.Model
	Name              string    `gorm:"uniqueIndex;not null"`           // Уникальное имя агента
	Prompt            string    `gorm:"type:text"`                      // Системный промпт
	LLMModel          string    `json:"model"`                          // Модель LLM (например, "llama3.1:8b")
	Provider          string    `json:"provider" gorm:"default:ollama"` // Провайдер (ollama, openai и др.)
	SupportsTools     bool      // Поддержка tool calling
	Avatar            string    // Имя файла аватара
	CurrentPromptFile string    `json:"prompt_file"` // Файл промпта (если загружен из файла)
	Messages          []Message // Сообщения агента
	WorkspaceID       *uint     `json:"workspace_id"` // Привязка к рабочему пространству
}

// Message — модель одного сообщения в чате.
// Хранит текст сообщения, роль отправителя и привязку к агенту и чату.
//
// Поля:
//   - Role: роль отправителя — "user" (пользователь), "assistant" (агент),
//     "system" (системное сообщение), "tool" (результат вызова инструмента).
//   - Content: текст сообщения (тип text для длинных сообщений).
//   - ToolCallID: идентификатор вызова инструмента (для tool-сообщений).
//   - AgentID: внешний ключ на агента, которому принадлежит сообщение.
//   - ChatID: внешний ключ на чат (UUID).
type Message struct {
	gorm.Model
	Role       string  // Роль: user, assistant, system, tool
	Content    string  `gorm:"type:text"` // Текст сообщения
	ToolCallID string  // ID вызова инструмента (для tool calling)
	AgentID    uint    // Внешний ключ на Agent
	Agent      Agent   // Связь с агентом
	ChatID     *string // Внешний ключ на Chat.ID (UUID), может быть NULL
}

// Chat — модель чата (сессии) пользователя.
// Каждый чат имеет уникальный UUID, может быть привязан к рабочему пространству.
// UUID генерируется автоматически на уровне PostgreSQL через gen_random_uuid().
//
// Поля:
//   - ID: уникальный идентификатор чата (UUID, генерируется PostgreSQL).
//   - Name: отображаемое имя чата.
//   - UserID: идентификатор пользователя (для будущей многопользовательности).
//   - WorkspaceID: привязка к рабочему пространству (может быть NULL).
//   - Messages: связь один-ко-многим с сообщениями чата.
type Chat struct {
	ID          string         `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"` // UUID чата
	Name        string         // Имя чата
	UserID      string         // ID пользователя (для многопользовательности)
	WorkspaceID *uint          // Привязка к рабочему пространству
	CreatedAt   time.Time      // Время создания
	UpdatedAt   time.Time      // Время последнего обновления
	DeletedAt   gorm.DeletedAt `gorm:"index"` // Мягкое удаление (soft delete)
	Messages    []Message      // Сообщения чата
}

// PromptFile — модель файла с системным промптом.
// Хранит содержимое файла промпта, привязанного к определённому агенту.
// Файлы промптов загружаются из директории prompts/{agent_name}/.
//
// Поля:
//   - AgentName: имя агента, которому принадлежит промпт.
//   - Filename: имя файла промпта (например, "default.txt").
//   - Content: полный текст промпта.
type PromptFile struct {
	gorm.Model
	AgentName string // Имя агента-владельца промпта
	Filename  string // Имя файла промпта
	Content   string `gorm:"type:text"` // Содержимое промпта
}

// ModelToolSupport — кэш информации о поддержке инструментов (tool calling) для моделей.
// При первом использовании модели выполняется тестовый запрос с инструментами.
// Результат сохраняется в эту таблицу, чтобы не проверять повторно.
//
// Поля:
//   - ModelName: уникальное имя модели (первичный ключ).
//   - SupportsTools: результат проверки — true, если модель поддерживает tool calling.
//   - Family: семейство модели (llama, qwen, mistral и др.) — определяется автоматически.
//   - ParameterSize: размер модели (например, "8B", "70B") — определяется из метаданных Ollama.
//   - IsCodeModel: true, если модель специализирована на генерации кода.
//   - SuitableRoles: JSON-массив подходящих ролей агентов (["admin","coder","novice"]).
//   - RoleNotes: JSON-объект с пояснениями для каждой роли.
//   - CheckedAt: время последней проверки.
type ModelToolSupport struct {
	ModelName     string    `gorm:"primaryKey"` // Имя модели (первичный ключ)
	SupportsTools bool      // Поддерживает ли модель tool calling
	Family        string    // Семейство модели (llama, qwen, mistral и др.)
	ParameterSize string    // Размер модели (8B, 70B и др.)
	IsCodeModel   bool      // Специализация на коде
	SuitableRoles string    `gorm:"type:text"` // JSON-массив подходящих ролей
	RoleNotes     string    `gorm:"type:text"` // JSON-объект с пояснениями
	CheckedAt     time.Time // Время проверки
}

// ProviderConfig — модель настроек облачного LLM-провайдера.
// Хранит API-ключи и параметры подключения к облачным сервисам.
// Настройки сохраняются в БД и загружаются при старте сервиса.
//
// Поля:
//   - ProviderName: уникальное имя провайдера (openai, anthropic, yandexgpt, gigachat).
//   - APIKey: API-ключ для авторизации (зашифрован при хранении).
//   - BaseURL: базовый URL API (для прокси-серверов или кастомных эндпоинтов).
//   - FolderID: идентификатор каталога Yandex Cloud (только для yandexgpt).
//   - Scope: область доступа GigaChat (GIGACHAT_API_PERS, GIGACHAT_API_B2B и др.).
//   - Enabled: активирован ли провайдер (по умолчанию true).
type ProviderConfig struct {
	gorm.Model
	ProviderName       string `gorm:"uniqueIndex;not null"` // Уникальное имя провайдера
	APIKey             string // API-ключ
	BaseURL            string // Базовый URL (опционально)
	FolderID           string // Folder ID для Yandex Cloud
	Scope              string // Scope для GigaChat
	ServiceAccountJSON string `gorm:"type:text"`    // JSON сервисного аккаунта Yandex Cloud (authorized_key.json)
	Enabled            bool   `gorm:"default:true"` // Активирован ли провайдер
}

// SystemLog — модель записи системного лога.
// Все ошибки и важные события из всех микросервисов собираются в единую таблицу.
// Админ-агент имеет доступ к этим логам через инструмент view_logs
// и может анализировать ошибки для их исправления.
//
// Поля:
//   - Level: уровень лога — "error", "warn", "info", "debug".
//   - Service: имя микросервиса-источника (agent-service, tools-service, memory-service, api-gateway).
//   - Message: текст сообщения лога.
//   - Details: дополнительные данные (стек вызовов, параметры запроса и т.д.).
//   - Resolved: отметка о том, что ошибка исправлена (для отслеживания).
type SystemLog struct {
	gorm.Model
	Level    string `gorm:"index;not null"`     // Уровень: error, warn, info, debug
	Service  string `gorm:"index;not null"`     // Источник: agent-service, tools-service и др.
	Message  string `gorm:"type:text;not null"` // Текст сообщения
	Details  string `gorm:"type:text"`          // Доп. данные (стек, параметры)
	Resolved bool   `gorm:"default:false"`      // Исправлена ли ошибка
}

// Workspace — модель рабочего пространства (проекта).
// Пространство объединяет чаты и агентов для работы над конкретным проектом.
// Каждое пространство может иметь свою рабочую директорию на ПК пользователя.
//
// Поля:
//   - Name: отображаемое имя пространства (обязательное поле).
//   - Path: путь к рабочей директории проекта на ПК (например, "/home/art/AgentCore-NG/").
//   - Chats: связь один-ко-многим с чатами пространства.
//   - Agents: связь один-ко-многим с агентами, привязанными к пространству.
type Workspace struct {
	gorm.Model
	Name   string  `gorm:"not null"` // Имя пространства
	Path   string  // Путь к рабочей директории
	Chats  []Chat  // Чаты пространства
	Agents []Agent // Агенты пространства
}

// RagDocument — документ в базе знаний RAG.
// Хранит загруженные пользователем документы для семантического поиска.
//
// Поля:
//   - Title: название документа
//   - Content: текстовое содержимое документа
//   - Source: источник документа (user-upload, file, web и т.д.)
//   - ChunkIndex: индекс чанка (если документ разбит на части)
//   - TotalChunks: общее количество чанков документа
type RagDocument struct {
	gorm.Model
	Title       string `gorm:"not null"`  // Название документа
	Content     string `gorm:"type:text"` // Содержимое
	Source      string // Источник (user-upload, file, web)
	ChunkIndex  int    // Индекс чанка
	TotalChunks int    // Всего чанков
	WorkspaceID *uint  // Привязка к рабочему пространству
}
