// Package domain — доменные сущности agent-service.
//
// Содержит основные бизнес-объекты системы: агенты, сообщения, чаты,
// рабочие пространства, конфигурации провайдеров, RAG-документы и системные логи.
// Этот пакет не зависит от инфраструктуры (БД, HTTP, внешние API).
package domain

import "time"

// Agent — агент (единственный в системе — admin).
// Содержит настройки LLM-модели, системный промпт и привязку к рабочему пространству.
type Agent struct {
	ID                uint      // Уникальный идентификатор агента
	Name              string    // Имя агента (admin)
	Prompt            string    // Системный промпт
	LLMModel          string    // Имя LLM-модели (например, llama3.1:8b)
	Provider          string    // Провайдер LLM (ollama, openai, yandexgpt и т.д.)
	SupportsTools     bool      // Поддерживает ли модель вызов инструментов (tool calling)
	Avatar            string    // Путь к аватару агента
	CurrentPromptFile string    // Путь к файлу текущего промпта
	WorkspaceID       *uint     // ID привязанного рабочего пространства (может быть nil)
	CreatedAt         time.Time // Дата создания
	UpdatedAt         time.Time // Дата последнего обновления
}

// Message — сообщение в чате.
// Может быть от пользователя (role=user), агента (role=assistant) или инструмента (role=tool).
type Message struct {
	ID         uint      // Уникальный идентификатор
	Role       string    // Роль: user, assistant, tool, system
	Content    string    // Текст сообщения
	ToolCallID string    // ID вызова инструмента (для ответов от tool)
	AgentID    uint      // ID агента, которому принадлежит сообщение
	ChatID     *string   // ID чата (может быть nil для системных сообщений)
	CreatedAt  time.Time // Дата создания
}

// Chat — диалог (сессия чата).
// Группирует сообщения пользователя и агента в рамках одной беседы.
type Chat struct {
	ID          string    // Уникальный идентификатор (UUID)
	Name        string    // Название чата (автогенерируемое или пользовательское)
	UserID      string    // ID пользователя (в текущей версии — единственный admin)
	WorkspaceID *uint     // ID рабочего пространства (может быть nil)
	CreatedAt   time.Time // Дата создания
	UpdatedAt   time.Time // Дата последнего обновления
}

// Workspace — рабочее пространство.
// Логическая группировка файлов, чатов и настроек для конкретного проекта.
type Workspace struct {
	ID        uint      // Уникальный идентификатор
	Name      string    // Название рабочего пространства
	Path      string    // Путь к директории на файловой системе
	CreatedAt time.Time // Дата создания
	UpdatedAt time.Time // Дата последнего обновления
}

// ProviderConfig — конфигурация облачного LLM-провайдера.
// Хранит API-ключи и параметры подключения к провайдерам (OpenAI, YandexGPT и т.д.).
type ProviderConfig struct {
	ID                 uint   // Уникальный идентификатор
	ProviderName       string // Имя провайдера (openai, yandexgpt, anthropic и т.д.)
	APIKey             string // API-ключ для аутентификации
	BaseURL            string // Базовый URL API (если отличается от стандартного)
	FolderID           string // ID каталога (для YandexGPT)
	Scope              string // OAuth scope (для YandexGPT)
	ServiceAccountJSON string // JSON ключ сервисного аккаунта (для YandexGPT)
	Enabled            bool   // Активен ли провайдер
}

// RagDocument — документ в RAG-хранилище.
// Используется для хранения чанков файлов, загруженных в систему знаний агента.
type RagDocument struct {
	ID          uint      // Уникальный идентификатор
	Title       string    // Заголовок документа
	Content     string    // Содержимое (текст чанка)
	Source      string    // Источник (имя файла, URL и т.д.)
	ChunkIndex  int       // Индекс чанка в исходном документе
	TotalChunks int       // Общее количество чанков исходного документа
	WorkspaceID *uint     // ID рабочего пространства (может быть nil)
	CreatedAt   time.Time // Дата создания
}

// SystemLog — запись системного лога.
// Используется для хранения событий, ошибок и предупреждений сервисов.
type SystemLog struct {
	ID        uint      // Уникальный идентификатор
	Level     string    // Уровень лога: info, warning, error, critical
	Service   string    // Имя сервиса-источника (agent-service, tools-service и т.д.)
	Message   string    // Текст сообщения
	Details   string    // Дополнительные детали (стек вызовов, параметры запроса)
	Resolved  bool      // Решена ли проблема
	CreatedAt time.Time // Дата создания
}
