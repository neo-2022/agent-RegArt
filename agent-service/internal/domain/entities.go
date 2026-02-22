package domain

import "time"

type Agent struct {
	ID                uint
	Name              string
	Prompt            string
	LLMModel          string
	Provider          string
	SupportsTools     bool
	Avatar            string
	CurrentPromptFile string
	WorkspaceID       *uint
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Message struct {
	ID         uint
	Role       string
	Content    string
	ToolCallID string
	AgentID    uint
	ChatID     *string
	CreatedAt  time.Time
}

type Chat struct {
	ID          string
	Name        string
	UserID      string
	WorkspaceID *uint
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Workspace struct {
	ID        uint
	Name      string
	Path      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProviderConfig struct {
	ID                 uint
	ProviderName       string
	APIKey             string
	BaseURL            string
	FolderID           string
	Scope              string
	ServiceAccountJSON string
	Enabled            bool
}

type RagDocument struct {
	ID          uint
	Title       string
	Content     string
	Source      string
	ChunkIndex  int
	TotalChunks int
	WorkspaceID *uint
	CreatedAt   time.Time
}

type SystemLog struct {
	ID        uint
	Level     string
	Service   string
	Message   string
	Details   string
	Resolved  bool
	CreatedAt time.Time
}
