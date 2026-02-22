package domain

type AgentRepository interface {
	GetByName(name string) (*Agent, error)
	Save(agent *Agent) error
	List() ([]Agent, error)
	CreateDefault() error
}

type MessageRepository interface {
	Save(msg *Message) error
	ListByChat(chatID string, limit int) ([]Message, error)
}

type ChatRepository interface {
	GetByID(id string) (*Chat, error)
	Create(chat *Chat) error
	List(workspaceID *uint) ([]Chat, error)
	Delete(id string) error
}

type WorkspaceRepository interface {
	GetByID(id uint) (*Workspace, error)
	Create(ws *Workspace) error
	List() ([]Workspace, error)
	Delete(id uint) error
}

type ProviderConfigRepository interface {
	GetByName(name string) (*ProviderConfig, error)
	Save(cfg *ProviderConfig) error
	List() ([]ProviderConfig, error)
	Delete(name string) error
}

type SystemLogRepository interface {
	Write(log *SystemLog) error
	List(level, service string, limit int) ([]SystemLog, error)
}

type RagDocumentRepository interface {
	Add(doc *RagDocument) error
	Search(query string, topK int) ([]RagDocument, error)
	Delete(id uint) error
	Stats() (total int, err error)
}
