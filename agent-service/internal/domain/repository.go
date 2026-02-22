package domain

// AgentRepository — интерфейс репозитория агентов.
// Обеспечивает операции создания, чтения, обновления и удаления для сущности Agent в хранилище данных.
type AgentRepository interface {
	// Получить агента по имени.
	// Возвращает nil и ошибку, если агент не найден в хранилище.
	GetByName(name string) (*Agent, error)
	// Сохранить или обновить агента в БД.
	Save(agent *Agent) error
	// Получить список всех агентов, сохранённых в хранилище.
	List() ([]Agent, error)
	// Создать агента по умолчанию (admin), если в БД нет ни одного.
	CreateDefault() error
}

// MessageRepository — интерфейс репозитория сообщений.
// Обеспечивает сохранение и выборку сообщений чата.
type MessageRepository interface {
	// Сохранить сообщение в БД.
	Save(msg *Message) error
	// Получить последние сообщения для указанного чата с ограничением по количеству (limit).
	ListByChat(chatID string, limit int) ([]Message, error)
}

// ChatRepository — интерфейс репозитория чатов (диалогов).
// Обеспечивает создание, получение, список и удаление чатов.
type ChatRepository interface {
	// Получить чат по уникальному идентификатору.
	GetByID(id string) (*Chat, error)
	// Создать новый чат.
	Create(chat *Chat) error
	// Получить список чатов.
	// Если workspaceID не nil — возвращает только чаты указанного рабочего пространства.
	List(workspaceID *uint) ([]Chat, error)
	// Удалить чат по ID.
	Delete(id string) error
}

// WorkspaceRepository — интерфейс репозитория рабочих пространств.
// Обеспечивает управление рабочими пространствами (проектами).
type WorkspaceRepository interface {
	// Получить рабочее пространство по ID.
	GetByID(id uint) (*Workspace, error)
	// Создать новое рабочее пространство.
	Create(ws *Workspace) error
	// Получить список всех рабочих пространств.
	List() ([]Workspace, error)
	// Удалить рабочее пространство по ID.
	Delete(id uint) error
}

// ProviderConfigRepository — интерфейс репозитория конфигураций LLM-провайдеров.
// Обеспечивает хранение и управление настройками облачных провайдеров (OpenAI, YandexGPT и т.д.).
type ProviderConfigRepository interface {
	// Получить конфигурацию провайдера по имени.
	GetByName(name string) (*ProviderConfig, error)
	// Сохранить или обновить конфигурацию провайдера.
	Save(cfg *ProviderConfig) error
	// Получить список всех конфигураций провайдеров.
	List() ([]ProviderConfig, error)
	// Удалить конфигурацию провайдера по имени.
	Delete(name string) error
}

// SystemLogRepository — интерфейс репозитория системных логов.
// Обеспечивает запись и чтение логов сервисов.
type SystemLogRepository interface {
	// Записать лог-запись в БД.
	Write(log *SystemLog) error
	// Получить список логов с фильтрацией по уровню, сервису и лимиту.
	List(level, service string, limit int) ([]SystemLog, error)
}

// RagDocumentRepository — интерфейс репозитория RAG-документов.
// Обеспечивает добавление, поиск и удаление документов в системе знаний агента.
type RagDocumentRepository interface {
	// Добавить документ (чанк) в RAG-хранилище.
	Add(doc *RagDocument) error
	// Выполнить семантический поиск документов по запросу.
	// Возвращает topK ближайших (наиболее релевантных) документов.
	Search(query string, topK int) ([]RagDocument, error)
	// Удалить документ по ID.
	Delete(id uint) error
	// Получить статистику: общее количество документов.
	Stats() (total int, err error)
}
